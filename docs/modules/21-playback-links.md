# Saídas permanentes para XUI One

## Uso

1. Abra **Canais**, selecione o canal e entre em **Saídas**.
2. Os endereços MPEG-TS e HLS aparecem automaticamente. Copie para o XUI One.
3. **Lista TS/HLS** exporta os canais filtrados com os mesmos endereços.

Não há geração manual, prazo de expiração ou botão/API de revogação. Os endereços não mudam ao reiniciar o Edge, editar a origem ou alterar contas e senhas administrativas. Excluir o canal invalida sua identidade; uma nova identidade não reaproveita a chave excluída.

HLS sempre tem um endereço reservado, mas só fornece segmentos quando estiver configurado, autorizado, com codecs compatíveis e recurso disponível. A lista não é uma declaração de canais online. O receptor precisa confiar no HTTPS do Edge. Os limites de concorrência, memória e processos de mídia são mantidos.

## Armazenamento e proteção

GET `/api/v1/live/playback-links` lista os links para administradores com sessão válida. Nenhuma senha repetida ou operação de emissão é necessária. POST com `action: issue` apenas lista os mesmos endereços para compatibilidade com telas em cache; `revoke` é recusado. Não existe chave que permita administrar o Edge.

A chave do canal usa HMAC-SHA256 com domínio `permanent-channel-v1`, ID e geração de exclusão. A chave raiz e as gerações ficam em armazenamento privado fora do checkout; não se gravam URLs completas, credenciais da matriz ou do administrador em logs. Preserve esse armazenamento no backup e na atualização do servidor.

`GET|HEAD /api/v1/playback/p1.<chave>/<canal>/stream.ts|index.m3u8|seg*.ts` aceita apenas o canal correspondente e valida as permissões atuais da origem. Não depende de cookie, conta emissora ou relógio. Paths codificados, traversal, queries e origem web externa são recusados. Não abre outra origem nem muda rede, firewall ou permissões do host. Chaves de saída devem ser tratadas como acesso ao canal; não publique os endereços em logs/tickets.

## Continuidade

A saída TS acompanha a recuperação do ingest e troca de hub mantendo a conexão HTTP. Durante a lacuna, pacotes nulos mantêm apenas o transporte; não simulam vídeo ou bitrate recebido. Pausa manual e exclusão encerram a resposta. Shutdown, cliente lento/desconectado e perda da permissão upstream também a encerram. HLS preserva o endereço entre tentativas e continua sujeito à disponibilidade dos segmentos. Não é possível garantir vídeo sem cortes quando a origem/rede falha; os endereços não possuem um temporizador que interrompa a distribuição.

## Migração

Os links temporários da versão anterior continuam válidos conforme o prazo e as restrições originais. Use os novos endereços exibidos no painel ao configurar o XUI One; não é necessário gerar nem renovar os novos. Os caminhos internos `/api/v1/live/...` continuam exigindo a sessão do navegador.

## Aceite

Testes cobrem estabilidade após salto de 30 anos no relógio e restart, escopo, invalidação na exclusão e endereço diferente ao recriar; independência de revisão de senha; recebimento HTTPS sem cookie; uma conexão TS atravessando queda e recuperação da origem; parada manual; padding sem poluir métricas; carregamento automático das URLs e ausência de controles de gerar/revogar. O teste nativo CI valida playlist/segmentos HLS e H.264/AAC com dados sintéticos e chave permanente.
