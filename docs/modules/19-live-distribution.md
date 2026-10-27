# Distribuição real, SRT e saídas autorizadas

## Escopo implementado para revisão

Esta entrega liga as páginas Canais, UDP e HLS ao mesmo catálogo real do parceiro,
preservando os módulos sintéticos anteriores e seus limites. Atualiza Canais e
WireGuard conforme as referências fornecidas, sem reproduzir dados fictícios.

O operador escolheu Kore Play como emissor SRT pelo WireGuard. A porta 20000/UDP
está aprovada somente como proposta para revisão no painel; não foi autorizada
sua ativação automática. Interface, IP, grupo e porta multicast ficam sem valor
até confirmação no painel.

## Arquivos e plano

- `internal/live`: recepção contínua por canal autorizado, supervisão limitada,
  buffers, métricas reais, prévia compartilhada e saídas UDP/HLS.
- `internal/edgecatalog`: contrato de SRT e permissões explícitas de saídas.
- `internal/api/live.go`: administração por sessão, CSRF e revisão (sem repetir senha para canais);
  prévia/saídas sempre sob autorização corrente.
- `web/src/channel-workspace.tsx`, `wireguard-workspace.tsx`,
  e estilos compartilhados nas saídas: interface, seleção, revisão e operação reais.
- Kore Play: configuração SRT privada, auxiliar sem privilégios de rede gerais,
  permissões por canal e transporte exclusivamente para peers cadastrados.
- Testes sintéticos de transporte, revogação, isolamento, processos, erros,
  buffers, interfaces explícitas e interfaces web; README/guia atualizados.

Não haverá transcodificação por padrão, seleção implícita de NIC/porta/grupo,
abertura automática de listeners ou início de origens reais durante testes.
Novas saídas exigem permissão publicada pela matriz; grants antigos não ganham
permissões UDP/HLS implicitamente.

## Pré-verificação e operação

Verificar binários e codecs, identidade dos hosts de teste, endereço WireGuard,
porta livre, permissões, capacidade, backup e impacto antes de ativar SRT/saídas.
A configuração só será aplicada após a revisão humana no painel. Rollback:
parar os canais/saídas novos e restaurar configuração/release anteriores,
preservando contas e chaves. Validar prontidão, autenticação, handshakes,
recepção e reprodução por canal piloto autorizado antes de ampliar a carga.

## Limites e autorização

Um canal abre uma origem e compartilha um hub de aproximadamente 2 MiB. Limite
de 16 recepções ativas, 512 configurações, 32 prévias globais e 32 assinantes por
hub. Consumidor lento é desconectado sem bloquear a origem. Quatro tentativas
por ativação, com backoff limitado; primeiro pacote até 20 s e fluxo parado
detectado em 8 s. Não há retry infinito ou retomada implícita após restart.

A matriz limita SRT a 16 conexões globais e à cota do parceiro, uma conexão por
stream ID. Aceita somente IP privado cadastrado e criptografia. A porta única
multiplexa `parceiro:canal`. Segredo derivado por HMAC/X25519 fica em memória e
arquivo privado 0600, fora do checkout, sem API, argv ou logs. Runtime é
reavaliado a cada segundo; falha fecha autorizações. O listener não retoma
automaticamente após restart da matriz. Buffers são limitados e watchdogs fecham
conexões; não dependem de deadlines não implementados no gosrt 0.9.0.
MSS é 1400 e payload 1316 bytes. MTU e capacidade exigem piloto real.

O Edge confere presença do endereço na interface salva e seleção de origem UDP
antes do handshake, sem enviar datagrama nessa pré-verificação, e confere a
origem do socket conectado. Não cria rotas ou altera NIC. Isso não substitui
revisão da rede ou proteção contra alterações feitas por root.

Entrada HLS usa relay loopback privado e master playlist em stdin, permitindo
atualização da playlist de mídia sem expor a capacidade em argv. Referências são
restritas ao canal autorizado. Saída HLS verifica primeiro vídeo H.264 e primeiro
áudio AAC, sem transcodificar, e só sinaliza pronto com playlist real recente.
HLS local exige sessão; não cria distribuição anônima para dispositivos.

