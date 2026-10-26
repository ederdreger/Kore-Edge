// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {act,cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {AccountContext} from './account-context';
import {KorePlayChannels} from './kore-play-channels';
const preview=vi.hoisted(()=>({start:vi.fn(),stop:vi.fn()}));
vi.mock('./preview-engine',()=>({startPreview:(...args:unknown[])=>{preview.start(...args);return preview.stop}}));
afterEach(()=>{cleanup();vi.restoreAllMocks();vi.clearAllMocks()});
const user={id:'a'.repeat(32),username:'owner',role:'admin' as const,disabled:false,revision:1};
const catalog={generation:1,configured:true,available:true,error:'',config:{origin:'https://matrix.example:443',tunnel_ip:'10.77.0.1',auto_sync:true},last_sync:new Date().toISOString(),catalog:{partner_name:'Synthetic Partner',expires_at:new Date(Date.now()+300000).toISOString(),channels:[{id:'ch-one',name:'Synthetic Channel',category:'Lab',protocols:[{id:'http_ts',label:'HTTP MPEG-TS',path:'/api/kore-edge/media/ch-one/http_ts/stream.ts',verification:'not_tested'}]}]}};
it('lists authorized protocols without claiming signal quality and explicitly syncs and probes',async()=>{
 const request=vi.fn(async(method:string,path:string,_body?:unknown)=>new Response(JSON.stringify(path.endsWith('/probe')?{reachable:true,detail:'Synthetic sample only',checked_at:new Date().toISOString()}:catalog)));
 render(<AccountContext.Provider value={{user,request,reload:vi.fn(),logout:vi.fn()}}><KorePlayChannels/></AccountContext.Provider>);
 await screen.findByText('Synthetic Channel');expect(screen.getByText('HTTP MPEG-TS')).toBeTruthy();expect(screen.getByText('Origem cadastrada · ainda não testada')).toBeTruthy();
 const sync=screen.getByRole('button',{name:'Buscar canais'});expect((sync as HTMLButtonElement).disabled).toBe(true);
 fireEvent.change(screen.getByLabelText('Senha deste Edge'),{target:{value:'synthetic-password'}});fireEvent.click(screen.getByRole('checkbox'));fireEvent.click(sync);
 await waitFor(()=>expect(request.mock.calls.some(c=>c[0]==='POST'&&c[1].endsWith('/sync'))).toBe(true));
 fireEvent.click(screen.getByRole('button',{name:'Testar sinal'}));await screen.findByText('Amostra recebida');
 expect(request.mock.calls.find(c=>c[1].endsWith('/probe'))?.[2]).toEqual({channel:'ch-one',protocol:'http_ts'});
});
it('viewers can read the partner list without configuration or network probe actions',async()=>{
 const request=vi.fn(async()=>new Response(JSON.stringify(catalog)));
 render(<AccountContext.Provider value={{user:{...user,role:'viewer'},request,reload:vi.fn(),logout:vi.fn()}}><KorePlayChannels/></AccountContext.Provider>);
 await screen.findByText('Synthetic Channel');expect(screen.queryByRole('button',{name:'Buscar canais'})).toBeNull();expect(screen.queryByRole('button',{name:'Testar sinal'})).toBeNull();expect(request).toHaveBeenCalledTimes(1);
});

it('opens one player only after a click, switches channels, and stops after revocation',async()=>{
 let poll:()=>void=()=>{};
 const originalInterval=globalThis.setInterval;
 vi.spyOn(globalThis,'setInterval').mockImplementation((callback,delay)=>{if(delay===15000){poll=callback as ()=>void;return -1 as unknown as ReturnType<typeof setInterval>}return originalInterval(callback,delay)});
 let available=true;
 const channels=[...catalog.catalog.channels,{id:'ch-two',name:'Second Synthetic',category:'Lab',protocols:[{id:'hls',label:'HLS',path:'/api/kore-edge/media/ch-two/hls/index.m3u8',verification:'not_tested'}]}];
 const request=vi.fn(async()=>new Response(JSON.stringify({...catalog,available,catalog:{...catalog.catalog,channels}})));
 render(<AccountContext.Provider value={{user,request,reload:vi.fn(),logout:vi.fn()}}><KorePlayChannels/></AccountContext.Provider>);
 await screen.findByText('Second Synthetic');expect(preview.start).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole('button',{name:'Reproduzir Synthetic Channel (HTTP MPEG-TS)'}));
 expect(screen.getByRole('dialog',{name:'Mini player: Synthetic Channel'})).toBeTruthy();expect(preview.start).toHaveBeenCalledTimes(1);
 fireEvent.click(screen.getByRole('button',{name:'Reproduzir Synthetic Channel (HTTP MPEG-TS)'}));
 expect(preview.start).toHaveBeenCalledTimes(2);expect(preview.stop).toHaveBeenCalledTimes(1);
 fireEvent.click(screen.getByRole('button',{name:'Reproduzir Second Synthetic (HLS)'}));
 expect(screen.getAllByRole('dialog')).toHaveLength(1);expect(preview.stop).toHaveBeenCalledTimes(2);expect(preview.start).toHaveBeenCalledTimes(3);
 fireEvent.keyDown(document,{key:'Escape'});expect(screen.queryByRole('dialog')).toBeNull();expect(preview.stop).toHaveBeenCalledTimes(3);
 fireEvent.click(screen.getByRole('button',{name:'Reproduzir Second Synthetic (HLS)'}));
 available=false;await act(async()=>poll());await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull());expect(preview.stop).toHaveBeenCalledTimes(4);
});
