// 顶栏和购买窗口共用的商业版状态：要不要升级、打开购买、以及源站要求重新绑定时怎么提示。
import { h, reactive } from 'vue'
import { ElButton, ElNotification } from 'element-plus'
import type { StoreAccount } from '@/api/store'
import CommercialMark from '@/components/business/commercial/CommercialMark.vue'
import { catalogItemAccess, type CatalogAccess } from './catalog-access'
import { storePayloadRebind } from './commercial-bind'

export { catalogItemAccess }

export {
  buyerRebindMessage,
  logoutFailureIsAlreadyGone,
  purchaseNeedsRebind,
  purchaseRebindNotice,
  sourceConfirmedBound
} from './commercial-bind'

export const commercialCopy: Record<string, string> = {
  multi_app: '免费版仅支持 1 个授权应用，升级商业版可创建多个',
  paid_plugin: '该插件需要购买后才能启用',
  paid_template: '该模板需要购买后才能启用'
}

/**
 * 购买窗对比文案。只写代码里真实存在的差别，改这里即可：
 * - 授权应用：第二个应用起要商业版的 multi_app（app.go / buyerFeatureEnabled）
 * - 官方付费插件、首页模板：标价大于 0 时，免费版需单独购买；商业版有效期内视为已包含，需点一次安装，不会自动全装（store_access.go）
 * 授权校验不看商业版，已有应用不会被删，这两项两边相同，不放进对比表。
 */
export const commercialPitch = {
  title: '升级商业版，解锁全部能力',
  subtitle:
    '免费版只能新建 1 个授权应用。商业版不限数量，官方付费插件和首页模板在有效期内可直接安装。'
}

export const commercialCompareRows = [
  { label: '授权应用', free: '1 个', commercial: '不限' },
  { label: '官方付费插件', free: '需单独购买', commercial: '全部包含' },
  { label: '官方首页模板', free: '需单独购买', commercial: '全部包含' }
] as const

export const commercialCompareNote =
  '已经存在的应用不会删除，授权校验也不看商业版。升级后不会自动安装全部插件和模板。'

export interface CatalogPurchaseOffer {
  kind: 'plugin' | 'template'
  id: string
  localId?: string
  name: string
  priceCents: number
  period?: string
  access?: CatalogAccess | null
  /** 开发者登记的图标。空则用名称首字。 */
  icon?: string
  /** 开发者登记的简介。界面最多先显示两行。 */
  summary?: string
  version?: string
  author?: string
  resume?: () => Promise<boolean>
}

export const catalogPurchaseResumeEvent = 'catalog-purchase-resume'

export const commercialUi = reactive({
  upgradeOpen: false,
  promptOpen: false,
  licenseOpen: false,
  rebindRequired: false,
  promptText: '',
  feature: '',
  account: null as StoreAccount | null,
  offer: null as CatalogPurchaseOffer | null
})

/** 源站已删除绑定。打开购买窗口并停在重新绑定，不把业务 401 当成管理员退出。 */
export function noteStoreRebind(data: unknown) {
  if (!storePayloadRebind(data)) return false
  commercialUi.rebindRequired = true
  commercialUi.licenseOpen = false
  commercialUi.promptOpen = false
  commercialUi.upgradeOpen = true
  return true
}

export type CommercialCta = 'upgrade' | 'renew' | 'view'

export function rememberCommercialAccount(account: StoreAccount | null) {
  commercialUi.account = account
}

/** 商业版仍有效：已是商业版，且没有域名不符或明确吊销。 */
export function isCommercialActive(account?: StoreAccount | null) {
  return (
    !!account &&
    account.edition === 'commercial' &&
    !account.domainMismatch &&
    !account.explicitRevoked
  )
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
  commercialUi.offer = null
  commercialUi.licenseOpen = false
  commercialUi.upgradeOpen = true
}

export function openCatalogPurchase(offer: CatalogPurchaseOffer) {
  // 已经拥有的条目直接启用。是否拥有只看 catalogItemAccess，不再在这里重算。
  if (!catalogItemAccess(offer).needsPurchase) {
    if (offer.resume) {
      void offer.resume()
      return
    }
    requestCatalogResume(offer)
    return
  }
  commercialUi.offer = offer
  commercialUi.promptOpen = false
  commercialUi.licenseOpen = false
  commercialUi.upgradeOpen = true
}

export function commercialYuanText(cents?: number) {
  const value = cents || 0
  if (!Number.isFinite(value) || value <= 0) return '¥0'
  if (value % 100 === 0) return `¥${value / 100}`
  return `¥${(value / 100).toFixed(2)}`
}

export function catalogCardBuyLabel(cents?: number) {
  return `购买 ${commercialYuanText(cents)}`
}

export function requestCatalogResume(offer: CatalogPurchaseOffer) {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent(catalogPurchaseResumeEvent, { detail: offer }))
}

export function openCommercialLicense() {
  commercialUi.upgradeOpen = false
  commercialUi.licenseOpen = true
}

/** 套餐时长只显示中文，不把接口里的英文代号露到界面上。 */
export function commercialPeriodText(period?: string) {
  const value = (period || '').trim().toLowerCase()
  if (!value) return ''
  if (value === 'permanent' || value === 'one_time') return '永久'
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
  const data = payload?.data as
    | {
        feature?: string
        kind?: string
        id?: string
        name?: string
        priceCents?: number
        period?: string
        access?: CatalogAccess | null
      }
    | undefined
  const feature = data?.feature || ''
  if ((feature === 'paid_plugin' || feature === 'paid_template') && data?.id) {
    const kind = data.kind === 'template' || feature === 'paid_template' ? 'template' : 'plugin'
    openCatalogPurchase({
      kind,
      id: data.id,
      name: data.name || (kind === 'template' ? '付费模板' : '付费插件'),
      priceCents: data.priceCents || 0,
      period: data.period,
      access: data.access
    })
    return
  }
  const text = commercialText(feature, payload?.msg)
  const note = ElNotification({
    title: '需要商业版',
    duration: 8000,
    message: h('div', { class: 'commercial-toast' }, [
      h(CommercialMark, { text, icon: 'ri:rocket-2-line' }),
      h(
        ElButton,
        {
          type: 'primary',
          size: 'small',
          style: 'margin-top:8px',
          onClick: () => {
            note.close()
            openCommercialUpgrade()
          }
        },
        () => commercialCtaLabel(commercialCta(commercialUi.account))
      )
    ])
  })
}
