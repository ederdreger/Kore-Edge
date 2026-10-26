const video=document.getElementById('video');
const status=document.getElementById('status');
const id=location.pathname.split('/').pop();
const url='/hls/'+encodeURIComponent(id)+'/index.m3u8';
video.addEventListener('playing',()=>{status.textContent='Reproduzindo fonte sintética';});
video.addEventListener('error',()=>{status.textContent='Falha na reprodução. Confira autorização e codecs.';});
if(video.canPlayType('application/vnd.apple.mpegurl')){video.src=url;status.textContent='Playlist disponível. Pressione reproduzir.';}
else if(window.Hls&&Hls.isSupported()){
 const hls=new Hls({enableWorker:false,maxBufferLength:12,maxMaxBufferLength:20,manifestLoadingMaxRetry:2,levelLoadingMaxRetry:2,fragLoadingMaxRetry:2});
 hls.loadSource(url);hls.attachMedia(video);
 hls.on(Hls.Events.MANIFEST_PARSED,()=>{status.textContent='Playlist disponível. Pressione reproduzir.';});
 hls.on(Hls.Events.ERROR,(_,data)=>{if(data.fatal){status.textContent='HLS indisponível. Confira canal iniciado e permissão no catálogo.';hls.destroy();}});
 window.addEventListener('pagehide',()=>hls.destroy(),{once:true});
}else{status.textContent='Este navegador não suporta HLS/MSE com H.264/AAC.';}
