import { describe, it, expect, vi, afterEach } from 'vitest'
import { useLatestRequest } from '../src/composables/useLatestRequest'
import { createPreloader } from '../src/utils/preloader'
import { createPinia, setActivePinia } from 'pinia'
vi.mock('@/api', () => ({default: { listDownloads: vi.fn() }}))
import api from '@/api'
import { useAppStore } from '../src/store/app'
afterEach(() => vi.useRealTimers())
describe('request generations', () => {
  it('old request cleanup cannot disable the current timeout', () => {
    vi.useFakeTimers()
    const req = useLatestRequest(100)
    const a = req.begin(), b = req.begin()
    req.end(a.my)
    expect(a.signal.aborted).toBe(true)
    vi.advanceTimersByTime(100)
    expect(b.signal.aborted).toBe(true)
  })
})
describe('preloader', () => {
  it('late callbacks from a previous chapter cannot drain the new queue', () => {
    const images = [], sizes = vi.fn()
    const queue = createPreloader(() => { const i = {}; images.push(i); return i }, 2)
    queue.schedule([1, 2, 3], n => '/old/'+n, sizes)
    const oldCallback = images[0].onload
    queue.reset()
    queue.schedule([4,5,6], n => '/new/'+n, sizes)
    oldCallback()
    expect(images).toHaveLength(4)
    expect(sizes).not.toHaveBeenCalled()
    images[2].naturalWidth = 100; images[2].naturalHeight = 200; images[2].onload()
    expect(images).toHaveLength(5)
    expect(sizes).toHaveBeenCalledWith(4,100,200)
    queue.reset()
    expect(images[4].src).toBe('')
  })
})
describe('task snapshots', () => {
  it('removes completed jobs from the active cache and includes all pages', async () => {
    setActivePinia(createPinia())
    const s = useAppStore()
    s.jobs = {1:{id:1,status:'running'}}
    api.listDownloads.mockImplementation(async ({status,page}) => ({total:status==='queued'?201:0,items:status==='queued'?[{id:page+1,status:'queued'}]:[]}))
    await s.loadActiveJobs()
    expect(s.jobs[1]).toBeUndefined()
    expect(Object.keys(s.jobs)).toHaveLength(2)
  })
})
