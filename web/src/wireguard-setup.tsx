import { useEffect, useRef, useState } from 'react';
import type { FormEvent } from 'react';
import { useAccount } from './account-context';
import { copyPublicText } from './public-copy';
import { WireGuardConnection } from './wireguard-connection';

type Settings={interface:string;address:string;listen_port:number;edge_endpoint:string;kore_play_public_key:string;kore_play_endpoint:string;allowed_ips:string[];keepalive_seconds:number};
type Status={state:string;applied:boolean;public_key:string;revision:number;settings:Settings|null;kore_play_peer:string};
export function WireGuardSetup(){
 const keyField=useRef<HTMLTextAreaElement>(null),peerField=useRef<HTMLTextAreaElement>(null);const[copyStatus,setCopyStatus]=useState('');const[copyBusy,setCopyBusy]=useState(false);
 const account=useAccount();const[status,setStatus]=useState<Status|null>(null);const[busy,setBusy]=useState(false);const[message,setMessage]=useState('');
 const[password,setPassword]=useState('');const[confirmed,setConfirmed]=useState(false);
 const[iface,setIface]=useState('');const[address,setAddress]=useState('');const[port,setPort]=useState('');const[endpoint,setEndpoint]=useState('');
 const[peerKey,setPeerKey]=useState('');const[peerEndpoint,setPeerEndpoint]=useState('');const[routes,setRoutes]=useState('');const[keepalive,setKeepalive]=useState('');
 function accept(value:Status){setStatus(value);if(value.settings){const s=value.settings;setIface(s.interface);setAddress(s.address);setPort(String(s.listen_port));setEndpoint(s.edge_endpoint);setPeerKey(s.kore_play_public_key);setPeerEndpoint(s.kore_play_endpoint);setRoutes(s.allowed_ips.join('\n'));setKeepalive(String(s.keepalive_seconds))}}
 async function load(){if(account?.user?.role!=='admin')return;try{const r=await account.request('GET','/api/v1/wireguard/setup');if(!r.ok)throw new Error();accept(await r.json())}catch{setMessage('Não foi possível carregar a configuração WireGuard. Confira se o servidor foi atualizado.')}}
 useEffect(()=>{void load()},[]);
 if(account?.user?.role!=='admin')return <section className="panel content-panel"><h2>Configuração WireGuard</h2><p>Somente administradores podem preparar as credenciais de integração.</p></section>;
 async function save(e:FormEvent){e.preventDefault();if(!account||busy||!confirmed||!status)return;setBusy(true);setMessage('');
  const generating=!status.public_key;
  try{const settings:Settings={interface:iface,address,listen_port:Number(port),edge_endpoint:endpoint,kore_play_public_key:peerKey,kore_play_endpoint:peerEndpoint,allowed_ips:routes.split(/[\n,]+/).map(s=>s.trim()).filter(Boolean),keepalive_seconds:Number(keepalive)};
   const r=await account.request(generating?'POST':'PUT',generating?'/api/v1/wireguard/setup/identity':'/api/v1/wireguard/setup/peer',generating?{current_password:password,confirmed}:{current_password:password,confirmed,revision:status.revision,settings});
   setPassword('');setConfirmed(false);
   if(!r.ok){setMessage(r.status===409?'A configuração foi alterada em outra sessão. Atualize os dados antes de salvar.':r.status===403?'Operação recusada. Confira sua senha atual e aguarde se excedeu o limite de tentativas.':'Não foi possível salvar. Confira IPs, portas, chave pública e rotas privadas sem sobreposição.');return}
   accept(await r.json());setMessage(generating?'Chaves geradas. Copie a chave pública e complete os dados do peer.':'Peer preparado. Copie o bloco público para cadastrar no Kore Play. O túnel ainda não foi aplicado.');
  }catch{setPassword('');setMessage('Não foi possível concluir. Atualize os dados para conferir o estado salvo.')}finally{setBusy(false)}
 }
 async function copy(text:string,field:HTMLTextAreaElement|null){setCopyBusy(true);setCopyStatus('Copiando dados públicos…');const result=await copyPublicText(text,field);setCopyStatus(result==='copied'?'Copiado! Cole no Kore Play.':'A cópia automática foi bloqueada. O texto está selecionado: pressione Ctrl+C ou use o menu Copiar do dispositivo.');setCopyBusy(false)}
 return <section className="panel user-manager wireguard-setup"><div className="section-title"><h2>Integração WireGuard com Kore Play</h2><button className="button" disabled={busy} onClick={()=>void load()}>Atualizar configuração</button></div>
 <p>Prepare a identidade do Edge e os dados do peer para cadastro no Kore Play. A chave privada permanece protegida no servidor.</p>
 <p><strong>{!status?'Carregando configuração…':status.state==='unconfigured'?'Identidade ainda não gerada':status.state==='credentials_generated'?'Chaves geradas · aguardando dados do peer':'Peer preparado · túnel não aplicado'}</strong></p>
 {message&&<p role="status">{message}</p>}
 {status?.public_key&&<div className="wg-public"><label>Chave pública do Edge<textarea ref={keyField} readOnly rows={2} value={status.public_key}/></label><button type="button" className="button" disabled={copyBusy} onClick={()=>void copy(status.public_key,keyField.current)}>Copiar chave pública</button><p role="status" aria-live="polite">{copyStatus}</p></div>}
 {status&&<form className="user-form" onSubmit={e=>void save(e)}>
 {status.public_key?<><h3>Dados confirmados do túnel</h3><label>Interface WireGuard do Edge<input required maxLength={15} value={iface} onChange={e=>setIface(e.target.value)} placeholder="Informe o nome confirmado"/></label>
 <label>IP do Edge no túnel (CIDR /32 ou /128)<input required value={address} onChange={e=>setAddress(e.target.value)} placeholder="Informe o IP atribuído pelo Kore Play"/></label>
 <label>Porta UDP de escuta no Edge<input required type="number" min={1} max={65535} value={port} onChange={e=>setPort(e.target.value)}/></label>
 <label>Endpoint público do Edge (opcional)<input value={endpoint} maxLength={300} onChange={e=>setEndpoint(e.target.value)} placeholder="domínio-ou-IP:porta UDP"/></label><small>Se o Edge inicia por NAT, pode ficar vazio no cadastro do Kore Play.</small>
 <h3>Peer do Kore Play</h3><label>Chave pública WireGuard do Kore Play<input required value={peerKey} maxLength={44} onChange={e=>setPeerKey(e.target.value)} autoComplete="off"/></label>
 <label>Endpoint WireGuard do Kore Play<input required value={peerEndpoint} maxLength={300} onChange={e=>setPeerEndpoint(e.target.value)} placeholder="domínio-ou-IP:porta UDP"/></label>
 <label>Rotas do Kore Play (AllowedIPs)<textarea required rows={4} value={routes} onChange={e=>setRoutes(e.target.value)} placeholder="Uma rede privada canônica por linha"/></label>
 <label>PersistentKeepalive em segundos (0 desativa)<input required type="number" min={0} max={65535} value={keepalive} onChange={e=>setKeepalive(e.target.value)}/></label></>:<h3>Gerar identidade do Edge</h3>}
 <label>Sua senha atual<input required type="password" maxLength={72} autoComplete="current-password" value={password} onChange={e=>setPassword(e.target.value)}/></label>
 <label className="wg-confirm"><input type="checkbox" checked={confirmed} onChange={e=>setConfirmed(e.target.checked)}/>{status.public_key?'Conferi interface, IPs, portas, chave pública e rotas.':'Confirmo a geração da identidade WireGuard deste Edge.'}</label>
 <button className="button primary" disabled={busy||!confirmed}>{busy?'Salvando…':status.public_key?'Salvar dados do peer':'Gerar credenciais WireGuard'}</button>
 </form>}
 {status?.kore_play_peer&&<div className="wg-public"><h3>Cadastrar no Kore Play</h3><p>Este bloco contém somente a chave pública e os dados do peer Edge.</p><textarea ref={peerField} aria-label="Peer público para Kore Play" readOnly rows={6} value={status.kore_play_peer}/><button type="button" className="button" disabled={copyBusy} onClick={()=>void copy(status.kore_play_peer,peerField.current)}>Copiar peer para Kore Play</button><p role="status" aria-live="polite">{copyStatus}</p></div>}
 {status?.public_key&&<WireGuardConnection setup={status} local={{interface:iface,listen_port:Number(port),keepalive_seconds:Number(keepalive)}} onSync={accept}/>}
 <p className="muted">Salvar prepara o cadastro local. Use o assistente de conexão para revisar e autorizar a ativação real.</p>
 </section>;
}
