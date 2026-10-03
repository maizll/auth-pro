<!-- 按存储位置浏览安装包：按客户端版本、收费插件、收费模板分组，每个版本一行；可查看、下载、校验、复制和删除。 -->
<template>
  <div class="storage-files art-full-height">
    <ElCard class="art-table-card" shadow="never">
      <ArtTableHeader :loading="loading" @refresh="loadObjects">
        <template #left>
          <div class="files-toolbar">
            <ElSelect
              v-model="locationId"
              placeholder="选择存储"
              class="location-select"
              @change="loadObjects"
            >
              <ElOption
                v-for="item in locations"
                :key="item.id"
                :label="`${item.name}（${item.roleText}）`"
                :value="item.id"
              />
            </ElSelect>
            <ElInput
              v-model="keyword"
              class="search-input"
              clearable
              placeholder="搜索版本、插件、文件名"
            />
            <ElButton @click="router.push('/source-station/storage')">返回存储列表</ElButton>
          </div>
        </template>
      </ArtTableHeader>
      <ElTabs v-model="activeGroup" class="group-tabs">
        <ElTabPane v-for="group in visibleGroups" :key="group.key" :name="group.key">
          <template #label>
            <span>{{ group.label }}</span>
            <span class="group-count">{{ groups[group.key].length }}</span>
          </template>
        </ElTabPane>
      </ElTabs>
      <ArtTable :loading="loading" :data="activeRows" :columns="columns" row-key="id">
        <template #version="{ row }">
          <div class="version-cell">
            <span class="version-title">{{ rowTitle(row) }}</span>
            <ElTag v-if="row.legacy" size="small" type="info" effect="plain">旧格式</ElTag>
            <ElTag v-if="row.orphan" size="small" type="info">孤儿文件</ElTag>
          </div>
        </template>
        <template #files="{ row }">
          <div class="file-chips">
            <span v-for="item in row.files" :key="item.key" class="file-chip">{{
              fileLabel(item)
            }}</span>
          </div>
        </template>
        <template #size="{ row }">{{ formatSize(row.size) }}</template>
        <template #updatedAt="{ row }">
          <span class="cell-one-line">{{ formatTime(row.updatedAt) }}</span>
        </template>
        <template #item="{ row }">
          <span class="cell-one-line">{{ row.item || '-' }}</span>
        </template>
        <template #operation="{ row }">
          <RowActions
            :primary="[{ key: 'files', label: '文件' }]"
            :more="[
              { key: 'download', label: '下载安装包' },
              { key: 'manifest', label: '下载 latest.json', hidden: !row.hasManifest },
              { key: 'verify', label: '重新校验' },
              { key: 'copy', label: '复制到' }
            ]"
            @click="(action) => onVersion(row, action.key)"
          />
        </template>
      </ArtTable>
    </ElCard>

    <AppDialog v-model="filesVisible" :title="filesTitle" size="lg" flow="long" destroy-on-close>
      <p v-if="current" class="tag-line">发布标签 {{ current.tag || '-' }}</p>
      <ElTable v-if="current" :data="current.files" size="small" row-key="key">
        <ElTableColumn prop="name" label="文件" min-width="200" show-overflow-tooltip />
        <ElTableColumn label="大小" width="96">
          <template #default="{ row }">{{ formatSize(row.size) }}</template>
        </ElTableColumn>
        <ElTableColumn label="校验码" min-width="140" show-overflow-tooltip>
          <template #default="{ row }">{{ row.sha256 || '-' }}</template>
        </ElTableColumn>
        <ElTableColumn label="操作" width="132" fixed="right">
          <template #default="{ row }">
            <RowActions
              :primary="[{ key: 'zip', label: '查看', hidden: !row.name.endsWith('.zip') }]"
              :more="[
                { key: 'download', label: '下载' },
                { key: 'verify', label: '重新校验' },
                { key: 'copy', label: '复制到' },
                { key: 'delete', label: '删除', danger: true }
              ]"
              @click="(action) => onFile(row, action.key)"
            />
          </template>
        </ElTableColumn>
      </ElTable>
    </AppDialog>

    <AppDialog v-model="zipVisible" title="压缩包内容" size="xl" flow="short" destroy-on-close>
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
    </AppDialog>

    <AppDialog v-model="copyVisible" title="复制到另一存储" size="sm" flow="short">
      <ElSelect v-model="copyTarget" placeholder="选择目标存储" style="width: 100%">
        <ElOption v-for="item in copyTargets" :key="item.id" :label="item.name" :value="item.id" />
      </ElSelect>
      <template #footer>
        <ElButton @click="copyVisible = false">取消</ElButton>
        <ElButton type="primary" :loading="copying" @click="confirmCopy">复制</ElButton>
      </template>
    </AppDialog>
  </div>
</template>

