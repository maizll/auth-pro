import { h, reactive } from 'vue'
import { ElButton, ElNotification } from 'element-plus'
import type { StoreAccount } from '@/api/store'
import CommercialMark from '@/components/business/commercial/CommercialMark.vue'

export const commercialCopy: Record<string, string> = {
  multi_app: '免费版仅支持 1 个授权应用，升级商业版可创建多个',
  paid_plugin: '该插件需要商业版，升级后可一键安装',
  paid_template: '该模板需要商业版，升级后可一键安装'
}

export const commercialUi = reactive({
  upgradeOpen: false,
  promptOpen: false,
  licenseOpen: false,
  promptText: '',
  feature: '',
  account: null as StoreAccount | null
})

export type CommercialCta = 'upgrade' | 'renew' | 'view'

export function rememberCommercialAccount(account: StoreAccount | null) {
  commercialUi.account = account
}

/** 商业版仍有效：已是商业版，且没有域名不符或明确吊销。 */
export function isCommercialActive(account?: StoreAccount | null) {
  return !!account && account.edition === 'commercial' && !account.domainMismatch && !account.explicitRevoked
}

export function commercialCta(account?: StoreAccount | null): CommercialCta {
  if (!isCommercialActive(account)) return 'upgrade'
  return account?.permanent ? 'view' : 'renew'
}

export function commercialCtaLabel(cta: CommercialCta) {
  if (cta === 'view') return '查看授权'
  if (cta === 'renew') return '续费'
  return '升级商业版'
}

export function commercialText(feature?: string, fallback?: string) {
  if (feature && commercialCopy[feature]) return commercialCopy[feature]
  if (fallback && fallback !== '该功能需要商业版') return fallback
  return '该功能需要商业版'
}

export function openCommercialUpgrade() {
  commercialUi.licenseOpen = false
  commercialUi.upgradeOpen = true
}

export function openCommercialLicense() {
  commercialUi.upgradeOpen = false
  commercialUi.licenseOpen = true
}

/** 套餐时长只显示中文，不把接口里的英文代号露到界面上。 */
export function commercialPeriodText(period?: string) {
  const value = (period || '').trim().toLowerCase()
  if (!value) return ''
  if (value === 'permanent') return '永久'
  if (value === 'yearly') return '一年'
  const days = /^d(\d+)$/.exec(value)
  if (days) return `${Number(days[1])} 天`
  if (/^\d+$/.test(value)) return `${Number(value)} 天`
  return ''
}

export function commercialPlanLabel(plan: { name: string; priceCents: number; period?: string }) {
  const price = `${(plan.priceCents / 100).toFixed(2)} 元`
  const period = commercialPeriodText(plan.period)
  return period ? `${plan.name} · ${period} · ${price}` : `${plan.name} · ${price}`
}

export function commercialExpireText(account?: StoreAccount | null) {
  if (!account) return '未知'
  if (account.permanent) return '永久'
  if (!account.editionExpireAt) return '未设置到期时间'
  const date = new Date(account.editionExpireAt * 1000)
  if (Number.isNaN(date.getTime())) return '未设置到期时间'
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
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
        () => commercialCtaLabel(commercialCta(commercialUi.account))
      )
    ])
  })
}
