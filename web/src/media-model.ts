import type {LiveChannel, LiveConfig, LiveStream} from './live-distribution';

export type MediaState = 'online' | 'waiting' | 'disabled' | 'error';
export type MediaRow = {channel:LiveChannel; config?:LiveConfig; stream?:LiveStream};
export const receptionRequested=(row:MediaRow)=>!!row.stream&&['running','starting','backoff','queued','waiting_capacity','waiting_hls_capacity','waiting_transcode_capacity'].includes(row.stream.state);
export function mediaState(row:MediaRow,now=Date.now()):MediaState{
 const s=row.stream;
 if(s&&['failed','unauthorized'].includes(s.state))return 'error';
 if(s?.state==='running'&&now-new Date(s.metrics.last_packet_at).getTime()<3000)return 'online';
 if(receptionRequested(row)||row.config?.resume&&!row.config.auto_paused)return 'waiting';
 return 'disabled';
}
export function mediaRows(channels:LiveChannel[],configs:LiveConfig[],streams:LiveStream[]):MediaRow[]{
 const configuration=new Map(configs.map(c=>[c.channel_id,c])),status=new Map(streams.map(s=>[s.channel_id,s]));
 return channels.map(channel=>({channel,config:configuration.get(channel.id),stream:status.get(channel.id)}));
}
export function filterMedia(rows:MediaRow[],query:string,state:string,order:string,now=Date.now()){
 const search=query.trim().toLocaleLowerCase('pt-BR');
 return rows.filter(r=>(r.channel.name+' '+r.channel.category+' '+(r.config?.protocol||'')+' '+r.channel.protocols.map(p=>p.id).join(' ')).toLocaleLowerCase('pt-BR').includes(search)&&(state==='all'||mediaState(r,now)===state))
 .sort((a,b)=>order==='bitrate'?(b.stream?.metrics.bits_per_second??-1)-(a.stream?.metrics.bits_per_second??-1):a.channel.name.localeCompare(b.channel.name,'pt-BR',{numeric:true})*(order==='desc'?-1:1));
}
export const mediaRate=(rate?:number|null)=>rate==null?'Não medido':rate>=1e6?(rate/1e6).toFixed(2)+' Mbps':(rate/1000).toFixed(1)+' kbps';
export const mediaDuration=(seconds:number)=>seconds>=86400?Math.floor(seconds/86400)+' d '+Math.floor(seconds%86400/3600)+' h':seconds>=3600?Math.floor(seconds/3600)+' h '+Math.floor(seconds%3600/60)+' min':Math.max(0,Math.floor(seconds/60))+' min';
