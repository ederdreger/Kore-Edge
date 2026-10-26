# Fase 10 — Provisionamento

## Implementação e fronteira de confiança

`internal/provision` implementa uma autoridade **local de laboratório**, mockando
o fluxo de registro da matriz. Não há conexão com headend, Flussonic, VPN, DNS
ou outro servidor. Esta autoridade não é apresentada como serviço de produção
multi-tenant. Emissão/revogação são operações de uma ferramenta offline, acessível
somente a quem já controla o diretório privado. Não existe emissão por HTTP.

`cmd/edge-provision` emite tokens criptograficamente aleatórios de 256 bits,
prefixo `ke1_`, validade de 1 a 900 segundos, vinculados a ID do Edge, parceiro
e chave pública confiável Ed25519 do catálogo. O registro armazena SHA-256 do
token, nunca o bearer em claro. Comparação de digest usa tempo constante; erros
não ecoam bearer, prova ou entrada do usuário. Formatação normal/debug de
`Credential` é redigida. Nenhum token é emitido como argumento ou stdout.

O único destino do bearer na ferramenta é um arquivo novo, explicitamente
indicado dentro do diretório privado de runtime. A credencial deve ser entregue
por canal seguro fora deste módulo. Não copiar para Git, logs, chat, URLs,
shell history ou variáveis de linha de comando. Chaves privadas só existem em
memória nos testes; armazenamento seguro/rotação de chaves do dispositivo é
trabalho da Fase 11. A chave de identidade P-256 não é chave WireGuard nem
chave Ed25519 do catálogo.

## Configuração opcional do agente

```yaml
provision:
  enabled: true
  edge_id: edge-lab
  directory: /absolute/private/laboratory/provision-runtime
```

`directory` deve ser absoluto, dedicado, fora do checkout/diretório de trabalho,
sem symlink direto e não ser raiz de volume. O módulo rejeita paths dentro do
CWD para evitar material de runtime no repositório; o operador também deve
mantê-los fora de outros repositórios, diretórios compartilhados ou backups
inseguros. Diretório novo é criado com modo solicitado 0700 e arquivos 0600.
No Unix, diretórios/arquivos com permissões de grupo/outros são rejeitados.

**Windows:** modos POSIX não comprovam DACL privada. O módulo não modifica ACLs,
reporta `filesystem_permissions_verified: false` e exige revisão manual das
permissões para qualquer uso além dos testes locais. Não foi executada mudança
de permissão de host nesta entrega. No Linux, a verificação cobre bits de modo,
não auditoria de administração/root, backup, criptografia ou hardware.

Sem bloco/enabled false, provisionamento fica desabilitado e endpoints retornam
503. O health do core não depende do estado de registro. Falha de storage em
runtime não modifica catálogo, streams ou saída de mídia em andamento.

## Ferramenta offline

Build:

```sh
go build -o bin/edge-provision ./cmd/edge-provision
```

`configs/provision-assignment.example.json` é exemplo sintético com placeholder
de **chave pública**. Não contém token e precisa ser preenchido localmente com
dados de laboratório e chave obtida por canal confiável. Campos: `edge_id`,
`partner_id`, `catalog_public_key` (Ed25519/base64) e `ttl_seconds` (máx. 900).

Fluxo proposto, somente com diretório/identificadores já revisados:

```sh
edge-provision -action issue -directory <DIRETORIO_PRIVADO> -token-out <DIRETORIO_PRIVADO>/delivery.json < <ATRIBUICAO_LOCAL_REVISADA.json>
edge-provision -action revoke -directory <DIRETORIO_PRIVADO> -token-id <ID_PUBLICO_DO_TOKEN>
edge-provision -action status -directory <DIRETORIO_PRIVADO> -edge-id <EDGE_ID_REVISADO>
```

No PowerShell, fornecer JSON por stdin com o mecanismo adequado ao shell;
os operadores `<...>` acima são placeholders, não valores reais. Não usar token
em comandos/curl/argv. A emissão imprime apenas ID público e expiração; o arquivo
de entrega contém bearer, ID e expiração. Ele é criado com exclusividade, sem
sobrescrever credencial ou arquivos do registro. Se entrega falhar após emissão,
a ferramenta tenta revogar a credencial e remove somente seu arquivo novo;
falha de storage nessa compensação exige revisão do registro pelo operador.

Revogação por ID público é idempotente. ID desconhecido é rejeitado. O limite
é de 128 tokens e 128 identidades por registro; registros usados/revogados não
são removidos automaticamente para não perder a prova de consumo. Capacidade
esgotada falha fechada e exige planejamento de rotação/arquivamento posterior.

