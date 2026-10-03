// 安装包文件按发布标签分组：客户端版本 / 收费插件 / 收费模板 / 其他。每个标签一行，安装包和 latest.json 合并显示。

export type PackageGroupKey = 'client' | 'plugin' | 'template' | 'other'

export interface PackageFile {
  key: string
  name: string
  size: number
  updatedAt?: string
  item?: string
  sha256?: string
  orphan?: boolean
  tag?: string
}

export interface PackageVersionRow {
  id: string
  group: PackageGroupKey
  tag: string
  /** 插件、模板的标识；客户端为空 */
  name: string
  version: string
  /** 旧格式标签（paid-plugin-、paid-template-） */
  legacy: boolean
  files: PackageFile[]
  /** 主文件：安装包本体，没有时取第一个文件 */
  main: PackageFile
  hasManifest: boolean
  size: number
  updatedAt: string
  item: string
  orphan: boolean
}

export const PACKAGE_GROUPS: { key: PackageGroupKey; label: string }[] = [
  { key: 'client', label: '客户端版本' },
  { key: 'plugin', label: '收费插件' },
  { key: 'template', label: '收费模板' },
  { key: 'other', label: '其他' }
]

const RULES: { prefix: string; group: PackageGroupKey; legacy: boolean }[] = [
  { prefix: 'client/', group: 'client', legacy: false },
  { prefix: 'plugins/paid/', group: 'plugin', legacy: false },
  { prefix: 'paid-plugin-', group: 'plugin', legacy: true },
  { prefix: 'templates/paid/', group: 'template', legacy: false },
  { prefix: 'paid-template-', group: 'template', legacy: true }
]

/** 文件所在的标签：GitHub、Gitee 由服务端给出；其他存储取文件所在目录。 */
function fileTag(file: PackageFile): string {
  if (file.tag) return file.tag
  const slash = file.key.lastIndexOf('/')
  return slash > 0 ? file.key.slice(0, slash) : ''
}

/** 按标签前缀认出分组、标识和版本号。 */
export function classifyTag(
  tag: string
): Pick<PackageVersionRow, 'group' | 'name' | 'version' | 'legacy'> {
  for (const rule of RULES) {
    if (!tag.startsWith(rule.prefix)) continue
    const rest = tag.slice(rule.prefix.length)
    if (rule.group === 'client') return { group: 'client', name: '', version: rest, legacy: false }
    const match = rest.match(/^(.+?)-(v?\d+(?:\.\d+)*(?:[-+.][0-9A-Za-z.-]+)?)$/)
    return {
      group: rule.group,
      name: match ? match[1] : rest,
      version: match ? match[2] : '',
      legacy: rule.legacy
    }
  }
  return { group: 'other', name: tag, version: '', legacy: false }
}

function versionParts(version: string): number[] {
  return version
    .replace(/^v/i, '')
    .split(/[.+-]/)
    .map((part) => (/^\d+$/.test(part) ? Number(part) : -1))
}

/** 版本号从新到旧。 */
export function compareVersionDesc(a: string, b: string): number {
  const left = versionParts(a)
  const right = versionParts(b)
  for (let i = 0; i < Math.max(left.length, right.length); i++) {
    const diff = (right[i] ?? 0) - (left[i] ?? 0)
    if (diff) return diff
  }
  return b.localeCompare(a)
}

function isManifest(file: PackageFile) {
  return file.name === 'latest.json'
}

/** 把文件按标签合并成行，并按分组归好、版本倒序。 */
export function groupPackageFiles(
  files: PackageFile[]
): Record<PackageGroupKey, PackageVersionRow[]> {
  const byTag = new Map<string, PackageFile[]>()
  for (const file of files) {
    const tag = fileTag(file)
    const list = byTag.get(tag) || []
    list.push(file)
    byTag.set(tag, list)
  }
  const result: Record<PackageGroupKey, PackageVersionRow[]> = {
    client: [],
    plugin: [],
    template: [],
    other: []
  }
  for (const [tag, list] of byTag) {
    const sorted = [...list].sort((a, b) => Number(isManifest(a)) - Number(isManifest(b)))
    const main = sorted[0]
    const info = classifyTag(tag)
    const updated =
      list
        .map((file) => file.updatedAt || '')
        .sort()
        .pop() || ''
    result[info.group].push({
      id: tag || main.key,
      tag,
      ...info,
      files: sorted,
      main,
      hasManifest: list.some(isManifest),
      size: main.size || 0,
      updatedAt: updated,
      item: list.find((file) => file.item)?.item || '',
      orphan: list.every((file) => file.orphan)
    })
  }
  for (const rows of Object.values(result)) {
    rows.sort(
      (a, b) =>
        a.name.localeCompare(b.name) ||
        compareVersionDesc(a.version, b.version) ||
        b.tag.localeCompare(a.tag)
    )
  }
  return result
}

/** 搜索：标签、标识、版本、对应条目、文件名都能搜，不分大小写。 */
export function filterPackageRows(rows: PackageVersionRow[], keyword: string): PackageVersionRow[] {
  const word = keyword.trim().toLowerCase()
  if (!word) return rows
  return rows.filter((row) =>
    [row.tag, row.name, row.version, row.item, ...row.files.map((file) => file.name)].some((text) =>
      text.toLowerCase().includes(word)
    )
  )
}
