import request from '@/utils/http'

export interface OnlineUpdatePackage {
  os: string
  arch: string
  fileName: string
  url: string
  sha256: string
  size: number
  signature: string
}

export interface OnlineUpdateManifest {
  version: string
  channel: string
  minVersion: string
  force: boolean
  releasedAt: string
  releasesUrl: string
  package: OnlineUpdatePackage
  actions: {
    updateFrontend: boolean
    updateBackend: boolean
    restartBackend: boolean
    backupDatabase: boolean
  }
  notes: string[]
}

export interface OnlineUpdateRelease {
  version: string
  channel: string
  releasedAt: string
  notes: string[]
}

export interface OnlineUpdateHistory {
  currentVersion: string
  releasesUrl: string
  releases: OnlineUpdateRelease[]
}

export interface OnlineUpdateJob {
  id: string
  status: 'running' | 'restarting' | 'success' | 'failed'
  message: string
  progress: number
  version: string
  logs: string[]
  error: string
  createdAt: string
  updatedAt: string
}

/** 官网读取发布仓库用的凭据，只在官网返回。 */
export interface OfficialUpdateSource {
  repository: string
  tokenSaved: boolean
  tokenHint: string
  lastCheck?: {
    credential: 'official_token' | 'saved_token' | 'anonymous'
    ok: boolean
    status?: number
    at: string
  }
}

export interface OfficialUpdateSourceTest {
  tokenOk: boolean
  tokenMessage: string
  latestVersion?: string
  anonymousReadable: boolean
}

export interface OnlineUpdateStatus {
  currentVersion: string
  buildTime: string
  updateUrl: string
  frontendDir: string
  serviceName: string
  latest?: OnlineUpdateManifest | null
  runningJob?: OnlineUpdateJob | null
  officialSource?: OfficialUpdateSource | null
}

export interface OnlineUpdateCheckResult {
  currentVersion: string
  latest: OnlineUpdateManifest
  updateUrl: string
  updateAvailable: boolean
  canApply: boolean
  packageValid: boolean
  packageError: string
  versionError: string
  officialSource?: OfficialUpdateSource | null
}

export function fetchOnlineUpdateStatus() {
  return request.get<OnlineUpdateStatus>({
    url: '/api/system/update/status',
    showErrorMessage: false
  })
}

export function fetchOnlineUpdateHistory(refresh = false) {
  return request.get<OnlineUpdateHistory>({
    url: '/api/system/update/history',
    params: refresh ? { refresh: 1 } : undefined,
    showErrorMessage: false
  })
}

export function fetchOnlineUpdateCheck() {
  return request.post<OnlineUpdateCheckResult>({
    url: '/api/system/update/check',
    showErrorMessage: false
  })
}

export function fetchOnlineUpdateApply() {
  return request.post<OnlineUpdateJob>({
    url: '/api/system/update/apply',
    showErrorMessage: false
  })
}

export function fetchOnlineUpdateJob(id: string) {
  return request.get<OnlineUpdateJob>({
    url: `/api/system/update/jobs/${encodeURIComponent(id)}`,
    showErrorMessage: false
  })
}

export function saveOfficialUpdateToken(token: string) {
  return request.post<OfficialUpdateSource>({
    url: '/api/system/update/official-token',
    data: { token },
    showErrorMessage: false
  })
}

export function testOfficialUpdateSource() {
  return request.post<OfficialUpdateSourceTest>({
    url: '/api/system/update/official-token/test',
    showErrorMessage: false
  })
}
