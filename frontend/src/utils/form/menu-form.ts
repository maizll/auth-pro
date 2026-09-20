import { resolveMenuTitle } from './menu-title'

export interface ManageMenuRow {
  id?: number
  parentId?: number
  parent_id?: number
  name?: string
  path?: string
  component?: string
  redirect?: string
  title?: string
  icon?: string
  sort?: number
  isHide?: boolean
  isHideTab?: boolean
  isFullPage?: boolean
  keepAlive?: boolean
  fixedTab?: boolean
  enabled?: boolean
  meta?: {
    title?: string
    icon?: string
    sort?: number
    isHide?: boolean
    isHideTab?: boolean
    isFullPage?: boolean
    keepAlive?: boolean
    fixedTab?: boolean
    isEnable?: boolean
    roles?: string[]
  }
}

export interface MenuEditForm {
  id: number
  parentId: number
  name: string
  path: string
  label: string
  component: string
  icon: string
  sort: number
  keepAlive: boolean
  isHide: boolean
  isHideTab: boolean
  isEnable: boolean
  fixedTab: boolean
  isFullPage: boolean
  roles: string[]
}

export function mapManageRowToForm(row: ManageMenuRow | null | undefined): MenuEditForm | null {
  if (!row) return null
  const title = resolveMenuTitle(row.title || row.meta?.title, '')
  return {
    id: Number(row.id || 0),
    parentId: Number(row.parentId ?? row.parent_id ?? 0) || 0,
    name: title || String(row.name || ''),
    path: row.path || '',
    label: row.name || '',
    component: row.component || '',
    icon: row.icon || row.meta?.icon || '',
    sort: row.sort ?? row.meta?.sort ?? 1,
    keepAlive: row.keepAlive ?? row.meta?.keepAlive ?? false,
    isHide: row.isHide ?? row.meta?.isHide ?? false,
    isHideTab: row.isHideTab ?? row.meta?.isHideTab ?? false,
    isEnable: row.enabled ?? row.meta?.isEnable ?? true,
    fixedTab: row.fixedTab ?? row.meta?.fixedTab ?? false,
    isFullPage: row.isFullPage ?? row.meta?.isFullPage ?? false,
    roles: row.meta?.roles || []
  }
}

export function toMenuSavePayload(data: {
  parentId?: number | null
  label?: string
  path?: string
  component?: string
  redirect?: string
  name?: string
  icon?: string
  sort?: number
  isHide?: boolean
  isHideTab?: boolean
  isFullPage?: boolean
  keepAlive?: boolean
  fixedTab?: boolean
  isEnable?: boolean
}) {
  const title = String(data.name || '').trim()
  return {
    parentId: Number(data.parentId ?? 0) || 0,
    name: data.label,
    path: data.path,
    component: data.component,
    redirect: data.redirect || '',
    title,
    icon: data.icon,
    sort: data.sort ?? 1,
    isHide: !!data.isHide,
    isHideTab: !!data.isHideTab,
    isFullPage: !!data.isFullPage,
    keepAlive: !!data.keepAlive,
    fixedTab: !!data.fixedTab,
    enabled: data.isEnable !== false
  }
}
