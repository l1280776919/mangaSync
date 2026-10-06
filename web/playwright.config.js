import { defineConfig } from '@playwright/test'
export default defineConfig({
 testDir:'./e2e', timeout:30000, retries:process.env.CI?1:0, workers:1,
 reporter:[['list'],['html',{open:'never'}]],
 use:{launchOptions:process.env.MANGASYNC_CHROMIUM ? {executablePath:process.env.MANGASYNC_CHROMIUM,args:['--no-sandbox','--disable-gpu','--disable-dev-shm-usage']} : {},baseURL:'http://127.0.0.1:4173',trace:'retain-on-failure'},
 webServer:{command:'npm run dev -- --host 127.0.0.1 --port 4173',url:'http://127.0.0.1:4173',reuseExistingServer:!process.env.CI}
})
