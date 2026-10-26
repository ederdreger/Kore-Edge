# Fase 04 — Catálogo

## Implementação

`internal/catalog` verifica assinaturas Ed25519 usando somente a biblioteca
padrão. O Edge conhece previamente a chave pública confiável e sua identidade
de parceiro via configuração local; não confia em chave enviada no manifesto.
Não existe conexão com matriz, ingest, provisionamento ou execução de streams.
Todas as fontes aceitas nesta fase são sintéticas (`synthetic://<channel-id>`).

Configuração opcional no YAML (os dois valores são obrigatórios para habilitar):

```yaml
catalog:
  partner_id: partner-lab
  public_key: <chave PUBLICA Ed25519 de 32 bytes em base64>
```

Sem esse bloco, o catálogo retorna 503 e o core continua pronto. Nunca colocar
chave privada nesse arquivo, na API, nos logs ou no Git. A chave pública precisa
ser obtida por canal de confiança; a API não permite substituí-la.

## Assinatura e schema v1

O emissor de laboratório constrói os bytes UTF-8 JSON do payload e assina
**os bytes exatos** `kore-edge/catalog/v1\n` seguidos do payload, com Ed25519
normal (não Ed25519ph). O prefixo contém uma quebra de linha LF real.
O envelope JSON contém apenas `payload` (bytes em base64) e `signature`
(assinatura de 64 bytes em base64). Não contém chave nem metadados não assinados.
Alterar qualquer byte do payload exige nova assinatura. Não reserializar o
payload entre assinatura e envio. Erros não ecoam dados recebidos.

`configs/catalog-payload.example.json` é um payload sintético **não assinado**,
com datas ilustrativas que precisam ser atualizadas antes de um ensaio.
Não serve diretamente como envelope aceito. Não há chave privada demonstrativa
no repositório. Os testes geram pares Ed25519 em memória para mockar o emissor.

Campos assinados:

- `version`: schema 1; `revision`: inteiro positivo monotônico por processo.
- `partner_id`: precisa coincidir com a identidade local configurada.
- `issued_at`/`expires_at`: RFC3339; emissão não futura, validade de até 24 horas,
  expiração estritamente posterior à emissão e ao horário de verificação.
- `channels`: até 512 canais únicos, com ID seguro de até 64 caracteres,
  nome de 1 a 128 bytes, fonte sintética correspondente ao ID e saídas `udp`/`hls`.
- `grants`: até 512 concessões únicas por canal, com as saídas autorizadas.
  Cada saída configurada de cada canal precisa estar concedida; concessões para
  canais inexistentes, saídas desconhecidas e duplicatas são rejeitadas.

Catálogo vazio com grants vazios é permitido para revogar todos os canais.
Não há RBAC de operadores nem quotas nesta etapa. Grants são permissões do
parceiro identificado no manifesto, não identidades fornecidas por requisição.

## API e estado

- `PUT /api/v1/catalog`: recebe envelope JSON de até 256 KiB; verifica assinatura,
  schema, datas, parceiro e permissões antes de publicar sob mutex; sucesso 204,
  rejeição 400. JSON com campos desconhecidos, chaves duplicadas, profundidade
  acima de 32 ou documentos adicionais é rejeitado.
- `GET /api/v1/catalog`: retorna manifesto verificado enquanto válido; sem
  catálogo, desabilitado ou expirado: 503. HEAD retorna status sem corpo.
  Outros métodos: 405. Respostas usam `Cache-Control: no-store`.
- `Store.Authorize(channel, output, now)`: nega canal/saída ausente e catálogo
  inválido/expirado. Será a fronteira de autorização para módulos posteriores;
  esta fase não implementa reprodução nem um endpoint de reprodução.
- Revisão maior substitui atomicamente o catálogo. Mesma revisão/mesmos bytes
  é idempotente; revisão menor ou mesmos números com conteúdo diferente são
  rejeitados. Leitores recebem cópias, sem acesso mutável ao estado confiável.

Exemplo de envio, somente após gerar envelope com emissor confiável e executar
o novo binário no endereço local já confirmado:

```sh
curl --fail -X PUT -H 'Content-Type: application/json' --data-binary @<envelope-assinado-local.json> http://127.0.0.1:18080/api/v1/catalog
curl --fail http://127.0.0.1:18080/api/v1/catalog
```

O placeholder de caminho deve ser substituído pelo arquivo local real.
Não usar dados de clientes no ensaio. A API continua restrita a loopback;
assinatura autentica o conteúdo, não o operador HTTP. Qualquer processo local
pode consultar ou enviar envelopes. Autenticação/RBAC de transporte é Fase 11.

## Limites, validação e rollback

O último catálogo válido é preservado quando há rejeição ou perda da matriz,
mas deixa de autorizar na expiração. Não existe persistência, histórico ou
high-water mark durável: reinício remove o catálogo e a proteção monotônica
anterior. Não há alegação de proteção contra replay entre reinícios.
O relógio do host precisa estar correto; não é ajustado pelo programa.

Rollback lógico de conteúdo: o emissor confiável deve reassinar o conteúdo
anterior com **nova revisão maior** e datas válidas. Não há bypass para downgrade.
Os testes comprovam preservação diante de falhas, atualização atômica, revogação,
cópias isoladas, idempotência e rejeição de revisões antigas. Rollback do código:
executar binário anterior com configuração compatível; retirar o bloco opcional
de catálogo se o binário for anterior à Fase 04. Nenhuma alteração de host é feita.
A instância local anterior não é substituída automaticamente.

Testes: `go test ./...`, `go vet ./...`, `go test -race ./...`,
`gofmt -l cmd internal`, build Windows e cross-build Linux amd64.
Nenhum teste exige conexão com matriz nem escreve chave privada em arquivo.

Resultado em 2026-10-25, Go 1.24.9 portátil no Windows: testes de todos os pacotes,
vet e formatação passaram. Builds Windows/Linux amd64, vet Linux e compilação
do teste de catálogo Linux passaram. `-race` foi tentado e bloqueado pelo
ambiente sem CGO/compilador C. Execução nativa no Ubuntu não foi realizada.
A pasta continua sem `.git`; não foi possível criar branch.

## Objetivo
Manifesto assinado de canais e permissões por parceiro.

## Pré-requisitos
Fase anterior aprovada, `AGENTS.md` lido, laboratório isolado.

## Entregáveis
- Implementação modular e documentada.
- Testes automatizados e exemplos de configuração sem segredos.
- Procedimento de validação e rollback, quando aplicável.

## Critério de aceite
Rejeita canais não autorizados e configurações inválidas.

## Prompt para Codex
Leia `AGENTS.md`, `README.md`, `docs/ARCHITECTURE.md` e este documento. Desenvolva somente a Fase 04 (Catálogo): Manifesto assinado de canais e permissões por parceiro. Antes de editar, apresente um plano, dependências, testes e riscos. Não execute mudanças privilegiadas no host. Entregue código, testes e documentação. Demonstre como validar: Rejeita canais não autorizados e configurações inválidas. Ao terminar, pare e solicite aprovação para a próxima fase.
