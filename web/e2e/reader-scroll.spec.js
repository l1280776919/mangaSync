import { test, expect } from '@playwright/test'

for (const mobile of [false,true]) test(`fast scroll remounts decoded pages safely (${mobile?'phone':'desktop'})`,async({browser})=>{
 const context=await browser.newContext({viewport:mobile?{width:390,height:844}:{width:1280,height:800},isMobile:mobile,hasTouch:mobile})
 const page=await context.newPage()
 await page.addInitScript(() => {
  const decode=HTMLImageElement.prototype.decode
  const pending=[]
  window.holdDecode=false
  window.releaseDecodes=()=>{window.holdDecode=false;pending.splice(0).forEach(resolve=>resolve())}
  HTMLImageElement.prototype.decode=function(){return decode.call(this).then(()=>window.holdDecode?new Promise(resolve=>pending.push(resolve)):undefined)}
 })
 await page.route('**/api/**',async route=>{
  const p=new URL(route.request().url()).pathname
  const send=data=>route.fulfill({contentType:'application/json',body:JSON.stringify(data)})
  if(p==='/api/auth/me')return send({username:'tester',isAdmin:true})
  if(p==='/api/comics/pica/scroll-book')return send({title:'Scroll test',chapters:[{order:1,title:'One'}]})
  if(p.endsWith('/meta'))return send({pages:40,sizes:Array.from({length:40},()=>[600,900])})
  if(p.includes('/page/')) {
   return route.fulfill({headers:{'Cache-Control':'no-store'},contentType:'image/svg+xml',body:'<svg xmlns="http://www.w3.org/2000/svg" width="600" height="900"><rect width="600" height="900" fill="#527dce"/></svg>'})
  }
  return send({})
 })
 try {
  await page.goto('http://127.0.0.1:4173/#/reader/pica/scroll-book/1')
  const first=page.locator('.rd-item[data-page="1"]')
  await expect(first.locator('img')).toHaveClass(/is-loaded/)
  await page.locator('.rd-scroll').evaluate(el=>{el.scrollTop=el.querySelector('[data-page="25"]').offsetTop-58})
  await expect(first.locator('img')).toHaveCount(0)
  await page.evaluate(()=>{window.holdDecode=true})
  await page.locator('.rd-scroll').evaluate(el=>{el.scrollTop=0})
  await expect(first.locator('img')).toHaveCount(1)
  await expect(first.locator('.rd-ph')).toBeVisible()
  await expect(first.locator('img')).not.toHaveClass(/is-loaded/)
  await page.evaluate(()=>window.releaseDecodes())
  await expect(first.locator('img')).toHaveClass(/is-loaded/)
  await expect(first.locator('.rd-ph')).toHaveCount(0)
  // Rapid direction changes must not leave stale load/error state on the final page.
  for(const target of [30,4,35,8,28,1]) {
   await page.locator('.rd-scroll').evaluate((el,n)=>{el.scrollTop=el.querySelector(`[data-page="${n}"]`).offsetTop-58},target)
   await page.evaluate(()=>new Promise(requestAnimationFrame))
  }
  await expect(first.locator('img')).toHaveClass(/is-loaded/)
  expect(await first.locator('img').evaluate(el=>el.complete&&el.naturalWidth>0)).toBe(true)
  expect(await page.locator('.rd-item img').count()).toBeLessThan(16)
 } finally { await page.evaluate(()=>window.releaseDecodes()).catch(()=>{});await context.close() }
})

test('short pages cover the whole viewport while scrolling',async({page})=>{
 await page.setViewportSize({width:390,height:844})
 await page.route('**/api/**',route=>{
  const p=new URL(route.request().url()).pathname
  const send=data=>route.fulfill({contentType:'application/json',body:JSON.stringify(data)})
  if(p==='/api/auth/me')return send({username:'tester',isAdmin:true})
  if(p.startsWith('/api/comics/'))return send({title:'Short pages',chapters:[{order:1,title:'One'}]})
  if(p.endsWith('/meta'))return send({pages:80,sizes:Array.from({length:80},()=>[600,60])})
  if(p.includes('/page/'))return route.fulfill({contentType:'image/svg+xml',body:'<svg xmlns="http://www.w3.org/2000/svg" width="600" height="60"><rect width="600" height="60" fill="#527dce"/></svg>'})
  return send({})
 })
 await page.goto('/#/reader/pica/short/1')
 await expect(page.locator('.rd-item').first().locator('img')).toHaveClass(/is-loaded/)
 await page.locator('.rd-scroll').evaluate(el=>{el.scrollTop=1000})
 await expect.poll(()=>page.locator('.rd-scroll').evaluate(sc=>{
  const top=sc.getBoundingClientRect().top,bottom=top+sc.clientHeight
  return [...sc.querySelectorAll('.rd-item')].filter(el=>{const r=el.getBoundingClientRect();return r.bottom>top&&r.top<bottom}).every(el=>{const img=el.querySelector('img');return img?.complete&&img.naturalWidth>0&&img.classList.contains('is-loaded')})
 })).toBe(true)
})
