import {useCallback,useEffect,useRef,useState} from 'react';
import type {ReactNode} from 'react';
import {Plus,Search,List,LayoutGrid,RefreshCw,Play,X,Info,ChevronLeft,ChevronRight,Tv} from 'lucide-react';
import {useLiveDistribution,stateLabel,failureLabel} from './live-distribution';
import {mediaRows,filterMedia,mediaState,receptionRequested,mediaRate,mediaDuration,type MediaRow,type MediaState} from './media-model';
import {ManualChannelEditor} from './manual-channel-editor';
import {ChannelPlayer} from './channel-player';
import type {PreviewSource} from './preview-engine';
import {PlaybackLinks} from './playback-links';
import './media-workspace.css';

const labels:Record<MediaState,string>={online:'Recebendo',waiting:'Aguardando',disabled:'Paradas',error:'Com erro'};
const protocolLabel=(value:string)=>({http_ts:'HTTP MPEG-TS',hls:'HLS',srt:'SRT',udp:'UDP MPEG-TS'}[value]||value||'Não selecionado');
type LocalSource={id:string;address:string};

function MediaDialog({title,onClose,children}:{title:string;onClose:()=>void;children:ReactNode}){
 const ref=useRef<HTMLElement>(null),close=useRef(onClose);close.current=onClose;
 useEffect(()=>{
  const previous=document.activeElement as HTMLElement|null,overflow=document.body.style.overflow;
  document.body.style.overflow='hidden';
  const elements=()=>Array.from(ref.current?.querySelectorAll<HTMLElement>('button:not([disabled]),a[href],input,select,video')||[]);
  elements()[0]?.focus();
  const key=(event:KeyboardEvent)=>{
   if(event.key==='Escape'){event.preventDefault();close.current()}
   if(event.key==='Tab'){const list=elements(),first=list[0],last=list.at(-1);if(event.shiftKey&&document.activeElement===first){event.preventDefault();last?.focus()}else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first?.focus()}}
  };
  document.addEventListener('keydown',key);
  return()=>{document.body.style.overflow=overflow;document.removeEventListener('keydown',key);previous?.focus()};
 },[]);
 return <div className="media-overlay"><section className="media-dialog" ref={ref} role="dialog" aria-modal="true" aria-label={title}><header><h2>{title}</h2><button type="button" aria-label="Fechar janela" onClick={onClose}><X size={18}/></button></header>{children}</section></div>;
}

