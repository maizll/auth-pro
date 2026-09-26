import zhMessages from '../../locales/langs/zh.json'

type MenuNode = string | { [key: string]: MenuNode }

/** 映射表缺失时的中文占位。不要回退成英文 key 片段。 */
export const MISSING_MENU_TITLE_ZH = '未命名菜单'

function flattenMenuTitles(node: MenuNode, prefix: string, out: Record<string, string>) {
  if (typeof node === 'string') {
    if (prefix) out[prefix] = node
    return
  }
  for (const [key, value] of Object.entries(node)) {
    flattenMenuTitles(value, prefix ? `${prefix}.${key}` : key, out)
  }
}

/**
 * 菜单 i18n key → 中文。与 locales/langs/zh.json 的 menus 共用一份来源，
 * 避免侧栏映射表漏登记后回退成 edition 这类英文片段。
 */
export const MENU_TITLE_ZH: Record<string, string> = {}
flattenMenuTitles(zhMessages.menus as MenuNode, 'menus', MENU_TITLE_ZH)

const DEMO_MENU_NAMES = new Set([
  'Result',
  'ResultSuccess',
  'ResultFail',
  'Exception',
  'Exception403',
  'Exception404',
  'Exception500'
])

export function resolveMenuTitle(title?: string | null, fallback = ''): string {
  const raw = String(title ?? '').trim()
  if (!raw) return fallback
  if (MENU_TITLE_ZH[raw]) return MENU_TITLE_ZH[raw]
  if (raw.startsWith('menus.')) return MISSING_MENU_TITLE_ZH
  return raw
}

export function isDemoMenuName(name?: string | null): boolean {
  return DEMO_MENU_NAMES.has(String(name || ''))
}

export function stripDemoMenus<T extends { name?: string; children?: T[] }>(items: T[]): T[] {
  return items
    .filter((item) => !isDemoMenuName(item.name))
    .map((item) =>
      item.children?.length ? { ...item, children: stripDemoMenus(item.children) } : item
    )
}

/** 管理列表「名称」列：解析 i18n key，空 title 回退路由 name，保证可读且非空。 */
export function formatManageMenuName(row: { title?: string | null; name?: string | null }): string {
  return resolveMenuTitle(row.title, String(row.name || ''))
}

/** 把管理树的 title 写成可读中文，编辑回填和表格默认单元格都能直接用。 */
export function resolveManageMenuTree<
  T extends { title?: string; name?: string; children?: T[] }
>(items: T[]): T[] {
  return items.map((item) => ({
    ...item,
    title: formatManageMenuName(item),
    children: item.children?.length ? resolveManageMenuTree(item.children) : item.children
  }))
}
