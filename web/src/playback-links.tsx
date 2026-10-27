import {useEffect,useRef,useState} from 'react';
export type PlaybackLink={channel_id:string;ts_path:string;hls_path:string};
type Call=(method:string,path:string,body?:unknown)=>Promise<any>;

export function PlaybackLinks({channelID,admin,hlsState,call}:{channelID:string;admin:boolean;hlsState?:string;call:Call}){
 const [link,setLink]=useState<PlaybackLink|null>(null),[busy,setBusy]=useState(false),[message,setMessage]=useState('');
 const ts=useRef<HTMLInputElement>(null),hls=useRef<HTMLInputElement>(null);
 useEffect(()=>{let cancelled=false;setLink(null);if(!admin){setBusy(false);return()=>{cancelled=true}}setBusy(true);setMessage('');void call('GET','live/playback-links').then(value=>{if(!cancelled)setLink(value.links.find((item:PlaybackLink)=>item.channel_id===channelID)||null)}).catch(e=>{if(!cancelled)setMessage(e.message)}).finally(()=>{if(!cancelled)setBusy(false)});return()=>{cancelled=true}},[channelID,call,admin]);
 async function copy(input:HTMLInputElement|null){if(!input)return;input.focus();input.select();try{if(!navigator.clipboard)throw new Error();await navigator.clipboard.writeText(input.value);setMessage('Link copiado. Cole o endereço no XUI One ou outro receptor.')}catch{setMessage('Link selecionado. Pressione Ctrl+C para copiar.')}}
 const absolute=(path:string)=>window.location.origin+path;
 return <section className="dist-output-links" aria-label="Saídas permanentes"><h3>Saídas do canal</h3><p>Endereços permanentes para XUI One e outros receptores. Permanecem os mesmos até a exclusão do canal.</p>{busy&&<p role="status">Carregando saídas...</p>}{link&&<><label>Link MPEG-TS<input ref={ts} readOnly autoComplete="off" value={absolute(link.ts_path)} onFocus={e=>e.target.select()}/></label><button className="button" onClick={()=>void copy(ts.current)}>Copiar link TS</button><label>Link HLS<input ref={hls} readOnly autoComplete="off" value={absolute(link.hls_path!)} onFocus={e=>e.target.select()}/></label><button className="button" onClick={()=>void copy(hls.current)}>Copiar link HLS</button>{hlsState!=='running'&&<p>Endereço HLS reservado. A saída precisa estar autorizada, configurada e pronta para fornecer segmentos.</p>}</>}<p>Sem vencimento e sem depender da senha do administrador. Os receptores precisam confiar no certificado HTTPS do Edge.</p>{message&&<p role="status">{message}</p>}</section>;
}