UDP exige interface/IP/grupo/porta explícitos, grupo 239/8 e TTL 1. Preparação
consulta inventário e não concede capabilities ao serviço. Mutações exigem
administrador, CSRF, senha e confirmação; Start confere a geração revisada.
Grants opcionais `outputs` não expandem permissões antigas. Header de recursos
preserva clientes antigos com JSON estrito.

## Validação

Testes sintéticos cobrem hub limitado, consumidores lentos, ingest compartilhado,
revogação, geração desatualizada, sessão/CSRF/senha/viewer, preparação sem abrir
origem e campos multicast vazios. O auxiliar SRT tem teste Go em loopback com
criptografia, chave errada, parceiro errado e revogação. O remux HLS possui teste
com FFmpeg real, TS sintético e referências opacas (requer FFmpeg disponível).
O catálogo central testa preparação SRT sem listener e compatibilidade de grants.

Isso não homologa banda, perdas, codecs de cada canal ou a rede física. A porta
proposta e os destinos multicast continuam pendentes de confirmação humana.
Veja o [passo a passo](../DISTRIBUICAO-AO-VIVO.md).

## Controle simplificado atual

Iniciar/Parar executam diretamente com sessão administrativa e CSRF, sem modal.
Configurar um canal ativo valida e persiste antes de cancelar o job antigo;
somente esse canal é reiniciado. A troca pode causar breve interrupção e fechar
uma prévia existente. Reabrir o player usa o novo fluxo compartilhado.

O reconciliador prepara e inicia entradas Kore Play autorizadas sem navegador,
com seleção existente preservada, até 16 ativas e 512 configurações. A fila
mostra Aguardando vaga. Parar persiste pausa e cancela retomada; Iniciar remove
a pausa. Quedas usam reconexão automática com backoff até 30 segundos, cancelável por Parar. Canais manuais habilitados entram na fila ao salvar.
Novas entradas preparam HLS apenas com grant; UDP exige destino confirmado; saídas antigas sem permissão
de retomada exigem início explícito. Inventário e destinos UDP continuam exigindo
revisão no formulário. Nenhum serviço do host ou rede é alterado pelo reconciliador.

Aceite automatizado: início sem configuração prévia, parada persistida após
reabrir armazenamento, fila e liberação de vaga, exclusão dos cadastros manuais,
revogação, rejeição de edição inválida sem interrupção, isolamento da
recepção vizinha e botão alternado sem confirmação. Testes usam fontes sintéticas.

## Recuperação e transporte contínuo

O watchdog detecta ausência de TS após 8 segundos (20 segundos no primeiro sinal),
fecha a tentativa e reconecta com backoff de 0,5 a 30 segundos. O job permanece
limitado a uma tentativa por canal e o limite global continua 16. Publica
`interruptions`, `last_failure_at`, `last_recovery_at` e `next_retry_at` reais.
Erros de escrita em saídas ativas também reiniciam apenas esse canal. Revogação
encerra a autorização; não se ignora grant para manter mídia aberta.

Cada consumidor HTTP possui fila de até 2 MiB, além do replay compartilhado
limitado a 2 MiB, e 32 consumidores globais. Escrita usa blocos de 32 KiB ou
flush em até 50 ms. Consumidor lento demais ainda é removido para proteger o
ingest. O mini player live/output reconecta nesses encerramentos com backoff
até 30 segundos, sem o teto de cinco minutos aplicado à prévia antiga sob demanda.
Fechar/pausar cancela a reconexão. HTTP 401/403 informado pelo motor é terminal.

Aceite sintético: mais de quatro quedas seguidas em HTTP/SRT, recuperação e
parada manual, consumidor com jitter superior à antiga fila de 42 KiB, limite de
buffer, player além de cinco minutos, reconexão cancelada ao fechar, bloqueio por
permissão, links autenticados e alternância lista/cards. Sem teste de origem real.
