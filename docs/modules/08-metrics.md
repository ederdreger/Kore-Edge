# Fase 08 — Monitoramento

## Implementação e acesso

`internal/tsmetrics` analisa cabeçalhos TS no ingest existente, sem alterar os
pacotes. `internal/monitoring` amostra uma vez por segundo os snapshots do engine
e contadores de NICs, mantém alertas locais e exporta JSON/Prometheus. Sem novas
dependências, comandos externos, captura de rede, gravação de payloads ou
alteração de host. Não instala Prometheus/Alertmanager e não envia notificações.

```sh
curl --fail http://127.0.0.1:18080/api/v1/monitoring
curl --fail http://127.0.0.1:18080/metrics
```

Usar o novo binário e o endereço local já confirmado. Ambos aceitam GET/HEAD,
outros métodos retornam 405; monitor ausente retorna 503. Respostas `no-store`.
O sampler funciona mesmo com engine desabilitado, indicado por `streams_state`.
Métricas são administrativas e continuam em loopback; acesso autenticado de
operadores/rede externa permanece na Fase 11.

## Contrato de medição

Cada item de `measurements` contém `name`, canal/interface quando aplicável,
`state`, `source`, `origin`, unidade, timestamp UTC, início/fim da janela e
`value`. Indisponível significa `state: unavailable`, `value: null` e motivo;
zero só é publicado como valor quando efetivamente observado/calculado.
`origin: synthetic` identifica medição real de fonte sintética, não números
simulados. `origin: host` identifica contadores do kernel. Não existe modo
simulado em runtime; fixtures de teste nunca são publicadas pela aplicação.

- `stream_ingest_available`: gauge 0/1 do ingest neste instante: running e
  pacote válido nos últimos dois segundos. Não comprova entrega UDP, reprodução
  HLS, disponibilidade da matriz ou quantidade de espectadores.
- `stream_ingest_bitrate_bits_per_second`: delta de bytes da janela real do
  engine (normalmente ~100 ms), com início/fim explícitos; amostragem do monitor
  em 1 s não muda essa janela. Após dois segundos sem janela fresca fica
  indisponível. Não se usa bitrate nominal do gerador como se fosse medido.
- Bytes, sync/framing e TEI acumulam na execução atual do canal; nova execução
  explícita zera esses contadores. TEI é bit de erro recebido, não uma prova de
  causa/perda de pacotes na rede.
- CC, duplicatas, descontinuidades e PCR são por tentativa atual/última, com
  janela desde a criação daquele analisador; reinício interno os zera.
- `stream_udp_submitted_bytes_total`, `stream_udp_write_errors_total` e
  `stream_udp_socket_available` aparecem somente para saída instrumentada.
  São contadores da tentativa da saída e estado do socket; não entrega ao receptor.

O sampler mantém somente o snapshot atual. Se parar de atualizar por mais de
três segundos, o JSON passa a `state: stale` e valores antes medidos tornam-se
indisponíveis. Não há armazenamento de séries de 30 minutos ou TSDB local;
histórico para gráficos deverá vir de um coletor externo ou módulo posterior.

## Continuidade e PCR

Analisador limitado aos 8.192 PIDs possíveis, com estado por PID observado,
sem guardar payloads além do último pacote de cada PID para comparar duplicatas.
Não registra pacotes em arquivos/logs. Ignora PID nulo 8191 para CC/PCR; fonte
exclusivamente nula tem CC/PCR indisponíveis, em vez de falsa qualidade perfeita.

CC acompanha wrap de 15 para 0, incremento em payload e manutenção em pacotes
somente de adaptação. Uma duplicata consecutiva byte a byte é contabilizada,
sem erro; repetições adicionais ou mesmo CC com conteúdo diferente são erros.
Essa política é conservadora; não implementa exceções de duplicatas com PCR
reescrito. Discontinuity indicator reinicia a baseline. TEI evita erro CC em
cascata, invalidando a baseline daquele PID. Framing inválido continua sujeito
à falha da tentativa no engine, não a resync silencioso.

