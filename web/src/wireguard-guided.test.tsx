// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { AccountContext } from './account-context';
import { WireGuardSetup } from './wireguard-setup';
afterEach(cleanup);
const user={id:'a'.repeat(32),username:'owner',role:'admin' as const,revision:1,disabled:false};
it('starts with one action and guides the operator between the two panels without exposing manual fields',async()=>{
 const initial={state:'unconfigured',public_key:'',settings:null,revision:0,applied:false,kore_play_peer:''};
 const request=vi.fn(async(method:string)=>new Response(JSON.stringify(method==='POST'?{...initial,public_key:'synthetic-public',revision:1}:initial)));
 render(<AccountContext.Provider value={{user,request,reload:vi.fn(),logout:vi.fn()}}><WireGuardSetup/></AccountContext.Provider>);
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
