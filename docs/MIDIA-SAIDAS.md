# Mídia — Saídas do canal

Em **Mídia → canal → Saídas**, uma única página reúne endereços permanentes, estado real, opções dos protocolos e destino UDP. Copiar não cria uma nova identidade nem expiração. Excluir o canal continua invalidando seus endereços. Os endereços são credenciais de leitura desse canal: não concedem acesso administrativo.

## Protocolos

- **MPEG-TS e HLS MPEG-TS:** preservam os links existentes e o fluxo compartilhado. HLS continua verificando H.264/AAC e depende da autorização da matriz. Seu segmentador atual mantém alvo de 2 segundos e janela de quatro segmentos.
- **DASH e HLS fMP4/CMAF:** um empacotador FFmpeg adicional lê o hub existente por pipe e gera os dois manifestos, sem transcodificar. Quantidade de segmentos de 2 a 10 e duração alvo de 1 a 6 segundos; o GOP da origem determina os cortes efetivos. Exigem H.264/AAC verificados e a mesma autorização de saída segmentada HLS. Não abrem uma origem adicional.
- **RTSP, RTMP e SRT:** MediaMTX v1.21.3 é um processo auxiliar do core, com configuração privada, recebendo MPEG-TS por socket Unix do mesmo fluxo. Não há serviço systemd adicional. RTSP usa TCP; RTMP/RTSP simples devem circular na rede privada/VPN escolhida; SRT exige autenticação e criptografia AES. Compatibilidade depende dos codecs aceitos pelo protocolo/receptor; não há transcodificação implícita.
- **PLAYER:** página HLS incorporável por iframe, reutilizando a biblioteca HLS.js já distribuída. O dashboard permanece bloqueado para framing; somente os caminhos exatos do player protegido por chave podem ser incorporados.
- **API:** estado e métricas do canal em JSON, sem origem ou credenciais administrativas.
- **UDP multicast:** usa o mecanismo existente com interface/IP/grupo/porta explícitos, validação antes de enviar, TTL 1 e permissões vigentes.

MSS, os formatos proprietários M4F/M4S da referência, SHOUTcast, JPEG, MSE de baixa latência, API de gravação, sobreposição de logos e legendas ainda não são implementados. A página identifica esses formatos como indisponíveis e não inventa URLs.

## Uso

1. Cadastre a origem e confira a recepção na Visão geral.
2. Abra Saídas. TS/HLS já possuem endereços permanentes; estado reservado ou parado não significa sinal disponível.
3. Marque os protocolos desejados. DASH/CMAF habilita também a preparação HLS necessária à autorização segmentada. Instalações sem FFmpeg/FFprobe não oferecem preparação segmentada.
4. Para RTMP/RTSP/SRT, instale a ferramenta opcional, revise **IP local de escuta**, **domínio/IP anunciado** e as portas de cada listener. Campos começam vazios; porta vazia deixa o protocolo global desativado. Portas de listeners devem ser 1024–65535; RTSP/RTMP não podem compartilhar a mesma porta TCP.
5. **Ativar os listeners confirmados** é uma escolha explícita. Salvar só inicia o auxiliar quando há um canal ativo com esses protocolos. Não altera firewall, rotas, NIC, NAT ou WireGuard. Salvar em canal pausado não inicia a recepção.
6. Confira os estados. “Enviando ao servidor de saída” é confirmação de entrega ao auxiliar; não é uma medição do player remoto. “Disponível” em DASH exige manifesto atualizado, e a falta de novos segmentos altera o estado para aguardando sinal.
7. Copie a URL adequada. O cliente HTTPS precisa confiar no certificado do Edge; o IP/domínio anunciado dos outros protocolos precisa ser alcançável pelo receptor.

A configuração de portas é global: alterá-la reinicia somente o auxiliar e desconecta seus leitores. Alterar opções de um canal pode reconstruir seu empacotador/feed. TS/HLS existentes e recepção são independentes, exceto quando se altera a preparação HLS ou o destino UDP, que reaplica a configuração existente somente no canal selecionado. Falha numa etapa informa que é necessário recarregar e conferir o estado salvo antes de repetir; não afirma rollback integral entre stores diferentes.

## Ferramenta opcional

Revise o script e seu plano no servidor de teste:

```sh
bash scripts/install-output-tools.sh --plan
# Somente após aprovação operacional para esse host:
sudo bash scripts/install-output-tools.sh --apply --lab-only
```

