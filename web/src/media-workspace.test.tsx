// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor,within} from '@testing-library/react';
import {AccountContext} from './account-context';
import {MediaWorkspace} from './media-workspace';
import type {LiveView} from './live-distribution';
afterEach(()=>{cleanup();vi.restoreAllMocks()});
function fixture(role='admin',count=2,running=false){
 const user={id:'synthetic',username:'operador',role,revision:1,disabled:false} as any;
 const ids=Array.from({length:count},(_,i)=>'synthetic-'+i);
 let view:LiveView={configuration:{generation:4,tools:{ffmpeg:'',ffprobe:''},channels:ids.map(id=>({channel_id:id,protocol:'http_ts',hls:false,resume:false,auto_paused:true}))},catalog:{configured:true,available:true,last_sync:'',catalog:{partner_name:'Laboratório sintético',channels:ids.map((id,i)=>({id,name:'Canal '+(i+1),category:'Laboratório',outputs:[],protocols:[{id:'http_ts',label:'HTTP MPEG-TS',path:'/synthetic'}]}))}},streams:[],max_active:16};
 if(running)view.streams=[{channel_id:ids[0],protocol:'http_ts',state:'running',startup_ms:10,attempts:1,started_at:new Date().toISOString(),udp_state:'disabled',hls_state:'disabled',metrics:{bytes:1316,subscribers:1,bits_per_second:1000000,last_packet_at:new Date().toISOString()}}];
 const request=vi.fn(async(method:string,path:string,body?:any)=>{
  if(method==='POST'){
   view={...view,configuration:{...view.configuration,generation:view.configuration.generation+1}};
   return new Response(JSON.stringify(view));
  }
  if(path==='/api/v1/manual-channels')return new Response(JSON.stringify({revision:0,channels:[]}));
  if(path==='/api/v1/live/playback-links')return new Response(JSON.stringify({links:[]}));
  return new Response(JSON.stringify(view));
 });
 render(<AccountContext.Provider value={{user,request,reload:vi.fn(),logout:vi.fn()}}><MediaWorkspace version="teste"/></AccountContext.Provider>);
 return {request,setRunning:()=>{view.streams=[{channel_id:ids[0],protocol:'http_ts',state:'running',startup_ms:10,attempts:1,started_at:new Date().toISOString(),udp_state:'disabled',hls_state:'disabled',metrics:{bytes:1316,subscribers:1,bits_per_second:1000000,last_packet_at:new Date().toISOString()}}]}};
}
it('opens a Portuguese media inventory without operating sources or exposing unavailable modules',async()=>{
 const f=fixture();
 expect(await screen.findByRole('button',{name:'Canal 1'})).toBeTruthy();
 expect(screen.getByRole('tab',{name:'Transmissões'})).toBeTruthy();
 expect(screen.getByRole('tab',{name:'Fontes'})).toBeTruthy();
 expect(screen.queryByRole('tab',{name:'Modelos'})).toBeNull();
 expect(screen.getByRole('columnheader',{name:'Transcodificação'})).toBeTruthy();
 expect(screen.getAllByText('Sem transcodificação')).toHaveLength(2);
 expect(f.request.mock.calls.every(c=>c[0]==='GET')).toBe(true);
 fireEvent.click(screen.getByRole('tab',{name:'Fontes'}));
 expect(screen.queryByRole('columnheader',{name:'Gravação'})).toBeNull();
});
it('starts directly and reuses each returned generation for selected channels',async()=>{
 const f=fixture();
 fireEvent.click(await screen.findByLabelText('Selecionar transmissões desta página'));
 fireEvent.click(screen.getByRole('button',{name:'Iniciar selecionadas'}));
 await waitFor(()=>expect(f.request.mock.calls.filter(c=>c[0]==='POST')).toHaveLength(2));
 const calls=f.request.mock.calls.filter(c=>c[0]==='POST');
 expect(calls.map(c=>c[2]?.generation)).toEqual([4,5]);
 expect(calls.map(c=>c[2]?.channel_id)).toEqual(['synthetic-0','synthetic-1']);
 expect(calls.every(c=>c[2]?.confirmed===true&&c[2]?.action==='start')).toBe(true);
});
it('limits page selection to visible rows and provides card view with the same filters',async()=>{
 fixture('admin',26);
 await screen.findByRole('button',{name:'Canal 26'});
 fireEvent.change(screen.getByLabelText('Transmissões por página'),{target:{value:'25'}});
 const table=screen.getByRole('table');
 expect(within(table).getAllByRole('checkbox')).toHaveLength(26);
 fireEvent.click(screen.getByRole('button',{name:'Próxima página'}));
 expect(within(table).getAllByRole('checkbox')).toHaveLength(2);
 fireEvent.change(screen.getByLabelText('Filtrar transmissões'),{target:{value:'Canal 26'}});
 expect(within(table).getAllByRole('checkbox')).toHaveLength(2);
 fireEvent.click(screen.getByRole('button',{name:'Visualização em cartões'}));
 expect(screen.queryByRole('table')).toBeNull();
 expect(screen.getByRole('button',{name:'Canal 26'})).toBeTruthy();
});
it('keeps operational switches and registration disabled for viewers',async()=>{
 const f=fixture('viewer');
 const toggle=await screen.findByRole('switch',{name:'Iniciar Canal 1'});
 expect((toggle as HTMLButtonElement).disabled).toBe(true);
 expect((screen.getByRole('button',{name:'Adicionar transmissão'}) as HTMLButtonElement).disabled).toBe(true);
 fireEvent.click(toggle);
 expect(f.request.mock.calls.every(c=>c[0]==='GET')).toBe(true);
});
it('reconnects only the chosen transmission using stop then start',async()=>{
 const f=fixture('admin',2,true);
 fireEvent.click(await screen.findByRole('button',{name:'Reconectar Canal 1'}));
 await waitFor(()=>expect(f.request.mock.calls.filter(c=>c[0]==='POST')).toHaveLength(2));
 expect(f.request.mock.calls.filter(c=>c[0]==='POST').map(c=>[c[2]?.channel_id,c[2]?.action,c[2]?.generation])).toEqual([['synthetic-0','stop',4],['synthetic-0','start',5]]);
});
it('closes channel details with Escape and returns focus without operating the channel',async()=>{
 const f=fixture();
 const name=await screen.findByRole('button',{name:'Canal 1'});
 name.focus();fireEvent.click(name);
 expect(screen.getByRole('dialog',{name:'Canal 1'})).toBeTruthy();
 fireEvent.keyDown(document,{key:'Escape'});
 expect(screen.queryByRole('dialog')).toBeNull();
 expect(document.activeElement).toBe(name);
 expect(f.request.mock.calls.every(c=>c[0]==='GET')).toBe(true);
});
it('halts a bulk request on a stale revision and reports the partial result',async()=>{
 const f=fixture('admin',3);
 await screen.findByRole('button',{name:'Canal 1'});
 const original=f.request.getMockImplementation()!;
 let posts=0;
 f.request.mockImplementation(async(method,path,body)=>{
  if(method==='POST'&&++posts===2)return new Response(JSON.stringify({error:'Revisão sintética alterada'}),{status:409});
  return original(method,path,body);
 });
 fireEvent.click(screen.getByLabelText('Selecionar transmissões desta página'));
 fireEvent.click(screen.getByRole('button',{name:'Iniciar selecionadas'}));
 await screen.findByText(/1 de 3 operação.*Revisão sintética alterada/);
 expect(f.request.mock.calls.filter(c=>c[0]==='POST')).toHaveLength(2);
});
