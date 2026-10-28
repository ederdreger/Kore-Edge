# Entrada e transcodificação

Implementação própria do Kore Edge, baseada no comportamento observável do
servidor de referência e nas documentações públicas. Nenhum código do servidor
de referência foi importado. A consulta SSH foi somente leitura.

## Entrada

1. Em **Mídia**, abra o canal e a aba **Entrada**.
2. Adicione até oito URLs. Para substituir uma URL salva, digite a nova no campo
   correspondente. O endereço atual aparece sem credenciais ou parâmetros secretos.
   Campo vazio preserva integralmente a URL salva, incluindo autenticação e stream ID.
3. Reordene pela prioridade ou remova entradas. Deve restar pelo menos uma.
4. Em **Opções**, escolha o protocolo; para UDP, confirme interface e IP local.
   O tempo sem sinal opcional aceita 2–300 segundos e monitora ausência de payload.
   Vazio preserva os prazos existentes dos receptores. Não detecta imagem congelada.
5. Clique **Salvar configuração**. Só então as alterações são persistidas.

Uma origem saudável permanece aberta. Ao falhar, o agendador tenta a próxima URL.
Salvar reinicia somente este canal se estiver ativo; canais pausados continuam
pausados. Sair com alterações pendentes pede confirmação para descartá-las;
recarregar a página também tem proteção. Polling não sobrescreve o rascunho.

## Transcodificação

O padrão continua **Transmitir sem transcodificar**. Ativar processamento exige
escolher um dispositivo que tenha codificado uma amostra sintética com sucesso
como usuário do serviço. Nenhum driver é instalado ou alterado pelo painel.

| Controle | Efeito no Edge |
|---|---|
| Copiar áudio | Preserva todas as faixas de áudio recebidas, sem recodificação |
| AAC / bitrate | Codifica as faixas de áudio com o bitrate escolhido |
| Amostragem / canais | 32, 44,1 ou 48 kHz; mono ou estéreo |
| Volume | Ganho em dB; zero preserva o ganho original |
| Separar estéreo | Primeira faixa de áudio torna-se duas faixas mono; exige entrada estéreo |
| Copiar vídeo | Mantém o primeiro vídeo original, processando somente áudio |
| Dispositivo | CPU, NVENC ou VAAPI efetivamente detectado; decodificação permanece por CPU |
| Desentrelaçar | YADIF ou BWDIF em quadros identificados como entrelaçados |
| Recorte | `largura:altura:x:y`, números pares; deve caber no vídeo recebido |
| FPS / GOP | Taxa de quadros e intervalo entre quadros-chave; GOP vazio = dois segundos |
| Faixas | Até três vídeos H.264 com bitrate e dimensões próprios |
| SAR | Proporção do pixel de saída; 1:1 significa pixel quadrado |
| Redimensionar | Ajustar com bordas, preencher recortando ou esticar |
| Fundo | Cor das bordas no ajuste proporcional |
| Perfil / nível | Restrições de compatibilidade H.264; valores incompatíveis são rejeitados |
| Quadros B | Sequência IP, IBP, IBBP etc.; pode aumentar atraso de decodificação |
| Referências | Quantidade de quadros de referência, na CPU |
| Varredura | Progressiva ou entrelaçada; saída entrelaçada disponível na CPU |
| Predefinição | Compromisso entre esforço e compressão; NVENC usa presets equivalentes P1–P6 |
| GOP aberto | Disponível na CPU; desativado por padrão |
| Duplicar / remover | Edita as faixas do rascunho antes de salvar |
| Copiar configuração | Copia apenas o perfil, sem URLs, senhas ou chaves |

Opções não suportadas por um dispositivo ficam desabilitadas. Ao trocar o
dispositivo, o rascunho limpa opções incompatíveis. O backend também valida
todos os campos. Não aceita argumentos de processo, filtros ou caminhos livres.
O processamento permanece em subprocesso isolado e com limite de dois canais;
até três faixas por canal aumentam o consumo de CPU/GPU. Não altera perfis salvos
nem inicia transcodificação apenas por abrir a aba.

O MPEG-TS contém as faixas configuradas. HLS e mini player usam a primeira faixa;
esta etapa não gera variantes HLS adaptativas. Codecs H.265/AV1 não são oferecidos
neste perfil H.264. Verifique capacidade e compatibilidade antes de salvar.

## Funções da referência ainda indisponíveis

Publicação push, vídeo de reserva em arquivo, gravação da reserva e ingestão em
cluster dependem de módulos ainda ausentes. Os tempos individuais de áudio/vídeo
exigem monitoramento de quadros, diferente do watchdog de payload atual.
Logotipo depende de uma biblioteca privada de imagens ainda ausente. A tela
identifica essas limitações; não grava opções sem efeito. O modo de processo
interno do produto de referência não é reproduzido: o Edge preserva isolamento.

## Validação e aceite

- URLs continuam privadas; reordenar sem substituir preserva os segredos por chave.
- Rejeita configurações malformadas, excessos e injeção de filtros/argumentos.
- Testes sintéticos verificam duas resoluções no TS, áudio mono em 44,1 kHz,
  ganho e separação estéreo, cancelamento e liberação de recursos.
- Testes de interface verificam rascunhos, navegação, duplicação, remoção e GPU.
- Recepção em copy e perfis antigos permanecem compatíveis com a nova release.

Rollback após salvar um perfil com novos campos exige restaurar também o backup
privado de `processing.json`; releases antigas usam decodificação estrita.
Nunca copie esse arquivo para um diretório versionado ou compartilhe seu conteúdo.

## Referências de comportamento e implementação

- [Transcodificador: controles e semântica](https://flussonic.com/doc/fms/transcoder/internals/)
- [Troca de origens e tempos](https://flussonic.com/doc/fms/protocols/sources/)
- [Filtros FFmpeg](https://ffmpeg.org/ffmpeg-filters.html)
- [Opções de codecs FFmpeg](https://ffmpeg.org/ffmpeg-codecs.html)
