# Fase 05 — Stream Engine

## Implementação e limites

`internal/stream` ingere MPEG-TS alinhado de 188 bytes por pipe e supervisiona
um processo filho direto por canal. O próprio binário `edge-agent -synthetic-ts`
é o gerador: somente pacotes nulos PID 8191, em lotes de sete a cada 10 ms,
alvo nominal 1.052.800 bit/s. O bitrate publicado é medido, não esse alvo.
Não há PAT/PMT, áudio, vídeo, PCR ou serviço reproduzível nessa fonte.
Ela valida ingest/transporte/lifecycle, não reprodução ou qualidade de mídia.
FFmpeg/TSDuck não são instalados nem necessários; sua integração não está
implementada nesta fase. Não há transcodificação, arquivos de mídia, socket de
ingest, multicast, HLS nem conexão com a matriz.

Cada processo usa argumentos fixos, sem shell, URL ou comando fornecido pelo
cliente. stdout é consumido em memória constante (um pacote por leitor),
stderr não é registrado. A fonte é ritmada, sem fila de payloads em memória.
Limites padrão por agente: quatro canais simultâneos, 64 IDs no histórico,
três reinícios após a tentativa inicial (máximo quatro tentativas por execução),
backoff exponencial de 250, 500 e 1.000 ms. O orçamento não é restaurado após
um período saudável; todo término inesperado, inclusive exit 0, consome tentativa.
Falha de spawn, TS inválido e fluxo parado são falhas supervisionadas.

O watchdog verifica a cada 100 ms e mata a fonte sem pacote válido por dois
segundos. Finalização fecha o pipe, termina o filho e aguarda/recolhe o processo
(`Wait`); o processo fixo não gera descendentes. Não há cgroups ou quotas de CPU
por processo; limites de execução do host continuam sujeitos à etapa operacional.

## Habilitação e autorização

No YAML, além do bloco de confiança do catálogo da Fase 04:

```yaml
stream:
  enabled: true
```

É desabilitado por padrão. Habilitação sem confiança configurada é rejeitada.
Não há auto-start: envie primeiro o manifesto assinado válido e solicite o canal.
Somente canal autorizado no catálogo vigente para pelo menos uma saída UDP/HLS
pode iniciar. Isso autoriza ingest sintético; não cria nenhuma dessas saídas.
O catálogo atual já exige fontes `synthetic://<id>`. Todos os canais desta fase
usam o mesmo gerador nulo, com um único processo por ID.

A autorização é conferida no início, antes de cada tentativa e a cada tick do
watchdog durante execução. Expiração/revogação produz `unauthorized`, sem
reinício automático. Em backoff, a verificação acontece na tentativa seguinte
(até um segundo no padrão). Não depende de conectividade com matriz; manifesto
local continua válido até expirar. Não há reprodução a interromper nesta fase.

## API local

- `POST /api/v1/streams` com `{"channel_id":"test-pattern"}`: 202 aceito,
  403 sem autorização, 409 por limite, 503 se desabilitado/encerrado e 400
  para JSON inválido. Corpo até 1 KiB, sem campos extras/duplicados. Nunca recebe
  comandos, paths ou endereços de origem/saída do cliente.
- `GET /api/v1/streams`: lista ordenada por ID de snapshots de estado/métricas;
  HEAD retorna somente cabeçalhos. Desabilitado: 503.
- `DELETE /api/v1/streams/<id>`: 202 solicita parada assíncrona; consulte GET
  até `stopped`. ID desconhecido: 404. Operações repetidas sobre o mesmo canal
  ativo/parado são idempotentes. Um POST explícito após término pode iniciar nova
  execução, zerando os contadores e o orçamento daquele ID.
- Estados: `starting`, `running`, `backoff`, `failed`, `stopped`, `unauthorized`.
  202 não garante fluxo; `running` exige recebimento de pacote válido.

Usar somente o endereço local previamente aprovado e o novo binário, após
configurar confiança e carregar catálogo conforme a Fase 04:

```sh
curl --fail -H 'Content-Type: application/json' --data '{"channel_id":"test-pattern"}' http://127.0.0.1:18080/api/v1/streams
curl --fail http://127.0.0.1:18080/api/v1/streams
curl --fail -X DELETE http://127.0.0.1:18080/api/v1/streams/test-pattern
```

Não há autenticação HTTP de operadores até Fase 11; manter loopback. A assinatura
do catálogo limita quais canais podem ser iniciados, mas não autentica o cliente
HTTP que solicita start/stop. O histórico tem limite rígido, sem persistência;
ao atingir 64 IDs diferentes, novos IDs são recusados até reinício do agente.

