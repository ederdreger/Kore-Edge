// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {AccountContext} from './account-context';
import {KorePlayChannels} from './kore-play-channels';
afterEach(cleanup);
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