## Prova e API

`GET/HEAD /api/v1/provisioning`: `pending` ou `registered`, identidade pública,
proposta e indicação de permissões verificadas; nunca lista tokens/digests.
`POST /api/v1/provisioning/register`: bearer no header Authorization, corpo
JSON de até 4 KiB, com nomes de campos exatos e sem campos extras/duplicados:

```json
{
  "version": 1,
  "edge_id": "edge-lab",
  "public_key": "<P256_SUBJECT_PUBLIC_KEY_INFO_DER_BASE64>",
  "challenge": "<NONCE_32_BYTES_BASE64URL_SEM_PADDING>",
  "signature": "<ASSINATURA_ECDSA_ASN1_BASE64>"
}
```

O cliente mantém sua chave privada P-256 fora da API. A chave pública é SPKI
DER canônico em base64; outras curvas/algoritmos são recusados. O nonce é gerado
pelo cliente com crypto/rand; não é um desafio autenticado emitido pelo servidor.
Ele integra a prova vinculada ao token de uso único.

Para assinar, usar bytes UTF-8 de `kore-edge/enroll/v1` seguidos de **LF real**,
depois `json.Marshal(provision.Proof{Version, EdgeID, PublicKey, Challenge})`,
na ordem de campos definida em Go. Calcular SHA-256 e assinar com ECDSA P-256
ASN.1 (`ecdsa.SignASN1`). A assinatura não faz parte dos bytes assinados.
O servidor reconstrói esses bytes tipados e verifica com `VerifyASN1`.
Não é assinatura do JSON bruto nem uma alegação de canonicalização JSON genérica.
Os testes da biblioteca/API demonstram geração completa de prova em memória.

Autorização exige: bearer conhecido, não usado/revogado, dentro da validade,
Edge ID igual ao configurado e ao atribuído no token, prova válida. O parceiro
e a chave do catálogo vêm da atribuição confiável, nunca do cliente. A validade
é conferida no ponto serializado de consumo, incluindo tempo de espera pelo lock.
Expiração é exclusiva: exatamente no `expires_at` o token já é rejeitado.

Sucesso: 201, identidade e proposta `configuration_applied: false`. Token inválido,
expirado, revogado, usado ou prova/binding incorretos: 403 genérico; JSON inválido:
400; storage ocupado/inválido: 503; métodos não permitidos: 405. Query strings
nesse endpoint são recusadas para não incentivar bearer em URL. Respostas usam
`no-store` e `no-referrer`; não há logs de requisição contendo credenciais.

## Persistência, concorrência e recuperação

Token consumido e identidade são gravados juntos em `registry.json`, sob lock
de arquivo exclusivo (`O_EXCL`) compartilhado entre instâncias/processos, com
espera máxima de um segundo e retries de 10 ms. O estado é relido a cada
transação para não perder revogações/consumos de outro processo. Tamanho máximo
256 KiB, schema estrito, IDs/digests únicos e tipos validados; corrupção não
é tratada como registro vazio. Nunca substitui silenciosamente identidade já
registrada, mesmo com outro token válido.

Commit: arquivo temporário 0600 na mesma pasta, escrita + fsync, rename e sync
de diretório no Unix. O alvo operacional é filesystem local Linux com suporte
a essas garantias; não usar como lock distribuído em NFS/compartilhamento.
**Windows:** os.Rename não promete atomicidade de crash como em Unix e não há
directory fsync neste adaptador. Os testes funcionais Windows não substituem
homologação de durabilidade no Ubuntu. O arquivo é protegido por permissões,
não é assinado/cifrado ou resistente a rollback feito por administrador.

Repetir um POST já consumido é rejeitado, inclusive com a mesma prova. O efeito
durável ocorre uma única vez; não existe replay que crie outra identidade.
Se o cliente perder a resposta, consultar GET status, sem reutilizar o bearer.
Em falha após rename/directory sync, o resultado pode ser indeterminado para
o chamador: consultar estado antes de decidir emitir outra credencial.

Crash pode deixar `registry.lock`; o módulo não apaga lock de dono desconhecido.
Nesse caso falha fechada. Recuperação requer verificar que todos os escritores
pararam e que o arquivo pertence a essa instância antes de ação manual aprovada.
Não restaurar backup anterior ao consumo como rollback comum: isso pode reativar
tokens antigos. Preservar o registro atual e revogar credenciais pendentes antes
de qualquer restauração excepcional. Hora UTC confiável é requisito; o módulo
não ajusta relógio e não oferece proteção contra rollback administrativo de clock.

## Identidade e proposta

