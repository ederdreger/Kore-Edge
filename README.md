# Kore Edge — Plano mestre de desenvolvimento

## Cadastro de canais pelo link da matriz

Em **Canais → Adicionar canal**, cole a saída do Flussonic ou de outra matriz:
HTTP MPEG-TS (`/mpegts` ou `.ts`), HLS (`.m3u8`), UDP MPEG-TS ou SRT caller.
O protocolo aparece ao digitar; **Verificar sinal** faz uma leitura curta e
**Salvar canal** prepara a recepção sem conectar. Depois use **Iniciar recepção**
para manter o fluxo aberto, compartilhado com o player e as saídas UDP/HLS.
Links sem extensão podem ser identificados pelo teste. UDP exige interface e IP
local confirmados; o stream ID Flussonic `#!::r=CANAL,m=request` é preservado.

Cadastro, teste e operação dos canais usam a sessão administrativa já autenticada,
com CSRF e auditoria, **sem solicitar novamente a senha**. Espectadores continuam
somente leitura. Credenciais das origens ficam em armazenamento privado e não
aparecem na listagem. Os canais do catálogo automático continuam sujeitos às
permissões da matriz. Consulte o [passo a passo e limites](docs/modules/20-manual-channels.md).

## Recepção contínua, SRT e saídas reais

**Canais** concentra cadastro, origem, recepção e saídas UDP/HLS por canal. O menu duplicado **Saídas** foi removido; links antigos abrem **Canais**. Configure um canal, salve a preparação e só então confirme **Iniciar
recepção**. O fluxo permanece aberto com o player fechado e é compartilhado com
as saídas autorizadas. O painel informa tempo até o primeiro sinal, bitrate
medido, continuidade TS, falhas e permissões; métricas ausentes não são simuladas.

SRT recebe da matriz pelo WireGuard, com criptografia e autorização por parceiro.
**20000/UDP é apenas proposta para revisão no Kore Play**. Salvar não inicia o
listener. Multicast não escolhe interface, IP, grupo ou porta automaticamente.
HLS local exige liberação na central e H.264/AAC verificados, sem transcodificar.
As novas telas mantêm a organização visual das referências e se adaptam à largura
da tela. Veja o [passo a passo de operação](docs/DISTRIBUICAO-AO-VIVO.md) e o
[contrato técnico](docs/modules/19-live-distribution.md).

## Canais autorizados do Kore Play

No Kore Play, **Canais → Parceiros Kore Edge** permite escolher o parceiro e
salvar o parceiro e abrir automaticamente a seleção de canais e protocolos.
Parceiros existentes têm botão explícito para editar a seleção e publicá-la.
No Edge, **Canais → Canais do Kore Play** busca a lista,
mostra HTTP MPEG-TS/HLS disponíveis na origem, teste curto de sinal e **Play**
por canal. O mini player abre uma prévia autenticada por vez, com pausa/fechamento
encerrando a conexão e limite de cinco minutos. Codecs dependem do navegador.
A página de canais usa a largura disponível, com resultados de teste quebrando
linha e apresentação em cartões nas telas pequenas.
Sem canais de demonstração, conversão automática ou liberação entre parceiros.
Veja o [passo a passo](docs/CANAIS-KORE-PLAY.md) e o
[contrato e limites](docs/modules/18-partner-channels.md).

**WireGuard** abre na tela principal com estado online/offline/indisponível,
horário da última resposta e consulta automática a cada cinco segundos. Operações
já aplicadas ficam bloqueadas; detalhes têm retorno explícito à tela principal.

Integração guiada: comece em **Edge → WireGuard**, copie a identificação e siga
as etapas nos dois painéis. Consulte o [passo a passo simples](docs/INTEGRACAO-SIMPLES.md).

