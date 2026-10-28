import {useEffect,useRef,useState} from 'react';
import {createPortal} from 'react-dom';
import {Play,X,RotateCw} from 'lucide-react';
import {startPreview,type PreviewSource,type PreviewStatus} from './preview-engine';
import './channel-player.css';

export function ChannelPlayer({source,onClose,embedded=false}:{source:PreviewSource;onClose:()=>void;embedded?:boolean}){
 const panel=useRef<HTMLElement>(null),video=useRef<HTMLVideoElement>(null),close=useRef<HTMLButtonElement>(null);
 const [attempt,setAttempt]=useState(0),[status,setStatus]=useState<PreviewStatus>({kind:'loading',text:'Carregando…'});
 useEffect(()=>{
  const previous=document.activeElement,overflow=document.body.style.overflow;
  if(!embedded)document.body.style.overflow='hidden';
  close.current?.focus();
  const escape=(event:KeyboardEvent)=>{
   if(event.key==='Escape'){event.preventDefault();onClose()}
   if(!embedded&&event.key==='Tab'){
    const items=Array.from(panel.current?.querySelectorAll<HTMLElement>('button:not([disabled]),video')||[]),first=items[0],last=items.at(-1);
    if(event.shiftKey&&document.activeElement===first){event.preventDefault();last?.focus()}
    else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first?.focus()}
   }
  };document.addEventListener('keydown',escape);
  return()=>{if(!embedded)document.body.style.overflow=overflow;document.removeEventListener('keydown',escape);if(previous instanceof HTMLElement&&previous.isConnected)previous.focus()};
 },[onClose,embedded]);
 useEffect(()=>startPreview(video.current!,source,setStatus),[source,attempt]);
 const retries=useRef(0);
 useEffect(()=>{if(!source.scope||!status.retryable)return;const delay=Math.min(30000,1000*2**Math.min(retries.current++,5));const timer=setTimeout(()=>setAttempt(n=>n+1),delay);return()=>clearTimeout(timer)},[status,source]);
 useEffect(()=>{retries.current=0},[source]);
 const ended=['paused','error','ended'].includes(status.kind);
 const content=<aside ref={panel} aria-modal={embedded?undefined:true} className={`channel-player${embedded?' channel-player-embedded':''}`} role={embedded?'region':'dialog'} aria-label={`Mini player: ${source.name}`}>
  <header><div><strong><Play size={15}/> {source.name}</strong><small>{source.protocol==='hls'?'HLS':'HTTP MPEG-TS'} · {source.scope?'recepção contínua':'prévia de até 5 minutos'}</small></div><button ref={close} className="icon-button" aria-label="Fechar mini player e parar transmissão" onClick={onClose}><X size={20}/></button></header>
  <video ref={video} controls playsInline muted aria-label={`Vídeo de ${source.name}`}/>
  <p role="status" data-state={status.kind}>{status.text}{status.retryable&&source.scope?' Reconectando automaticamente...':''}</p>
  <footer>{ended&&<button className="button primary" onClick={()=>setAttempt(n=>n+1)}><RotateCw size={14}/>Reproduzir novamente</button>}<button className="button" onClick={onClose}>Parar e fechar</button><small>Áudio inicia silenciado. Ative-o nos controles do vídeo. Pausar encerra a conexão da prévia.</small></footer>
 </aside>;
 return embedded?content:createPortal(<div className="channel-player-overlay" onMouseDown={event=>{if(event.target===event.currentTarget)onClose()}}>{content}</div>,document.body);
}
