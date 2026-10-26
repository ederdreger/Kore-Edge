# Fase 07 — HTTP/HLS

## Implementação

`internal/hls` faz preflight de codecs, segmentação, armazenamento temporário,
limpeza e entrega HTTP autorizada. HLS é desabilitado por padrão. Quando
habilitado, substitui o gerador nulo pela fonte FFmpeg sintética de barras/imagem
de teste 320×180, 25 fps, H.264 baseline/yuv420p e tom AAC mono 48 kHz. A geração
usa parâmetros fixos, uma thread de encoder e bitrate alvo limitado. Não aceita
URL, arquivo, comando ou filtro fornecido pela API. Não conecta à matriz.

Antes de habilitar, gera dois segundos em memória (até 4 MiB), consulta FFprobe
e exige exatamente um vídeo H.264 e um áudio AAC. Preflight tem orçamento de
10 s e resposta limitada a 64 KiB. HEVC/MP2, ausência de streams e ferramentas
incompatíveis são rejeitados; não existe fallback silencioso para transcode.
Este preflight cobre o gerador fixo atual, não fontes externas futuras.

O source encode cria a mídia sintética. O ingest MPEG-TS desse processo é único
por canal e compartilhado com UDP/HLS. O processo HLS recebe pipe e usa
**`-c copy`**, sem reencodificar. Protocolos de entrada do segmentador limitados
a pipe/file; argumentos de processos são fixos e sem shell. stdout/stderr desses
processos não são registrados. Fonte e métricas são identificadas como sintéticas.

## Configuração e execução local

Requisitos externos: FFmpeg com lavfi, libx264, AAC e muxer HLS, mais FFprobe
compatível. O binário Go não inclui essas ferramentas. Nenhum pacote, Nginx ou
serviço é instalado automaticamente. Configure confiança de catálogo e
`stream.enabled: true` conforme fases anteriores e adicione:

```yaml
hls:
  enabled: true
  ffmpeg: /absolute/reviewed/path/to/ffmpeg
  ffprobe: /absolute/reviewed/path/to/ffprobe
  root: /absolute/dedicated/temporary/hls-directory
```

No Windows, use caminhos absolutos locais, preferencialmente YAML com aspas
simples. A raiz é dedicada a mídia temporária, fora do Git e sem dados de clientes.
Ela é criada se necessário; não pode ser raiz de volume, arquivo ou symlink
direto. O manager cria um subdiretório exclusivo por execução e por tentativa
de canal, sem apagar a raiz configurada nem diretórios de outros agentes.
Não troque/sobreponha esses diretórios enquanto o agente estiver ativo.

```sh
go build -o bin/edge-agent ./cmd/edge-agent
go run ./cmd/edge-agent -config <arquivo-local-revisado.yaml>
```

Após configurar/carregar manifesto assinado vigente com grant `hls`, iniciar
o canal pelo endereço local já confirmado. Catálogo/identidade continuam em
memória; nenhum start é automático.

```sh
curl --fail -H 'Content-Type: application/json' --data '{"channel_id":"test-pattern"}' http://127.0.0.1:18080/api/v1/streams
curl --fail http://127.0.0.1:18080/hls/test-pattern/index.m3u8
```

Abrir `http://127.0.0.1:18080/player/test-pattern` e pressionar reproduzir.
Esperar alguns segundos pela primeira playlist; 404 antes dela ficar pronta
não significa que há HLS reproduzível. Consultar o estado do canal em streams.
O player é somente ferramenta de laboratório; não antecipa a dashboard da Fase 09.

O player usa HLS nativo quando disponível, ou hls.js **1.6.15** servido localmente,
sem CDN durante reprodução. Biblioteca, licença e SHA-256 estão em
`internal/hls/web/THIRD_PARTY.md`. CSP restringe recursos à própria origem/blob;
não há autoplay com áudio. Compatibilidade real depende do navegador/MSE/codecs.

## Acesso, playlists e expiração

- `GET/HEAD /hls/<id>/index.m3u8` e `/hls/<id>/seg<sequência>.ts`: exigem
  permissão `hls` no catálogo atual em **cada requisição**, inclusive segmentos.
  Sem grant/expirado: 403; sem arquivo/sessão válida: 404; outros métodos: 405.
- Nomes de canal/arquivo são estritos, sem listagem, travessia, arquivos `.tmp`
  ou symlinks. Tipos MIME corretos, `nosniff`, cache `no-store`, suporte a Range.
- Playlist live com janela de quatro segmentos, duração alvo de dois segundos,
  corte em keyframes da fonte, nomes baseados em epoch microseconds, marca de
  descontinuidade e publicação por arquivo temporário/rename. `delete_segments`
  mantém uma margem de segmento fora da playlist antes da expiração. URLs não
  são garantia de retenção por tempo fixo; clientes muito atrasados recebem 404.
- Parada, revogação ou erro encerra o segmentador; fechamento remove seus
  arquivos. Revogação bloqueia HTTP imediatamente; bytes já baixados/bufferizados
  no player não podem ser revogados. Isto não é DRM/autenticação de espectadores.

A proteção desta fase é catálogo assinado + HTTP em loopback; qualquer processo
local pode consultar mídia autorizada para o parceiro. Autenticação/RBAC/mTLS de
operadores/clientes é Fase 11. Não expor este endpoint publicamente.

## Limites e supervisão

