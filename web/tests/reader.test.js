import { it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import Reader from '../src/views/ReaderView.vue'
vi.mock('@/api', () => ({ default: {
  comic: vi.fn(async () => ({title:'Test',chapters:[{order:1,title:'One'},{order:2,title:'Two'}]})),
  readerMeta: vi.fn(async () => ({pages:2,sizes:[[100,200],[100,200]]})),
  saveReading: vi.fn(async () => ({})), reading: vi.fn(async () => ({order:2,page:2,updatedAt:100})),
  listAccounts: vi.fn()
}}))
vi.mock('@/store/auth', () => ({auth:{user:{username:'test'}}}))
vi.mock('@/composables/useIsMobile', () => ({useIsMobile:()=>false}))
import api from '@/api'
beforeEach(()=>{
  vi.clearAllMocks()
  localStorage.clear()
  HTMLElement.prototype.scrollTo = vi.fn()
})
async function reader(path) {
  const router=createRouter({history:createMemoryHistory(),routes:[{path:'/reader/:kind/:comicId/:order',component:Reader}]})
  await router.push(path);await router.isReady()
  const w=mount(Reader,{global:{plugins:[router],stubs:{ElDrawer:true,ElSwitch:true,ArrowLeft:true,DArrowLeft:true,DArrowRight:true,FullScreen:true,Setting:true,ElDropdownItem:true,ElDropdownMenu:true,ElSelect:{props:['modelValue'],emits:['change'],template:'<select :value="modelValue" @change="$emit(\'change\', Number($event.target.value))"><option value="1">One</option><option value="2">Two</option></select>'},ElButton:{template:'<button><slot /></button>'},ElIcon:true,ElOption:true,ElRadioGroup:true,ElRadioButton:true,ElDropdown:true}}})
  await flushPromises()
  return {w,router}
}
it('chapter selector navigates and loads the selected chapter',async()=>{
  const {w,router}=await reader('/reader/jm/book/1')
  await w.find('select').setValue('2');await flushPromises()
  expect(router.currentRoute.value.params.order).toBe('2')
  expect(api.readerMeta).toHaveBeenLastCalledWith('jm','book',2,expect.any(AbortSignal))
  w.unmount()
})
it('resume opens the server chapter and saves progress on exit',async()=>{
  const {w,router}=await reader('/reader/jm/book/1?resume=1')
  await flushPromises()
  expect(router.currentRoute.value.params.order).toBe('2')
  expect(router.currentRoute.value.query.page).toBe('2')
  w.unmount()
  expect(api.saveReading).toHaveBeenCalledWith('jm','book',expect.objectContaining({order:2}),true)
})

it('supports nonconsecutive chapter orders and does not reload book details on chapter change',async()=>{
  api.comic.mockResolvedValueOnce({title:'Gaps',chapters:[{order:2,title:'Two'},{order:5,title:'Five'}]})
  const {w,router}=await reader('/reader/jm/book/2')
  await w.get('[aria-label="下一章"]').trigger('click');await flushPromises()
  expect(router.currentRoute.value.params.order).toBe('5')
  expect(api.comic).toHaveBeenCalledTimes(1)
  w.unmount()
})
it('clamps page jumps and saves the previous chapter when route changes externally',async()=>{
  const {w,router}=await reader('/reader/jm/book/1')
  await w.get('[aria-label="当前页码"]').setValue('999')
  expect(api.saveReading).toHaveBeenLastCalledWith('jm','book',expect.objectContaining({order:1,page:2}),false)
  await router.replace('/reader/jm/book/2');await flushPromises()
  expect(api.saveReading).toHaveBeenCalledWith('jm','book',expect.objectContaining({order:1,page:2}),false)
  w.unmount()
})
