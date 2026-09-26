import { isHttpsLocation, parseCatalogPriceYuan } from './catalog-slug'

export type CatalogPackageSource = 'upload' | 'public' | 'github'

export interface CatalogUploadGateInput {
  appId: number
  source: CatalogPackageSource
  push: boolean
  hasFile: boolean
  location: string
  priceYuan: string
}

const githubReleaseAsset =
  /^https:\/\/github\.com\/[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+\/releases\/download\/[^/]+\/[^/?#]+$/

export function isGitHubReleaseAssetURL(raw: string): boolean {
  return githubReleaseAsset.test(String(raw || '').trim())
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
    return '免费条目请改用公开地址。上传压缩包用于本站托管的收费条目，或勾选推送 Release'
  }
  if (input.source === 'github') {
    if (cents <= 0) return '私有 GitHub 仓库用于收费条目，请填写大于 0 的售价'
    if (!location) return '请粘贴 GitHub Release 资产链接'
    if (!isGitHubReleaseAssetURL(location)) {
      return '请粘贴 GitHub Release 资产链接，例如 https://github.com/所有者/仓库/releases/download/标签/文件名.zip'
    }
    return ''
  }
  if (cents > 0) return '公开地址只能用于免费条目。收费请改用私有 GitHub 仓库，或上传压缩包'
  if (!location) return '请填写 https 公开地址'
  if (!isHttpsLocation(location)) return '外部地址须以 https:// 开头'
  return ''
}
