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
          <ElButton size="small" :icon="Search" :loading="checking" @click="handleCheck">
            检查更新
          </ElButton>
          <UploadUpdatePackage
            ref="uploadRef"
            :official="!!officialSource"
            :disabled="isJobActive"
            :hide-trigger="narrow"
            @started="handleUploadStarted"
          />
          <ElDropdown v-if="narrow" trigger="click" @command="uploadRef?.open()">
            <ElButton size="small">更多</ElButton>
            <template #dropdown>
              <ElDropdownMenu>
                <ElDropdownItem command="upload" :disabled="isJobActive">上传更新包</ElDropdownItem>
              </ElDropdownMenu>
            </template>
          </ElDropdown>
          <ElButton
            size="small"
            type="primary"
            :icon="Download"
            :disabled="!canApply"
            :loading="applying"
            @click="handleApply"
          >
            {{ applyLabel }}
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
            <ElTag :type="behindCount ? 'danger' : 'info'" effect="plain">
              {{ behindCount ? `落后 ${behindCount} 个版本` : '当前' }}
            </ElTag>
          </div>
          <span class="version-meta">{{
            status?.buildTime ? formatDate(status.buildTime) : '未记录构建时间'
          }}</span>
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
          <div>
            <strong>更新内容</strong>
            <span v-if="behindCount" class="section-description">
              v{{ currentVersion }} → v{{ latest?.version }}，共 {{ behindCount }} 个版本
            </span>
          </div>
          <ElText v-if="latest?.force" type="danger" size="small">强制更新</ElText>
        </div>
        <template v-if="behindCount">
          <p class="pending-tip">
            {{
              behindCount > 1
                ? `一次更新会直接升到 v${latest?.version}，中间的 ${behindCount - 1} 个版本不用逐个安装。下面按版本列出这 ${behindCount} 个版本的全部更新内容。`
                : `一次更新会升到 v${latest?.version}。`
            }}
          </p>
          <div class="pending-list">
            <div
              v-for="(release, index) in shownPending"
              :key="release.version"
              class="pending-version"
            >
              <div class="pending-version-head">
                <strong>v{{ release.version }}</strong>
                <ElTag v-if="index === 0" type="success" effect="plain" size="small">最新</ElTag>
                <span class="pending-date">{{ formatDay(release.releasedAt) }}</span>
              </div>
              <div v-if="release.notes.length" class="notes-list">
                <div v-for="note in release.notes" :key="note" class="note-item">
                  <ArtSvgIcon icon="ri:checkbox-circle-line" />
                  <span>{{ note }}</span>
                </div>
              </div>
              <p v-else class="pending-empty">这一版没有写更新说明</p>
            </div>
          </div>
          <ElButton
            v-if="pending.length > shownPending.length"
            class="pending-more"
            size="small"
            text
            type="primary"
            @click="pendingExpanded = true"
          >
            展开其余 {{ pending.length - shownPending.length }} 个版本
          </ElButton>
        </template>
        <div v-else-if="latest?.notes?.length" class="notes-list">
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

      <OfficialUpdateSource v-if="officialSource" :source="officialSource" />

      <div class="update-section auto-section">
        <strong class="auto-title">自动更新</strong>
        <ElSwitch
          :model-value="autoUpdate"
          :loading="autoSaving"
          size="small"
          @change="handleAutoChange"
        />
        <span class="auto-text">
          收到强制更新时，在服务器时间{{
            status?.autoWindow || '凌晨 3:00–5:00'
          }}自动安装，同样核对签名、先备份，失败自动回退，结果记入操作日志。
        </span>
      </div>

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
  import { Download, Search } from '@element-plus/icons-vue'
  import { useWindowSize } from '@vueuse/core'
  import {
    saveOnlineUpdateAuto,
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
  import { refreshForceNotice, setForceNotice } from './force-notice'
  import { UPDATE_RESTART_RECOVERY } from './restart-timeout'
  import { latestVersionStatus, updateChannelLabel } from './status-label'
  import { pendingReleases } from './release-history'
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
  // 当前版本之后到最新版的每一版。隔了很多版没更新时，用户要知道落后几版、这次会装进哪些改动。
  const pending = computed(() =>
    updateAvailable.value
      ? pendingReleases(historyReleases.value, currentVersion.value, latest.value)
      : []
  )
  const behindCount = computed(() => pending.value.length)
  // 版本多时先列最近 5 个，其余点一下再展开
  const PENDING_PREVIEW = 5
  const pendingExpanded = ref(false)
  const shownPending = computed(() =>
    pendingExpanded.value ? pending.value : pending.value.slice(0, PENDING_PREVIEW)
  )
  const packageError = computed(() => checkResult.value?.packageError || '')
  const versionError = computed(() => checkResult.value?.versionError || '')
  const packageValid = computed(() => checkResult.value?.packageValid === true)
  const isJobActive = computed(() =>
    job.value ? ['running', 'restarting'].includes(job.value.status) : false
  )
  const canApply = computed(
    () => checkResult.value?.canApply === true && !applying.value && !isJobActive.value
  )
  // 检查过且没有可装的新版本时，主按钮直接写「已是最新」，不留一个看不出原因的灰按钮
  const applyLabel = computed(() =>
    checkResult.value && !updateAvailable.value && !isJobActive.value ? '已是最新' : '立即更新'
  )
  // 按钮一行排开；屏幕太窄放不下时把「上传更新包」收进「更多」
  const { width: windowWidth } = useWindowSize()
  const narrow = computed(() => windowWidth.value < 360)
  const uploadRef = ref<InstanceType<typeof UploadUpdatePackage> | null>(null)

  const autoUpdate = computed(() => status.value?.autoUpdate === true)
  const autoSaving = ref(false)
  const handleAutoChange = async (value: string | number | boolean) => {
    autoSaving.value = true
    try {
      const notice = await saveOnlineUpdateAuto(value === true)
      if (status.value) status.value.autoUpdate = notice.autoUpdate
      setForceNotice(notice)
      ElMessage.success(notice.autoUpdate ? '已开启自动更新' : '已关闭自动更新')
    } catch {
      // 失败提示由请求层给出，开关保持原状
    } finally {
      autoSaving.value = false
    }
  }
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
      void loadStatus()
      void loadHistory(true)
      void refreshForceNotice()
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
      const span =
        behindCount.value > 1
          ? `将从 v${currentVersion.value} 直接升到 v${latest.value.version}（包含其间 ${behindCount.value} 个版本的全部改动）。`
          : `确认更新到 v${latest.value.version}？`
      await appConfirm(`${span}更新过程中服务会短暂重启。`, '在线更新', {
        confirmButtonText: '开始更新',
        cancelButtonText: '取消',
        type: 'warning'
      })
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

  const formatDay = (value?: string) => {
    if (!value) return ''
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return ''
    return date.toLocaleDateString()
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
        flex-wrap: nowrap;
        gap: 8px;
        align-items: center;

        :deep(.el-button + .el-button) {
          margin-left: 0;
        }
      }
    }

    .update-alert {
      margin-bottom: 12px;
    }

    .auto-section {
      display: flex;
      flex-wrap: wrap;
      gap: 6px 12px;
      align-items: center;

      .auto-title {
        font-size: 15px;
        color: var(--art-gray-900);
      }

      .auto-text {
        flex: 1;
        min-width: 220px;
        font-size: 13px;
        line-height: 20px;
        color: var(--art-gray-600);
      }
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
        background: var(--el-color-primary-light-9);
        border-color: var(--el-color-primary-light-8);
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

    .pending-tip {
      margin: 0 0 14px;
      font-size: 13px;
      line-height: 1.6;
      color: var(--art-gray-600);
    }

    .pending-list {
      display: grid;
      gap: 16px;
    }

    .pending-version {
      padding: 14px;
      background: var(--art-gray-100);
      border: 1px solid var(--art-border-color);
      border-radius: 10px;

      .pending-version-head {
        display: flex;
        align-items: center;
        gap: 8px;
        margin-bottom: 10px;

        strong {
          font-size: 14px;
          color: var(--art-gray-900);
        }

        .pending-date {
          margin-left: auto;
          font-size: 12px;
          color: var(--art-gray-500);
        }
      }

      .pending-empty {
        margin: 0;
        font-size: 13px;
        color: var(--art-gray-500);
      }
    }

    .pending-more {
      margin-top: 10px;
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

        gap: 12px;

        .update-actions {
          width: 100%;
        }
      }
    }
  }
</style>
