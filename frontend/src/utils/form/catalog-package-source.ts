import { isHttpsLocation } from './catalog-slug'

export interface CatalogUploadGateInput {
  appId: number
  push: boolean
  hasFile: boolean
  location: string
}

/** 返回空字符串表示「仅校验解析」和「校验并保存元数据」可以点击。 */
export function catalogUploadBlockReason(input: CatalogUploadGateInput): string {
  if (!input.appId || input.appId <= 0) return '请选择应用'
  if (input.push) {
    return input.hasFile ? '' : '推送 Release 时请先选择压缩包'
  }
  const location = String(input.location || '').trim()
  if (input.hasFile) {
    return location ? '' : '未推送 Release 时请填写 https 外部地址'
  }
  if (!location) return '请上传压缩包，或填写 https 外部地址（二选一）'
  if (!isHttpsLocation(location)) return '外部地址须以 https:// 开头'
  return ''
}
