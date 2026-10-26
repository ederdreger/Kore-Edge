# Fase 14 — Homologação

## Escopo e decisão

Suíte de aceitação, testes de carga limitada, perda TS simulada, recuperação e
restart, regressão e checklist go/no-go. Resultado desta execução: **NO-GO para
implantação e homologação final pendente**. Entrega dos testes e relatório não
equivale à aprovação do sistema. Veja [evidências](../acceptance/phase14-report.md)
e [resumo JSON](../acceptance/phase14-report.json).

## Suíte de aceitação

`tests/acceptance` depende somente de Go e módulos já existentes:

| Cenário | Estímulo | Critério automático |
| --- | --- | --- |
| Carga HTTP | 512 GETs com 8 workers; health, ready, sessão e integração | 512 respostas 200, corpos drenados, zero erro |
| Perda TS simulada | 40 pacotes PID não nulo, omitir dois pacotes separados | 38 observados, duas quebras CC, alerta continuity_errors |
| Recuperação/capacidade | Um erro transitório de saída, canal saudável paralelo, limite de 2 ativos | Falho recupera em 2 tentativas; saudável permanece em 1; terceiro recusado |
| Restart de processo | Dois processos auxiliares do core, parada limpa e rebind no mesmo endpoint | Health/ready antes/depois; listener fecha; processos distintos |

O teste de restart inicia somente o próprio executável de testes com filtro de
helper. Usa `app.Serve`, sem ler configuração do usuário, sem executar o agente
instalado e sem sinalizar processos existentes. O helper publica apenas seu
endereço loopback temporário. Pipes de controle encerram o filho; deadline de
15 segundos limita cada filho em falha. Esse cenário não testa systemd, reboot,
crash ou persistência de data plane. O teste auxiliar isolado é infraestrutura,
não um quinto cenário independente de aceite.

Os testes usam portas efêmeras detectadas pelo sistema e não alteram NIC, rotas,
firewall, WireGuard ou serviços. Não há multicast emitido, cliente real, matriz ou
Flussonic. A perda é injetada nos dados em memória; CC detecta descontinuidades,
não calcula porcentagem de perda de rede nem certifica qualidade audiovisual.
Carga curta em loopback não estabelece SLA, número de clientes IPTV suportados,
vazão de produção, consumo máximo de CPU/RAM ou capacidade sustentada.

```text
go test ./tests/acceptance -count=1 -timeout=60s -v
go test ./internal/provision -run '^TestConcurrentConsumptionAcrossRegistryInstances$' -count=50 -timeout=60s
go test ./... -count=1
go vet ./...
gofmt -l cmd internal tests
go test -race ./...
```

`make acceptance` executa apenas a nova suíte, sem instalação. Para a regressão
do painel: `npm --prefix web run check`, `npm --prefix web test` e
`npm --prefix web run build`. No PowerShell use `npm.cmd` caso a política bloqueie
o wrapper `.ps1`; não altere a política do sistema para executar esses checks.

O teste HLS real é opt-in com paths absolutos revisados em `KORE_EDGE_FFMPEG`,
`KORE_EDGE_FFPROBE` e, opcionalmente, `KORE_EDGE_FFPLAY`. Execute
`go test ./internal/hls -run '^TestFFmpegIntegration$' -count=1 -timeout=60s -v`.
Usa fonte H.264/AAC sintética, servidor de teste em loopback e diretório temporário.
Não requer canal da matriz. FFplay usa sinks SDL dummy, sem janela.

## Plano de homologação Ubuntu pendente

Antes de qualquer mudança operacional, registrar host de laboratório, conta não
root, release/digests, configuração anterior, janela aprovada e responsável pelo
rollback. Os placeholders abaixo não são valores detectados ou aprovados:

```sh
id -u
uname -a
go version
go env CGO_ENABLED
stat -c '%a %U %G %n' "$CONFIG_PATH" "$RELEASE_DIR" "$CANDIDATE"
sha256sum "$CANDIDATE"
df -h "$RELEASE_DIR"
ss -ltn
```

Depois da revisão, executar os mesmos testes como conta não root e guardar
resultados privados sem credenciais/dados de clientes. Definir previamente a
carga alvo e limites de latência, CPU/RAM, bitrate e tempo de recuperação. O perfil
local de 512 leituras não substitui esse acordo. Compilação Linux no Windows não
substitui execução nativa, detector de corrida ou permissões POSIX.

Homologação física exige uma etapa separada e autorizada: identificar NIC/VLAN/IP/
grupo/porta em rede isolada, comprovar ausência de conflito e observar o estado
anterior. Só depois entregar comandos concretos, impacto, rollback e validação
para o host/NIC aprovados. Este módulo não executa `tc`, comandos de firewall,
alterações de NIC, WireGuard ou systemd. Não injetar perda na rede existente.

Cenários ainda obrigatórios: multicast limitado à VLAN autorizada/TTL, recepção
em dispositivos alvo, HLS com mTLS/proxy revisado, renderização visual e comparação
da dashboard com referência, carga sustentada e soak test, crash/power loss com
locks/arquivos temporários, atualização/rollback de serviço e verificação de logs.
Revisar também persistência antirreplay e contrato oficial antes de integração real.

## Regra de go/no-go

GO exige todos os itens obrigatórios aprovados no ambiente alvo, nenhuma falha
crítica aberta, nenhuma aprovação inferida de teste pulado e aceite humano com
responsável/data/release/ambiente. Um teste intermitente continua sendo falha,
mesmo se a próxima execução passar. Ausência de medição é pendência, não sucesso.

Nesta execução, duas de 50 repetições de concorrência do provisionamento falharam;
o restante da regressão em uma passagem passou. A causa de baixo nível não foi
isolada. O teste não foi removido, relaxado ou considerado aprovado por uma
repetição posterior. Investigar esse defeito é condição antes de fechar o aceite.

Rollback de uma futura troca de serviço seguirá a Fase 12: manter seleção/binário/
YAML anteriores privados, confirmar integridade e recursos externos, selecionar
o anterior com geração revisada, e trocar processo só com autorização operacional.
Após eventual rollback, conferir health/readiness, segurança, streams e alertas.
Troca de processo pode interromper streams; não há promessa de atualização contínua.
Os processos temporários desta suíte foram encerrados pelos próprios testes;
não foi necessário rollback de serviço ou configuração do host.

## Estado da entrega

Código de aceitação, documentação e evidências entregues. **Checklist não aprovado**;
não há liberação para produção ou acesso real à matriz. Não existe Fase 15 definida
no plano atual. O próximo trabalho deve tratar os bloqueios registrados e o aceite
de laboratório, sem presumir autorização operacional.
