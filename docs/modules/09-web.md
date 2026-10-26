# Fase 09 — Painel web

## Entrega e referência visual

Frontend em `web/`, React 19/TypeScript/Vite, ícones Lucide e CSS local.
`internal/webui` incorpora o build no binário do agente. Nenhuma hospedagem
externa, CDN de runtime, fonte remota, analytics ou alteração operacional.
O novo endpoint GET/HEAD `/api/v1/edge` fornece metadados reais do agente:
runtime OS/arquitetura, versão `dev`, início/uptime da API e recursos habilitados.
Não representa uptime do Ubuntu nem uma sessão administrativa autenticada.

A imagem fornecida em 2026-10-25 orienta a composição: sidebar escura roxa,
busca/perfil/indicador no topo, welcome e WireGuard, cinco cards, tabela central,
status/interfaces/saídas/dispositivos à direita e gráficos abaixo. Cores,
proporções, espaçamento, gradientes e densidade seguem essa referência.
Marcas dos canais são aproximações CSS/texto na demonstração, não arquivos
oficiais de logotipos. Não houve comparação pixel a pixel em navegador conectado:
esse aceite visual permanece pendente de revisão humana no link local.

## Modo Edge e demonstração

O modo padrão lê apenas GET de health, Edge, catálogo, streams, monitoramento,
diagnóstico, inventário de interfaces e WireGuard. Requisições têm timeout de
quatro segundos; atualização a cada cinco segundos, pausada com página oculta,
cancelada no unmount e sem loops de requisições sobrepostas.

`/?demo=1#dashboard` é opt-in e exibe aviso permanente **DEMONSTRAÇÃO · DADOS
SIMULADOS**. Usa oito linhas ilustrativas e indicadores da referência; total
26/27 e 143 clientes são somente exemplos. Esse modo não chama APIs, não faz
sincronização de matriz e não abre grupos/IPs ilustrados. O link “Dados do Edge”
retorna ao modo real, sem persistir a opção nem dados em localStorage.

No modo real:

- Catálogo ausente tem estado vazio e convite explícito para abrir a prévia;
  nunca é preenchido com canais fictícios. Canal sem observação não vira
  “Parado”: é “Sem medição”; parada exige estado conhecido do engine.
- Entrada é soma dos bitrates de ingest frescos, não taxa de NIC física.
  Saída total e clientes conectados ficam `—` porque não há medição equivalente.
- CPU exige duas leituras Linux monotônicas em até 15 s. Memória/disco vêm de
  bytes do diagnóstico; indisponibilidade no Windows não recebe fallback falso.
- Gráficos reais acumulam até 360 amostras de cinco segundos somente enquanto
  a página está aberta (~30 min). Sem dados há placeholder, não curva simulada;
  lacunas não são interpoladas. Refresh/reload perde esse histórico de sessão.
- Taxas RX/TX são apresentadas individualmente na página Monitoramento, sem
  somar NIC física/VPN. Nomes/papéis de rede não são assumidos.
- Handshake é apresentado como observação histórica, sem afirmar conexão atual.
  Labels de permissões UDP/HLS não significam entrega confirmada ao receptor.
- “Online” significa resposta do health da API, não integridade dos streams.
  Perfil real é “Operador local”; não afirma autenticação/RBAC de Fase 11.

## Fluxos e acessibilidade

Dashboard, Canais, Saídas UDP/HLS, WireGuard, Sistema/Edge e Monitoramento têm
consultas/navegação funcionais. Logs mostra os alertas disponíveis, sem alegar
acesso a stdout/stderr/auditoria. Configuração e Atualizações mostram leitura
de recursos/versão, sem editar arquivos, aplicar updates ou provisionar Edge.

Busca global/local por nome/ID/origem, filtros combinados, ordenação por nome ou
bitrate, seleção de linhas, abas de saídas/qualidade/eventos e detalhes em diálogo.
Controles “Adicionar canal”/“Ações em lote” explicam o fluxo de catálogo assinado
e a restrição de leitura; não fingem cadastrar/reiniciar canais. “Atualizar dados”
real faz somente GET. Botão de reprodução abre o player da Fase 07 somente para
canal online e autorizado HLS; não inicia ingest automaticamente. Demonstração
não reproduz seus canais comerciais ilustrados.

Links e botões semânticos, labels de inputs, `aria-current`, tablist e estados,
atalho Ctrl/Cmd+K, skip-link com foco no conteúdo, diálogo modal nativo com Escape,
foco visível, suporte a movimento reduzido e menu móvel. Layout tem breakpoints
para desktop/tablet/móvel; tabela mantém scroll horizontal quando necessário.
A aparência responsiva e leitura com screen reader ainda exigem homologação
em navegador real; testes DOM não substituem inspeção visual.

## Build e execução

