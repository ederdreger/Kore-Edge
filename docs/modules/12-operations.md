# Fase 12 — Operação

## Objetivo e aceite

Alertas, backoff, backup da configuração, atualização e rollback de laboratório.
Falhas de um canal ou de preparação de release não devem produzir loops infinitos
nem interromper streams sadios. A entrega não instala serviços, não modifica rede,
não conecta à matriz/Flussonic e não implementa a integração da Fase 13.

## Funcionamento

`cmd/edge-ops` prepara versões offline em um diretório privado, absoluto e externo
ao checkout. Execute a partir da raiz do checkout para aplicar a barreira de paths.
Usa somente a biblioteca padrão e as validações existentes do projeto.

- `stage`: lê um binário revisado e um YAML válido, exige o SHA-256 esperado do
  binário e publica um release com cópia exata de ambos e manifesto de integridade.
  Não altera a seleção. Repetir os mesmos bytes devolve a mesma revisão.
- `status`: informa somente versão do schema, geração e IDs atual/anterior.
  É estado de seleção, não medição de saúde do processo.
- `select`: valida a integridade do destino e troca a seleção **para a próxima
  inicialização**. Exige `expected-generation` igual à geração observada.
- `rollback`: valida o release anterior e o seleciona sob a mesma regra de geração.
  Sem versão anterior ou com backup corrompido, recusa sem alterar a seleção.

Cada revisão contém `agent`, `config.yaml` e `manifest.json`. O ID é SHA-256 dos
digests de binário/configuração. A seleção guarda `current`, `previous` e geração
monotônica; selecionar novamente o atual não aumenta a geração. Uma troca com
geração antiga falha, inclusive em concorrência. Rollback troca atual/anterior e
incrementa a geração; restaurar versões anteriores às duas selecionadas requer
`select` explícito do ID revisado.

O agente aceita exatamente um entre `-config` e `-release-dir`. No segundo modo,
verifica manifesto, configuração, binário armazenado e SHA-256 do executável que
realmente está rodando. Executar um binário diferente da seleção é recusado antes
de abrir o listener. Não há download, execução do candidato pelo preparador,
hot-swap, reload do listener ou reinício de processos. Alterar a seleção não afeta
uma instância em execução. O modo `-config` permanece disponível e não lê releases.

No Ubuntu, o binário armazenado tem modo 0700 e pode ser iniciado explicitamente
como `<RELEASE_DIR>/<ID>/agent -release-dir <RELEASE_DIR>`. No laboratório Windows,
use o `.exe` revisado correspondente com `-release-dir`; o arquivo armazenado
`agent` é a cópia de backup e não recebe extensão automaticamente.

O SHA-256 esperado precisa vir de fonte de confiança revisada. Calcular o hash de
um arquivo suspeito não o autentica. Não há assinatura de fornecedor nem serviço
automático de atualização nesta fase. A verificação não afirma compatibilidade
funcional de um executável: ela requer teste isolado antes da seleção operacional.

## Armazenamento, backup e falhas

Diretórios privados 0700, arquivos 0600 e executável 0700 no Linux; symlinks e
arquivos não regulares são recusados. Conteúdo de configuração, paths de clientes,
credenciais e erros de parsing não são impressos. A saída da CLI contém apenas
IDs/digests e geração. Os arquivos reais ficam fora do checkout. Não coloque
políticas reais, certificados, tokens ou chaves nos exemplos versionados.

No Windows, as DACLs não são verificadas pelo módulo de segurança; os testes
locais não certificam a privacidade POSIX exigida para o Ubuntu.

O backup cobre o YAML e o binário, não chaves privadas, política mTLS, estado de
provisionamento, catálogo em memória ou segmentos HLS. Os arquivos externos
referenciados pelo YAML precisam ser preservados e revisados separadamente.
Rollback não pode revalidar certificados expirados nem restaurar permissões
revogadas. A validação do YAML não substitui a checagem desses recursos no startup.

