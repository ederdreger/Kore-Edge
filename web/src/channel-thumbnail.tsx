import {useEffect,useRef,useState} from 'react';
import {Play,Tv} from 'lucide-react';

// Bound browser requests as well as server-side decoders. Queued cards that
// leave the viewport are skipped before starting another request.
const pending:Array<()=>Promise<void>>=[];
let active=0;
function drain(){while(active<2&&pending.length){const next=pending.shift()!;active++;void next().finally(()=>{active--;drain()})}}
function enqueue(task:()=>Promise<void>){pending.push(task);drain()}
function dataURL(blob:Blob){return new Promise<string>((resolve,reject)=>{const reader=new FileReader();reader.onload=()=>resolve(String(reader.result));reader.onerror=()=>reject(new Error('thumbnail_read'));reader.readAsDataURL(blob)})}

export function ChannelThumbnail({id,name,online,onPlay}:{id:string;name:string;online:boolean;onPlay:()=>void}){
 const ref=useRef<HTMLDivElement>(null);
 const [visible,setVisible]=useState(typeof IntersectionObserver==='undefined'),[image,setImage]=useState(''),[message,setMessage]=useState('Preparando prévia');
 useEffect(()=>{
  if(typeof IntersectionObserver==='undefined')return;
  const observer=new IntersectionObserver(entries=>setVisible(entries.some(entry=>entry.isIntersecting)),{rootMargin:'100px'});
  if(ref.current)observer.observe(ref.current);return()=>observer.disconnect();
 },[]);
 useEffect(()=>{
  let disposed=false,timer:ReturnType<typeof setTimeout>|undefined,controller:AbortController|undefined,queued=false;
  if(!online){setImage('');setMessage('Canal sem recepção');return}
  const load=()=>{
   if(disposed||!visible||document.visibilityState==='hidden'||queued)return;
   queued=true;controller=new AbortController();const current=controller,signal=current.signal;
   enqueue(async()=>{
    let retry=true,timedOut=false,deadline:ReturnType<typeof setTimeout>|undefined;
    try{
     if(signal.aborted||disposed)return;
     deadline=setTimeout(()=>{timedOut=true;current.abort()},6500);
     if(!/^[a-zA-Z0-9_-]{1,80}$/.test(id))throw new Error('thumbnail_id');
     const response=await fetch(`/api/v1/live/media/${id}/preview.jpg`,{credentials:'same-origin',cache:'no-store',redirect:'error',signal});
     if(response.status===401||response.status===403){retry=false;throw new Error('thumbnail_denied')}
     if(!response.ok||!response.headers.get('Content-Type')?.startsWith('image/jpeg'))throw new Error('thumbnail_unavailable');
     const blob=await response.blob();if(blob.size>128*1024)throw new Error('thumbnail_size');
     const url=await dataURL(blob);if(!disposed&&!signal.aborted){setImage(url);setMessage('Prévia da saída · atualização a cada 20 s')}
    }catch{if(!disposed&&(!signal.aborted||timedOut)){setImage('');setMessage('Prévia indisponível')}}
    finally{clearTimeout(deadline);queued=false;if(!disposed&&(!signal.aborted||timedOut)&&retry)timer=setTimeout(load,20000)}
   });
  };
  const visibility=()=>{if(document.visibilityState==='hidden'){controller?.abort();clearTimeout(timer)}else{queued=false;load()}};
  document.addEventListener('visibilitychange',visibility);load();
  return()=>{disposed=true;controller?.abort();clearTimeout(timer);document.removeEventListener('visibilitychange',visibility)};
 },[id,online,visible]);
 return <div ref={ref} className="channel-thumbnail" title={message}>
  {image&&online?<img decoding="async" src={image} alt={`Prévia da saída de ${name}`}/>:<div className="channel-thumbnail-empty"><Tv size={30}/><span>{online?message:'Canal sem recepção'}</span></div>}
  <button type="button" className="channel-thumbnail-play" aria-label={`Abrir player de ${name}`} disabled={!online} onClick={onPlay}><Play size={22} fill="currentColor"/></button>
  <small className="channel-thumbnail-caption">{image&&online?'Prévia da saída':online?'Prévia indisponível':'Sem recepção'}</small>
 </div>;
}
