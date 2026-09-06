<!-- 授权版本下载弹层：用户端 / 代理端共用 -->
<template>
  <el-dialog
    v-model="visible"
    :title="`版本下载${appName ? ` · ${appName}` : ''}`"
    width="640px"
    destroy-on-close
    class="license-versions-dialog"
  >
    <div v-loading="loading" class="version-list">
      <template v-if="versions.length">
        <div v-for="item in versions" :key="item.id" class="version-item">
          <div class="version-main">
            <div class="version-title-row">
              <span class="version-number">v{{ item.version }}</span>
              <el-tag v-if="item.forceUpdate" type="danger" size="small" effect="plain">
                强制更新
              </el-tag>
              <el-tag v-if="isLatest(item)" type="success" size="small" effect="plain">
                最新版本
              </el-tag>
            </div>
            <div class="version-title">{{ item.title }}</div>
            <div class="version-meta">
              <span>{{ item.publishedAt }}</span>
              <template v-if="item.fileSizeBytes">
                <i></i>
                <span>{{ formatFileSize(item.fileSizeBytes) }}</span>
              </template>
              <template v-if="item.packageName">
                <i></i>
                <span class="package-name">{{ item.packageName }}</span>
              </template>
            </div>
            <div v-if="item.changelog" class="version-changelog">{{ item.changelog }}</div>
          </div>
          <el-button
            type="primary"
            plain
            size="small"
            :loading="downloadingId === item.id"
            :disabled="!item.downloadable"
            @click="handleDownload(item)"
          >
            下载
          </el-button>
        </div>
      </template>
      <el-empty v-else-if="!loading" description="暂无可用版本" :image-size="72" />
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
  import { computed, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import axios from 'axios'

  interface VersionItem {
    id: number
    version: string
    title: string
    changelog: string
    packageName: string
    sourceType: string
    fileSizeBytes: number
    fileMd5: string
    forceUpdate: boolean
    publishedAt: string
    downloadable: boolean
  }

  const props = defineProps<{
    /** 接口前缀：/api/user-panel 或 /api/agent-panel */
    apiPrefix: string
    /** 本地存储 token 键名 */
    tokenKey: string
  }>()

  const visible = ref(false)
  const loading = ref(false)
  const downloadingId = ref(0)
  const versions = ref<VersionItem[]>([])
  const appName = ref('')
  const licenseId = ref(0)

  const latestId = computed(() => versions.value[0]?.id)

  function isLatest(item: VersionItem) {
    return item.id === latestId.value
  }

  function authHeaders() {
    return { Authorization: `Bearer ${localStorage.getItem(props.tokenKey) || ''}` }
  }

  async function open(row: { id: number; appName?: string }) {
    licenseId.value = Number(row.id)
    appName.value = row.appName || ''
    versions.value = []
    visible.value = true
    loading.value = true
    try {
      const { data } = await axios.get(`${props.apiPrefix}/licenses/${licenseId.value}/versions`, {
        headers: authHeaders()
      })
      if (data.code === 200) {
        versions.value = data.data.list || []
        appName.value = data.data.appName || appName.value
      } else {
        ElMessage.error(data.msg || '加载版本列表失败')
      }
    } catch (error: any) {
      ElMessage.error(error?.response?.data?.msg || '加载版本列表失败')
    } finally {
      loading.value = false
    }
  }

  function absoluteDownloadUrl(downloadUrl: string) {
    try {
      return new URL(downloadUrl, window.location.origin).toString()
    } catch {
      return downloadUrl
    }
  }

  async function handleDownload(item: VersionItem) {
    if (downloadingId.value) return
    downloadingId.value = item.id
    try {
      const { data } = await axios.post(
        `${props.apiPrefix}/licenses/${licenseId.value}/versions/${item.id}/download-url`,
        {},
        { headers: authHeaders() }
      )
      if (data.code === 200 && data.data?.downloadUrl) {
        window.open(absoluteDownloadUrl(data.data.downloadUrl), '_blank', 'noopener,noreferrer')
      } else {
        ElMessage.error(data.msg || '生成下载地址失败')
      }
    } catch (error: any) {
      ElMessage.error(error?.response?.data?.msg || '生成下载地址失败')
    } finally {
      downloadingId.value = 0
    }
  }

  function formatFileSize(bytes: number) {
    const value = Number(bytes || 0)
    if (value < 1024) return `${value} B`
    if (value < 1024 * 1024) return `${(value / 1024).toFixed(2)} KB`
    return `${(value / 1024 / 1024).toFixed(2)} MB`
  }

  defineExpose({ open })
</script>

<style scoped lang="scss">
  .version-list {
    min-height: 160px;
    max-height: 480px;
    overflow-y: auto;
  }

  .version-item {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    padding: 14px 0;
    border-bottom: 1px solid var(--el-border-color-lighter);

    &:last-child {
      border-bottom: none;
    }

    .el-button {
      flex-shrink: 0;
      margin-top: 4px;
    }
  }

  .version-main {
    flex: 1;
    min-width: 0;
  }

  .version-title-row {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .version-number {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 15px;
    font-weight: 700;
    color: var(--el-color-primary);
  }

  .version-title {
    margin-top: 4px;
    font-size: 14px;
    font-weight: 600;
    color: var(--el-text-color-primary);
  }

  .version-meta {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 6px;
    font-size: 12px;
    color: var(--el-text-color-secondary);

    i {
      width: 3px;
      height: 3px;
      background: var(--el-border-color);
      border-radius: 50%;
    }

    .package-name {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      max-width: 260px;
    }
  }

  .version-changelog {
    margin-top: 8px;
    padding: 8px 10px;
    font-size: 12px;
    line-height: 1.6;
    color: var(--el-text-color-regular);
    background: var(--el-fill-color-light);
    border-radius: 6px;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    display: -webkit-box;
    -webkit-line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
</style>
