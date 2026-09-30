<!-- 从已保存令牌连接的私有仓库选择一个 Release，并回填版本、标题、说明和校验值。 -->
<template>
  <div class="release-import">
    <ElEmpty v-if="unbound" description="这个应用还没有绑定仓库。" />
    <div v-else class="release-import__bar">
      <span v-if="connectedRepo">{{ connectedRepo }}</span>
      <ElButton :loading="listing" @click="loadReleases">列出发布</ElButton>
    </div>
    <div v-if="releases.length" class="release-import__table-wrap">
      <ElTable
        :data="releases"
        row-key="tag"
        size="small"
        highlight-current-row
        :current-row-key="picked"
        :row-class-name="rowClass"
        class="release-import__table"
        @row-click="choose"
      >
        <ElTableColumn label="版本" width="120">
          <template #default="{ row }">
            <button
              type="button"
              class="cell-copy"
              :title="row.tag"
              @click.stop="pick(row, row.tag)"
            >
              {{ row.tag }}
            </button>
          </template>
        </ElTableColumn>
        <ElTableColumn label="标题" min-width="140">
          <template #default="{ row }">
            <button
              type="button"
              class="cell-copy"
              :title="row.title"
              @click.stop="pick(row, row.title)"
            >
              {{ row.title }}
            </button>
          </template>
        </ElTableColumn>
        <ElTableColumn label="说明" min-width="180">
          <template #default="{ row }">
            <button
              type="button"
              class="cell-copy"
              :title="row.changelog"
              @click.stop="pick(row, row.changelog)"
            >
              {{ row.changelog || '—' }}
            </button>
          </template>
        </ElTableColumn>
        <ElTableColumn label="安装包" min-width="160">
          <template #default="{ row }">
            <span class="cell-copy">{{ row.assetName || '—' }}</span>
          </template>
        </ElTableColumn>
      </ElTable>
    </div>
    <p v-else-if="listed" class="release-import__hint">{{ emptyText }}</p>
    <p v-if="loadingTag" class="release-import__hint"
      >正在导入 {{ loadingTag }}，完成后可以再改表单</p
    >
  </div>
</template>

<script setup lang="ts">
  import { ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import {
    fetchReleaseAsset,
    fetchReleaseImports,
    type ReleaseImportItem,
    type ReleaseImportResult
  } from '@/api/release-import'

  const props = defineProps<{
    apiBase: string
    purpose: 'app' | 'plugin' | 'template'
    appId?: number
    priceCents?: number
    appKey?: string
  }>()

  const emit = defineEmits<{
    filled: [value: ReleaseImportResult]
  }>()

  const unbound = ref(false)
  const emptyText = ref('这个位置还没有可导入的发布。')
  const connectedRepo = ref('')
  const releases = ref<ReleaseImportItem[]>([])
  const listing = ref(false)
  const listed = ref(false)
  const loadingTag = ref('')
  const picked = ref('')

  function rowClass({ row }: { row: ReleaseImportItem }) {
    return row.tag === picked.value ? 'is-picked' : ''
  }

  async function loadReleases() {
    listing.value = true
    listed.value = false
    try {
      if (!props.appId) {
        unbound.value = true
        return
      }
      const data = await fetchReleaseImports(
        props.apiBase,
        props.purpose,
        props.appId,
        props.priceCents || 0
      )
      unbound.value = false
      releases.value = data?.releases || []
      connectedRepo.value = data?.repo || ''
      emptyText.value = data?.empty || '这个位置还没有可导入的发布。'
      listed.value = true
    } finally {
      listing.value = false
    }
  }

  function pick(row: ReleaseImportItem, text: string) {
    void copy(text, true)
    void choose(row)
  }

  async function choose(row: ReleaseImportItem) {
    if (!row?.tag || loadingTag.value) return
    picked.value = row.tag
    loadingTag.value = row.tag
    try {
      const filled = await fetchReleaseAsset(props.apiBase, {
        purpose: props.purpose,
        appId: props.appId || 0,
        priceCents: props.priceCents || 0,
        appKey: props.appKey || '',
        tag: row.tag,
        assetName: row.assetName
      })
      emit('filled', filled)
      ElMessage.success('已填入发布信息，确认前仍可修改')
    } finally {
      loadingTag.value = ''
    }
  }

  async function copy(text: string, quiet = false) {
    const value = (text || '').trim()
    if (!value) return
    try {
      await navigator.clipboard.writeText(value)
      if (!quiet) ElMessage.success('已复制')
    } catch {
      if (!quiet) ElMessage.info(value)
    }
  }
</script>

<style scoped>
  .release-import__bar {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .release-import__repo {
    flex: 1;
    min-width: 0;
  }

  .release-import__hint {
    margin: 8px 0 0;
    font-size: 12px;
    color: var(--el-text-color-secondary);
  }

  .release-import__table-wrap {
    margin-top: 8px;
    overflow-x: auto;
  }

  .release-import__table {
    min-width: 560px;
  }

  .release-import__table :deep(.el-table__body tr.is-picked > td) {
    color: var(--el-color-primary);
    background: var(--el-color-primary-light-8) !important;
  }

  .release-import__table :deep(.el-table__body tr.is-picked > td:first-child) {
    box-shadow: inset 3px 0 0 var(--el-color-primary);
  }

  .release-import__table :deep(.el-table__body tr.is-picked .cell-copy) {
    font-weight: 600;
  }

  .cell-copy {
    display: block;
    max-width: 100%;
    padding: 0;
    overflow: hidden;
    font: inherit;
    color: inherit;
    text-align: left;
    text-overflow: ellipsis;
    white-space: nowrap;
    cursor: pointer;
    background: transparent;
    border: 0;
  }
</style>
