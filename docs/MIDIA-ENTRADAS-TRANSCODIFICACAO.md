# Mídia: entradas e transcodificação

## Uso

1. Abra **Mídia → nome do canal**. O player da **Visão geral** permanece dentro da mesma caixa, inclusive nas telas pequenas. Fechá-lo encerra somente a reprodução, sem parar a recepção contínua.
2. Em um canal local, abra **Entrada**. A URL existente aparece sanitizada como indicação; o campo fica vazio para não devolver credenciais ao navegador. Deixe-o vazio para preservar o endereço salvo.
3. Use **Adicionar URL**, selecione o protocolo se necessário e ordene pelas setas. Há até oito entradas HTTP MPEG-TS, HLS, SRT caller ou UDP MPEG-TS. UDP exige interface ativa e IP local confirmados, sem escolha automática.
4. **Salvar configuração** aplica a ordem. Apenas uma origem é aberta por vez. Falha de abertura tenta a próxima; falta de pacotes/encerramento é detectada pelo supervisor e a próxima tentativa roda a lista a partir da origem seguinte. Não há tráfego paralelo de sondagem nem retorno periódico à principal enquanto a reserva estiver saudável. Ao salvar, a prioridade volta à primeira URL.
5. **Selecionada** indica a última URL aberta; confirme **Recebendo**, horário dos pacotes e bitrate para verificar sinal. Uma origem selecionada pode estar em reconexão. A lista não afirma que as reservas estão online sem recebê-las.
6. Em **Transcodificação**, o padrão é **Transmitir sem transcodificar**. Para transformar o fluxo, escolha explicitamente **Transcodificar áudio e vídeo** e um dispositivo validado. Informe bitrate, resolução e FPS; escolha copiar áudio ou AAC estéreo/48 kHz.
7. Salvar entradas ou processamento reinicia somente o canal que já estava ativo. Um canal pausado continua pausado. URLs de saída permanentes são preservadas. Alterar processamento pode provocar um breve corte no canal selecionado.

As entradas dos canais importados continuam pertencendo ao Kore Play e não são substituídas nesta tela. Esses canais permitem configurar processamento local, sujeito aos grants atuais. A revogação na matriz continua encerrando o fluxo.

## Processamento real

Uma única transformação recebe MPEG-TS por pipe e alimenta o hub existente. Player, TS, segmentador HLS e UDP usam esse mesmo fluxo. URLs, senhas e tokens não entram em argv, stderr ou resposta da API. Sem transcodificar não inicia um codificador extra; entradas HLS continuam exigindo o remux existente.

Esta etapa oferece uma faixa de vídeo H.264, entre 200 e 30.000 kbps, dimensões pares até 3840×2160 e 1–60 FPS. Preserva proporção com bordas, usa GOP de dois segundos e nenhum quadro B. A opção AAC aceita 32–320 kbps. Copiar áudio preserva seu codec; não torna automaticamente esse áudio compatível com HLS ou navegador. A validação existente do HLS continua obrigatória.

Dispositivos candidatos são CPU/libx264, NVIDIA/NVENC e nós render VAAPI. Cada candidato precisa codificar um quadro sintético com sucesso, em comando limitado e com o usuário do serviço. O resultado é armazenado em cache por um minuto. Placa encontrada ou encoder compilado, isoladamente, não basta. A decodificação e o redimensionamento são por CPU; apenas a codificação usa a GPU selecionada. Não se instalam drivers nem se alteram permissões de dispositivos.

O modelo systemd existente usa `PrivateDevices=true`; placas físicas podem não estar visíveis ao core. A tela mostra somente os dispositivos utilizáveis dentro desse ambiente. Liberar acesso a GPU exige procedimento operacional separado e aprovação; o painel não afrouxa o isolamento do serviço. CPU permanece uma alternativa quando validada.

