// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {AccountContext} from './account-context';
import {ChannelWorkspace} from './channel-workspace';
afterEach(cleanup);
const user={id:'a'.repeat(32),username:'owner',role:'admin' as const,revision:1,disabled:false};
const channel={id:'synthetic',name:'Canal sintético',category:'Lab',protocols:[{id:'http_ts',label:'MPEG-TS'}],outputs:['udp','hls']};
const snapshot={configuration:{generation:0,channels:[],tools:{ffmpeg:'',ffprobe:''}},catalog:{available:true,configured:true,catalog:{channels:[channel],partner_name:'Synthetic'}},streams:[],max_active:16};
function mount(mode:'channels'|'udp'|'hls'='channels'){
 const request=vi.fn(async(method:string,path:string)=>new Response(JSON.stringify(path.endsWith('/capabilities')?{tools:{ffmpeg:'/usr/bin/ffmpeg',ffprobe:'/usr/bin/ffprobe'},interfaces:[{name:'synthetic0',up:true,multicast:true,ipv4:['10.77.0.2']}]}:snapshot)));
 render(<AccountContext.Provider value={{user,request,reload:vi.fn(),logout:vi.fn()}}><ChannelWorkspace mode={mode}/></AccountContext.Provider>);return request;
}
it('lists received channels in UDP output without invoking sources or the old read-only placeholder',async()=>{
 const request=mount('udp');await screen.findByRole('button',{name:'Configurar Canal sintético'});
 expect(screen.queryByText(/Este painel é somente leitura/)).toBeNull();
 fireEvent.click(screen.getByRole('button',{name:'Adicionar canal à saída'}));await screen.findByRole('dialog');
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
 fireEvent.change(screen.getByLabelText('Senha administrativa deste Edge'),{target:{value:'synthetic-password'}});
 fireEvent.click(screen.getByLabelText('Conferi o canal, a operação e os destinos acima.'));
 fireEvent.click(screen.getByRole('button',{name:'Salvar preparação'}));await waitFor(()=>expect(request.mock.calls.some(c=>c[0]==='POST')).toBe(true));
 const call=request.mock.calls.find(c=>c[0]==='POST')!;expect(call[1]).toBe('/api/v1/live/action');
 expect(screen.queryByText(/Origem cadastrada.*Online/)).toBeNull();
});
