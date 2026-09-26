export function licenseTargetError(type: string, value: string): string {
  const target = (value || '').trim().toLowerCase()
  if (!target) return '请填写授权目标'
  if (type === 'domain' && !isValidSingleDomain(target)) return '单域名格式不正确'
  if (type === 'wildcard' && (!target.startsWith('*.') || !isValidSingleDomain(target.slice(2)))) {
    return '泛域名格式不正确'
  }
  if (type === 'ip' && !isValidIP(target)) return 'IP 格式不正确'
  return ''
}

function isValidSingleDomain(value: string) {
  if (!value || value.startsWith('*.') || value.endsWith('.') || /[/:@\s]/.test(value) || isValidIP(value)) {
    return false
  }
  const labels = value.split('.')
  if (labels.length < 2) return false
  if (!/^[a-z]{2,}$/.test(labels[labels.length - 1])) return false
  return labels.every((label) => /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(label))
}

function isValidIP(value: string) {
  const ipv4 = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/
  const ipv6 = /^(([0-9a-f]{1,4}:){7}[0-9a-f]{1,4}|::1|::)$/i
  return ipv4.test(value) || ipv6.test(value)
}
