import type {Transcode} from './transcode-controls';

// Structural validation protects rendering; authoritative ranges remain on the API.
export function readTranscodeClipboard(raw:string):Transcode{
 if(raw.length>32768)throw new Error('oversize');const value=JSON.parse(raw);
 if(!value||typeof value!=='object'||Array.isArray(value))throw new Error('shape');
 if(value.mode==='copy')return {mode:'copy'};
 if(value.mode!=='h264')throw new Error('mode');
 const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v);
 const fields=(v:Record<string,unknown>,strings:string[],numbers:string[],booleans:string[],arrays:string[])=>{
  for(const [key,item] of Object.entries(v)){
   if(strings.includes(key)&&typeof item==='string')continue;
   if(numbers.includes(key)&&typeof item==='number'&&Number.isFinite(item))continue;
   if(booleans.includes(key)&&typeof item==='boolean')continue;
   if(arrays.includes(key)&&Array.isArray(item))continue;
   throw new Error('field');
  }
 };
 fields(value,['mode','device','audio','deinterlace','crop','pixel_format'],['video_kbps','width','height','fps','audio_kbps','sample_rate','channels','volume_db','gop'],['video_copy','split_channels'],['tracks']);
 if(value.tracks){if(value.tracks.length<1||value.tracks.length>3)throw new Error('tracks');for(const track of value.tracks){
  if(!object(track))throw new Error('track');fields(track,['codec','profile','preset','level','sar','resize','background','logo','logo_position'],['video_kbps','width','height','bframes','refs','logo_width','logo_height','logo_margin'],['interlace','open_gop'],['extended']);
  if(track.extended){if(!Array.isArray(track.extended)||track.extended.length>4)throw new Error('options');for(const option of track.extended){if(!object(option))throw new Error('option');fields(option,['key','value'],[],[],[])}}
 }}
 return value;
}
