export type BizStatusDomain = 'review' | 'license' | 'campaign' | 'ad'

export type BizTagType = 'primary' | 'success' | 'warning' | 'info' | 'danger'

export interface BizStatusView {
  label: string
  type: BizTagType
  effect: 'dark' | 'light' | 'plain'
  known: boolean
}

interface BizStatusEntry {
  label: string
  type: BizTagType
  effect: 'dark' | 'light' | 'plain'
}

/** 入驻审核：SOURCE_APPLICATION_STATUS */
const reviewStatus: Record<string, BizStatusEntry> = {
  pending: { label: '待审核', type: 'warning', effect: 'light' },
  approved: { label: '已通过', type: 'success', effect: 'light' },
  rejected: { label: '已拒绝', type: 'danger', effect: 'light' },
  cancelled: { label: '已取消', type: 'info', effect: 'light' },
  frozen: { label: '已冻结', type: 'info', effect: 'light' }
}

/**
 * 广告申请页与入驻审核共用 SOURCE_APPLICATION_STATUS。
 * 单独成域，避免以后广告文案变化时改到审核列表。
 */
const adStatus: Record<string, BizStatusEntry> = { ...reviewStatus }

/** 授权列表颜色 + 用户/代理端即将到期 */
const licenseStatus: Record<string, BizStatusEntry> = {
  active: { label: '正常', type: 'success', effect: 'light' },
  expiring: { label: '即将到期', type: 'warning', effect: 'light' },
  expired: { label: '已过期', type: 'info', effect: 'light' },
  disabled: { label: '已禁用', type: 'danger', effect: 'light' }
}

/** 活动页 statusMeta。disabled 是 warning，与授权域的 danger 不同。 */
const campaignStatus: Record<string, BizStatusEntry> = {
  active: { label: '进行中', type: 'success', effect: 'light' },
  upcoming: { label: '未开始', type: 'primary', effect: 'light' },
  ended: { label: '已结束', type: 'info', effect: 'light' },
  disabled: { label: '已禁用', type: 'warning', effect: 'light' }
}

const dictionaries: Record<BizStatusDomain, Record<string, BizStatusEntry>> = {
  review: reviewStatus,
  ad: adStatus,
  license: licenseStatus,
  campaign: campaignStatus
}

export function resolveBizStatus(input: {
  domain: BizStatusDomain
  status?: string | null
  label?: string
  emptyText?: string
}): BizStatusView {
  const raw = String(input.status ?? '').trim()
  const override = String(input.label ?? '').trim()
  const found = raw ? dictionaries[input.domain]?.[raw] : undefined
  if (!found) {
    return {
      label: override || raw || input.emptyText || '-',
      type: 'info',
      effect: 'light',
      known: false
    }
  }
  return {
    label: override || found.label,
    type: found.type,
    effect: found.effect,
    known: true
  }
}
