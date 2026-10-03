/**
 * 在线更新的全局会话：点「立即更新」后一直轮询到新版本启动完成。
 * 放在页面外面，切到别的页面、重启中刷新页面、同一浏览器的其他标签页都接着等，
 * 期间只显示「正在更新」卡片（OnlineUpdateBusyCard），请求层不弹 502/连不上之类的错误。
 * 重启中整页刷新时由 nginx 给出 public/backend-unavailable.html 的「正在更新」页，恢复后回到这里接着等。
 */
import { reactive } from 'vue'
import { ElMessage } from 'element-plus'
import { fetchOnlineUpdateJob, type OnlineUpdateJob } from '@/api/update'
import { HttpError } from '@/utils/http/error'
import {
  UPDATE_WINDOW_KEY,
  beginOnlineUpdateWindow,
  endOnlineUpdateWindow,
  onlineUpdateWindow
} from '@/utils/http/backend-unavailable'
import { clearRestartStart } from './restart-timeout'
import { stoppedBeforeInstall } from './status-label'
import {
  clearUpdateWait,
  interpretUpdatePoll,
  markUpdateReloaded,
  rememberRestartingSince,
  rememberUpdateWaitStart,
  sampleFromVersionHTTP,
  updateAlreadyReloaded,
  versionPollURL,
  type UpdatePollClock
} from './restart-watch'

const POLL_INTERVAL_MS = 2000
const DEFAULT_TIMEOUT_REASON =
  '在限定时间内没有确认新版本已经启动。若服务已经恢复，请刷新页面查看版本号。'

export { stoppedBeforeInstall }

export const updateSession = reactive({
  job: null as OnlineUpdateJob | null,
  /** 正在等更新完成（卡片显示中）。 */
  watching: false,
  /** 已经进入重启阶段。 */
  awaitingRestart: false,
  /** 新版本已就绪，马上刷新。 */
  finished: false,
  /** 点「立即更新」的时间（毫秒），卡片上的「已等待」从这里算。 */
  since: 0,
  restartTimedOut: false,
  restartFailureReason: DEFAULT_TIMEOUT_REASON
})

let timer: ReturnType<typeof setInterval> | undefined
let pollClock: UpdatePollClock = { startedAt: 0, restartingSince: 0 }
let reloadScheduled = false
let redirectScheduled = false
let listening = false

const tabStorage = () => window.sessionStorage

/** 开始（或接着）等某个更新任务。 */
export function watchUpdateJob(job: OnlineUpdateJob): void {
  stopPolling()
  updateSession.job = job
  updateSession.watching = true
  updateSession.finished = false
  updateSession.restartTimedOut = false
  updateSession.restartFailureReason = DEFAULT_TIMEOUT_REASON
  if (job.status === 'restarting') updateSession.awaitingRestart = true
  beginOnlineUpdateWindow(job.id, job.version)
  updateSession.since = onlineUpdateWindow()?.since || Date.now()
  pollClock = {
    startedAt: rememberUpdateWaitStart(job.id, Date.now(), tabStorage()),
    restartingSince: 0
  }
  const savedRestart = Number(tabStorage().getItem(`auth-pro-update-restarting:${job.id}`))
  if (Number.isFinite(savedRestart) && savedRestart > 0) pollClock.restartingSince = savedRestart
  void pollUpdate(job.id)
  timer = setInterval(() => void pollUpdate(job.id), POLL_INTERVAL_MS)
}

/** 页面打开时如果还在更新窗口里（重启中刷新了页面，或别的标签页点了更新），接着等。 */
function resumeUpdateWindow(): void {
  const current = onlineUpdateWindow()
  if (!current || (updateSession.watching && updateSession.job?.id === current.jobId)) return
  const now = new Date().toISOString()
  watchUpdateJob({
    id: current.jobId,
    version: current.version,
    status: 'restarting',
    message: '正在重启服务',
    progress: 95,
    logs: [],
    error: '',
    createdAt: now,
    updatedAt: now
  })
}

/** App 启动时调用一次：接着等未完成的更新，并跟随其他标签页开始的更新。 */
export function installUpdateSessionWatcher(): void {
  resumeUpdateWindow()
  if (listening || typeof window === 'undefined') return
  listening = true
  window.addEventListener('storage', (event) => {
    if (event.key === UPDATE_WINDOW_KEY && event.newValue) resumeUpdateWindow()
  })
}