Node 24/npm 11 foram usados nesta entrega; lockfile versionado. Go continua
aceitando build direto a partir dos arquivos estáticos versionados em
`internal/webui/dist`. Ao alterar frontend, gerar build antes de compilar Go:

```sh
npm --prefix web ci
npm --prefix web run check
npm --prefix web test
npm --prefix web run build
go test ./...
go vet ./...
go build -o bin/edge-agent-phase09 ./cmd/edge-agent
```

Windows gera `.exe` no destino indicado. `make web-build` e `make web-test`
fornecem os fluxos de frontend. O build copia licenças React/React DOM/Lucide
para `dist/licenses`. Dependências de testes não entram no bundle.

Servidor integrado, depois de liberar/confirmar a porta configurada:

```powershell
.\bin\edge-agent-phase09.exe -config configs/edge.example.yaml
```

Abrir `http://127.0.0.1:18080/` (dados do agente) ou
`http://127.0.0.1:18080/?demo=1#dashboard` (referência ilustrativa), usando o
endereço já aprovado. O processo antigo na mesma porta precisa ser encerrado
pelo operador antes de executar o novo binário; não executar instâncias
concorrentes no mesmo endereço. Não houve autorização para tocar serviços externos.

Prévia frontend isolada usada nesta entrega:

```powershell
$env:KORE_EDGE_API_URL = 'http://127.0.0.1:18080'
npm --prefix web run dev -- --port 0
```

Porta de teste escolhida pelo SO e impressa pelo Vite; não assume porta de
produção. O endereço desta sessão é `http://127.0.0.1:61130/ui/?demo=1#dashboard`.
A prévia foi mantida para revisão local solicitada anteriormente. No modo Edge,
se o proxy aponta para binário antigo, APIs novas ficarão indisponíveis; a
demonstração permanece independente. Não é servidor de implantação.

## Segurança e rollback

A UI não contém chamadas POST/PUT/DELETE, URLs externas operacionais ou segredos.
Não altera WireGuard, firewall, NIC, rotas ou catálogo. Strings de API são
renderizadas por React sem HTML arbitrário. CSP limita scripts/conexões à
origem; estilos inline são permitidos para dimensões SVG/estilos React.
O Go serve somente index, assets e licenças incorporados, sem listar diretórios,
expor o workspace ou transformar caminhos `/api` desconhecidos em HTML.
Index é `no-store`; assets com hash têm cache imutável.

Pré-verificação de troca local: confirmar dono/binário do processo, configuração
e porta; preservar executável anterior. Após troca, validar health/ready, raiz e
assets. Rollback: encerrar somente o novo agente e executar o anterior com o
arquivo de configuração preservado. Não há migração de dados da UI a reverter.

Nesta sessão a tentativa de encerrar o processo local antigo para a troca foi
rejeitada pela política automática de execução (`blocked by policy`). Nenhuma
substituição ocorreu na porta 18080. O processo anterior permanece intacto;
o binário novo e a prévia isolada permitem revisar a entrega sem contornar o bloqueio.

## Validação — 2026-10-25

- TypeScript, nove testes Vitest de adaptação/DOM e build frontend passaram.
- Testes incluem busca/filtros, diálogo, navegação, foco/atalho e ausência de
  consultas no demo ou mutações no modo Edge; “Sem medição” difere de “Parado”.
- Auditoria npm de dependências runtime: zero vulnerabilidades reportadas.
- Go: testes do servidor estático, metadata e regressão completa, vet e builds
  Windows/Linux amd64 passaram, assim como a compilação do teste webui Linux.
- Browser QA não disponível: inventário não retornou navegador; tentativa de
  abrir a prévia no IAB retornou `Browser is not available: iab`. Não há
  screenshot/alegação de fidelidade pixel-perfect nem teste real de responsividade.
- `go test -race ./...` foi tentado e bloqueado por ausência de CGO/compilador C. Nenhuma
  homologação nativa no Ubuntu ou deployment Nginx foi executada nesta fase.
- Pasta continua sem `.git`, impedindo criar branch.

## Objetivo
React/TypeScript, dashboard e páginas de canais/Edge.

## Pré-requisitos
Fase anterior aprovada, `AGENTS.md` lido, laboratório isolado.

## Entregáveis
- Implementação modular e documentada.
- Testes automatizados e exemplos de configuração sem segredos.
- Procedimento de validação e rollback, quando aplicável.

## Critério de aceite
Navegação acessível; sem ações perigosas automáticas.

## Prompt para Codex
Leia `AGENTS.md`, `README.md`, `docs/ARCHITECTURE.md` e este documento. Desenvolva somente a Fase 09 (Painel web): React/TypeScript, dashboard e páginas de canais/Edge. Antes de editar, apresente um plano, dependências, testes e riscos. Não execute mudanças privilegiadas no host. Entregue código, testes e documentação. Demonstre como validar: Navegação acessível; sem ações perigosas automáticas. Ao terminar, pare e solicite aprovação para a próxima fase.
