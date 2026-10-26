# Kore Edge Web

React/TypeScript/Vite, sem CDN. Login animado baseado na referência: datacenter
local, roxo/magenta, formulário à direita e adaptação mobile. Animações respeitam
prefers-reduced-motion. PanelRoot consulta a sessão antes de carregar o dashboard.
Senhas e tokens não usam localStorage. Sessão expirada volta ao login. Usuários
administra contas e senhas com confirmação da senha atual. APIs operacionais em
leitura. Não há demonstração: ?demo=1 é ignorado. Fixtures são exclusivas de testes.
Métricas indisponíveis não recebem valores inventados.

Na raiz:

```sh
npm --prefix web ci
npm --prefix web run check
npm --prefix web test
npm --prefix web run build
go build ./cmd/edge-agent
```

Bundle/licenças incorporados em internal/webui/dist: reconstruir após mudar UI.
Node não é runtime do servidor. Para desenvolvimento, npm run dev -- --port 0 em
web. KORE_EDGE_API_URL configura explicitamente proxy de API local. Login funcional
exige HTTPS e Origin configurada; não enfraquecer cookies para testar HTTP. Nunca
expor Vite publicamente.

Vitest: métricas, ausência de demonstração, login, sessão e visibilidade da senha.
Build verifica tipos e gera bundle/licenças. Aviso use client do Lucide refere-se
a RSC; esta SPA não usa RSC. A imagem local foi gerada para o projeto a partir da
referência fornecida. Fontes do sistema e ícones SVG locais. React/React DOM MIT,
Lucide ISC; licenças copiadas pelo build; dependências de teste fora do bundle.
