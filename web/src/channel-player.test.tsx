// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {act,cleanup,fireEvent,render,screen} from '@testing-library/react';
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

it('opens a standalone player in a modal overlay and restores scrolling and focus',()=>{
 const trigger=document.createElement('button');document.body.append(trigger);trigger.focus();
 const close=vi.fn(),overflow=document.body.style.overflow;
 const view=render(<ChannelPlayer source={{id:'synthetic',name:'Synthetic',protocol:'http_ts',scope:'live'}} onClose={close}/>);
 const dialog=screen.getByRole('dialog',{name:'Mini player: Synthetic'});
 expect(dialog.parentElement?.classList.contains('channel-player-overlay')).toBe(true);
 expect(dialog.getAttribute('aria-modal')).toBe('true');expect(document.body.style.overflow).toBe('hidden');
 fireEvent.mouseDown(dialog);expect(close).not.toHaveBeenCalled();
 fireEvent.keyDown(document,{key:'Escape'});expect(close).toHaveBeenCalledTimes(1);
 view.unmount();expect(document.body.style.overflow).toBe(overflow);expect(document.activeElement).toBe(trigger);trigger.remove();
});
it('keeps an embedded overview player inside its parent instead of creating another modal',()=>{
 const view=render(<ChannelPlayer embedded source={{id:'synthetic',name:'Synthetic',protocol:'http_ts',scope:'live'}} onClose={vi.fn()}/>);
 expect(screen.getByRole('region').classList.contains('channel-player-embedded')).toBe(true);
 expect(view.container.querySelector('.channel-player-overlay')).toBeNull();expect(screen.queryByRole('dialog')).toBeNull();
});
