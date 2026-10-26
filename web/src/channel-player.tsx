import {useEffect,useRef,useState} from 'react';
import {Play,X,RotateCw} from 'lucide-react';
import {startPreview,type PreviewSource,type PreviewStatus} from './preview-engine';
import './channel-player.css';

export function ChannelPlayer({source,onClose}:{source:PreviewSource;onClose:()=>void}){
 const video=useRef<HTMLVideoElement>(null),close=useRef<HTMLButtonElement>(null);
 const [attempt,setAttempt]=useState(0),[status,setStatus]=useState<PreviewStatus>({kind:'loading',text:'Carregando…'});
 useEffect(()=>{
  const previous=document.activeElement;
  close.current?.focus();
  const escape=(event:KeyboardEvent)=>{if(event.key==='Escape')onClose()};document.addEventListener('keydown',escape);
  return()=>{document.removeEventListener('keydown',escape);if(previous instanceof HTMLElement&&previous.isConnected)previous.focus()};
 },[onClose]);
 useEffect(()=>startPreview(video.current!,source,setStatus),[source,attempt]);
 const ended=['paused','error','ended'].includes(status.kind);
 return <aside className="channel-player" role="dialog" aria-label={`Mini player: ${source.name}`}>
  <header><div><strong><Play size={15}/> {source.name}</strong><small>{source.protocol==='hls'?'HLS':'HTTP MPEG-TS'} · prévia de até 5 minutos</small></div><button ref={close} className="icon-button" aria-label="Fechar mini player e parar transmissão" onClick={onClose}><X size={20}/></button></header>
  <video ref={video} controls playsInline muted aria-label={`Vídeo de ${source.name}`}/>
  <p role="status" data-state={status.kind}>{status.text}</p>
  <footer>{ended&&<button className="button primary" onClick={()=>setAttempt(n=>n+1)}><RotateCw size={14}/>Reproduzir novamente</button>}<button className="button" onClick={onClose}>Parar e fechar</button><small>Áudio inicia silenciado. Ative-o nos controles do vídeo. Pausar encerra a conexão da prévia.</small></footer>
 </aside>;
}
