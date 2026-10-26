// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { AccountContext } from './account-context';
import { WireGuardSetup } from './wireguard-setup';
afterEach(cleanup);
afterEach(()=>vi.restoreAllMocks());
const user={id:'a'.repeat(32),username:'owner',role:'admin' as const,revision:1,disabled:false};
it('starts with one action and guides the operator between the two panels without exposing manual fields',async()=>{
 const initial={state:'unconfigured',public_key:'',settings:null,revision:0,applied:false,kore_play_peer:''};
 const request=vi.fn(async(method:string)=>new Response(JSON.stringify(method==='POST'?{...initial,public_key:'synthetic-public',revision:1}:initial)));
 render(<AccountContext.Provider value={{user,request,reload:vi.fn(),logout:vi.fn()}}><WireGuardSetup/></AccountContext.Provider>);
 await screen.findByRole('button',{name:'Iniciar configuração'});
 fireEvent.click(screen.getByRole('button',{name:'Iniciar configuração'}));
 const button=await screen.findByRole('button',{name:'Preparar este Edge'});
 expect((button as HTMLButtonElement).disabled).toBe(true);
 expect(screen.queryByLabelText('IP do Edge no túnel (CIDR /32 ou /128)')).toBeNull();
 fireEvent.change(screen.getByLabelText('Senha deste painel Edge'),{target:{value:'synthetic-password'}});fireEvent.click(screen.getByRole('checkbox'));fireEvent.click(button);
 await screen.findByRole('button',{name:'Copiar identificação do Edge'});
 const identification=JSON.parse((screen.getByLabelText('Identificação deste Edge') as HTMLTextAreaElement).value);
 expect(identification).toEqual({version:1,type:'kore-edge-identification',public_key:'synthetic-public'});
 fireEvent.click(screen.getByRole('button',{name:'Já tenho o código do Kore Play'}));
 await screen.findByLabelText('Cole o código recebido do Kore Play');
 expect(request.mock.calls.filter(c=>c[0]==='POST')).toHaveLength(1);
});

it('opens on an online overview, returns home and continues polling after success',async()=>{
 const intervals:(()=>void)[]=[];
 vi.spyOn(globalThis,'setInterval').mockImplementation(callback=>{intervals.push(callback as ()=>void);return 1 as unknown as ReturnType<typeof setInterval>});
 vi.spyOn(globalThis,'clearInterval').mockImplementation(()=>{});
 let fail=false,online=true;
 const request=vi.fn(async(method:string,path:string)=>{
  if(path.endsWith('/status')){if(fail)return new Response('{}',{status:503});return new Response(JSON.stringify({stage:online?'handshake_recent':'applied_waiting_handshake',applied:true,handshake_recent:online,observation_available:true,can_rollback:true,peers:[{public_key:'matrix',last_handshake:online?Math.floor(Date.now()/1000):1,online}]}))}
  return new Response(JSON.stringify({state:'peer_prepared',public_key:'synthetic',revision:2,kore_play_peer:'',settings:{interface:'wg-test',address:'10.77.0.2/32',listen_port:51821,keepalive_seconds:25,allowed_ips:['10.77.0.1/32'],kore_play_endpoint:'matrix.example:51820'}}));
 });
 await act(async()=>{render(<AccountContext.Provider value={{user,request,reload:vi.fn(),logout:vi.fn()}}><WireGuardSetup/></AccountContext.Provider>)});
 expect(screen.getByText('Online · conectado ao Kore Play')).toBeTruthy();
 expect((screen.getByRole('button',{name:'Configuração aplicada ✓'}) as HTMLButtonElement).disabled).toBe(true);
 fireEvent.click(screen.getByRole('button',{name:'Ver detalhes da conexão'}));
 expect(screen.queryByRole('button',{name:'Confirmar e ativar conexão'})).toBeNull();
 fireEvent.click(screen.getByRole('button',{name:'Voltar à tela principal do WireGuard'}));
 expect(screen.getByRole('button',{name:'Ver detalhes da conexão'})).toBeTruthy();
 online=false;
 await act(async()=>{for(const callback of intervals)callback()});
 expect(screen.getByText('Sem resposta do Kore Play')).toBeTruthy();
 fail=true;
 await act(async()=>{for(const callback of intervals)callback()});
 expect(screen.getByText('Estado indisponível')).toBeTruthy();
 expect(screen.queryByText('Online · conectado ao Kore Play')).toBeNull();
 expect(request.mock.calls.every(call=>call[0]==='GET')).toBe(true);
});
