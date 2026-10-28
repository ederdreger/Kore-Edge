import {useEffect,useState} from 'react';
import {ArrowDown,ArrowUp,Plus,Trash2} from 'lucide-react';
import {detectLink} from './manual-channel-editor';
import {stateLabel,failureLabel,type LiveStream,type Capabilities,useLiveDistribution} from './live-distribution';

type Live=ReturnType<typeof useLiveDistribution>;
type Input={key:string;address?:string;url?:string;protocol:string;interface:string;local_ip:string};
export type Transcode={mode:'copy'|'h264';device?:string;video_kbps?:number;width?:number;height?:number;fps?:number;audio?:'copy'|'aac';audio_kbps?:number};
type MediaInfo={video?:string;audio?:string;width?:number;height?:number;measured_at:string};
type Profile={input_media?:MediaInfo;output_media?:MediaInfo;revision:number;inputs:Input[];transcode:Transcode;active_key:string;local:boolean};
type Devices={devices:{id:string;name:string}[];max_concurrent:number;warning:string};
type Props={id:string;live:Live;blocked:boolean;tab:'input'|'transcoder'};
const defaults:Transcode={mode:'h264',device:'',video_kbps:3000,width:1280,height:720,fps:25,audio:'aac',audio_kbps:128};
const label=(p:string)=>({http_ts:'HTTP MPEG-TS',hls:'HLS',udp:'UDP MPEG-TS',srt:'SRT'}[p]||'Detectar pelo link');

function MeasuredMedia({info,online}:{info?:MediaInfo;online:boolean}){
 if(!online||!info)return <p>Codecs e resolução: {online?'Aguardando medição do fluxo':'Sem medição atual'}</p>;
 return <><p>Vídeo: {info.video?.toUpperCase()||'Não identificado'}{info.width&&info.height?` · ${info.width} × ${info.height}`:''}</p><p>Áudio: {info.audio?.toUpperCase()||'Não identificado'}</p></>;
}
export function ChannelProcessingSummary({id,live,stream,fallbackProtocol}:{id:string;live:Live;stream?:LiveStream;fallbackProtocol:string}){
 const [value,setValue]=useState<Profile|null>(null);
 useEffect(()=>{if(!live.admin)return;let disposed=false;setValue(null);const load=()=>{void live.call('GET','manual-channels/processing?id='+encodeURIComponent(id)).then(v=>{if(!disposed)setValue(v)}).catch(()=>{if(!disposed)setValue(null)})};load();const timer=setInterval(load,5000);return()=>{disposed=true;clearInterval(timer)}},[id,live.admin,live.call]);
 const active=value?.inputs?.find(v=>v.key===value.active_key);
 const online=stream?.state==='running';
 return <><div><h2>Informações de entrada</h2><p>{active?label(active.protocol):value?.local?'Entrada ativa ainda não identificada':label(fallbackProtocol)}</p><MeasuredMedia info={value?.input_media} online={online}/><p>Interrupções: {stream?.interruptions??'Não medido'}</p><p>Última queda: {failureLabel(stream?.last_failure_reason)}</p></div><div><h2>Informações de saída</h2><MeasuredMedia info={value?.output_media} online={online}/><p>MPEG-TS: {online?'Disponível':'Sem recepção'}</p><p>HLS: {stateLabel(stream?.hls_state)}</p><p>UDP: {stateLabel(stream?.udp_state)}</p><p>{value?.transcode?.mode==='copy'?'Sem transcodificação':value?.transcode?.mode==='h264'?'Perfil H.264 · '+value.transcode.device:'Processamento: indisponível nesta consulta'}</p>{active&&<p>Origem selecionada: {label(active.protocol)} · URL {value!.inputs.indexOf(active)+1}</p>}<p>Codecs medidos em amostra real; bitrate no fluxo compartilhado, após processamento quando ativo.</p></div></>;

}

