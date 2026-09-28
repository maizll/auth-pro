<!-- 源站设置。安装包位置改到存储管理，这里只保留旧应用软件源地址。 -->
<template>
  <div class="source-station-page">
    <el-card shadow="never" class="art-card">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">安装包存储</span>
            <p class="card-hint">
              私有仓库和对象存储已改到「存储管理」。可以设主存储和备用，密钥加密保存，页面不能查看。
            </p>
          </div>
          <div class="table-actions">
            <el-button type="primary" @click="goStorage">打开存储管理</el-button>
          </div>
        </div>
      </template>
      <el-alert
        type="info"
        :closable="false"
        show-icon
        title="原来的发布仓库和收费仓库会自动变成存储列表里的两条。"
      />
    </el-card>

    <el-card v-loading="aliasLoading" shadow="never" class="art-card alias-card">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">旧应用软件源地址</span>
            <p class="card-hint">
              1.6.1 之前已经删掉的应用，可以在这里把旧标识转到还在使用的应用。旧地址
              /software-source/旧标识/index.json
              会返回目标应用的目录，已经订阅的客户端不用改。同一个旧标识再保存一次会更新目标，不会多出一条。正在使用的应用标识不能转走，请先归档。
            </p>
          </div>
          <div class="table-actions">
            <el-button type="primary" :loading="aliasSaving" @click="handleAliasSave"
              >保存映射</el-button
            >
          </div>
        </div>
      </template>
      <el-form label-width="140px" class="settings-form">
        <el-form-item label="旧应用标识">
          <el-input v-model.trim="aliasForm.oldAppKey" placeholder="例如 app_f93896d80066_5811" />
        </el-form-item>
        <el-form-item label="转到应用">
          <el-select
            v-model="aliasForm.targetAppId"
            placeholder="选择还在使用的应用"
            class="product-app-select"
          >
            <el-option
              v-for="app in liveAliasApps"
              :key="app.id"
              :label="`${app.name}（${app.appKey}）`"
              :value="app.id"
            />
          </el-select>
          <p v-if="!liveAliasApps.length" class="field-hint">还没有可接收地址的应用。</p>
        </el-form-item>
      </el-form>
      <el-table :data="aliases" size="small" empty-text="还没有旧地址映射" @row-click="fillAlias">
        <el-table-column prop="oldAppKey" label="旧标识" min-width="180" show-overflow-tooltip />
        <el-table-column label="目标应用" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            {{ row.targetName || '应用' }}（{{ row.targetAppKey || row.targetAppId }}）
          </template>
        </el-table-column>
        <el-table-column
          prop="indexUrl"
          label="仍可用的地址"
          min-width="220"
          show-overflow-tooltip
        />
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, reactive, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import { useRouter } from 'vue-router'
  import {
    fetchSoftwareSourceAliases,
    fetchSourceCatalogApps,
    saveSoftwareSourceAlias,
    type SoftwareSourceAlias,
    type SourceCatalogApp
  } from '@/api/source-station'

  const router = useRouter()
  const aliasLoading = ref(false)
  const aliasSaving = ref(false)
  const aliases = ref<SoftwareSourceAlias[]>([])
  const aliasApps = ref<SourceCatalogApp[]>([])
  const aliasForm = reactive({
    oldAppKey: '',
    targetAppId: undefined as number | undefined
  })
  const liveAliasApps = computed(() => aliasApps.value.filter((app) => !app.archived))

  function goStorage() {
    router.push('/source-station/storage')
  }

  function fillAlias(row: SoftwareSourceAlias) {
    aliasForm.oldAppKey = row.oldAppKey
    aliasForm.targetAppId = row.targetAppId
  }

  async function loadAliases() {
    aliasLoading.value = true
    try {
      const [aliasData, appData] = await Promise.all([
        fetchSoftwareSourceAliases(),
        fetchSourceCatalogApps()
      ])
      aliases.value = aliasData.list || []
      aliasApps.value = appData.list || []
    } finally {
      aliasLoading.value = false
    }
  }

  async function handleAliasSave() {
    const oldAppKey = aliasForm.oldAppKey.trim()
    if (!oldAppKey || !aliasForm.targetAppId) {
      ElMessage.info('请填写旧应用标识，并选择要转到的应用')
      return
    }
    aliasSaving.value = true
    try {
      await saveSoftwareSourceAlias({ oldAppKey, targetAppId: aliasForm.targetAppId })
      await loadAliases()
    } finally {
      aliasSaving.value = false
    }
  }

  onMounted(() => {
    void loadAliases()
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

  .table-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
  }

  .table-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
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
    max-width: 720px;
  }

  .alias-card {
    margin-top: 16px;
  }

  .settings-form {
    max-width: 560px;
  }

  .product-app-select {
    width: 100%;
  }

  .field-hint {
    margin: 6px 0 0;
    font-size: 12px;
    line-height: 1.5;
    color: var(--art-gray-600);
  }

  @media (max-width: 767px) {
    .settings-form {
      max-width: none;

      :deep(.el-form-item) {
        display: block;
      }

      :deep(.el-form-item__label) {
        width: auto !important;
        height: auto;
        justify-content: flex-start;
        margin-bottom: 4px;
      }

      :deep(.el-form-item__content) {
        margin-left: 0 !important;
      }
    }
  }
</style>
