import {useCallback,useEffect,useRef,useState} from 'react';
import {useAccount} from './account-context';
export type LiveChannel={id:string;name:string;category:string;outputs?:string[];protocols:{id:string;label:string;path:string}[]};
export type Target={channel_id:string;interface:string;source_ip:string;group:string;port:number;ttl:number};
export type LiveConfig={channel_id:string;protocol:string;udp?:Target;hls:boolean;resume:boolean};
export type Tools={ffmpeg:string;ffprobe:string};
export type LiveStream={channel_id:string;protocol:string;state:string;error?:string;startup_ms:number|null;attempts:number;started_at:string;udp_state:string;hls_state:string;metrics:{transport?:{cc_state:string;cc_errors:number;tei:number;malformed:number;pcr_state:string};bits_per_second:number|null;bytes:number;subscribers:number;last_packet_at:string}};
export type LiveView={manual_revision?:number;configuration:{generation:number;tools:Tools;channels:LiveConfig[]};catalog:{available:boolean;configured:boolean;last_sync:string;catalog:null|{partner_name:string;channels:LiveChannel[];srt?:{port:number;latency_ms:number}}};streams:LiveStream[];max_active:number};
export type Capabilities={interfaces_available?:boolean;warning?:string;tools:Tools;platform:string;interfaces:{name:string;up:boolean;multicast:boolean;ipv4:string[]}[]};
export const stateLabel=(state?:string)=>({running:'Recebendo',starting:'Iniciando',backoff:'Reconectando',failed:'Falha',unauthorized:'Não autorizado',stopped:'Parado',sending:'Enviando',disabled:'Desativada',warming:'Preparando segmentos',validating_codecs:'Verificando codecs',codec_or_tools_unavailable:'Codecs ou ferramentas incompatíveis',interface_or_permission_unavailable:'Interface ou permissão indisponível'}[state||'']||state||'Não iniciada');
export function useLiveDistribution(){
 const account=useAccount(),request=useRef(account?.request);request.current=account?.request;
 const [view,setView]=useState<LiveView|null>(null),[error,setError]=useState(''),[busy,setBusy]=useState(false);
 const lock=useRef(false),mounted=useRef(true);
 const call=useCallback(async(method:string,path:string,body?:unknown)=>{if(!request.current)throw new Error('Entre no painel.');const r=await request.current(method,'/api/v1/'+path,body);const value=await r.json().catch(()=>({}));if(!r.ok)throw new Error(value.error||'Distribuição indisponível. Confira se este servidor foi atualizado.');return value},[]);
 const refresh=useCallback(async()=>{if(lock.current)return;lock.current=true;try{const data=await call('GET','live');if(mounted.current){setView(data);setError('')}}catch(e){if(mounted.current){setView(null);setError((e as Error).message)}}finally{lock.current=false}},[call]);
 useEffect(()=>{mounted.current=true;void refresh();const timer=setInterval(()=>void refresh(),3000);return()=>{mounted.current=false;clearInterval(timer)}},[refresh]);
 async function action(body:unknown){setBusy(true);try{const data=await call('POST','live/action',body);if(mounted.current){setView(data);setError('')}return data}finally{if(mounted.current)setBusy(false)}}
 return {view,error,busy,call,refresh,action,admin:account?.user?.role==='admin'};
}
