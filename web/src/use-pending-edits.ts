import {useEffect,useRef} from 'react';

// Save is explicit. Polling and navigation never silently commit a draft.
export function usePendingEdits(dirty:boolean){
 const current=useRef(dirty);current.current=dirty;
 useEffect(()=>{
  const leave=(e:BeforeUnloadEvent)=>{if(current.current){e.preventDefault();e.returnValue=''}};
  let previous=window.location.hash;
  const navigate=(e:Event)=>{
   if(window.location.hash===previous)return;
   if(current.current&&!window.confirm('Existem alterações não salvas. Sair e descartá-las?')){
    window.history.replaceState(null,'',previous||'#media');e.stopImmediatePropagation();return;
   }
   previous=window.location.hash;
  };
  window.addEventListener('beforeunload',leave);window.addEventListener('hashchange',navigate,true);
  return()=>{window.removeEventListener('beforeunload',leave);window.removeEventListener('hashchange',navigate,true)};
 },[]);
}
