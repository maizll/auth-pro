import assert from 'node:assert/strict'
import { placeNotificationPanel, type Box } from './placement'

const viewport: Box = { top: 0, right: 1280, bottom: 800, left: 0 }

/** 后台主区域：左侧是侧栏，顶栏铃铛在主区域右上角 */
const adminFrame: Box = { top: 0, right: 1280, bottom: 800, left: 230 }

const bell: Box = { top: 14, right: 1188, bottom: 46, left: 1156 }

const placed = placeNotificationPanel(bell, viewport, adminFrame)

assert.equal(placed.top, bell.bottom + 6, '面板应紧贴铃铛下方，只留很小的间距')
assert.equal(placed.left + placed.width, bell.right, '面板右缘应与铃铛按钮右缘对齐')
assert.ok(
  placed.left < bell.right && placed.left + placed.width > bell.left,
  '面板应压在铃铛正下方'
)
assert.ok(placed.left >= adminFrame.left + 8, '面板不得越过后台主区域左边界')
assert.ok(placed.left + placed.width <= adminFrame.right - 8, '面板不得越过后台主区域右边界')
assert.ok(placed.top + placed.maxHeight <= adminFrame.bottom - 8, '面板不得超出站点下边界')
assert.ok(placed.top > bell.top, '面板不应跑到铃铛上方的顶边缘')

const insetFrame: Box = { top: 24, right: 1100, bottom: 700, left: 80 }
const edgeBell: Box = { top: 40, right: 1092, bottom: 72, left: 1060 }
const edgePlaced = placeNotificationPanel(edgeBell, viewport, insetFrame)
assert.ok(edgePlaced.left >= insetFrame.left + 8)
assert.ok(edgePlaced.left + edgePlaced.width <= insetFrame.right - 8)
assert.ok(
  edgePlaced.left < edgeBell.right && edgePlaced.left + edgePlaced.width > edgeBell.left,
  '贴边时仍应盖住铃铛，而不是整块挪到很远的地方'
)
assert.equal(edgePlaced.top, edgeBell.bottom + 6)

const shortViewport: Box = { top: 0, right: 900, bottom: 220, left: 0 }
const lowBell: Box = { top: 160, right: 860, bottom: 192, left: 828 }
const flipped = placeNotificationPanel(lowBell, shortViewport, shortViewport)
assert.equal(
  flipped.top + flipped.maxHeight,
  lowBell.top - 6,
  '下方空间不够时，面板底边应贴住铃铛上方'
)
assert.ok(flipped.top >= 8, '翻上去后仍应留在视口内')
assert.ok(flipped.maxHeight < shortViewport.bottom - 16, '不应把整块视口都铺成面板')
