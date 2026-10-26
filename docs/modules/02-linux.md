# Fase 02 — Diagnóstico Linux

## Implementação e contrato

`GET /api/v1/diagnostics` retorna JSON com `timestamp` UTC e grupos `cpu`,
`memory`, `disk`, `interfaces`, `routes`, `sensors`. Cada grupo possui timestamp
de início da coleta, `source`, `state`, `value` e, se indisponível, `reason`.
Uma falha parcial não interrompe os demais grupos nem altera `/readyz`.
HTTP 200 indica entrega do diagnóstico; consulte o estado de cada grupo.
HEAD retorna apenas cabeçalhos e POST retorna 405.

- CPU: ticks agregados de `/proc/stat`, sem duplicar guest; idle inclui iowait.
  Não é percentual instantâneo. Para uma janela, use
  `100 * (1 - delta_idle / delta_total)`, rejeitando resets e delta total zero.
- RAM: total, disponível e usado (`total - disponível`) em bytes, via meminfo.
- Disco: espaço do filesystem `/` via statfs; usado é total menos livre,
  disponível ao usuário pode ser menor que livre por blocos reservados.
- NICs: nomes detectados e contadores cumulativos RX/TX em bytes de net/dev.
  Não seleciona interface, não configura IP e não converte contadores em Mbps.
  Inventário de endereços/estado de link ainda não é exposto.
- Rotas IPv4/IPv6: destino CIDR, gateway, interface e flags via procfs, apenas
  a visão oferecida por esses arquivos no namespace atual; não equivale a um
  inventário completo de policy routing. Falha de uma família deixa o grupo
  indisponível para não apresentar tabela parcial como completa.
- Sensores: zonas térmicas legíveis, em Celsius; hardware sem zonas válidas
  resulta em indisponível. Não representa todos os sensores hwmon.

No Windows e outros sistemas não Linux, todos os grupos retornam `unavailable`
com `value: null`. Nunca há fallback para números fictícios. Fixtures de teste
são sintéticas e não são servidas pelo endpoint em execução.

## Segurança, limites e validação

Não são executados comandos externos. Os arquivos são abertos somente para
leitura, com limite de 1 MiB por arquivo; sensores publicados limitados a 128.
Não há coleta periódica, gravação de diagnóstico em disco ou logs de IPs.
A coleta é síncrona por requisição, sem promessa de snapshot simultâneo entre
fontes. Leituras locais do kernel/statfs não possuem cancelamento individual;
não usar este endpoint para polling intensivo ou exposição pública. A API
continua restrita a loopback. Nenhuma nova dependência foi adicionada.

```sh
go vet ./...
go test ./...
go test -race ./...
gofmt -l cmd internal
go build -o bin/edge-agent ./cmd/edge-agent
# Após revisão/confirmação do endereço do exemplo:
go run ./cmd/edge-agent -config configs/edge.example.yaml
curl --fail http://127.0.0.1:18080/api/v1/diagnostics
```

Testes com fs sintético e função statfs injetada verificam unidades, timestamps,
contadores, rotas, sensores, arquivos inválidos/ausentes/grandes e falha parcial.
O teste Linux nativo aceita fontes indisponíveis, sem exigir sensores físicos.
Nenhuma mudança operacional é necessária. Rollback do código: executar o binário
anterior com a configuração v1 existente; nenhum estado do host é modificado.
O processo local iniciado anteriormente continua executando a versão anterior
até que seja substituído explicitamente.

## Referência visual da futura dashboard

A imagem fornecida pelo usuário em 2026-10-25 é a referência para a Fase 09:
tema escuro roxo, sidebar à esquerda, busca e perfil no topo, faixa de boas-vindas,
cards de indicadores, tabela central de canais, coluna direita de status e
interfaces, gráficos inferiores. A reprodução visual deverá seguir a imagem
fielmente. Números, nomes de canais e estados ilustrados não são dados reais
do Edge e não devem ser tratados como medições. A Fase 02 não implementa a UI.

## Objetivo

Validação em 2026-10-25 com Go 1.24.9 portátil no Windows: `go vet ./...`,
`go test ./...` e gofmt passaram. Vet e build para Linux amd64 passaram;
o binário dos testes de diagnóstico Linux também foi compilado. O build Windows
foi gerado separadamente, sem substituir o executável local em execução.
`go test -race ./...` foi bloqueado por ausência de CGO/compilador C.
Coleta nativa no Ubuntu ainda não executada; cross-build não substitui esse
aceite em laboratório. A pasta continua sem `.git`, impossibilitando branch.
CPU, RAM, disco, NICs, rotas e sensores em leitura.

## Pré-requisitos
Fase anterior aprovada, `AGENTS.md` lido, laboratório isolado.

## Entregáveis
- Implementação modular e documentada.
- Testes automatizados e exemplos de configuração sem segredos.
- Procedimento de validação e rollback, quando aplicável.

## Critério de aceite
Nenhum comando altera o host; métricas têm timestamp.

## Prompt para Codex
Leia `AGENTS.md`, `README.md`, `docs/ARCHITECTURE.md` e este documento. Desenvolva somente a Fase 02 (Diagnóstico Linux): CPU, RAM, disco, NICs, rotas e sensores em leitura. Antes de editar, apresente um plano, dependências, testes e riscos. Não execute mudanças privilegiadas no host. Entregue código, testes e documentação. Demonstre como validar: Nenhum comando altera o host; métricas têm timestamp. Ao terminar, pare e solicite aprovação para a próxima fase.
