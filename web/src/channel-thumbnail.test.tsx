// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {act,cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {ChannelThumbnail} from './channel-thumbnail';

beforeEach(()=>{vi.spyOn(document,'visibilityState','get').mockReturnValue('visible')});
afterEach(async()=>{cleanup();await act(async()=>{await Promise.resolve()});vi.restoreAllMocks();vi.unstubAllGlobals()});
const jpeg=()=>({ok:true,status:200,headers:new Headers({'Content-Type':'image/jpeg'}),blob:async()=>new Blob([new Uint8Array([255,216,255,217])],{type:'image/jpeg'})});

it('never requests media or enables playback for a stopped channel',()=>{
 const request=vi.fn();vi.stubGlobal('fetch',request);
 render(<ChannelThumbnail id="synthetic" name="Canal sintético" online={false} onPlay={vi.fn()}/>);
 expect(request).not.toHaveBeenCalled();expect((screen.getByRole('button',{name:'Abrir player de Canal sintético'}) as HTMLButtonElement).disabled).toBe(true);
 expect(screen.queryByRole('img')).toBeNull();
});
it('shows an authenticated output image and opens playback only on click',async()=>{
 const request=vi.fn().mockResolvedValue(jpeg()),play=vi.fn();vi.stubGlobal('fetch',request);
 render(<ChannelThumbnail id="synthetic" name="Canal sintético" online onPlay={play}/>);
 const image=await screen.findByRole('img',{name:'Prévia da saída de Canal sintético'});
 expect(image.getAttribute('src')).toMatch(/^data:image\/jpeg;base64,/);
 expect(request).toHaveBeenCalledWith('/api/v1/live/media/synthetic/preview.jpg',expect.objectContaining({credentials:'same-origin',redirect:'error',signal:expect.any(AbortSignal)}));
 expect(play).not.toHaveBeenCalled();fireEvent.click(screen.getByRole('button',{name:'Abrir player de Canal sintético'}));expect(play).toHaveBeenCalledTimes(1);
});
it('bounds simultaneous requests and cancels queued cards when unmounted',async()=>{
 const signals:AbortSignal[]=[];
 const request=vi.fn((_url:string,options:RequestInit)=>new Promise((_resolve,reject)=>{const signal=options.signal!;signals.push(signal as AbortSignal);signal.addEventListener('abort',()=>reject(new DOMException('cancelled','AbortError')))}));vi.stubGlobal('fetch',request);
 const view=render(<>{['one','two','three'].map(id=><ChannelThumbnail key={id} id={id} name={id} online onPlay={vi.fn()}/>)}</>);
 await waitFor(()=>expect(request).toHaveBeenCalledTimes(2));view.unmount();
 await act(async()=>{await Promise.resolve();await Promise.resolve()});
 expect(signals.every(signal=>signal.aborted)).toBe(true);expect(request).toHaveBeenCalledTimes(2);
});
it('clears the image when reception stops without calling an operational endpoint',async()=>{
 const request=vi.fn().mockResolvedValue(jpeg());vi.stubGlobal('fetch',request);
 const view=render(<ChannelThumbnail id="synthetic" name="Synthetic" online onPlay={vi.fn()}/>);
 await screen.findByRole('img');view.rerender(<ChannelThumbnail id="synthetic" name="Synthetic" online={false} onPlay={vi.fn()}/>);
 expect(screen.queryByRole('img')).toBeNull();expect(request).toHaveBeenCalledTimes(1);
});

it('waits until a card enters the viewport and disconnects the observer on close',async()=>{
 let intersect:(entries:Partial<IntersectionObserverEntry>[])=>void=()=>{};const disconnect=vi.fn();
 vi.stubGlobal('IntersectionObserver',class{
  constructor(callback:IntersectionObserverCallback){intersect=entries=>callback(entries as IntersectionObserverEntry[],this as unknown as IntersectionObserver)}
  observe(){} disconnect=disconnect;
 });
 const request=vi.fn().mockResolvedValue(jpeg());vi.stubGlobal('fetch',request);
 const view=render(<ChannelThumbnail id="synthetic" name="Synthetic" online onPlay={vi.fn()}/>);
 expect(request).not.toHaveBeenCalled();act(()=>intersect([{isIntersecting:true}]));await screen.findByRole('img');
 act(()=>intersect([{isIntersecting:false}]));expect((request.mock.calls[0][1] as RequestInit).signal?.aborted).toBe(true);
 view.unmount();expect(disconnect).toHaveBeenCalledTimes(1);
});
