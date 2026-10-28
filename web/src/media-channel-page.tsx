import {useCallback,useEffect,useRef,useState} from 'react';
import {ChevronRight,Play,RefreshCw,Square,Tv,Trash2} from 'lucide-react';
import {detectLink} from './manual-channel-editor';
import {ChannelPlayer} from './channel-player';
import type {PreviewSource} from './preview-engine';
import {MediaChannelOutput} from './media-channel-output';
import {mediaRows,mediaState,receptionRequested,mediaRate,type MediaRow} from './media-model';
import {stateLabel,failureLabel,type Capabilities,useLiveDistribution} from './live-distribution';
import {channelPath,type MediaRoute} from './media-channel-route';
import './media-channel-page.css';
import {ChannelProcessing,ChannelProcessingSummary} from './media-channel-processing';

type Source={id:string;name:string;title?:string;description?:string;category:string;address:string;protocol:string;interface:string;local_ip:string;enabled:boolean};
type Inventory={revision:number;channels:Source[]};
type Point={at:number;input:number|null;clients:number|null};
type Props={route:Exclude<MediaRoute,{kind:'list'}>;live:ReturnType<typeof useLiveDistribution>;working:boolean;notice:string;onCreated:(id:string,message:string)=>void;onOperate:(rows:MediaRow[],action:'start'|'stop'|'restart')=>Promise<void>;version?:string};
const protocolLabel=(p:string)=>({http_ts:'HTTP MPEG-TS',hls:'HLS',udp:'UDP MPEG-TS',srt:'SRT'}[p]||'Não identificado');

function ChannelChart({points}:{points:Point[]}){
 const inputMax=Math.max(1,...points.map(p=>p.input||0)),clientMax=Math.max(1,...points.map(p=>p.clients||0));
 const paths=(field:'input'|'clients',maximum:number)=>{
  const segments:string[]=[];let current:string[]=[];
  points.forEach((p,i)=>{const value=p[field];if(value===null){if(current.length)segments.push(current.join(' '));current=[];return}current.push((20+i/Math.max(1,points.length-1)*560).toFixed(1)+','+(130-value/maximum*110).toFixed(1))});
  if(current.length)segments.push(current.join(' '));return segments;
 };
 return <section className="mc-panel mc-chart" aria-label="Histórico observado do canal"><h2>Fluxo compartilhado e conexões TS</h2><div className="mc-chart-legend"><span className="mc-input-line">Fluxo compartilhado</span><span className="mc-client-line">Conexões TS</span><span>Saída: não medida</span></div><svg viewBox="0 0 600 155" role="img" aria-label="Amostras reais desde a abertura desta página">{[20,55,90,130].map(y=><line key={y} x1="20" x2="580" y1={y} y2={y} stroke="#dce4ef"/>)}{paths('input',inputMax).map((p,i)=><polyline key={'input'+i} points={p} fill="none" stroke="#e75b69" strokeWidth="2"/>)}{paths('clients',clientMax).map((p,i)=><polyline key={'clients'+i} points={p} fill="none" stroke="#3b69db" strokeWidth="2"/>)}</svg><div className="mc-chart-scale"><span>Escala do fluxo: {mediaRate(inputMax===1&&points.every(p=>p.input===null)?null:inputMax)}</span><span>Escala TS: {clientMax} conexão(ões)</span></div><p>{points.length<2?'Aguardando novas amostras.':'Horário: '+new Date(points[0].at).toLocaleTimeString('pt-BR')+' – '+new Date(points[points.length-1].at).toLocaleTimeString('pt-BR')+'. As escalas são independentes.'}</p></section>;
}

