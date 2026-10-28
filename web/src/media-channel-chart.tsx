import {useState} from 'react';
import {mediaRate} from './media-model';

export type ChannelPoint={at:number;input:number|null;output:number|null;clients:number|null};
const series=[{key:'input',label:'Entrada',color:'#ff3b3b'},{key:'output',label:'Saída HTTP',color:'#129f24'},{key:'clients',label:'Conexões TS',color:'#423bff'}] as const;
export function ChannelChart({points}:{points:ChannelPoint[]}){
 const [hover,setHover]=useState<number|null>(null),[hidden,setHidden]=useState<string[]>([]);
 const maximum=Math.max(1e6,...points.flatMap(p=>[p.input??0,p.output??0]))*1.15,clientMax=Math.max(3,Math.ceil(Math.max(0,...points.map(p=>p.clients??0))/3)*3);
 const x=(i:number)=>65+i/Math.max(1,points.length-1)*875;
 const y=(value:number,clients:boolean)=>180-value/(clients?clientMax:maximum)*135;
 const paths=(key:'input'|'output'|'clients')=>{
  const result:string[]=[];let path='';
  points.forEach((point,i)=>{const value=point[key];if(value===null){if(path)result.push(path);path='';return}path+=(path?' L':'M')+x(i).toFixed(1)+','+y(value,key==='clients').toFixed(1)});
  if(path)result.push(path);return result;
 };
 const active=hover===null?null:points[Math.min(hover,points.length-1)];
 const time=(at:number)=>new Date(at).toLocaleTimeString('pt-BR',{hour:'2-digit',minute:'2-digit',second:'2-digit'});
 return <section className="mc-panel mc-chart mc-flow-chart" aria-label="Histórico observado do canal">
  <div className="mc-chart-legend">{series.map(s=><button type="button" key={s.key} aria-pressed={!hidden.includes(s.key)} onClick={()=>setHidden(old=>old.includes(s.key)?old.filter(k=>k!==s.key):[...old,s.key])}><i style={{borderColor:s.color}}/>{s.label}</button>)}</div>
  <div className="mc-chart-canvas">
   <svg viewBox="0 0 1000 220" preserveAspectRatio="none" role="img" tabIndex={0} aria-label="Taxas reais de entrada e entrega HTTP; conexões TS no eixo direito" onMouseLeave={()=>setHover(null)} onMouseMove={event=>{const bounds=event.currentTarget.getBoundingClientRect();if(bounds.width)setHover(Math.max(0,Math.min(points.length-1,Math.round(((event.clientX-bounds.left)/bounds.width*1000-65)/875*(points.length-1)))))}} onFocus={()=>setHover(Math.max(0,points.length-1))} onBlur={()=>setHover(null)} onKeyDown={event=>{if(event.key==='ArrowLeft'||event.key==='ArrowRight'){event.preventDefault();setHover(old=>Math.max(0,Math.min(points.length-1,(old??points.length-1)+(event.key==='ArrowLeft'?-1:1))))}}}>
    {[0,1,2,3].map(i=><g key={i}><line x1="65" x2="940" y1={180-i*45} y2={180-i*45} stroke="#dce4ef"/><text x="57" y={184-i*45} textAnchor="end">{mediaRate(maximum*i/3)}</text><text x="950" y={184-i*45}>{Math.round(clientMax*i/3)}</text></g>)}
    {[0,1,2,3,4,5].map(i=>{const index=Math.round(i/5*Math.max(0,points.length-1));return <g key={i}><line x1={65+i*175} x2={65+i*175} y1="45" y2="180" stroke="#e8edf5"/><text x={65+i*175} y="205" textAnchor="middle">{points[index]?time(points[index].at):'—'}</text></g>})}
    {series.filter(s=>!hidden.includes(s.key)).map(s=><g key={s.key} data-series={s.key}>{paths(s.key).map((path,i)=><path key={i} d={path} fill="none" stroke={s.color} strokeWidth="1.5"/>)}</g>)}
    {active&&<g><line x1={x(hover!)} x2={x(hover!)} y1="45" y2="180" stroke="#a2afc5" strokeDasharray="4 4"/>{series.filter(s=>!hidden.includes(s.key)&&active[s.key]!==null).map(s=><circle key={s.key} cx={x(hover!)} cy={y(active[s.key]!,s.key==='clients')} r="3" fill="#fff" stroke={s.color}/>)}</g>}
   </svg>
   {active&&<div className="mc-chart-tooltip" role="status" style={{left:`${Math.min(73,Math.max(8,(hover??0)/Math.max(1,points.length-1)*85))}%`}}><time>{new Date(active.at).toLocaleString('pt-BR')}</time>{series.map(s=><div key={s.key}><i style={{background:s.color}}/><span>{s.label}</span><strong>{active[s.key]===null?'Não medido':s.key==='clients'?active.clients:mediaRate(active[s.key])}</strong></div>)}</div>}
  </div>
  {points.length<2&&<p>Aguardando novas amostras reais.</p>}
  <p>Saída: bytes entregues pelo HTTP do Edge em TS/HLS. Conexões TS não representam usuários únicos nem sessões HLS. Outros protocolos não estão somados.</p>
 </section>;
}
