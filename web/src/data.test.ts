import { describe, expect, it, vi, afterEach } from 'vitest';
import { adapt, cpuPercent, filterChannels, rate, readAPI, API_PATHS } from './data';
import { demoData } from './fixtures.test-data';
import type { Diagnostics, Raw } from './types';

afterEach(()=>vi.unstubAllGlobals());
describe('observações sem dados inventados',()=>{
 it('não usa a referência como fallback de telemetria',()=>{
  const d=adapt({});expect(d.demo).toBe(false);expect(d.catalogAvailable).toBe(false);expect(d.channels).toEqual([]);expect(d.input).toBeNull();expect(d.output).toBeNull();expect(d.clients).toBeNull();expect(d.cpu).toBeNull();expect(d.wireguard.state).toBe('Indisponível');expect(rate(null)).toBe('—');
 });
 it('descarta amostras antigas e não soma interfaces sobrepostas',()=>{
  const raw:Raw={monitoring:{state:'measured',timestamp:new Date(Date.now()-10000).toISOString(),measurements:[{name:'stream_ingest_bitrate_bits_per_second',state:'measured',value:1e6,origin:'synthetic',timestamp:new Date().toISOString()}],alerts:[],recent_alerts:[]}};
  const d=adapt(raw);expect(d.input).toBeNull();expect(d.networkRx).toBeNull();expect(d.networkTx).toBeNull();expect(d.output).toBeNull();
 });
 it('não confunde falta de medição com um canal parado',()=>{
  const d=adapt({catalog:{channels:[{id:'synthetic-test',name:'Teste',source:'synthetic://synthetic-test',outputs:['hls']}]}});
  expect(d.channels[0].status).toBe('unavailable');expect(d.channels[0].bitrate).toBeNull();
 });
 it('usa somente a identidade registrada devolvida pelo agente',()=>{
  const d=adapt({edge:{version:'dev',os:'windows',arch:'amd64',uptime_seconds:5,features:{catalog:false,hls:false,stream:false},identity:{edge_id:'edge-lab'}}});
  expect(d.edge.id).toBe('edge-lab');expect(adapt({}).edge.id).toBeNull();expect(d.demo).toBe(false);
 });
 it('não transforma erro HTTP em métricas simuladas e só usa GET',async()=>{
  const fetch=vi.fn(async()=>new Response('{}',{status:503}));vi.stubGlobal('fetch',fetch);
  const result=await readAPI(new AbortController().signal);expect(result.problems).toHaveLength(API_PATHS.length);expect(Object.keys(result.raw)).toHaveLength(0);expect(fetch.mock.calls).toHaveLength(API_PATHS.length);
  for(const call of fetch.mock.calls as unknown as [string,RequestInit][]){expect(call[1].method).toBe('GET');expect(API_PATHS).toContain(call[0]);}
 });
});
describe('CPU e filtros',()=>{
 it('mostra a identidade de acesso somente quando informada pelo agente',()=>{
  expect(adapt({}).session).toBeUndefined();
  expect(adapt({session:{authenticated:true,role:'operator',mode:'mtls'}}).session).toEqual({authenticated:true,role:'operator',mode:'mtls'});
 });
 const diag=(total:number,idle:number,seconds:number):Diagnostics=>({timestamp:new Date(1000000+seconds*1000).toISOString(),cpu:{state:'real',value:{total_ticks:total,idle_ticks:idle}},memory:{state:'unavailable',value:null},disk:{state:'unavailable',value:null}});
 it('exige duas leituras monotônicas frescas',()=>{expect(cpuPercent(undefined,diag(100,50,0))).toBeNull();expect(cpuPercent(diag(100,50,0),diag(200,130,5))).toBeCloseTo(20);expect(cpuPercent(diag(100,50,0),diag(10,1,5))).toBeNull();expect(cpuPercent(diag(100,50,0),diag(200,100,20))).toBeNull();});
 it('combina busca, status e categoria sem alterar o catálogo',()=>{const d=demoData();expect(d.demo).toBe(true);expect(filterChannels(d.channels,' ESPN ','all','ESPORTES')).toHaveLength(1);expect(filterChannels(d.channels,'','attention','all')[0].name).toBe('Discovery');expect(filterChannels(d.channels,'Globo','attention','all')).toEqual([]);expect(d.channels).toHaveLength(8);});
});
