import request from '@/utils/http'

export interface AppRepoItem {
  appId: number
  name: string
  appKey: string
  archived: boolean
  status: 'unbound' | 'ready' | 'degraded' | string
  repo: string
  health: string
}

export function fetchAppRepos() {
  return request.get<{ list: AppRepoItem[]; empty?: string; note?: string; tokenReady?: boolean }>({
    url: '/api/v1/source/admin/app-repos',
    showErrorMessage: true
  })
}

export function fetchAppRepoToken() {
  return request.get<{ ready: boolean; message: string; location?: string }>({
    url: '/api/v1/source/admin/app-repos/token'
  })
}

export function suggestAppRepo(name: string, appKey = '') {
  return request.post<{ repo: string; tokenReady: boolean }>({
    url: '/api/v1/source/admin/app-repos/suggest',
    data: { name, appKey },
    showSuccessMessage: false
  })
}

export function bindAppRepo(data: { appId: number; action: 'create' | 'bind'; repo: string }) {
  return request.post<{ repo: string }>({
    url: '/api/v1/source/admin/app-repos/bind',
    data,
    showSuccessMessage: true
  })
}

export function previewAppRepo(appId: number, repo: string) {
  return request.post<{
    from: string
    to: string
    empty?: string
    groups: { prefix: string; text: string }[]
  }>({
    url: '/api/v1/source/admin/app-repos/preview',
    data: { appId, repo },
    showSuccessMessage: false
  })
}

export function rebindAppRepo(data: { appId: number; repo: string; mode: 'switch' | 'copy' }) {
  return request.post({
    url: '/api/v1/source/admin/app-repos/rebind',
    data,
    showSuccessMessage: true
  })
}

export function fetchAppRepoImpact(appId: number) {
  return request.post<{ published: number; drafts: number; blocked?: boolean; busy?: boolean }>({
    url: '/api/v1/source/admin/app-repos/impact',
    data: { appId },
    showSuccessMessage: false,
    showErrorMessage: false
  })
}

export function unbindAppRepo(appId: number) {
  return request.post<{ published: number; drafts: number; blocked?: boolean }>({
    url: '/api/v1/source/admin/app-repos/unbind',
    data: { appId },
    showSuccessMessage: false,
    showErrorMessage: false
  })
}
