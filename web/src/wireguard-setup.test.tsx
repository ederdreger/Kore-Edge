// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { WireGuardSetup } from './wireguard-setup';
import { AccountContext } from './account-context';
import type { PanelUser } from './account-context';
afterEach(cleanup);
const admin:PanelUser={id:'a'.repeat(32),username:'owner',role:'admin',disabled:false,revision:1};
it('requires confirmation and password to generate identity without invented network settings',async()=>{
 const clean={state:'unconfigured',applied:false,public_key:'',revision:0,settings:null,kore_play_peer:''};
 const request=vi.fn(async(method:string,_path:string,_body?:unknown)=>new Response(JSON.stringify(method==='GET'?clean:{...clean,state:'credentials_generated',revision:1,public_key:'synthetic-public-key'})));
 render(<AccountContext.Provider value={{user:admin,request,reload:vi.fn(),logout:vi.fn()}}><WireGuardSetup/></AccountContext.Provider>);
 const generate=await screen.findByRole('button',{name:'Gerar credenciais WireGuard'});expect((generate as HTMLButtonElement).disabled).toBe(true);
 fireEvent.change(screen.getByLabelText('Sua senha atual'),{target:{value:'synthetic-password'}});
 fireEvent.click(screen.getByRole('checkbox'));fireEvent.click(generate);
 await screen.findByDisplayValue('synthetic-public-key');
 expect(request.mock.calls.find(c=>c[0]==='POST')).toEqual(['POST','/api/v1/wireguard/setup/identity',{current_password:'synthetic-password',confirmed:true}]);
 for(const name of ['Interface WireGuard do Edge','IP do Edge no túnel (CIDR /32 ou /128)','Porta UDP de escuta no Edge','Endpoint WireGuard do Kore Play'])expect((screen.getByLabelText(name) as HTMLInputElement).value).toBe('');
 expect((screen.getByLabelText('Sua senha atual') as HTMLInputElement).value).toBe('');
});
it('submits peer settings and shows only public export',async()=>{
 const state={state:'credentials_generated',applied:false,public_key:'edge-public',revision:1,settings:null,kore_play_peer:''};
 const request=vi.fn(async(method:string,_path:string,_body?:unknown)=>new Response(JSON.stringify(method==='GET'?state:{...state,revision:2,state:'peer_prepared',kore_play_peer:'[Peer]\nPublicKey = edge-public\nAllowedIPs = 10.65.0.2/32\n'})));
 render(<AccountContext.Provider value={{user:admin,request,reload:vi.fn(),logout:vi.fn()}}><WireGuardSetup/></AccountContext.Provider>);
 await screen.findByDisplayValue('edge-public');
 for(const [name,value] of [['Interface WireGuard do Edge','wg-edge'],['IP do Edge no túnel (CIDR /32 ou /128)','10.65.0.2/32'],['Porta UDP de escuta no Edge','51821'],['Chave pública WireGuard do Kore Play','matrix-public'],['Endpoint WireGuard do Kore Play','matrix.example:51820'],['Rotas do Kore Play (AllowedIPs)','10.65.0.1/32'],['PersistentKeepalive em segundos (0 desativa)','25'],['Sua senha atual','synthetic-password']])fireEvent.change(screen.getByLabelText(name),{target:{value}});
 fireEvent.click(screen.getByRole('checkbox'));fireEvent.click(screen.getByRole('button',{name:'Salvar dados do peer'}));
 await screen.findByLabelText('Peer público para Kore Play');
 const sent=request.mock.calls.find(c=>c[0]==='PUT');expect(sent?.[1]).toBe('/api/v1/wireguard/setup/peer');expect(sent?.[2]).toMatchObject({confirmed:true,revision:1,settings:{address:'10.65.0.2/32',listen_port:51821,allowed_ips:['10.65.0.1/32'],keepalive_seconds:25}});
 expect(screen.queryByText(/PrivateKey/)).toBeNull();
});
it('does not request setup for a read-only user',async()=>{
 const request=vi.fn();render(<AccountContext.Provider value={{user:{...admin,role:'viewer'},request,reload:vi.fn(),logout:vi.fn()}}><WireGuardSetup/></AccountContext.Provider>);
 await waitFor(()=>expect(request).not.toHaveBeenCalled());expect(screen.queryByRole('button',{name:'Gerar credenciais WireGuard'})).toBeNull();
});
