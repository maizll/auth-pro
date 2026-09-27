import type { ColumnOption } from '@/types'

/** 手机列表最多同时看见的列，含操作列。再多就会挤出屏幕。 */
const MOBILE_MAX_COLUMNS = 4

/** 390 宽手机扣掉页面边距后，表格大约这么宽。真正排版时会按表格实际宽度再算。 */
const MOBILE_BUDGET = 316
const WIDTH_OP = 104
const WIDTH_STATUS = 72
const WIDTH_DATE = 92
const WIDTH_COMPACT = 64
const FLEX_MIN = 40

function columnKey(col: ColumnOption) {
  return `${col.prop || ''} ${col.label || ''} ${col.type || ''}`
}

function isOperation(col: ColumnOption) {
  return col.prop === 'operation' || col.label === '操作'
}

function isStatus(col: ColumnOption) {
  return /状态|status|enabled/i.test(columnKey(col))
}

function isExpire(col: ColumnOption) {
  return /到期|expire/i.test(columnKey(col))
}

function isDate(col: ColumnOption) {
  return /到期|expire|时间|日期/.test(columnKey(col))
}

function isCompact(col: ColumnOption) {
  return /类型|金额|级别|权重|数量|次数|来源/.test(columnKey(col))
}

function isName(col: ColumnOption) {
  return /名称|域名|标题|账号|应用|活动|流水|内容|邮箱|name|domain|title/i.test(columnKey(col))
}

function isUtility(col: ColumnOption) {
  return col.type === 'index' || col.type === 'selection' || col.type === 'globalIndex'
}

function hiddenByUser(col: ColumnOption) {
  return col.visible === false || col.checked === false
}

function score(col: ColumnOption) {
  if (isOperation(col)) return 0
  if (isStatus(col)) return 1
  if (isExpire(col)) return 2
  if (isName(col)) return 3
  if (isDate(col)) return 4
  if (/类型|type/i.test(columnKey(col))) return 5
  return 9
}

function hasMobileMarks(cols: ColumnOption[]) {
  return cols.some(
    (col) =>
      col.mobileHidden != null || col.mobilePriority != null || col.mobileLabel || col.mobileWidth
  )
}

function pickMarked(cols: ColumnOption[]) {
  const kept = cols.filter((col) => {
    if (col.mobileHidden) return false
    if (isUtility(col) && col.mobilePriority == null) return false
    return true
  })
  if (kept.length <= MOBILE_MAX_COLUMNS) return kept
  const ops = kept.filter(isOperation)
  const rest = kept
    .filter((col) => !isOperation(col))
    .sort((a, b) => (a.mobilePriority ?? 50) - (b.mobilePriority ?? 50) || score(a) - score(b))
  return [...rest.slice(0, Math.max(0, MOBILE_MAX_COLUMNS - ops.length)), ...ops]
}

function pickAuto(cols: ColumnOption[]) {
  const ops = cols.filter(isOperation).slice(0, 1)
  const rest = cols
    .filter((col) => !isOperation(col) && !isUtility(col))
    .sort((a, b) => score(a) - score(b))
  return [...rest.slice(0, Math.max(0, MOBILE_MAX_COLUMNS - ops.length)), ...ops]
}

/** 选出手机上要留下的列，顺序仍按原表从左到右。 */
export function pickMobileColumns<T extends ColumnOption>(cols: T[]) {
  const available = (cols || []).filter((col) => !hiddenByUser(col))
  const picked = hasMobileMarks(available) ? pickMarked(available) : pickAuto(available)
  const pickedSet = new Set(picked)
  return available.filter((col) => pickedSet.has(col))
}

function baseWidth(col: ColumnOption) {
  if (col.mobileWidth) return col.mobileWidth
  if (isOperation(col)) return WIDTH_OP
  if (isStatus(col)) return WIDTH_STATUS
  if (isDate(col)) return WIDTH_DATE
  if (isCompact(col)) return WIDTH_COMPACT
  return 0
}

function canShrink(col: ColumnOption, width: number) {
  if (isStatus(col)) return width > WIDTH_STATUS
  if (isDate(col)) return width > WIDTH_DATE
  if (isOperation(col)) return width > WIDTH_OP
  return width > FLEX_MIN
}

/**
 * 按预算把列宽加成正数，加起来不超过 budget。
 * 名称类列吃掉剩余空间；状态、日期、操作优先保证能看全。
 */
export function assignMobileWidths(cols: ColumnOption[], budget: number) {
  const widths = cols.map((col) => baseWidth(col))
  let flexIndexes = widths
    .map((width, index) => (width === 0 ? index : -1))
    .filter((index) => index >= 0)
  if (!flexIndexes.length) {
    const index = cols.findIndex((col) => !isOperation(col) && !isStatus(col) && !isDate(col))
    if (index >= 0) {
      widths[index] = 0
      flexIndexes = [index]
    }
  }
  for (let guard = 0; guard < 400; guard += 1) {
    const fixed = widths.reduce((sum, width) => sum + width, 0)
    const leftover = budget - fixed
    const share = Math.floor(leftover / Math.max(flexIndexes.length, 1))
    if (!flexIndexes.length || share >= FLEX_MIN) {
      flexIndexes.forEach((index) => {
        widths[index] = share
      })
      const sum = widths.reduce((total, width) => total + width, 0)
      if (flexIndexes.length && sum < budget) widths[flexIndexes[0]] += budget - sum
      return widths
    }
    const shrinkAt = widths.findIndex((width, index) => canShrink(cols[index], width))
    if (shrinkAt < 0) {
      flexIndexes.forEach((index) => {
        widths[index] = Math.max(32, share)
      })
      return widths
    }
    widths[shrinkAt] -= 1
  }
  return widths
}

/**
 * 手机上只留 3～4 个关键列，并改成能放进屏幕的宽度。
 * 列上可以标 mobileHidden，或用 mobilePriority（数字小的优先保留）。
 * 没标注的列表按列名自动取名称、状态、到期和操作。
 */
export function layoutMobileColumns(cols: ColumnOption[]) {
  const picked = pickMobileColumns(cols)
  const widths = assignMobileWidths(picked, MOBILE_BUDGET)
  return picked.map((col, index) => ({
    ...col,
    label: col.mobileLabel || col.label,
    width: widths[index],
    minWidth: widths[index],
    fixed: undefined
  }))
}