Limites: até 8 revisões e 128 MiB por binário, 64 KiB por configuração. Diretórios
temporários incompletos também consomem a quota. Não há remoção automática de
revisões, rotação de backups ou retry automático de escrita. Ao atingir o limite,
o operador revisa retenção fora do processo; esta ferramenta não apaga backups.
O staging usa buffer limitado de até 128 MiB; dimensione memória para a CLI.
A verificação de binários no startup é feita em streaming.

Um lock exclusivo impede processos concorrentes; mutex protege chamadas simultâneas
na mesma instância. Publicação usa arquivo/diretório temporário, sync e rename no
mesmo filesystem. No Linux, sincroniza também os diretórios. No Windows, rename
não tem a mesma garantia de atomicidade/durabilidade POSIX e diretórios não são
fsyncados: a homologação de crash deve ocorrer no filesystem Ubuntu de destino.
Se houver erro após rename/sync, releia `status` antes de repetir uma ação; o
commit pode ter ocorrido. Arquivo temporário próprio é limpo em falhas normais;
crash pode deixar lock ou temporário, nunca removidos automaticamente no startup.
Não remova um lock sem comprovar que nenhum proprietário continua usando o store.
Administradores da conta/diretório integram a fronteira de confiança; hashes não
protegem contra alguém que possa substituir simultaneamente manifestos e binários.

## Backoff e alertas

O Stream Engine continua supervisionando cada canal separadamente. A política
padrão permite uma tentativa inicial e até 3 reinícios: esperas de 250 ms, 500 ms
e 1 s. A espera exponencial tem teto de 30 s, é cancelável e não zera o orçamento
após receber pacotes. Ao esgotar tentativas, o canal fica `failed`; não há restart
global nem tentativa infinita. Ação manual posterior de início é explícita.

Monitoramento usa as observações reais do supervisor, com origem sintética quando
essa for a fonte. Novos alertas locais:

- `stream_restarting`: warning em backoff ou reinício em andamento.
- `authorization_lost`: critical quando o supervisor perde autorização.
- `channel_failed`: critical existente para falha definitiva.

Um alerta ativo por condição/canal; resolução fica no histórico limitado a 128.
Não há envio de e-mail/webhook, contato externo nem reinício disparado por alerta.
Falhas muito curtas entre amostras de 1 s podem não gerar alerta de backoff;
o estado final `failed` permanece observável. O painel existente apresenta esses
alertas via `/api/v1/monitoring`, mantendo layout e acesso somente leitura.

## Procedimento futuro de atualização e rollback

Nenhuma operação abaixo foi executada sobre um serviço. Todas as variáveis são
placeholders a preencher somente após revisão do host de laboratório, conta,
caminhos, endereço/porta e janela operacional. Não usar hosts de produção.
Não instalar/reiniciar systemd sem autorização humana específica.

### Pré-verificação em Ubuntu (somente leitura)

```sh
id -u
stat -c '%a %U %G %n' "$RELEASE_DIR" "$CONFIG_PATH" "$CANDIDATE"
df -h "$RELEASE_DIR"
ss -ltn
sha256sum "$CANDIDATE"
"$OPS" -action status -directory "$RELEASE_DIR"
```

Confirmar usuário não root, origem confiável do digest, disco para o candidato,
permissões privadas, SAN/cadeia/validade dos certificados, política de funções,
recursos externos do YAML e endereço/porta previamente aprovados. O comando
`status` adquire um lock temporário; só é permitido no store privado revisado.
Guardar IDs/generation e caminho do binário atual. Na migração inicial, preparar
e selecionar primeiro uma baseline recuperável; sem ela não há rollback anterior.

### Preparar e revisar (sem afetar o processo atual)

```sh
"$OPS" -action stage -directory "$RELEASE_DIR" -binary "$CANDIDATE" \
  -sha256 "$EXPECTED_SHA256" -config "$CONFIG_PATH"
```

