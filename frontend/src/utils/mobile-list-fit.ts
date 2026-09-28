import type { ColumnOption } from '@/types'
import { mobileScrollLayout } from '@/utils/mobile-table'

const MOBILE_QUERY = '(max-width: 767px)'

type ColumnBackup = {
  width?: ColumnOption['width']
  minWidth?: ColumnOption['minWidth']
  fixed?: ColumnOption['fixed']
  className?: string
  showOverflowTooltip?: boolean
}

type TableColumn = ColumnOption & {
  id?: string
  property?: string
  realWidth?: number
  className?: string
  showOverflowTooltip?: boolean
  __mobileBackup?: ColumnBackup
}

type TableStore = {
  states: { _columns: { value: TableColumn[] } }
  scheduleLayout: (needUpdateColumns?: boolean, immediate?: boolean) => void
}

function tableStore(root: HTMLElement): TableStore | null {
  const inst = (root as HTMLElement & { __vueParentComponent?: { proxy?: { store?: TableStore } } })
    .__vueParentComponent
  return inst?.proxy?.store || null
}

function asOptions(columns: TableColumn[]): ColumnOption[] {
  return columns.map((column) => ({
    prop: column.property || column.prop,
    label: column.label,
    type: column.type,
    width: column.width,
    minWidth: column.minWidth,
    fixed: column.fixed,
    visible: column.visible,
    checked: column.checked,
    mobileWidth: column.mobileWidth,
    mobileLabel: column.mobileLabel
  }))
}

function remember(column: TableColumn) {
  if (column.__mobileBackup) return
  column.__mobileBackup = {
    width: column.width,
    minWidth: column.minWidth,
    fixed: column.fixed,
    className: column.className,
    showOverflowTooltip: column.showOverflowTooltip
  }
}

function restoreColumn(column: TableColumn) {
  const saved = column.__mobileBackup
  if (!saved) return
  column.width = saved.width
  column.minWidth = saved.minWidth
  column.fixed = saved.fixed
  column.className = saved.className
  column.showOverflowTooltip = saved.showOverflowTooltip
  delete column.__mobileBackup
}

function applyWidths(root: HTMLElement, store: TableStore) {
  const columns = store.states._columns?.value || []
  if (!columns.length) return
  const laid = mobileScrollLayout(asOptions(columns))
  const opWidth = Number(root.dataset.opWidth || 0)
  const applied = (column: { prop?: string; label?: string; width?: string | number }) => {
    const operation = (column.label === '操作' || column.prop === 'operation') && opWidth > 0
    return operation ? opWidth : column.width
  }
  const signature = laid
    .map((column) => `${column.prop || column.label}:${applied(column)}:${column.fixed || ''}`)
    .join(',')
  const current = columns
    .map(
      (column) =>
        `${column.property || column.prop || column.label}:${column.width}:${column.fixed || ''}`
    )
    .join(',')
  if (root.dataset.mobileFit === signature && current === signature) return
  columns.forEach((column, index) => {
    remember(column)
    const next = laid[index]
    if (!next) return
    const operation = (next.label === '操作' || next.prop === 'operation') && opWidth > 0
    column.width = operation ? opWidth : next.width
    column.minWidth = operation ? opWidth : next.minWidth
    column.fixed = next.fixed
    if (operation) (column as TableColumn & { align?: string }).align = 'left'
    column.showOverflowTooltip = false
    column.className = (column.className || '').replace(/\bmobile-col-hidden\b/g, '').trim()
  })
  root.dataset.mobileFit = signature
  try {
    store.scheduleLayout(true, true)
  } catch {
    delete root.dataset.mobileFit
  }
}

function restoreTable(root: HTMLElement, store: TableStore) {
  const columns = store.states._columns?.value || []
  if (!columns.some((column) => column.__mobileBackup)) return
  columns.forEach(restoreColumn)
  delete root.dataset.mobileFit
  try {
    store.scheduleLayout(true, true)
  } catch {
    // 表格正在卸载时不再排版。
  }
}

const NARROW_QUERY = '(max-width: 767px)'

function plainText(value: string) {
  return value.replace(/\s+/g, ' ').trim()
}

function cellBox(cell: HTMLElement) {
  return cell.querySelector('.cell') as HTMLElement | null
}

function cellText(cell: HTMLElement) {
  const box = cellBox(cell)
  return plainText(box?.innerText || '')
}

function isTruncated(cell: HTMLElement) {
  const box = cellBox(cell)
  if (!box) return false
  if (box.scrollWidth - box.clientWidth > 1 || box.scrollHeight - box.clientHeight > 1) return true
  return Array.from(box.querySelectorAll('*')).some(
    (node) => node instanceof HTMLElement && node.scrollWidth - node.clientWidth > 1
  )
}

function isInteractive(target: EventTarget | null) {
  if (!(target instanceof Element)) return false
  return Boolean(
    target.closest(
      'button, a, input, textarea, select, .el-button, .el-dropdown, .el-checkbox, .el-switch, .el-select, .table-row-detail'
    )
  )
}

