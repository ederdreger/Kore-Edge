// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {act,cleanup,render} from '@testing-library/react';
import type {PreviewStatus} from './preview-engine';
import {ChannelPlayer} from './channel-player';
const mock=vi.hoisted(()=>({start:vi.fn(),stop:vi.fn(),notify:null as null|((s:PreviewStatus)=>void)}));
vi.mock('./preview-engine',()=>({startPreview:(_v:unknown,_s:unknown,notify:(s:PreviewStatus)=>void)=>{mock.start();mock.notify=notify;return mock.stop}}));
afterEach(()=>{cleanup();vi.useRealTimers();vi.clearAllMocks()});
it('reconnects a recoverable continuous preview and cancels retries on close',async()=>{
 vi.useFakeTimers();const view=render(<ChannelPlayer source={{id:'synthetic',name:'Synthetic',protocol:'http_ts',scope:'live'}} onClose={vi.fn()}/>);
 act(()=>mock.notify?.({kind:'error',text:'Synthetic interruption',retryable:true}));
 await act(()=>vi.advanceTimersByTimeAsync(1000));expect(mock.start).toHaveBeenCalledTimes(2);expect(mock.stop).toHaveBeenCalledTimes(1);
 act(()=>mock.notify?.({kind:'error',text:'Synthetic interruption',retryable:true}));view.unmount();await vi.advanceTimersByTimeAsync(60000);expect(mock.start).toHaveBeenCalledTimes(2);
});
it('does not retry a manual pause or denied access',async()=>{
 vi.useFakeTimers();render(<ChannelPlayer source={{id:'synthetic',name:'Synthetic',protocol:'http_ts',scope:'live'}} onClose={vi.fn()}/>);
 act(()=>mock.notify?.({kind:'paused',text:'Manual pause'}));await act(()=>vi.advanceTimersByTimeAsync(60000));expect(mock.start).toHaveBeenCalledTimes(1);
 act(()=>mock.notify?.({kind:'error',text:'Denied',retryable:false}));await act(()=>vi.advanceTimersByTimeAsync(60000));expect(mock.start).toHaveBeenCalledTimes(1);
});