export function MediaChannelPage({route,live,working,notice,onCreated,onOperate,version}:Props){
 const {view,admin}=live,isNew=route.kind==='new',id=route.kind==='channel'?route.id:'';
 const [inventory,setInventory]=useState<Inventory|null>(null),[loadError,setLoadError]=useState(''),[error,setError]=useState(''),[message,setMessage]=useState('');
 const [name,setName]=useState(''),[url,setURL]=useState(''),[format,setFormat]=useState('auto'),[iface,setInterface]=useState(''),[localIP,setLocalIP]=useState(''),[cap,setCap]=useState<Capabilities|null>(null);
 const [metadata,setMetadata]=useState({name:'',title:'',description:''}),[baseRevision,setBaseRevision]=useState(0),[dirty,setDirty]=useState(false),[reload,setReload]=useState(0);
 const [busy,setBusy]=useState(false),[deleting,setDeleting]=useState(false),[playing,setPlaying]=useState<PreviewSource|null>(null),[points,setPoints]=useState<Point[]>([]);
 const lock=useRef(false),alive=useRef(true),draft=useRef(false);
 const rows=mediaRows(view?.catalog.catalog?.channels||[],view?.configuration.channels||[],view?.streams||[]),row=rows.find(r=>r.channel.id===id),source=inventory?.channels.find(c=>c.id===id);
 const local=id.startsWith('local_'),blocked=busy||working||live.busy,protocol=format==='auto'?detectLink(url):format;
 const closePlayer=useCallback(()=>setPlaying(null),[]);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false}},[]);
 useEffect(()=>{
  let cancelled=false;
  if(!admin||!isNew&&!local)return;
  setLoadError('');
  void live.call('GET','manual-channels').then((value:Inventory)=>{if(cancelled)return;setInventory(value);if(!draft.current){const current=value.channels.find(c=>c.id===id);if(current)setMetadata({name:current.name,title:current.title||'',description:current.description||''});setBaseRevision(value.revision)}}).catch(e=>{if(!cancelled)setLoadError(e.message)});
  return()=>{cancelled=true};
 },[admin,isNew,id,local,live.call,view?.manual_revision,reload]);
 useEffect(()=>{
  let cancelled=false;
  if(protocol!=='udp')return;
  void live.call('GET','live/capabilities').then(value=>{if(!cancelled)setCap(value)}).catch(()=>{if(!cancelled)setError('Não foi possível consultar as interfaces UDP. Tente novamente antes de salvar.')});
  return()=>{cancelled=true};
 },[protocol,live.call]);
 useEffect(()=>{
  if(!row?.stream)return;
  const stream=row.stream,fresh=mediaState(row)==='online';
  setPoints(previous=>[...previous,{at:Date.now(),input:fresh?stream.metrics.bits_per_second:null,clients:stream.metrics.subscribers}].slice(-120));
 },[row?.stream]);
 async function create(event:React.FormEvent){
  event.preventDefault();if(lock.current||!admin||!inventory||blocked)return;
  lock.current=true;setBusy(true);setError('');
  try{
   const value=await live.call('POST','manual-channels',{action:'save',revision:inventory.revision,confirmed:true,source:{id:'',name:name.trim(),category:'',url,protocol:format==='auto'?(protocol||'auto'):format,interface:iface,local_ip:localIP,enabled:true}});
   if(!alive.current)return;
   setURL('');
   await live.refresh();
   if(!alive.current)return;
   if(typeof value.channel_id!=='string'||!/^local_[a-f0-9]{32}$/.test(value.channel_id)){window.location.hash='#media';return}
   onCreated(value.channel_id,value.started?'Canal salvo. A recepção foi solicitada.':'Canal salvo. Acompanhe o estado da recepção e as ferramentas necessárias.');
  }catch(e){if(alive.current)setError((e as Error).message)}
  finally{lock.current=false;if(alive.current)setBusy(false)}
 }
 async function saveMetadata(event:React.FormEvent){
  event.preventDefault();if(lock.current||!admin||!local||blocked||!source)return;
  lock.current=true;setBusy(true);setError('');setMessage('');
  try{
   const value=await live.call('POST','manual-channels',{action:'metadata',revision:baseRevision,source:{id},metadata,confirmed:true});
   if(!alive.current)return;
   setInventory(value);setBaseRevision(value.revision);draft.current=false;setDirty(false);setMessage('Descrição salva. A recepção foi mantida.');
   await live.refresh();
  }catch(e){if(alive.current)setError((e as Error).message)}
  finally{lock.current=false;if(alive.current)setBusy(false)}
 }
 async function remove(){
  if(lock.current||!source||!row||!admin||blocked)return;
  lock.current=true;setBusy(true);setError('');let stopped=false;
  try{
   if(receptionRequested(row)){await live.action({action:'stop',channel_id:id,generation:view!.configuration.generation,manual_revision:view?.manual_revision||0,confirmed:true});stopped=true}
   await live.call('POST','manual-channels',{action:'delete',revision:inventory!.revision,source:{id},confirmed:true});
   await live.refresh();if(alive.current)window.location.hash='#media';
  }catch(e){if(alive.current)setError((stopped?'Recepção parada; exclusão não concluída. ':'')+(e as Error).message)}
  finally{lock.current=false;if(alive.current)setBusy(false)}
 }
 function editMetadata(field:keyof typeof metadata,value:string){draft.current=true;setDirty(true);setMetadata(old=>({...old,[field]:value}));setError('');setMessage('')}
 function play(protocol:'http_ts'|'hls'){
  if(!row)return;
  setPlaying({id,name:row.channel.name,protocol,scope:protocol==='hls'?'output':'live'});
 }
 const online=!!row&&mediaState(row)==='online';
 return <section className="media-workspace media-channel-page">
  <header className="media-heading"><nav className="mc-breadcrumb" aria-label="Caminho do canal"><a href="#media">Transmissões</a><ChevronRight size={14}/><h1>{isNew?'Novo canal':row?.channel.name||source?.name||'Configuração do canal'}</h1></nav><div className="media-summary">{version&&<span>Versão <b>{version}</b></span>}<span>Transmissões <b>{view?rows.length:'—'}</b></span><span>Saída <b>Não medida</b></span></div></header>
  {isNew?<><div className="media-tabs"><a href="#media">Transmissões</a><span className="mc-current-tab">Novo canal</span></div>{!admin?<p role="alert" className="media-notice">Somente administradores podem cadastrar canais.</p>:<form className="mc-create mc-panel" onSubmit={event=>void create(event)} aria-label="Novo canal"><div className="mc-create-fields"><label>Nome do canal<input autoFocus required maxLength={128} value={name} onChange={e=>setName(e.target.value)} disabled={busy}/></label><label>URL da origem<input required type="url" autoComplete="off" spellCheck={false} maxLength={4096} value={url} onChange={e=>{setURL(e.target.value);setFormat('auto');setInterface('');setLocalIP('');setError('')}} disabled={busy}/></label></div><div className="mc-format"><span>Protocolo: <strong>{protocolLabel(protocol)}</strong></span>{/^https?:/i.test(url)&&<label>Formato HTTP<select aria-label="Formato HTTP" value={format} onChange={e=>setFormat(e.target.value)}><option value="auto">Detectar pelo link</option><option value="http_ts">HTTP MPEG-TS</option><option value="hls">HLS</option></select></label>}</div>{protocol==='udp'&&<div className="mc-create-fields"><label>Interface UDP<select required value={iface} onChange={e=>{setInterface(e.target.value);setLocalIP('')}}><option value="">Escolha a interface</option>{cap?.interfaces.filter(i=>i.up&&i.ipv4.length).map(i=><option key={i.name}>{i.name}</option>)}</select></label><label>IP local de recepção<select required value={localIP} onChange={e=>setLocalIP(e.target.value)}><option value="">Confirme o IP local</option>{cap?.interfaces.find(i=>i.name===iface)?.ipv4.map(ip=><option key={ip}>{ip}</option>)}</select></label></div>}{(error||loadError)&&<p role="alert" className="media-notice">{error||loadError}</p>}{loadError&&<button type="button" onClick={()=>setReload(n=>n+1)}>Tentar novamente</button>}<div className="mc-form-actions"><button className="mc-primary" type="submit" disabled={blocked||!inventory||!name.trim()||!url.trim()||!protocol||protocol==='udp'&&(!iface||!localIP)}>{busy?'Salvando…':'Salvar'}</button><button type="button" disabled={busy} onClick={()=>{window.location.hash='#media'}}>Cancelar</button></div></form>}</>:<>
   <div className="media-tabs mc-channel-tabs" role="tablist" aria-label="Configuração do canal">{[['overview','Visão geral'],['input','Entrada'],['transcoder','Transcodificação'],['dvr','Gravação'],['output','Saídas'],['epg','Guia de programação'],['auth','Autenticação'],['sessions','Sessões de reprodução']].map(([tab,label])=>{const enabled=['overview','input','output','transcoder'].includes(tab);return <button type="button" role="tab" key={tab} aria-selected={route.kind==='channel'&&route.tab===tab} disabled={!enabled} title={enabled?label:'Configuração prevista para uma próxima etapa'} onClick={()=>{if(enabled)window.location.hash=channelPath(id,tab as 'overview'|'input'|'output'|'transcoder')}}>{label}</button>})}</div>
   {(notice||message)&&<p role="status" className="media-notice">{message||notice}</p>}
   {(error||loadError||live.error)&&<p role="alert" className="media-notice">{error||loadError||live.error}</p>}
   {loadError&&<button type="button" onClick={()=>setReload(n=>n+1)}>Tentar novamente</button>}
   {!row&&<p className="media-empty">{view?'Canal não encontrado no catálogo atual.':'Carregando canal…'} <a href="#media">Voltar às transmissões</a></p>}
   {row&&route.kind==='channel'&&route.tab==='overview'&&<>
    <form className="mc-panel mc-description" aria-label="Descrição do canal" onSubmit={event=>void saveMetadata(event)}><h2>Descrição</h2><div className="mc-description-fields"><label>Nome<input required maxLength={128} value={source?metadata.name:row.channel.name} disabled={!admin||!source||blocked} onChange={e=>editMetadata('name',e.target.value)}/></label><label>Título<input maxLength={256} value={metadata.title} disabled={!admin||!source||blocked} onChange={e=>editMetadata('title',e.target.value)}/></label><label>Descrição<textarea rows={1} maxLength={2048} value={metadata.description} disabled={!admin||!source||blocked} onChange={e=>editMetadata('description',e.target.value)}/></label></div>{!local&&<p className="mc-muted">Este canal é gerenciado pela matriz Kore Play.</p>}<div className="mc-form-actions">{local&&admin&&<button className="mc-primary" type="submit" disabled={!source||!dirty||blocked||!metadata.name.trim()}>Salvar descrição</button>}{dirty&&<button type="button" disabled={blocked} onClick={()=>{draft.current=false;setDirty(false);setReload(n=>n+1);setError('')}}>Recarregar descrição</button>}</div></form>
    <div className="mc-overview-grid"><section className="mc-panel mc-player"><div className="mc-player-toolbar"><div><button type="button" disabled={!admin||blocked} onClick={()=>void onOperate([row],receptionRequested(row)?'stop':'start')}>{receptionRequested(row)?<Square size={13}/>:<Play size={13}/>} {receptionRequested(row)?'Parar canal':'Iniciar canal'}</button><button type="button" disabled={!admin||blocked||!receptionRequested(row)} onClick={()=>void onOperate([row],'restart')}><RefreshCw size={13}/>Reconectar</button></div><span className="mc-continuous">Recepção contínua</span></div>{playing?<ChannelPlayer embedded source={playing} onClose={closePlayer}/>:<div className="mc-video-placeholder"><Tv size={46}/><button type="button" className="mc-primary" disabled={!online} onClick={()=>play('http_ts')}><Play size={20}/>Reproduzir canal</button><span>{online?'Recepção ativa. Abra o player para assistir.':stateLabel(row.stream?.state)}</span></div>}<div className="mc-player-formats"><button type="button" disabled={!online} aria-pressed={playing?.protocol==='http_ts'} onClick={()=>play('http_ts')}>MPEG-TS</button><button type="button" disabled={row.stream?.hls_state!=='running'} aria-pressed={playing?.protocol==='hls'} onClick={()=>play('hls')}>HLS</button><a href={channelPath(id,'output')}>Ver saídas e links</a></div></section><div className="mc-observations"><section className="mc-panel"><div className="mc-status-heading"><span className={'media-status '+mediaState(row)}>{online?'Recebendo':stateLabel(row.stream?.state)}</span><span>{mediaRate(row.stream?.metrics.bits_per_second)}</span></div><div className="mc-media-info"><div><h2>Informações de entrada</h2><p>{protocolLabel(row.config?.protocol||row.channel.protocols[0]?.id||'')}</p><p>Codecs e resolução: não medidos</p><p>Interrupções: {row.stream?.interruptions??'Não medido'}</p><p>Última queda: {failureLabel(row.stream?.last_failure_reason)}</p></div><div><h2>Informações de saída</h2><p>MPEG-TS: {online?'Disponível':'Sem recepção'}</p><p>HLS: {stateLabel(row.stream?.hls_state)}</p><p>UDP: {stateLabel(row.stream?.udp_state)}</p><ChannelProcessingSummary id={id} live={live}/></div></div></section><ChannelChart points={points}/></div></div>
   </>}
   {row&&route.kind==='channel'&&(route.tab==='input'||route.tab==='transcoder')&&<ChannelProcessing key={id+route.tab} id={id} live={live} blocked={blocked} tab={route.tab}/>}
   {row&&route.kind==='channel'&&route.tab==='output'&&<MediaChannelOutput key={id} row={row} live={live} blocked={blocked}/>}
   {local&&admin&&row&&<footer className="mc-channel-footer">{!deleting?<button type="button" className="mc-delete" disabled={blocked||!source} onClick={()=>setDeleting(true)}><Trash2 size={13}/>Excluir canal</button>:<div className="mc-delete-confirm" role="alert"><p>Excluir este canal encerra sua recepção e invalida seus links permanentes.</p><button type="button" className="mc-delete" disabled={blocked} onClick={()=>void remove()}>Confirmar exclusão</button><button type="button" disabled={blocked} onClick={()=>setDeleting(false)}>Cancelar exclusão</button></div>}</footer>}
  </>}
 </section>;
}
