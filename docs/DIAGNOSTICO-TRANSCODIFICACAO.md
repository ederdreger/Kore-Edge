# Transcodificacao: dimensionamento e validacao

## Evidencias de 29/10/2026

Consulta somente leitura ao Flussonic de teste, pela API e configuracao publica
ao administrador, sem acessar codigo-fonte: HISTORY2 usa NVENC na GPU 1,
H.264 720p30, preset medium, entrelacamento e AAC. Na amostra de 20 segundos:
zero retries, coder_error=false e transcoder_overloaded=false.
O Edge nao tem NVIDIA detectada e usa libx264 na CPU. Esses perfis nao possuem
o mesmo custo de processamento. A presenca de renderD128 nao comprova que
VAAPI funciona no isolamento do servico; o ensaio sintetico do painel e necessario.

HISTORY2 importado e Kore-History local sao registros diferentes. A primeira
medicao de quota acompanhou o local. A verificacao posterior do HISTORY2
importado registrou 18,31 fps, speed=0,736 e source_idle_timeout, com alvo 25 fps.
O teste anterior confirmou quota global de 4 CPUs atingida em 86% dos periodos;
o ensaio de 8 CPUs usou em media 6,14 CPUs, mas o canal local permaneceu em fila.
Isso nao valida a reproducao do HISTORY2 nem prova uma causa unica das quedas.

## Quanto exige um canal?

Nao existe numero fixo de nucleos por canal. Medir entrada, resolucao, FPS,
codec, preset, filtros, quantidade de trilhas e hardware utilizado. Memoria,
limite de tarefas, fonte e consumidores lentos tambem podem interromper o fluxo.

Ensaio sintetico separado no Edge, como nobody, sem rede, sem gravar arquivos
e sem alterar os canais: 12 s de testsrc2 1920x1080p25, saida H.264 1280x720p25
a 3500 kbit/s, sem audio, filtros limitados a duas threads.

| Perfil CPU | Threads encoder | Tempo real de execucao | CPU consumida | CPU por segundo de video |
| --- | ---: | ---: | ---: | ---: |
| medium, entrelacado | 4 | 5,815 s | 17,872 s | 1,489 |
| medium, entrelacado | automaticas | 3,988 s | 20,343 s | 1,695 |
| veryfast, progressivo | 4 | 1,462 s | 6,889 s | 0,574 |

CPU por segundo de video = (tempo user + system do processo) / duracao de midia.
Sao referencias sinteticas, sem decodificacao da entrada H.264 real ou audio:
nao representam o custo total de um canal de producao. Nao usar para prometer
quantidade de canais. Threads automaticas aumentaram o throughput e tambem
o consumo: mais paralelismo nao significa trabalho gratuito. Nenhum preset
ou entrelacamento salvo foi alterado para obter esses resultados.

## Configuracao de capacidade

O agente nao fixa mais duas transcodificacoes. A admissao inicial escala com
as CPUs visiveis: CPUs/4, minimo 1, maximo 32 processos. Trata-se de um ponto
de partida, sem reserva de quatro CPUs por canal e sem garantia de capacidade.
O operador pode definir KORE_EDGE_TRANSCODERS entre 1 e 128. Conferir tambem
quota/cpuset do servico, RAM e tarefas antes de aumentar a admissao.

KORE_EDGE_ENCODER_THREADS=0 (padrao do agente) permite ao codificador CPU
escolher automaticamente suas threads. Valores de 1 a 64 permitem ajuste
explicito. Isso controla paralelismo, nao e quota de CPU, afinidade ou reserva.
Decodificador e filtros conservam paralelismo limitado a duas threads.
NVENC e VAAPI continuam exigindo selecao explicita e ensaio como usuario do
servico. Copy nao recebe transcodificacao automatica nem threads adicionais.

Essas variaveis nao mudam CPUQuota, memoria, NIC, firewall ou servicos.
GOMAXPROCS controla o runtime Go, nao os processos FFmpeg. Quotas externas
continuam soberanas. A atualizacao nao aplica o exemplo systemd de capacidade.

## Validacao e rollback operacional

Antes de alterar o Edge de teste: registrar release atual, cpu.max, memory.max,
pids.max, variaveis de capacidade, prontidao e hashes privados das configuracoes.
Nao publicar URLs, tokens, arquivos privados ou argv de processos de midia.

Proposta para revisao: remover a quota global de CPU do core com
`systemctl set-property --runtime kore-edge.service CPUQuota=infinity`.
Isso nao reinicia o processo. Rollback do teste:
`systemctl set-property --runtime kore-edge.service CPUQuota=400%`.
Nao executar sem aprovacao operacional especifica. Quota runtime nao sobrevive
ao reboot; uma mudanca persistente exige revisao do drop-in existente.

Para a nova release, guardar backup e usar o atualizador existente. Reiniciar
somente core/gateway apos aprovacao. Falha de validacao restaura release e
configuracao anteriores. Reverter variaveis de capacidade junto com a release.

Acompanhar o HISTORY2 pelo ID do catalogo, nao por correspondencia parcial de
nome. Medir por pelo menos cinco minutos: delta de quadros, timestamps de saida,
interrupcoes, motivo de queda, CPU/throttling, RAM, tarefas, continuidade de TS e
avanco dos segmentos HLS. O painel prefere a janela recente do progresso FFmpeg;
a media de toda a execucao permanece na API para comparacao. Ritmo sustentado
de pelo menos 1x e necessario; zero interrupcoes, segmentos avancando e reproducao
real tambem precisam ser verificados. Nao declarar resolvido apenas por speed=1x.

## Referencias de comportamento

- [FFmpeg: progresso e opcoes de processamento](https://ffmpeg.org/ffmpeg.html)
- [Flussonic: transcodificacao NVENC](https://flussonic.com/doc/fms/transcoder/hardware-transcoder/)
- [Flussonic: monitoramento do transcodificador](https://flussonic.com/doc/retroview/dashboard-transcoder/)

Implementacao Kore Edge independente, sem importar codigo ou componentes do
Flussonic. A consulta da referencia nao alterou seu servico ou configuracao.