O script baixa o binário Linux amd64 oficial v1.21.3, verifica SHA-256 fixado e preserva a licença MIT em `/opt/kore-edge/tools/mediamtx-v1.21.3`. Cria somente o link controlado `/usr/local/bin/mediamtx`. Recusa substituir instalação desconhecida. Não usa APT, systemctl, nem inicia listeners. O core detecta ferramentas no início; a aplicação de nova release/reinício do core segue aprovação operacional existente.

Pré-verificação: arquitetura, destinos sem symlink inesperado, ausência de instalação conflitante e revisão dos IPs/portas. Rollback: desativar os novos listeners no painel; restaurar release/configuração anteriores se necessário e remover apenas o symlink conhecido da ferramenta. Preservar todos os arquivos privados. Não remover binário de um auxiliar ainda em uso. Pós-validação: prontidão do core, estados de saída, leitura de canal piloto em cada protocolo e preservação das pausas manuais.

## Persistência, segurança e limites

`users/outputs/outputs.json` é privado (0600), com revisão monotônica e substituição atômica. O schema legado de `live/distribution.json` não muda. A versão anterior ignora as novas saídas; links TS/HLS permanecem iguais. A configuração não contém chaves de mídia; as credenciais por canal são derivadas da identidade permanente existente.

Arquivos temporários do auxiliar e sockets Unix ficam em diretório 0700 exclusivo. As chaves SRT necessárias ficam somente nesse diretório privado; nunca em argv/logs. Hooks, publicação por clientes, API, métricas, gravação, HLS, WebRTC e MoQ do auxiliar ficam desativados. A autenticação HTTP interna é vinculada apenas a loopback e valida ação de leitura, protocolo habilitado, canal ativo, grant vigente e chave permanente com comparação em tempo constante. A pausa/revogação remove o feed/path; a reconexão de novos clientes continua negada.

O core não cria listeners externos só por abrir a página. Endereços e portas são persistidos apenas por POST administrativo com sessão, CSRF e confirmação da configuração. O endpoint não permite escolher executáveis, filtros, argumentos, comandos, caminhos arbitrários ou hooks. O auxiliar recebe apenas o caminho de seu arquivo privado. Saídas alimentam-se do hub e nunca reabrem a matriz.

Há até 32 canais auxiliares simultâneos, com espera explícita para os demais. Empacotadores usam as vagas de saída existentes e liberam recursos no cancelamento. Cada canal limita leitores RTMP/RTSP/SRT entre 1 e 32. Esse número não é uma contagem de usuários HLS e não modifica as cotas anteriores de TS/HLS. Segmentos são limitados a 8 MiB por resposta, quatro respostas segmentadas simultâneas e 128 MiB/100 arquivos por empacotador. Estouro encerra essa preparação, sem parar o ingest. O processo MediaMTX usa GOMAXPROCS=2 e GOMEMLIMIT=256MiB; os limites do serviço do core continuam valendo. Falhas do auxiliar usam reconexão com espera limitada a 30 segundos.

API administrativa: `GET /api/v1/live/outputs?id=ID` e `POST /api/v1/live/outputs`. GET retorna revisão, rede, opções do canal, disponibilidade de ferramentas, estados e links dos protocolos configurados. POST exige `channel_id`, revisão, `confirmed`, objeto `channel` completo e `network` opcional. Edição de um canal não estende grants da matriz.

## Validação

Testes usam origens sintéticas e loopback: persistência/revisão, padrão desligado, configuração inválida, autenticação somente leitura por protocolo/canal, revogação, travessia de caminhos, limites e encerramento. O ensaio Linux opcional com `KORE_OUTPUT_FFMPEG` e `KORE_OUTPUT_MEDIAMTX` lê DASH, RTSP, RTMP e SRT criptografado com FFprobe e confirma H.264/AAC usando uma única origem sintética. A CI baixa as versões fixadas sem instalação de pacotes ou mudança de serviços.

Referências públicas utilizadas na integração: [configuração MediaMTX](https://mediamtx.org/docs/references/configuration-file), [MPEG-TS por socket Unix](https://mediamtx.org/docs/static-sources/mpeg-ts), [autenticação](https://mediamtx.org/docs/features/authentication) e [release v1.21.3](https://github.com/bluenviron/mediamtx/releases/tag/v1.21.3). Nenhum código proprietário foi utilizado.