Limite desta etapa: dois canais transcodificados simultaneamente, independentemente das vagas de entrada/saída HLS. Saturação aparece como **Aguardando capacidade de transcodificação**, sem contar como queda de sinal. Cancelamento fecha a origem, cancela o processo e libera a vaga. Não há alteração automática dos limites de CPU/RAM/tarefas do host. Qualidade depende da origem, rede, codecs e capacidade; transcodificar não corrige perda de pacotes anterior ao Edge.

O bitrate do hub é medido após o processamento quando ele está ativo; não representa uma segunda medida independente da banda de entrada ou de saída física. A Visão geral identifica isso explicitamente.

Publicação push, vídeo de fallback, timeout por faixa de áudio/vídeo, ingest de cluster, múltiplas faixas de vídeo, logos e opções arbitrárias de FFmpeg não foram implementados nesta etapa. Permanecem os prazos existentes do supervisor: 20 segundos sem primeiro pacote e 8 segundos sem novos pacotes, com backoff e cancelamento.

## API e persistência

- `GET /api/v1/manual-channels/processing?id=ID`: revisão, entradas sanitizadas com chaves estáveis, origem selecionada e perfil.
- `GET /api/v1/manual-channels/encoders`: candidatos efetivamente validados por mídia sintética; somente administrador.
- `POST /api/v1/manual-channels`, `action: inputs`, `source: {id}`, `revision`, `confirmed: true`, `inputs`: lista ordenada. Uma chave existente com URL vazia preserva seu segredo. Chave vazia com URL cria nova entrada. Nenhum endereço privado é devolvido.
- Mesmo POST com `action: transcode` e `transcode: {mode: copy}` desliga a transformação. O modo H.264 exige `mode: h264`, `device`, `video_kbps`, `width`, `height`, `fps`, `audio` e `audio_kbps` quando AAC.

Sessão administrativa, CSRF, auditoria e revisão permanecem obrigatórios; não é solicitada novamente a senha. O corpo é limitado a 48 KiB. Não são aceitos caminhos, filtros, argumentos livres, protocolos adicionais ou destinos bloqueados pela política de origem existente.

`processing.json` permanece no diretório privado de canais manuais, 0600, com escrita atômica, até 512 perfis/4 MiB. A revisão pública soma origem, apresentação e processamento. O perfil local é associado ao hash da configuração-base da origem; alterar a origem pelo editor antigo invalida o perfil obsoleto. `channels.json` não muda de schema nem é reescrito por esta configuração. Reordenar não depende da posição para recuperar segredos.

Rollback de código: a versão anterior ignora `processing.json` e usa a origem-base, sem transcodificar. Portanto, uma origem alterada apenas no perfil não será aplicada pelo binário antigo. Preserve o diretório privado completo em backup, além de YAML, distribuição e release. Falha ao aplicar a recepção após persistir deve ser tratada recarregando a configuração e conferindo o estado; a API não afirma que um reinício malsucedido foi concluído.

## Critérios de aceite automatizados

- Player montado dentro da caixa da Visão geral; CSS elimina o posicionamento flutuante nesse contexto.
- Adição, reordenação, detecção de protocolo e preservação de URL protegida sem nova senha.
- UDP sem interface/IP assumidos; canal importado não recebe URLs locais por engano.
- Configuração abre em cópia, sem POST; GPU indisponível não aparece como selecionável.
- Revisões antigas, sessão ausente, viewer, ausência de CSRF e parâmetros inválidos rejeitados.
- Failover sequencial, cancelamento, credenciais sanitizadas, persistência privada e compatibilidade da origem-base.
- Salvar em canal pausado não o reativa; editar um canal não reinicia os demais.
- Ensaio nativo opcional com `KORE_MANUAL_FFMPEG` usa apenas mídia sintética, valida H.264 e resolução final com FFprobe e comprova liberação de processo/vaga após encerramento e cancelamento. A CI Linux executa esse ensaio.
- Dispositivos GPU reais dependem de validação no ambiente onde forem disponibilizados; testes de CPU não homologam uma placa inexistente no executor.
