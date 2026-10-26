import { useEffect, useRef, useState } from 'react';
import { useAccount } from './account-context';
import { WireGuardAdvanced } from './wireguard-setup';
import type { Status } from './wireguard-setup';
import { WireGuardConnection } from './wireguard-connection';
import { copyPublicText } from './public-copy';
import './wireguard-guide.css';

export function WireGuardGuided() {
 const account=useAccount();
 const [state,setState]=useState<Status|null>(null),[error,setError]=useState(''),[busy,setBusy]=useState(false);
 const [password,setPassword]=useState(''),[confirmed,setConfirmed]=useState(false),[advanced,setAdvanced]=useState(false);
 const [active,setActive]=useState(false);
 const [copied,setCopied]=useState(''),[connecting,setConnecting]=useState(false);
 const field=useRef<HTMLTextAreaElement>(null);
 const [local,setLocal]=useState({interface:'wg-edge',listen_port:51820,keepalive_seconds:25});
 async function load(){
  if(account?.user?.role!=='admin')return;
  try{const r=await account.request('GET','/api/v1/wireguard/setup');if(!r.ok)throw new Error();const value:Status=await r.json();accept(value);setError('');if(value.public_key){try{const status=await account.request('GET','/api/v1/wireguard/setup/connection/status');if(status.ok){const live=await status.json();setActive(live.applied===true);if(live.applied)setConnecting(true)}}catch{/* connection status is optional here */}}}
  catch{setError('Não conseguimos carregar este Edge. Clique em Tentar novamente. Se continuar, verifique se o painel foi atualizado.')}
 }
 function accept(value:Status){setState(value);if(value.settings)setLocal({interface:value.settings.interface,listen_port:value.settings.listen_port,keepalive_seconds:value.settings.keepalive_seconds})}
 useEffect(()=>{void load()},[]);
 if(account?.user?.role!=='admin')return <section className="panel content-panel"><h2>Conectar ao Kore Play</h2><p>Entre com uma conta de administrador para configurar a conexão.</p></section>;
 async function generate(e:React.FormEvent){e.preventDefault();if(!account||busy||!confirmed)return;setBusy(true);setError('');try{
  const r=await account.request('POST','/api/v1/wireguard/setup/identity',{current_password:password,confirmed:true});
  if(!r.ok)throw new Error(r.status===403?'Senha não aceita. Use a senha com que você entra neste painel Edge.':'Não foi possível preparar a identificação. Tente novamente.');
  accept(await r.json());
 }catch(e){setError(e instanceof Error?e.message:'Não foi possível preparar este Edge.')}finally{setBusy(false);setPassword('');setConfirmed(false)}}
 const identification=state?.public_key?JSON.stringify({version:1,type:'kore-edge-identification',public_key:state.public_key}):'';
 async function copy(){setBusy(true);try{const result=await copyPublicText(identification,field.current);setCopied(result==='copied'?'Identificação copiada. Agora abra o Kore Play e cole em Adicionar Edge.':'O navegador bloqueou a cópia. O texto está selecionado: pressione Ctrl+C.')}finally{setBusy(false)}}
 return <section className="panel user-manager wireguard-setup wg-guide">
  <h2>Conectar este Edge ao Kore Play</h2>
  <p>Comece aqui. Você vai copiar apenas dois códigos entre os painéis. Não precisa copiar chaves, rotas ou arquivos de configuração.</p>
  <ol className="wg-steps" aria-label="Passo a passo da integração"><li aria-current={!state?.public_key?'step':undefined}>1. Prepare este Edge</li><li aria-current={state?.public_key&&!connecting?'step':undefined}>2. Cadastre no Kore Play</li><li aria-current={connecting?'step':undefined}>3. Volte aqui para conectar</li></ol>
  {error&&<p role="alert">{error}</p>}
  {!state?<><p>Carregando a configuração…</p><button className="button" onClick={()=>void load()}>Tentar novamente</button></>:<>
  {!state.public_key?<form className="user-form" onSubmit={e=>void generate(e)}>
   <h3>1. Preparar a identificação</h3><p>Isso identifica este servidor no Kore Play. Não liga a conexão nem altera a rede.</p>
   <label>Senha deste painel Edge<input type="password" required autoComplete="current-password" value={password} onChange={e=>setPassword(e.target.value)}/></label>
   <label className="wg-confirm"><input type="checkbox" checked={confirmed} onChange={e=>setConfirmed(e.target.checked)}/>Quero preparar este Edge para conexão.</label>
   <button className="button primary" disabled={busy||!confirmed}>{busy?'Preparando…':'Preparar este Edge'}</button>
  </form>:<>
   {!connecting&&<div className="wg-public"><h3>2. Levar a identificação ao Kore Play</h3>
    <p>Abra o <strong>Kore Play → Canais → WireGuard Kore Edge</strong>. Escolha <strong>Adicionar Edge</strong> e cole a identificação abaixo. Dê um nome ao servidor e siga o botão Continuar.</p>
    <label>Identificação deste Edge<textarea ref={field} rows={3} readOnly value={identification}/></label>
    <button className="button primary" disabled={busy} onClick={()=>void copy()}>Copiar identificação do Edge</button><p role="status" aria-live="polite">{copied}</p>
    <h3>3. Voltar com o código de conexão</h3><p>Quando o Kore Play mostrar o código de conexão, copie-o e volte a esta tela.</p>
    <button className="button primary" onClick={()=>setConnecting(true)}>Já tenho o código do Kore Play</button>
    
   </div>}
   {connecting&&<>
    <p>{active?'A conexão já foi aplicada. Acompanhe abaixo se o Kore Play respondeu.':'Cole o código recebido do Kore Play e use a senha deste painel. Depois confira o resumo e confirme a conexão.'}</p>
    <details><summary>Opções deste servidor (normalmente não é preciso alterar)</summary><p>Os valores abaixo são sugestões quando não há cadastro anterior. Eles serão conferidos no plano antes da sua confirmação.</p>
     <label>Nome da conexão<input value={local.interface} onChange={e=>setLocal({...local,interface:e.target.value})}/></label>
     <label>Porta de conexão deste Edge<input type="number" min={1} max={65535} value={local.listen_port} onChange={e=>setLocal({...local,listen_port:Number(e.target.value)})}/></label>
    </details>
    <WireGuardConnection setup={state} local={local} onSync={accept} guided/>
    <button className="button" onClick={()=>setConnecting(false)}>Voltar à identificação</button>
   </>}
  </>}
  </>}
  <details open={advanced} onToggle={e=>{const next=e.currentTarget.open;setAdvanced(next);if(!next)void load()}}><summary>Configuração avançada / cadastro manual</summary>{advanced&&<WireGuardAdvanced/>}</details>
 </section>;
}
