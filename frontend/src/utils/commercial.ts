import { h, reactive } from 'vue'
import { ElButton, ElNotification } from 'element-plus'
import CommercialMark from '@/components/business/commercial/CommercialMark.vue'

export const commercialCopy: Record<string, string> = {
  multi_app: '免费版仅支持 1 个授权应用，升级商业版可创建多个',
  paid_plugin: '该插件需要商业版，升级后可一键安装',
  paid_template: '该模板需要商业版，升级后可一键安装'
}

export const commercialUi = reactive({
  upgradeOpen: false,
  promptOpen: false,
  promptText: '',
  feature: ''
})

export function commercialText(feature?: string, fallback?: string) {
  if (feature && commercialCopy[feature]) return commercialCopy[feature]
  if (fallback && fallback !== '该功能需要商业版') return fallback
  return '该功能需要商业版'
}

export function openCommercialUpgrade() {
  commercialUi.upgradeOpen = true
}

export function openCommercialPrompt(text: string, feature = '') {
  commercialUi.promptText = text
  commercialUi.feature = feature
  commercialUi.promptOpen = true
}

export function notifyCommercialRequired(payload?: { msg?: string; data?: unknown }) {
  const data = payload?.data as { feature?: string } | undefined
  const feature = data?.feature || ''
  const text = commercialText(feature, payload?.msg)
  let note: { close: () => void } | undefined
  note = ElNotification({
    title: '需要商业版',
    duration: 8000,
    message: h('div', { class: 'commercial-toast' }, [
      h(CommercialMark, { text, icon: 'ri:vip-crown-fill' }),
      h(
        ElButton,
        {
          type: 'primary',
          size: 'small',
          style: 'margin-top:8px',
          onClick: () => {
            note?.close()
            openCommercialUpgrade()
          }
        },
        () => '升级商业版'
      )
    ])
  })
}
