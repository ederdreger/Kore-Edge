// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {MediaChannelOutput} from './media-channel-output';
afterEach(()=>{cleanup();vi.restoreAllMocks()});
const base='/api/v1/playback/SYNTHETIC/synthetic/';
function fixture({admin=true,paused=false,hls=true,conflict=false,udpError=false,inventoryFailsOnce=false}={}){
 let inventoryCalls=0;
 let configuration:any={revision:3,network:{enabled:false,bind_ip:'',public_host:'',rtsp_port:0,rtmp_port:0,srt_port:0},channel:{dash:false,rtsp:false,rtmp:false,srt:false,segments:4,duration:2,max_readers:16},status:{bridge:'disabled',feed:'disabled',dash:'disabled',last_packet:''},mediamtx_available:true,ffmpeg_available:true};
 const row:any={channel:{id:'synthetic',name:'Canal sintético',outputs:['hls','udp'],protocols:[{id:'http_ts'}]},config:{channel_id:'synthetic',protocol:'http_ts',hls,resume:!paused,auto_paused:paused},stream:{state:paused?'stopped':'running',hls_state:hls&&!paused?'running':'disabled',udp_state:'disabled',metrics:{last_packet_at:new Date().toISOString()}}};
 const call=vi.fn(async(method:string,path:string,body?:any)=>{
  if(path==='live/playback-links')return {links:[{channel_id:'synthetic',ts_path:base+'stream.ts',hls_path:base+'index.m3u8',embed_path:base+'embed.html',dash_path:base+'cmaf/index.mpd',cmaf_path:base+'cmaf/master.m3u8',info_path:base+'media_info.json'}]};
  if(path==='live/capabilities'&&inventoryFailsOnce&&inventoryCalls++===0)throw new Error('synthetic inventory unavailable');
  if(path==='live/capabilities')return {tools:{ffmpeg:'/synthetic/ffmpeg',ffprobe:'/synthetic/ffprobe'},interfaces:[{name:'eth-lab',up:true,multicast:true,ipv4:['192.0.2.10']}]};
  if(path==='live/udp-plan'){if(udpError)throw new Error('Destino sintético inválido');return {}}
  if(method==='POST'){if(conflict)throw new Error('A configuração de saídas mudou.');configuration={...configuration,revision:4,channel:body.channel,network:body.network}}
  return {configuration,links:{}};
 });
 const action=vi.fn(async(body:any)=>{row.config=body.channel;return {}}),refresh=vi.fn(async()=>{});
 render(<MediaChannelOutput row={row} live={{admin,call,action,refresh,view:{manual_revision:5,configuration:{generation:9,tools:{ffmpeg:'/synthetic/ffmpeg',ffprobe:'/synthetic/ffprobe'}}}} as any} blocked={false}/>);
 return {call,action,refresh};
}
it('lists permanent URLs automatically, with no generation controls or assumed listener ports',async()=>{
 const {call}=fixture();await screen.findByRole('form',{name:'Configuração das saídas'});
 expect((screen.getByLabelText('URL MPEG-TS') as HTMLInputElement).value).toBe(window.location.origin+base+'stream.ts');
 expect((screen.getByLabelText('URL PLAYER') as HTMLInputElement).value).toContain('/embed.html');
 expect((screen.getByLabelText('URL DASH') as HTMLInputElement).value).toBe('');
 expect((screen.getByLabelText('IP local de escuta') as HTMLInputElement).value).toBe('');
 expect((screen.getByLabelText('Porta SRT UDP') as HTMLInputElement).value).toBe('');
 expect(call.mock.calls.every(v=>v[0]==='GET')).toBe(true);expect(screen.queryByRole('button',{name:/Gerar|Revogar/})).toBeNull();
});
it('provides a clipboard fallback without rotating the URL',async()=>{
 fixture();await screen.findByRole('form',{name:'Configuração das saídas'});fireEvent.click(screen.getByRole('button',{name:'Copiar MPEG-TS'}));await screen.findByText('Link selecionado. Pressione Ctrl+C.');expect(document.activeElement).toBe(screen.getByLabelText('URL MPEG-TS'));
});
it('enables DASH together with required HLS preparation while retaining manual pause',async()=>{
 const {call,action}=fixture({paused:true,hls:false});await screen.findByRole('form',{name:'Configuração das saídas'});
 fireEvent.click(screen.getByRole('checkbox',{name:'Preparar DASH e HLS CMAF'}));fireEvent.click(screen.getByRole('button',{name:'Salvar saídas'}));
 await screen.findByText(/Saídas salvas/);expect(action).toHaveBeenCalledTimes(1);expect(action.mock.calls[0][0]).toMatchObject({action:'save',generation:9,manual_revision:5,channel:{hls:true,resume:false,auto_paused:true}});
 const writes=call.mock.calls.filter(v=>v[0]==='POST');expect(writes).toHaveLength(1);expect(writes[0][2]).toMatchObject({revision:3,channel:{dash:true,segments:4,duration:2},network:{enabled:false}});
 expect((screen.getByLabelText('URL DASH') as HTMLInputElement).value).toContain('/cmaf/index.mpd');
});
it('can prepare RTMP without activating any listener or restarting reception',async()=>{
 const {call,action}=fixture();await screen.findByRole('form',{name:'Configuração das saídas'});fireEvent.click(screen.getByRole('checkbox',{name:'Preparar RTMP'}));fireEvent.click(screen.getByRole('button',{name:'Salvar saídas'}));await screen.findByText(/Saídas salvas/);
 expect(action).not.toHaveBeenCalled();expect(call.mock.calls.find(v=>v[0]==='POST')?.[2]).toMatchObject({channel:{rtmp:true},network:{enabled:false,rtmp_port:0,bind_ip:''}});
});
it('validates the UDP destination before applying it and keeps explicit interface/IP empty initially',async()=>{
 const {call,action}=fixture({udpError:true});await screen.findByRole('form',{name:'Configuração das saídas'});fireEvent.click(screen.getByRole('checkbox',{name:'Habilitar envio ao destino'}));
 expect((screen.getByLabelText('Interface de saída') as HTMLSelectElement).value).toBe('');expect((screen.getByLabelText('IP local de saída') as HTMLSelectElement).value).toBe('');
 fireEvent.change(screen.getByLabelText('Interface de saída'),{target:{value:'eth-lab'}});fireEvent.change(screen.getByLabelText('IP local de saída'),{target:{value:'192.0.2.10'}});fireEvent.change(screen.getByLabelText('Grupo multicast (239.x.x.x)'),{target:{value:'239.77.0.1'}});fireEvent.change(screen.getByLabelText('Porta UDP'),{target:{value:'12000'}});fireEvent.click(screen.getByRole('button',{name:'Salvar saídas'}));
 await screen.findByRole('alert');expect(action).not.toHaveBeenCalled();expect(call.mock.calls.filter(v=>v[0]==='POST').map(v=>v[1])).toEqual(['live/udp-plan']);
});
it('preserves drafts after a revision conflict and does not claim success',async()=>{
 fixture({conflict:true});await screen.findByRole('form',{name:'Configuração das saídas'});fireEvent.change(screen.getByLabelText('Quantidade de segmentos DASH/CMAF'),{target:{value:'6'}});fireEvent.click(screen.getByRole('button',{name:'Salvar saídas'}));await screen.findByRole('alert');expect((screen.getByLabelText('Quantidade de segmentos DASH/CMAF') as HTMLInputElement).value).toBe('6');expect(screen.queryByText(/Saídas salvas/)).toBeNull();
});
it('does not fetch playback credentials or configuration for viewers',()=>{const {call}=fixture({admin:false});expect(call).not.toHaveBeenCalled();expect(screen.getByText(/Somente administradores/)).toBeTruthy()});
it('reloads interfaces independently without discarding a UDP destination draft',async()=>{
 const {action}=fixture({inventoryFailsOnce:true});await screen.findByRole('form',{name:'Configuração das saídas'});
 await screen.findByText(/Não foi possível consultar as interfaces/);
 fireEvent.click(screen.getByRole('checkbox',{name:'Habilitar envio ao destino'}));
 fireEvent.change(screen.getByLabelText('Grupo multicast (239.x.x.x)'),{target:{value:'239.77.0.4'}});
 fireEvent.click(screen.getByRole('button',{name:'Recarregar interfaces'}));
 await screen.findByRole('option',{name:/eth-lab/});
 expect((screen.getByLabelText('Grupo multicast (239.x.x.x)') as HTMLInputElement).value).toBe('239.77.0.4');
 expect((screen.getByLabelText('Interface de saída') as HTMLSelectElement).value).toBe('');expect(action).not.toHaveBeenCalled();
});
