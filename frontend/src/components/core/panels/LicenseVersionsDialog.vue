<!-- 授权版本下载弹层：用户端 / 代理端共用 -->
<template>
  <el-dialog
    v-model="visible"
    width="680px"
    destroy-on-close
    class="license-versions-dialog"
    :show-close="false"
  >
    <template #header>
      <div class="dialog-header">
        <div class="dialog-header-icon">
          <iconify-icon icon="ri:archive-stack-line" width="20" />
        </div>
        <div class="dialog-header-text">
          <span class="dialog-title">版本下载</span>
          <span v-if="appName" class="dialog-subtitle">{{ appName }}</span>
        </div>
        <button class="dialog-close" type="button" aria-label="关闭" @click="visible = false">
          <iconify-icon icon="ri:close-line" width="16" />
        </button>
      </div>
    </template>

    <div v-loading="loading" class="version-list">
      <template v-if="versions.length">
        <div
          v-for="item in versions"
          :key="item.id"
          class="version-item"
          :class="{ 'is-latest': isLatest(item) }"
        >
          <div class="version-icon">
            <iconify-icon icon="ri:file-zip-line" width="20" />
          </div>
          <div class="version-main">
            <div class="version-title-row">
              <span class="version-number">v{{ item.version }}</span>
              <el-tag v-if="isLatest(item)" type="success" size="small" effect="light" round>
                最新版本
              </el-tag>
              <el-tag v-if="item.forceUpdate" type="danger" size="small" effect="light" round>
                强制更新
              </el-tag>
            </div>
            <div v-if="item.title" class="version-title">{{ item.title }}</div>
            <div class="version-meta">
              <span class="meta-entry">
                <iconify-icon icon="ri:time-line" width="13" />
                {{ item.publishedAt }}
              </span>
              <span v-if="item.fileSizeBytes" class="meta-entry">
                <iconify-icon icon="ri:hard-drive-3-line" width="13" />
                {{ formatFileSize(item.fileSizeBytes) }}
              </span>
              <span v-if="item.packageName" class="meta-entry package-name">
                <iconify-icon icon="ri:file-list-line" width="13" />
                {{ item.packageName }}
              </span>
            </div>
            <div v-if="item.changelog" class="version-changelog">{{ item.changelog }}</div>
          </div>
          <el-button
            type="primary"
            :plain="!isLatest(item)"
            size="small"
            class="download-btn"
            :loading="downloadingId === item.id"
            :disabled="!item.downloadable"
            @click="handleDownload(item)"
          >
            <iconify-icon v-if="downloadingId !== item.id" icon="ri:download-2-line" width="15" />
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
  import { Icon as IconifyIcon } from '@iconify/vue'
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
  // 弹窗外壳：圆角、柔和阴影、头部底部分隔线
  :deep(.el-dialog) {
    border-radius: 16px;
    overflow: hidden;
    box-shadow: 0 24px 64px rgba(15, 23, 42, 0.16);
  }

  :deep(.el-dialog__header) {
    padding: 18px 20px 14px;
    margin-right: 0;
    border-bottom: 1px solid var(--el-border-color-extra-light);
  }

  :deep(.el-dialog__body) {
    padding: 16px 20px 20px;
  }

  .dialog-header {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .dialog-header-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 38px;
    height: 38px;
    border-radius: 11px;
    flex-shrink: 0;
    color: var(--el-color-primary);
    background: linear-gradient(
      135deg,
      var(--el-color-primary-light-8),
      var(--el-color-primary-light-9)
    );
  }

  .dialog-header-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .dialog-title {
    font-size: 15px;
    font-weight: 700;
    color: var(--el-text-color-primary);
  }

  .dialog-subtitle {
    font-size: 12px;
    font-weight: 400;
    color: var(--el-text-color-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .dialog-close {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    margin-left: auto;
    padding: 0;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    color: var(--el-text-color-secondary);
    background: transparent;
    transition: all 0.2s;

    &:hover {
      color: var(--el-text-color-primary);
      background: var(--el-fill-color);
    }
  }

  .version-list {
    min-height: 160px;
    max-height: 460px;
    overflow-y: auto;
  }

  .version-item {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    padding: 14px 16px;
    margin-bottom: 10px;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 12px;
    background: var(--el-bg-color);
    transition: all 0.2s ease;

    &:last-child {
      margin-bottom: 0;
    }

    &:hover {
      border-color: var(--el-color-primary-light-5);
      box-shadow: 0 6px 18px rgba(15, 23, 42, 0.06);
    }

    &.is-latest {
      border-color: var(--el-color-primary-light-7);
      background: linear-gradient(
        180deg,
        var(--el-color-primary-light-9) 0%,
        var(--el-bg-color) 90%
      );
    }
  }

  .version-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 40px;
    border-radius: 10px;
    flex-shrink: 0;
    margin-top: 2px;
    color: var(--el-color-primary);
    background: var(--el-color-primary-light-9);
  }

  .version-item.is-latest .version-icon {
    color: #fff;
    background: linear-gradient(135deg, var(--el-color-primary), var(--el-color-primary-light-3));
    box-shadow: 0 4px 12px rgba(64, 158, 255, 0.28);
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
    font-family: 'DIN Alternate', 'Roboto Mono', monospace;
    font-size: 16px;
    font-weight: 800;
    color: var(--el-text-color-primary);
  }

  .version-title {
    margin-top: 3px;
    font-size: 13px;
    font-weight: 500;
    color: var(--el-text-color-regular);
  }

  .version-meta {
    display: flex;
    align-items: center;
    gap: 14px;
    flex-wrap: wrap;
    margin-top: 7px;
    font-size: 12px;
    color: var(--el-text-color-secondary);

    .meta-entry {
      display: inline-flex;
      align-items: center;
      gap: 4px;
      min-width: 0;
    }

    .package-name {
      max-width: 240px;

      span,
      & {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }
    }
  }

  .version-changelog {
    margin-top: 9px;
    padding: 8px 12px;
    font-size: 12px;
    line-height: 1.7;
    color: var(--el-text-color-regular);
    background: var(--el-fill-color-light);
    border-radius: 8px;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    display: -webkit-box;
    -webkit-line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .download-btn {
    flex-shrink: 0;
    margin-top: 6px;
    display: inline-flex;
    align-items: center;
    gap: 4px;
    border-radius: 8px;
    font-weight: 600;
  }

  @media (max-width: 768px) {
    :deep(.el-dialog) {
      width: calc(100vw - 24px) !important;
      margin-top: 8vh !important;
    }

    .version-item {
      flex-wrap: wrap;
    }

    .download-btn {
      width: 100%;
      justify-content: center;
      margin-top: 10px;
    }
  }
</style>
