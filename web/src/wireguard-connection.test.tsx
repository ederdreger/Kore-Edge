// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { AccountContext } from './account-context';
import { WireGuardConnection } from './wireguard-connection';

afterEach(cleanup);
it('reviews the public plan and requires password plus confirmation before real application',async()=>{
 const request=vi.fn(async(method:string,path:string,_body?:unknown)=>new Response(JSON.stringify(path.endsWith('/plan')?{digest:'synthetic-digest',config:{interface:'wg-test',address:'10.77.0.2/32',listen_port:51821,peers:[]},routes_added:['10.77.0.1/32'],routes_removed:[]}:{stage:method==='POST'?'applying':'not_applied',applied:false,can_rollback:false})));
 render(<AccountContext.Provider value={{user:{id:'a'.repeat(32),username:'owner',role:'admin',revision:1,disabled:false},request,reload:vi.fn(),logout:vi.fn()}}><WireGuardConnection setup={{public_key:'synthetic',revision:1,settings:{}}} local={{interface:'wg-test',listen_port:51821,keepalive_seconds:25}} onSync={vi.fn()}/></AccountContext.Provider>);
 fireEvent.click(screen.getByRole('button',{name:'Revisar e ativar dados já salvos'}));
 const apply=await screen.findByRole('button',{name:'Confirmar e ativar conexão'});
 expect((apply as HTMLButtonElement).disabled).toBe(true);
 expect(screen.getByText(/"rotas_adicionadas"/).textContent).toContain('10.77.0.1/32');
 expect(request.mock.calls.every(c=>c[0]==='GET')).toBe(true);
 fireEvent.change(screen.getByLabelText('Sua senha atual'),{target:{value:'synthetic-password'}});fireEvent.click(screen.getByRole('checkbox'));fireEvent.click(apply);
 await waitFor(()=>expect(request.mock.calls.find(c=>c[0]==='POST')).toEqual(['POST','/api/v1/wireguard/setup/connection/apply',{digest:'synthetic-digest',current_password:'synthetic-password',confirmed:true}]));
 await screen.findByText('Criando conexão…');
});
