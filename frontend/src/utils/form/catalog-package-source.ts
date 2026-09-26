import { isHttpsLocation, isTemplateLocation, parseCatalogPriceYuan } from './catalog-slug'

export type CatalogPackageSource = 'upload' | 'public'
export type CatalogPriceSwitchPolicy = 'grandfather' | 'purchase_only'

/** 已公开的免费条目要改成收费，或收费条目改回免费。 */
export function catalogPriceSwitchAction(
  currentCents: number,
  nextCents: number,
  status: string,
  latestVersion: string
): '' | 'to-paid' | 'to-free' {
  const visible = status === 'published' || status === 'hidden' || String(latestVersion || '').trim() !== ''
  if ((currentCents || 0) <= 0 && nextCents > 0 && visible) return 'to-paid'
  if ((currentCents || 0) > 0 && nextCents <= 0) return 'to-free'
  return ''
}

export interface CatalogUploadGateInput {
  appId: number
  source: CatalogPackageSource
  push: boolean
  hasFile: boolean
  location: string
  priceYuan: string
}

/** 返回空字符串表示「仅校验解析」和「校验并保存元数据」可以点击。 */
export function catalogUploadBlockReason(input: CatalogUploadGateInput): string {
  if (!input.appId || input.appId <= 0) return '请选择应用'
  const priceText = String(input.priceYuan ?? '').trim()
  if (priceText && parseCatalogPriceYuan(priceText) == null) return '售价请填写数字，最多两位小数'
  const cents = parseCatalogPriceYuan(priceText || '0') ?? 0
  const location = String(input.location || '').trim()
  if (input.source === 'upload') {
    if (!input.hasFile) return '请选择压缩包'
    if (cents > 0) return ''
    if (input.push) return ''
    return '免费条目请改用公开地址。上传压缩包用于收费条目，或勾选推送 Release'
  }
  if (!location) return '请填写 https 公开地址'
  if (!isHttpsLocation(location)) return '外部地址须以 https:// 开头'
  return ''
}

export interface DeveloperCatalogGateInput {
  kind: 'plugin' | 'template'
  source: CatalogPackageSource
  location: string
  sha256: string
  priceYuan: string
  /** 已有本站保管的安装包时，不必再传地址。 */
  hasStoredPackage?: boolean
  /** 提交审核时要求地址和校验码已经填好。保存草稿允许免费公开地址先留空。 */
  requirePackage: boolean
}

function developerPublicLocationOK(kind: DeveloperCatalogGateInput['kind'], location: string) {
  return kind === 'template' ? isTemplateLocation(location) : isHttpsLocation(location)
}

/** 开发者登记插件/模板时，按钮不可用的中文原因。空字符串表示可以继续。 */
export function developerCatalogBlockReason(input: DeveloperCatalogGateInput): string {
  const priceText = String(input.priceYuan ?? '').trim()
  if (priceText && parseCatalogPriceYuan(priceText) == null) return '售价请填写数字，最多两位小数'
  const cents = parseCatalogPriceYuan(priceText || '0') ?? 0
  const location = String(input.location || '').trim()
  const sha = String(input.sha256 || '').trim()
  const kept = Boolean(input.hasStoredPackage)
  if (input.source === 'upload') {
    if (!location && !kept) return '请先上传压缩包'
    return ''
  }
  if (location && !developerPublicLocationOK(input.kind, location)) {
    return input.kind === 'template'
      ? '模板地址须为 https 开头，或相对路径如 templates/demo-home.json'
      : '下载地址须为 https 开头的外链'
  }
  if (cents > 0) {
    if (input.requirePackage && !location && !kept) {
      return input.kind === 'template' ? '请填写 https 公开地址' : '请填写 https 公开地址'
    }
    return ''
  }
  if (input.requirePackage && !location) {
    return input.kind === 'template' ? '请填写模板地址' : '请填写 https 公开地址'
  }
  if (input.requirePackage && location && !sha) return '提交审核前请填写校验码'
  return ''
}
