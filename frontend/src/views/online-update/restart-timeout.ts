/** 重启阶段轮询上限。超时后页面必须离开「一直 95%」。 */
export const UPDATE_RESTART_TIMEOUT_MS = 3 * 60 * 1000

export const UPDATE_RESTART_RECOVERY =
  '进程守护会继续按 backend/start.sh 拉起本站点，运行目录为 backend/。请先看上面的失败原因。不要再手动 nohup 一份脱离守护的 backend/auth_pro。若守护状态变成 FATAL，在宝塔进程守护里重新启动该项。'

const clockKey = (jobId: string) => `auth-pro-update-restart:${jobId}`

export function restartTimedOut(startedAt: number, now: number, status: string): boolean {
  return status === 'restarting' && startedAt > 0 && now - startedAt >= UPDATE_RESTART_TIMEOUT_MS
}

export function rememberRestartStart(jobId: string, now: number, storage: Pick<Storage, 'getItem' | 'setItem'>): number {
  const key = clockKey(jobId)
  const existing = Number(storage.getItem(key))
  if (Number.isFinite(existing) && existing > 0) return existing
  storage.setItem(key, String(now))
  return now
}

export function clearRestartStart(jobId: string, storage: Pick<Storage, 'removeItem'>): void {
  storage.removeItem(clockKey(jobId))
}
