import { test, expect } from '@playwright/test'
import fs from 'node:fs'
const books=Array.from({length:8},(_,i)=>({id:i+1,kind:i%2?'jm':'pica',comicId:String(i+1),title:['雨后的城市','山与海之间','无声的旅程','星河来信'][i%4],author:'示例作者',chapters:12,chaptersDone:12,images:240,bytes:128000000,complete:true,downloaded:true,categories:['冒险'],path:'/comics/book',source:'download'}))
async function fixtures(page){
 await page.route('**/api/**',route=>{
  const p=new URL(route.request().url()).pathname
  const send=data=>route.fulfill({contentType:'application/json',body:JSON.stringify(data)})
  if(p==='/api/auth/me')return send({username:'reader',isAdmin:true})
  if(p==='/api/accounts')return send([{id:1,kind:'pica',username:'reader',label:'我的哔咔',status:'ok',favoritesCount:8}])
  if(p==='/api/stats')return send({accounts:1,favorites:8,downloads:{running:1,queued:2,done:24,failed:0},library:{comics:8,images:1920,bytes:1024000000},disk:{freeBytes:128000000000,totalBytes:256000000000}})
  if(p==='/api/library'||p==='/api/search'||p.endsWith('/favorites'))return send({items:books,total:8,stats:{comics:8,chapters:96,images:1920,bytes:1024000000},snapshot:{hasSnapshot:true,updatedAt:new Date().toISOString()}})
  if(p==='/api/reading')return send(books.slice(0,2).map(b=>({...b,order:3,page:8})))
  if(p==='/api/downloads')return send({items:[{id:1,title:'山与海之间',comicId:'2',kind:'jm',status:'running',chaptersTotal:12,chaptersDone:4,imagesTotal:240,imagesDone:82,bytes:24000000,speedBps:2400000}],total:1})
  if(p.endsWith('/cover')) {
   const id=Number(p.split('/').at(-2))||1,colors=['#a1b7af','#d0b79d','#a7b6ca','#c8adac'],color=colors[id%4]
   return route.fulfill({contentType:'image/svg+xml',body:`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 300 400"><rect width="300" height="400" fill="${color}"/><circle cx="215" cy="100" r="42" fill="#fff" opacity=".7"/><path d="M0 320 L130 120 L230 300 L300 180 V400 H0Z" fill="#233b4a" opacity=".65"/><text x="24" y="365" fill="white" font-family="serif" font-size="30">VOLUME 0${id}</text></svg>`})
  }
  if(p==='/api/health')return send({version:'main',commit:'test'})
  if(p==='/api/diagnostics')return send({network:[]})
  if(p==='/api/settings')return send({downloadRoot:'/comics',concurrency:2,imageWorkers:8,quality:'original',schedule:{enabled:true,time:'04:30'}})
  if(['/api/sync-status','/api/sync-history','/api/trash'].includes(p))return send([])
  return send({})
 })
 // Optional local font, never required by CI or shipped with the app.
 if(fs.existsSync('/tmp/mangasync-review-font.css')) {
  const css=fs.readFileSync('/tmp/mangasync-review-font.css','utf8')
  await page.addInitScript(css=>{document.addEventListener('DOMContentLoaded',()=>{const s=document.createElement('style');s.textContent=css;document.head.append(s)})},css)
 }
}
for(const device of ['desktop','mobile'])test(`UI review ${device}`,async({browser})=>{
 const context=await browser.newContext({viewport:device==='mobile'?{width:390,height:844}:{width:1440,height:1000},isMobile:device==='mobile',hasTouch:device==='mobile'})
 const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));await fixtures(page)
 for(const name of ['dashboard','favorites','library','search','downloads','accounts','settings']) {
  await page.goto(`http://127.0.0.1:4173/#/${name}`)
  await expect(page.locator('.app-header')).toBeVisible()
  await expect(page.locator('.el-loading-mask')).toHaveCount(0)
  if(name==='search'){await page.getByPlaceholder('输入关键词，回车搜索').fill('山');await page.getByRole('button',{name:'搜索',exact:true}).click();await expect(page.locator('.ms-comic')).toHaveCount(8)}
  await expect(page.locator('.page-intro h1')).toBeVisible()
  if(device==='mobile') {
   await expect(page.getByRole('navigation',{name:'常用页面'})).toBeVisible()
   if(name==='favorites') {
    await expect(page.locator('.ms-mbar')).toHaveCount(0)
    await page.getByRole('button',{name:'全选本页',exact:true}).click()
    await expect(page.locator('.ms-mbar')).toBeVisible()
    const bar=await page.locator('.ms-mbar').boundingBox(),nav=await page.locator('.mobile-nav').boundingBox()
    expect(bar.y+bar.height).toBeLessThanOrEqual(nav.y+1)
    await page.locator('.ms-mbar').getByRole('button',{name:'取消全选'}).click()
   }
   if(name==='library') { await expect(page.locator('.library-summary')).not.toHaveAttribute('open','');await page.locator('.library-summary summary').click();await expect(page.locator('.library-summary .ms-stat-grid')).toBeVisible();await page.locator('.library-summary summary').click() }
  }
  await page.evaluate(()=>document.fonts.ready)
  const overflow=await page.locator('.app-main').evaluate(el=>el.scrollWidth>el.clientWidth+2)
  expect(overflow,`${name} horizontal overflow`).toBe(false)
  await page.screenshot({animations:'disabled',path:`${process.env.UI_REVIEW_DIR||'test-results'}/${device}-${name}.png`})
 }
 if(device==='mobile'){await page.setViewportSize({width:320,height:740});await page.getByRole('button',{name:'前往漫画库'}).click();await expect(page).toHaveURL(/library/);expect(await page.locator('.app-main').evaluate(el=>el.scrollWidth>el.clientWidth+2)).toBe(false);await page.getByRole('button',{name:'打开导航'}).click();await expect(page.locator('.nav-drawer')).toBeVisible()}
 expect(errors).toEqual([]);await context.close()
})
