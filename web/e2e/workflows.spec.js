import { test, expect } from '@playwright/test'
const user={username:'tester',isAdmin:true,mustChangePassword:false}
const book={kind:'pica',comicId:'test-book',title:'测试漫画',author:'Test',chapters:1}
const account={id:1,kind:'pica',username:'test-site',status:'ok'}
async function mockBackend(page,{logged=true,searchError=false}={}){
 let syncState=[],downloads=[]
 await page.route('**/api/**',async route=>{
  const req=route.request(),url=new URL(req.url()),p=url.pathname
  const send=(data,status=200)=>route.fulfill({status,contentType:'application/json',body:JSON.stringify(data)})
  if(p==='/api/auth/me')return logged?send(user):send({error:'未登录'},401)
  if(p==='/api/auth/login'){logged=true;return send(user)}
  if(p==='/api/accounts')return send([account])
  if(p==='/api/sync-status')return send(syncState)
  if(p==='/api/accounts/1/sync'){syncState=[{id:1,accountId:1,status:'running',stage:'checking',processed:1,total:10,enqueued:1,skipped:0}];return send(syncState[0],202)}
  if(p==='/api/accounts/1/favorites')return send({items:[book],total:1,snapshot:{hasSnapshot:true,updatedAt:new Date().toISOString()},refreshing:false})
  if(p==='/api/search')return searchError?send({error:'上游暂时不可用'},502):send({items:[book],total:80,page:1,pageSize:40})
  if(p==='/api/downloads'&&req.method()==='POST'){downloads=[{id:7,kind:'pica',comicId:'test-book',title:'测试漫画',status:'queued'}];return send({id:7})}
  if(p==='/api/downloads')return send({items:downloads,total:downloads.length})
  if(p==='/api/comics/pica/test-book')return send({...book,chapters:[{order:1,title:'第一章'},{order:2,title:'第二章'}]})
  if(p.endsWith('/meta'))return send({pages:2,local:true,sizes:[[2,2],[2,2]]})
  if(p.includes('/page/')||p.endsWith('/cover'))return route.fulfill({contentType:'image/png',body:Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aFOkAAAAASUVORK5CYII=','base64')})
  if(p.startsWith('/api/reading/'))return send(null)
  if(['/api/reading','/api/sync-history','/api/trash'].includes(p))return send([])
  if(p==='/api/stats')return send({comics:0,jobs:{}})
  if(p==='/api/health')return send({status:'ok',version:'test',commit:'test'})
  return send({})
 })
 return {finishSync(){syncState=[{...syncState[0],status:'success',stage:'finished',processed:10,enqueued:3,skipped:7}]}}
}
test('login, search and enqueue download',async({page})=>{
 await mockBackend(page,{logged:false});await page.goto('/#/login')
 await page.getByPlaceholder('admin').fill('tester');await page.getByPlaceholder('请输入密码').fill('test-only-password');await page.getByRole('button',{name:'登 录',exact:true}).click()
 await expect(page).toHaveURL(/dashboard/)
 await page.goto('/#/search');await page.getByPlaceholder('输入关键词，回车搜索').fill('测试');await page.getByRole('button',{name:'搜索',exact:true}).click()
 await expect(page.locator('.ms-comic-title')).toHaveText('测试漫画')
 await page.getByRole('button',{name:'下载',exact:true}).click()
 await expect(page.getByText(/已加入下载队列/)).toBeVisible()
})
test('background sync survives navigation and displays completion',async({page})=>{
 const backend=await mockBackend(page);await page.goto('/#/accounts')
 await page.getByRole('button',{name:'立即同步',exact:true}).click()
 await expect(page.getByText(/同步已在后台启动/)).toBeVisible()
 await page.goto('/#/downloads');await expect(page.getByText('收藏同步',{exact:true})).toBeVisible()
 backend.finishSync();await expect(page.getByText(/账号 #1 · 完成/)).toBeVisible({timeout:10000})
})
test('reader shows images and supports chapter navigation',async({page})=>{
 await mockBackend(page);await page.goto('/#/reader/pica/test-book/1')
 await expect(page.locator('.rd-item img').first()).toBeVisible()
 await page.locator('.el-select').first().click();await page.getByRole('option',{name:/第二章/}).click()
 await expect(page).toHaveURL(/test-book\/2/)
 await page.locator('.rd-back').click();await expect(page).toHaveURL(/library/)
})
test('search reports upstream failure and recovers after network interruption',async({page,context})=>{
 await mockBackend(page,{searchError:true});await page.goto('/#/search')
 await page.getByPlaceholder('输入关键词，回车搜索').fill('测试');await page.getByRole('button',{name:'搜索',exact:true}).click()
 await expect(page.locator('.el-alert')).toContainText('上游暂时不可用')
 await context.setOffline(true);await page.getByRole('button',{name:'重试',exact:true}).click();await context.setOffline(false)
 await page.unroute('**/api/**');await mockBackend(page)
 await page.getByRole('button',{name:'重试',exact:true}).click();await expect(page.locator('.ms-comic-title')).toHaveText('测试漫画')
})

test('desktop reader jumps pages, keeps menu geometry and respects input keyboard focus',async({page})=>{
 await page.setViewportSize({width:1440,height:900});await mockBackend(page)
 await page.goto('/#/reader/pica/test-book/1')
 const input=page.getByRole('spinbutton',{name:'当前页码'})
 await expect(page.locator('.rd-item img').first()).toBeVisible()
 await input.fill('2');await input.press('Enter');await expect(input).toHaveValue('2')
 await input.focus();await input.press('ArrowLeft');await expect(input).toHaveValue('2')
 await input.blur()
 const before=await page.locator('.rd-scroll').evaluate(el=>el.scrollTop)
 await page.locator('.rd-scroll').click({position:{x:600,y:200}})
 await expect(page.getByRole('button',{name:'显示阅读工具栏'})).toBeVisible()
 expect(await page.locator('.rd-scroll').evaluate(el=>el.scrollTop)).toBe(before)
 await page.keyboard.press('Escape');await expect(page.locator('.rd-bar')).not.toHaveClass(/is-hidden/)
 await page.getByRole('button',{name:'阅读设置',exact:true}).click()
 await expect(page.getByRole('slider',{name:'阅读宽度'})).toBeVisible()
 await page.keyboard.press('Escape'); await expect(page.getByRole('dialog')).not.toBeVisible()
 await page.screenshot({path:'test-results/reader-desktop.png'})
})
test('phone reader fits narrow screens and provides chapter and page navigation',async({browser})=>{
 const context=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true,deviceScaleFactor:2})
 const page=await context.newPage();await mockBackend(page)
 await page.goto('http://127.0.0.1:4173/#/reader/pica/test-book/1')
 await expect(page.locator('.rd-item img').first()).toBeVisible()
 const bounds=await page.locator('.rd-more').boundingBox();expect(bounds.x+bounds.width).toBeLessThanOrEqual(390)
 await page.getByRole('button',{name:'下一页',exact:true}).tap()
 await expect(page.getByRole('spinbutton',{name:'当前页码'})).toHaveValue('2')
 await page.getByRole('button',{name:'阅读设置',exact:true}).tap()
 await expect(page.getByRole('dialog')).toBeVisible()
 await page.getByRole('dialog').locator('.el-select').click();await page.getByRole('option',{name:/第二章/}).click()
 await expect(page).toHaveURL(/test-book\/2/)
 await expect(page.getByRole('spinbutton',{name:'当前页码'})).toHaveValue('1')
 await expect(page.getByRole('dialog')).not.toBeVisible()
 await page.screenshot({path:'test-results/reader-mobile.png'})
 await page.locator('.rd-back').tap();await expect(page).toHaveURL(/library/)
 await context.close()
})