PCR extrai base/extensão em ticks de 27 MHz, valida bits reservados/extensão,
calcula intervalo somente entre PCRs do **mesmo PID**, trata wrap de 33 bits da
base e detecta recuo de relógio. Snapshot de stream expõe contagens, PID, wraps,
recuos, PCR inválido e descontinuidades. Um único PCR não produz intervalo.
O valor consolidado `stream_pcr_interval_seconds` é o último intervalo codificado
do mesmo PID, com janela entre timestamps de observação desse par; não é média
de todos os PIDs e fica indisponível após dois segundos sem PCR fresco.

**Não mede precisão/jitter de PCR contra relógio calibrado.**
`stream_pcr_accuracy_seconds` é explicitamente indisponível. Scheduling, buffers
e pipes impedem usar os horários de chegada como medição de conformidade de
PCR. O analisador não afirma certificação ETSI/TR 101 290, nem decodifica PAT/PMT
para identificar qual PID é o PCR de cada programa.

## RX/TX

`diagnostics.Network()` lê somente `/proc/net/dev` no Linux. Para cada interface
detectada, calcula `8 * delta_bytes / delta_segundos` em duas leituras válidas,
publicando início/fim reais. Primeira leitura, reset/decréscimo de contador,
intervalo não positivo, gap superior a cinco segundos ou fonte indisponível
produzem valor nulo e nova baseline. Interface removida sai do próximo snapshot.

São interfaces do namespace do agente, físicas/virtuais conforme detectadas,
sem assumir nomes como eth0/wg0 nem classificar gestão/IPTV/VPN por heurística.
Não diferencia recriação de NIC com mesmo nome e contador maior. Não estima
capacidade de link, tráfego de clientes ou soma contadores sobrepostos de NIC/VPN.
No Windows esses contadores Linux permanecem indisponíveis, sem fallback falso.

## Alertas locais

JSON contém `alerts` ativos/pending e `recent_alerts` resolvidos (até 128).
Cada alerta tem código, canal, severidade, estado, início e atualização.
Não dispara ação corretiva, envio de mensagem, restart ou alteração operacional.

| Código | Condição | Severidade / espera |
| --- | --- | --- |
| `channel_failed` | engine em failed | crítica, imediata |
| `stream_stalled` | running sem pacote fresco há mais de 2 s | crítica, hold de 5 s |
| `continuity_errors` | novos erros CC desde amostra anterior | aviso, imediato |
| `transport_errors` | novos TEI desde amostra anterior | aviso, imediato |
| `sync_errors` | novos erros de framing/sync | crítica, imediata |
| `udp_write_errors` | novos erros de escrita UDP | aviso, imediato |

Hold positivo passa por `pending` e depois `firing`; ao cessar a condição, alertas
ativos vão a `resolved`. Pending que recupera antes do hold desaparece sem
resolução fictícia. Gap de amostragem acima de 3 s/recuo de relógio reinicia hold
pendente. Contadores são avaliados por delta, com baseline resetada em nova
execução/tentativa; erro histórico não gera aviso eterno. Primeiro snapshot com
erros já observados gera alerta. `stopped`/revogação não disparam channel_failed.
O watchdog do engine pode mudar o estado antes de um stream_stalled completar
hold; nesse caso alerta de falha final é avaliado quando/ se houver failed.

Limites: até 64 canais históricos e 128 NICs amostradas, cinco alertas TS/engine
mais um UDP por canal e 128 resoluções em memória. Estado/histórico somem no
reinício. Alertas não têm persistência, acknowledgement, e-mail ou integração
com Alertmanager nesta fase.

## Prometheus e limites de exposição

`/metrics` usa `text/plain; version=0.0.4; charset=utf-8`, HELP/TYPE agrupados,
labels escapadas e LF final. Nomes recebem prefixo `kore_edge_`; contadores têm
sufixo `_total`, gauges representam taxas/estados. Cada medida também tem
`_available`; indisponíveis têm disponibilidade 0 e **não** têm amostra numérica
inventada. Medidas válidas incluem `_timestamp_seconds` e `_window_seconds`.
Labels de medida: `channel_id`, `interface`, `source`, `origin`; não publica
IPs de clientes, URLs com credenciais, chaves ou payloads.

