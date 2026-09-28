import type { ColumnOption } from '@/types'

const WIDTH_OP = 108
const WIDTH_OP_MAX = 200
const WIDTH_STATUS = 80
const WIDTH_EXPIRE = 168
const WIDTH_DATE = 120
const WIDTH_NAME = 168
const WIDTH_COMPACT = 96
const WIDTH_DEFAULT = 120

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

function asNumber(value: unknown) {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed > 1 ? parsed : 0
}

function floorWidth(col: ColumnOption) {
  if (isOperation(col)) return WIDTH_OP
  if (isUtility(col)) return 48
  if (isStatus(col)) return WIDTH_STATUS
  if (isExpire(col)) return WIDTH_EXPIRE
  if (isDate(col)) return WIDTH_DATE
  if (isName(col)) return WIDTH_NAME
  if (isCompact(col)) return WIDTH_COMPACT
  return WIDTH_DEFAULT
}

/** 手机上保留全部列，并给出不挤进一屏的列宽。操作列固定在右侧。 */
export function mobileScrollLayout<T extends ColumnOption>(cols: T[]): ColumnOption[] {
  return (cols || [])
    .filter((col) => !hiddenByUser(col))
    .map((col) => {
      const next: ColumnOption = { ...col }
      if (col.mobileLabel) next.label = col.mobileLabel
      const floor = Math.max(floorWidth(col), asNumber(col.mobileWidth))
      let declared = Math.max(asNumber(col.width), asNumber(col.minWidth), floor)
      if (isOperation(col)) {
        // 操作列按按钮内容留出宽度，不超过一排文字按钮所需。
        declared = declared > 0 && declared <= WIDTH_OP_MAX ? declared : WIDTH_OP
      }
      next.width = declared
      next.minWidth = declared
      next.fixed = isOperation(col) ? 'right' : col.fixed === 'left' ? 'left' : undefined
      // 单元格自己做单行省略，不再让表格组件再包一层提示。
      next.showOverflowTooltip = false
      return next
    })
}

/** 后台表格在手机上的列宽。列都保留，总宽可以超出屏幕，由表格自己横向滚动。 */
export function layoutMobileColumns(cols: ColumnOption[]) {
  return mobileScrollLayout(cols)
}
