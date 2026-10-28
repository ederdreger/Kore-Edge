import {channelPath} from './media-channel-route';
import {MediaWorkspace} from './media-workspace';
import { useCallback, useEffect, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { Activity, ArrowDown, ArrowUp, ArrowUpCircle, Bell, ChartNoAxesCombined, Check, ChevronDown, ChevronRight, Clock3, CloudUpload, Cpu, Ellipsis, ExternalLink, FileText, HardDrive, House, Info, Menu, Monitor, Network, Play, Plus, Radio, RefreshCw, Search, Settings, ShieldCheck, Tv, Users, Waves, X, MemoryStick } from 'lucide-react';
import { adapt, filterChannels, gigabytes, rate, readAPI, uptime } from './data';
import { useAccount } from './account-context';
import { UserManager } from './users';
import {WireGuardWorkspace} from './wireguard-workspace';
import type { Channel, Diagnostics, Point, ViewData } from './types';
type Route = 'media' | 'dashboard' | 'wireguard' | 'system' | 'monitoring' | 'logs' | 'settings' | 'updates' | 'users';
const routeNames: Record<Route, string> = { media: 'Mídia', dashboard: 'Dashboard', wireguard: 'WireGuard', system: 'Sistema', monitoring: 'Monitoramento', logs: 'Logs', settings: 'Configurações', users: 'Usu\u00e1rios', updates: 'Atualizações' };
const route = (): Route => { const key = typeof window === 'undefined' ? '' : window.location.hash.slice(1).split('/')[0]; if(['channels','udp','hls'].includes(key))return 'media'; return key in routeNames ? key as Route : 'dashboard'; };
function useEdge() {
    const [data, setData] = useState<ViewData>(() => adapt({}));
    const [loading, setLoading] = useState(true);
    const previous = useRef<Diagnostics | undefined>(undefined);
    const history = useRef<Point[]>([]);
    const request = useRef<AbortController | null>(null);
    const refresh = useCallback(async () => {
        request.current?.abort();
        const control = new AbortController();
        request.current = control;
        setLoading(true);
        const timeout = setTimeout(() => control.abort(), 4000);
        try {
            const { raw, problems } = await readAPI(control.signal);
            if (request.current !== control)
                return;
            const next = adapt(raw, previous.current, [], problems);
            previous.current = raw.diagnostics;
            history.current = [...history.current, { at: Date.now(), rx: next.input, tx: next.output, cpu: next.cpu, memory: next.memoryUsed !== null && next.memoryTotal ? next.memoryUsed / next.memoryTotal * 100 : null }].slice(-360);
            next.history = history.current;
            setData(next);
        }
        finally {
            clearTimeout(timeout);
            if (request.current === control)
                setLoading(false);
        }
    }, []);
    useEffect(() => { void refresh(); const timer = setInterval(() => { if (!document.hidden)
        void refresh(); }, 5000); return () => { clearInterval(timer); request.current?.abort(); }; }, [refresh]);
    return { data, loading, refresh };
}
function Badge({ children, tone = 'green' }: {
    children: ReactNode;
    tone?: string;
}) { return <span className={`badge ${tone}`}><span className="dot"/>{children}</span>; }
function IconBox({ children, tone = 'cyan' }: {
    children: ReactNode;
    tone?: string;
}) { return <span className={`icon-box ${tone}`}>{children}</span>; }
function Spark({ values, color = 'var(--cyan)' }: {
    values: (number | null)[];
    color?: string;
}) {
    const clean = values.filter((v): v is number => v !== null);
    if (clean.length < 2)
        return <span className="spark-empty">—</span>;
    const max = Math.max(...clean, 1);
    const min = Math.min(...clean) * 0.85;
    const points = values.map((v, i) => v === null ? '' : `${i === 0 || values[i - 1] === null ? 'M' : 'L'}${i / (values.length - 1) * 130},${37 - (v - min) / (max - min || 1) * 29}`).join(' ');
    return <svg className="spark" viewBox="0 0 130 42" aria-hidden="true"><path fill="none" stroke={color} strokeWidth="1.6" d={points}/></svg>;
}
function Graph({ data, kind, minutes }: {
    data: ViewData;
    kind: 'network' | 'cpu';
    minutes: number;
}) {
    const points = data.history.filter(p => p.at >= Date.now() - minutes * 60000);
    const keys: ('rx' | 'tx' | 'cpu' | 'memory')[] = kind === 'network' ? ['rx', 'tx'] : ['cpu', 'memory'];
    const max = kind === 'network' ? Math.max(1e6, ...points.flatMap(p => [p.rx ?? 0, p.tx ?? 0])) * 1.15 : 100;
    const colors = kind === 'network' ? ['#00aaff', '#bf1bff'] : ['#00dbac', '#9a42ff'];
    const lines = keys.map(key => { let path = ''; points.forEach((p, i) => { const v = p[key]; if (v === null)
        return; path += `${i === 0 || points[i - 1][key] === null ? 'M' : 'L'}${i / Math.max(points.length - 1, 1) * 600},${90 - v / max * 85} `; }); return path; });
    const present = points.length > 1 && keys.some(k => points.filter(p => p[k] !== null).length > 1);
    return <div className="graph" role="img" aria-label={`${kind === 'network' ? 'Tráfego' : 'CPU e memória'}; ${present ? 'amostras locais desta sessão' : 'aguardando amostras'}`}>
  <div className="graph-axis">{(kind === 'cpu' ? ['100%', '75%', '50%', '25%', '0%'] : [rate(max), rate(max * .66), rate(max * .33), '0']).map(v => <span key={v}>{v}</span>)}</div>
  <div className="graph-body"><svg viewBox="0 0 600 100" preserveAspectRatio="none" aria-hidden="true"><defs>{colors.map((c, i) => <linearGradient key={c} id={`${kind}-fill-${i}`} x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stopColor={c} stopOpacity=".32"/><stop offset="100%" stopColor={c} stopOpacity=".01"/></linearGradient>)}</defs>{[0, 25, 50, 75, 100].map(y => <line key={`h${y}`} x1="0" x2="600" y1={y} y2={y} className="grid-line"/>)}{[0, 100, 200, 300, 400, 500, 600].map(x => <line key={`v${x}`} x1={x} x2={x} y1="0" y2="100" className="grid-line"/>)}{present && lines.map((path, i) => <g key={path}><path d={path} fill="none" stroke={colors[i]} strokeWidth="1.3"/>{path.length > 0 && points.every(p => p[keys[i]] !== null) && <path d={`${path} L600,100 L0,100 Z`} fill={`url(#${kind}-fill-${i})`}/>}</g>)}</svg>{!present && <span className="graph-placeholder">Aguardando amostras disponíveis</span>}<div className="graph-time">{Array.from({ length: 6 }, (_, i) => { const p = points[Math.floor(i / 5 * Math.max(points.length - 1, 0))]; return <span key={i}>{p ? new Date(p.at).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' }) : '—'}</span>; })}</div></div>
 </div>;
}
function Charts({ data }: {
    data: ViewData;
}) { const [minutes, setMinutes] = useState(30); return <div className="charts">{(['network', 'cpu'] as const).map(kind => <section className="panel chart-panel" key={kind}><div className="section-title"><h2>{kind === 'network' ? 'Tráfego de Ingest' : 'Uso de CPU e Memória'} <small>(últimos {minutes} minutos)</small></h2><select aria-label={`Janela do gráfico de ${kind === 'network' ? 'tráfego' : 'CPU'}`} value={minutes} onChange={e => setMinutes(Number(e.target.value))}><option value={30}>Últimos 30 minutos</option><option value={10}>Últimos 10 minutos</option></select></div><Graph data={data} kind={kind} minutes={minutes}/><div className="legend">{kind === 'network' ? <><span><i className="cyan-bg"/>Entrada ({rate(data.input)})</span><span><i className="purple-bg"/>Saída ({rate(data.output)})</span></> : <><span><i className="green-bg"/>CPU ({data.cpu === null ? '—' : `${data.cpu.toFixed(0)}%`})</span><span><i className="purple-bg"/>RAM ({data.memoryTotal && data.memoryUsed !== null ? `${(data.memoryUsed / data.memoryTotal * 100).toFixed(0)}%` : '—'})</span></>}</div></section>)}</div>; }
function NetworkRates({ data }: {
    data: ViewData;
}) { return <section className="panel content-panel"><h2>Tr?fego por interface</h2><p className="muted">Taxas da janela medida. NIC f?sica e VPN podem representar os mesmos bytes; n?o s?o somadas.</p>{data.networkRates.length ? data.networkRates.map(n => <div className="list-row" key={n.name}><strong>{n.name}</strong><span>RX {rate(n.rx)}</span><span>TX {rate(n.tx)}</span><small>{'Kernel ? janela do monitoramento'}</small></div>) : <p className="empty-small">Nenhuma taxa de interface dispon?vel.</p>}</section>; }
export function App() {
    const account = useAccount();
    const { data, loading, refresh } = useEdge();
    const [page, setPage] = useState<Route>(route);
    const [search, setSearch] = useState('');
    const [filter, setFilter] = useState('all');
    const [category, setCategory] = useState('all');
    const [tab, setTab] = useState('channels');
    const [selected, setSelected] = useState<string[]>([]);
    const [mobile, setMobile] = useState(false);
    const [sort, setSort] = useState<'default' | 'name' | 'bitrate'>('default');
    const [modal, setModal] = useState<{
        title: string;
        body: ReactNode;
    } | null>(null);
    const dialog = useRef<HTMLDialogElement>(null);
    const searchInput = useRef<HTMLInputElement>(null);
    useEffect(() => { const onHash = () => { if(['channels','udp','hls'].includes(window.location.hash.slice(1).split('/')[0]))window.history.replaceState(null,'','#media');setPage(route()); setMobile(false); }; onHash();window.addEventListener('hashchange', onHash); const keyboard = (e: KeyboardEvent) => { if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        searchInput.current?.focus();
    } }; window.addEventListener('keydown', keyboard); return () => { window.removeEventListener('hashchange', onHash); window.removeEventListener('keydown', keyboard); }; }, []);
    useEffect(() => { if (modal)
        dialog.current?.showModal();
    else
        dialog.current?.close(); }, [modal]);
    const info = (title: string, body: ReactNode) => setModal({ title, body });
    const nav = (id: Route, Icon: typeof House) => <a href={`#${id}`} className={`nav-item ${page === id ? 'active' : ''}`} aria-current={page === id ? 'page' : undefined}><Icon size={20}/><span>{routeNames[id]}</span>{['system', 'monitoring'].includes(id) && <ChevronDown size={13} className="nav-end"/>}</a>;
    const rows = filterChannels(data.channels, search, filter, category).sort((a, b) => sort === 'name' ? a.name.localeCompare(b.name) : sort === 'bitrate' ? (b.bitrate ?? -1) - (a.bitrate ?? -1) : 0);
    const detail = (c: Channel) => {window.location.hash=channelPath(c.id)};
    const noMutation = (title: string) => info(title, <><p>Este painel é somente leitura. Nenhum comando operacional será executado.</p><p>Esta função depende de uma etapa operacional posterior ou de dados ainda indisponíveis.</p></>);
    const onlineText = data.connected ? 'Online' : 'API indisponível';
    const wirePanel = <section className="panel wire-panel"><IconBox tone="green"><ShieldCheck size={29}/></IconBox><div><span className="muted">Estado do túnel WireGuard</span><strong className={'muted'}><span className="diamond">◆</span> {data.wireguard.state}</strong><small>{data.wireguard.detail}</small></div><a href="#wireguard" aria-label="Ver WireGuard"><ChevronRight size={18}/></a></section>;
    const serverPanel = <section className="panel server-panel"><div className="section-title"><h2>Status do Servidor</h2><Badge tone={data.connected ? 'green' : 'gray'}>{onlineText}</Badge></div>{[
            { name: 'CPU', Icon: Cpu, tone: 'cyan', value: data.cpu === null ? '—' : `${data.cpu.toFixed(0)}%`, series: data.history.map(p => p.cpu), color: 'var(--green)' },
            { name: 'RAM', Icon: MemoryStick, tone: 'purple', value: data.memoryUsed === null ? '—' : `${gigabytes(data.memoryUsed)} / ${gigabytes(data.memoryTotal)} GB`, series: data.history.map(p => p.memory), color: 'var(--purple)' },
            { name: 'Disco', Icon: HardDrive, tone: 'cyan', value: data.diskUsed === null ? '—' : `${gigabytes(data.diskUsed)} / ${gigabytes(data.diskTotal)} GB`, series: [], color: '#0aa7ff' },
            { name: 'Rede', Icon: Network, tone: 'green', value: 'Por interface', series: [], color: '#f008a3' },
        ].map(({ name, Icon, tone, value, series, color }) => <div className="server-row" key={name}><IconBox tone={tone}><Icon size={21}/></IconBox><div><span>{name}</span><strong>{value}</strong>{name === 'Rede' && false && <strong className="network-small">680 Mbps <ArrowUp size={12}/></strong>}</div><Spark values={series} color={color}/></div>)}</section>;
    const interfacesPanel = <section className="panel interfaces-panel"><h2>Interfaces de Rede</h2>{data.interfaces.length ? data.interfaces.slice(0, 4).map(nic => <div className="interface-row" key={nic.name}><strong>{nic.name}</strong><span title={nic.ipv4.join(', ')}>{nic.ipv4[0] ?? 'Sem IPv4'}</span><Badge tone={nic.up ? 'green' : 'gray'}>{nic.up ? 'Up' : 'Down'}</Badge></div>) : <p className="empty-small">Inventário indisponível.</p>}</section>;
    const devicesPanel = <section className="panel devices-panel"><h2>Dispositivos Conectados <small>(TV Box / Apps)</small></h2><div><strong>{data.clients ?? '—'}</strong><span className="muted">{null}</span><button className="button compact" onClick={() => info('Dispositivos conectados', <p>{'Contagem e inventário de espectadores ainda não são medidos pelo agente.'}</p>)}>Ver todos</button></div></section>;
    const channelTable = <section className="panel table-panel"><div className="table-top"><div className="tabs" role="tablist" aria-label="Dados dos canais">{[['channels', `Canais (${data.total})`], ['outputs', 'Saídas'], ['quality', 'Qualidade'], ['events', 'Eventos']].map(([id, label]) => <button key={id} role="tab" aria-selected={tab === id} className={tab === id ? 'chosen' : ''} onClick={() => setTab(id)}>{label}</button>)}</div><div className="table-actions"><button className="button primary" disabled={account?.user?.role!=='admin'} onClick={() => {setPage('media');window.location.hash='media/new';}}><Plus size={17}/>Adicionar Canal</button><button className="button" onClick={() => info('Ações em lote', <p>{selected.length ? `${selected.length} canal(is) selecionado(s).` : 'Selecione canais na tabela.'} Não há alterações operacionais disponíveis neste painel.</p>)}><ExternalLink size={13}/>Ações em lote<ChevronDown size={12}/></button></div></div>
  {tab === 'channels' ? <><div className="filters"><label className="search-field"><Search size={16}/><input aria-label="Buscar canal" placeholder="Buscar canal..." value={search} onChange={e => setSearch(e.target.value)}/></label><select aria-label="Filtrar por status" value={filter} onChange={e => setFilter(e.target.value)}><option value="all">Todos os status</option><option value="online">Online</option><option value="attention">Atenção</option><option value="stopped">Parado</option><option value="unavailable">Sem medi??o</option></select><select aria-label="Filtrar por categoria" value={category} onChange={e => setCategory(e.target.value)}><option value="all">Todas as categorias</option>{[...new Set(data.channels.map(c => c.category))].map(c => <option key={c} value={c}>{c}</option>)}</select></div>
   <div className="table-scroll"><table><thead><tr><th><input type="checkbox" aria-label="Selecionar todos os canais filtrados" checked={rows.length > 0 && rows.every(c => selected.includes(c.id))} onChange={e => setSelected(e.target.checked ? rows.map(c => c.id) : [])}/></th><th>#</th><th><button onClick={() => setSort(sort === 'name' ? 'default' : 'name')}>Canal {sort === 'name' ? '↑' : ''}</button></th><th>Categoria <ChevronDown size={10}/></th><th>Entrada <ChevronDown size={10}/></th><th>Saídas <ChevronDown size={10}/></th><th><button onClick={() => setSort(sort === 'bitrate' ? 'default' : 'bitrate')}>Bitrate <ChevronDown size={10}/></button></th><th>Status</th><th>Qualidade</th><th className="align-right">Ações</th></tr></thead><tbody>{rows.map((c, i) => <tr key={c.id}><td><input type="checkbox" aria-label={`Selecionar ${c.name}`} checked={selected.includes(c.id)} onChange={e => setSelected(e.target.checked ? [...selected, c.id] : selected.filter(id => id !== c.id))}/></td><td>{i + 1}</td><td><button className="channel-name" onClick={() => detail(c)}><span className="channel-logo" aria-hidden="true"><Tv size={18}/></span>{c.name}</button></td><td><span className={`category category-${c.category === 'ESPORTES' ? 'sport' : c.category === 'KIDS' ? 'kids' : c.category === 'DOCUMENTÁRIOS' ? 'docs' : 'open'}`}>{c.category}</span></td><td className="source-cell" title={c.source}>{c.synthetic ? 'Fonte sintética' : c.source}</td><td><span className="output-tags">{c.outputs.map(o => <span key={o} className={`output-tag ${o}`}>{o.toUpperCase()}</span>)}</span></td><td className="bitrate-cell">{rate(c.bitrate)}</td><td><Badge tone={c.status === 'online' ? 'green' : c.status === 'attention' ? 'gold' : 'gray'}>{c.status === 'online' ? 'Online' : c.status === 'attention' ? 'Atenção' : 'Parado'}</Badge></td><td><button className="quality-action" title={c.quality} aria-label={`Qualidade de ${c.name}`} onClick={() => detail(c)}>{<Activity size={16}/>}</button></td><td><div className="row-actions"><a className="icon-button" href={channelPath(c.id)} aria-label={`Reproduzir ${c.name}`}><Play size={14}/></a><button className="icon-button" aria-label={`Monitorar ${c.name}`} onClick={() => { window.location.hash = 'monitoring'; }}><ChartNoAxesCombined size={15}/></button><button className="icon-button" aria-label={`Detalhes de ${c.name}`} onClick={() => detail(c)}><Settings size={15}/></button><button className="icon-button" aria-label={`Mais informações sobre ${c.name}`} onClick={() => detail(c)}><Ellipsis size={16}/></button></div></td></tr>)}</tbody></table>{!rows.length && <div className="empty-state"><Radio size={26}/><strong>{data.channels.length ? 'Nenhum canal corresponde aos filtros' : 'Nenhum catálogo disponível'}</strong><p>{data.channels.length ? 'Altere a busca ou os filtros.' : 'Nenhum canal cadastrado nesta instalação.'}</p></div>}</div></> : <div className="tab-content">{tab === 'outputs' ? <><h3>Permissões de saída</h3><p>As etiquetas UDP e HLS indicam autorização no catálogo. A entrega ao receptor precisa de validação própria.</p>{data.channels.map(c => <div className="list-row" key={c.id}><strong>{c.name}</strong><span>{c.outputs.join(' · ').toUpperCase()}</span></div>)}</> : tab === 'quality' ? <><h3>Qualidade do transporte</h3><p>CC, TEI e PCR são apresentados pelo monitoramento quando medidos. Precisão de PCR e qualidade visual não são inferidas.</p>{data.channels.map(c => <div className="list-row" key={c.id}><strong>{c.name}</strong><span>{c.quality}</span></div>)}</> : <><h3>Eventos do monitoramento</h3>{data.alerts.length ? data.alerts.map(a => <div className="list-row" key={a.id}><Badge tone="gold">{a.state}</Badge><span>{a.code} · {a.channel_id}</span></div>) : <p>Nenhum alerta ativo observado.</p>}</>}</div>}
 </section>;
    const dashboard = <><div className="welcome-grid"><section className="panel welcome"><IconBox tone="purple"><CloudUpload size={32}/></IconBox><div className="welcome-copy"><h1>Bem-vindo ao Kore Edge</h1><p>Distribuição de canais da matriz para parceiros via WireGuard</p></div><div className="sync-status"><span className="status-orb"/><div><strong>{'Catálogo local do Edge'}</strong><small>{data.catalogAvailable ? 'Catálogo disponível · sem conexão com matriz' : 'Aguardando catálogo assinado'}</small></div></div><button className="button primary" onClick={() => void refresh()} disabled={loading}>{loading ? <RefreshCw size={15} className="spin"/> : null}{'Atualizar dados'}</button></section>{wirePanel}</div>
  <div className="stat-grid">{[
            { label: 'Canais Ativos', value: data.catalogAvailable ? `${data.active} / ${data.total}` : '—', Icon: Tv, tone: 'cyan' },
            { label: 'Tráfego de Entrada', value: rate(data.input), Icon: Network, tone: 'blue' },
            { label: 'Tráfego de Saída', value: rate(data.output), Icon: ArrowUpCircle, tone: 'magenta' },
            { label: 'Clientes Conectados', value: data.clients ?? '—', Icon: Users, tone: 'gold' },
            { label: 'Uptime do agente', value: uptime(data.uptime), Icon: Clock3, tone: 'green' },
        ].map(({ label, value, Icon, tone }, i) => <section className={`stat-card ${tone}`} key={label}><IconBox tone={tone}><Icon size={28}/></IconBox><div><span>{label}</span><strong>{value}</strong></div>{i === 0 ? null : null}</section>)}</div>
  <div className="main-grid"><div className="center-column">{channelTable}<Charts data={data}/></div><aside className="right-column" aria-label="Status do Edge">{serverPanel}{interfacesPanel}{devicesPanel}</aside></div>
 </>;
    const extraPage = <>{!['wireguard','media'].includes(page)&&<div className="page-heading"><div><span className="eyebrow">KORE EDGE / {'EDGE'}</span><h1>{routeNames[page]}</h1></div><button className="button" onClick={() => void refresh()}><RefreshCw size={15}/>Atualizar dados</button></div>}{page === 'media' ? <MediaWorkspace version={data.edge.version} uptimeSeconds={data.uptime??undefined}/> : page === 'monitoring' ? <><Charts data={data}/><NetworkRates data={data}/><section className="panel content-panel"><h2>Alertas locais</h2>{data.alerts.length ? data.alerts.map(a => <div className="list-row" key={a.id}><Badge tone="gold">{a.state}</Badge><span>{a.code}</span><small>{a.channel_id}</small></div>) : <p className="muted">Nenhum alerta ativo observado. Indisponibilidade de dados não é ausência comprovada de falhas.</p>}</section></> : page === 'system' ? <><div className="edge-grid">{serverPanel}{interfacesPanel}<section className="panel content-panel"><h2>Informações do Edge</h2><dl className="details"><dt>Sistema</dt><dd>{data.edge.os}</dd><dt>Arquitetura</dt><dd>{data.edge.arch}</dd><dt>Versão</dt><dd>{data.edge.version}</dd><dt>Uptime da API</dt><dd>{uptime(data.uptime)}</dd><dt>Identidade</dt><dd>{'Não provisionada'}</dd></dl></section></div><section className="panel content-panel"><h2>Condições de leitura</h2><p>CPU requer duas amostras válidas. Métricas Linux podem estar indisponíveis no Windows. Interfaces são exibidas conforme inventário, sem assumir papéis de gestão, IPTV ou VPN.</p></section></> : page === 'wireguard' ? <WireGuardWorkspace rates={data.networkRates} observed={data.updated}/> : page === 'logs' ? <section className="panel content-panel"><h2>Eventos disponíveis</h2><p className="muted">Somente alertas locais do monitoramento. Logs de processos e dados de clientes não são coletados pelo painel.</p>{data.alerts.map(a => <div className="list-row" key={a.id}><span>{a.code}</span><span>{a.channel_id}</span><Badge tone="gold">{a.state}</Badge></div>)}</section> : <section className="panel content-panel"><h2>{page === 'updates' ? 'Versão do agente' : 'Configuração em leitura'}</h2><dl className="details"><dt>Versão</dt><dd>{data.edge.version}</dd><dt>Stream Engine</dt><dd>{data.edge.stream ? 'Habilitado' : 'Desabilitado'}</dd><dt>Catálogo</dt><dd>{data.edge.catalog ? 'Confiança configurada' : 'Não configurado'}</dd><dt>HLS</dt><dd>{data.edge.hls ? 'Habilitado' : 'Desabilitado'}</dd></dl><p className="muted">{page === 'updates' ? 'Atualização automática não implementada. Nenhum binário será alterado.' : 'Alterações de configuração e ações operacionais seguem revisão fora deste painel.'}</p></section>}</>;
    return <div className="app-shell"><a className="skip-link" href="#main" onClick={e => { e.preventDefault(); document.getElementById('main')?.focus(); }}>Pular para o conteúdo</a><aside className={`sidebar ${mobile ? 'is-open' : ''}`}><a href="#dashboard" className="brand"><span className="brand-symbol"><Play size={27}/></span><span><strong>Kore Edge</strong><small>IPTV Distribution</small></span></a><nav aria-label="Navegação principal">{nav('dashboard', House)}{nav('media', Radio)}{nav('wireguard', ShieldCheck)}{nav('system', Settings)}{nav('monitoring', Activity)}{nav('logs', FileText)}{nav('users', Users)}{nav('settings', Settings)}{nav('updates', RefreshCw)}</nav><div className="sidebar-footer"><strong><span className="dot"/> Kore Edge {data.edge.version}</strong><span>{data.edge.os}</span><span>{'Instalação local'}</span><span>Uptime: {uptime(data.uptime)}</span></div></aside>{mobile && <button className="sidebar-backdrop" aria-label="Fechar menu" onClick={() => setMobile(false)}/>}
 <div className="workspace"><header className="topbar"><button className="icon-button mobile-toggle" aria-label="Abrir menu" onClick={() => setMobile(true)}><Menu size={22}/></button><label className="global-search"><Search size={19}/><input ref={searchInput} aria-label="Busca global de canais" placeholder="Buscar canal, URL, IP ou configuração..." value={search} onChange={e => { setSearch(e.target.value); if (page !== 'dashboard' && page !== 'media')
        window.location.hash = 'media'; }}/><kbd>Ctrl K</kbd></label><div className="top-actions"><button className="notification" aria-label={`Alertas: ${data.alerts.length}`} onClick={() => { setPage('monitoring'); window.location.hash = 'monitoring'; }}><Bell size={19}/>{data.alerts.length > 0 && <i />}</button><button className="profile" onClick={() => { window.location.hash = 'users'; }}><span className="avatar">{account?.user?.username.slice(0, 1).toUpperCase() ?? 'A'}</span><strong>{account?.user?.username ?? 'Conta'}</strong></button><button className="button" onClick={() => void account?.logout()}>Sair</button><Badge tone={data.connected ? 'green' : 'gray'}>{onlineText}</Badge></div></header>
 <main id="main" className={page==='media'?'media-mode':undefined} tabIndex={-1}><div className={`provenance ${''}`} role="status"><span>{<><span className="dot"/>{page==='media'?'MÍDIA · TRANSMISSÕES E FONTES':'MÉTRICAS DO EDGE · LEITURA'}</>}</span><small>{data.updated ? `Amostra: ${new Date(data.updated).toLocaleTimeString('pt-BR')}` : 'Aguardando dados disponíveis'}{data.problems.length > 0 && ` · ${data.problems.length} fontes indisponíveis`}</small></div>{page === 'users' ? <UserManager /> : page === 'dashboard' ? dashboard : extraPage}</main>
 </div><dialog ref={dialog} onCancel={() => setModal(null)} onClose={() => setModal(null)} aria-labelledby="dialog-title"><div className="dialog-head"><h2 id="dialog-title">{modal?.title}</h2><button className="icon-button" aria-label="Fechar diálogo" onClick={() => setModal(null)}><X size={19}/></button></div><div className="dialog-body">{modal?.body}</div><div className="dialog-footer"><button className="button primary" onClick={() => setModal(null)}><Check size={15}/>Entendi</button></div></dialog></div>;
}
