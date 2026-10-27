// @vitest-environment jsdom
import {StrictMode} from 'react';
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {PlaybackLinks} from './playback-links';
afterEach(()=>{cleanup();vi.restoreAllMocks()});
const link={channel_id:'synthetic',ts_path:'/api/v1/playback/SYNTHETIC/synthetic/stream.ts',hls_path:'/api/v1/playback/SYNTHETIC/synthetic/index.m3u8'};
it('automatically displays stable TS/HLS addresses without generation, expiry or revoke controls',async()=>{
 const call=vi.fn(async(_method?:string,_path?:string)=>({links:[{...link,hls_path:'/api/v1/playback/SYNTHETIC/synthetic/index.m3u8'}]}));render(<StrictMode><PlaybackLinks channelID="synthetic" admin hlsState="disabled" call={call}/></StrictMode>);
 await screen.findByLabelText('Link MPEG-TS');expect(call).toHaveBeenCalledWith('GET','live/playback-links');expect(call.mock.calls.every(c=>c[0]==='GET')).toBe(true);
 expect(screen.getByLabelText('Link HLS')).toBeTruthy();expect(screen.queryByRole('button',{name:/Gerar|Revogar/})).toBeNull();expect(screen.queryByText(/V\u00e1lidos at\u00e9/)).toBeNull();
});
it('does not fetch playback credentials for spectators',()=>{
 const call=vi.fn();render(<PlaybackLinks channelID="synthetic" admin={false} call={call}/>);expect(call).not.toHaveBeenCalled();
});
it('provides a selectable-copy fallback when clipboard access is unavailable',async()=>{
 const call=vi.fn(async(_method?:string,_path?:string)=>({links:[link]}));render(<PlaybackLinks channelID="synthetic" admin call={call}/>);
 await screen.findByLabelText('Link MPEG-TS');fireEvent.click(screen.getByRole('button',{name:'Copiar link TS'}));await screen.findByText(/Pressione Ctrl\+C/);
 expect(document.activeElement).toBe(screen.getByLabelText('Link MPEG-TS'));
});
