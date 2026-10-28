import {expect,it} from 'vitest';
import {mediaRows,mediaState,filterMedia,mediaRate} from './media-model';
import type {LiveChannel,LiveStream} from './live-distribution';
const channels:LiveChannel[]=[{id:'one',name:'Canal 10',category:'Notícias',protocols:[{id:'hls',label:'HLS',path:'/synthetic'}]},{id:'two',name:'Canal 2',category:'Música',protocols:[]}];
it('distinguishes a fresh stream, stale reception, deliberate stop and denied grant',()=>{
 const now=Date.now(),stream={channel_id:'one',state:'running',metrics:{last_packet_at:new Date(now).toISOString()}} as LiveStream;
 const row=mediaRows(channels,[],[stream])[0];
 expect(mediaState(row,now)).toBe('online');
 expect(mediaState(row,now+5000)).toBe('waiting');
 expect(mediaState({...row,stream:{...stream,state:'unauthorized'}},now)).toBe('error');
 expect(mediaState({...row,stream:undefined,config:{channel_id:'one',protocol:'hls',hls:false,resume:false,auto_paused:true}},now)).toBe('disabled');
});
it('searches advertised inputs and orders names naturally without changing the catalog',()=>{
 const rows=mediaRows(channels,[],[]);
 expect(filterMedia(rows,'HLS','all','asc').map(r=>r.channel.id)).toEqual(['one']);
 expect(filterMedia(rows,'','all','asc').map(r=>r.channel.id)).toEqual(['two','one']);
 expect(channels.map(c=>c.id)).toEqual(['one','two']);
 expect(mediaRate(null)).toBe('Não medido');
 expect(mediaRate(800000)).toBe('800.0 kbps');
});
