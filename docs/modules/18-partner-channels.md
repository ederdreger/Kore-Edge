# Catálogo e mídia por parceiro Kore Edge

## Escopo

Implementação separada em `internal/edgecatalog`, API em `internal/api/edgecatalog.go` e tela `web/src/kore-play-channels.tsx`. A matriz implementa `server/edgePartners.js`, `server/edgePartnerMedia.js` e a página `EdgePartners.jsx`. Não altera contratos do catálogo/Stream Engine sintético, rede, firewall ou o auxiliar privilegiado WireGuard.

O operador publica no Kore Play um parceiro vinculado a um único peer WireGuard existente, canais e protocolo efetivamente cadastrado na origem. O Edge consulta uma lista filtrada; não recebe URLs nem credenciais das origens. [Passo a passo](../CANAIS-KORE-PLAY.md).

## Contrato e autorização

- Matriz: administração `GET/POST /api/admin/kore-edge/partners`, restrita a administrador; publicação exige senha atual e revisão global esperada.
- Comunicação Edge/matriz: `GET /api/kore-edge/catalog`, `/probe/:channel/:protocol` e `/media/:channel/:protocol/*tail`.
- Edge: sessão HTTPS para `GET /api/v1/kore-play/catalog` e mídia; configuração/sincronização exigem administrador, CSRF e senha atual. O teste curto exige administrador e CSRF.
- Identidade X25519 já existente no WireGuard; nenhuma chave privada é exportada. HMAC-SHA256 autentica cada GET com domínio `kore-edge/distribution/request/v1`, método, caminho, timestamp e nonce de 16 bytes. Janela de relógio de 30 segundos e nonces retidos por 65 segundos impedem replay. Catálogo e resultado do teste retornam envelope autenticado com domínio `kore-edge/distribution/reply/v1` e o nonce da solicitação.
- HTTPS verifica certificado/hostname. O dial usa somente o IP privado confirmado dentro das AllowedIPs, com bind ao endereço do Edge no túnel; não usa proxy de ambiente nem redirecionamento.
- Manifesto v1: parceiro, revisão, emissão, validade de até cinco minutos, canais e protocolos autorizados. Rejeita assinatura/esquema inválido e regressão de revisão. Uma resposta inválida preserva o catálogo ainda válido; 401/403 revoga o cache. Catálogo vencido bloqueia mídia.
- Identidades Edge/matriz ficam fixadas na configuração; trocar as chaves exige nova confirmação administrativa da central.

## Persistência e recursos

Estado do Edge em `accounts/kore-play/catalog.json`, privado e fora do checkout, com gravação atômica. Matriz usa `edge-partners/partners.json` no diretório de dados privado, arquivo 0600 e diretório 0700, revisão, auditoria limitada e segredo AES-GCM para referências HLS. Nenhum segredo deve entrar no Git ou logs.

Limites: 128 parceiros, 512 canais por parceiro, 1–128 conexões por parceiro (padrão da tela 32), teto global de 256 na matriz e 64 no Edge. Testes simultâneos: um por parceiro/oito na matriz, dois no Edge. Playlists até 1 MiB/1024 referências. Controle tem deadline de três segundos; teste da origem até 2,5 segundos. Atualização opcional a cada minuto, sem loop imediato de tentativas.

Mídia mantém backpressure, limite de conexões e timeouts; não transcodifica. Cada nova solicitação revalida o grant na matriz e o relay em andamento revalida a cada três segundos (inventário de canais com cache de até um segundo). O Edge também encerra ao expirar a autorização. Uma sessão HTTP contínua do Edge tem duração máxima de uma hora; o consumidor deve reconectar.

HLS reescreve playlists/URI e usa referências cifradas vinculadas a parceiro/canal/versão da origem. As referências não substituem autorização: exigem nova prova autenticada em cada pedido. Mudança da origem ou revogação bloqueia referências anteriores. Somente referências HTTP/HTTPS da mesma origem são aceitas; redirects/CDNs externos não são suportados. URLs HLS assinadas já configuradas na matriz são resolvidas exclusivamente no servidor; não há consulta Flussonic durante desenvolvimento/testes.

## Aceite e validação

Testes sintéticos verificam publicação, revisão concorrente, autenticação, replay, canais não concedidos, revogação de relay em andamento, HLS sem vazamento de credenciais, escopo de referências, filtros de endereços, persistência, validade e preservação de catálogo. Testes da tela verificam busca explícita, resultado amostral e leitura sem ações administrativas para viewer.

Executar `go vet ./...`, `go test ./...`, `go test -race ./...` em Linux; `npm --prefix web test`, `npm --prefix web run check`, `npm --prefix web run build`. Na matriz: `npm test`, lint das páginas de integração e `npm run build`. Usar apenas fontes sintéticas nestas verificações.

Teste operacional posterior depende do operador: publicar um canal piloto, buscar no Edge, testar sinal e validar reprodução prolongada, codecs e capacidade. Não homologa automaticamente produção, multicast, UDP ingest, DRM ou recuperação de pacotes.

## Atualização e rollback

Pré-verificação: conferir identidade dos dois hosts de teste, revisão instalada, estado do túnel em leitura e backups. Use o atualizador existente do painel Edge (`scripts/update-panel.sh --plan` antes de `--apply`) e o fluxo de deploy versionado do Kore Play; não executar instalador completo sobre instalação existente. Nenhuma mudança de rede é necessária.

Após atualização, validar health/readiness, login/assets e rejeição de acesso anônimo às novas APIs. Não criar grants nem iniciar mídia automaticamente. Rollback restaura as releases e configurações dos backups do atualizador; arquivos novos de catálogo/permissões podem permanecer privados e inativos na versão anterior. Preservar contas e identidades WireGuard.
