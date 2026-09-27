import type { ColumnOption } from '@/types'
import { assignMobileWidths, pickMobileColumns } from '@/utils/mobile-table'

const MOBILE_QUERY = '(max-width: 767px)'

type ColumnBackup = {
  width?: ColumnOption['width']
  minWidth?: ColumnOption['minWidth']
  fixed?: ColumnOption['fixed']
  className?: string
}

type TableColumn = ColumnOption & {
  id?: string
  property?: string
  realWidth?: number
  className?: string
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
    type: column.type
  }))
}

function remember(column: TableColumn) {
  if (column.__mobileBackup) return
  column.__mobileBackup = {
    width: column.width,
    minWidth: column.minWidth,
    fixed: column.fixed,
    className: column.className
  }
}

function restoreColumn(column: TableColumn) {
  const saved = column.__mobileBackup
  if (!saved) return
  column.width = saved.width
  column.minWidth = saved.minWidth
  column.fixed = saved.fixed
  column.className = saved.className
  delete column.__mobileBackup
}

function applyWidths(root: HTMLElement, store: TableStore) {
  const columns = store.states._columns?.value || []
  if (!columns.length) return
  const wrap = root.querySelector('.el-scrollbar__wrap, .el-table__body-wrapper')
  const clientWidth = wrap instanceof HTMLElement ? wrap.clientWidth : root.clientWidth
  if (clientWidth < 40) return
  const budget = Math.max(220, clientWidth - 1)
  const options = asOptions(columns)
  const picked = new Set(pickMobileColumns(options))
  const kept = columns.filter((_, index) => picked.has(options[index]))
  const widths = assignMobileWidths(
    kept.map((column) => ({
      prop: column.property || column.prop,
      label: column.label,
      type: column.type
    })),
    budget
  )
  const signature = `${clientWidth}|${kept.map((column, index) => `${column.id}:${widths[index]}`).join(',')}`
  if (root.dataset.mobileFit === signature) return
  let keptIndex = 0
  columns.forEach((column, index) => {
    remember(column)
    if (!picked.has(options[index])) {
      column.width = 0.01
      column.minWidth = 0.01
      column.fixed = false
      if (!column.className?.includes('mobile-col-hidden')) {
        column.className = `${column.className || ''} mobile-col-hidden`.trim()
      }
      return
    }
    const width = widths[keptIndex] || 48
    keptIndex += 1
    column.width = width
    column.minWidth = width
    column.fixed = false
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

function actionLabel(el: HTMLElement) {
  const text = (el.innerText || '').replace(/\s+/g, '')
  if (text && text !== '更多') return text
  const html = el.innerHTML
  if (html.includes('delete')) return '删除'
  if (html.includes('pencil')) return '编辑'
  if (html.includes('eye')) return '查看'
  if (html.includes('login')) return '登录'
  return '操作'
}

function isDirectAction(el: HTMLElement) {
  if (el.classList.contains('el-dropdown') || el.classList.contains('mobile-op-more')) return false
  const text = (el.innerText || '').trim()
  return (
    el.classList.contains('el-button') ||
    el.classList.contains('c-p') ||
    Boolean(text && el.querySelector('svg'))
  )
}

let menu: HTMLDivElement | null = null

function closeMenu() {
  menu?.remove()
  menu = null
}

function openMenu(anchor: HTMLElement, actions: HTMLElement[]) {
  closeMenu()
  const panel = document.createElement('div')
  panel.className = 'mobile-op-menu'
  actions.forEach((action) => {
    const item = document.createElement('button')
    item.type = 'button'
    item.className = 'mobile-op-menu__item'
    if (action.classList.contains('el-button--danger') || action.innerHTML.includes('delete')) {
      item.classList.add('is-danger')
    }
    item.textContent = actionLabel(action)
    item.addEventListener('click', (event) => {
      event.stopPropagation()
      closeMenu()
      action.click()
    })
    panel.appendChild(item)
  })
  document.body.appendChild(panel)
  const rect = anchor.getBoundingClientRect()
  const width = panel.offsetWidth
  panel.style.top = `${Math.min(window.innerHeight - panel.offsetHeight - 8, rect.bottom + 4)}px`
  panel.style.left = `${Math.max(8, Math.min(rect.right - width, window.innerWidth - width - 8))}px`
  menu = panel
}

function collapseOperation(root: HTMLElement) {
  const headerRow = root.querySelector('.el-table__header-wrapper thead tr')
  if (!headerRow) return
  const headers = Array.from(headerRow.children) as HTMLElement[]
  const opIndex = headers.findIndex((cell) => (cell.innerText || '').replace(/\s+/g, '') === '操作')
  if (opIndex < 0) return
  root.querySelectorAll('.el-table__body-wrapper tbody tr').forEach((row) => {
    const cell = row.children[opIndex]?.querySelector('.cell') as HTMLElement | null
    if (!cell) return
    const host = (cell.querySelector('.row-actions') as HTMLElement | null) || cell
    const actions = Array.from(host.children).filter(
      (node): node is HTMLElement => node instanceof HTMLElement && isDirectAction(node)
    )
    if (actions.length <= 1) return
    const hidden = actions.slice(1)
    hidden.forEach((action) => action.classList.add('mobile-op-hidden'))
    if (host.querySelector('.mobile-op-more')) return
    const more = document.createElement('button')
    more.type = 'button'
    more.className = 'el-button el-button--primary is-link mobile-op-more'
    more.textContent = '更多'
    more.addEventListener('click', (event) => {
      event.stopPropagation()
      openMenu(more, hidden)
    })
    host.appendChild(more)
  })
}

function fitOne(root: HTMLElement, mobile: boolean) {
  if (root.closest('.el-dialog, .el-drawer')) return
  const store = tableStore(root)
  if (!store) return
  if (!mobile) {
    restoreTable(root, store)
    return
  }
  applyWidths(root, store)
  collapseOperation(root)
}

/** 手机上所有后台、用户端、代理端列表共用：收列、取消右侧固定、操作收进更多。 */
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
    if (!menu) return
    const target = event.target
    if (!(target instanceof Node) || menu.contains(target)) return
    if (target instanceof Element && target.closest('.mobile-op-more')) return
    closeMenu()
  })
  const observer = new MutationObserver(schedule)
  observer.observe(document.body, { childList: true, subtree: true })
  schedule()
}
