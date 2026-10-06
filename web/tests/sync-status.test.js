import {it,expect,vi,afterEach} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import {useSyncStatus} from '../src/composables/useSyncStatus'
vi.mock('@/api',()=>({default:{syncStatus:vi.fn()}}))
import api from '@/api'
let wrapper
function setup(onFinished){let sync;wrapper=mount({setup(){sync=useSyncStatus(onFinished);return()=>null}});return sync}
afterEach(()=>{wrapper?.unmount();vi.useRealTimers();vi.resetAllMocks()})
it('polls running jobs through temporary failures and reports completion once',async()=>{
 vi.useFakeTimers();const done=vi.fn()
 api.syncStatus.mockResolvedValueOnce([{id:1,accountId:2,status:'running'}]).mockRejectedValueOnce(new Error('offline')).mockResolvedValue([{id:1,accountId:2,status:'success',enqueued:5}])
 const sync=setup(done);sync.start();await flushPromises();expect(sync.isRunning(2)).toBe(true);expect(done).not.toHaveBeenCalled()
 await vi.advanceTimersByTimeAsync(2500);expect(sync.error.value).toContain('正在重试');expect(sync.isRunning(2)).toBe(true)
 await vi.advanceTimersByTimeAsync(2500);expect(done).toHaveBeenCalledTimes(1);expect(sync.isRunning(2)).toBe(false);expect(sync.error.value).toBe('')
 await vi.advanceTimersByTimeAsync(2500);expect(done).toHaveBeenCalledTimes(1)
})
it('stopping a view discards a late response and prevents further polling',async()=>{
 vi.useFakeTimers();let resolve;api.syncStatus.mockReturnValue(new Promise(r=>resolve=r));const sync=setup(vi.fn());sync.start();sync.stop();resolve([{accountId:2,status:'running'}]);await flushPromises();await vi.advanceTimersByTimeAsync(10000);expect(api.syncStatus).toHaveBeenCalledTimes(1);expect(sync.runs.value).toEqual({})
})