async function pollUpdate(id: string): Promise<void> {
  if (!updateSession.watching || updateSession.job?.id !== id) return
  const now = Date.now()
  let sample = sampleFromVersionHTTP(undefined, undefined, '', true)
  try {
    const response = await fetch(versionPollURL(now), {
      cache: 'no-store',
      headers: { Accept: 'application/json', 'Cache-Control': 'no-cache', Pragma: 'no-cache' }
    })
    sample = sampleFromVersionHTTP(
      response.status,
      response.headers.get('content-type') || '',
      await response.text()
    )
  } catch {
    sample = sampleFromVersionHTTP(undefined, undefined, '', true)
  }
  if (!updateSession.watching) return
  const decision = interpretUpdatePoll(sample, updateSession.job?.version || '', pollClock, now)
  pollClock.restartingSince = decision.restartingSince
  if (decision.restartingSince > 0) {
    rememberRestartingSince(id, decision.restartingSince, tabStorage())
    updateSession.awaitingRestart = true
  }
  if (decision.action === 'reload') return finishUpdate()
  if (decision.action === 'rollback') return failUpdate(decision.reason)
  if (decision.action === 'timeout') return timeoutUpdate(decision.reason)
  if (decision.action === 'auth') return sessionExpired()
  if (decision.action === 'wait') updateSession.awaitingRestart = decision.restarting

  // 再看任务进度；重启中这个请求失败是正常的，请求层在更新窗口里不会提示。
  try {
    const nextJob = await fetchOnlineUpdateJob(id)
    if (!updateSession.watching) return
    updateSession.job = nextJob
    if (nextJob.status === 'restarting') updateSession.awaitingRestart = true
    if (nextJob.status === 'success') return finishUpdate()
    if (nextJob.status === 'failed') {
      failUpdate(nextJob.error || nextJob.message || '更新失败，已回滚到更新前的版本。')
    }
  } catch (error) {
    if (error instanceof HttpError && (error.code === 401 || error.code === 403)) {
      sessionExpired()
      return
    }
    updateSession.awaitingRestart = true
  }
}

function finishUpdate(): void {
  const message = '更新成功，正在刷新'
  const id = updateSession.job?.id
  if (updateSession.job) {
    updateSession.job = { ...updateSession.job, status: 'success', progress: 100, message }
  }
  stopPolling()
  endOnlineUpdateWindow()
  updateSession.awaitingRestart = false
  // 刷新过一次后（新版本页面）只显示完成，不再循环刷新。
  if (id && updateAlreadyReloaded(id, tabStorage())) {
    clearUpdateWait(id, tabStorage())
    updateSession.watching = false
    return
  }
  if (reloadScheduled) return
  reloadScheduled = true
  updateSession.finished = true
  if (id) markUpdateReloaded(id, tabStorage())
  window.setTimeout(() => window.location.reload(), 1200)
}

function failUpdate(reason: string): void {
  if (updateSession.job) {
    updateSession.job = {
      ...updateSession.job,
      status: 'failed',
      error: reason,
      message: stoppedBeforeInstall(updateSession.job.progress, reason)
        ? '更新没有执行，网站没有改动'
        : '更新失败，已回滚到更新前的版本'
    }
  }
  updateSession.restartTimedOut = false
  endWait()
  ElMessage.error(reason)
}

function timeoutUpdate(reason: string): void {
  updateSession.restartTimedOut = true
  updateSession.restartFailureReason = reason
  endWait()
  ElMessage.error(reason)
}

function sessionExpired(): void {
  endWait()
  ElMessage.error('登录已失效，请重新登录')
  if (redirectScheduled) return
  redirectScheduled = true
  window.setTimeout(() => window.location.replace('/admin'), 1000)
}

/** 失败、超时或登录失效：停止等待并结束更新窗口，之后的错误照常提示。 */
function endWait(): void {
  const id = updateSession.job?.id
  if (id) {
    clearRestartStart(id, tabStorage())
    clearUpdateWait(id, tabStorage())
  }
  pollClock = { startedAt: 0, restartingSince: 0 }
  updateSession.awaitingRestart = false
  updateSession.watching = false
  stopPolling()
  endOnlineUpdateWindow()
}

function stopPolling(): void {
  if (timer) clearInterval(timer)
  timer = undefined
}
