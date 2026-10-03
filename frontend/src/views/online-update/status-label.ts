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

/**
 * 下载和验签（进度不超过 50%）阶段失败时还没动网站文件。
 * 更新脚本在动文件之前失败时（如建不出备份目录）进度已到 95%，但原因里会写明「线上目录未改动」。
 */
export const stoppedBeforeInstall = (progress: number | undefined, reason?: string) =>
  Number(progress) <= 50 || /线上目录未改动|网站没有任何改动/.test(reason || '')
