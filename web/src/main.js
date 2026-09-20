import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
/* 图标按需注册（清单见 icons.js）：全量注册会白送约 58KB gzip 的无用图标 */
import { registerIcons } from '@/icons'

/* mangaSync 浅色主题：脚本不再注入暗色类，Element Plus 使用默认浅色 css-vars */
import 'element-plus/dist/index.css'
import '@/styles/theme.css'

import App from '@/App.vue'
import router from '@/router'

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(ElementPlus, { locale: zhCn })

registerIcons(app)

app.mount('#app')
