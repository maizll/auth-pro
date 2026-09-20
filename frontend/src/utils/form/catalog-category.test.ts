import assert from 'node:assert/strict'
import {
  canDeleteCatalogCategory,
  catalogCategoryUsageCount,
  deleteCatalogCategoryConfirmMessage,
  extrasAfterDeletingCategory
} from './catalog-category'

const categories = [
  { key: 'payment', label: '支付', kind: 'plugin' as const, builtin: true },
  { key: 'realname', label: '实名认证', kind: 'plugin' as const, builtin: true },
  { key: 'other', label: '其他', kind: 'plugin' as const, builtin: true },
  { key: 'home-template', label: '首页模板', kind: 'template' as const, builtin: true },
  { key: 'theme', label: '主题', kind: 'plugin' as const, builtin: false },
  { key: 'landing', label: '落地页', kind: 'template' as const, builtin: false }
]

assert.equal(canDeleteCatalogCategory(categories[0]), false)
assert.equal(canDeleteCatalogCategory(categories[1]), false)
assert.equal(canDeleteCatalogCategory(categories[2]), false)
assert.equal(canDeleteCatalogCategory(categories[3]), false)
assert.equal(canDeleteCatalogCategory({ key: 'payment', builtin: false }), false)
assert.equal(canDeleteCatalogCategory({ key: 'home-template' }), false)
assert.equal(canDeleteCatalogCategory(categories[4]), true)
assert.equal(canDeleteCatalogCategory({ key: 'theme', builtin: false }), true)

assert.deepEqual(extrasAfterDeletingCategory(categories, 'theme'), [
  { key: 'landing', label: '落地页', kind: 'template' }
])
assert.deepEqual(extrasAfterDeletingCategory(categories, 'landing'), [
  { key: 'theme', label: '主题', kind: 'plugin' }
])
assert.deepEqual(extrasAfterDeletingCategory(categories, 'payment'), [
  { key: 'theme', label: '主题', kind: 'plugin' },
  { key: 'landing', label: '落地页', kind: 'template' }
])

assert.equal(catalogCategoryUsageCount([{ category: 'theme' }, { category: 'other' }], 'theme'), 1)
assert.equal(catalogCategoryUsageCount([{ category: 'other' }], 'theme'), 0)

assert.equal(deleteCatalogCategoryConfirmMessage('主题', 0), '确认删除分类「主题」？')
assert.equal(
  deleteCatalogCategoryConfirmMessage('主题', 2),
  '确认删除分类「主题」？\n仍有目录项使用该分类，删除后仅去掉分类标签，条目仍保留原 category 字段'
)

console.log('catalog-category ok')
