import { it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ReaderPageImage from '../src/components/ReaderPageImage.vue'

it('keeps the placeholder until this image decodes and resets readiness on retry',async()=>{
 const w=mount(ReaderPageImage,{props:{src:'/page/1',page:1}})
 const node=w.get('img').element
 Object.defineProperties(node,{complete:{value:true,configurable:true},naturalWidth:{value:600,configurable:true}})
 let resolve
 node.decode=vi.fn(()=>new Promise(r=>{resolve=r}))
 await w.get('img').trigger('load')
 expect(w.find('.rd-ph').exists()).toBe(true)
 resolve();await flushPromises()
 expect(w.find('.rd-ph').exists()).toBe(false)
 Object.defineProperty(node,'complete',{value:false,configurable:true})
 await w.setProps({src:'/page/1?retry=1'})
 expect(w.find('.rd-ph').exists()).toBe(true)
 expect(w.get('img').classes()).not.toContain('is-loaded')
 w.unmount()
})
it('ignores late decode completion after the source changes or the image unmounts',async()=>{
 const w=mount(ReaderPageImage,{props:{src:'/page/1',page:1}})
 const node=w.get('img').element
 Object.defineProperties(node,{complete:{value:true,configurable:true},naturalWidth:{value:600,configurable:true}})
 let resolve
 node.decode=vi.fn(()=>new Promise(r=>{resolve=r}))
 await w.get('img').trigger('load')
 Object.defineProperty(node,'complete',{value:false,configurable:true})
 await w.setProps({src:'/page/2'})
 resolve();await flushPromises()
 expect(w.emitted('load')).toBeUndefined()
 expect(w.find('.rd-ph').exists()).toBe(true)
 Object.defineProperty(node,'complete',{value:true,configurable:true})
 await w.get('img').trigger('load')
 w.unmount();resolve();await flushPromises()
 expect(w.emitted('load')).toBeUndefined()
})
