// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {AccountContext} from './account-context';
import {ChannelWorkspace} from './channel-workspace';
afterEach(cleanup);
const user={id:'a'.repeat(32),username:'owner',role:'admin' as const,revision:1,disabled:false};
const channel={id:'synthetic',name:'Canal sintético',category:'Lab',protocols:[{id:'http_ts',label:'MPEG-TS'}],outputs:['udp','hls']};
const snapshot={configuration:{generation:0,channels:[],tools:{ffmpeg:'',ffprobe:''}},catalog:{available:true,configured:true,catalog:{channels:[channel],partner_name:'Synthetic'}},streams:[],max_active:16};
function mount(view:unknown=snapshot){
 const request=vi.fn(async(method:string,path:string,_body?:unknown)=>new Response(JSON.stringify(path.endsWith('manual-channels')?{revision:0,channels:[]}:path.endsWith('/capabilities')?{tools:{ffmpeg:'/usr/bin/ffmpeg',ffprobe:'/usr/bin/ffprobe'},interfaces:[{name:'synthetic0',up:true,multicast:true,ipv4:['10.77.0.2']}]}:view)));
 render(<AccountContext.Provider value={{user,request,reload:vi.fn(),logout:vi.fn()}}><ChannelWorkspace/></AccountContext.Provider>);return request;
}
it('lists received channels in UDP output without invoking sources or the old read-only placeholder',async()=>{
 const request=mount();await screen.findByRole('button',{name:'Configurar Canal sintético'});
 expect(screen.queryByText(/Este painel é somente leitura/)).toBeNull();
 fireEvent.click(screen.getByRole('button',{name:'Configurar Canal sintético'}));await screen.findByRole('dialog');await waitFor(()=>expect(screen.queryByText('Consultando ferramentas e interfaces...')).toBeNull());
 fireEvent.click(screen.getByLabelText('Preparar saída multicast UDP'));
 expect((screen.getByLabelText('Interface') as HTMLSelectElement).value).toBe('');
 expect((screen.getByLabelText('IP de origem') as HTMLSelectElement).value).toBe('');
 expect((screen.getByLabelText('Grupo multicast privado (239.x.x.x)') as HTMLInputElement).value).toBe('');
 expect((screen.getByLabelText('Porta UDP') as HTMLInputElement).value).toBe('');
 expect((screen.getByRole('button',{name:'Salvar preparação'}) as HTMLButtonElement).disabled).toBe(true);
 expect(request.mock.calls.every(c=>c[0]==='GET')).toBe(true);
});
it('saves reviewed preparation separately from starting a stream',async()=>{
 const request=mount();await screen.findByRole('button',{name:'Configurar Canal sintético'});fireEvent.click(screen.getByRole('button',{name:'Configurar Canal sintético'}));await screen.findByRole('dialog');
 expect(screen.queryByLabelText('Senha administrativa deste Edge')).toBeNull();
 fireEvent.click(screen.getByLabelText('Conferi o canal, a operação e os destinos acima.'));
 fireEvent.click(screen.getByRole('button',{name:'Salvar preparação'}));await waitFor(()=>expect(request.mock.calls.some(c=>c[0]==='POST')).toBe(true));
 const call=request.mock.calls.find(c=>c[0]==='POST')!;expect(call[1]).toBe('/api/v1/live/action');
 expect(screen.queryByText(/Origem cadastrada.*Online/)).toBeNull();
 expect(call[2]).not.toHaveProperty('current_password');
});
it('offers manual registration even when no partner catalog exists',async()=>{const request=mount({...snapshot,catalog:{available:false,catalog:null}});await screen.findByRole('button',{name:'Adicionar canal'});fireEvent.click(screen.getByRole('button',{name:'Adicionar canal'}));await screen.findByRole('heading',{name:'Adicionar canal por link'});expect(request.mock.calls.every(c=>c[0]==='GET')).toBe(true)});
it('does not open on-demand media when playing a stopped manual channel',async()=>{const c={...channel,id:'local_'+'a'.repeat(32),name:'Manual synthetic'};const request=mount({...snapshot,catalog:{...snapshot.catalog,catalog:{channels:[c]}}});await screen.findByRole('button',{name:'Reproduzir Manual synthetic'});fireEvent.click(screen.getByRole('button',{name:'Reproduzir Manual synthetic'}));await screen.findByText('Inicie a recepção contínua deste canal antes de abrir o player.');expect(request.mock.calls.every(c=>c[0]==='GET')).toBe(true)});

