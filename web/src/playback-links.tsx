import {useEffect,useRef,useState} from 'react';
export type PlaybackLink={channel_id:string;ts_path:string;hls_path?:string;expires_at:string};
type Call=(method:string,path:string,body?:unknown)=>Promise<any>;

export function PlaybackLinks({channelID,admin,hlsState,call}:{channelID:string;admin:boolean;hlsState?:string;call:Call}){
 const [link,setLink]=useState<PlaybackLink|null>(null),[busy,setBusy]=useState(false),[message,setMessage]=useState('');
 const alive=useRef(true),lock=useRef(false),ts=useRef<HTMLInputElement>(null),hls=useRef<HTMLInputElement>(null);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false}},[]);
 async function act(action:'issue'|'revoke'){
  if(lock.current)return;lock.current=true;setBusy(true);setMessage('');
  try{const value=await call('POST','live/playback-links',{action,channel_ids:[channelID]});if(!alive.current)return;setLink(action==='issue'?value.links[0]:null);if(action==='revoke')setMessage('Links anteriores deste canal revogados. Gere novos links para voltar a usar no VLC.')}
  catch(e){if(alive.current)setMessage((e as Error).message)}finally{lock.current=false;if(alive.current)setBusy(false)}
 }
 async function copy(input:HTMLInputElement|null){if(!input)return;input.focus();input.select();try{if(!navigator.clipboard)throw new Error();await navigator.clipboard.writeText(input.value);setMessage('Link copiado. No VLC, use Mídia → Abrir fluxo de rede (Ctrl+N).')}catch{setMessage('Link selecionado. Pressione Ctrl+C para copiar e cole no VLC com Ctrl+N.')}}
 const absolute=(path:string)=>window.location.origin+path;
 return <section className="dist-output-links" aria-label="Links para VLC"><h3>Reproduzir no VLC</h3><p>Gere um link exclusivo de reprodução. Não precisa estar logado no navegador para usá-lo no VLC.</p><div className="dist-actions"><button className="button primary" disabled={!admin||busy} onClick={()=>void act('issue')}>{busy?'Aguarde...':'Gerar links para VLC'}</button><button className="button" disabled={!admin||busy} onClick={()=>void act('revoke')}>Revogar links deste canal</button></div>{link&&<><label>Link MPEG-TS para VLC<input ref={ts} readOnly autoComplete="off" value={absolute(link.ts_path)} onFocus={e=>e.target.select()}/></label><button className="button" onClick={()=>void copy(ts.current)}>Copiar link TS</button>{link.hls_path?<><label>Link HLS para VLC<input ref={hls} readOnly autoComplete="off" value={absolute(link.hls_path)} onFocus={e=>e.target.select()}/></label><button className="button" onClick={()=>void copy(hls.current)}>Copiar link HLS</button>{hlsState!=='running'&&<p>HLS ainda não está pronto. Use MPEG-TS enquanto a saída HLS é preparada.</p>}</>:<p>HLS não está configurado ou autorizado. Use o link MPEG-TS.</p>}<small>Válidos até {new Date(link.expires_at).toLocaleString('pt-BR')}. Trate estes links como credenciais de reprodução.</small></>}<p>O VLC precisa confiar no certificado HTTPS deste Edge. O canal deve estar recebendo; gerar um link não inicia outra origem.</p>{message&&<p role="status">{message}</p>}</section>;
}
