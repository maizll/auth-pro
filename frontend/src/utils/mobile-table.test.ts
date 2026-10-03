import assert from 'node:assert/strict'
import { hasOwnDetailAction, mobileScrollLayout } from './mobile-table'

const cols = [
  { type: 'index' as const, width: 60, label: '序号', mobileHidden: true },
  { prop: 'domain', label: '域名', minWidth: 48, mobileHidden: true },
  { prop: 'appName', label: '应用', width: 120, mobileHidden: true },
  { prop: 'statusLabel', label: '状态', width: 72 },
  { prop: 'expireAt', label: '到期时间', width: 96 },
  { prop: 'createdAt', label: '创建时间', width: 80, mobileHidden: true },
  { prop: 'operation', label: '操作', width: 104 }
]

const laid = mobileScrollLayout(cols)
assert.equal(laid.length, cols.length)
assert.equal(laid.at(-1)?.fixed, 'right')
assert.equal(laid.at(-1)?.prop, 'operation')
assert.ok(Number(laid.find((col) => col.prop === 'domain')?.width) >= 168)
assert.equal(laid.at(-1)?.width, 108)
const wideOp = mobileScrollLayout([{ prop: 'operation', label: '操作', width: 176 }])
assert.equal(wideOp[0].width, 176)
const sum = laid.reduce((total, col) => total + Number(col.width || 0), 0)
assert.ok(sum > 390, `列宽合计 ${sum} 应超出手机屏`)

const hidden = mobileScrollLayout([
  { prop: 'name', label: '名称', width: 200, visible: false },
  { prop: 'operation', label: '操作', width: 80 }
])
assert.equal(hidden.length, 1)
assert.equal(hidden[0].fixed, 'right')

// 操作列已有「详情」或「查看」时不再插入通用「详情」
assert.equal(hasOwnDetailAction(['详情', '编辑', '更多']), true)
assert.equal(hasOwnDetailAction([' 查看 ']), true)
assert.equal(hasOwnDetailAction(['查看详情']), true)
assert.equal(hasOwnDetailAction(['编辑', '删除', '更多']), false)
assert.equal(hasOwnDetailAction([]), false)