Até quatro sessões HLS; fila fixa de 128 pacotes TS (24.064 bytes) por sessão,
mais buffer de pipe de sete pacotes. Enqueue limitado a 100 ms; fila cheia falha
a saída, sem crescer memória. Watchdog de 250 ms cancela segmentador por falta
de atualização da playlist por 10 s, mais de 16 arquivos, arquivo acima de 8 MiB
ou soma acima de 32 MiB por sessão. São limites monitorados, com possível
overshoot entre inspeções; não substituem quota/cgroup do SO.

HTTP limita oito solicitações simultâneas e leitura de arquivo a 8 MiB; excesso
retorna 429. File I/O local não tem cancelamento individual. Sessões/procedimentos
de fechamento recolhem os processos e removem somente subdiretórios possuídos.
Após crash/kill abrupto podem restar diretórios antigos; não são apagados
automaticamente para evitar remover execução de outro agente. Inspecionar e
limpar resíduos só com confirmação de propriedade/inatividade.

Se houver UDP e HLS, falha/revogação de uma saída fecha aquela saída e mantém
a outra saudável. Não existe loop de retries por saída: restaurar a saída exige
restart explícito do canal. Se todas as saídas falharem, vale o orçamento finito
de reinícios do Stream Engine. UDP só é aberto se `multicast.mode: send` estiver
explicitamente configurado/aprovado; habilitar HLS não habilita multicast.

## Nginx e operação futura

`deploy/nginx/kore-edge-hls.conf.example` é um modelo **não instalado**, com
placeholders de portas a descobrir/confirmar. Faz proxy de player e mídia para
o agente, sem cache e sem `alias` que contorne a autorização. O modelo escuta
somente loopback e não publica a API administrativa. Não reinicia nem modifica
Nginx/systemd/firewall. É preciso validá-lo com `nginx -t` após substituir os
valores e obter aprovação específica antes de instalar/recarregar.

Pré-verificação proposta: confirmar host isolado, executáveis e codecs, portas
livres, dono/permissões/espaço livre da raiz temporária, grants e estado anterior
do Nginx. Comandos de leitura a adaptar aos paths reais aprovados:

```sh
<FFMPEG_REVISADO> -version
<FFPROBE_REVISADO> -version
df -h <RAIZ_TEMPORARIA_REVISADA>
```

O modelo systemd antigo não foi alterado; revisar acesso de escrita à raiz,
limites de RAM/processos e permissões das ferramentas antes da implantação.
Qualquer alteração de unit/permissões/serviço exige aprovação operacional.
Rollback local: DELETE dos canais, confirmar fechamento/limpeza, encerrar o
agente e restaurar binário/configuração anteriores (remover bloco `hls` para
versão anterior). Reenviar catálogo após reinício. Para futura operação Nginx,
guardar/restaurar configuração e estado anteriores; comandos concretos serão
apresentados com o ambiente real antes de qualquer mudança.

## Testes e evidências — 2026-10-25

Unitários cobrem codecs compatíveis/incompatíveis, remux sem transcode,
autorização/HEAD/MIME/paths, player local, limpeza restrita, watchdog de quota,
configuração e ciclo de vida de fan-out. Integração real é opcional nas suítes
sem ferramentas; habilitar no PowerShell com paths absolutos revisados:

```powershell
$env:KORE_EDGE_FFMPEG = '<path-absoluto-ffmpeg.exe>'
$env:KORE_EDGE_FFPROBE = '<path-absoluto-ffprobe.exe>'
$env:KORE_EDGE_FFPLAY = '<path-absoluto-ffplay.exe>' # opcional
go test -v -run TestFFmpegIntegration ./internal/hls
```

Executado nesta entrega com distribuição portátil BtbN
`N-128094-g31855b2a70-20261025`, fora do repositório: preflight H.264/AAC,
geração real, HTTP em loopback/porta efêmera, decodificação de áudio **e vídeo**,
reprodução FFplay de áudio com relógio avançando em SDL dummy sem janela,
expiração de segmento antigo, bloqueio por revogação e remoção de temporários
passaram. FFplay live é parado deliberadamente após janela limitada de ensaio;
não se exige EOF de playlist live. Não houve renderização visual no navegador.

Testes de todos os pacotes, vet e builds Windows/Linux amd64 passaram.
`-race` foi tentado e bloqueado por ausência de CGO/compilador C. Execução nativa
no Ubuntu, instalação/validação Nginx e homologação visual do player em navegador
continuam pendentes. Instância local antiga não foi substituída; novo binário
em `bin/edge-agent-phase07.exe`. Pasta sem `.git`: branch não criada.

Referências: [muxer HLS do FFmpeg](https://ffmpeg.org/ffmpeg-formats.html#hls-2),
[FFprobe](https://ffmpeg.org/ffprobe.html) e [hls.js](https://github.com/video-dev/hls.js).

## Objetivo
HLS com segmentos, playlist e Nginx; compatibilidade de codecs.

## Pré-requisitos
Fase anterior aprovada, `AGENTS.md` lido, laboratório isolado.

## Entregáveis
- Implementação modular e documentada.
- Testes automatizados e exemplos de configuração sem segredos.
- Procedimento de validação e rollback, quando aplicável.

## Critério de aceite
Player reproduz canal sintético e segmentos expiram.

## Prompt para Codex
Leia `AGENTS.md`, `README.md`, `docs/ARCHITECTURE.md` e este documento. Desenvolva somente a Fase 07 (HTTP/HLS): HLS com segmentos, playlist e Nginx; compatibilidade de codecs. Antes de editar, apresente um plano, dependências, testes e riscos. Não execute mudanças privilegiadas no host. Entregue código, testes e documentação. Demonstre como validar: Player reproduz canal sintético e segmentos expiram. Ao terminar, pare e solicite aprovação para a próxima fase.
