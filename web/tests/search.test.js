import {it,expect,vi,afterEach} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import {ref} from 'vue'
import Search from '../src/views/SearchView.vue'
vi.mock('@/api',()=>({default:{search:vi.fn()}}))
vi.mock('@/store/app',()=>({useAppStore:()=>({accounts:[],loadAccounts:async()=>{},loadActiveJobs:async()=>{}})}))
vi.mock('@/composables/useViewActive',()=>({useViewActive:()=>ref(true)}))
vi.mock('@/composables/useDownloadActions',()=>({useDownloadActions:()=>({isBusy:()=>false})}))
import api from '@/api'
let w
function setup(){w=mount(Search,{global:{directives:{loading:()=>{}},stubs:{ComicCard:{props:['item'],template:'<div class="result">{{item.title}}</div>'},ComicDetailDialog:true,PageBar:true,ElRadioGroup:true,ElRadioButton:true,ElSelect:true,ElOption:true,ElInput:true,ElButton:{template:'<button><slot/></button>'},ElEmpty:{props:['description'],template:'<div>{{description}}</div>'},ElTag:true,ElIcon:true,ElAlert:{props:['title'],template:'<div>{{title}}<slot/></div>'},Search:true}}});return w}
function deferred(){let resolve;const promise=new Promise(r=>resolve=r);return {promise,resolve}}
afterEach(()=>{w?.unmount();vi.resetAllMocks()})
it('normal search trims keyword, resets page and shows results',async()=>{
 api.search.mockResolvedValue({total:1,items:[{kind:'jm',comicId:'1',title:'result'}]});setup();w.vm.keyword=' test ';w.vm.page=3;await w.vm.submit();await flushPromises();expect(api.search).toHaveBeenCalledWith(expect.objectContaining({keyword:'test',page:1}));expect(w.find('.result').text()).toBe('result')
})
it('newer search wins when old response arrives late',async()=>{
 const old=deferred(),latest=deferred();api.search.mockReturnValueOnce(old.promise).mockReturnValueOnce(latest.promise);setup();w.vm.keyword='old';w.vm.submit();w.vm.keyword='new';w.vm.submit();latest.resolve({total:1,items:[{kind:'pica',comicId:'2',title:'new'}]});await flushPromises();old.resolve({total:1,items:[{kind:'pica',comicId:'1',title:'old'}]});await flushPromises();expect(w.find('.result').text()).toBe('new')
})
it('switching source must discard an in-flight response from previous source',async()=>{
 const old=deferred();api.search.mockReturnValue(old.promise);setup();w.vm.keyword='test';w.vm.submit();w.vm.kind='jm';await flushPromises();old.resolve({total:1,items:[{kind:'pica',comicId:'1',title:'old-source-result'}]});await flushPromises();expect(w.findAll('.result')).toHaveLength(0)
})
it('request failure must not be presented as a successful empty search',async()=>{
 api.search.mockRejectedValue(new Error('upstream failed'));setup();w.vm.keyword='test';w.vm.submit();await flushPromises();expect(w.text()).not.toContain('没有搜索结果')
})
it('clearing keyword must not allow old pending results to reappear',async()=>{
 const old=deferred();api.search.mockReturnValue(old.promise);setup();w.vm.keyword='test';w.vm.submit();await flushPromises();w.vm.keyword='';w.vm.searched=false;await flushPromises();old.resolve({total:1,items:[{kind:'pica',comicId:'1',title:'stale'}]});await flushPromises();expect(w.findAll('.result')).toHaveLength(0)
})
it('uses the upstream page size instead of a misleading local size selector',async()=>{
 api.search.mockResolvedValue({total:160,pageSize:80,items:[]});setup();w.vm.keyword='test';w.vm.submit();await flushPromises();expect(w.vm.pageSize).toBe(80)
})
