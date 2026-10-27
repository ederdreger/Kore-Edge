# Cadastro manual e compatibilidade de entradas

## Uso no painel

1. Entre no Edge com sua conta administrativa e abra **Canais → Adicionar canal**.
2. Cole o **link de saída** do canal na matriz. O protocolo aparece imediatamente.
3. Ajuste nome e categoria, se desejar. Sem nome, o servidor usa o nome extraído
   do caminho ou do stream ID. Não são criados canais de demonstração.
4. Para HTTP sem extensão, selecione o formato informado pela matriz. Não há botão de teste no cadastro.
5. Clique em **Salvar canal**. Um canal habilitado inicia automaticamente ou aguarda vaga.
6. Use o botão alternado **Iniciar/Parar** para controlar a recepção. HLS é preparado quando autorizado; ferramentas/codecs incompatíveis ficam visíveis no status.
7. Use **Reproduzir canal**. A prévia compartilha o fluxo já aberto e fechá-la não
   interrompe a recepção. Canais manuais parados não são iniciados pelo player.
8. Em a engrenagem do canal, configure HLS local e/ou multicast UDP, se necessários.
   As saídas ficam no próprio canal. Para editar a origem,
   pare a recepção e use **Editar origem**. Ao editar apenas o nome/categoria,
   deixe o link vazio: usuário, senha e parâmetros cadastrados serão preservados.

Não há nova pergunta de senha nessas operações: a sessão administrativa já
autenticada, o Origin, o token CSRF e a auditoria protegem as requisições.
Espectadores não podem cadastrar, testar ou operar canais. Uma sessão expirada,
revogada ou cuja conta foi desabilitada continua sendo recusada. Configuração
WireGuard, contas e outros módulos mantêm suas políticas próprias.

## Links compatíveis

Os endereços e portas abaixo são exemplos de formato; substitua pelos dados
revisados da sua matriz e rede, sem copiar os exemplos para ativar tráfego.

| Entrada | Exemplo | Funcionamento |
| --- | --- | --- |
| HTTP MPEG-TS | `http://media.example/CANAL/mpegts` | Recepção direta do MPEG-TS. HTTPS e caminhos `.ts` também são aceitos. |
| HLS | `https://media.example/CANAL/index.m3u8` | Remux com FFmpeg, `-c copy`, sem transcodificar. |
| SRT | `srt://media.example:12000?streamid=#!::r=CANAL,m=request` | Edge caller recebe da matriz listener; stream ID preservado. |
| UDP multicast | `udp://239.77.0.1:12000` | Recebe MPEG-TS no grupo da interface confirmada. Aceita também `udp://@239.77.0.1:12000`. |
| UDP unicast | `udp://IP_LOCAL_DO_EDGE:PORTA` | Abre a recepção naquele endereço local; a matriz deve enviar para ele. |

Para UDP unicast, **não informe o IP da matriz como endereço de escuta**. UDP não
faz uma requisição de canal: a matriz precisa estar configurada para enviá-lo ao
Edge. Selecione a interface e um IP pertencente a ela; multicast fica restrito a
239.0.0.0/8. Nenhuma interface, endereço, rota ou regra de firewall é configurada.
Entrada UDP é MPEG-TS bruto; RTP encapsulado não é aceito nesta entrega.

SRT aceita `mode=caller` (padrão), `streamid`, `passphrase`, `pbkeylen` 16/24/32,
`latency`, `rcvlatency`, `peerlatency`, `transtype=live` e `pkt_size=1316`.
Latências na URL usam **microssegundos**: `latency=200000` corresponde a 200 ms.
Porta é obrigatória. A passphrase da origem, quando exigida, é diferente da senha
administrativa do Edge e permanece no link privado. SRT sem criptografia também
é aceito quando a origem não a exige; o painel não inventa uma chave.
Listener e rendezvous no Edge não fazem parte deste cadastro.

