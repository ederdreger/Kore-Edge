import {expect,it} from 'vitest';
import {readTranscodeClipboard} from './transcode-clipboard';
it('accepts typed profiles and rejects fields that could break the editor',()=>{
 expect(readTranscodeClipboard('{"mode":"h264","fps":25,"tracks":[{"width":640,"height":360,"video_kbps":800}]}').tracks).toHaveLength(1);
 for(const value of [{mode:'h264',tracks:'wrong'},{mode:'h264',tracks:[null]},{mode:'h264',tracks:[{extended:'wrong'}]},{mode:'h264',fps:{}},{mode:'h264',url:'https://synthetic.example/private'}])expect(()=>readTranscodeClipboard(JSON.stringify(value))).toThrow();
});
