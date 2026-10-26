# Fase 06 — Multicast UDP

## Implementação

`internal/multicast` valida destinos, descobre interfaces em leitura, prepara
dry-run e implementa socket UDP IPv4 no Linux. Não altera NIC, rotas, firewall,
WireGuard ou serviços. Windows permite inventário/dry-run, sem envio implementado.
A fonte continua sendo o MPEG-TS nulo sintético da Fase 05, sem mídia reproduzível.
Não há integração com matriz, Flussonic, HLS ou transcodificação.

São obrigatórios: ID do canal, nome da interface, IP de origem RFC1918 que
pertença à interface, grupo em `239.0.0.0/8`, porta 1–65535 e TTL exatamente 1.
Interface deve estar ativa, suportar multicast, não ser loopback/ponto-a-ponto
e ter MTU de pelo menos 1.344 bytes (1.316 payload + 28 IPv4/UDP).
Não se assume nome/IP/interface/porta. Grupos/portas repetidos e IDs duplicados
são rejeitados; máximo quatro destinos, um por canal. A VLAN correta deve ser
confirmada pelo operador; flags do host não comprovam isolamento de switch.

O Linux fixa `SO_BINDTODEVICE`, `IP_MULTICAST_IF` (índice e endereço),
`IP_MULTICAST_TTL=1` e `IP_MULTICAST_LOOP=0`, além de solicitar send buffer de
64 KiB (o kernel pode ajustar/dobrar esse valor). Qualquer erro de configuração
fecha a abertura; não há fallback para NIC/rota default. A porta de origem é
efêmera escolhida pelo SO; a porta de destino é explícita. Não ingressa em grupos
IGMP no emissor; receptores/switches precisam estar preparados separadamente.
Referências: [Linux ip(7)](https://man7.org/linux/man-pages/man7/ip.7.html) e
[socket(7)](https://man7.org/linux/man-pages/man7/socket.7.html).

`SO_BINDTODEVICE` pode exigir privilégios/capabilities conforme o host. O modelo
systemd anterior não foi alterado para concedê-los; falha de permissão mantém
a saída indisponível. Não executar como root nem ampliar capabilities
automaticamente: isso exige revisão/aprovação operacional específica.

## Configuração e API

Após descobrir e confirmar os valores do laboratório, configurar no YAML:

```yaml
multicast:
  mode: dry_run
  outputs:
    - channel_id: test-pattern
      interface: lab0
      source_ip: 10.250.0.2
      group: 239.18.10.1
      port: 12000
      ttl: 1
```

Todos os valores acima são ilustrativos, não uma seleção aprovada. Exige
`stream.enabled: true` e confiança do catálogo. Bloco ausente: desabilitado;
`dry_run`: não abre sockets nem envia, mesmo que o canal seja iniciado.
Somente `mode: send`, após aprovação operacional específica e reinício local
aprovado, habilita criação de saída ao iniciar um canal configurado. O bloco
é imutável durante a execução; não existe endpoint para ativar envio/mudar NIC.

- `GET/HEAD /api/v1/multicast/interfaces`: inventário real com timestamp, nomes,
  índices, IPv4, MTU e flags; 503 se consulta indisponível. Não é simulação.
- `POST /api/v1/multicast/plan`: recebe um Target JSON, valida contra interfaces
  descobertas, retorna plano `dry_run`, `applied: false`, índice, tamanho de
  datagrama, pré-verificações, rollback e pós-validação. Nunca abre socket.
  Corpo até 2 KiB, JSON estrito, erros 400; indisponibilidade de inventário 503.
  O plano não testa permissões do kernel nem garante entrega/isolamento físico.
- Start/stop continuam em `/api/v1/streams`. Em `send`, o canal configurado
  exige grant `udp` no catálogo; grant apenas `hls` não basta. Permissão é
  verificada antes de abrir a saída e antes de cada pacote. Uma escrita já
  iniciada pode terminar durante revogação/parada, dentro do timeout.

Exemplo JSON de dry-run: `configs/multicast-target.example.json`, também sintético.
Após substituir os valores por dados descobertos e confirmados, usar o endereço
local anteriormente aprovado:

```sh
curl --fail http://127.0.0.1:18080/api/v1/multicast/interfaces
curl --fail -H 'Content-Type: application/json' --data-binary @configs/multicast-target.example.json http://127.0.0.1:18080/api/v1/multicast/plan
```

## Integração, contadores e falhas

O mesmo ingest/processo fornece os pacotes para a saída, sem duplicar fonte.
Sete pacotes formam um datagrama de 1.316 bytes; buffer parcial é descartado
na parada/falha, sem flush após cancelamento. Timeout de escrita: 100 ms;
abertura do socket: até um segundo. Não há fila ilimitada ou reenvio de datagrama
falho. UDP não garante entrega, ordenação ou ausência de duplicatas na rede.

GET de streams inclui `udp` quando há saída criada: bytes/datagramas entregues
ao socket, erros e pacotes ainda no buffer. Contadores são da tentativa atual
(ou última após fechar), não acumulados entre reaberturas. `submitted_to_socket`
não significa recepção pelo parceiro. Os testes mockados não produzem métricas
de rede em runtime. A origem sintética continua explícita no status do stream.

Erro de saída falha somente aquele canal e consome o orçamento de reinícios da
Fase 05; outro canal saudável permanece ativo. Revogação produz `unauthorized`.
Não há HLS paralelo nesta fase. Encerramento recolhe processo e fecha socket.
A autorização por pacote usa leitura do manifesto validado sob lock, sem
decodificar/copiar o catálogo inteiro no caminho de dados.

## Pré-verificação e ensaio operacional futuro

**Nenhum envio real foi executado nesta entrega.** Antes do ensaio, confirmar
host Ubuntu isolado, interface/VLAN/IP/grupo/porta, ausência de conflitos,
receptores sintéticos, IGMP snooping e fronteira multicast do switch/upstream.
Registrar baseline sem alterar o host. Comandos de inspeção propostos, ainda
não executados no laboratório:

```sh
cat /proc/net/dev
cat /proc/net/route
cat /proc/net/ipv6_route
cat /sys/class/net/<IF_LAB_CONFIRMADA>/operstate
cat /sys/class/net/<IF_LAB_CONFIRMADA>/mtu
```

Apresentar os valores reais e solicitar aprovação antes de habilitar `send`.
Após aprovação, capturar cabeçalhos (sem gravar payload/clientes), ao mesmo
tempo na interface aprovada e em **todas** as não laboratoriais e link upstream:

```sh
tcpdump -ni <IF_CONFIRMADA> -vv 'udp and dst host <GRUPO_CONFIRMADO> and dst port <PORTA_CONFIRMADA>'
```

Substituir placeholders antes de qualquer execução; permissões de captura devem
ser revisadas sem elevar privilégios automaticamente. Confirmar origem, TTL 1,
payload 1.316 e taxa esperada na VLAN aprovada; exigir zero correspondências nas
demais interfaces/link upstream. TTL 1 não impede vazamento na camada 2 nem
substitui isolamento de VLAN/switch. Não alegar aceite sem essas evidências.

Rollback: DELETE do canal, aguardar estado terminal/socket fechado, confirmar
cessação das capturas; restaurar `dry_run` ou configuração/binário anterior e
reiniciar somente agente local mediante aprovação. Remover bloco `multicast`
ao voltar para binário anterior à Fase 06. Comparar rotas/firewall/NIC com
baseline: o módulo não os modifica. Não é necessário reiniciar serviço externo.

## Testes

Fixtures de NIC e socket falsos verificam validação, origem/interface exatas,
ausência de fallback, ordem/falha das opções, TTL e loopback, datagramas,
timeout/short-write, cancelamento e descarte de buffer parcial. Integração
stream→sender é testada sem rede, com ingest único e isolamento de falha por
canal. Testes de catálogo distinguem autorização UDP de HLS e snapshots de
configuração preservam isolamento das listas mutáveis. API de planejamento
também usa inventário injetado; nenhum teste de multicast abre socket real.

Resultado em 2026-10-25 com Go 1.24.9 portátil no Windows: `go test ./...`,
`go vet ./...` e formatação passaram. Builds Windows e Linux amd64, vet Linux
e compilação do teste multicast Linux passaram. `go test -race ./...` foi
tentado e bloqueado por ausência de CGO/compilador C. O novo binário Windows
está em `bin/edge-agent-phase06.exe`, sem substituir a instância local antiga.
Pasta sem `.git`: não foi possível criar a branch.

Aceite de código/testes mockados concluído; **aceite operacional de tráfego
restrito à VLAN e ausência de vazamento permanece pendente**, assim como
execução nativa no Ubuntu. Os testes de opções verificam o contrato do adaptador;
não comprovam comportamento real do kernel, permissões ou isolamento físico.

## Objetivo
Saída por interface explícita, TTL, endereço e porta.

## Pré-requisitos
Fase anterior aprovada, `AGENTS.md` lido, laboratório isolado.

## Entregáveis
- Implementação modular e documentada.
- Testes automatizados e exemplos de configuração sem segredos.
- Procedimento de validação e rollback, quando aplicável.

## Critério de aceite
Tráfego restrito à VLAN de laboratório; sem vazamento.

## Prompt para Codex
Leia `AGENTS.md`, `README.md`, `docs/ARCHITECTURE.md` e este documento. Desenvolva somente a Fase 06 (Multicast UDP): Saída por interface explícita, TTL, endereço e porta. Antes de editar, apresente um plano, dependências, testes e riscos. Não execute mudanças privilegiadas no host. Entregue código, testes e documentação. Demonstre como validar: Tráfego restrito à VLAN de laboratório; sem vazamento. Ao terminar, pare e solicite aprovação para a próxima fase.