function isOperationHeader(text: string) {
  return plainText(text) === '操作'
}

function headerText(cell: HTMLElement) {
  const row = cell.parentElement
  const table = cell.closest('.el-table')
  if (!row || !table) return ''
  const index = Array.from(row.children).indexOf(cell)
  const header = table.querySelectorAll('.el-table__header-wrapper thead th')[index]
  return header instanceof HTMLElement ? plainText(header.innerText || '') : ''
}

let preview: HTMLDivElement | null = null
let previewTimer = 0
let detail: HTMLDivElement | null = null

function closePreview() {
  window.clearTimeout(previewTimer)
  preview?.remove()
  preview = null
}

async function copyText(text: string, button: HTMLButtonElement) {
  const previous = button.textContent
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    const area = document.createElement('textarea')
    area.value = text
    area.style.position = 'fixed'
    area.style.left = '-999px'
    document.body.appendChild(area)
    area.select()
    document.execCommand('copy')
    area.remove()
  }
  button.textContent = '已复制'
  window.setTimeout(() => {
    button.textContent = previous
  }, 1200)
}

function openPreview(cell: HTMLElement) {
  const text = cellText(cell)
  if (!text) return
  closePreview()
  const panel = document.createElement('div')
  panel.className = 'table-cell-preview'
  const body = document.createElement('div')
  body.className = 'table-cell-preview__text'
  body.textContent = text
  const button = document.createElement('button')
  button.type = 'button'
  button.className = 'table-cell-preview__copy'
  button.textContent = '复制'
  button.addEventListener('click', (event) => {
    event.stopPropagation()
    void copyText(text, button)
  })
  panel.append(body, button)
  panel.addEventListener('mouseenter', () => window.clearTimeout(previewTimer))
  document.body.appendChild(panel)
  const rect = cell.getBoundingClientRect()
  const width = Math.min(320, window.innerWidth - 16)
  panel.style.width = `${width}px`
  const left = Math.max(8, Math.min(rect.left, window.innerWidth - width - 8))
  panel.style.left = `${left}px`
  const top = rect.bottom + 6
  if (top + panel.offsetHeight > window.innerHeight - 8) {
    panel.style.top = `${Math.max(8, rect.top - panel.offsetHeight - 6)}px`
  } else {
    panel.style.top = `${top}px`
  }
  preview = panel
}

function closeDetail() {
  detail?.remove()
  detail = null
}

function rowFields(row: HTMLTableRowElement) {
  const table = row.closest('.el-table')
  const headers = Array.from(table?.querySelectorAll('.el-table__header-wrapper thead th') || [])
  return Array.from(row.children).flatMap((cell, index) => {
    if (!(cell instanceof HTMLElement)) return []
    const label = plainText((headers[index] as HTMLElement | undefined)?.innerText || '')
    if (!label || isOperationHeader(label)) return []
    const value = cellText(cell)
    if (!value) return []
    return [{ label, value }]
  })
}

function openDetail(row: HTMLTableRowElement) {
  const fields = rowFields(row)
  if (!fields.length) return
  closePreview()
  closeDetail()
  const mask = document.createElement('div')
  mask.className = 'table-row-preview'
  const panel = document.createElement('div')
  panel.className = 'table-row-preview__panel'
  panel.setAttribute('role', 'dialog')
  panel.setAttribute('aria-label', '详情')
  const head = document.createElement('div')
  head.className = 'table-row-preview__head'
  const title = document.createElement('strong')
  title.textContent = '详情'
  const close = document.createElement('button')
  close.type = 'button'
  close.className = 'table-row-preview__close'
  close.textContent = '关闭'
  close.addEventListener('click', (event) => {
    event.stopPropagation()
    closeDetail()
  })
  head.append(title, close)
  const list = document.createElement('dl')
  list.className = 'table-row-preview__list'
  fields.forEach((field) => {
    const item = document.createElement('div')
    item.className = 'table-row-preview__item'
    const label = document.createElement('dt')
    label.textContent = field.label
    const value = document.createElement('dd')
    const text = document.createElement('span')
    text.textContent = field.value
    const button = document.createElement('button')
    button.type = 'button'
    button.className = 'table-cell-preview__copy'
    button.textContent = '复制'
    button.addEventListener('click', (event) => {
      event.stopPropagation()
      void copyText(field.value, button)
    })
    value.append(text, button)
    item.append(label, value)
    list.appendChild(item)
  })
  panel.append(head, list)
  mask.appendChild(panel)
  mask.addEventListener('click', (event) => {
    if (event.target === mask) closeDetail()
  })
  document.body.appendChild(mask)
  detail = mask
}

