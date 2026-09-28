/** 后端 resolveCatalogAccess 的结果。列表、按钮和购买弹窗只读这一份。 */
export interface CatalogAccess {
  party: 'official' | 'third' | string
  commercialIncluded: boolean
  owned: boolean
  /** 空、commercial（商业版包含）或 purchase（单独买过）。 */
  grant: '' | 'commercial' | 'purchase' | string
}

interface CatalogAccessView {
  party: 'official' | 'third'
  commercialIncluded: boolean
  owned: boolean
  needsPurchase: boolean
  badge: { text: string; icon: string; tone: 'ok' | 'primary' } | null
}

/** 把目录接口返回的 access 变成界面用的官方/第三方、是否包含、是否要购买。 */
export function catalogItemAccess(
  item?: {
    priceCents?: number
    access?: CatalogAccess | null
  } | null
): CatalogAccessView {
  const price = item?.priceCents || 0
  const access = item?.access
  const party = access?.party === 'third' ? 'third' : 'official'
  const commercialIncluded = !!access?.commercialIncluded
  const owned = price <= 0 || !!access?.owned
  const needsPurchase = price > 0 && !owned
  let badge: CatalogAccessView['badge'] = null
  if (price > 0 && access?.grant === 'purchase') {
    badge = { text: '已包含', icon: 'ri:shield-check-fill', tone: 'ok' }
  } else if (price > 0 && commercialIncluded) {
    badge = { text: '商业版免费', icon: 'ri:rocket-2-line', tone: 'primary' }
  }
  return { party, commercialIncluded, owned, needsPurchase, badge }
}
