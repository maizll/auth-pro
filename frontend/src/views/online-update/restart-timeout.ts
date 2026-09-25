/** 重启阶段轮询上限。超时后页面必须离开「一直 95%」。 */
export const UPDATE_RESTART_TIMEOUT_MS = 3 * 60 * 1000

export const UPDATE_RESTART_RECOVERY =
  '在宝塔「进程守护」里先停止本站点，再执行 ss -lptn | grep 19127 查看占用端口的进程。结束的应是本站 backend/auth_pro。确认 backend/auth_pro 后，只由进程守护启动 backend/start.sh，运行目录为 backend/。不要再手动 nohup 一个脱离守护的进程。'

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
