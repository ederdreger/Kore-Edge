export type PreviewSource={id:string;name:string;protocol:'hls'|'http_ts';scope?:'live'|'output'};
export type PreviewStatus={kind:'loading'|'playing'|'blocked'|'paused'|'error'|'ended';text:string};

// Only the authenticated local media proxy is a valid preview source.
export function previewPath(source:PreviewSource){
 if(!/^[a-zA-Z0-9_-]{1,80}$/.test(source.id)||!['hls','http_ts'].includes(source.protocol))throw new Error('invalid_preview');
 if(source.scope==='live')return `/api/v1/live/media/${source.id}/stream.ts`;
 if(source.scope==='output'&&source.protocol==='hls')return `/api/v1/live/hls/${source.id}/index.m3u8`;
 return `/api/v1/kore-play/media/${source.id}/${source.protocol}/${source.protocol==='hls'?'index.m3u8':'stream.ts'}`;
}

export function startPreview(video:HTMLVideoElement,source:PreviewSource,notify:(status:PreviewStatus)=>void){
 const requestedAt=performance.now();let firstFrame:number|undefined;
 let stopped=false,release:undefined|(()=>void),wasPlaying=false,blocked=false,lastProgress=Date.now(),lastTime=video.currentTime;
 const listeners:Array<[string,EventListener]>=[];
 const listen=(name:string,callback:()=>void)=>{listeners.push([name,callback]);video.addEventListener(name,callback)};
 const timers:Array<ReturnType<typeof setTimeout>>=[];
 function stop(){
  if(stopped)return;stopped=true;
  timers.forEach(clearTimeout);clearInterval(watchdog);
  listeners.forEach(([name,callback])=>video.removeEventListener(name,callback));
  try{release?.()}catch{/* release must not prevent closing the media element */}
  video.pause();video.removeAttribute('src');video.load();
 }
 function finish(kind:PreviewStatus['kind'],text:string){if(stopped)return;stop();notify({kind,text})}
 function play(){void video.play().catch(error=>{
  if(stopped)return;
  if(error?.name==='NotAllowedError'){blocked=true;notify({kind:'blocked',text:'O navegador bloqueou o início automático. Clique em Reproduzir no vídeo.'})}
  else finish('error','O navegador não conseguiu reproduzir este formato. Confira os codecs e teste a origem.');
 })}
 listen('playing',()=>{wasPlaying=true;blocked=false;lastProgress=Date.now();firstFrame??=Math.round(performance.now()-requestedAt);notify({kind:'playing',text:`Reproduzindo · início em ${(firstFrame/1000).toFixed(1)} s · ${source.scope==='live'?'recepção contínua':source.scope==='output'?'HLS local':'origem sob demanda'}`})});
 listen('timeupdate',()=>{if(video.currentTime!==lastTime){lastTime=video.currentTime;lastProgress=Date.now()}});
 listen('waiting',()=>notify({kind:'loading',text:'Aguardando vídeo ou preenchendo o buffer…'}));
 listen('pause',()=>{if(wasPlaying)finish('paused','Prévia pausada e conexão encerrada. Use Reproduzir novamente para continuar.')});
 listen('error',()=>finish('error','Falha ao reproduzir. Verifique o sinal, a autorização e os codecs de áudio/vídeo suportados pelo navegador.'));
 listen('ended',()=>finish('ended','A transmissão terminou. Use Reproduzir novamente para consultar o sinal atual.'));
 const watchdog=setInterval(()=>{if(!blocked&&Date.now()-lastProgress>20000)finish('error','Nenhum avanço de vídeo em 20 segundos. Confira sinal, banda, autorização e codecs antes de tentar novamente.')},2000);
 timers.push(setTimeout(()=>finish('ended','Prévia de cinco minutos encerrada. Você pode reproduzir novamente ou testar outro canal.'),300000));
 video.muted=true;
 notify({kind:'loading',text:'Carregando a prévia autenticada…'});
 void (async()=>{
  const url=previewPath(source);
  if(source.protocol==='hls'){
   if(video.canPlayType('application/vnd.apple.mpegurl')){video.src=url;video.load();play();return}
   const {default:Hls}=await import('hls.js');if(stopped)return;
   if(!Hls.isSupported()){finish('error','Este navegador não oferece suporte a HLS. Use um navegador com MediaSource ou HLS nativo.');return}
   const prefix=source.scope==='output'?`/api/v1/live/hls/${source.id}/`:`/api/v1/kore-play/media/${source.id}/hls/`;
   function checked(raw:string){const target=new URL(raw,window.location.origin);if(target.origin!==window.location.origin||!target.pathname.startsWith(prefix)||target.username||target.password||target.search||target.hash)throw new Error('invalid_media_request');return target.href}
   const hls=new Hls({enableWorker:false,debug:false,maxBufferLength:15,maxMaxBufferLength:30,maxBufferSize:24*1024*1024,backBufferLength:10,
    manifestLoadingTimeOut:8000,manifestLoadingMaxRetry:1,levelLoadingTimeOut:8000,levelLoadingMaxRetry:1,fragLoadingTimeOut:8000,fragLoadingMaxRetry:1,
    xhrSetup:(xhr,url)=>{checked(url);xhr.withCredentials=true},
    fetchSetup:(context,init)=>new Request(checked(context.url),{...init,credentials:'same-origin',redirect:'error'})});
   release=()=>hls.destroy();
   hls.on(Hls.Events.ERROR,(_event,data)=>{if(data.fatal)finish('error',data.response?.code===401||data.response?.code===403?'Acesso ao canal recusado. Confira a sessão e as permissões do parceiro.':'Não foi possível reproduzir HLS. Confira playlist, segmentos, sinal e codecs na origem.')});
   hls.on(Hls.Events.MANIFEST_PARSED,play);hls.loadSource(url);hls.attachMedia(video);
  }else{
   const {default:mpegts}=await import('mpegts.js');if(stopped)return;
   mpegts.LoggingControl.enableAll=false;
   if(!mpegts.getFeatureList().mseLivePlayback){finish('error','Este navegador não suporta prévia HTTP MPEG-TS. Use Chrome ou Edge atualizado e codecs compatíveis.');return}
   const player=mpegts.createPlayer({type:'mpegts',isLive:true,withCredentials:true,url},{enableWorker:false,enableWorkerForMSE:false,enableStashBuffer:true,autoCleanupSourceBuffer:true,autoCleanupMaxBackwardDuration:15,autoCleanupMinBackwardDuration:5,liveBufferLatencyChasing:true,liveBufferLatencyMaxLatency:10,liveBufferLatencyMinRemain:4});
   release=()=>player.destroy();player.on(mpegts.Events.ERROR,()=>finish('error','Não foi possível reproduzir MPEG-TS. Confira o sinal e os codecs de áudio/vídeo suportados pelo navegador.'));
   player.attachMediaElement(video);player.load();play();
  }
 })().catch(()=>finish('error','Não foi possível iniciar o player. Atualize a página e confira sinal, autorização e compatibilidade do navegador.'));
 return stop;
}