`kore_edge_monitoring_sample_available` indica se o sampler está fresco.
`kore_edge_alert_active` identifica pending (0) ou firing (1); resoluções ficam
no JSON e séries antigas deixam a exposição. Não há claim de OpenMetrics 1.0
ou servidor Prometheus instalado. Contadores podem resetar em retry/restart;
um scrape menos frequente pode não observar todos os resets/alertas transitórios.

Referência do formato: [documentação oficial Prometheus](https://prometheus.io/docs/instrumenting/exposition_formats/).

## Validação e rollback local

Testes com TS sintético e tempos controlados verificam CC independente por PID,
wrap, duplicata única, adaptação, TEI, descontinuidade, par PCR/wrap/recuo inválido,
indisponibilidade de fonte nula, snapshots isolados, RX/TX/reset/gap, alertas
pending/firing/resolved, histórico limitado e API/exportador. O ensaio AV real
da Fase 07 foi estendido para exigir PCRs, intervalo e CC medidos sem CC/TEI
na fonte sintética ideal. Nenhum teste de monitoramento envia multicast.

```sh
go vet ./...
go test ./...
go test -race ./...
gofmt -l cmd internal
go build -o bin/edge-agent ./cmd/edge-agent
```

Para o ensaio AV real, usar as variáveis FFmpeg/FFprobe/FFplay e o comando de
integração documentados na Fase 07. Sem ferramentas/variáveis o ensaio opcional
é pulado; testes determinísticos continuam disponíveis.

Não há mudança operacional nesta fase. Rollback local: encerrar o agente e
executar o binário anterior com a configuração existente; não há migração de
estado, arquivo de métricas ou serviço externo a restaurar. Não foi substituída
a instância antiga da API. Instalação futura de collector externo exige plano
e aprovação operacional específicos.

## Resultado — 2026-10-25

Go 1.24.9 portátil no Windows: testes de todos os pacotes, `go vet ./...` e
formatação passaram. Builds Windows/Linux amd64, vet Linux e compilação dos
testes de monitoramento/analisador Linux passaram. O ensaio AV real com FFmpeg
portátil também passou: PCR/intervalo presentes e CC/TEI iguais a zero no
caso nominal, além da reprodução/expiração de segmentos já verificadas na Fase 07.

`go test -race ./...` foi tentado e bloqueado pelo ambiente sem CGO/compilador C.
Coleta nativa RX/TX no Ubuntu e scrape com servidor Prometheus real não foram
executados; RX/TX foi validado com fixtures e o formato por testes de exposição.
Não houve tráfego multicast real ou alterações operacionais. Novo binário:
`bin/edge-agent-phase08.exe`; instância local antiga preservada. A pasta continua
sem `.git`, portanto não foi possível criar a branch da fase.

## Objetivo
Bitrate, PCR/CC/TEI quando disponíveis, RX/TX e disponibilidade.

## Pré-requisitos
Fase anterior aprovada, `AGENTS.md` lido, laboratório isolado.

## Entregáveis
- Implementação modular e documentada.
- Testes automatizados e exemplos de configuração sem segredos.
- Procedimento de validação e rollback, quando aplicável.

## Critério de aceite
Métricas identificam origem e janela; alertas testados.

## Prompt para Codex
Leia `AGENTS.md`, `README.md`, `docs/ARCHITECTURE.md` e este documento. Desenvolva somente a Fase 08 (Monitoramento): Bitrate, PCR/CC/TEI quando disponíveis, RX/TX e disponibilidade. Antes de editar, apresente um plano, dependências, testes e riscos. Não execute mudanças privilegiadas no host. Entregue código, testes e documentação. Demonstre como validar: Métricas identificam origem e janela; alertas testados. Ao terminar, pare e solicite aprovação para a próxima fase.
