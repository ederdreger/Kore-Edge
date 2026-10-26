import type { Channel, Diagnostics, Point, Raw, ViewData } from './types';

export const API_PATHS = ['/healthz', '/api/v1/edge', '/api/v1/catalog', '/api/v1/streams', '/api/v1/monitoring', '/api/v1/diagnostics', '/api/v1/multicast/interfaces', '/api/v1/wireguard', '/api/v1/session'] as const;
const keys = ['health', 'edge', 'catalog', 'streams', 'monitoring', 'diagnostics', 'interfaces', 'wireguard', 'session'] as const;
export async function readAPI(signal: AbortSignal): Promise<{ raw: Raw; problems: string[] }> {
  const raw: Raw = {}; const problems: string[] = [];
  await Promise.all(API_PATHS.map(async (path, i) => {
    try {
      const response = await fetch(path, { method: 'GET', signal, cache: 'no-store', credentials: 'same-origin' });
      if (response.status===401) window.dispatchEvent(new Event('kore:unauthorized'));
      if (!response.ok) { problems.push(`${keys[i]}: HTTP ${response.status}`); return; }
      const value: unknown = await response.json();
      if (!value || typeof value !== 'object') throw new Error('invalid response');
      const obj=value as Record<string,unknown>;
      if (keys[i]==='streams'&&!Array.isArray(value)) throw new Error('invalid streams');
      if (keys[i]==='catalog'&&!Array.isArray(obj.channels)) throw new Error('invalid catalog');
      if (keys[i]==='monitoring'&&(!Array.isArray(obj.measurements)||!Array.isArray(obj.alerts)||typeof obj.timestamp!=='string')) throw new Error('invalid monitoring');
      if (keys[i]==='diagnostics'&&(!obj.cpu||!obj.memory||!obj.disk||typeof obj.timestamp!=='string')) throw new Error('invalid diagnostics');
      if (keys[i]==='interfaces'&&!Array.isArray(obj.interfaces)) throw new Error('invalid interfaces');
      if (keys[i]==='session'&&(typeof obj.authenticated!=='boolean'||typeof obj.role!=='string'||!['viewer','operator','admin','enroller','metrics'].includes(obj.role)||!['local_readonly','mtls','panel'].includes(String(obj.mode)))) throw new Error('invalid session');
      Object.assign(raw, { [keys[i]]: value });
    } catch { if (!signal.aborted) problems.push(`${keys[i]}: indisponível`); }
  }));
  return { raw, problems };
}
export function cpuPercent(previous: Diagnostics | undefined, current: Diagnostics | undefined): number | null {
  if (!previous?.cpu.value || !current?.cpu.value || previous.cpu.state !== 'real' || current.cpu.state !== 'real') return null;
  if (Date.parse(current.timestamp) <= Date.parse(previous.timestamp)) return null;
  if (Date.parse(current.timestamp) - Date.parse(previous.timestamp) > 15000) return null;
  const total = current.cpu.value.total_ticks - previous.cpu.value.total_ticks;
  const idle = current.cpu.value.idle_ticks - previous.cpu.value.idle_ticks;
  return total > 0 && idle >= 0 && idle <= total ? 100 * (1 - idle / total) : null;
}
export function adapt(raw: Raw, previous?: Diagnostics, history: Point[] = [], problems: string[] = []): ViewData {
  const now = Date.now();
  const age = raw.monitoring ? now - Date.parse(raw.monitoring.timestamp) : Infinity;
  const fresh = raw.monitoring?.state === 'measured' && age >= 0 && age <= 3000;
  const metrics = fresh ? raw.monitoring?.measurements ?? [] : [];
  const values = (name: string) => metrics.filter(m => m.name === name && m.state === 'measured' && m.value !== null && Number.isFinite(m.value));
  const sum = (name: string): number | null => { const all = values(name); return all.length ? all.reduce((s, m) => s + (m.value ?? 0), 0) : null; };
  const channels: Channel[] = (raw.catalog?.channels ?? []).map(c => {
    const stream = raw.streams?.find(s => s.channel_id === c.id);
    const available = values('stream_ingest_available').find(m => m.channel_id === c.id)?.value === 1;
    const errors = (stream?.transport?.cc_errors ?? 0) + (stream?.transport?.tei ?? 0);
    return { ...c, category: c.source.startsWith('synthetic://') ? 'LABORATÓRIO' : 'CATÁLOGO', mark: c.name.slice(0, 2).toUpperCase(), brand: 'lab', bitrate: values('stream_ingest_bitrate_bits_per_second').find(m => m.channel_id === c.id)?.value ?? null,
      status: stream?.state === 'failed' || errors > 0 ? 'attention' : available ? 'online' : stream?.state==='stopped' ? 'stopped' : 'unavailable', quality: errors > 0 ? 'Erros TS observados' : stream?.transport?.cc_state === 'measured' ? 'Cabeçalhos TS medidos; não certifica qualidade' : 'Qualidade indisponível', synthetic: c.source.startsWith('synthetic://') };
  });
  const memory = raw.diagnostics?.memory.state === 'real' ? raw.diagnostics.memory.value : null;
  const disk = raw.diagnostics?.disk.state === 'real' ? raw.diagnostics.disk.value : null;
  const peers = raw.wireguard?.state === 'real' ? raw.wireguard.peers ?? [] : [];
  const observed = peers.filter(p => p.last_handshake).sort((a, b) => Date.parse(b.last_handshake!) - Date.parse(a.last_handshake!))[0];
  return {
    session: raw.session, demo: false, connected: raw.health?.status === 'ok', catalogAvailable:raw.catalog!==undefined, channels, active: channels.filter(c => c.status === 'online').length, total: channels.length,
    input: sum('stream_ingest_bitrate_bits_per_second'), output: null, clients: null, uptime: raw.edge?.uptime_seconds ?? null,
    cpu: cpuPercent(previous, raw.diagnostics), memoryUsed: memory?.used_bytes ?? null, memoryTotal: memory?.total_bytes ?? null, diskUsed: disk?.used_bytes ?? null, diskTotal: disk?.total_bytes ?? null,
    // All interfaces overlap (physical/VPN). Do not invent an aggregate bitrate.
    networkRx: null, networkTx: null, interfaces: raw.interfaces?.interfaces ?? [],
    networkRates: [...new Set(metrics.filter(m=>m.interface).map(m=>m.interface!))].map(name=>({name,rx:values('network_rx_bits_per_second').find(m=>m.interface===name)?.value??null,tx:values('network_tx_bits_per_second').find(m=>m.interface===name)?.value??null})),
    wireguard: { state: raw.wireguard?.state !== 'real' ? 'Indisponível' : observed ? 'Handshake observado' : 'Sem handshake', detail: observed ? `${observed.interface} · não comprova conexão atual` : peers.length ? `${peers.length} peer(s) inventariado(s)` : 'Nenhum túnel confirmado', observed: observed?.last_handshake ?? null },
    alerts: raw.monitoring?.alerts ?? [], history, updated: raw.monitoring?.timestamp ?? null, problems,
    edge: { version: raw.edge?.version ?? 'dev', os: raw.edge?.os ?? 'Indisponível', arch: raw.edge?.arch ?? '—', hls: raw.edge?.features?.hls ?? false, stream: raw.edge?.features?.stream ?? false, catalog: raw.edge?.features?.catalog ?? false, id:raw.edge?.identity?.edge_id??null },
  };
}
export function filterChannels(channels: Channel[], search: string, status: string, category: string): Channel[] {
  const query = search.toLocaleLowerCase('pt-BR').trim();
  return channels.filter(c => `${c.name} ${c.id} ${c.source}`.toLocaleLowerCase('pt-BR').includes(query) && (status === 'all' || c.status === status) && (category === 'all' || c.category === category));
}
export function rate(value: number | null): string { return value === null || !Number.isFinite(value) ? '—' : `${(value / 1_000_000).toLocaleString('pt-BR', { maximumFractionDigits: 1 })} Mbps`; }
export function gigabytes(value: number | null): string { return value === null ? '—' : (value / 2 ** 30).toLocaleString('pt-BR', { maximumFractionDigits: 1 }); }
export function uptime(value: number | null): string { if (value === null) return '—'; const n = Math.max(0, Math.floor(value)); return `${Math.floor(n / 86400)}d ${String(Math.floor(n / 3600) % 24).padStart(2, '0')}:${String(Math.floor(n / 60) % 60).padStart(2, '0')}:${String(n % 60).padStart(2, '0')}`; }
