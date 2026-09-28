<!-- 按存储位置浏览安装包：看压缩包内容、下载、校验、复制和删除。 -->
<template>
  <div class="storage-files art-full-height">
    <ElCard class="art-table-card" shadow="never">
      <ArtTableHeader :loading="loading" @refresh="loadObjects">
        <template #left>
          <ElSpace wrap>
            <ElSelect
              v-model="locationId"
              placeholder="选择存储"
              style="width: 220px"
              @change="loadObjects"
            >
              <ElOption
                v-for="item in locations"
                :key="item.id"
                :label="`${item.name}（${item.roleText}）`"
                :value="item.id"
              />
            </ElSelect>
            <ElButton @click="router.push('/source-station/storage')">返回存储列表</ElButton>
          </ElSpace>
        </template>
      </ArtTableHeader>
      <ArtTable :loading="loading" :data="rows" :columns="columns">
        <template #name="{ row }">
          <span class="cell-one-line">{{ row.name }}</span>
        </template>
        <template #item="{ row }">
          <ElTag v-if="row.orphan" size="small" type="info">孤儿文件</ElTag>
          <span v-else class="cell-one-line">{{ row.item || '-' }}</span>
        </template>
        <template #sha256="{ row }">
          <span class="cell-one-line">{{ row.sha256 || '-' }}</span>
        </template>
        <template #size="{ row }">{{ formatSize(row.size) }}</template>
        <template #updatedAt="{ row }">
          <span class="cell-one-line">{{ formatTime(row.updatedAt) }}</span>
        </template>
        <template #operation="{ row }">
          <RowActions
            :primary="[{ key: 'zip', label: '查看' }]"
            :more="[
              { key: 'download', label: '下载' },
              { key: 'verify', label: '重新校验' },
              { key: 'copy', label: '复制到' },
              { key: 'delete', label: '删除', danger: true }
            ]"
            @click="(action) => onRow(row, action.key)"
          />
        </template>
      </ArtTable>
    </ElCard>

    <ElDialog
      v-model="zipVisible"
      title="压缩包内容"
      :width="narrow ? '92%' : '720px'"
      destroy-on-close
    >
      <p class="zip-name cell-one-line">{{ zipName }}</p>
      <ElTable :data="zipFiles" size="small" empty-text="压缩包里没有文件">
        <ElTableColumn prop="name" label="文件" min-width="220" show-overflow-tooltip />
        <ElTableColumn label="大小" width="100">
          <template #default="{ row }">{{ formatSize(row.size) }}</template>
        </ElTableColumn>
      </ElTable>
      <div v-if="previewEntries.length" class="preview-block">
        <p class="preview-title">清单和说明</p>
        <ElTabs>
          <ElTabPane v-for="item in previewEntries" :key="item.name" :label="item.name">
            <pre class="preview-text">{{ item.text }}</pre>
          </ElTabPane>
        </ElTabs>
      </div>
    </ElDialog>

    <ElDialog v-model="copyVisible" title="复制到另一存储" :width="narrow ? '92%' : '420px'">
      <ElSelect v-model="copyTarget" placeholder="选择目标存储" style="width: 100%">
        <ElOption v-for="item in copyTargets" :key="item.id" :label="item.name" :value="item.id" />
      </ElSelect>
      <template #footer>
        <ElButton @click="copyVisible = false">取消</ElButton>
        <ElButton type="primary" :loading="copying" @click="confirmCopy">复制</ElButton>
      </template>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { ElMessageBox } from 'element-plus'
  import { useRoute, useRouter } from 'vue-router'
  import RowActions from '@/components/business/row-actions/index.vue'
  import { useNarrowScreen } from '@/hooks/core/useNarrowScreen'
  import { useTableColumns } from '@/hooks/core/useTableColumns'
  import {
    copyStorageObject,
    deleteStorageObject,
    downloadStorageObject,
    fetchStorageLocations,
    fetchStorageObjects,
    fetchStorageZip,
    verifyStorageObject,
    type StorageLocation,
    type StorageObjectRow
  } from '@/api/source-station'

  defineOptions({ name: 'SourceStationStorageFiles' })

  const route = useRoute()
  const router = useRouter()
  const narrow = useNarrowScreen()
  const loading = ref(false)
  const copying = ref(false)
  const locationId = ref('')
  const locations = ref<StorageLocation[]>([])
  const rows = ref<StorageObjectRow[]>([])
  const zipVisible = ref(false)
  const zipName = ref('')
  const zipFiles = ref<{ name: string; size: number }[]>([])
  const previewEntries = ref<{ name: string; text: string }[]>([])
  const copyVisible = ref(false)
  const copyTarget = ref('')
  const copyRow = ref<StorageObjectRow | null>(null)

  const copyTargets = computed(() => locations.value.filter((item) => item.id !== locationId.value))

  const { columns } = useTableColumns<StorageObjectRow>(() => [
    { prop: 'name', label: '文件名', minWidth: 180, useSlot: true },
    { prop: 'size', label: '大小', width: 100, useSlot: true },
    { prop: 'updatedAt', label: '时间', minWidth: 160, useSlot: true },
    { prop: 'item', label: '对应条目', minWidth: 160, useSlot: true },
    { prop: 'sha256', label: '校验码', minWidth: 180, useSlot: true },
    { prop: 'operation', label: '操作', width: 148, fixed: 'right', useSlot: true }
  ])

  function formatSize(size?: number) {
    const value = Number(size || 0)
    if (value < 1024) return `${value} B`
    if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
    return `${(value / 1024 / 1024).toFixed(1)} MB`
  }

  function formatTime(value?: string) {
    if (!value || value.startsWith('0001')) return '-'
    return value.replace('T', ' ').replace('Z', '').slice(0, 19)
  }

  async function loadLocations() {
    const data = await fetchStorageLocations()
    locations.value = data.locations || []
    const queryId = String(route.query.locationId || '')
    if (queryId && locations.value.some((item) => item.id === queryId)) {
      locationId.value = queryId
    } else if (!locationId.value && locations.value[0]) {
      locationId.value = locations.value[0].id
    }
  }

  async function loadObjects() {
    if (!locationId.value) {
      rows.value = []
      return
    }
    loading.value = true
    try {
      const data = await fetchStorageObjects(locationId.value)
      rows.value = data.list || []
    } finally {
      loading.value = false
    }
  }

  async function openZip(row: StorageObjectRow) {
    const data = await fetchStorageZip(locationId.value, row.key)
    zipName.value = row.name
    zipFiles.value = data.files || []
    previewEntries.value = Object.entries(data.preview || {}).map(([name, text]) => ({
      name,
      text
    }))
    zipVisible.value = true
  }

  async function onRow(row: StorageObjectRow, key: string) {
    if (key === 'zip') {
      await openZip(row)
      return
    }
    if (key === 'download') {
      const data = await downloadStorageObject(locationId.value, row.key)
      if (data.url) window.open(data.url, '_blank', 'noopener')
      return
    }
    if (key === 'verify') {
      await verifyStorageObject(locationId.value, row.key)
      return
    }
    if (key === 'copy') {
      copyRow.value = row
      copyTarget.value = copyTargets.value[0]?.id || ''
      copyVisible.value = true
      return
    }
    if (key === 'delete') {
      let typed = ''
      try {
        const result = await ElMessageBox.prompt(
          `请输入文件名「${row.name}」以确认删除。`,
          '删除文件',
          {
            inputPlaceholder: row.name,
            confirmButtonText: '删除',
            cancelButtonText: '取消'
          }
        )
        typed = String(result.value || '')
      } catch {
        return
      }
      await deleteStorageObject(locationId.value, row.key, typed)
      await loadObjects()
    }
  }

  async function confirmCopy() {
    if (!copyRow.value || !copyTarget.value) return
    copying.value = true
    try {
      await copyStorageObject(locationId.value, copyRow.value.key, copyTarget.value)
      copyVisible.value = false
    } finally {
      copying.value = false
    }
  }

  onMounted(async () => {
    await loadLocations()
    await loadObjects()
  })
</script>

<style scoped lang="scss">
  .zip-name,
  .preview-title {
    margin: 0 0 8px;
    color: var(--art-gray-700);
  }

  .preview-block {
    margin-top: 16px;
  }

  .preview-text {
    max-height: 240px;
    overflow: auto;
    margin: 0;
    padding: 12px;
    border-radius: 8px;
    background: var(--el-color-primary-light-9);
    color: var(--el-text-color-primary);
    white-space: pre-wrap;
    word-break: break-word;
  }

  .cell-one-line {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
