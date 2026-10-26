import type { ViewData } from './types';
// Opt-in visual fixture only. Never sent to an API or presented as real telemetry.
export function demoData(): ViewData {
  const names = ['Globo HD', 'SBT HD', 'Record HD', 'Band HD', 'ESPN', 'SporTV 1', 'Disney Channel', 'Discovery'];
  const marks = ['◉', 'sbt', '◉', '◉', 'ESPN', 'SPORTV', 'Disney', 'D'];
  const categories = ['ABERTOS', 'ABERTOS', 'ABERTOS', 'ABERTOS', 'ESPORTES', 'ESPORTES', 'KIDS', 'DOCUMENTÁRIOS'];
  const mbps = [8.2, 7.6, 7.9, 6.8, 9.1, 8.7, 5.4, 6.1];
  const now = Date.now();
  return {
    demo: true, connected: true, catalogAvailable:true,
    channels: names.map((name, i) => ({ id: `demo-${i + 1}`, name, category: categories[i], source: `10.250.0.1:${i < 4 ? 10001 + i : i < 6 ? 10021 + i - 4 : 10030 + i - 6}`, outputs: ['udp', 'hls'], bitrate: mbps[i] * 1e6, status: i === 7 ? 'attention' : 'online', quality: i === 7 ? 'Alerta ilustrativo' : 'Qualidade ilustrativa', mark: marks[i], brand: `brand-${i}`, synthetic: true })),
    active: 26, total: 27, input: 648e6, output: 612e6, clients: 143, uptime: 12 * 86400 + 4 * 3600 + 31 * 60 + 26,
    cpu: 18, memoryUsed: 2.4 * 2 ** 30, memoryTotal: 8 * 2 ** 30, diskUsed: 28 * 2 ** 30, diskTotal: 120 * 2 ** 30, networkRx: 1.2e9, networkTx: 680e6,
    interfaces: [{ name: 'eth0 · Gestão', index: 1, up: true, multicast: true, mtu: 1500, ipv4: ['192.168.1.10/24'] }, { name: 'eth1 · IPTV', index: 2, up: true, multicast: true, mtu: 1500, ipv4: ['10.10.50.1/24'] }, { name: 'wg0 · WireGuard', index: 3, up: true, multicast: false, mtu: 1420, ipv4: ['10.250.0.11/32'] }],
    wireguard: { state: 'Conectado', detail: '10.250.0.11 → 10.250.0.1', observed: null }, alerts: [{ id: 'demo-warning', code: 'continuity_errors', channel_id: 'demo-8', state: 'firing', severity: 'warning', since: new Date(now).toISOString() }],
    networkRates: [{name:'eth0 · Gestão',rx:12e6,tx:4e6},{name:'eth1 · IPTV',rx:1.2e9,tx:680e6},{name:'wg0 · WireGuard',rx:648e6,tx:1e6}],
    history: Array.from({ length: 121 }, (_, i) => ({ at: now - (120 - i) * 15000, rx: (570 + Math.sin(i / 10) * 70 - (i > 28 && i < 43 ? 370 : 0) - (i > 99 && i < 112 ? 400 : 0)) * 1e6, tx: (950 + Math.sin(i / 5) * 60 + Math.sin(i * 1.4) * 40) * 1e6, cpu: 18 + Math.sin(i / 8) * 5 + Math.sin(i / 3) * 2, memory: 30 + Math.sin(i / 15) * 5 })),
    updated: new Date(now).toISOString(), problems: [], edge: { version: '1.0.0', os: 'Ubuntu 24.04 LTS', arch: 'amd64', hls: true, stream: true, catalog: true,id:'EDGE-001' },
  };
}
