# Kore Edge Web — Fase 09

React/TypeScript/Vite. O build é incorporado pelo agente Go, sem CDN em runtime.
Na raiz do repositório: `npm --prefix web ci`, `npm --prefix web run build`,
depois `go build -o bin/edge-agent ./cmd/edge-agent`. Os arquivos gerados em
`internal/webui/dist` são mantidos para permitir build/teste Go sem Node.
Sempre reconstruir o frontend antes de gerar um novo binário após editar UI.

O modo padrão consome somente GET das APIs locais. `?demo=1#dashboard` habilita
explicitamente a demonstração visual com aviso permanente e sem chamadas API.
Não importar a demonstração como catálogo nem tratar suas marcas/dados como
medidas do Edge. Não existem operações POST/PUT/DELETE na UI desta fase.

Desenvolvimento com Node 24 e npm 11 (lockfile versionado):

```powershell
npm ci
$env:KORE_EDGE_API_URL = 'http://127.0.0.1:18080' # endereço local já aprovado
npm run dev -- --port 0
```

Vite imprime a porta escolhida pelo SO para a prévia isolada; não há fallback
silencioso para servidor remoto. Sem `KORE_EDGE_API_URL`, não há proxy de API.
Usar o Local URL impresso com `/ui/?demo=1#dashboard` para avaliar a referência.
As ferramentas de desenvolvimento não devem ser expostas fora de loopback.

`npm run check`: TypeScript; `npm test`: dez testes Vitest de adaptação e DOM,
incluindo busca/filtro, diálogos, navegação por teclado, ausência de mutações e
separação da demonstração. `npm run build`: verificação de tipos + bundle +
licenças. Aviso `use client` do Lucide no bundler é relativo a RSC; este projeto
é uma SPA de cliente e não usa React Server Components.

Licenças runtime são copiadas de cada pacote para `dist/licenses` pelo script
de build: React/React DOM (MIT) e Lucide (ISC). Fontes e ícones são locais/SVG,
sem requisições a serviços de imagens, fontes ou analytics. Dependências de
teste não são incluídas no bundle. O player HLS da Fase 07 mantém sua licença
própria e continua separado desta interface.
