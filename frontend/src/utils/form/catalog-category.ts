export type CatalogCategoryKind = 'plugin' | 'template'

export const BUILTIN_CATALOG_CATEGORY_KEYS = [
  'payment',
  'realname',
  'other',
  'home-template'
] as const

export interface CatalogCategoryInput {
  key: string
  label?: string
  kind?: CatalogCategoryKind
  builtin?: boolean
}

export function canDeleteCatalogCategory(item: CatalogCategoryInput): boolean {
  if (item.builtin === true) return false
  return !BUILTIN_CATALOG_CATEGORY_KEYS.includes(
    item.key as (typeof BUILTIN_CATALOG_CATEGORY_KEYS)[number]
  )
}

export function extrasAfterDeletingCategory(
  categories: CatalogCategoryInput[],
  key: string
): Array<{ key: string; label: string; kind: CatalogCategoryKind }> {
  const target = String(key || '').trim().toLowerCase()
  return categories
    .filter((item) => item.builtin !== true && item.key !== target)
    .map((item) => ({
      key: item.key,
      label: item.label || item.key,
      kind: item.kind === 'template' ? 'template' : 'plugin'
    }))
}

export function catalogCategoryUsageCount(
  items: Array<{ category?: string }>,
  key: string
): number {
  const target = String(key || '').trim().toLowerCase()
  return items.filter((item) => String(item.category || '').toLowerCase() === target).length
}

export function deleteCatalogCategoryConfirmMessage(label: string, usedCount: number): string {
  const message = `确认删除分类「${label}」？`
  if (usedCount > 0) {
    return `${message}\n仍有目录项使用该分类，删除后仅去掉分类标签，条目仍保留原 category 字段`
  }
  return message
}
