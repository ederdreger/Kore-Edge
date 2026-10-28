import {InputPolicyControls,emptyPolicy,type InputPolicy} from './input-policy-controls';
import {type MediaAsset} from './media-assets';
import {TranscodeControls,processingDefaults,type Transcode} from './transcode-controls';
export type {Transcode} from './transcode-controls';
import {usePendingEdits} from './use-pending-edits';
import {mediaRate,mediaDuration} from './media-model';
import {useEffect,useState} from 'react';
import {ArrowDown,ArrowUp,Plus,Trash2,ArrowRightCircle,ArrowLeftCircle,Tv,Activity,Clock3} from 'lucide-react';
import {detectLink} from './manual-channel-editor';
import {stateLabel,failureLabel,type LiveStream,type Capabilities,useLiveDistribution} from './live-distribution';

type Live=ReturnType<typeof useLiveDistribution>;
type Input={key:string;address?:string;url?:string;protocol:string;interface:string;local_ip:string;source_timeout?:number;token?:string;certificate?:string};

type MediaTrack={kind:'video'|'audio'|'subtitle';codec:string;index:number;pid?:number;width?:number;height?:number;channels?:number;sample_rate?:number;bits_per_second?:number;language?:string};
type MediaInfo={tracks?:MediaTrack[];video?:string;audio?:string;width?:number;height?:number;continuity_errors?:number;transport_errors?:number;measured_at:string};
type Profile={policy?:InputPolicy;backup_active?:boolean;input_receiver?:string;input_srt?:{received:number;missing:number;retransmitted:number;dropped:number;undecryptable:number;rtt_ms:number;latency_ms:number};input_media?:MediaInfo;output_media?:MediaInfo;revision:number;inputs:Input[];transcode:Transcode;active_key:string;local:boolean};
type Devices={devices:{id:string;name:string}[];max_concurrent:number;warning:string};
type Props={id:string;live:Live;blocked:boolean;tab:'input'|'transcoder'};
const defaults=processingDefaults;
const label=(p:string)=>({http_ts:'HTTP MPEG-TS',hls:'HLS',udp:'UDP MPEG-TS',srt:'SRT',publish:'Publica\u00e7\u00e3o HTTPS',cluster:'Cluster Edge HTTPS'}[p]||'Detectar pelo link');