## Métricas e critérios de aceite

Cada snapshot contém timestamp UTC, início, último pacote, tentativas/reinícios,
bytes lidos (inclusive trecho inválido), pacotes válidos, erros de framing/sync
e pacotes com TEI. Framing inválido/truncado falha a tentativa; não há resync
silencioso. O parser valida sync byte, adaptation control/length, sem alegar
validação completa de MPEG-TS. Não calcula CC/PCR/jitter nem detecta perda de rede.

Bitrate: `8 * delta_bytes / segundos_da_janela`, atualizado no tick do watchdog,
com `window_seconds` explícito. Estado `measured_synthetic` identifica medição
real de bytes de origem sintética; não é bitrate de canal comercial. Antes da
primeira janela e em estados terminais/backoff: `unavailable`, valor nulo.
Contadores acumulam entre tentativas de uma execução; `media_quality` fica
indisponível porque não existem componentes de mídia na fonte nula.

Testes verificam contadores conhecidos, framing truncado/corrompido e TEI,
gerador ritmado, reinício finito para falha de spawn/exit/corrupção, backoff
cancelável, watchdog, autorização revogada, limites de concorrência/histórico,
start/stop idempotentes, contrato HTTP e processo filho real recolhido no fim.
O teste do binário construído é habilitado explicitamente (PowerShell):

```powershell
go build -o bin/edge-agent-phase05.exe ./cmd/edge-agent
$env:KORE_EDGE_AGENT_TEST_BINARY = (Resolve-Path bin/edge-agent-phase05.exe).Path
go test -v ./internal/stream
```

Sem essa variável, apenas o teste opcional do binário construído é pulado;
o teste de subprocesso usando o binário dos testes continua sendo executado.

## Validação local e rollback

Nenhuma configuração de host é alterada. Pré-verificar versão/configuração
local e catálogo autorizado; consultar GET antes de iniciar. Depois, confirmar
pacotes/bytes crescentes, estado e janela de bitrate, e DELETE seguido de estado
terminal. SIGINT/SIGTERM do agente cancela os canais e recolhe seus filhos.
Rollback local: parar os canais, encerrar o agente, restaurar o binário/configuração
anteriores (remover bloco `stream` para versões anteriores à Fase 05) e validar
`/healthz` e `/readyz`. O catálogo em memória precisará ser reenviado.
Não há alteração de rede, firewall, systemd ou serviço externo a reverter.

## Resultado da validação — 2026-10-25

Go 1.24.9 portátil no Windows: testes de todos os pacotes, `go vet ./...` e
formatação passaram. O teste opcional `TestBuiltAgentSource` também passou,
executando `bin/edge-agent-phase05.exe -synthetic-ts` como filho real e verificando
fluxo, bitrate positivo e recolhimento do processo ao encerrar o engine.
Build Windows, cross-build Linux amd64, vet Linux e compilação do teste Linux
passaram. Execução nativa no Ubuntu não foi realizada. `go test -race ./...`
foi tentado e bloqueado por ausência de CGO/compilador C no ambiente.

Nenhum FFmpeg/TSDuck foi executado, nenhum vídeo reproduzido e nenhum pacote
UDP enviado. A instância antiga da API não foi reiniciada; o novo binário está
separado em `bin/edge-agent-phase05.exe`. A pasta continua sem `.git`, portanto
não foi possível criar a branch da fase.

## Objetivo
Ingest MPEG-TS de fonte sintética, supervisão de processos.

## Pré-requisitos
Fase anterior aprovada, `AGENTS.md` lido, laboratório isolado.

## Entregáveis
- Implementação modular e documentada.
- Testes automatizados e exemplos de configuração sem segredos.
- Procedimento de validação e rollback, quando aplicável.

## Critério de aceite
Reinício limitado; métricas de fluxo verificadas.

## Prompt para Codex
Leia `AGENTS.md`, `README.md`, `docs/ARCHITECTURE.md` e este documento. Desenvolva somente a Fase 05 (Stream Engine): Ingest MPEG-TS de fonte sintética, supervisão de processos. Antes de editar, apresente um plano, dependências, testes e riscos. Não execute mudanças privilegiadas no host. Entregue código, testes e documentação. Demonstre como validar: Reinício limitado; métricas de fluxo verificadas. Ao terminar, pare e solicite aprovação para a próxima fase.
