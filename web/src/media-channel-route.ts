export type ChannelTab='overview'|'input'|'output';
export type MediaRoute={kind:'list'}|{kind:'new'}|{kind:'channel';id:string;tab:ChannelTab};
export const channelPath=(id:string,tab:ChannelTab='overview')=>'#media/channel/'+encodeURIComponent(id)+'/'+tab;
export function readMediaRoute(hash=typeof window==='undefined'?'':window.location.hash):MediaRoute{
 if(hash==='#media/new')return {kind:'new'};
 const match=/^#media\/channel\/([a-zA-Z0-9_-]{1,80})(?:\/(overview|input|output))?$/.exec(hash);
 return match?{kind:'channel',id:match[1],tab:(match[2]||'overview') as ChannelTab}:{kind:'list'};
}
