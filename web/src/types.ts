export type Point = { at: number; rx: number | null; tx: number | null; cpu: number | null; memory: number | null };
export type Channel = { id: string; name: string; category: string; source: string; outputs: string[]; bitrate: number | null; status: 'online' | 'attention' | 'stopped' | 'unavailable'; quality: string; mark: string; brand: string; synthetic: boolean };
export type Interface = { name: string; ipv4: string[]; up: boolean; multicast: boolean; mtu: number; index: number };
export type Alert = { id: string; code: string; channel_id: string; state: string; severity: string; since: string };
export type Measurement = { name: string; channel_id?: string; interface?: string; state: string; value: number | null; timestamp: string; origin: string };
export type ViewData = {
  session?: Session;
  demo: boolean; connected: boolean; catalogAvailable: boolean; channels: Channel[]; active: number; total: number;
  input: number | null; output: number | null; clients: number | null; uptime: number | null;
  cpu: number | null; memoryUsed: number | null; memoryTotal: number | null; diskUsed: number | null; diskTotal: number | null;
  networkRx: number | null; networkTx: number | null; interfaces: Interface[];
  networkRates: { name: string; rx: number | null; tx: number | null }[];
  wireguard: { state: string; detail: string; observed: string | null };
  alerts: Alert[]; history: Point[]; updated: string | null; problems: string[];
  edge: { version: string; os: string; arch: string; hls: boolean; stream: boolean; catalog: boolean; id: string | null };
};
export type Diagnostics = { timestamp: string; cpu: { state: string; value: { total_ticks: number; idle_ticks: number } | null }; memory: { state: string; value: { used_bytes: number; total_bytes: number } | null }; disk: { state: string; value: { used_bytes: number; total_bytes: number } | null } };
export type Session = { authenticated: boolean; role: string; mode: string };
export type Raw = {
 session?: Session;
  health?: { status: string }; edge?: { uptime_seconds: number; version: string; os: string; arch: string; features: { hls: boolean; stream: boolean; catalog: boolean }; identity?: { edge_id: string } | null };
  catalog?: { channels: { id: string; name: string; source: string; outputs: string[] }[] };
  streams?: { channel_id: string; state: string; source: string; bitrate: { bits_per_second: number | null }; transport: { cc_errors?: number; tei?: number; cc_state?: string }; udp?: { state: string; bytes: number; errors: number } }[];
  monitoring?: { timestamp: string; state: string; measurements: Measurement[]; alerts: Alert[]; recent_alerts: Alert[] };
  diagnostics?: Diagnostics; interfaces?: { interfaces: Interface[] };
  wireguard?: { state: string; peers?: { interface: string; last_handshake: string | null; handshake_state: string }[] };
};
