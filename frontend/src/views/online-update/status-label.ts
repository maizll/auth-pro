/** 更新通道只显示中文。不把产品内部名称露在页面上。 */
export function updateChannelLabel(channel?: string) {
  const value = (channel || '').trim().toLowerCase()
  if (!value || value === 'stable' || value === 'auth_pro') return '正式版'
  if (value === 'beta' || value === 'preview') return '测试版'
  return '正式版'
}

/** 没拿到最新版本时不能写成「已是最新」。 */
export function latestVersionStatus(input: {
  version?: string
  updateAvailable?: boolean
  unreachable?: boolean
}) {
  const version = (input.version || '').trim()
  if (input.updateAvailable && version) return '可更新'
  if (!version) return input.unreachable ? '暂时连不上官网' : '未检查'
  return '已是最新'
}
