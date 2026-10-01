import App from './App.vue'
import { createApp } from 'vue'
import { Icon } from '@iconify/vue'
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

const bootstrap = () => {
  const app = createApp(App)
  // 模板里的 iconify-icon 与 ArtSvgIcon 共用同一套离线图标。
  app.component('iconify-icon', Icon)
  initStore(app)
  // 先用本地记住的站点配置挂载，再在后台刷新，避免首屏被配置请求挡住。
  void useSystemConfigStore().loadPublicConfig()
  initRouter(app)
  setupGlobDirectives(app)
  setupErrorHandle(app)

  app.use(language)
  app.mount('#app')
  installMobileListFit()
}

void bootstrap()