Identidade contém Edge ID, parceiro atribuído, chave pública P-256 e horário de
registro. `/api/v1/edge` inclui `enrollment_state` e `identity`; o painel apenas
lê o Edge ID, sem formulário de token nem mutações novas. Perfil do operador
continua sem autenticação de Fase 11.

A proposta contém somente schema 1, modo laboratory e confiança do catálogo
(parceiro/chave pública). Não define NIC/IP/porta, não altera YAML, não concede
novos canais, não troca confiança em runtime e não inicia stream. Aplicação
exige revisão manual e procedimento operacional específico. `configuration_applied`
informa que o registro não aplicou a proposta; não audita alterações manuais
feitas depois. Revogar token bootstrap já usado não desprovisiona a identidade
nem revoga credenciais de dispositivo futuras; esse ciclo de vida é Fase 11/13.

## Testes, pré-verificação e rollback

Testes usam parceiros sintéticos e chaves geradas em memória, diretórios de
teste temporários fora do checkout, sem credencial em fixture versionada.
Verificam expiração no limite, not-before, revogação persistente/idempotente,
reutilização, binding/prova inválidos sem consumo, reabertura, falha de commit,
storage corrompido/lock ocupado, concorrência entre instâncias e quatro processos
reais (exatamente um consumo), ausência de bearer em stdout/estado/respostas e
restrições de caminhos. O teste de modo privado Unix é compilado para Linux.

```sh
go test ./...
go vet ./...
go test -race ./...
gofmt -l cmd internal
go build -o bin/edge-agent-phase10 ./cmd/edge-agent
go build -o bin/edge-provision-phase10 ./cmd/edge-provision
```

Pré-verificação para uso futuro: confirmar laboratório e Edge ID, proprietário/
permissões/ACLs do diretório, filesystem local, relógio UTC, política atribuída,
chave pública de confiança e ausência de registro anterior inesperado. Operações
reais de emissão/registro não foram executadas fora de testes nesta entrega.
Nenhuma mudança de Ubuntu, rede, firewall, Nginx/systemd ou processo local existente.

Rollback de código: preservar diretório de registro, encerrar somente o binário
novo e executar anterior com config compatível (remover bloco `provision` se
anterior à Fase 10). Não apagar/recuar o estado de tokens/identidade. Não há
migração de rede ou catálogo a desfazer. Limpeza/ACLs/recuperação operacional
exigem pré-verificação e aprovação conforme AGENTS.

Referências primárias: [Go ECDSA](https://pkg.go.dev/crypto/ecdsa),
[chaves públicas PKIX](https://pkg.go.dev/crypto/x509#ParsePKIXPublicKey) e
[garantias de os.Rename](https://pkg.go.dev/os#Rename).

## Resultado — 2026-10-25

Go 1.24.9 portátil no Windows: testes de todos os pacotes, vet e formatação
passaram. O teste entre quatro processos reais confirmou exatamente um consumo
do bearer. Builds do agente e ferramenta offline para Windows/Linux amd64,
vet Linux e compilação dos testes Unix de provisionamento passaram.
Frontend: TypeScript e dez testes passaram; a integração de identidade foi
reconstruída no bundle, sem alterar a composição visual ou permitir mutações.

`go test -race ./...` foi tentado e bloqueado pela ausência de CGO/compilador C.
Permissões Unix e durabilidade de crash no Ubuntu continuam pendentes de
homologação nativa; ACLs Windows não foram certificadas. Não houve emissão ou
registro real fora de fixtures de laboratório. Nenhum token/chave privada foi
impresso pelos testes/ferramentas executados. Binários novos estão em
`bin/edge-agent-phase10.exe` e `bin/edge-provision-phase10.exe`; os processos
existentes foram preservados. Pasta sem `.git`: branch não criada.

## Objetivo
Registro por token de uso único, identidade e configuração.

## Pré-requisitos
Fase anterior aprovada, `AGENTS.md` lido, laboratório isolado.

## Entregáveis
- Implementação modular e documentada.
- Testes automatizados e exemplos de configuração sem segredos.
- Procedimento de validação e rollback, quando aplicável.

## Critério de aceite
Expiração e revogação de token validadas.

## Prompt para Codex
Leia `AGENTS.md`, `README.md`, `docs/ARCHITECTURE.md` e este documento. Desenvolva somente a Fase 10 (Provisionamento): Registro por token de uso único, identidade e configuração. Antes de editar, apresente um plano, dependências, testes e riscos. Não execute mudanças privilegiadas no host. Entregue código, testes e documentação. Demonstre como validar: Expiração e revogação de token validadas. Ao terminar, pare e solicite aprovação para a próxima fase.
