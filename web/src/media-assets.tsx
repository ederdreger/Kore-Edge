import {useEffect,useState} from 'react';
export type MediaAsset={id:string;kind:'logo'|'backup';bytes:number;name?:string};
type Props={kind:'logo'|'backup';call:(method:string,path:string,body?:unknown)=>Promise<any>;onImported:(asset:MediaAsset)=>void;disabled:boolean};
export function MediaAssetImport({kind,call,onImported,disabled}:Props){
 const [url,setURL]=useState(''),[busy,setBusy]=useState(false),[error,setError]=useState('');
 useEffect(()=>()=>setURL(''),[]);
 async function submit(){if(!url||busy)return;setBusy(true);setError('');try{const value=await call('POST','manual-channels/assets',{action:'import',kind,url});setURL('');onImported(value.asset)}catch(e){setError((e as Error).message)}finally{setBusy(false)}}
 return <div className="mc-asset-import"><label>{kind==='logo'?'URL do logotipo (PNG/JPEG)':'URL do vídeo de reserva (MP4 H.264/AAC)'}<input type="url" maxLength={4096} autoComplete="off" value={url} disabled={disabled||busy} onChange={e=>setURL(e.target.value)}/></label><button type="button" disabled={disabled||busy||!url.trim()} onClick={()=>void submit()}>{busy?'Importando e validando…':'Importar arquivo'}</button>{error&&<p role="alert">{error}</p>}<small>{kind==='logo'?'Até 2 MB e 2048×2048 pixels.':'Até 64 MB e 5 minutos.'} Uma cópia privada é usada no fluxo; a URL não é enviada ao player.</small></div>;
}

export function MediaAssetLibrary({assets,call,onRemoved,disabled}:{assets:MediaAsset[];call:Props['call'];onRemoved:(id:string)=>void;disabled:boolean}){
 const [busy,setBusy]=useState(false),[error,setError]=useState('');
 async function remove(id:string){if(busy)return;setBusy(true);setError('');try{await call('POST','manual-channels/assets',{action:'delete',id});onRemoved(id)}catch(e){setError((e as Error).message)}finally{setBusy(false)}}
 return <details className="mc-advanced"><summary>Arquivos privados ({assets.length}/20)</summary>{error&&<p role="alert">{error}</p>}{assets.map(a=><div className="mc-asset-row" key={a.id}><span>{a.kind==='logo'?'Logotipo PNG':'Reserva MP4'} · {a.name||a.id.slice(0,10)} · {(a.bytes/1048576).toFixed(2)} MB</span><button type="button" disabled={disabled||busy} onClick={()=>void remove(a.id)}>Excluir arquivo {a.id.slice(0,10)}</button></div>)}<p className="mc-muted">Um arquivo em uso não pode ser excluído. Remova a seleção do canal e salve primeiro.</p></details>;
}
