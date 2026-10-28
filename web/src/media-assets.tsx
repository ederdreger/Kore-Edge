import {useEffect,useState} from 'react';
export type MediaAsset={id:string;kind:'logo'|'backup';bytes:number};
type Props={kind:'logo'|'backup';call:(method:string,path:string,body?:unknown)=>Promise<any>;onImported:(asset:MediaAsset)=>void;disabled:boolean};
export function MediaAssetImport({kind,call,onImported,disabled}:Props){
 const [url,setURL]=useState(''),[busy,setBusy]=useState(false),[error,setError]=useState('');
 useEffect(()=>()=>setURL(''),[]);
 async function submit(){if(!url||busy)return;setBusy(true);setError('');try{const value=await call('POST','manual-channels/assets',{action:'import',kind,url});setURL('');onImported(value.asset)}catch(e){setError((e as Error).message)}finally{setBusy(false)}}
 return <div className="mc-asset-import"><label>{kind==='logo'?'URL do logotipo (PNG/JPEG)':'URL do vídeo de reserva (MP4 H.264/AAC)'}<input type="url" maxLength={4096} autoComplete="off" value={url} disabled={disabled||busy} onChange={e=>setURL(e.target.value)}/></label><button type="button" disabled={disabled||busy||!url.trim()} onClick={()=>void submit()}>{busy?'Importando e validando…':'Importar arquivo'}</button>{error&&<p role="alert">{error}</p>}<small>{kind==='logo'?'Até 2 MB e 2048×2048 pixels.':'Até 64 MB e 5 minutos.'} Uma cópia privada é usada no fluxo; a URL não é enviada ao player.</small></div>;
}
