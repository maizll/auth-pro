// 在线更新页「历史版本」的筛选、分组和默认展开规则。页面只负责渲染，规则都在这里，便于单测。
import type { OnlineUpdateRelease } from '@/api/update'

/** 首屏显示的版本数，滚到底每次再加这么多。 */
export const RELEASE_PAGE_SIZE = 10

export interface ReleaseGroup {
  /** 大版本，如 1.8.x */
  key: string
  /** 这一组在筛选结果里的全部版本，新版本在前 */
  releases: OnlineUpdateRelease[]
}

interface ReleaseGroupView {
  group: ReleaseGroup
  index: number
  open: boolean
  /** 这一组当前渲染几个版本（收起的组是 0） */
  shown: number
  /** 展开了但还没加载出来的版本数 */
  hidden: number
}

export const normalizeVersion = (value?: string) => (value || '').trim().replace(/^v/i, '')

/** 1.8.6 → 1.8.x；不是数字版本号的单独成组。 */
export function majorGroupKey(version: string): string {
  const parts = normalizeVersion(version).split('.')
  if (parts.length >= 2 && /^\d+$/.test(parts[0]) && /^\d+$/.test(parts[1])) {
    return `${parts[0]}.${parts[1]}.x`
  }
  return normalizeVersion(version) || '其他'
}

/** 按版本号或更新内容关键词筛选，不区分大小写；空关键词返回全部。 */
export function filterReleases(
  releases: OnlineUpdateRelease[],
  query: string
): OnlineUpdateRelease[] {
  const keyword = query
    .trim()
    .toLowerCase()
    .replace(/^v(?=\d)/, '')
  if (!keyword) return releases
  return releases.filter(
    (release) =>
      normalizeVersion(release.version).toLowerCase().includes(keyword) ||
      release.notes.some((note) => note.toLowerCase().includes(keyword))
  )
}

/** 保持接口给的顺序（新版本在前），按大版本分组。 */
export function groupReleases(releases: OnlineUpdateRelease[]): ReleaseGroup[] {
  const groups: ReleaseGroup[] = []
  for (const release of releases) {
    const key = majorGroupKey(release.version)
    let group = groups.find((item) => item.key === key)
    if (!group) {
      group = { key, releases: [] }
      groups.push(group)
    }
    group.releases.push(release)
  }
  return groups
}

/**
 * 决定每组渲染几个版本：分组标题总是显示；收起的组不渲染版本；展开的组最多显示 limitOf(组) 个，
 * 没显示完的组末尾放「加载更多」，滚到那里给这一组再加一页。默认只有最新一组展开，所以首屏最多 10 个。
 */
export function layoutGroups(
  groups: ReleaseGroup[],
  isOpen: (group: ReleaseGroup, index: number) => boolean,
  limitOf: (group: ReleaseGroup) => number
): ReleaseGroupView[] {
  return groups.map((group, index) => {
    const open = isOpen(group, index)
    const shown = open ? Math.min(Math.max(0, limitOf(group)), group.releases.length) : 0
    return { group, index, open, shown, hidden: open ? group.releases.length - shown : 0 }
  })
}

/** 默认展开的版本：最新版和当前版。 */
export function releaseOpenByDefault(version: string, latest?: string, current?: string): boolean {
  const value = normalizeVersion(version)
  return value !== '' && (value === normalizeVersion(latest) || value === normalizeVersion(current))
}

/** 默认展开的分组：第一组和含最新版或当前版的组；搜索时全部展开。 */
export function groupOpenByDefault(
  group: ReleaseGroup,
  index: number,
  latest?: string,
  current?: string,
  searching = false
): boolean {
  if (searching || index === 0) return true
  return group.releases.some((release) => releaseOpenByDefault(release.version, latest, current))
}

/** 数字版本号比较：a 比 b 新返回正数，旧返回负数；认不出的返回 null。 */
export function compareReleaseVersions(a?: string, b?: string): number | null {
  const parse = (value?: string) => {
    const parts = normalizeVersion(value).split('.')
    if (parts.length < 2 || parts.some((part) => !/^\d+$/.test(part))) return null
    return parts.map(Number)
  }
  const left = parse(a)
  const right = parse(b)
  if (!left || !right) return null
  for (let i = 0; i < Math.max(left.length, right.length); i++) {
    const diff = (left[i] ?? 0) - (right[i] ?? 0)
    if (diff) return diff
  }
  return 0
}

/**
 * 当前版本之后、最新版本为止的每一版，新版本在前。这一次更新会直接装到最新版，中间的版本不用逐个装。
 * releases.json 还没收录最新版时，用清单里的最新版补上，「落后几个版本」不会少算。
 */
export function pendingReleases(
  releases: OnlineUpdateRelease[],
  current?: string,
  latest?: OnlineUpdateRelease | null
): OnlineUpdateRelease[] {
  const latestVersion = latest?.version
  const newer = (version: string) => {
    const afterCurrent = compareReleaseVersions(version, current)
    const notAfterLatest = latestVersion ? compareReleaseVersions(version, latestVersion) : 0
    return (
      afterCurrent !== null && afterCurrent > 0 && notAfterLatest !== null && notAfterLatest <= 0
    )
  }
  const seen = new Set<string>()
  const list: OnlineUpdateRelease[] = []
  for (const release of releases) {
    const version = normalizeVersion(release.version)
    if (!newer(version) || seen.has(version)) continue
    seen.add(version)
    // 最新版的说明以清单为准，和「立即更新」装的是同一份
    if (latest && version === normalizeVersion(latestVersion) && latest.notes?.length) {
      list.push({ ...release, notes: latest.notes })
    } else {
      list.push(release)
    }
  }
  if (
    latest &&
    latestVersion &&
    newer(latestVersion) &&
    !seen.has(normalizeVersion(latestVersion))
  ) {
    list.push({ ...latest, version: normalizeVersion(latestVersion) })
  }
  return list.sort((a, b) => compareReleaseVersions(b.version, a.version) ?? 0)
}
