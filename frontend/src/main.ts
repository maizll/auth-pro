import App from './App.vue'
import { createApp } from 'vue'
import { initStore } from './store'                 // Store
import { initRouter } from './router'               // Router
import language from './locales'                    // 国际化
import '@styles/core/tailwind.css'                  // tailwind
import '@styles/index.scss'                         // 样式
import '@utils/ui/iconify-loader'                   // 离线图标
import { setupGlobDirectives } from './directives'
import { setupErrorHandle } from './utils/sys/error-handle'
import { useSystemConfigStore } from './store/modules/system-config'

document.addEventListener(
  'touchstart',
  function () {},
  { passive: false }
)

// 控制台标识。不展示外部社区或联系方式。
const consoleTail = () => {
  const style1 = 'background: linear-gradient(90deg, #667eea 0%, #764ba2 100%); color: #fff; padding: 5px 10px; border-radius: 3px; font-weight: bold;'
  console.log('%c授权管理系统', style1)
}

consoleTail()

const bootstrap = async () => {
  const app = createApp(App)
  initStore(app)
  await useSystemConfigStore().loadPublicConfig()
  initRouter(app)
  setupGlobDirectives(app)
  setupErrorHandle(app)

  app.use(language)
  app.mount('#app')
}

void bootstrap()