export function ChannelProcessing({id,live,blocked,tab}:Props){
 const [profile,setProfile]=useState<Profile|null>(null),[inputs,setInputs]=useState<Input[]>([]),[transcode,setTranscode]=useState<Transcode>({mode:'copy'});
 const [devices,setDevices]=useState<Devices|null>(null),[cap,setCap]=useState<Capabilities|null>(null),[error,setError]=useState(''),[message,setMessage]=useState(''),[busy,setBusy]=useState(false),[dirty,setDirty]=useState(false),[reload,setReload]=useState(0);
 useEffect(()=>{
  if(!live.admin)return;
  let disposed=false;setError('');setProfile(null);
  void live.call('GET','manual-channels/processing?id='+encodeURIComponent(id)).then((value:Profile)=>{if(disposed)return;if(!value.transcode||!Array.isArray(value.inputs))throw new Error('Configuração do canal indisponível.');setProfile(value);setInputs(value.inputs.map(v=>({...v,url:''})));setTranscode(value.transcode);setDirty(false)}).catch(e=>{if(!disposed)setError(e.message)});
  return()=>{disposed=true};
 },[id,live.admin,live.call,reload]);
 useEffect(()=>{if(!live.admin||tab!=='transcoder')return;let disposed=false;void live.call('GET','manual-channels/encoders').then(v=>{if(!disposed)setDevices(v)}).catch(()=>{if(!disposed)setError('Não foi possível verificar os codificadores. Sem transcodificar continua disponível.')});return()=>{disposed=true}},[tab,live.admin,live.call,reload]);
 useEffect(()=>{if(!live.admin||!inputs.some(v=>v.protocol==='udp'))return;let disposed=false;void live.call('GET','live/capabilities').then(v=>{if(!disposed)setCap(v)}).catch(()=>{if(!disposed)setError('Interfaces UDP indisponíveis. Recarregue antes de salvar.')});return()=>{disposed=true}},[inputs.some(v=>v.protocol==='udp'),live.call,live.admin]);
 // Only the indicator is refreshed; polling never overwrites an unsaved draft.
 useEffect(()=>{if(!live.admin||tab!=='input')return;let disposed=false;const timer=setInterval(()=>{void live.call('GET','manual-channels/processing?id='+encodeURIComponent(id)).then((v:Profile)=>{if(!disposed)setProfile(old=>old?{...old,active_key:v.active_key}:old)}).catch(()=>{})},5000);return()=>{disposed=true;clearInterval(timer)}},[id,tab,live.call,live.admin]);
 function editInput(index:number,patch:Partial<Input>){setInputs(old=>old.map((v,i)=>i===index?{...v,...patch}:v));setDirty(true);setMessage('')}
 function move(index:number,offset:number){setInputs(old=>{const next=[...old];[next[index],next[index+offset]]=[next[index+offset],next[index]];return next});setDirty(true);setMessage('')}
 function editTranscode(patch:Partial<Transcode>){setTranscode(old=>({...old,...patch}));setDirty(true);setMessage('')}
 async function save(event:React.FormEvent){
  event.preventDefault();if(busy||blocked||!profile||!live.admin)return;
  setBusy(true);setError('');setMessage('');
  try{
   const body=tab==='input'?{inputs:inputs.map(({address,...v})=>({...v,key:v.key.startsWith('new-')?'':v.key}))}:{transcode:transcode.mode==='copy'?{mode:'copy'}:transcode};
   const value:Profile=await live.call('POST','manual-channels',{action:tab==='input'?'inputs':'transcode',revision:profile.revision,source:{id},confirmed:true,...body});
   setProfile(value);setInputs(value.inputs.map(v=>({...v,url:''})));setTranscode(value.transcode);setDirty(false);setMessage('Configuração salva. Se o canal estava recebendo, somente ele foi reiniciado; canais parados permanecem parados.');await live.refresh();
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
     <div className="mc-input-fields"><label>URL da entrada {index+1}<input type="url" required={!v.key||v.key.startsWith('new-')} autoComplete="off" spellCheck={false} maxLength={4096} placeholder={v.address||'https://matriz/canal/mpegts'} value={v.url||''} disabled={disabled} onChange={e=>{const protocol=detectLink(e.target.value);editInput(index,{url:e.target.value,protocol:protocol||v.protocol,interface:protocol==='udp'?v.interface:'',local_ip:protocol==='udp'?v.local_ip:''})}}/></label><label>Protocolo da entrada {index+1}<select value={v.protocol} disabled={disabled} onChange={e=>editInput(index,{protocol:e.target.value,interface:'',local_ip:''})}>{['http_ts','hls','udp','srt'].map(p=><option key={p} value={p}>{label(p)}</option>)}</select></label>
     {v.protocol==='udp'&&<><label>Interface da entrada {index+1}<select required value={v.interface} disabled={disabled} onChange={e=>editInput(index,{interface:e.target.value,local_ip:''})}><option value="">Escolha a interface</option>{cap?.interfaces.filter(v=>v.up&&v.ipv4.length).map(v=><option key={v.name}>{v.name}</option>)}</select></label><label>IP local da entrada {index+1}<select required value={v.local_ip} disabled={disabled} onChange={e=>editInput(index,{local_ip:e.target.value})}><option value="">Confirme o IP</option>{cap?.interfaces.find(i=>i.name===v.interface)?.ipv4.map(ip=><option key={ip}>{ip}</option>)}</select></label></>}
     </div><button type="button" aria-label={'Remover entrada '+(index+1)} disabled={disabled||inputs.length===1} onClick={()=>{setInputs(old=>old.filter((_,i)=>i!==index));setDirty(true)}}><Trash2 size={15}/></button>
    </section>)}</div>
    <button type="button" disabled={disabled||inputs.length>=8} onClick={()=>{setInputs(old=>[...old,{key:'new-'+crypto.randomUUID(),url:'',protocol:'http_ts',interface:'',local_ip:''}]);setDirty(true)}}><Plus size={13}/>Adicionar URL</button><p className="mc-muted">Até 8 entradas. A indicação “Selecionada” identifica a última origem aberta; o estado Recebendo e o bitrate confirmam a chegada do sinal. Alterações podem causar um breve corte apenas neste canal.</p>
   </>:<p>As entradas deste canal são administradas pelo Kore Play. Cadastre um canal local para configurar URLs independentes.</p>}
  </>:<>
   <h2>Processamento do canal</h2><div className="mc-processing-mode"><label><input type="radio" name="processing-mode" checked={transcode.mode==='copy'} disabled={disabled} onChange={()=>{setTranscode({mode:'copy'});setDirty(true)}}/>Transmitir sem transcodificar</label><label><input type="radio" name="processing-mode" checked={transcode.mode==='h264'} disabled={disabled||!devices?.devices?.length} onChange={()=>{setTranscode({...defaults});setDirty(true)}}/>Transcodificar áudio e vídeo</label></div>
   <p className="mc-muted">Sem transcodificar mantém os codecs originais e não inicia codificador. A seleção de GPU exige uma codificação sintética bem-sucedida com o mesmo usuário do serviço. Drivers não são instalados pelo painel.</p>
   {transcode.mode==='h264'&&<><section className="mc-panel"><h2>Vídeo · faixa única H.264</h2><div className="mc-transcode-grid">
    <label>Dispositivo<select required aria-label="Dispositivo" value={transcode.device||''} disabled={disabled||!devices} onChange={e=>editTranscode({device:e.target.value})}><option value="">Escolha o dispositivo</option>{devices?.devices?.map(d=><option key={d.id} value={d.id}>{d.name}</option>)}{transcode.device&&!devices?.devices?.some(d=>d.id===transcode.device)&&<option value={transcode.device} disabled>{transcode.device} · indisponível</option>}</select></label>
    {(['video_kbps','width','height','fps'] as const).map(field=><label key={field}>{{video_kbps:'Bitrate de vídeo (kbps)',width:'Largura',height:'Altura',fps:'Quadros por segundo'}[field]}<input type="number" required min={{video_kbps:200,width:160,height:90,fps:1}[field]} max={{video_kbps:30000,width:3840,height:2160,fps:60}[field]} step={field==='width'||field==='height'?2:1} value={transcode[field]||''} disabled={disabled} onChange={e=>editTranscode({[field]:Number(e.target.value)})}/></label>)}
   </div><p className="mc-muted">Proporção preservada com bordas quando necessário. GOP de 2 segundos; sem quadros B. Decodificação por CPU, codificação no dispositivo escolhido.</p></section>
   <section className="mc-panel"><h2>Áudio</h2><div className="mc-transcode-grid"><label>Tratamento de áudio<select value={transcode.audio||'copy'} disabled={disabled} onChange={e=>editTranscode({audio:e.target.value as 'copy'|'aac',audio_kbps:e.target.value==='aac'?128:0})}><option value="copy">Copiar áudio original</option><option value="aac">AAC · 48 kHz · estéreo</option></select></label>{transcode.audio==='aac'&&<label>Bitrate de áudio (kbps)<input type="number" min={32} max={320} required value={transcode.audio_kbps||128} disabled={disabled} onChange={e=>editTranscode({audio_kbps:Number(e.target.value)})}/></label>}</div></section></>}
   {devices&&<p className="mc-muted">{devices.devices?.length?devices.devices.length+' dispositivo(s) validado(s).':'Nenhum codificador utilizável detectado.'} Limite: {devices.max_concurrent||2} canais transcodificados simultâneos; demais aguardam recursos. {devices.warning}</p>}
  </>}
  <div className="mc-form-actions">{profile&&(tab==='transcoder'||profile.local)&&<button type="submit" className="mc-primary" disabled={disabled||!dirty||tab==='transcoder'&&transcode.mode==='h264'&&!devices?.devices?.some(d=>d.id===transcode.device)}>{busy?'Salvando…':'Salvar configuração'}</button>}<button type="button" disabled={disabled} onClick={()=>{if(!dirty||window.confirm('Descartar as alterações não salvas?')){setReload(n=>n+1);setMessage('')}}}>Recarregar configuração</button></div>
 </form>;
}
