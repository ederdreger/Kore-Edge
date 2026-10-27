// @vitest-environment jsdom
import {StrictMode} from 'react';
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {PlaybackLinks} from './playback-links';
afterEach(()=>{cleanup();vi.restoreAllMocks()});
const link={channel_id:'synthetic',ts_path:'/api/v1/playback/SYNTHETIC/synthetic/stream.ts',expires_at:'2030-01-01T00:00:00Z'};
it('generates and revokes a media-only link without passwords or automatic activation',async()=>{
 const call=vi.fn(async()=>({links:[link]}));render(<StrictMode><PlaybackLinks channelID="synthetic" admin hlsState="disabled" call={call}/></StrictMode>);
 expect(call).not.toHaveBeenCalled();fireEvent.click(screen.getByRole('button',{name:'Gerar links para VLC'}));await screen.findByLabelText('Link MPEG-TS para VLC');
 expect(call).toHaveBeenCalledWith('POST','live/playback-links',{action:'issue',channel_ids:['synthetic']});expect(screen.queryByLabelText(/Senha/)).toBeNull();expect(screen.queryByLabelText('Link HLS para VLC')).toBeNull();
 fireEvent.click(screen.getByRole('button',{name:'Revogar links deste canal'}));await waitFor(()=>expect(screen.queryByLabelText('Link MPEG-TS para VLC')).toBeNull());expect(call).toHaveBeenLastCalledWith('POST','live/playback-links',{action:'revoke',channel_ids:['synthetic']});
});
it('keeps issuance and revocation disabled for spectators',()=>{
 const call=vi.fn();render(<PlaybackLinks channelID="synthetic" admin={false} call={call}/>);
 expect((screen.getByRole('button',{name:'Gerar links para VLC'}) as HTMLButtonElement).disabled).toBe(true);expect((screen.getByRole('button',{name:'Revogar links deste canal'}) as HTMLButtonElement).disabled).toBe(true);
});
it('provides a selectable-copy fallback when clipboard access is unavailable',async()=>{
 const call=vi.fn(async()=>({links:[link]}));render(<PlaybackLinks channelID="synthetic" admin call={call}/>);
 fireEvent.click(screen.getByRole('button',{name:'Gerar links para VLC'}));await screen.findByLabelText('Link MPEG-TS para VLC');fireEvent.click(screen.getByRole('button',{name:'Copiar link TS'}));await screen.findByText(/Pressione Ctrl\+C/);
 expect(document.activeElement).toBe(screen.getByLabelText('Link MPEG-TS para VLC'));
});
