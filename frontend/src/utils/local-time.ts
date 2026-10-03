// 接口里带时区的时间（如 2026-10-03T04:18:04Z）按浏览器所在时区显示，避免把 UTC 当成本地时间。
// 不带时区的「2026-10-03 12:18:04」本来就是站点时间，只整理格式。
const ZONED = /T.*([zZ]|[+-]\d{2}:?\d{2})$/

export function formatLocalDateTime(value?: string | null, withSeconds = true): string {
  const text = (value || '').trim()
  if (!text || text.startsWith('0001')) return ''
  const length = withSeconds ? 19 : 16
  if (!ZONED.test(text)) return text.replace('T', ' ').slice(0, length)
  const date = new Date(text)
  if (Number.isNaN(date.getTime())) return text.replace('T', ' ').slice(0, length)
  const pad = (n: number) => String(n).padStart(2, '0')
  const minutes = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
  return withSeconds ? `${minutes}:${pad(date.getSeconds())}` : minutes
}
