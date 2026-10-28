// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,render} from '@testing-library/react';
import {usePendingEdits} from './use-pending-edits';
afterEach(()=>{cleanup();vi.restoreAllMocks();window.history.replaceState(null,'','#media')});
it('keeps the current route and draft when leaving unsaved edits is declined',()=>{
 window.history.replaceState(null,'','#media/synthetic/input');
 function Form(){usePendingEdits(true);return null}render(<Form/>);
 vi.spyOn(window,'confirm').mockReturnValue(false);
 window.history.replaceState(null,'','#media');window.dispatchEvent(new HashChangeEvent('hashchange'));
 expect(window.location.hash).toBe('#media/synthetic/input');
 const unload=new Event('beforeunload',{cancelable:true});window.dispatchEvent(unload);expect(unload.defaultPrevented).toBe(true);
});