<script setup lang="ts">
  import { appPrompt } from '@/utils/app-confirm'
  import { formatLocalDateTime } from '@/utils/local-time'
  import AppDialog from '@/components/core/dialog/AppDialog.vue'

  import { useRoute, useRouter } from 'vue-router'
  import RowActions from '@/components/business/row-actions/index.vue'
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
  import {
    PACKAGE_GROUPS,
    filterPackageRows,
    groupPackageFiles,
    type PackageGroupKey,
    type PackageVersionRow
  } from './package-groups'

  defineOptions({ name: 'SourceStationStorageFiles' })

  const route = useRoute()
  const router = useRouter()
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

  const keyword = ref('')
  const activeGroup = ref<PackageGroupKey>('client')
  const groups = computed(() => groupPackageFiles(rows.value))
  // 「其他」没有文件时不显示；前三组始终显示，便于看出缺了哪类。
  const visibleGroups = computed(() =>
    PACKAGE_GROUPS.filter((group) => group.key !== 'other' || groups.value.other.length)
  )
  const activeRows = computed(() =>
    filterPackageRows(groups.value[activeGroup.value], keyword.value)
  )

  // 搜索时当前分组没有结果、别的分组有，就自动切过去。
  watch(keyword, () => {
    if (activeRows.value.length) return
    const hit = visibleGroups.value.find(
      (group) => filterPackageRows(groups.value[group.key], keyword.value).length
    )
    if (hit) activeGroup.value = hit.key
  })

  const { columns } = useTableColumns<PackageVersionRow>(() => [
    { prop: 'version', label: '版本', minWidth: 200, useSlot: true },
    { prop: 'files', label: '文件', minWidth: 160, useSlot: true },
    { prop: 'size', label: '大小', width: 100, useSlot: true },
    { prop: 'updatedAt', label: '时间', minWidth: 160, useSlot: true },
    { prop: 'item', label: '对应条目', minWidth: 160, useSlot: true },
    { prop: 'operation', label: '操作', width: 168, fixed: 'right', useSlot: true }
  ])

  const filesVisible = ref(false)
  const current = ref<PackageVersionRow | null>(null)
  const filesTitle = computed(() => (current.value ? `${rowTitle(current.value)} 的文件` : '文件'))

  function rowTitle(row: PackageVersionRow) {
    if (row.group === 'client') return row.version || row.tag
    if (row.group === 'other') return row.tag || row.main.name
    return row.version ? `${row.name} ${row.version}` : row.name
  }

  function fileLabel(file: StorageObjectRow) {
    if (file.name === 'latest.json') return 'latest.json'
    if (/\.(zip|tar\.gz|tgz)$/i.test(file.name)) return '安装包'
    return file.name
  }

  function formatSize(size?: number) {
    const value = Number(size || 0)
    if (value < 1024) return `${value} B`
    if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
    return `${(value / 1024 / 1024).toFixed(1)} MB`
  }

  function formatTime(value?: string) {
    return formatLocalDateTime(value) || '-'
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
      if (!groups.value[activeGroup.value].length) {
        activeGroup.value =
          PACKAGE_GROUPS.find((group) => groups.value[group.key].length)?.key || 'client'
      }
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

  async function onVersion(row: PackageVersionRow, key: string) {
    if (key === 'files') {
      current.value = row
      filesVisible.value = true
      return
    }
    if (key === 'manifest') {
      const manifest = row.files.find((item) => item.name === 'latest.json')
      if (manifest) await onFile(manifest, 'download')
      return
    }
    await onFile(row.main, key)
  }

  async function onFile(row: StorageObjectRow, key: string) {
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
        const result = await appPrompt(`请输入文件名「${row.name}」以确认删除。`, '删除文件', {
          inputPlaceholder: row.name,
          confirmButtonText: '删除',
          cancelButtonText: '取消'
        })
        typed = String(result.value || '')
      } catch {
        return
      }
      await deleteStorageObject(locationId.value, row.key, typed)
      await loadObjects()
      // 文件弹框里删完后同步刷新；这个版本的文件都删光就关掉弹框。
      if (current.value) {
        const tag = current.value.tag
        const next = Object.values(groups.value)
          .flat()
          .find((item) => item.tag === tag)
        current.value = next || null
        filesVisible.value = Boolean(next)
      }
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
  .files-toolbar {
    display: flex;
    flex-wrap: wrap;
    gap: 8px 12px;
    align-items: center;
  }

  .location-select {
    width: 220px;
  }

  .search-input {
    width: 240px;
  }

  .group-tabs {
    margin-bottom: 4px;

    :deep(.el-tabs__header) {
      margin-bottom: 8px;
    }
  }

  .group-count {
    display: inline-block;
    min-width: 20px;
    padding: 0 6px;
    margin-left: 6px;
    font-size: 12px;
    line-height: 18px;
    color: var(--el-text-color-secondary);
    text-align: center;
    background: var(--el-fill-color-light);
    border-radius: 9px;
  }

  .version-cell {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 6px;
    align-items: center;
    min-width: 0;
  }

  .version-title {
    font-weight: 500;
    color: var(--el-text-color-primary);
    word-break: break-all;
  }

  .file-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }

  .file-chip {
    padding: 0 8px;
    font-size: 12px;
    line-height: 20px;
    color: var(--el-color-primary);
    white-space: nowrap;
    background: var(--el-color-primary-light-9);
    border-radius: 4px;
  }

  .tag-line {
    margin: 0 0 10px;
    font-size: 13px;
    color: var(--art-gray-600);
    word-break: break-all;
  }

  @media (width <= 640px) {
    .location-select,
    .search-input {
      width: 100%;
    }
  }

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
