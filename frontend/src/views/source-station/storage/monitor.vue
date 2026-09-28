<!-- 存储检查：定时任务的结果。出问题只提醒，不改目录上下架。 -->
<template>
  <div class="storage-monitor art-full-height">
    <ElCard class="art-table-card" shadow="never">
      <ArtTableHeader :loading="loading" @refresh="loadHealth">
        <template #left>
          <ElSpace wrap>
            <ElButton type="primary" :loading="running" @click="runNow">立即检查</ElButton>
            <span class="hint">{{ intervalText }}</span>
          </ElSpace>
        </template>
      </ArtTableHeader>
      <ElAlert class="page-alert" type="info" :closable="false" show-icon :title="summary" />
      <ArtTable :loading="loading" :data="rows" :columns="columns">
        <template #level="{ row }">
          <ElTag :type="row.level === 'problem' ? 'danger' : 'success'" size="small">
            {{ row.level === 'problem' ? '有问题' : '正常' }}
          </ElTag>
        </template>
        <template #locationName="{ row }">
          <span class="cell-one-line">{{ row.locationName || '-' }}</span>
        </template>
        <template #target="{ row }">
          <span class="cell-one-line">{{ row.target || '-' }}</span>
        </template>
        <template #message="{ row }">
          <span class="cell-one-line">{{ row.message }}</span>
        </template>
      </ArtTable>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
  import { useTableColumns } from '@/hooks/core/useTableColumns'
  import { fetchStorageHealth, runStorageHealth, type StorageHealthRow } from '@/api/source-station'

  defineOptions({ name: 'SourceStationStorageMonitor' })

  const loading = ref(false)
  const running = ref(false)
  const rows = ref<StorageHealthRow[]>([])
  const checkedAt = ref('')
  const intervalText = ref('每小时检查一次。出问题只提醒，不会自动下架。')

  const summary = computed(() => {
    const when = formatTime(checkedAt.value)
    const problems = rows.value.filter((row) => row.level === 'problem').length
    if (!when) return '还没有检查记录。点「立即检查」会连上各存储并确认安装包还在。'
    if (!problems) return `${when} 检查完成，存储可以连接，已登记的安装包都在。`
    return `${when} 发现 ${problems} 个问题。目录不会因此下架，请按说明处理。`
  })

  const { columns } = useTableColumns<StorageHealthRow>(() => [
    { prop: 'level', label: '结果', width: 90, useSlot: true },
    { prop: 'locationName', label: '存储', minWidth: 140, useSlot: true },
    { prop: 'target', label: '对象', minWidth: 140, useSlot: true },
    { prop: 'message', label: '说明', minWidth: 240, useSlot: true }
  ])

  function formatTime(value?: string) {
    if (!value || value.startsWith('0001')) return ''
    return value.replace('T', ' ').replace('Z', '').slice(0, 19)
  }

  function apply(data: { checkedAt?: string; list: StorageHealthRow[] | null; interval: string }) {
    rows.value = data.list || []
    checkedAt.value = data.checkedAt || ''
    if (data.interval) intervalText.value = data.interval
  }

  async function loadHealth() {
    loading.value = true
    try {
      apply(await fetchStorageHealth())
    } finally {
      loading.value = false
    }
  }

  async function runNow() {
    running.value = true
    try {
      apply(await runStorageHealth())
    } finally {
      running.value = false
    }
  }

  onMounted(loadHealth)
</script>

<style scoped lang="scss">
  .page-alert {
    margin-bottom: 12px;
  }

  .hint {
    color: var(--art-gray-600);
    font-size: 13px;
  }

  .cell-one-line {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
