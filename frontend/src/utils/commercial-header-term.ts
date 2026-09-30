/** 顶栏胶囊右侧只写到期日或「永久」，来源留在点击后的授权详情里。 */
export function commercialHeaderTerm(
  account?: {
    permanent?: boolean
    editionExpireAt?: number | null
  } | null
) {
  if (!account) return ''
  if (account.permanent || !account.editionExpireAt) return '永久'
  const date = new Date(account.editionExpireAt * 1000)
  if (Number.isNaN(date.getTime())) return '永久'
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} 到期`
}