function MeasuredMedia({info,online}:{info?:MediaInfo;online:boolean}){
 if(!online||!info)return <p>Codecs e resolução: {online?'Aguardando medição do fluxo':'Sem medição atual'}</p>;
 if(info.tracks?.length)return <div className="mc-tracks">{info.tracks.map((track,index)=>{
  const number=info.tracks!.slice(0,index+1).filter(t=>t.kind===track.kind).length;
  const code=track.codec==='h264'?'H.264':track.codec==='hevc'?'H.265':track.codec.toUpperCase();
  return <details key={track.index+'-'+index} className={'mc-track mc-track-'+track.kind}><summary>{track.kind==='video'?'v':track.kind==='audio'?'a':'s'}{number} {code}{track.width&&track.height?` ${track.width}×${track.height}`:''}{track.channels?` · ${track.channels} canal(is)`:''}{track.language?` · ${track.language}`:''}{track.pid!==undefined?` · PID ${track.pid}`:''}{track.bits_per_second?` (${mediaRate(track.bits_per_second)})`:''}</summary><dl><dt>Tipo</dt><dd>{track.kind==='video'?'Vídeo':track.kind==='audio'?'Áudio':'Legenda'}</dd><dt>Codec</dt><dd>{code}</dd>{track.sample_rate&&<><dt>Amostragem</dt><dd>{track.sample_rate} Hz</dd></>}</dl></details>
 })}</div>;
 return <><p>Vídeo: {info.video?.toUpperCase()||'Não identificado'}{info.width&&info.height?` · ${info.width} × ${info.height}`:''}</p><p>Áudio: {info.audio?.toUpperCase()||'Não identificado'}</p><p>Erros na amostra TS: continuidade {info.continuity_errors??'Não medido'} · transporte {info.transport_errors??'Não medido'}</p></>;
}
export function ChannelProcessingSummary({id,live,stream,fallbackProtocol}:{id:string;live:Live;stream?:LiveStream;fallbackProtocol:string}){
 const [value,setValue]=useState<Profile|null>(null);
 useEffect(()=>{if(!live.admin)return;let disposed=false;setValue(null);const load=()=>{void live.call('GET','manual-channels/processing?id='+encodeURIComponent(id)).then(v=>{if(!disposed)setValue(v)}).catch(()=>{if(!disposed)setValue(null)})};load();const timer=setInterval(load,5000);return()=>{disposed=true;clearInterval(timer)}},[id,live.admin,live.call]);
 const active=value?.inputs?.find(v=>v.key===value.active_key);
 const online=stream?.state==='running';
 const started=Date.parse(stream?.started_at||''),height=value?.output_media?.height;
 return <><div className="mc-status-heading"><span className={'mc-online-pill '+(online?'is-online':'')}>{online?'Recebendo':stateLabel(stream?.state)}</span><div className="mc-stream-facts">{height&&<span><Tv size={14}/>{height>=2160?'UHD':height>=1080?'Full HD':height>=720?'HD':'SD'}</span>}<span title="Taxa do fluxo compartilhado após processamento"><Activity size={14}/>{mediaRate(stream?.metrics.bits_per_second)}</span>{online&&Number.isFinite(started)&&<span title="Tempo desta recepção"><Clock3 size={14}/>{mediaDuration((Date.now()-started)/1000)}</span>}</div></div>
 <div className="mc-media-info"><div><h2><ArrowRightCircle size={16}/>Informações de entrada</h2><p className="mc-info-protocol">{active?label(active.protocol):value?.local?'Entrada ativa ainda não identificada':label(fallbackProtocol)}</p><MeasuredMedia info={value?.input_media} online={online}/></div><div><h2><ArrowLeftCircle size={16}/>Informações de saída</h2><p className="mc-info-protocol">Fluxo distribuído pelo Edge</p><MeasuredMedia info={value?.output_media} online={online}/></div></div>
 <details className="mc-reception-diagnostics"><summary>Diagnóstico da recepção e das saídas</summary><div><p>Interrupções: {stream?.interruptions??'Não medido'}</p><p>Última queda: {stream?.interruptions===0?'Sem quedas nesta recepção':failureLabel(stream?.last_failure_reason)}</p>{online&&value?.input_receiver&&<p>Receptor SRT: {value.input_receiver}</p>}{online&&value?.input_srt&&<p>SRT: {value.input_srt.dropped} descartados · {value.input_srt.retransmitted} retransmitidos · RTT {value.input_srt.rtt_ms.toFixed(1)} ms</p>}<p>MPEG-TS: {online?'Disponível':'Sem recepção'} · HLS: {stateLabel(stream?.hls_state)} · UDP: {stateLabel(stream?.udp_state)}</p><p>{value?.transcode?.mode==='copy'?'Sem transcodificação':value?.transcode?.mode==='h264'?'Perfil H.264 · '+value.transcode.device:'Processamento: indisponível nesta consulta'}</p>{active&&<p>Origem selecionada: {label(active.protocol)} · URL {value!.inputs.indexOf(active)+1}</p>}<p>Codecs, resolução e PIDs medidos em amostras reais; taxas por trilha aparecem somente quando informadas pelo fluxo.</p></div></details></>;


}

