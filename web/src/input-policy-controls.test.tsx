// @vitest-environment jsdom
import {useState} from 'react';
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen} from '@testing-library/react';
import {InputPolicyControls,emptyPolicy,type InputPolicy} from './input-policy-controls';
afterEach(cleanup);
it('keeps publication and cluster opt-in and imports a reserve URL before selecting it',async()=>{
 const call=vi.fn(async()=>({asset:{id:'a'.repeat(64)+'.mp4',kind:'backup',bytes:1000}})),publication=vi.fn();
 function Form(){const [value,set]=useState<InputPolicy>(emptyPolicy),[assets,update]=useState<any[]>([]);return <><InputPolicyControls id="synthetic" value={value} onChange={set} onPublication={publication} disabled={false} call={call} assets={assets} onImported={a=>update([a])} transcoding={false}/><output>{JSON.stringify(value)}</output></>}
 render(<Form/>);expect(call).not.toHaveBeenCalled();
 fireEvent.click(screen.getByLabelText('Permitir publicação push por HTTPS'));expect(publication).toHaveBeenCalledWith(true);
 fireEvent.click(screen.getByLabelText('Permitir que outro Edge receba este canal por HTTPS'));
 expect(screen.getByText(/Salve para gerar/)).toBeTruthy();
 fireEvent.change(screen.getByLabelText('URL do vídeo de reserva (MP4 H.264/AAC)'),{target:{value:'https://synthetic.example/reserve.mp4'}});
 fireEvent.click(screen.getByRole('button',{name:'Importar arquivo'}));await screen.findByRole('option',{name:/MP4/});
 expect(call).toHaveBeenCalledWith('POST','manual-channels/assets',{action:'import',kind:'backup',url:'https://synthetic.example/reserve.mp4'});
 expect((screen.getByLabelText('Arquivo de reserva') as HTMLSelectElement).value).toBe('a'.repeat(64)+'.mp4');
});
