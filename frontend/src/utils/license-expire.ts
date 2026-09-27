/** 到期时间为空表示永久授权。列表里写「永久」，不要写成「--」。手机上只保留日期，避免时间把列撑出屏幕。 */
export function formatLicenseExpire(value?: string | null, dateOnly = false) {
  const text = String(value ?? '').trim()
  if (!text || text === '--') return '永久'
  if (dateOnly) {
    const matched = /^(\d{4}-\d{2}-\d{2})/.exec(text)
    if (matched) return matched[1]
  }
  return text
}
