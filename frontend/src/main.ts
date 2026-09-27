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
import { installMobileListFit } from './utils/mobile-list-fit'
import { useSystemConfigStore } from './store/modules/system-config'

document.addEventListener(
  'touchstart',
  function () {},
  { passive: false }
)

const bootstrap = async () => {
  const app = createApp(App)
  initStore(app)
  await useSystemConfigStore().loadPublicConfig()
  initRouter(app)
  setupGlobDirectives(app)
  setupErrorHandle(app)

  app.use(language)
  app.mount('#app')
  installMobileListFit()
}

void bootstrap()
