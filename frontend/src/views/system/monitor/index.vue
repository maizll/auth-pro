<template>
  <div class="monitor-page art-full-height">
    <ElCard class="page-card" shadow="never">
      <template #header>
        <div class="card-header">
          <div>
            <h2>定时任务 / 系统监控</h2>
            <p>进程内调度，不接入第三方监控。可开关、查看上次结果和下次运行，也可以立即执行。</p>
          </div>
          <ElButton :loading="loading" @click="loadJobs">刷新</ElButton>
        </div>
      </template>

      <ElTable v-loading="loading" :data="jobs" border>
        <ElTableColumn prop="name" label="任务" min-width="160" />
        <ElTableColumn prop="description" label="说明" min-width="280" show-overflow-tooltip />
        <ElTableColumn label="间隔" width="110">
          <template #default="{ row }">{{ intervalLabel(row.intervalSeconds) }}</template>
        </ElTableColumn>
        <ElTableColumn label="启用" width="90" align="center">
          <template #default="{ row }">
            <ElSwitch
              :model-value="row.enabled"
              :loading="pendingId === row.id && pendingAction === 'toggle'"
              @change="(value) => handleToggle(row, Boolean(value))"
            />
          </template>
        </ElTableColumn>
        <ElTableColumn label="上次结果" min-width="240">
          <template #default="{ row }">
            <div class="result-cell">
              <ElTag v-if="row.lastStatus" :type="row.lastStatus === 'ok' ? 'success' : 'danger'" size="small">
                {{ row.lastStatus === 'ok' ? '成功' : '失败' }}
              </ElTag>
              <span v-else class="muted">尚未执行</span>
              <span class="result-text">{{ row.lastMessage || '' }}</span>
              <span v-if="row.lastRunAt" class="muted">{{ formatTime(row.lastRunAt) }}</span>
            </div>
          </template>
        </ElTableColumn>
        <ElTableColumn label="下次运行" width="180">
          <template #default="{ row }">
            {{ row.enabled ? formatTime(row.nextRunAt) : '已暂停' }}
          </template>
        </ElTableColumn>
        <ElTableColumn label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <ElButton
              link
              type="primary"
              :loading="pendingId === row.id && pendingAction === 'run'"
              :disabled="row.running"
              @click="handleRun(row)"
            >
              立即执行
            </ElButton>
          </template>
        </ElTableColumn>
      </ElTable>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import { fetchMonitorJobs, runMonitorJob, updateMonitorJob, type MonitorJob } from '@/api/system-monitor'

  defineOptions({ name: 'SystemMonitor' })

  const loading = ref(false)
  const jobs = ref<MonitorJob[]>([])
  const pendingId = ref('')
  const pendingAction = ref<'toggle' | 'run' | ''>('')

  function intervalLabel(seconds: number) {
    if (seconds % 3600 === 0) return `每 ${seconds / 3600} 小时`
    if (seconds % 60 === 0) return `每 ${seconds / 60} 分钟`
    return `每 ${seconds} 秒`
  }

  function formatTime(value: string) {
    if (!value) return '-'
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
  }

  async function loadJobs() {
    loading.value = true
    try {
      const data = await fetchMonitorJobs()
      jobs.value = data.jobs || []
    } finally {
      loading.value = false
    }
  }

  async function handleToggle(row: MonitorJob, enabled: boolean) {
    pendingId.value = row.id
    pendingAction.value = 'toggle'
    try {
      const data = await updateMonitorJob(row.id, enabled)
      jobs.value = data.jobs || []
    } catch {
      ElMessage.error('更新任务开关失败')
    } finally {
      pendingId.value = ''
      pendingAction.value = ''
    }
  }

  async function handleRun(row: MonitorJob) {
    pendingId.value = row.id
    pendingAction.value = 'run'
    try {
      const data = await runMonitorJob(row.id)
      jobs.value = data.jobs || []
    } catch {
      ElMessage.error('执行失败')
      await loadJobs()
    } finally {
      pendingId.value = ''
      pendingAction.value = ''
    }
  }

  onMounted(loadJobs)
</script>

<style scoped lang="scss">
  .monitor-page {
    padding: 12px;
  }

  .card-header {
    display: flex;
    gap: 12px;
    align-items: flex-start;
    justify-content: space-between;

    h2 {
      margin: 0;
      font-size: 18px;
    }

    p {
      margin: 6px 0 0;
      font-size: 13px;
      line-height: 1.5;
      color: var(--el-text-color-secondary);
    }
  }

  .result-cell {
    display: flex;
    flex-direction: column;
    gap: 4px;
    align-items: flex-start;
  }

  .result-text {
    line-height: 1.4;
    word-break: break-word;
  }

  .muted {
    font-size: 12px;
    color: var(--el-text-color-secondary);
  }
</style>
