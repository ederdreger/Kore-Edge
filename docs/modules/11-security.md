# Fase 11 — Segurança

## Escopo entregue

Plano de controle exclusivamente em loopback, com dois modos explícitos:

- `local_readonly` (padrão): HTTP local sem identidade autenticada, somente GET/HEAD.
  PUT de catálogo, POST de registro/streams/planos e DELETE de streams retornam 403.
- `mtls`: HTTPS direto com TLS 1.3, CA cliente confiável e fingerprint SHA-256 do
  certificado DER cadastrado na política. Sem autenticação por CN, cookie, senha,
  `X-Forwarded-*` ou certificado repassado em header.

`/api/v1/session` informa apenas `authenticated`, `role` e `mode`. A prévia visual
continua explicitamente simulada. O painel mantém seu desenho e permanece de leitura.
Os handlers públicos sempre passam pelo controle; testes unitários dos contratos
anteriores usam o roteador interno, com teste separado do Handler público.

## Permissões

| Função | Leitura | Escrita |
| --- | --- | --- |
| viewer | Dashboard, catálogo, streams, diagnóstico, HLS/player, métricas e estado de registro | Nenhuma |
| operator | Mesma leitura de viewer | Iniciar/parar streams; planos dry-run WireGuard/multicast |
| admin | Mesma leitura de viewer | Operação, PUT do catálogo e registro |
| enroller | Health/readiness, sessão e estado de registro | POST de registro |
| metrics | Health/readiness e `/metrics` | Nenhuma |

Rotas e métodos sem autorização são negados; novas rotas exigem inclusão explícita
na política de código. Manifesto assinado, canais autorizados, token de uso único e
prova de posse do provisionamento continuam obrigatórios. O registro inicial requer
certificado enroller/admin, sem exceção de bootstrap. HLS também exige certificado
cliente em mTLS; dispositivos sem esse suporte não estão homologados nesta fase.

## Configuração sem segredos no checkout

Veja o bloco comentado em `configs/edge.example.yaml` e a estrutura em
`configs/security-policy.example.json`. Os placeholders são intencionalmente inválidos.
Não coloque certificados, chaves, política real ou auditoria no repositório.
Todos os caminhos devem ser absolutos, distintos e externos ao diretório de execução;
execute a partir da raiz do checkout para que essa barreira proteja o projeto.
Arquivos devem ser regulares, sem symlinks, limitados em tamanho: chave 16 KiB,
certificado/CA/política 64 KiB cada. Ancestrais com symlinks são recusados.

No Linux, diretórios privados não podem conceder acesso a grupo/outros (0700), nem
arquivos privados (0600). O agente recusa execução como root. No Windows, validação
de DACL não está implementada: `PermissionsVerified()` retorna false; o laboratório
Windows não comprova as permissões exigidas para Ubuntu. Administradores do
diretório e da conta do serviço integram a fronteira de confiança.

A política JSON versão 1 aceita 1–64 clientes. Cada cliente tem ID aleatório opaco
de 32 caracteres hexadecimais minúsculos (sem nome de cliente), fingerprint único
de 64 caracteres hexadecimais minúsculos, função e `disabled`. Campos desconhecidos,
duplicações, roles desconhecidas e documentos extras são rejeitados.
O fingerprint é SHA-256 do DER, não do texto PEM. A função nunca vem do nome do
certificado. Certificados precisam de EKU cliente e cadeia válida; o servidor deve
ter SAN para o endereço explicitamente escolhido.

A política é relida por requisição; desabilitar/remover um fingerprint revoga também
conexões TLS abertas. A validade temporal da cadeia é reavaliada por pedido.
Política ilegível/corrompida resulta em 503, sem usar a última versão válida.
CA e chave/certificado servidor são carregados na inicialização; troca exige reinício
planejado. Não há consulta externa de CRL/OCSP ou PKI automática.

## Auditoria e limites

O diretório privado recebe `audit.jsonl` append-only e `audit.lock` exclusivo por
processo. Eventos contêm somente UTC, request ID gerado no servidor, ID opaco do
principal/função, nome padronizado da rota, método normalizado, fase e status.
Nunca contêm corpos, query strings, Authorization, certificados, CN, chave, IP,
URI bruta ou dados de canais/clientes. Falhas de TLS ocorrem antes do HTTP;
o logger padrão de handshake é suprimido para não expor dados do peer e tais
falhas não produzem eventos HTTP. Leituras autorizadas não são auditadas.

