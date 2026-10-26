import { useEffect, useRef, useState } from 'react';
import { useAccount } from './account-context';

export type LiveStatus={stage:string;applied:boolean;handshake_recent:boolean;can_rollback:boolean;checked_at?:string;observation_available?:boolean;error?:string;peers?:{public_key:string;last_handshake:number;online:boolean}[]};
export function useWireGuardLive(enabled=true){
 const account=useAccount(),[data,setData]=useState<LiveStatus|null>(null),[checking,setChecking]=useState(false),[checked,setChecked]=useState('');
 const requestRef=useRef(account?.request);requestRef.current=account?.request;
 const lock=useRef(false),mounted=useRef(true);const [now,setNow]=useState(Date.now());
 async function refresh(){
  if(!enabled||lock.current||!requestRef.current)return;
  lock.current=true;setChecking(true);
  try{const response=await requestRef.current('GET','/api/v1/wireguard/setup/connection/status');if(!response.ok)throw new Error();const value=await response.json();if(typeof value.stage!=='string')throw new Error();if(mounted.current){setData(value);setChecked(new Date().toISOString())}}
  catch{if(mounted.current){setData(null);setChecked(new Date().toISOString())}}
  finally{lock.current=false;if(mounted.current)setChecking(false)}
 }
 useEffect(()=>{mounted.current=true;if(!enabled)return;void refresh();const timer=setInterval(()=>{setNow(Date.now());void refresh()},5000);return()=>{mounted.current=false;clearInterval(timer)}},[enabled]);
 return {data:checked&&now-new Date(checked).getTime()>15000?null:data,checking,checked,refresh,update:setData};
}
export type LiveSource=ReturnType<typeof useWireGuardLive>;
export function connectionLabel(data:LiveStatus|null){
 if(!data)return {label:'Estado indisponível',kind:'unknown',detail:'Não foi possível consultar o servidor. A confirmação anterior não é considerada atual.'};
 if(data.stage==='applying')return {label:'Conectando…',kind:'waiting',detail:'Aguarde a aplicação terminar. O estado é atualizado automaticamente.'};
 if(data.stage==='rolling_back')return {label:'Desfazendo configuração…',kind:'waiting',detail:'Aguarde o término da operação.'};
 if(['failed','recovery_required'].includes(data.stage))return {label:'Falha na conexão',kind:'error',detail:'A operação não foi concluída. Abra os detalhes para verificar a falha.'};
 if(data.applied&&data.observation_available===false)return {label:'Estado indisponível',kind:'unknown',detail:'A conexão está aplicada, mas não foi possível consultar a resposta do outro servidor.'};
 if(data.applied&&data.handshake_recent)return {label:'Online · conectado ao Kore Play',kind:'online',detail:'Resposta real recente do WireGuard confirmada. A configuração foi concluída.'};
 if(data.applied)return {label:'Sem resposta do Kore Play',kind:'waiting',detail:'A configuração está aplicada, mas não há resposta recente. Verifique a conexão da central e sua porta UDP.'};
 return {label:'Offline · conexão não aplicada',kind:'offline',detail:data.stage==='configuration_changed'?'O cadastro foi alterado e precisa ser aplicado novamente.':'A configuração ainda não está ativa neste servidor.'};
}
export function WireGuardLiveCard({live}:{live:LiveSource}){
 const status=connectionLabel(live.data);
 const last=live.data?.peers?.[0]?.last_handshake;
 return <section className={`wg-live wg-live-${status.kind}`} aria-label="Estado da conexão WireGuard">
  <h3 role="status" aria-live="polite">{status.label}</h3><p>{status.detail}</p>
  {last!==undefined&&<p>Última resposta do Kore Play: {last>0?new Date(last*1000).toLocaleString('pt-BR'):'Ainda não recebida'}</p>}
  <p>Última verificação: {live.checked?new Date(live.checked).toLocaleTimeString('pt-BR'):'Consultando…'} · atualização automática a cada 5 segundos</p>
  <button type="button" className="button" disabled={live.checking} onClick={()=>void live.refresh()}>{live.checking?'Verificando…':'Atualizar estado'}</button>
 </section>;
}
