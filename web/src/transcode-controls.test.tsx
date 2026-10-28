// @vitest-environment jsdom
import {useState} from 'react';
import {afterEach,expect,it} from 'vitest';
import {cleanup,fireEvent,render,screen} from '@testing-library/react';
import {TranscodeControls,processingDefaults,type Transcode} from './transcode-controls';
afterEach(cleanup);
function fixture(){function Form(){const [value,set]=useState<Transcode>({...processingDefaults,device:'cpu'});return <><TranscodeControls value={value} onChange={set} disabled={false} devices={[{id:'cpu',name:'CPU'},{id:'vaapi:renderD128',name:'GPU sintética'}]}/><output data-testid="value">{JSON.stringify(value)}</output></>}render(<Form/>)}
const value=()=>JSON.parse(screen.getByTestId('value').textContent!);
it('edits audio independently and adds, duplicates and removes bounded video tracks',()=>{
 fixture();fireEvent.change(screen.getByLabelText('Volume (dB)'),{target:{value:-3}});
 fireEvent.change(screen.getByLabelText('Predefinição de faixa'),{target:{value:'640,360,700'}});
 expect(value().tracks[1]).toMatchObject({width:640,height:360,video_kbps:700});
 fireEvent.click(screen.getByRole('button',{name:'Duplicar faixa 2'}));expect(value().tracks).toHaveLength(3);
 expect((screen.getByRole('button',{name:'Adicionar faixa'}) as HTMLButtonElement).disabled).toBe(true);
 fireEvent.click(screen.getByRole('button',{name:'Remover faixa 1'}));expect(value().tracks).toHaveLength(2);expect(value().volume_db).toBe(-3);
 fireEvent.click(screen.getAllByRole('checkbox',{name:'Copiar da entrada'})[0]);expect(value().audio).toBe('copy');expect(value().volume_db).toBe(0);
});
it('removes CPU-only settings when moving to VAAPI and keeps pure copy available',()=>{
 fixture();fireEvent.change(screen.getByLabelText('Perfil · faixa 1'),{target:{value:'high'}});
 fireEvent.click(screen.getByLabelText('GOP aberto · faixa 1 · CPU'));
 fireEvent.change(screen.getByLabelText('Dispositivo'),{target:{value:'vaapi:renderD128'}});
 expect(value().tracks[0]).toMatchObject({profile:'',open_gop:false,refs:0,bframes:0});
 expect((screen.getByLabelText('Predefinição · faixa 1') as HTMLSelectElement).disabled).toBe(true);
 fireEvent.click(screen.getByRole('button',{name:'Desativar transcodificação'}));expect(value()).toEqual({mode:'copy'});
});
it('returns to pure copy when both input-copy controls are selected',()=>{
 fixture();fireEvent.click(screen.getAllByRole('checkbox',{name:'Copiar da entrada'})[1]);
 expect(value().video_copy).toBe(true);
 fireEvent.click(screen.getAllByRole('checkbox',{name:'Copiar da entrada'})[0]);
 expect(value()).toEqual({mode:'copy'});
});