export function ChannelProcessing({id,live,blocked,tab}:Props){
 const [profile,setProfile]=useState<Profile|null>(null),[inputs,setInputs]=useState<Input[]>([]),[transcode,setTranscode]=useState<Transcode>({mode:'copy'});
 const [policy,setPolicy]=useState<InputPolicy>(emptyPolicy),[assets,setAssets]=useState<MediaAsset[]>([]);
 const [devices,setDevices]=useState<Devices|null>(null),[cap,setCap]=useState<Capabilities|null>(null),[error,setError]=useState(''),[message,setMessage]=useState(''),[busy,setBusy]=useState(false),[dirty,setDirty]=useState(false),[reload,setReload]=useState(0);
 useEffect(()=>{
  if(!live.admin)return;
  let disposed=false;setError('');setProfile(null);
  void live.call('GET','manual-channels/processing?id='+encodeURIComponent(id)).then((value:Profile)=>{if(disposed)return;if(!value.transcode||!Array.isArray(value.inputs))throw new Error('Configuração do canal indisponível.');setProfile(value);setInputs(value.inputs.map(v=>({...v,url:''})));setTranscode(value.transcode);setPolicy(value.policy||emptyPolicy);setDirty(false)}).catch(e=>{if(!disposed)setError(e.message)});
  return()=>{disposed=true};
 },[id,live.admin,live.call,reload]);
 useEffect(()=>{if(!live.admin||tab!=='transcoder')return;let disposed=false;void live.call('GET','manual-channels/encoders').then(v=>{if(!disposed)setDevices(v)}).catch(()=>{if(!disposed)setError('Não foi possível verificar os codificadores. Sem transcodificar continua disponível.')});return()=>{disposed=true}},[tab,live.admin,live.call,reload]);
 useEffect(()=>{if(!live.admin||!inputs.some(v=>v.protocol==='udp'))return;let disposed=false;void live.call('GET','live/capabilities').then(v=>{if(!disposed)setCap(v)}).catch(()=>{if(!disposed)setError('Interfaces UDP indisponíveis. Recarregue antes de salvar.')});return()=>{disposed=true}},[inputs.some(v=>v.protocol==='udp'),live.call,live.admin]);
 // Only the indicator is refreshed; polling never overwrites an unsaved draft.
 useEffect(()=>{if(!live.admin||tab!=='input')return;let disposed=false;const timer=setInterval(()=>{void live.call('GET','manual-channels/processing?id='+encodeURIComponent(id)).then((v:Profile)=>{if(!disposed)setProfile(old=>old?{...old,active_key:v.active_key}:old)}).catch(()=>{})},5000);return()=>{disposed=true;clearInterval(timer)}},[id,tab,live.call,live.admin]);
 useEffect(()=>{if(!live.admin)return;let gone=false;void live.call('GET','manual-channels/assets').then(v=>{if(!gone)setAssets(v.assets||[])}).catch(()=>{});return()=>{gone=true}},[live.admin,live.call]);
 usePendingEdits(dirty);
 function editPolicy(p:InputPolicy){setPolicy(p);setDirty(true);setMessage('')}
 function imported(a:MediaAsset){setAssets(old=>old.some(v=>v.id===a.id)?old:[...old,a])}
 function publication(enabled:boolean){if(enabled&&inputs.length>=8){setError('Remova uma entrada antes de habilitar a publicação.');return}if(!enabled&&inputs.every(v=>v.protocol==='publish')){setError('Adicione outra entrada antes de desabilitar a publicação.');return}editPolicy({...policy,publication:enabled});setInputs(old=>enabled&&!old.some(v=>v.protocol==='publish')?[...old,{key:'new-'+crypto.randomUUID(),url:'publish://incoming',protocol:'publish',interface:'',local_ip:''}]:enabled?old:old.filter(v=>v.protocol!=='publish'))}
 function editInput(index:number,patch:Partial<Input>){setInputs(old=>old.map((v,i)=>i===index?{...v,...patch}:v));setDirty(true);setMessage('')}
 function move(index:number,offset:number){setInputs(old=>{const next=[...old];[next[index],next[index+offset]]=[next[index+offset],next[index]];return next});setDirty(true);setMessage('')}
 async function save(event:React.FormEvent){
  event.preventDefault();if(busy||blocked||!profile||!live.admin)return;
  setBusy(true);setError('');setMessage('');
  try{
   const body=tab==='input'?{policy:{...policy,token:undefined},inputs:inputs.map(({address,...v})=>({...v,key:v.key.startsWith('new-')?'':v.key}))}:{transcode:transcode.mode==='copy'?{mode:'copy'}:transcode};
   const value:Profile=await live.call('POST','manual-channels',{action:tab==='input'?'inputs':'transcode',revision:profile.revision,source:{id},confirmed:true,...body});
   setProfile(value);setInputs(value.inputs.map(v=>({...v,url:''})));setTranscode(value.transcode);setPolicy(value.policy||emptyPolicy);setDirty(false);setMessage('Configuração salva. Se o canal estava recebendo, somente ele foi reiniciado; canais parados permanecem parados.');await live.refresh();
  }catch(e){setError((e as Error).message)}finally{setBusy(false)}
 }
 if(!live.admin)return <section className="mc-panel"><h2>{tab==='input'?'Entradas':'Transcodificação'}</h2><p>Somente administradores podem consultar e editar esta configuração.</p></section>;
 const disabled=busy||blocked;
 return <form className="mc-panel mc-processing" aria-label={tab==='input'?'Configuração de entradas':'Configuração de transcodificação'} onSubmit={e=>void save(e)}>
  {error&&<p role="alert" className="media-notice">{error}</p>}{message&&<p role="status" className="media-notice">{message}</p>}
  {!profile?<><p>Carregando configuração…</p><button type="button" onClick={()=>setReload(n=>n+1)}>Tentar novamente</button></>:tab==='input'?<>
   <h2>Entradas do canal</h2>
   {profile.local?<><p className="mc-muted">A primeira URL tem prioridade ao salvar. Apenas uma origem é recebida por vez. Após falha ou ausência de pacotes, o Edge tenta a próxima, mantendo a origem saudável até nova falha. URLs salvas ficam protegidas; deixe o campo vazio para preservá-las.</p>
    <div className="mc-inputs">{inputs.map((v,index)=><section className="mc-input-row" key={v.key} aria-label={'Entrada '+(index+1)}>
     <div className="mc-input-priority"><strong>URL {index+1}</strong><span className={'media-status '+(v.key&&v.key===profile.active_key?'online':'stopped')}>{v.key&&v.key===profile.active_key?'Selecionada':index===0?'Prioritária':'Reserva'}</span><div><button type="button" aria-label={'Subir entrada '+(index+1)} disabled={disabled||index===0} onClick={()=>move(index,-1)}><ArrowUp size={13}/></button><button type="button" aria-label={'Descer entrada '+(index+1)} disabled={disabled||index===inputs.length-1} onClick={()=>move(index,1)}><ArrowDown size={13}/></button></div></div>
     <div className="mc-input-fields"><label>URL da entrada {index+1}{v.address&&<span className="mc-saved-address">Atual: {v.address}</span>}<input aria-label={'URL da entrada '+(index+1)} type="url" required={!v.key||v.key.startsWith('new-')} autoComplete="off" spellCheck={false} maxLength={4096} placeholder={v.address||'https://matriz/canal/mpegts'} value={v.url||''} disabled={disabled||v.protocol==='publish'} onChange={e=>{const protocol=detectLink(e.target.value);editInput(index,{url:e.target.value,protocol:v.protocol==='cluster'?'cluster':protocol||v.protocol,interface:protocol==='udp'?v.interface:'',local_ip:protocol==='udp'?v.local_ip:''})}}/></label><details className="mc-input-options"><summary>Opções da entrada {index+1}</summary><label>Protocolo da entrada {index+1}<select value={v.protocol} disabled={disabled} onChange={e=>{if(e.target.value==='publish'){editPolicy({...policy,publication:true});editInput(index,{protocol:'publish',url:'publish://incoming',interface:'',local_ip:''})}else editInput(index,{protocol:e.target.value,url:v.protocol==='publish'?'':v.url,interface:'',local_ip:''})}}>{['http_ts','hls','udp','srt','cluster','publish'].map(p=><option key={p} value={p}>{label(p)}</option>)}</select></label><label>Tempo sem sinal da entrada {index+1} (segundos)<input type="number" min={2} max={300} value={v.source_timeout||''} placeholder="Padrão do receptor" disabled={disabled} onChange={e=>editInput(index,{source_timeout:Number(e.target.value)})}/></label><p className="mc-muted">Sem payload, encerra a tentativa e tenta a próxima URL. Vazio preserva o limite atual.</p></details>
     {v.protocol==='cluster'&&<><label>Token do Edge de origem<input type="password" value={v.token||''} autoComplete="off" maxLength={48} disabled={disabled} placeholder="Preservado quando a URL fica vazia" onChange={e=>editInput(index,{token:e.target.value})}/></label><label>SHA256 do certificado (opcional)<input value={v.certificate||''} maxLength={64} disabled={disabled} onChange={e=>editInput(index,{certificate:e.target.value.toLowerCase().replace(/:/g,'')})}/></label></>}
     {v.protocol==='udp'&&<><label>Interface da entrada {index+1}<select required value={v.interface} disabled={disabled} onChange={e=>editInput(index,{interface:e.target.value,local_ip:''})}><option value="">Escolha a interface</option>{cap?.interfaces.filter(v=>v.up&&v.ipv4.length).map(v=><option key={v.name}>{v.name}</option>)}</select></label><label>IP local da entrada {index+1}<select required value={v.local_ip} disabled={disabled} onChange={e=>editInput(index,{local_ip:e.target.value})}><option value="">Confirme o IP</option>{cap?.interfaces.find(i=>i.name===v.interface)?.ipv4.map(ip=><option key={ip}>{ip}</option>)}</select></label></>}
     </div><button type="button" aria-label={'Remover entrada '+(index+1)} disabled={disabled||inputs.length===1} onClick={()=>{setInputs(old=>old.filter((_,i)=>i!==index));setDirty(true);setMessage('')}}><Trash2 size={15}/></button>
    </section>)}</div>
    <button type="button" disabled={disabled||inputs.length>=8} onClick={()=>{setInputs(old=>[...old,{key:'new-'+crypto.randomUUID(),url:'',protocol:'http_ts',interface:'',local_ip:''}]);setDirty(true)}}><Plus size={13}/>Adicionar URL</button><p className="mc-muted">Até 8 entradas. A indicação “Selecionada” identifica a última origem aberta; o estado Recebendo e o bitrate confirmam a chegada do sinal. Alterações podem causar um breve corte apenas neste canal.</p>
    <InputPolicyControls id={id} value={policy} onChange={editPolicy} onPublication={publication} disabled={disabled} call={live.call} assets={assets} onImported={imported} transcoding={transcode.mode!=='copy'}/>

   </>:<p>As entradas deste canal são administradas pelo Kore Play. Cadastre um canal local para configurar URLs independentes.</p>}
  </>:<>
   <h2>Processamento do canal</h2><div className="mc-processing-mode"><label><input type="radio" name="processing-mode" checked={transcode.mode==='copy'} disabled={disabled} onChange={()=>{setTranscode({mode:'copy'});setDirty(true)}}/>Transmitir sem transcodificar</label><label><input type="radio" name="processing-mode" checked={transcode.mode==='h264'} disabled={disabled||!devices?.devices?.length} onChange={()=>{setTranscode({...defaults});setDirty(true)}}/>Transcodificar áudio e vídeo</label></div>
   <p className="mc-muted">Sem transcodificar mantém os codecs originais e não inicia codificador. A seleção de GPU exige uma codificação sintética bem-sucedida com o mesmo usuário do serviço. Drivers não são instalados pelo painel.</p>
   {transcode.mode==='h264'&&<TranscodeControls value={transcode} onChange={value=>{setTranscode(value);if(value.mode!=='copy'&&policy.backup_asset)setPolicy(p=>({...p,backup_transcode:true}));setDirty(true);setMessage('')}} disabled={disabled} devices={devices?.devices} assets={assets} call={live.call} onImported={imported}/>}
   {devices&&<p className="mc-muted">{devices.devices?.length?devices.devices.length+' dispositivo(s) validado(s).':'Nenhum codificador utilizável detectado.'} Limite: {devices.max_concurrent||2} canais transcodificados simultâneos; demais aguardam recursos. {devices.warning}</p>}
  </>}
  {dirty&&<p role="status" className="mc-pending">Alterações não salvas. Use Salvar configuração antes de sair.</p>}
  <div className="mc-form-actions">{profile&&(tab==='transcoder'||profile.local)&&<button type="submit" className="mc-primary" disabled={disabled||!dirty||tab==='transcoder'&&transcode.mode==='h264'&&!devices?.devices?.some(d=>d.id===transcode.device)}>{busy?'Salvando…':'Salvar configuração'}</button>}<button type="button" disabled={disabled} onClick={()=>{if(!dirty||window.confirm('Descartar as alterações não salvas?')){setReload(n=>n+1);setMessage('')}}}>Recarregar configuração</button></div>
 </form>;
}