Referências oficiais: [reprodução Flussonic](https://flussonic.com/doc/fms/play/)
e [SRT playback](https://flussonic.com/doc/fms/play/srt/). Os outros formatos
exibidos no Flussonic (DASH, RTMP, RTSP, etc.) não são entradas deste módulo.

## Separação de origem e qualidade

Detectar o formato do link não comprova que há sinal. O teste explícito verifica
pacotes MPEG-TS recebidos, com prazo máximo de sete segundos e duas amostras
concorrentes. Verificar imagem, áudio, codecs e estabilidade exige o player e um
piloto autorizado; não se promete corrigir uma origem danificada.

HTTP MPEG-TS, UDP e SRT preservam o transporte. HLS usa relay HTTP loopback
privado, referências opacas e master playlist em stdin do FFmpeg. Não há URL,
passphrase ou capacidade privada em argv. Variantes, segmentos e chaves HLS
permanecem na mesma origem; links externos e redirecionamentos são recusados.
As referências mantêm extensões de mídia para compatibilidade com FFmpeg moderno.

A saída HLS local exige H.264/AAC verificados pelo módulo já existente. MPEG-2,
HEVC ou outros codecs podem ser recebidos em TS/SRT, mas não se presume que o
navegador ou HLS local consigam reproduzi-los. Não há transcodificação automática.

## Persistência, permissões e limites

- `accounts/manual-channels/channels.json` fica fora do checkout, em diretório
  privado 0700 e arquivo 0600, com revisão monotônica e substituição atômica.
- A listagem expõe somente nome, categoria, protocolo, interface e endereço sem
  usuário, senha ou query string. Mensagens de erro e auditoria não incluem URLs
  privadas ou segredos. A política de logs do operador/receptor também deve
  proteger os links fornecidos.
- Canais manuais são de responsabilidade do administrador local. Não dependem
  de sincronização, WireGuard ou grants do Kore Play. Seu namespace é `local_`
  seguido de identificador aleatório. Canais importados continuam delegando
  autorização, validade e revogação à matriz em cada uso; misturar as listagens
  não estende uma lease assinada expirada.
- Mutação por sessão revalida conta, papel e revisão; operações de canais são
  serializadas e auditadas. Revisões do cadastro manual e da recepção impedem
  iniciar uma origem com uma revisão antiga aberta em outra sessão.
- Cadastro ou alteração não ativa rede/mídia. Ao trocar o link, a retomada após
  restart fica desmarcada e precisa ser revisada novamente. Para editar/excluir,
  pare o canal primeiro. Cadastro desabilitado não autoriza recepção ou saída.
- Continuam valendo os limites do `live`: 16 recepções simultâneas, 512 planos,
  fan-out limitado, reconexão com backoff limitado a 30 segundos e detecção de
  ausência de mídia. A capacidade real do servidor exige homologação.
- HLS tem até 1 MiB por playlist, 32 MiB por segmento/resposta e 1.024 referências
  opacas por canal. Requisições são limitadas, com timeouts de conexão e resposta.
- DNS é validado e o IP é fixado na conexão. São recusados loopback, endereços
  não especificados, link-local, metadados e multicast como origem HTTP/SRT.
  Destinos privados de matriz são aceitos, sem alterar rotas existentes. HTTPS
  verifica certificados normalmente; não existe opção de ignorar TLS.

## Testes e aceite

Testes sintéticos cobrem detecção dos quatro protocolos, o stream ID Flussonic
literal `#!::`, SRT real em loopback com e sem criptografia, UDP sem truncar
datagramas, remux HLS com variantes/Basic auth, limites de referências, bloqueio
de origens inseguras, persistência, credenciais ocultas, revogação de sessão,
CSRF, isolamento de grants e recepção contínua compartilhada com o player.

```powershell
# Use caminhos locais existentes para os seus executáveis; não instale serviços.
go vet ./...
go test ./...
npm --prefix web test
npm --prefix web run build
# Opcional local; obrigatório no CI com FFmpeg portátil fixado por SHA-256:
# KORE_MANUAL_FFMPEG aponta para o executável ffmpeg ou ffmpeg.exe confiável.
go test ./internal/manualchannels -run TestNativeHLSRemux -count=1
```

O CI também executa `go test -race`. Nenhum teste acessa a matriz/Flussonic real.
Aceite operacional posterior: cadastrar um canal piloto, testar, iniciar sem
player, conferir avanço de bytes/bitrate, abrir/fechar prévia sem reabrir origem,
e só então revisar uma saída. UDP multicast real exige interface/grupo/porta
confirmados pelo operador; o teste local não é homologação de rede.

## Instalação e rollback

A entrega de código não reinicia o servidor. Antes de atualizar o Edge, obter a
autorização da janela, confirmar o host de laboratório e executar a pré-verificação:

```sh
git status --short
bash scripts/update-panel.sh --plan
```

O plano deve mostrar endereço/porta atuais e o impacto nos canais. Registrar o
backup indicado pelo atualizador, preservar contas, identidades, cadastros e
configurações de recepção. Em janela autorizada, o atualizador `--apply` valida
prontidão e HTTPS e restaura configuração/release anteriores se falhar.
Rollback operacional posterior: parar apenas os serviços Kore Edge autorizados e
restaurar os arquivos e o symlink do backup dessa execução. Não reinstalar o
sistema, não alterar rede/firewall e não regenerar as credenciais WireGuard.

Pós-validação: `/healthz` e `/readyz` 200, API de cadastros sem sessão 401, nova
tela disponível, cadastros anteriores preservados e nenhuma origem nova iniciada
automaticamente. A retomada de planos já autorizados segue o contrato de `live`.

A navegação de saídas foi consolidada em **Canais**. Use a engrenagem ou a engrenagem do canal para preparar cada canal; os endereços antigos `#udp` e `#hls` abrem essa tela. A janela abre antes da consulta de ferramentas e permite repetir uma consulta que falhou. Inventário indisponível impede UDP, sem inventar interfaces ou alterar o sandbox do serviço.
