// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {ChannelProcessing} from './media-channel-processing';
afterEach(()=>{cleanup();vi.restoreAllMocks()});
function fixture({tab='input',local=true,admin=true,devices=[{id:'cpu',name:'CPU sintética'}],stale=false}:{tab?:'input'|'transcoder';local?:boolean;admin?:boolean;devices?:{id:string;name:string}[];stale?:boolean}={}){
 let profile={revision:7,inputs:local?[{key:'primary',address:'https://synthetic.example/canal/mpegts',protocol:'http_ts',interface:'',local_ip:''}]:[],transcode:{mode:'copy'},active_key:'primary',local} as any;
 const call=vi.fn(async(method:string,path:string,body?:any)=>{
  if(path==='manual-channels/encoders')return {devices,max_concurrent:2,warning:''};
  if(path==='live/capabilities')return {interfaces:[{name:'eth-lab',up:true,ipv4:['192.0.2.10']}]};
  if(method==='GET')return profile;
  if(stale)throw new Error('O cadastro mudou. Recarregue.');
  profile={...profile,revision:8,inputs:body.inputs?body.inputs.map((v:any,i:number)=>({...v,key:v.key||'b'.repeat(48),address:'https://synthetic.example/'+i})):profile.inputs,transcode:body.transcode||profile.transcode};return profile;
 });
 const refresh=vi.fn(async()=>{});
 render(<ChannelProcessing id="local_synthetic" tab={tab} live={{admin,call,refresh} as any} blocked={false}/>);
 return {call,refresh};
}
it('adds and reorders protected input URLs then saves without another password or stop action',async()=>{
 const {call,refresh}=fixture();await screen.findByRole('heading',{name:'Entradas do canal'});
 expect((screen.getByLabelText('URL da entrada 1') as HTMLInputElement).value).toBe('');
 fireEvent.click(screen.getByRole('button',{name:'Adicionar URL'}));
 fireEvent.change(screen.getByLabelText('URL da entrada 2'),{target:{value:'https://synthetic-user:synthetic-password@backup.example/index.m3u8?token=synthetic'}});
 expect((screen.getByLabelText('Protocolo da entrada 2') as HTMLSelectElement).value).toBe('hls');
 fireEvent.click(screen.getByRole('button',{name:'Subir entrada 2'}));
 fireEvent.click(screen.getByRole('button',{name:'Salvar configuração'}));
 await waitFor(()=>expect(refresh).toHaveBeenCalledTimes(1));
 const writes=call.mock.calls.filter(v=>v[0]==='POST');expect(writes).toHaveLength(1);
 expect(writes[0][2]).toMatchObject({action:'inputs',revision:7,source:{id:'local_synthetic'},inputs:[{key:'',protocol:'hls'},{key:'primary',url:'',protocol:'http_ts'}]});
 expect(screen.queryByDisplayValue(/synthetic-password/)).toBeNull();
 expect(writes[0][2].current_password).toBeUndefined();
});
it('requires explicit UDP interface and IP rather than assuming them',async()=>{
 fixture();await screen.findByRole('heading',{name:'Entradas do canal'});fireEvent.click(screen.getByRole('button',{name:'Adicionar URL'}));
 fireEvent.change(screen.getByLabelText('URL da entrada 2'),{target:{value:'udp://239.77.0.2:12000'}});
 await screen.findByRole('option',{name:'eth-lab'});
 expect((screen.getByLabelText('Interface da entrada 2') as HTMLSelectElement).value).toBe('');
 expect((screen.getByLabelText('IP local da entrada 2') as HTMLSelectElement).value).toBe('');
 fireEvent.change(screen.getByLabelText('Interface da entrada 2'),{target:{value:'eth-lab'}});
 expect((screen.getByLabelText('IP local da entrada 2') as HTMLSelectElement).value).toBe('');
});
it('opens in passthrough and saves an explicit validated encoder profile',async()=>{
 const {call}=fixture({tab:'transcoder'});
 const copy=await screen.findByRole('radio',{name:'Transmitir sem transcodificar'});expect((copy as HTMLInputElement).checked).toBe(true);
 expect(call.mock.calls.every(v=>v[0]==='GET')).toBe(true);
 const enable=screen.getByRole('radio',{name:'Transcodificar áudio e vídeo'});
 await waitFor(()=>expect((enable as HTMLInputElement).disabled).toBe(false));fireEvent.click(enable);
 const save=screen.getByRole('button',{name:'Salvar configuração'});expect((save as HTMLButtonElement).disabled).toBe(true);
 fireEvent.change(screen.getByLabelText('Dispositivo'),{target:{value:'cpu'}});fireEvent.click(save);
 await screen.findByText(/Configuração salva/);
 expect(call.mock.calls.find(v=>v[0]==='POST')?.[2]).toMatchObject({action:'transcode',transcode:{mode:'h264',device:'cpu',video_kbps:3000,width:1280,height:720,fps:25,audio:'aac',audio_kbps:128}});
 fireEvent.click(screen.getByRole('radio',{name:'Transmitir sem transcodificar'}));fireEvent.click(save);
 await waitFor(()=>expect(call.mock.calls.filter(v=>v[0]==='POST')).toHaveLength(2));
 expect(call.mock.calls.filter(v=>v[0]==='POST')[1][2].transcode).toEqual({mode:'copy'});
});
it('never offers unavailable GPUs or writes settings merely by opening a tab',async()=>{
 const {call}=fixture({tab:'transcoder',devices:[]});await screen.findByText(/Nenhum codificador utilizável/);
 expect((screen.getByRole('radio',{name:'Transcodificar áudio e vídeo'}) as HTMLInputElement).disabled).toBe(true);
 expect(screen.queryByRole('option',{name:/NVIDIA/})).toBeNull();expect(call.mock.calls.every(v=>v[0]==='GET')).toBe(true);
});
it('keeps the draft after a conflict and offers reload',async()=>{
 fixture({stale:true});await screen.findByRole('heading',{name:'Entradas do canal'});
 fireEvent.change(screen.getByLabelText('URL da entrada 1'),{target:{value:'https://replacement.example/new.ts'}});
 fireEvent.click(screen.getByRole('button',{name:'Salvar configuração'}));await screen.findByRole('alert');
 expect(screen.getByDisplayValue('https://replacement.example/new.ts')).toBeTruthy();
 expect(screen.getByRole('button',{name:'Recarregar configuração'})).toBeTruthy();
});
it('protects matrix origins and denies configuration to viewers',async()=>{
 const {call}=fixture({local:false});await screen.findByText(/As entradas deste canal são administradas pelo Kore Play/);
 expect(screen.queryByRole('button',{name:'Adicionar URL'})).toBeNull();expect(call.mock.calls.every(v=>v[0]==='GET')).toBe(true);
 cleanup();const viewer=fixture({admin:false});expect(screen.getByText(/Somente administradores/)).toBeTruthy();expect(viewer.call).not.toHaveBeenCalled();
});