Guardar o ID retornado. Validar o candidato em laboratório separado, com seu
próprio store/YAML, listener explicitamente aprovado e streams sintéticos. Não
reutilizar paths HLS/provisionamento/auditoria de uma instância ativa. Verificar
health/readiness, mTLS/RBAC, digest, qualidade disponível e limitações do HLS.
Somente depois, selecionar a revisão revisada:

```sh
"$OPS" -action select -directory "$RELEASE_DIR" -revision "$REVIEWED_ID" \
  -expected-generation "$REVIEWED_GENERATION"
```

A seleção é preparatória. A troca de processo é manual, depende da autorização
operacional e pode interromper os streams da instância reiniciada. Esta entrega
não garante atualização sem interrupção nem implementa migração de sessões.

### Validação após eventual troca autorizada

```sh
"$OPS" -action status -directory "$RELEASE_DIR"
curl --fail --cacert "$SERVER_CA" --cert "$CLIENT_CERT" --key "$CLIENT_KEY" "$BASE_URL/healthz"
curl --fail --cacert "$SERVER_CA" --cert "$CLIENT_CERT" --key "$CLIENT_KEY" "$BASE_URL/readyz"
```

Confirmar release/configuração corretos, função da sessão, alertas e tráfego
sintético esperado. Ausência de alerta não certifica qualidade de mídia. Não usar
`--insecure`, não copiar credenciais para logs e não deduzir disponibilidade de
streams apenas de `/readyz`.

### Rollback

```sh
"$OPS" -action status -directory "$RELEASE_DIR"
"$OPS" -action rollback -directory "$RELEASE_DIR" \
  -expected-generation "$REVIEWED_GENERATION"
```

Revisar o ID selecionado e iniciar o executável anterior correspondente somente
na janela/autorização acordada. Repetir validações de saúde, segurança e streams.
Rollback recusado por corrupção deixa a seleção inalterada; recuperar uma cópia
privada confiável e revisada, sem desabilitar integridade ou autenticação.

## Testes e resultados

```text
go test ./... -count=1
go vet ./...
go test -race ./...
go build -o bin/edge-ops ./cmd/edge-ops
go build -o bin/edge-agent ./cmd/edge-agent
```

Testes cobrem staging idempotente, backup exato, checksum incorreto, YAML inválido,
quota, limites de tamanho, geração concorrente, lock, corrupção de manifesto/
binário/configuração/seleção, rollback e verificação do executável no startup.
Supervisor: canal saudável progride enquanto outro esgota tentativas; backoff
cancela prontamente, escala e respeita teto. Alertas são deduplicados e resolvidos.
Não são testes de atualização de serviço real nem de resiliência a power loss.

Validação em 25/10/2026, Windows, Go 1.24.9 portátil:

- `go test ./... -count=1`: pacotes aprovados. O caso de symlink em operações foi
  pulado porque o Windows não permitiu criar o link; validar nativamente no Ubuntu.
- `go vet ./...`: aprovado no Windows e na análise cruzada Linux/amd64.
- `gofmt -l cmd internal`: sem arquivos pendentes.
- Painel: `npm.cmd run check` aprovado e `npm.cmd test` com 11 testes aprovados.
  Os assets do painel não mudaram nesta fase, portanto não foi necessário rebuild.
- `bin/edge-agent-phase12.exe`, `bin/edge-ops-phase12.exe` e as versões
  `*-phase12-linux-amd64` compiladas; testes de operações Linux compilados em
  `bin/operations-linux.test`.
- `go test -race ./...`: não executou, pois este ambiente está sem CGO.
- Pendentes: execução nativa Ubuntu, detector de corrida, validação POSIX/symlinks,
  crash durability e troca/rollback de serviço em laboratório autorizado.

Não houve substituição do agente local, instalação/reinício de serviços ou acesso
à matriz. Stores de teste foram temporários, com binários sintéticos e sem secrets.
A Fase 13 requer autorização.
