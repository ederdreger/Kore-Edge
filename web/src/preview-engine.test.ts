// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {previewPath,startPreview,type PreviewStatus} from './preview-engine';

const mocks=vi.hoisted(()=>({
 hlsOptions:vi.fn(),hlsSource:vi.fn(),hlsAttach:vi.fn(),hlsDestroy:vi.fn(),hlsSupported:true,hlsHandlers:new Map(),
 tsCreate:vi.fn(),tsLoad:vi.fn(),tsAttach:vi.fn(),tsDestroy:vi.fn(),tsSupported:true,tsHandlers:new Map(),
}));
vi.mock('hls.js',()=>({default:class {
 static Events={ERROR:'error',MANIFEST_PARSED:'manifest'};
 static isSupported(){return mocks.hlsSupported}
 constructor(options:unknown){mocks.hlsOptions(options)}
 on(event:string,handler:unknown){mocks.hlsHandlers.set(event,handler)}
 loadSource(url:string){mocks.hlsSource(url)}
 attachMedia(video:HTMLVideoElement){mocks.hlsAttach(video)}
 destroy(){mocks.hlsDestroy()}
}}));
vi.mock('mpegts.js',()=>({default:{
 LoggingControl:{enableAll:true},Events:{ERROR:'error'},getFeatureList:()=>({mseLivePlayback:mocks.tsSupported}),
 createPlayer:(...args:unknown[])=>{mocks.tsCreate(...args);return {on:(event:string,handler:unknown)=>mocks.tsHandlers.set(event,handler),attachMediaElement:mocks.tsAttach,load:mocks.tsLoad,destroy:mocks.tsDestroy}},
}}));
let video:HTMLVideoElement,stop:undefined|(()=>void);
const source={id:'synthetic-one',name:'Synthetic channel',protocol:'hls' as const};
beforeEach(()=>{
 vi.clearAllMocks();mocks.hlsHandlers.clear();mocks.tsHandlers.clear();mocks.hlsSupported=true;mocks.tsSupported=true;
 video=document.createElement('video');
 vi.spyOn(video,'canPlayType').mockReturnValue('');vi.spyOn(video,'play').mockResolvedValue();vi.spyOn(video,'pause').mockImplementation(()=>{});vi.spyOn(video,'load').mockImplementation(()=>{});
});
afterEach(()=>{stop?.();stop=undefined;vi.restoreAllMocks();vi.useRealTimers()});
it('builds only fixed local authorized media paths',()=>{
 expect(previewPath(source)).toBe('/api/v1/kore-play/media/synthetic-one/hls/index.m3u8');
 expect(()=>previewPath({...source,id:'../private'})).toThrow();
 expect(previewPath({...source,protocol:'http_ts'})).toContain('/http_ts/stream.ts');
 expect(previewPath({...source,protocol:'http_ts',scope:'live'})).toBe('/api/v1/live/media/synthetic-one/stream.ts');
 expect(previewPath({...source,scope:'output'})).toBe('/api/v1/live/hls/synthetic-one/index.m3u8');
});
it('loads HLS lazily, restricts nested requests and releases the stream on pause',async()=>{
 const statuses:PreviewStatus[]=[];stop=startPreview(video,source,s=>statuses.push(s));
 await vi.waitFor(()=>expect(mocks.hlsAttach).toHaveBeenCalledWith(video));
 expect(mocks.hlsSource).toHaveBeenCalledWith(previewPath(source));
 const options=mocks.hlsOptions.mock.calls[0][0];expect(options.enableWorker).toBe(false);
 const xhr={withCredentials:false};options.xhrSetup(xhr,previewPath(source));expect(xhr.withCredentials).toBe(true);
 expect(()=>options.xhrSetup(xhr,'https://untrusted.invalid/segment.ts')).toThrow();
 expect(()=>options.xhrSetup(xhr,'/api/v1/kore-play/media/another/hls/index.m3u8')).toThrow();
 mocks.hlsHandlers.get('manifest')();expect(video.play).toHaveBeenCalledTimes(1);
 video.dispatchEvent(new Event('playing'));expect(statuses.at(-1)?.kind).toBe('playing');
 video.dispatchEvent(new Event('pause'));expect(statuses.at(-1)?.kind).toBe('paused');
 stop();expect(mocks.hlsDestroy).toHaveBeenCalledTimes(1);expect(video.hasAttribute('src')).toBe(false);
});
it('uses the TS engine with bounded cleanup and a cookie-authenticated local URL',async()=>{
 stop=startPreview(video,{...source,protocol:'http_ts'},vi.fn());
 await vi.waitFor(()=>expect(mocks.tsLoad).toHaveBeenCalledTimes(1));
 expect(mocks.tsCreate.mock.calls[0][0]).toEqual({type:'mpegts',isLive:true,withCredentials:true,url:'/api/v1/kore-play/media/synthetic-one/http_ts/stream.ts'});
 expect(mocks.tsCreate.mock.calls[0][1]).toMatchObject({enableWorker:false,autoCleanupSourceBuffer:true});
 stop();stop();expect(mocks.tsDestroy).toHaveBeenCalledTimes(1);
});
it('closing before an engine finishes importing never starts a late stream',async()=>{
 const notify=vi.fn();stop=startPreview(video,source,notify);stop();
 await vi.dynamicImportSettled();expect(mocks.hlsAttach).not.toHaveBeenCalled();expect(mocks.hlsOptions).not.toHaveBeenCalled();expect(notify).toHaveBeenCalledTimes(1);
});
it('reports authorization denial without exposing raw engine messages and closes on fatal error',async()=>{
 const statuses:PreviewStatus[]=[];stop=startPreview(video,source,s=>statuses.push(s));
 await vi.waitFor(()=>expect(mocks.hlsAttach).toHaveBeenCalled());
 mocks.hlsHandlers.get('error')('error',{fatal:true,response:{code:403},url:'SYNTHETIC_NEVER_ECHO'});
 expect(statuses.at(-1)?.text).toContain('Acesso ao canal recusado');expect(JSON.stringify(statuses)).not.toContain('SYNTHETIC_NEVER_ECHO');expect(mocks.hlsDestroy).toHaveBeenCalledTimes(1);
});
it('reports unsupported TS browsers without opening a stream',async()=>{
 mocks.tsSupported=false;const notify=vi.fn();stop=startPreview(video,{...source,protocol:'http_ts'},notify);
 await vi.waitFor(()=>expect(notify.mock.calls.at(-1)?.[0].kind).toBe('error'));expect(mocks.tsCreate).not.toHaveBeenCalled();
});
it('uses native HLS, stops an idle preview and enforces the five minute preview limit',async()=>{
 vi.useFakeTimers();vi.mocked(video.canPlayType).mockReturnValue('probably');
 let last:PreviewStatus|undefined;stop=startPreview(video,source,s=>{last=s});
 expect(video.getAttribute('src')).toBe(previewPath(source));
 await vi.advanceTimersByTimeAsync(22000);expect(last?.kind).toBe('error');expect(last?.text).toContain('20 segundos');expect(vi.getTimerCount()).toBe(0);
 vi.mocked(video.play).mockRejectedValue(new DOMException('blocked','NotAllowedError'));
 stop=startPreview(video,source,s=>{last=s});await Promise.resolve();expect(last?.kind).toBe('blocked');
 await vi.advanceTimersByTimeAsync(300000);expect(last?.kind).toBe('ended');expect(last?.text).toContain('cinco minutos');expect(vi.getTimerCount()).toBe(0);
});

it('keeps a continuous stream beyond five minutes and reports a stall as recoverable',async()=>{
 vi.useFakeTimers();vi.mocked(video.canPlayType).mockReturnValue('probably');vi.mocked(video.play).mockRejectedValue(new DOMException('blocked','NotAllowedError'));
 let last:PreviewStatus|undefined;stop=startPreview(video,{...source,scope:'output'},s=>{last=s});await Promise.resolve();
 await vi.advanceTimersByTimeAsync(301000);expect(last?.kind).toBe('blocked');expect(video.getAttribute('src')).toContain('/live/hls/');
 video.dispatchEvent(new Event('playing'));await vi.advanceTimersByTimeAsync(22000);expect(last?.kind).toBe('error');expect(last?.retryable).toBe(true);
});
it('never automatically retries a continuous HLS authorization refusal',async()=>{
 let last:PreviewStatus|undefined;stop=startPreview(video,{...source,scope:'output'},s=>{last=s});await vi.waitFor(()=>expect(mocks.hlsAttach).toHaveBeenCalled());
 mocks.hlsHandlers.get('error')('error',{fatal:true,response:{code:403}});expect(last?.retryable).toBe(false);
});
