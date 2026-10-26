# Fase 01 — Core e estrutura

## Implementação entregue

- Entrada: `cmd/edge-agent`; lifecycle: `internal/app`; HTTP: `internal/api`.
- YAML estrito v1 e snapshots atômicos em memória: `internal/config`.
- Exemplo sem segredos: `configs/edge.example.yaml`.
- Modelo systemd inativo: `deploy/systemd/kore-edge.service`.
- Testes cobrem validação, retenção da configuração válida, concorrência,
  contrato HTTP e inicialização/encerramento em listener loopback efêmero.
- Comandos de build, testes, limitações e rollback local estão no README.

Aceite: executar `go vet ./...`, `go test ./...`, `go test -race ./...`,
verificar saída vazia de `gofmt -l cmd internal` e construir o binário.
O teste de lifecycle comprova respostas 200 de health/readiness em laboratório
e fechamento do listener após cancelamento. Não exige acesso a servidores.
Prontidão cobre apenas o core; nenhum módulo da Fase 02 está implementado.

### Resultado da validação — 2026-10-25

Ambiente Windows, Go portátil 1.24.9 em diretório temporário:

- `go vet ./...`: passou.
- `go test ./...`: passou nos pacotes api, app e config; entrada sem testes próprios.
- `gofmt -l cmd internal`: passou, saída vazia.
- Build Windows amd64 e cross-build Linux amd64: passaram.
- `go test -race ./...`: bloqueado pelo ambiente sem CGO/compilador C no PATH.

O teste de app iniciou a API em loopback com porta efêmera, consultou ambos os
endpoints e verificou encerramento. Não houve instalação nem execução do modelo
systemd. Execução nativa em Ubuntu e detector de corrida continuam pendentes;
cross-build não comprova comportamento operacional no Ubuntu. A pasta recebida
não tinha `.git`, portanto não foi possível criar a branch de entrega.

## Objetivo
API Go, configuração versionada, healthcheck, serviços systemd.

## Pré-requisitos
Fase anterior aprovada, `AGENTS.md` lido, laboratório isolado.

## Entregáveis
- Implementação modular e documentada.
- Testes automatizados e exemplos de configuração sem segredos.
- Procedimento de validação e rollback, quando aplicável.

## Critério de aceite
API inicia em modo laboratório; testes unitários e lint passam.

## Prompt para Codex
Leia `AGENTS.md`, `README.md`, `docs/ARCHITECTURE.md` e este documento. Desenvolva somente a Fase 01 (Core e estrutura): API Go, configuração versionada, healthcheck, serviços systemd. Antes de editar, apresente um plano, dependências, testes e riscos. Não execute mudanças privilegiadas no host. Entregue código, testes e documentação. Demonstre como validar: API inicia em modo laboratório; testes unitários e lint passam. Ao terminar, pare e solicite aprovação para a próxima fase.