function ensureDetailButton(root: HTMLElement) {
  const headers = Array.from(root.querySelectorAll('.el-table__header-wrapper thead th'))
  const opIndex = headers.findIndex((cell) =>
    isOperationHeader((cell as HTMLElement).innerText || '')
  )
  if (opIndex < 0) return
  root.querySelectorAll('.el-table__body-wrapper tbody tr').forEach((row) => {
    if (!(row instanceof HTMLTableRowElement)) return
    const cell = row.children[opIndex]?.querySelector('.cell')
    if (!(cell instanceof HTMLElement) || cell.querySelector('.table-row-detail')) return
    const button = document.createElement('button')
    button.type = 'button'
    button.className = 'el-button el-button--primary is-link table-row-detail'
    button.textContent = '详情'
    button.addEventListener('click', (event) => {
      event.stopPropagation()
      openDetail(row)
    })
    cell.prepend(button)
  })
}

function bindPreview(root: HTMLElement) {
  if (root.dataset.tablePreview === '1') return
  root.dataset.tablePreview = '1'
  root.addEventListener('click', (event) => {
    const target = event.target
    if (!(target instanceof Element) || target.closest('.table-cell-preview, .table-row-preview'))
      return
    const row = target.closest('tbody tr')
    if (!(row instanceof HTMLTableRowElement) || !root.contains(row)) return
    if (target.closest('.table-row-detail')) {
      openDetail(row)
      return
    }
    if (isInteractive(target)) return
    const cell = target.closest('td')
    const header = cell instanceof HTMLElement ? headerText(cell) : ''
    if (
      window.matchMedia(NARROW_QUERY).matches &&
      cell instanceof HTMLElement &&
      header &&
      !isOperationHeader(header) &&
      isTruncated(cell)
    ) {
      event.stopPropagation()
      openPreview(cell)
      return
    }
    openDetail(row)
  })
  root.addEventListener('mouseover', (event) => {
    if (window.matchMedia(NARROW_QUERY).matches) return
    const target = event.target
    if (!(target instanceof Element)) return
    const cell = target.closest('td')
    if (!(cell instanceof HTMLElement) || !root.contains(cell) || !isTruncated(cell)) return
    window.clearTimeout(previewTimer)
    openPreview(cell)
  })
  root.addEventListener('mouseout', (event) => {
    const next = event.relatedTarget
    if (next instanceof Node && preview?.contains(next)) return
    previewTimer = window.setTimeout(closePreview, 180)
  })
}

function silenceTooltip(store: TableStore) {
  const columns = store.states._columns?.value || []
  columns.forEach((column) => {
    if (column.showOverflowTooltip) column.showOverflowTooltip = false
  })
}

function fitOne(root: HTMLElement, mobile: boolean) {
  if (root.closest('.el-dialog, .el-drawer')) return
  bindPreview(root)
  ensureDetailButton(root)
  const store = tableStore(root)
  if (!store) return
  silenceTooltip(store)
  if (!mobile) {
    restoreTable(root, store)
    silenceTooltip(store)
    return
  }
  applyWidths(root, store)
  fitOperationWidth(root, store)
}

function fitOperationWidth(root: HTMLElement, store: TableStore) {
  const measured = measureOperationWidth(root)
  if (!measured) return
  if (Math.abs(measured - Number(root.dataset.opWidth || 0)) <= 4) return
  root.dataset.opWidth = String(measured)
  delete root.dataset.mobileFit
  applyWidths(root, store)
}

function measureOperationWidth(root: HTMLElement) {
  const headers = Array.from(root.querySelectorAll('.el-table__header-wrapper thead th'))
  const opIndex = headers.findIndex((cell) =>
    isOperationHeader((cell as HTMLElement).innerText || '')
  )
  if (opIndex < 0) return 0
  let width = 0
  root.querySelectorAll('.el-table__body-wrapper tbody tr').forEach((row) => {
    const cell = row.children[opIndex]
    const box = cell instanceof HTMLElement ? cellBox(cell) : null
    if (!box) return
    width = Math.max(width, box.scrollWidth)
  })
  if (!width) return 0
  return Math.min(200, Math.max(72, width + 12))
}

/** 后台、用户端、代理端列表共用：手机横向滚动，操作列固定，截断内容可预览和复制。 */
export function installMobileListFit() {
  const query = window.matchMedia(MOBILE_QUERY)
  let scheduled = false
  const run = () => {
    scheduled = false
    document.querySelectorAll('.el-table').forEach((node) => {
      if (node instanceof HTMLElement) fitOne(node, query.matches)
    })
  }
  const schedule = () => {
    if (scheduled) return
    scheduled = true
    requestAnimationFrame(run)
  }
  query.addEventListener('change', schedule)
  window.addEventListener('resize', schedule)
  document.addEventListener('click', (event) => {
    const target = event.target
    if (!(target instanceof Node)) return
    if (preview && !preview.contains(target)) closePreview()
  })
  const observer = new MutationObserver(schedule)
  observer.observe(document.body, { childList: true, subtree: true })
  schedule()
}