Antes de qualquer mutação, a intenção é gravada e sincronizada em disco. Depois,
registra-se o status retornado. Recusas também são auditadas em mTLS. Uma intenção
sem conclusão pode significar interrupção durante a operação; não implica rollback.
Falha ao registrar conclusão não desfaz efeito aplicado; falhas de escrita são
travadas em memória e bloqueiam pedidos administrativos seguintes.
O arquivo tem teto de 8 MiB e reserva para concluir operações em andamento.
Quando cheio ou indisponível, novas mutações falham com 503 antes do handler.
Sem rotação automática nesta fase. Crash pode deixar lock; nunca é removido
automaticamente. Auditoria local não é inviolável contra administrador do host.

Limites: 32 requisições simultâneas (429), 64 conexões TCP, timeouts HTTP e limites
de corpo existentes nos módulos. Pedidos precisam de Host loopback e origem
compatível; acessos cross-site e caminhos ambíguos são recusados. A terminação TLS
por proxy não recebe confiança implícita: o exemplo HTTP Nginx anterior não autentica
usuários nem deve publicar o plano de controle desta fase.

## Validação e operação

Testes geram CA e certificados sintéticos P-256 em diretórios temporários.
Nenhum segredo real é necessário. Execute na raiz:

```text
go test ./...
go vet ./...
go test -race ./...
cd web
npm run check
npm test
npm run build
```

Cobertura: handshakes reais, certificado ausente/expirado/CA estranha, fingerprint
desconhecido, TLS antigo, papéis, revogação em conexão aberta, política inválida,
negação local de mutações, Host/Origin, tentativa por proxy, auditoria exclusiva/
cheia/indisponível e ausência de dados sensíveis nos eventos. O teste POSIX precisa
ser executado no Ubuntu como usuário não root.

Nenhuma mudança operacional foi executada. Para futura ativação autorizada:

1. Pré-verificação: confirmar conta não root, endereço/porta loopback livres,
   SAN/validade/cadeias, fingerprints e funções revisados, paths/DACL ou modos,
   quota/disco e diretórios privados graváveis. Não inferir NIC/IP/porta.
2. Guardar cópia privada da configuração e binário anteriores; validar configuração
   em listener isolado com clientes sintéticos antes de trocar o serviço.
3. Pós-mudança: health/readiness HTTPS com certificado válido; cliente sem certificado
   falha; viewer não modifica catálogo; ação autorizada gera intenção e conclusão;
   sessão informa mTLS. Nunca usar `--insecure` para validar a CA.
4. Rollback: restaurar configuração/binário revisados e reiniciar somente com
   autorização operacional específica. `local_readonly` preserva leitura e bloqueia
   mutações; não restaurar exposição insegura para recuperar escrita.

O template systemd recebeu somente restrições declarativas, sem instalação ou
execução. Continua sem capabilities; paths graváveis de auditoria/provisionamento/HLS
devem ser revisados antes de eventual uso com `ProtectSystem=strict`.

Base técnica: [TLS Config](https://pkg.go.dev/crypto/tls#Config),
[X.509 Verify](https://pkg.go.dev/crypto/x509#Certificate.Verify) e
[HTTP ServeTLS](https://pkg.go.dev/net/http#Server.ServeTLS).

## Resultado desta entrega

Validação em 25/10/2026 no Windows, Go 1.24.9 portátil:

- `go test ./... -count=1`: todos os pacotes aprovados.
- `go vet ./...`: aprovado no Windows e na análise cruzada para Linux/amd64.
- `gofmt -l` nos pacotes alterados: sem arquivos pendentes.
- `npm.cmd run check`: aprovado; `npm.cmd test`: 11 testes aprovados.
- `npm.cmd run build`: aprovado; bundle incorporado atualizado. Há avisos de
  diretiva `use client` do Lucide, sem falha de build.
- Binários `bin/edge-agent-phase11.exe` e `bin/edge-agent-phase11-linux-amd64`
  compilados; testes de segurança Linux compilados em `bin/security-linux.test`.
- `go test -race ./...`: não executou, pois o ambiente está sem CGO.
- Execução nativa Ubuntu, permissões POSIX e homologação de dispositivos HLS
  permanecem pendentes. Compilação cruzada não substitui esses testes.

Não houve troca do agente local já em execução nem ativação operacional do mTLS.
Os testes HTTPS usaram listeners temporários isolados. Nenhum host de produção,
matriz/Flussonic, firewall, NIC ou serviço instalado foi alterado.
A Fase 12 permanece pendente de autorização.