it('opens configuration immediately and keeps failures inside a retryable dialog',async()=>{
 let finish!:(value:Response)=>void;
 const request=vi.fn(async(_method:string,path:string)=>path.endsWith('/capabilities')?await new Promise<Response>(resolve=>{finish=resolve}):new Response(JSON.stringify(snapshot)));
 render(<AccountContext.Provider value={{user,request,reload:vi.fn(),logout:vi.fn()}}><ChannelWorkspace/></AccountContext.Provider>);
 fireEvent.click(await screen.findByRole('button',{name:'Configurar Canal sint\u00e9tico'}));
 expect(screen.getByRole('dialog')).toBeTruthy();expect(screen.getByText('Consultando ferramentas e interfaces...')).toBeTruthy();
 finish(new Response(JSON.stringify({error:'Inventario temporariamente indisponivel'}),{status:503}));
 await screen.findByRole('button',{name:'Tentar novamente'});expect(screen.getByRole('dialog')).toBeTruthy();
 fireEvent.click(screen.getByRole('button',{name:'Tentar novamente'}));
 finish(new Response(JSON.stringify({tools:{ffmpeg:'',ffprobe:''},interfaces:[],interfaces_available:false,warning:'Sem inventario de rede'})));
 await screen.findByText('Sem inventario de rede');expect((screen.getByLabelText('Preparar sa\u00edda multicast UDP') as HTMLInputElement).disabled).toBe(true);
 fireEvent.click(screen.getByLabelText('Conferi o canal, a opera\u00e7\u00e3o e os destinos acima.'));
 expect((screen.getByRole('button',{name:'Salvar prepara\u00e7\u00e3o'}) as HTMLButtonElement).disabled).toBe(false);
});

it('starts an unconfigured channel directly with no confirmation dialog',async()=>{
 const request=mount();fireEvent.click(await screen.findByRole('button',{name:'Iniciar Canal sint\u00e9tico'}));
 await waitFor(()=>expect(request.mock.calls.some(c=>c[0]==='POST')).toBe(true));
 expect(request.mock.calls.find(c=>c[0]==='POST')?.[2]).toMatchObject({action:'start',channel_id:'synthetic',confirmed:true});
 expect(screen.queryByRole('dialog')).toBeNull();
});
it('shows one stop control per row and allows configuring an active channel',async()=>{
 const request=mount({...snapshot,streams:[{channel_id:'synthetic',state:'running',metrics:{last_packet_at:new Date().toISOString()},udp_state:'disabled',hls_state:'disabled'}]});
 fireEvent.click(await screen.findByRole('button',{name:'Configurar Canal sint\u00e9tico'}));await screen.findByRole('dialog');
 expect(screen.queryByText(/Pare a recep.*antes de alterar/)).toBeNull();
 fireEvent.click(screen.getByRole('button',{name:'Fechar revis\u00e3o'}));
 expect(screen.queryByRole('button',{name:'Iniciar Canal sint\u00e9tico'})).toBeNull();
 fireEvent.click(screen.getByRole('button',{name:'Parar Canal sint\u00e9tico'}));
 await waitFor(()=>expect(request.mock.calls.find(c=>c[0]==='POST')?.[2]).toMatchObject({action:'stop',channel_id:'synthetic',confirmed:true}));
 expect(screen.queryByRole('dialog')).toBeNull();
});

it('never opens an on-demand preview for a stopped partner channel',async()=>{
 const request=mount();fireEvent.click(await screen.findByRole('button',{name:'Reproduzir Canal sint\u00e9tico'}));
 await screen.findByText('Inicie a recep\u00e7\u00e3o cont\u00ednua deste canal antes de abrir o player.');
 expect(request.mock.calls.every(c=>c[0]==='GET')).toBe(true);expect(screen.queryByText(/sob demanda/)).toBeNull();
});

it('switches between a filterable channel list and mini cards without media actions',async()=>{
 const request=mount();await screen.findByRole('button',{name:'Configurar Canal sint\u00e9tico'});
 expect(screen.queryByRole('button',{name:'Testar sinal'})).toBeNull();expect(screen.queryByRole('button',{name:'Recep\u00e7\u00e3o e sa\u00eddas'})).toBeNull();
 fireEvent.click(screen.getByRole('button',{name:'Mini cards'}));expect(screen.getByRole('button',{name:'Mini cards'}).getAttribute('aria-pressed')).toBe('true');expect(screen.queryByRole('table')).toBeNull();
 fireEvent.change(screen.getByRole('textbox',{name:'Buscar canal'}),{target:{value:'missing'}});expect(screen.queryByRole('button',{name:'Configurar'})).toBeNull();
 fireEvent.change(screen.getByRole('textbox',{name:'Buscar canal'}),{target:{value:''}});fireEvent.click(screen.getByRole('button',{name:'Lista'}));expect(screen.getByRole('table')).toBeTruthy();expect(request.mock.calls.every(c=>c[0]==='GET')).toBe(true);
});
it('offers authenticated TS and HLS URLs without creating a UDP destination',async()=>{
 const request=mount();await screen.findByRole('button',{name:'Configurar Canal sint\u00e9tico'});fireEvent.click(screen.getByRole('button',{name:'Sa\u00eddas'}));
 expect((screen.getByLabelText('Link MPEG-TS (sess\u00e3o autenticada)') as HTMLInputElement).value).toContain('/live/media/synthetic/stream.ts');
 expect((screen.getByLabelText(/Link HLS/) as HTMLInputElement).value).toContain('/live/hls/synthetic/index.m3u8');
 expect(screen.getByRole('button',{name:'Configurar destino UDP'})).toBeTruthy();expect(request.mock.calls.every(c=>c[0]==='GET')).toBe(true);
});
