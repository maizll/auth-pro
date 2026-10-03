<!-- 在线更新页面：检查新版本、查看说明，并在 Linux amd64 上一键更新。 -->
<template>
  <div class="online-update">
    <ElCard shadow="never" class="art-table-card">
      <div class="update-header">
        <div>
          <h2 class="update-title">在线更新</h2>
          <p class="update-subtitle">整包更新网站页面和后台服务</p>
        </div>
        <div class="update-actions">
          <ElButton
            :icon="Refresh"
            :loading="loading || historyLoading"
            circle
            @click="loadPage(true)"
          />
          <ElButton :icon="Search" :loading="checking" @click="handleCheck">检查更新</ElButton>
          <UploadUpdatePackage
            :official="!!officialSource"
            :disabled="isJobActive"
            @started="handleUploadStarted"
          />
          <ElButton
            type="primary"
            :icon="Download"
            :disabled="!canApply"
            :loading="applying"
            @click="handleApply"
          >
            立即更新
          </ElButton>
        </div>
      </div>

      <ElAlert
        v-if="packageError"
        :title="packageError"
        type="info"
        show-icon
        :closable="false"
        class="update-alert"
      />
      <ElAlert
        v-if="versionError"
        :title="versionError"
        type="error"
        show-icon
        :closable="false"
        class="update-alert"
      />
      <ElAlert
        v-if="restartTimedOut"
        title="更新重启失败"
        type="error"
        show-icon
        :closable="false"
        class="update-alert"
      >
        <p>{{ restartFailureReason }}</p>
        <p>{{ restartRecovery }}</p>
      </ElAlert>
      <ElAlert
        v-else-if="job?.status === 'failed'"
        :title="failedBeforeInstall ? '更新没有执行，网站没有改动' : '更新失败，已尝试回滚'"
        type="error"
        show-icon
        :closable="false"
        class="update-alert"
      >
        <p>{{ job.error || job.message || '新版本没有健康启动' }}</p>
        <p v-if="!failedBeforeInstall">{{ restartRecovery }}</p>
      </ElAlert>

      <div v-if="job" ref="jobSectionRef" class="update-section job-section">
        <div class="section-header">
          <strong>更新任务</strong>
          <ElTag :type="jobStatusTag(job.status)" effect="plain">
            {{ jobStatusText(job.status) }}
          </ElTag>
        </div>
        <div class="job-progress">
          <ElProgress :percentage="jobProgress" :status="jobProgressStatus" :stroke-width="10" />
          <div class="job-progress-meta">
            <span>{{ job.message }}</span>
            <span>目标版本 v{{ job.version }}</span>
          </div>
        </div>
        <ElScrollbar max-height="220px" class="job-logs">
          <div v-for="log in job.logs" :key="log" class="job-log">{{ log }}</div>
          <ElEmpty v-if="!job.logs.length" description="暂无日志" :image-size="60" />
        </ElScrollbar>
      </div>

      <div class="version-grid">
        <div class="version-item">
          <span class="version-label">当前版本</span>
          <div class="version-value">
            <strong>v{{ currentVersion }}</strong>
            <ElTag type="info" effect="plain">当前</ElTag>
          </div>
          <span class="version-meta">{{ status?.buildTime || '未记录构建时间' }}</span>
        </div>

        <div class="version-item latest" :class="{ available: updateAvailable }">
          <span class="version-label">最新版本</span>
          <div class="version-value">
            <strong>{{ latestVersion }}</strong>
            <ElTag :type="updateAvailable ? 'success' : 'info'" effect="plain">
              {{ latestStatusText }}
            </ElTag>
          </div>
          <span class="version-meta">{{ latestVersionMeta }}</span>
        </div>

        <div class="version-item">
          <span class="version-label">更新通道</span>
          <div class="version-value plain">
            <strong>{{ updateChannelLabel(latest?.channel) }}</strong>
          </div>
        </div>
      </div>

      <div class="update-section">
        <div class="section-header">
          <strong>更新内容</strong>
          <ElText v-if="latest?.force" type="danger" size="small">强制更新</ElText>
        </div>
        <div v-if="latest?.notes?.length" class="notes-list">
          <div v-for="note in latest.notes" :key="note" class="note-item">
            <ArtSvgIcon icon="ri:checkbox-circle-line" />
            <span>{{ note }}</span>
          </div>
        </div>
        <ElEmpty v-else description="暂无更新日志" :image-size="72" />
      </div>

      <ReleaseHistory
        :releases="historyReleases"
        :current-version="currentVersion"
        :latest-version="latest?.version || historyReleases[0]?.version"
        :loading="historyLoading"
        :error="historyError"
        @refresh="loadHistory(true)"
      />

      <OfficialUpdateSource
        v-if="officialSource"
        :source="officialSource"
        @changed="handleSourceChanged"
      />

      <div class="update-section package-section">
        <div class="section-header">
          <strong>更新包</strong>
          <ElTag :type="packageValid ? 'success' : 'info'" effect="plain">
            {{ packageValid ? '已就绪' : '待完善' }}
          </ElTag>
        </div>
        <div class="package-grid">
          <div class="package-item">
            <span>文件</span>
            <strong>{{ latest?.package.fileName || '未配置' }}</strong>
          </div>
          <div class="package-item">
            <span>大小</span>
            <strong>{{ formatBytes(latest?.package.size) }}</strong>
          </div>
        </div>
      </div>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
  import { appConfirm } from '@/utils/app-confirm'
  import { computed, nextTick, onMounted, ref, toRef } from 'vue'
  import { ElMessage } from 'element-plus'
  import { Download, Refresh, Search } from '@element-plus/icons-vue'
  import {
    fetchOnlineUpdateApply,
    fetchOnlineUpdateCheck,
    fetchOnlineUpdateHistory,
    fetchOnlineUpdateStatus,
    OfficialUpdateSource as OfficialUpdateSourceView,
    OnlineUpdateCheckResult,
    OnlineUpdateHistory,
    OnlineUpdateJob,
    OnlineUpdateStatus
  } from '@/api/update'
  import { HttpError } from '@/utils/http/error'
  import OfficialUpdateSource from './OfficialUpdateSource.vue'
  import ReleaseHistory from './ReleaseHistory.vue'
  import UploadUpdatePackage from './UploadUpdatePackage.vue'
  import { UPDATE_RESTART_RECOVERY } from './restart-timeout'
  import { latestVersionStatus, updateChannelLabel } from './status-label'
  import { stoppedBeforeInstall, updateSession, watchUpdateJob } from './update-session'

  defineOptions({ name: 'OnlineUpdate' })

  const loading = ref(false)
  const historyLoading = ref(false)
  const checking = ref(false)
  const applying = ref(false)
  const status = ref<OnlineUpdateStatus | null>(null)
  const statusFailed = ref(false)
  const checkFailed = ref(false)
  const history = ref<OnlineUpdateHistory | null>(null)
  const historyError = ref('')
  const checkResult = ref<OnlineUpdateCheckResult | null>(null)
  // 任务状态和轮询放在全局会话里，离开本页也继续等，重启期间只显示「正在更新」卡片。
  const job = toRef(updateSession, 'job')
  const jobSectionRef = ref<HTMLElement | null>(null)
  const restartTimedOut = toRef(updateSession, 'restartTimedOut')
  const restartFailureReason = toRef(updateSession, 'restartFailureReason')
  const restartRecovery = UPDATE_RESTART_RECOVERY
  let redirectScheduled = false

  // 只有官网返回更新来源；检查更新后用最新的读取结果。
  const sourceOverride = ref<OfficialUpdateSourceView | null>(null)
  const officialSource = computed(
    () =>
      sourceOverride.value ||
      checkResult.value?.officialSource ||
      status.value?.officialSource ||
      null
  )
  const handleSourceChanged = (next: OfficialUpdateSourceView) => {
    sourceOverride.value = next
  }
  // 检查失败时接口不带更新来源，重新取一次，页面上能看到这次用的是哪种凭据、为什么失败。
  const refreshOfficialSource = async () => {
    try {
      const next = (await fetchOnlineUpdateStatus()).officialSource
      if (next) sourceOverride.value = next
    } catch {
      // 状态接口也连不上时保持原样，错误提示由检查更新给出。
    }
  }
  const latest = computed(() => checkResult.value?.latest || status.value?.latest || null)
  const currentVersion = computed(
    () => checkResult.value?.currentVersion || status.value?.currentVersion || '-'
  )
  const latestVersion = computed(() => (latest.value?.version ? `v${latest.value.version}` : '-'))
  const historyReleases = computed(() => history.value?.releases || [])
  const updateAvailable = computed(() => checkResult.value?.updateAvailable === true)
  const latestStatusText = computed(() =>
    latestVersionStatus({
      version: latest.value?.version,
      updateAvailable: updateAvailable.value,
      unreachable: statusFailed.value || checkFailed.value
    })
  )
  const latestVersionMeta = computed(() => {
    if (!latest.value?.version) return ''
    return formatDate(latest.value.releasedAt)
  })
  const packageError = computed(() => checkResult.value?.packageError || '')
  const versionError = computed(() => checkResult.value?.versionError || '')
  const packageValid = computed(() => checkResult.value?.packageValid === true)
  const isJobActive = computed(() =>
    job.value ? ['running', 'restarting'].includes(job.value.status) : false
  )
  const canApply = computed(
    () => checkResult.value?.canApply === true && !applying.value && !isJobActive.value
  )
  const jobProgress = computed(() => {
    if (!job.value) return 0
    if (job.value.status === 'success') return 100
    const progress = Number(job.value.progress)
    const normalizedProgress = Number.isFinite(progress) ? Math.min(100, Math.max(0, progress)) : 0
    if (job.value.status === 'restarting') return Math.max(95, normalizedProgress)
    return normalizedProgress
  })
  // 下载和验签阶段失败时还没动网站文件，不提回滚和进程守护。
  const failedBeforeInstall = computed(
    () => job.value?.status === 'failed' && stoppedBeforeInstall(job.value.progress)
  )
  const jobProgressStatus = computed(() => {
    if (job.value?.status === 'success') return 'success' as const
    if (job.value?.status === 'failed') return 'exception' as const
    return undefined
  })

  const loadStatus = async () => {
    loading.value = true
    try {
      status.value = await fetchOnlineUpdateStatus()
      statusFailed.value = false
      const running = status.value.runningJob
      if (running && ['running', 'restarting'].includes(running.status)) {
        if (!updateSession.watching || job.value?.id !== running.id) watchUpdateJob(running)
      } else if (running && !updateSession.watching) {
        job.value = running
      }
    } catch (error) {
      statusFailed.value = true
      if (handleUpdateError(error)) return
      ElMessage.error('更新状态加载失败')
    } finally {
      loading.value = false
    }
  }

  const loadHistory = async (refresh = false) => {
    historyLoading.value = true
    historyError.value = ''
    try {
      history.value = await fetchOnlineUpdateHistory(refresh)
    } catch (error: any) {
      historyError.value = error?.message || '历史版本加载失败'
    } finally {
      historyLoading.value = false
    }
  }

  const loadPage = async (refreshHistory = false) => {
    await Promise.all([loadStatus(), loadHistory(refreshHistory)])
  }

  const handleCheck = async () => {
    checking.value = true
    try {
      checkResult.value = await fetchOnlineUpdateCheck()
      sourceOverride.value = null
      checkFailed.value = false
      void loadHistory(true)
      if (checkResult.value.updateAvailable) {
        ElMessage.success(`发现新版本 v${checkResult.value.latest.version}`)
      } else {
        ElMessage.success('当前已经是最新版本')
      }
    } catch (error: any) {
      checkFailed.value = true
      void refreshOfficialSource()
      if (handleUpdateError(error)) return
      ElMessage.error(error?.message || '检查更新失败')
    } finally {
      checking.value = false
    }
  }

  const handleApply = async () => {
    if (!latest.value) return
    try {
      await appConfirm(
        `确认更新到 v${latest.value.version}？更新过程中服务会短暂重启。`,
        '在线更新',
        {
          confirmButtonText: '开始更新',
          cancelButtonText: '取消',
          type: 'warning'
        }
      )
    } catch {
      return
    }

    applying.value = true
    try {
      watchUpdateJob(await fetchOnlineUpdateApply())
      await nextTick()
      jobSectionRef.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    } catch (error: any) {
      if (handleUpdateError(error)) return
      ElMessage.error(error?.message || '更新启动失败')
    } finally {
      applying.value = false
    }
  }

  // 上传的包确认安装后，和在线更新一样进入「正在更新」。
  const handleUploadStarted = async (started: OnlineUpdateJob) => {
    watchUpdateJob(started)
    await nextTick()
    jobSectionRef.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  // 登录态失效（401）或无权限（403）时，在线更新接口已不可用，跳转 /admin 重新登录。
  const isSessionExpired = (error: unknown): boolean =>
    error instanceof HttpError && (error.code === 401 || error.code === 403)

  const redirectToAdminLogin = () => {
    if (redirectScheduled) return
    redirectScheduled = true
    window.setTimeout(() => window.location.replace('/admin'), 1000)
  }

  // 返回 true 表示已处理（跳转登录），调用方直接返回；否则按普通错误继续处理。
  const handleUpdateError = (error: unknown): boolean => {
    if (!isSessionExpired(error)) return false
    // 401 已由 axios 拦截器提示并触发登出；403 需自行提示后跳转。
    if (error instanceof HttpError && error.code === 403) {
      ElMessage.error('登录已失效，请重新登录')
    }
    redirectToAdminLogin()
    return true
  }

  const jobStatusText = (value: OnlineUpdateJob['status']) => {
    const labels: Record<OnlineUpdateJob['status'], string> = {
      running: '执行中',
      restarting: '重启中',
      success: '已完成',
      failed: '失败'
    }
    return labels[value] || value
  }

  const jobStatusTag = (value: OnlineUpdateJob['status']) => {
    const types: Record<
      OnlineUpdateJob['status'],
      'primary' | 'success' | 'warning' | 'danger' | 'info'
    > = {
      running: 'primary',
      restarting: 'info',
      success: 'success',
      failed: 'danger'
    }
    return types[value] || 'primary'
  }

  const formatBytes = (value?: number) => {
    if (!value || value <= 0) return '未配置'
    if (value < 1024) return `${value} B`
    if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
    return `${(value / 1024 / 1024).toFixed(2)} MB`
  }

  const formatDate = (value?: string) => {
    if (!value) return '-'
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    return date.toLocaleString()
  }

  onMounted(() => {
    void loadPage()
  })
</script>

<style lang="scss" scoped>
  .online-update {
    .update-header {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 16px;
      margin-bottom: 18px;

      .update-title {
        margin: 0;
        font-size: 20px;
        color: var(--art-gray-900);
      }

      .update-subtitle {
        margin: 6px 0 0;
        font-size: 13px;
        color: var(--art-gray-600);
      }

      .update-actions {
        display: flex;
        flex-shrink: 0;
        gap: 10px;
        align-items: center;
      }
    }

    .update-alert {
      margin-bottom: 12px;
    }

    .version-grid {
      display: grid;
      grid-template-columns: repeat(3, minmax(0, 1fr));
      gap: 14px;
    }

    .version-item {
      min-width: 0;
      padding: 18px;
      background: var(--art-gray-100);
      border: 1px solid var(--art-border-color);
      border-radius: 12px;

      &.latest.available {
        background: rgba(var(--art-primary-rgb), 0.07);
        border-color: rgba(var(--art-primary-rgb), 0.18);
      }

      .version-label {
        display: block;
        font-size: 13px;
        color: var(--art-gray-500);
      }

      .version-value {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 10px;
        margin-top: 10px;

        strong {
          overflow: hidden;
          font-size: 24px;
          color: var(--art-gray-900);
          text-overflow: ellipsis;
          white-space: nowrap;
        }

        &.plain {
          justify-content: flex-start;
        }
      }

      .version-meta {
        display: block;
        margin-top: 10px;
        overflow: hidden;
        font-size: 12px;
        color: var(--art-gray-500);
        text-overflow: ellipsis;
        white-space: nowrap;
      }
    }

    .update-section {
      margin-top: 22px;
      padding-top: 20px;
      border-top: 1px solid var(--art-border-color);

      .section-header {
        display: flex;
        align-items: center;
        justify-content: space-between;
        margin-bottom: 14px;

        strong {
          font-size: 15px;
          color: var(--art-gray-900);
        }

        .section-description {
          margin-left: 10px;
          font-size: 12px;
          color: var(--art-gray-500);
        }
      }
    }

    .notes-list {
      display: grid;
      gap: 10px;

      .note-item {
        display: flex;
        align-items: flex-start;
        gap: 8px;
        font-size: 13px;
        line-height: 1.6;
        color: var(--art-gray-700);

        .art-svg-icon {
          flex-shrink: 0;
          margin-top: 3px;
          color: var(--el-color-success);
        }
      }
    }

    .package-grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 12px;
    }

    .package-item {
      min-width: 0;
      padding: 12px;
      background: var(--art-gray-100);
      border-radius: 10px;

      &.wide {
        grid-column: span 2;
      }

      span {
        display: block;
        font-size: 12px;
        color: var(--art-gray-500);
      }

      strong {
        display: block;
        margin-top: 6px;
        overflow: hidden;
        font-size: 13px;
        color: var(--art-gray-800);
        text-overflow: ellipsis;
        white-space: nowrap;
      }
    }

    .job-section {
      scroll-margin-top: 20px;
    }

    .job-progress {
      padding: 14px;
      margin-bottom: 14px;
      background: var(--art-gray-100);
      border: 1px solid var(--art-border-color);
      border-radius: 10px;

      .job-progress-meta {
        display: flex;
        justify-content: space-between;
        gap: 12px;
        margin-top: 10px;
        font-size: 12px;
        color: var(--art-gray-600);

        span:last-child {
          flex-shrink: 0;
          color: var(--art-gray-500);
        }
      }
    }

    .job-logs {
      padding: 12px;
      background: #111827;
      border-radius: 10px;

      .job-log {
        font-family: 'Roboto Mono', Consolas, monospace;
        font-size: 12px;
        line-height: 1.8;
        color: #d1d5db;
        word-break: break-all;
      }
    }

    @media (max-width: 900px) {
      .version-grid,
      .package-grid {
        grid-template-columns: 1fr;
      }

      .package-item.wide {
        grid-column: span 1;
      }
    }

    @media (max-width: 768px) {
      .update-header {
        flex-direction: column;

        .update-actions {
          flex-wrap: wrap;
          width: 100%;

          :deep(.el-button + .el-button) {
            margin-left: 0;
          }
        }
      }
    }
  }
</style>
