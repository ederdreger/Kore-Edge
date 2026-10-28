// @vitest-environment jsdom
import {afterEach,expect,it} from 'vitest';
import {cleanup,fireEvent,render,screen} from '@testing-library/react';
import {ChannelChart} from './media-channel-chart';
afterEach(cleanup);
it('shows measured input/output/client series and exposes exact samples on keyboard focus',()=>{
 const at=Date.now();render(<ChannelChart points={[{at,input:4000000,output:8000000,clients:2},{at:at+3000,input:4500000,output:9000000,clients:3}]}/>);
 const graph=screen.getByRole('img');fireEvent.focus(graph);
 expect(screen.getByRole('status').textContent).toContain('9.00 Mbps');
 expect(screen.getByRole('status').textContent).toContain('3');
 expect(document.querySelectorAll('[data-series="output"] path')).toHaveLength(1);
 fireEvent.click(screen.getByRole('button',{name:'Saída HTTP'}));expect(document.querySelector('[data-series="output"]')).toBeNull();
});
it('does not draw missing measurements as zero and labels the delivery scope',()=>{
 render(<ChannelChart points={[{at:Date.now(),input:null,output:null,clients:null},{at:Date.now()+3000,input:null,output:null,clients:null}]}/>);
 expect(document.querySelectorAll('[data-series] path')).toHaveLength(0);
 fireEvent.focus(screen.getByRole('img'));
 expect(screen.getAllByText('Não medido')).toHaveLength(3);
 expect(screen.getByText(/Outros protocolos não estão somados/)).toBeTruthy();
});