export function MediaWorkspace({version,uptimeSeconds}:{version?:string;uptimeSeconds?:number}){
 const live=useLiveDistribution(),{view,admin,busy}=live;
 const [tab,setTab]=useState<'streams'|'sources'>('streams'),[query,setQuery]=useState(''),[filter,setFilter]=useState('all'),[sort,setSort]=useState('asc');
 const [layout,setLayout]=useState<'list'|'cards'>('list'),[page,setPage]=useState(0),[pageSize,setPageSize]=useState(100);
 const [selection,setSelection]=useState<Set<string>>(new Set()),[editor,setEditor]=useState<string|null>(null),[details,setDetails]=useState<string|null>(null);
 const [playing,setPlaying]=useState<PreviewSource|null>(null),[notice,setNotice]=useState(''),[working,setWorking]=useState(false),[sources,setSources]=useState<LocalSource[]>([]),[sourceError,setSourceError]=useState('');
 const lock=useRef(false),selectAll=useRef<HTMLInputElement>(null);
 const rows=mediaRows(view?.catalog.catalog?.channels||[],view?.configuration.channels||[],view?.streams||[]);
 const now=Date.now(),visible=filterMedia(rows,query,filter,sort,now),pages=Math.max(1,Math.ceil(visible.length/pageSize)),currentPage=Math.min(page,pages-1),shown=visible.slice(currentPage*pageSize,(currentPage+1)*pageSize);
 const selected=rows.filter(r=>selection.has(r.channel.id)),detail=rows.find(r=>r.channel.id===details),disabled=busy||working;
 const closePlayer=useCallback(()=>setPlaying(null),[]);
 useEffect(()=>{setPage(0)},[query,filter,sort,pageSize]);
 useEffect(()=>{if(selectAll.current)selectAll.current.indeterminate=shown.some(r=>selection.has(r.channel.id))&&!shown.every(r=>selection.has(r.channel.id))},[shown,selection]);
 useEffect(()=>{
  let cancelled=false;
  if(!admin)return;
  void live.call('GET','manual-channels').then(value=>{if(!cancelled){setSources(value.channels||[]);setSourceError('')}}).catch(()=>{if(!cancelled)setSourceError('Endereços locais indisponíveis nesta consulta.')});
  return()=>{cancelled=true};
 },[admin,live.call,view?.manual_revision]);
 const counts=(state:MediaState)=>rows.filter(row=>mediaState(row,now)===state).length;
 const measured=rows.filter(r=>mediaState(r,now)==='online'&&r.stream?.metrics.bits_per_second!=null);
 const input=measured.length?measured.reduce((sum,r)=>sum+(r.stream!.metrics.bits_per_second||0),0):null;
 const viewers=view?rows.reduce((sum,r)=>sum+(r.stream?.metrics.subscribers||0),0):null;
 const source=(row:MediaRow)=>row.channel.id.startsWith('local_')?sources.find(s=>s.id===row.channel.id)?.address||'Origem local protegida':'Origem autorizada do Kore Play';
 function choose(id:string,checked:boolean){setSelection(previous=>{const next=new Set(previous);if(checked)next.add(id);else next.delete(id);return next})}
 async function operate(targets:MediaRow[],action:'start'|'stop'|'restart'){
  if(lock.current||!view||!admin||!targets.length)return;
  lock.current=true;setWorking(true);setNotice('');
  let generation=view.configuration.generation,manual=view.manual_revision||0,completed=0,pendingRestart='';
  const send=async(id:string,operation:string)=>{
   const next=await live.action({action:operation,channel_id:id,generation,manual_revision:manual,confirmed:true});
   generation=next.configuration.generation;manual=next.manual_revision||0;
  };
  try{
   for(const row of targets){
    if(action==='restart'){await send(row.channel.id,'stop');pendingRestart=row.channel.name;}
    await send(row.channel.id,action==='restart'?'start':action);pendingRestart='';completed++;
   }
   setNotice(completed+' transmissão(ões) '+(action==='stop'?'parada(s).':'encaminhada(s) para início.'));setSelection(new Set());
  }catch(error){setNotice((pendingRestart?pendingRestart+': recepção parada; novo início não concluído. ':'')+completed+' de '+targets.length+' operação(ões) concluída(s). '+(error as Error).message);void live.refresh()}
  finally{lock.current=false;setWorking(false)}
 }
 function play(row:MediaRow){if(mediaState(row,now)==='online')setPlaying({id:row.channel.id,name:row.channel.name,protocol:'http_ts',scope:'live'})}
 const controls=(row:MediaRow)=><div className="media-controls">
  <button type="button" title="Reconectar recepção" aria-label={'Reconectar '+row.channel.name} disabled={!admin||disabled||!receptionRequested(row)} onClick={()=>void operate([row],'restart')}><RefreshCw size={13}/></button>
  <button type="button" title="Reproduzir transmissão" aria-label={'Reproduzir '+row.channel.name} disabled={mediaState(row,now)!=='online'} onClick={()=>play(row)}><Play size={13}/></button>
  <button type="button" role="switch" aria-checked={receptionRequested(row)} aria-label={(receptionRequested(row)?'Parar ':'Iniciar ')+row.channel.name} title={receptionRequested(row)?'Parar recepção':'Iniciar recepção'} className="media-switch" disabled={!admin||disabled} onClick={()=>void operate([row],receptionRequested(row)?'stop':'start')}><span/></button>
 </div>;
 const inputCell=(row:MediaRow)=><div className="media-cell"><strong>{protocolLabel(row.config?.protocol||row.channel.protocols[0]?.id||'')}</strong><span className="media-address" title={source(row)}>{source(row)}</span><span className={'media-status '+mediaState(row,now)}><i/>{mediaState(row,now)==='online'?'Recebendo dados':stateLabel(row.stream?.state)}</span><small>Ativação: {row.stream?.state==='running'?mediaDuration((now-new Date(row.stream.started_at).getTime())/1000):'—'}</small><small>Taxa de entrada: {mediaRate(row.stream?.metrics.bits_per_second)}</small></div>;
 const outputCell=(row:MediaRow)=><div className="media-cell"><span>Conexões TS: {row.stream?.metrics.subscribers??'Não medido'}</span><small>MPEG-TS: {mediaState(row,now)==='online'?'Disponível':'Sem recepção'}</small><small>HLS: {stateLabel(row.stream?.hls_state)}</small><small>UDP: {stateLabel(row.stream?.udp_state)}</small><button type="button" className="media-link" onClick={()=>setDetails(row.channel.id)}>Detalhes e saídas <Info size={11}/></button></div>;
 return <section className="media-workspace" aria-label="Mídia">
  <header className="media-heading"><h1>Mídia</h1><div className="media-summary" aria-label="Resumo de mídia">
   {version&&<span>Versão <b>{version}</b></span>}<span>Transmissões <b>{view?rows.length:'—'}</b></span><span title="Conexões TS observadas neste agente. Não representa clientes únicos ou receptores HLS.">Conexões TS <b>{viewers??'—'}</b></span><span>Entrada <b>{mediaRate(input)}</b>{input!==null&&measured.length<counts('online')?' (parcial)':''}</span><span>Saída <b>Não medida</b></span>{uptimeSeconds!=null&&<span>Agente ativo <b>{mediaDuration(uptimeSeconds)}</b></span>}
  </div></header>
  <div className="media-tabs"><button type="button" className="media-add" disabled={!admin||disabled} onClick={()=>setEditor('')} aria-label="Adicionar transmissão" title="Adicionar transmissão"><Plus size={15}/></button><div role="tablist" aria-label="Seções de mídia">{[['streams','Transmissões'],['sources','Fontes']].map(([id,label])=><button key={id} type="button" role="tab" aria-selected={tab===id} onClick={()=>setTab(id as typeof tab)}>{label}</button>)}</div><button type="button" className="media-refresh" aria-label="Atualizar mídia" onClick={()=>void live.refresh()}><RefreshCw size={14}/><span>Atualizar</span></button></div>
  {(notice||live.error)&&<p role={live.error?'alert':'status'} className="media-notice">{notice||live.error}</p>}
  <div className="media-toolbar"><label className="media-search"><Search size={15}/><input aria-label="Filtrar transmissões" placeholder="Filtrar por nome, categoria ou protocolo" value={query} onChange={e=>setQuery(e.target.value)}/></label><div className="media-view" aria-label="Visualização"><button type="button" aria-label="Visualização em lista" aria-pressed={layout==='list'} onClick={()=>setLayout('list')}><List size={16}/></button><button type="button" aria-label="Visualização em cartões" aria-pressed={layout==='cards'} onClick={()=>setLayout('cards')}><LayoutGrid size={16}/></button></div><label className="media-order">Ordenar por<select aria-label="Ordenação das transmissões" value={sort} onChange={e=>setSort(e.target.value)}><option value="asc">Nome (A–Z)</option><option value="desc">Nome (Z–A)</option><option value="bitrate">Taxa de entrada</option></select></label><div className="media-filters" aria-label="Filtrar por estado"><span>Estado:</span><button type="button" aria-pressed={filter==='all'} onClick={()=>setFilter('all')}>Todas ({rows.length})</button>{(['online','waiting','disabled','error'] as MediaState[]).map(state=><button key={state} type="button" className={state} aria-pressed={filter===state} onClick={()=>setFilter(state)}><i/>{labels[state]} ({counts(state)})</button>)}</div></div>
  {selection.size>0&&<div className="media-bulk" role="region" aria-label="Ações nas transmissões selecionadas"><strong>{selected.length} selecionada(s)</strong><button type="button" disabled={!admin||disabled||!selected.length} onClick={()=>void operate(selected,'start')}>Iniciar selecionadas</button><button type="button" disabled={!admin||disabled||!selected.length} onClick={()=>void operate(selected,'stop')}>Parar selecionadas</button><button type="button" onClick={()=>setSelection(new Set())}>Limpar seleção</button><small>A seleção pode incluir outras páginas e filtros.</small></div>}
  {!view&&!live.error&&<p role="status" className="media-empty">Carregando transmissões…</p>}
  {sourceError&&tab==='sources'&&<p className="media-notice" role="status">{sourceError}</p>}
  {view&&layout==='list'&&<div className="media-table-wrap"><table><thead><tr><th className="media-check"><input ref={selectAll} type="checkbox" aria-label="Selecionar transmissões desta página" disabled={!shown.length} checked={!!shown.length&&shown.every(r=>selection.has(r.channel.id))} onChange={e=>{const checked=e.target.checked;setSelection(previous=>{const next=new Set(previous);for(const row of shown){if(checked)next.add(row.channel.id);else next.delete(row.channel.id)}return next})}}/></th><th>Transmissão</th><th>Entrada</th>{tab==='streams'&&<><th>Transcodificação</th><th>Gravação</th><th>Saída</th></>}<th className="media-action-heading">Ações</th></tr></thead><tbody>{shown.map(row=><tr key={row.channel.id} className={selection.has(row.channel.id)?'is-selected':''}><td><input type="checkbox" aria-label={'Selecionar '+row.channel.name} checked={selection.has(row.channel.id)} onChange={e=>choose(row.channel.id,e.target.checked)}/></td><td><div className="media-cell"><button type="button" className="media-name" onClick={()=>setDetails(row.channel.id)}>{row.channel.name}</button><small>Recepção contínua</small><span className={'media-status '+mediaState(row,now)}>{labels[mediaState(row,now)]}</span>{row.channel.category&&<small>{row.channel.category}</small>}</div></td><td>{inputCell(row)}</td>{tab==='streams'&&<><td><div className="media-cell"><span>Fluxo original</span><small>Sem transcodificação</small></div></td><td><small className="media-muted">Não disponível</small></td><td>{outputCell(row)}</td></>}<td>{tab==='sources'&&row.channel.id.startsWith('local_')&&<button type="button" className="media-link" disabled={!admin||disabled} onClick={()=>setEditor(row.channel.id)}>Editar origem</button>}{controls(row)}</td></tr>)}</tbody></table>{!shown.length&&<p className="media-empty">{rows.length?'Nenhuma transmissão corresponde aos filtros.':'Nenhuma transmissão cadastrada. Use + para adicionar a primeira.'}</p>}</div>}
  {view&&layout==='cards'&&<div className="media-cards">{shown.map(row=><article key={row.channel.id}><header><Tv size={18}/><button type="button" className="media-name" onClick={()=>setDetails(row.channel.id)}>{row.channel.name}</button><input type="checkbox" aria-label={'Selecionar '+row.channel.name} checked={selection.has(row.channel.id)} onChange={e=>choose(row.channel.id,e.target.checked)}/></header>{inputCell(row)}{tab==='streams'&&outputCell(row)}{controls(row)}</article>)}{!shown.length&&<p className="media-empty">Nenhuma transmissão encontrada.</p>}</div>}
  <footer className="media-pagination"><label>Por página<select aria-label="Transmissões por página" value={pageSize} onChange={e=>setPageSize(Number(e.target.value))}>{[25,50,100].map(n=><option key={n} value={n}>{n}</option>)}</select></label><span>{visible.length?currentPage*pageSize+1:0}–{Math.min((currentPage+1)*pageSize,visible.length)} de {visible.length}</span><button type="button" aria-label="Página anterior" disabled={currentPage===0} onClick={()=>setPage(currentPage-1)}><ChevronLeft size={15}/></button><button type="button" aria-label="Próxima página" disabled={currentPage>=pages-1} onClick={()=>setPage(currentPage+1)}><ChevronRight size={15}/></button><small>Atualização automática · sem iniciar canais ao abrir a página</small></footer>
  {editor!==null&&<ManualChannelEditor id={editor} onClose={()=>setEditor(null)} onSaved={()=>{setEditor(null);void live.refresh()}}/>}
  {detail&&<MediaDialog title={detail.channel.name} onClose={()=>setDetails(null)}><dl className="media-details"><dt>Entrada</dt><dd>{protocolLabel(detail.config?.protocol||detail.channel.protocols[0]?.id||'')}</dd><dt>Estado</dt><dd>{stateLabel(detail.stream?.state)}</dd><dt>Interrupções detectadas</dt><dd>{detail.stream?.interruptions??'Não medido'}</dd><dt>Motivo da última queda</dt><dd>{failureLabel(detail.stream?.last_failure_reason)}</dd><dt>Erros de continuidade TS</dt><dd>{detail.stream?.metrics.transport?.cc_errors??'Não medido'}</dd></dl>{detail.channel.id.startsWith('local_')&&admin&&<button type="button" onClick={()=>{setEditor(detail.channel.id);setDetails(null)}}>Editar origem</button>}<PlaybackLinks channelID={detail.channel.id} admin={admin} hlsState={detail.stream?.hls_state} call={live.call}/><a className="media-link" href="#channels" onClick={()=>setDetails(null)}>Abrir gerenciamento de canais e destinos UDP</a></MediaDialog>}
  {playing&&<MediaDialog title={'Reproduzir '+playing.name} onClose={closePlayer}><ChannelPlayer embedded source={playing} onClose={closePlayer}/></MediaDialog>}
 </section>;
}
