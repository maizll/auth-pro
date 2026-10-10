/** 同屏最多 2 处广告；按声明顺序抢占。 */
const MAX_ON_SCREEN = 2
const active = new Set<string>()

const PRIORITY: Record<string, number> = {
  'console-home': 1,
  'console-sidebar': 2,
  sidebar: 2,
  'console-login': 3,
  'console-topbar': 4,
  'console-rail': 5,
  'store-native': 6,
  'update-done': 7,
  'list-footer': 8,
  'profile-side': 9,
  'docs-side': 10,
  'lock-screen': 11,
  'home-banner': 12
}

export function claimAdSlot(position: string): boolean {
  if (active.has(position)) return true
  if (active.size >= MAX_ON_SCREEN) {
    // 若当前位优先级高于已占位，可替换最低优先级（简化：直接拒绝后续位）
    const worst = [...active].sort((a, b) => (PRIORITY[b] || 99) - (PRIORITY[a] || 99))[0]
    const my = PRIORITY[position] || 99
    const their = PRIORITY[worst] || 99
    if (my >= their) return false
    active.delete(worst)
  }
  active.add(position)
  return true
}

export function releaseAdSlot(position: string) {
  active.delete(position)
}
