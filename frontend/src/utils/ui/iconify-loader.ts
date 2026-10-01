/**
 * 离线图标加载器
 *
 * 入口只注册源码和菜单里实际出现的 Remix 图标。
 * 菜单图标选择器，或数据库里出现未收录图标时，再异步加载完整图标集。
 *
 * @module utils/ui/iconify-loader
 */
import { addCollection, iconLoaded } from '@iconify/vue'
import usedRemixIcons from './remix-icons-used.json'

type RemixIconSet = Parameters<typeof addCollection>[0]

addCollection(usedRemixIcons as RemixIconSet)

let fullRemixIcons: Promise<void> | null = null

function asRemixIconSet(mod: { default?: RemixIconSet } & Partial<RemixIconSet>): RemixIconSet {
  return (mod.default ?? mod) as RemixIconSet
}

/** 异步加载完整 Remix 图标集，不打进首屏入口。 */
function loadFullRemixIcons(): Promise<void> {
  if (!fullRemixIcons) {
    fullRemixIcons = import('@iconify-json/ri/icons.json').then((mod) => {
      addCollection(asRemixIconSet(mod))
    })
  }
  return fullRemixIcons
}

/** 图标选择器用的全部图标名。会先把完整图标集注册好。 */
export async function listRemixIconNames(): Promise<string[]> {
  const mod = await import('@iconify-json/ri/icons.json')
  const data = asRemixIconSet(mod)
  addCollection(data)
  fullRemixIcons = Promise.resolve()
  const names = [...Object.keys(data.icons ?? {}), ...Object.keys(data.aliases ?? {})]
  names.sort()
  return names
}

type IconMenu = { meta?: { icon?: string }; children?: IconMenu[] }

/** 菜单里有入口未收录的图标时，后台补齐完整图标集。 */
export function ensureRenderedRemixIcons(menus: IconMenu[]): void {
  const pending = [...menus]
  while (pending.length) {
    const item = pending.pop()
    if (!item) continue
    const icon = item.meta?.icon || ''
    if (icon.startsWith('ri:') && !iconLoaded(icon)) {
      void loadFullRemixIcons()
      return
    }
    if (item.children?.length) pending.push(...item.children)
  }
}
