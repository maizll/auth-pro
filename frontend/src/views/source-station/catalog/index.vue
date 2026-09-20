<template>
  <div class="source-station-page">
    <el-card shadow="never" class="art-card mb-4">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">公开软件源目录</span>
            <p class="card-hint">
              软件源按应用隔离。消费者在「软件源管理」为对应应用添加
              <code>{{ publicIndexUrl }}</code>
              （兼容 <code>?app_key=</code>）。未带 app_key 的
              <code>/software-source/index.json</code> 返回空目录，不会串应用。公开清单仍按分类拆出
              <code>plugins</code> 与 <code>homeTemplates</code>。
            </p>
          </div>
          <div class="table-actions">
            <el-button @click="loadIndex">刷新预览</el-button>
            <el-button type="primary" :loading="regenerating" @click="handleRegenerate"
              >从数据库重生快照</el-button
            >
          </div>
        </div>
      </template>
      <el-form inline class="mb-4">
        <el-form-item label="应用">
          <el-select
            v-model="selectedAppId"
            placeholder="请选择应用"
            style="width: 260px"
            @change="onAppChange"
          >
            <el-option
              v-for="app in apps"
              :key="app.id"
              :label="app.enabled ? `${app.name}（${app.appKey}）` : `${app.name}（${app.appKey}）· 已停用`"
              :value="app.id"
            />
          </el-select>
        </el-form-item>
      </el-form>
      <el-descriptions :column="3" border>
        <el-descriptions-item label="源名称">{{ indexData?.name || '-' }}</el-descriptions-item>
        <el-descriptions-item label="应用">{{ selectedAppHint }}</el-descriptions-item>
        <el-descriptions-item label="插件类（兼容 plugins）">{{
          indexData?.pluginCount ?? 0
        }}</el-descriptions-item>
        <el-descriptions-item label="模板类（兼容 homeTemplates）">{{
          indexData?.templateCount ?? 0
        }}</el-descriptions-item>
        <el-descriptions-item label="快照生成人">
          {{ indexData?.snapshot?.generatedBy || '-' }}
        </el-descriptions-item>
        <el-descriptions-item label="快照时间" :span="2">
          {{ snapshotTime }}
        </el-descriptions-item>
      </el-descriptions>
      <pre class="index-json" v-loading="loading">{{ prettyLive }}</pre>
    </el-card>

    <el-card shadow="never" class="art-card">
      <template #header>
        <span class="card-title">审计日志</span>
      </template>
      <el-table :data="audit" stripe v-loading="auditLoading">
        <el-table-column prop="createdAt" label="时间" width="180" />
        <el-table-column prop="actorName" label="操作人" width="120" />
        <el-table-column prop="action" label="动作" width="140" />
        <el-table-column prop="targetType" label="对象类型" width="110" />
        <el-table-column prop="targetId" label="对象" min-width="160" show-overflow-tooltip />
        <el-table-column prop="detail" label="详情" min-width="220" show-overflow-tooltip />
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import {
    fetchSourceAudit,
    fetchSourceCatalogApps,
    fetchSourceIndex,
    regenerateSourceIndex,
    type SourceAuditEntry,
    type SourceCatalogApp,
    type SourceIndexData
  } from '@/api/source-station'

  const APP_STORAGE_KEY = 'source-station-catalog-app-id'
  const loading = ref(false)
  const regenerating = ref(false)
  const auditLoading = ref(false)
  const indexData = ref<SourceIndexData | null>(null)
  const audit = ref<SourceAuditEntry[]>([])
  const apps = ref<SourceCatalogApp[]>([])
  const selectedAppId = ref(0)

  const selectedApp = computed(
    () => apps.value.find((item) => item.id === selectedAppId.value) || null
  )
  const selectedAppHint = computed(() => {
    const app = selectedApp.value
    if (!app) return '-'
    return `${app.name}（${app.appKey}）`
  })
  const publicIndexUrl = computed(() => {
    const app = selectedApp.value
    const path = app?.indexUrl || (app?.appKey ? `/software-source/${app.appKey}/index.json` : '/software-source/{app_key}/index.json')
    return `${window.location.origin}${path}`
  })
  const prettyLive = computed(() => JSON.stringify(indexData.value?.live || {}, null, 2))
  const snapshotTime = computed(() => {
    const raw = indexData.value?.snapshot?.generatedAt
    if (!raw || raw.startsWith('0001-')) return '尚未生成'
    return raw
  })

  async function loadApps() {
    const data = await fetchSourceCatalogApps()
    apps.value = data.list || []
    const stored = Number(localStorage.getItem(APP_STORAGE_KEY) || 0)
    selectedAppId.value = apps.value.some((item) => item.id === stored)
      ? stored
      : apps.value[0]?.id || 0
    if (selectedAppId.value) {
      localStorage.setItem(APP_STORAGE_KEY, String(selectedAppId.value))
    }
  }

  function onAppChange() {
    if (selectedAppId.value) {
      localStorage.setItem(APP_STORAGE_KEY, String(selectedAppId.value))
    }
    loadIndex()
  }

  async function loadIndex() {
    if (!selectedAppId.value) {
      indexData.value = null
      return
    }
    loading.value = true
    try {
      indexData.value = await fetchSourceIndex(selectedAppId.value)
    } finally {
      loading.value = false
    }
  }

  async function loadAudit() {
    auditLoading.value = true
    try {
      const data = await fetchSourceAudit(100)
      audit.value = data.list || []
    } finally {
      auditLoading.value = false
    }
  }

  async function handleRegenerate() {
    if (!selectedAppId.value) {
      ElMessage.warning('请先选择应用')
      return
    }
    regenerating.value = true
    try {
      indexData.value = await regenerateSourceIndex(selectedAppId.value)
      ElMessage.success('已从数据库重新生成公开目录')
      await loadAudit()
    } finally {
      regenerating.value = false
    }
  }

  onMounted(async () => {
    await loadApps()
    await Promise.all([loadIndex(), loadAudit()])
  })
</script>

<style scoped lang="scss">
  .source-station-page {
    padding-bottom: 8px;

    :deep(.el-card) {
      --el-card-border-color: var(--art-card-border);
      border-radius: calc(var(--custom-radius) + 4px);
      background: var(--default-box-color);
      box-shadow: none;
    }
  }

  .mb-4 {
    margin-bottom: 16px;
  }

  .table-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
  }

  .table-actions {
    display: flex;
    gap: 8px;
  }

  .card-title {
    font-size: 16px;
    font-weight: 700;
    color: var(--art-gray-900);
  }

  .card-hint {
    margin: 6px 0 0;
    font-size: 13px;
    color: var(--art-gray-600);
    line-height: 1.5;
  }

  .index-json {
    margin: 16px 0 0;
    max-height: 420px;
    overflow: auto;
    padding: 12px 14px;
    border-radius: 8px;
    background: var(--art-gray-100);
    font-size: 12px;
    line-height: 1.5;
  }
</style>
