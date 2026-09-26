import { UPDATE_RESTART_TIMEOUT_MS } from './restart-timeout'

/** 从点击更新到必须停下来的上限，避免下载或重启阶段无限转圈。 */
export const UPDATE_OVERALL_TIMEOUT_MS = 10 * 60 * 1000

export const UPDATE_RESTART_TIMEOUT_REASON =
  '等待新版本启动超时。服务重启超过 3 分钟仍没有返回目标版本。请刷新页面查看版本号；若仍是旧版本，请查看进程守护日志。'

export const UPDATE_OVERALL_TIMEOUT_REASON =
  '更新等待超时，仍未看到目标版本。请刷新页面确认当前版本。'

export const UPDATE_ROLLBACK_FALLBACK = '更新失败，已回滚到更新前的版本。'

export interface UpdateVersionHint {
  jobId?: string
  targetVersion?: string
  status?: string
  rolledBack?: boolean
  reason?: string
}

export interface UpdateVersionPayload {
  version?: string
  update?: UpdateVersionHint
}

export type UpdatePollSample =
  | { reachable: false }
  | { reachable: true; authFailure?: boolean; payload: UpdateVersionPayload }

export interface UpdatePollClock {
  startedAt: number
  restartingSince: number
}

export type UpdatePollDecision =
  | { action: 'wait'; restarting: boolean; restartingSince: number }
  | { action: 'reload'; restartingSince: number }
  | { action: 'rollback'; reason: string; restartingSince: number }
  | { action: 'timeout'; reason: string; restartingSince: number }
  | { action: 'auth'; restartingSince: number }

const waitKey = (jobId: string) => `auth-pro-update-wait:${jobId}`
const restartKey = (jobId: string) => `auth-pro-update-restarting:${jobId}`
const reloadKey = (jobId: string) => `auth-pro-update-reloaded:${jobId}`

export function normalizeUpdateVersion(value?: string): string {
  return (value || '').trim().replace(/^v/i, '')
}

export function isUnreachableUpdateResponse(
  status: number | undefined,
  contentType: string | undefined,
  bodyText: string
): boolean {
  if (status == null || status === 0) return true
  if (status === 502 || status === 503 || status === 504) return true
  const type = (contentType || '').toLowerCase()
  if (type.includes('text/html')) return true
  const trimmed = bodyText.trimStart()
  if (trimmed.startsWith('<')) return true
  return false
}

/** 把一次版本接口的原始响应收成轮询样本。网络错误、502/504 和 nginx HTML 都是重启中。 */
export function sampleFromVersionHTTP(
  status: number | undefined,
  contentType: string | undefined,
  bodyText: string,
  networkError = false
): UpdatePollSample {
  if (networkError || isUnreachableUpdateResponse(status, contentType, bodyText)) {
    return { reachable: false }
  }
  let parsed: { code?: number; data?: UpdateVersionPayload; version?: string; update?: UpdateVersionHint }
  try {
    parsed = JSON.parse(bodyText) as typeof parsed
  } catch {
    return { reachable: false }
  }
  if (status === 401 || status === 403 || parsed.code === 401 || parsed.code === 403) {
    return { reachable: true, authFailure: true, payload: {} }
  }
  const payload = parsed.data && typeof parsed.data === 'object' ? parsed.data : parsed
  if (typeof payload.version !== 'string' || payload.version.trim() === '') {
    return { reachable: false }
  }
  return { reachable: true, payload }
}

export function interpretUpdatePoll(
  sample: UpdatePollSample,
  targetVersion: string,
  clock: UpdatePollClock,
  now: number
): UpdatePollDecision {
  const target = normalizeUpdateVersion(targetVersion)
  if (!sample.reachable) {
    const since = clock.restartingSince > 0 ? clock.restartingSince : now
    if (now - since >= UPDATE_RESTART_TIMEOUT_MS) {
      return { action: 'timeout', reason: UPDATE_RESTART_TIMEOUT_REASON, restartingSince: since }
    }
    if (clock.startedAt > 0 && now - clock.startedAt >= UPDATE_OVERALL_TIMEOUT_MS) {
      return { action: 'timeout', reason: UPDATE_OVERALL_TIMEOUT_REASON, restartingSince: since }
    }
    return { action: 'wait', restarting: true, restartingSince: since }
  }
  if (sample.authFailure) {
    return { action: 'auth', restartingSince: clock.restartingSince }
  }
  const version = normalizeUpdateVersion(sample.payload.version)
  if (target !== '' && version === target) {
    return { action: 'reload', restartingSince: 0 }
  }
  const hint = sample.payload.update
  if (hint?.rolledBack && version !== target) {
    const reason = (hint.reason || '').trim() || UPDATE_ROLLBACK_FALLBACK
    return { action: 'rollback', reason, restartingSince: 0 }
  }
  if (clock.startedAt > 0 && now - clock.startedAt >= UPDATE_OVERALL_TIMEOUT_MS) {
    return { action: 'timeout', reason: UPDATE_OVERALL_TIMEOUT_REASON, restartingSince: clock.restartingSince }
  }
  if (clock.restartingSince > 0 && now - clock.restartingSince >= UPDATE_RESTART_TIMEOUT_MS) {
    return {
      action: 'timeout',
      reason: '服务已经恢复，但仍是更新前的版本，且没有收到回滚说明。请刷新页面查看版本号。',
      restartingSince: clock.restartingSince
    }
  }
  return { action: 'wait', restarting: clock.restartingSince > 0, restartingSince: clock.restartingSince }
}

export function versionPollURL(now: number): string {
  return `/api/system/version?_=${now}`
}

export function rememberUpdateWaitStart(
  jobId: string,
  now: number,
  storage: Pick<Storage, 'getItem' | 'setItem'>
): number {
  const key = waitKey(jobId)
  const existing = Number(storage.getItem(key))
  if (Number.isFinite(existing) && existing > 0) return existing
  storage.setItem(key, String(now))
  return now
}

export function rememberRestartingSince(
  jobId: string,
  now: number,
  storage: Pick<Storage, 'getItem' | 'setItem'>
): number {
  const key = restartKey(jobId)
  const existing = Number(storage.getItem(key))
  if (Number.isFinite(existing) && existing > 0) return existing
  storage.setItem(key, String(now))
  return now
}

export function clearUpdateWait(
  jobId: string,
  storage: Pick<Storage, 'removeItem'>
): void {
  storage.removeItem(waitKey(jobId))
  storage.removeItem(restartKey(jobId))
  storage.removeItem(reloadKey(jobId))
}

export function markUpdateReloaded(jobId: string, storage: Pick<Storage, 'setItem'>): void {
  storage.setItem(reloadKey(jobId), '1')
}

export function updateAlreadyReloaded(jobId: string, storage: Pick<Storage, 'getItem'>): boolean {
  return storage.getItem(reloadKey(jobId)) === '1'
}
