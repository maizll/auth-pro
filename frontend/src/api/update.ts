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

/** 官网读取发布仓库的情况，只在官网返回。source 是用上的令牌说明，例如「存储「X」的令牌」。 */
export interface OfficialUpdateSource {
  repository: string
  lastCheck?: {
    ok: boolean
    source?: string
    reason?: string
    at: string
  }
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
  /** 自动更新开关：开了以后强制更新会在 autoWindow 时段自动安装 */
  autoUpdate: boolean
  autoWindow: string
}

/** 强制更新提醒：后台横幅和登录后弹框用 */
export interface OnlineUpdateNotice {
  force: boolean
  version?: string
  releasedAt?: string
  notes?: string[]
  autoUpdate: boolean
  autoWindow: string
  /** 这个版本自动安装失败过，不再自动重试 */
  autoFailed?: boolean
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

export function fetchOnlineUpdateNotice() {
  return request.get<OnlineUpdateNotice>({
    url: '/api/system/update/notice',
    showErrorMessage: false
  })
}

export function saveOnlineUpdateAuto(enabled: boolean) {
  return request.put<OnlineUpdateNotice>({
    url: '/api/system/update/auto',
    data: { enabled }
  })
}

/** 上传更新包后服务器从包里认出的信息，只读展示给用户确认。 */
export interface OnlineUpdateUpload {
  uploadId: string
  currentVersion: string
  version: string
  edition: 'official' | 'client'
  editionLabel: string
  releasedAt: string
  notes: string[]
  size: number
  sha256: string
}

/** 上传 Release 里的整包原文件，服务器验签并识别版本，不安装。 */
export function uploadOnlineUpdatePackage(file: File, onProgress?: (percent: number) => void) {
  const data = new FormData()
  data.append('file', file)
  return request.post<OnlineUpdateUpload>({
    url: '/api/system/update/upload',
    data,
    timeout: 30 * 60 * 1000,
    showErrorMessage: false,
    onUploadProgress: (event) => {
      if (onProgress && event.total) onProgress(Math.round((event.loaded * 100) / event.total))
    }
  })
}

/** 确认安装刚才上传的包，返回和在线更新同样的任务。 */
export function applyUploadedOnlineUpdate(uploadId: string) {
  return request.post<OnlineUpdateJob>({
    url: '/api/system/update/upload/apply',
    data: { uploadId },
    showErrorMessage: false
  })
}
