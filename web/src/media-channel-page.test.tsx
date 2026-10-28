// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {AccountContext} from './account-context';
import {MediaWorkspace} from './media-workspace';
import {channelPath,readMediaRoute} from './media-channel-route';
const id='local_'+'a'.repeat(32),url='https://synthetic-user:synthetic-password@media.example/CANAL/mpegts?token=synthetic-token';
afterEach(()=>{cleanup();vi.restoreAllMocks();window.history.replaceState(null,'','#media')});
function fixture({existing=false,failSave=false,viewer=false,tab='overview'}={}){
 window.history.replaceState(null,'',existing?channelPath(id,tab as 'overview'|'input'|'output'):'#media/new');
 let source:any=existing?{id,name:'Canal sintético',title:'Título original',description:'Descrição original',category:'',protocol:'http_ts',address:'https://media.example/CANAL/mpegts',enabled:true,interface:'',local_ip:''}:null;
 let revision=existing?1:0;
 const view=()=>({manual_revision:revision,configuration:{generation:1,tools:{ffmpeg:'',ffprobe:''},channels:source?[{channel_id:id,protocol:source.protocol,resume:true,hls:false}]:[]},catalog:{available:true,configured:false,last_sync:'',catalog:{partner_name:'',channels:source?[{id,name:source.name,category:'',outputs:['hls','udp'],protocols:[{id:source.protocol,label:'HTTP MPEG-TS',path:'/synthetic'}]}]:[]}},streams:source?[{channel_id:id,protocol:source.protocol,state:'running',started_at:new Date().toISOString(),startup_ms:1,attempts:1,interruptions:0,udp_state:'disabled',hls_state:'disabled',metrics:{bytes:1316,bits_per_second:1000000,subscribers:0,last_packet_at:new Date().toISOString()}}]:[],max_active:16});
 const inventory=()=>({revision,channels:source?[source]:[]});
 const request=vi.fn(async(method:string,path:string,body?:any)=>{
  if(path==='/api/v1/live/capabilities')return new Response(JSON.stringify({tools:{ffmpeg:'',ffprobe:''},interfaces:[{name:'eth-lab',up:true,multicast:true,ipv4:['192.0.2.10']}]}));
  if(path==='/api/v1/manual-channels'){
   if(method==='GET')return new Response(JSON.stringify(inventory()));
   if(body.action==='save'){
    if(failSave)return new Response(JSON.stringify({error:'Falha sintética ao salvar'}),{status:503});
    source={...body.source,id,address:'https://media.example/CANAL/mpegts',title:'',description:''};delete source.url;revision++;
    return new Response(JSON.stringify({...inventory(),channel_id:id,prepared:true,started:true}));
   }
   if(body.action==='metadata'){source={...source,...body.metadata};revision++;return new Response(JSON.stringify(inventory()))}
   if(body.action==='delete'){source=null;revision++;return new Response(JSON.stringify(inventory()))}
  }
  if(path==='/api/v1/live/playback-links')return new Response(JSON.stringify({links:[]}));
  return new Response(JSON.stringify(view()));
 });
 const user={id:'synthetic',username:'operador',role:viewer?'viewer':'admin',revision:1,disabled:false} as any;
 render(<AccountContext.Provider value={{user,request,reload:vi.fn(),logout:vi.fn()}}><MediaWorkspace/></AccountContext.Provider>);
 return request;
}
it('saves name and source then opens the actual channel overview without placing credentials in history',async()=>{
 const request=fixture();
 fireEvent.change(screen.getByLabelText('Nome do canal'),{target:{value:'Canal sintético'}});
 fireEvent.change(screen.getByLabelText('URL da origem'),{target:{value:url}});
 const save=screen.getByRole('button',{name:'Salvar'});
 await waitFor(()=>expect((save as HTMLButtonElement).disabled).toBe(false));
 fireEvent.click(save);
 await screen.findByRole('form',{name:'Descrição do canal'});
 expect(window.location.hash).toBe(channelPath(id));
 expect(window.location.hash).not.toContain('synthetic-password');
 expect(screen.queryByDisplayValue(url)).toBeNull();
 const calls=request.mock.calls.filter(c=>c[0]==='POST');
 expect(calls).toHaveLength(1);
 expect(calls[0][2].source).toMatchObject({name:'Canal sintético',url,protocol:'http_ts',enabled:true});
 expect(screen.getByRole('tab',{name:'Visão geral'}).getAttribute('aria-selected')).toBe('true');
 expect((screen.getByRole('tab',{name:'Transcodificação'}) as HTMLButtonElement).disabled).toBe(true);
});
it('cancel does not create a channel and a failed save stays on the creation page',async()=>{
 const request=fixture({failSave:true});
 fireEvent.change(screen.getByLabelText('Nome do canal'),{target:{value:'Teste'}});
 fireEvent.change(screen.getByLabelText('URL da origem'),{target:{value:url}});
 await waitFor(()=>expect((screen.getByRole('button',{name:'Salvar'}) as HTMLButtonElement).disabled).toBe(false));
 fireEvent.click(screen.getByRole('button',{name:'Salvar'}));
 await screen.findByText('Falha sintética ao salvar');
 expect(window.location.hash).toBe('#media/new');
 expect(screen.getByDisplayValue(url)).toBeTruthy();
 fireEvent.click(screen.getByRole('button',{name:'Cancelar'}));
 await screen.findByRole('heading',{name:'Mídia'});
 expect(request.mock.calls.filter(c=>c[0]==='POST')).toHaveLength(1);
});
it('saves descriptive metadata on a running channel without stop/start or resending its URL',async()=>{
 const request=fixture({existing:true});
 await screen.findByDisplayValue('Título original');
 fireEvent.change(screen.getByLabelText('Título'),{target:{value:'Novo título'}});
 fireEvent.click(screen.getByRole('button',{name:'Salvar descrição'}));
 await screen.findByText('Descrição salva. A recepção foi mantida.');
 const calls=request.mock.calls.filter(c=>c[0]==='POST');
 expect(calls).toHaveLength(1);
 expect(calls[0][2]).toMatchObject({action:'metadata',revision:1,source:{id},metadata:{title:'Novo título'}});
 expect(calls[0][2].source.url).toBeUndefined();
 expect(request.mock.calls.some(c=>c[1]==='/api/v1/live/action')).toBe(false);
});
it('supports direct input links and returning to transmissions without any channel action',async()=>{
 const request=fixture({existing:true,tab:'input'});
 await screen.findByRole('heading',{name:'Entrada do canal'});
 expect(screen.getByText('https://media.example/CANAL/mpegts')).toBeTruthy();
 fireEvent.click(screen.getByRole('link',{name:'Transmissões'}));
 await screen.findByRole('heading',{name:'Mídia'});
 expect(request.mock.calls.every(c=>c[0]==='GET')).toBe(true);
});
it('requires explicit UDP interface and local IP before saving',async()=>{
 fixture();
 fireEvent.change(screen.getByLabelText('Nome do canal'),{target:{value:'UDP sintético'}});
 fireEvent.change(screen.getByLabelText('URL da origem'),{target:{value:'udp://239.77.0.1:12000'}});
 await screen.findByRole('option',{name:'eth-lab'});
 expect((screen.getByLabelText('Interface UDP') as HTMLSelectElement).value).toBe('');
 expect((screen.getByLabelText('IP local de recepção') as HTMLSelectElement).value).toBe('');
 expect((screen.getByRole('button',{name:'Salvar'}) as HTMLButtonElement).disabled).toBe(true);
 fireEvent.change(screen.getByLabelText('Interface UDP'),{target:{value:'eth-lab'}});
 fireEvent.change(screen.getByLabelText('IP local de recepção'),{target:{value:'192.0.2.10'}});
 await waitFor(()=>expect((screen.getByRole('button',{name:'Salvar'}) as HTMLButtonElement).disabled).toBe(false));
});
it('does not allow a viewer to open registration operations',async()=>{
 const request=fixture({viewer:true});
 expect(screen.getByRole('alert').textContent).toContain('Somente administradores');
 expect(screen.queryByRole('button',{name:'Salvar'})).toBeNull();
 expect(request.mock.calls.every(c=>c[0]==='GET')).toBe(true);
});
it('requires explicit deletion confirmation and stops only the selected active channel first',async()=>{
 const request=fixture({existing:true});
 const button=await screen.findByRole('button',{name:'Excluir canal'});
 await waitFor(()=>expect((button as HTMLButtonElement).disabled).toBe(false));
 fireEvent.click(button);
 expect(request.mock.calls.every(c=>c[0]==='GET')).toBe(true);
 fireEvent.click(screen.getByRole('button',{name:'Confirmar exclusão'}));
 await screen.findByRole('heading',{name:'Mídia'});
 const calls=request.mock.calls.filter(c=>c[0]==='POST');
 expect(calls.map(c=>[c[1],c[2].action])).toEqual([['/api/v1/live/action','stop'],['/api/v1/manual-channels','delete']]);
 expect(calls[0][2].channel_id).toBe(id);
 expect(calls[1][2].source.id).toBe(id);
});
it('rejects malformed channel routes and never interprets a URL as a channel id',()=>{
 expect(readMediaRoute('#media/channel/../overview')).toEqual({kind:'list'});
 expect(readMediaRoute('#media/channel/https://secret/overview')).toEqual({kind:'list'});
 expect(readMediaRoute(channelPath(id,'output'))).toEqual({kind:'channel',id,tab:'output'});
});
