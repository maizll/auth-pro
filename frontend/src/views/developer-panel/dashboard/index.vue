<template>
  <div v-loading="loading" class="developer-dashboard">
    <div class="art-card p-6 mb-5 welcome-card">
      <div class="welcome-content">
        <div>
          <h2 class="welcome-title">欢迎回来，{{ profile.displayName || profile.username || '开发者' }}</h2>
          <p class="welcome-desc">
            当前可查看已登记的插件与首页模板。源站不存储源码，仅管理元数据与审核状态。
          </p>
        </div>
        <iconify-icon icon="ri:code-s-slash-line" width="56" color="var(--el-color-primary-light-5)" />
      </div>
    </div>

    <div class="stats-row">
      <div class="art-card p-5 stat-card">
        <div class="stat-icon" style="background: var(--el-color-primary-light-9)">
          <iconify-icon icon="ri:puzzle-2-line" width="22" color="var(--el-color-primary)" />
        </div>
        <div class="stat-info">
          <span class="stat-num">{{ plugins.length }}</span>
          <span class="stat-label">插件</span>
        </div>
      </div>
      <div class="art-card p-5 stat-card">
        <div class="stat-icon" style="background: var(--el-color-success-light-9)">
          <iconify-icon icon="ri:layout-3-line" width="22" color="var(--el-color-success)" />
        </div>
        <div class="stat-info">
          <span class="stat-num">{{ templates.length }}</span>
          <span class="stat-label">首页模板</span>
        </div>
      </div>
      <div class="art-card p-5 stat-card">
        <div class="stat-icon" style="background: var(--el-color-warning-light-9)">
          <iconify-icon icon="ri:user-3-line" width="22" color="var(--el-color-warning)" />
        </div>
        <div class="stat-info">
          <span class="stat-num">{{ profile.username || '-' }}</span>
          <span class="stat-label">开发者账号</span>
        </div>
      </div>
    </div>

    <el-card shadow="never" class="mb-5">
      <template #header>
        <span class="card-title">我的插件</span>
      </template>
      <el-empty v-if="!plugins.length" description="暂无插件草稿。审核通过后，可在此查看已提交的目录项。" />
      <el-table v-else :data="plugins" stripe>
        <el-table-column prop="id" label="标识" min-width="140" show-overflow-tooltip />
        <el-table-column prop="name" label="名称" min-width="140" show-overflow-tooltip />
        <el-table-column prop="version" label="版本" width="110" />
        <el-table-column label="状态" width="110" align="center">
          <template #default="{ row }">
            <el-tag :type="statusMeta(row.status).type" size="small">
              {{ statusMeta(row.status).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="updatedAt" label="更新时间" width="180" />
      </el-table>
    </el-card>

    <el-card shadow="never">
      <template #header>
        <span class="card-title">我的首页模板</span>
      </template>
      <el-empty v-if="!templates.length" description="暂无模板草稿。审核通过后，可在此查看已提交的目录项。" />
      <el-table v-else :data="templates" stripe>
        <el-table-column prop="id" label="标识" min-width="140" show-overflow-tooltip />
        <el-table-column prop="name" label="名称" min-width="140" show-overflow-tooltip />
        <el-table-column prop="version" label="版本" width="110" />
        <el-table-column label="状态" width="110" align="center">
          <template #default="{ row }">
            <el-tag :type="statusMeta(row.status).type" size="small">
              {{ statusMeta(row.status).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="updatedAt" label="更新时间" width="180" />
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, reactive, ref } from 'vue'
  import { useRouter } from 'vue-router'
  import { Icon as IconifyIcon } from '@iconify/vue'
  import { ElMessage } from 'element-plus'
  import { SOURCE_ITEM_STATUS } from '@/api/source-station'
  import {
    DEVELOPER_INFO_KEY,
    DEVELOPER_TOKEN_KEY,
    fetchSourceDeveloperItems,
    fetchSourceDeveloperMe,
    type SourceDeveloperCatalogItem,
    type SourceDeveloperProfile
  } from '@/api/source-developer'

  const router = useRouter()
  const loading = ref(false)
  const plugins = ref<SourceDeveloperCatalogItem[]>([])
  const templates = ref<SourceDeveloperCatalogItem[]>([])
  const profile = reactive<SourceDeveloperProfile>({
    id: 0,
    username: '',
    displayName: '',
    email: '',
    roleCode: ''
  })

  function statusMeta(value: string) {
    return SOURCE_ITEM_STATUS[value] || { label: value || '-', type: 'info' as const }
  }

  function logoutToLogin() {
    localStorage.removeItem(DEVELOPER_TOKEN_KEY)
    localStorage.removeItem(DEVELOPER_INFO_KEY)
    router.replace('/developer-panel/login')
  }

  async function loadDashboard() {
    loading.value = true
    try {
      const [meRes, itemsRes] = await Promise.all([
        fetchSourceDeveloperMe(),
        fetchSourceDeveloperItems()
      ])
      if (meRes.status === 401 || itemsRes.status === 401) {
        logoutToLogin()
        return
      }
      if (meRes.data.code === 401 || itemsRes.data.code === 401) {
        logoutToLogin()
        return
      }
      if (meRes.data.code !== 200) {
        ElMessage.error(meRes.data.msg || '加载开发者信息失败')
        return
      }
      Object.assign(profile, meRes.data.data || {})
      localStorage.setItem(
        DEVELOPER_INFO_KEY,
        JSON.stringify({
          username: profile.username,
          displayName: profile.displayName
        })
      )
      if (itemsRes.data.code === 200) {
        plugins.value = itemsRes.data.data?.plugins || []
        templates.value = itemsRes.data.data?.homeTemplates || []
      }
    } catch (error: unknown) {
      const status = (error as { response?: { status?: number } })?.response?.status
      if (status === 401) {
        logoutToLogin()
        return
      }
      ElMessage.error('加载开发者工作台失败')
    } finally {
      loading.value = false
    }
  }

  onMounted(loadDashboard)
</script>

<style scoped lang="scss">
  .welcome-card {
    background:
      radial-gradient(circle at 92% 12%, rgb(64 158 255 / 12%), transparent 26%),
      var(--el-bg-color);
  }

  .welcome-content {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
  }

  .welcome-title {
    margin: 0 0 6px;
    font-size: 22px;
  }

  .welcome-desc {
    margin: 0;
    font-size: 13px;
    line-height: 1.6;
    color: var(--el-text-color-secondary);
  }

  .stats-row {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 16px;
    margin-bottom: 20px;
  }

  .stat-card {
    display: flex;
    gap: 14px;
    align-items: center;
  }

  .stat-icon {
    display: grid;
    place-items: center;
    width: 44px;
    height: 44px;
    border-radius: 12px;
  }

  .stat-info {
    display: flex;
    min-width: 0;
    flex-direction: column;
  }

  .stat-num {
    overflow: hidden;
    font-size: 20px;
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .stat-label {
    margin-top: 2px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
  }

  .card-title {
    font-weight: 600;
  }

  @media (width <= 800px) {
    .stats-row {
      grid-template-columns: 1fr;
    }
  }
</style>