Repositório público: [ederdreger/Kore-Edge](https://github.com/ederdreger/Kore-Edge).

## Credenciais WireGuard para Kore Play

No menu **WireGuard**, administradores podem gerar a identidade do Edge e preparar
os dados do peer para cadastro no Kore Play. A chave privada permanece no servidor;
somente chave pública e bloco `[Peer]` são apresentados. Exige senha atual e dados
de rede confirmados, sem ativar túnel ou alterar rotas. Atualizações preservam a
identidade já gerada. Veja [passo a passo e limites](docs/modules/16-wireguard-credentials.md).

## Login, usuários e atualização de instalação existente

O painel usa sessão individual HTTPS e senhas bcrypt. Instalações novas ficam
sem canais, catálogo de exemplo ou tráfego simulado. O perfil `production` exige
contas e rejeita módulos sintéticos/mock; isso não substitui a homologação de
transporte real descrita na Fase 14.

Para uma instalação já em funcionamento, revise e aplique a atualização guiada:

```sh
git pull --ff-only
bash scripts/update-panel.sh --plan
sudo bash scripts/update-panel.sh --apply
```

Ela mostra endereços detectados e pede confirmação antes de reiniciar somente os
dois serviços Kore Edge. Preserva certificado e o administrador atual, cria backup
e faz rollback se a validação falhar. Dados anteriores são preservados; a nova
configuração de painel desabilita catálogo, streams e integrações de teste.
O teste HTTPS aguarda o listener iniciar e distingue indisponibilidade, erro TLS
e resposta HTTP inesperada, mantendo a verificação do certificado.
Também valida HTML e JavaScript do login sem sessão, enviando o Origin público
como o navegador. A origem é conferida antes da normalização para o guard local.
Não execute o instalador novo sobre a instalação existente.
Veja [passo a passo](docs/INSTALACAO-SIMPLES.md) e [contrato de contas](docs/modules/15-accounts.md).

## Instalação Ubuntu Server 24.04

Use o **assistente de instalação**, que solicita os dados por etapas:

```sh
git clone https://github.com/ederdreger/Kore-Edge.git
cd Kore-Edge
bash scripts/setup.sh
```

Ele confirma a instalação limpa, pergunta o que instalar, **domínio ou IP da VPS**,
porta HTTPS, usuário do painel e se deve iniciar o serviço. O acesso externo usa
HTTPS com login individual; a API permanece privada. O certificado inicial é local
e causa aviso de confiança no navegador. Ao final mostra o resumo: escolha **1** para
instalar, **2** para apenas simular ou **3** para cancelar. Se necessário, o sudo
solicita a senha do seu usuário administrador.

IPs privados são aceitos e classificados automaticamente; a lista de interfaces
detectadas pode ser usada com confirmação. IP privado exige LAN/VPN ou NAT para acesso.

Já baixou o projeto? Dentro da pasta, execute `git pull --ff-only` e depois
`bash scripts/setup.sh`.

Se uma tentativa falhou nos testes após instalar pacotes, consulte a
[retomada da instalação](docs/INSTALACAO-SIMPLES.md) antes de remover arquivos.
Para erro Nginx `mkdir() /var/lib/nginx/fastcgi ... Permission denied` após os
testes, existe um [reparo específico do gateway](docs/INSTALACAO-SIMPLES.md),
preservando senha, certificado e configuração já instalados.
O reparador também trata a primeira inicialização de unidades ainda não carregadas.
Se o navegador recusar o login, use `sudo bash scripts/panel-access.sh` para ver
o usuário cadastrado, conferir a senha ou redefini-la com entrada oculta.

Veja o [passo a passo simples](docs/INSTALACAO-SIMPLES.md).
[Documentação técnica e rollback](docs/INSTALL.md) para operadores avançados.
O sistema permanece em laboratório, **sem homologação para produção**.

## Implementação — execução local

Fase 14: suíte de aceitação em `tests/acceptance` para carga HTTP limitada, perda
TS simulada, recuperação isolada e reinicialização de processos auxiliares do core.
Checklist atual: **NO-GO para implantação**. O ensaio repetido reproduziu falha de
concorrência no provisionamento; validações Ubuntu/rede/players continuam pendentes.
Veja [procedimento da Fase 14](docs/modules/14-acceptance.md) e
[evidências e checklist](docs/acceptance/phase14-report.md).

Fase 13: contrato proposto para Kore-Player TV e sincronização opcional de catálogo
assinado por mock em memória, sem sockets externos. Revisão, parceiro, assinatura
e validade são verificados; falhas conservam o catálogo ainda válido. Estado em
`GET /api/v1/integration`, com tentativas limitadas e encerramento coordenado.
Não há compatibilidade homologada com a API real nem acesso à matriz/Flussonic.
Veja [contrato e limites da Fase 13](docs/modules/13-integration.md).

Fase 12: releases offline com backup privado da configuração, verificação SHA-256,
seleção por geração e rollback para a próxima inicialização. A ferramenta
`cmd/edge-ops` não reinicia processos nem aplica mudanças ao host. O agente aceita
`-release-dir` como alternativa a `-config`, verificando o executável selecionado.
Backoff permanece limitado por canal, com alertas de reinício e perda de autorização.
Veja [procedimento, testes e limites da Fase 12](docs/modules/12-operations.md).

Fase 11: segurança do plano de controle com mTLS/TLS 1.3, RBAC por certificado,
arquivos privados e auditoria administrativa limitada. Sem configuração mTLS,
o acesso local é **somente leitura**; as mutações das fases anteriores passam
a exigir certificado autorizado. O painel informa a função da sessão.
Veja [configuração, validação e limites da Fase 11](docs/modules/11-security.md).
Nenhuma mudança de rede, firewall ou serviço do host foi aplicada.

Fase 10: provisionamento local de laboratório com token aleatório de uso único,
hash persistido, expiração/revogação, identidade P-256 e proposta de configuração
sem aplicação automática. Emissão/revogação via ferramenta offline
`cmd/edge-provision`; consulta/registro em `/api/v1/provisioning` e
`/api/v1/provisioning/register`. Desabilitado por padrão. Veja
[procedimento e limites da Fase 10](docs/modules/10-provision.md).

Dashboard React/TypeScript incorporada ao agente em `/`, com login animado,
navegação, busca e filtros. A demonstração visual foi removida: `?demo=1`
não ativa exemplos. Métricas mostram observações do agente ou indisponibilidade.
O menu **Usuários** permite criar contas e alterar senhas, inclusive do administrador.
Build frontend: `npm --prefix web ci` e `npm --prefix web run build`, antes do
build Go. Veja [execução e limites da Fase 09](docs/modules/09-web.md).

Fase 08: monitoramento em `GET /api/v1/monitoring` e `/metrics` (Prometheus
0.0.4), com origem/janelas explícitas, taxas RX/TX, bitrate, CC/TEI/PCR quando
disponíveis e alertas locais testados. Não instala Prometheus nem envia avisos
externos. Veja [contrato e limites da Fase 08](docs/modules/08-metrics.md).

Fase 07: HLS opcional com fonte sintética H.264/AAC, verificação por FFprobe,
segmentação FFmpeg com `-c copy`, playlists rotativas, limpeza e acesso condicionado
à permissão `hls`. Player local em `/player/<channel_id>` e mídia em
`/hls/<channel_id>/index.m3u8`. Exige ferramentas externas com caminhos explícitos;
fica desabilitado por padrão. Veja [execução e resultados da Fase 07](docs/modules/07-hls.md).

Fase 06: saída multicast IPv4 opcional por canal com interface/IP/grupo/porta
explícitos, TTL 1, datagramas de sete pacotes TS e permissão UDP do catálogo.
Por padrão não envia tráfego. Inventário e dry-run em
`/api/v1/multicast/interfaces` e `/api/v1/multicast/plan`.
Veja [validação e homologação pendente da Fase 06](docs/modules/06-multicast.md).

Fase 05: Stream Engine opcional (`stream.enabled: true`, exige confiança do
catálogo) com processo filho por canal, ingest MPEG-TS sintético por pipe,
reinícios limitados e métricas medidas da fonte sintética.
`GET/POST /api/v1/streams` e `DELETE /api/v1/streams/<id>` controlam somente
esses processos locais. Veja [o contrato da Fase 05](docs/modules/05-stream.md).

Fase 04: catálogo assinado Ed25519 por parceiro, com canais e permissões de
saída, validade, revisões monotônicas e atualização atômica em memória.
`GET/PUT /api/v1/catalog` permanece desabilitado até configurar parceiro e
chave pública confiável no YAML. Veja [o contrato da Fase 04](docs/modules/04-catalog.md).

Fase 03: `GET /api/v1/wireguard` consulta peers/handshakes em leitura no Linux;
`POST /api/v1/wireguard/plan` gera somente dry-run e rollback público, sem aplicar
configurações. Veja [o contrato da Fase 03](docs/modules/03-wireguard.md).

Fase 02: `GET /api/v1/diagnostics` expõe CPU, RAM, disco raiz, contadores de
NICs, rotas e sensores Linux, com timestamps, origem e disponibilidade explícita.
No Windows, os grupos ficam indisponíveis. Consulte
[`docs/modules/02-linux.md`](docs/modules/02-linux.md) para unidades, limites,
testes e referência visual da dashboard fornecida pelo usuário. A UI permanece
na Fase 09; a imagem orientará sua reprodução visual.

O core contém a API de controle em modo `laboratory` e diagnóstico Linux em
leitura. O ingest é exclusivamente sintético local; não há integração com
matriz, Flussonic ou alteração de configuração do host.
Requisitos: Go 1.22 ou superior e acesso ao módulo `gopkg.in/yaml.v3` v3.0.1
na primeira resolução de dependências. Make é opcional; os comandos Go abaixo
também podem ser executados diretamente no PowerShell.

Estrutura: `cmd/edge-agent` (entrada), `internal/config` (configuração),
`internal/api` (HTTP), `internal/app` (ciclo de vida), `configs` (exemplo),
`deploy/systemd` (modelo inativo). Os módulos de mídia das próximas fases
permanecem fora desta implementação.

Revise o endereço e a porta em `configs/edge.example.yaml` antes de executar.
O exemplo usa loopback e não representa descoberta ou confirmação da rede.
Não há bind padrão: `-config` é obrigatório e somente IP literal de loopback
com porta explícita entre 1 e 65535 é aceito.

```sh
go mod download
go vet ./...
go test ./...
go test -race ./...
go build -o bin/edge-agent ./cmd/edge-agent
go run ./cmd/edge-agent -config configs/edge.example.yaml
```

Em outro terminal, após confirmar o endereço do exemplo:

```sh
curl --fail http://127.0.0.1:18080/healthz
curl --fail http://127.0.0.1:18080/readyz
```

`/healthz` informa que o processo HTTP responde; `/readyz` retorna 200 quando
o core está pronto e 503 antes da prontidão ou durante encerramento. Ambos
aceitam GET e HEAD; outros métodos retornam 405 e caminhos desconhecidos, 404.
A prontidão refere-se exclusivamente ao core; não afirma disponibilidade de
streams, VPN ou matriz. Não são publicadas métricas simuladas.

A configuração YAML tem schema `version: 1`, modo `laboratory` e `http.listen`.
Campos desconhecidos, documentos múltiplos, chaves duplicadas e arquivos acima
de 64 KiB são rejeitados. Configuração inválida no início encerra o processo.
O Store interno publica snapshots completos sob mutex e preserva o último
válido se uma substituição for rejeitada. Não há endpoint de atualização nem
reload automático. A Fase 12 oferece backup e seleção offline de releases;
para alterar o listener, valide o arquivo e planeje a troca manual do processo.
Guarde cópias privadas fora do Git e nunca inclua segredos nas configurações.

Logs JSON vão para stderr sem conteúdo do YAML. O HTTP possui limites de
cabeçalhos e timeouts; SIGINT/SIGTERM retira a prontidão e permite até cinco
segundos para encerramento. Ctrl+C encerra a execução local. Rollback local:
encerre o processo, restaure o arquivo anterior e execute novamente; valide
os dois endpoints. Nenhum serviço externo precisa ser reiniciado.

`make check` executa vet, verificação de gofmt e testes com detector de corrida;
`make build` gera o binário. No Windows, `-race` exige compilador C compatível.
Verifique formatação com `gofmt -l cmd internal` (saída esperada: vazia).

O arquivo `deploy/systemd/kore-edge.service` é somente um modelo, não instalado.
Seu uso futuro exige aprovação operacional específica, revisão de caminhos,
usuário sem privilégios e configuração. Pré-verificação futura: confirmar
identidade do host de laboratório, existência do usuário, permissões de leitura,
disponibilidade do endereço/porta e ausência de serviço conflitante; registrar
estado anterior. Rollback: restaurar unidade/binário/configuração anteriores e
o estado de ativação previamente registrado. Validação posterior: conferir
estado, logs e os dois endpoints no endereço aprovado. Comandos operacionais
concretos deverão ser apresentados com esses dados antes de qualquer instalação.

Servidor físico Ubuntu 24.04 em laboratório isolado. Distribuição UDP multicast e HTTP/HLS. Desenvolvimento no VSCode com Codex.

## Regras inegociáveis
- NÃO tocar em servidores de produção, Flussonic, MikroTik, Cisco, VLAN 18 ou grupos existentes.
- Antes de alterar rede, firewall, rotas, WireGuard ou systemd: apresentar plano, impacto, comandos de inspeção e rollback; solicitar aprovação explícita.
- Nunca executar comandos destrutivos automaticamente, nem reiniciar serviços externos.
- Começar com fonte sintética e endereços RFC1918 de laboratório; não assumir NIC/IP sem descoberta.
- O túnel WireGuard carrega UDP **unicast** entre matriz e Edge; multicast só na LAN do parceiro.
- Não incluir segredos no Git. Não alegar que medições simuladas são reais.
- Separar plano de controle (API, catálogo) do plano de dados (recepção e distribuição).

## Stack sugerida
- Agent/API: Go (stdlib preferencial); serviços systemd.
- Stream: FFmpeg e TSDuck; Nginx para HLS.
- UI: React + TypeScript + Vite, Tailwind CSS; consumir API Go.
- Configuração: YAML validado, com versionamento e último estado válido; sem banco local obrigatório.
- Métricas: Prometheus/OpenMetrics; logs estruturados.

## Etapas
01. **Core e estrutura** — API Go, configuração versionada, healthcheck, serviços systemd. Aceite: API inicia em modo laboratório; testes unitários e lint passam.
02. **Diagnóstico Linux** — CPU, RAM, disco, NICs, rotas e sensores em leitura. Aceite: Nenhum comando altera o host; métricas têm timestamp.
03. **WireGuard** — Inventário de peers, configuração gerada, handshake, rotas permitidas. Aceite: Dry-run e rollback testados; sem alterar rotas default.
04. **Catálogo** — Manifesto assinado de canais e permissões por parceiro. Aceite: Rejeita canais não autorizados e configurações inválidas.
05. **Stream Engine** — Ingest MPEG-TS de fonte sintética, supervisão de processos. Aceite: Reinício limitado; métricas de fluxo verificadas.
06. **Multicast UDP** — Saída por interface explícita, TTL, endereço e porta. Aceite: Tráfego restrito à VLAN de laboratório; sem vazamento.
07. **HTTP/HLS** — HLS com segmentos, playlist e Nginx; compatibilidade de codecs. Aceite: Player reproduz canal sintético e segmentos expiram.
08. **Monitoramento** — Bitrate, PCR/CC/TEI quando disponíveis, RX/TX e disponibilidade. Aceite: Métricas identificam origem e janela; alertas testados.
09. **Painel web** — React/TypeScript, dashboard e páginas de canais/Edge. Aceite: Navegação acessível; sem ações perigosas automáticas.
10. **Provisionamento** — Registro por token de uso único, identidade e configuração. Aceite: Expiração e revogação de token validadas.
11. **Segurança** — mTLS ou autenticação forte, RBAC, secrets, auditoria. Aceite: Sem segredos em logs; privilégio mínimo.
12. **Operação** — Alertas, backoff, backup de config, atualização e rollback. Aceite: Falhas não provocam loops nem interrompem streams sadios.
13. **Integração matriz** — Contrato API com Kore-Player TV e sincronização de catálogo. Aceite: Integração por mocks antes de acessar matriz.
14. **Homologação** — Testes de carga, perda de pacotes, restart e documentação. Aceite: Checklist de go/no-go aprovado em laboratório.

## Convenções de entrega por etapa
1. Ler `AGENTS.md` e `docs/ARCHITECTURE.md`.
2. Criar branch `feat/<modulo>`; explicar plano e arquivos afetados.
3. Implementar somente o escopo da etapa, com testes e documentação.
4. Executar lint/testes disponíveis, relatar resultados reais e limitações.
5. Aguardar aprovação antes de passar ao módulo seguinte.

## Primeiro prompt ao Codex
Leia `AGENTS.md`, `README.md`, `docs/ARCHITECTURE.md` e `docs/modules/01-core.md`. Implemente apenas a Fase 01 em modo laboratório, sem comandos privilegiados ou alterações no host. Crie estrutura de monorepo, API Go com `/healthz` e `/readyz`, configuração de exemplo, testes unitários, Makefile e documentação para execução local. Antes de codificar, apresente plano e arquivos que serão criados.

## Conexão WireGuard automática (opcional)

O assistente sincroniza peers por código temporário e permite revisar/ativar o
túnel real com auxiliar local restrito. Salvar dados continua apenas preparando
a configuração. A cópia de dados públicos confirma sucesso ou orienta Ctrl+C.
Veja [habilitação, pré-verificação, aplicação e rollback](docs/modules/17-wireguard-connection.md).

A janela de configuração abre imediatamente, com carregamento e nova tentativa em caso de falha. Inventário de rede indisponível bloqueia UDP, sem impedir preparar entradas HTTP/HLS/SRT. Nenhuma permissão de rede do serviço foi ampliada.
