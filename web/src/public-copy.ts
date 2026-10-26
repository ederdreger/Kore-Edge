// Copy public integration data only. Keep a selected, visible fallback when the
// Clipboard API is unavailable, denied or waiting for browser permission.
export async function copyPublicText(text:string, field:HTMLTextAreaElement|null):Promise<'copied'|'manual'> {
 if(field){field.focus();field.select();field.setSelectionRange(0,field.value.length)}
 let timer:ReturnType<typeof setTimeout>|undefined;
 try {
  if(!navigator.clipboard?.writeText)throw new Error();
  await Promise.race([navigator.clipboard.writeText(text),new Promise<never>((_,reject)=>{timer=setTimeout(()=>reject(new Error()),1500)})]);
  return 'copied';
 } catch {
  try {if(field&&typeof document.execCommand==='function'&&document.execCommand('copy'))return 'copied'}catch{/* selected manual fallback */}
  return 'manual';
 } finally {if(timer!==undefined)clearTimeout(timer)}
}
