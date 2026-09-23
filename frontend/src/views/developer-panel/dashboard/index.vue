<template>
  <div v-loading="loading" class="developer-dashboard">
    <div class="art-card p-6 mb-5 welcome-card">
      <div class="welcome-content">
        <div>
          <h2 class="welcome-title">欢迎回来，{{ profile.displayName || profile.username || '开发者' }}</h2>
          <p class="welcome-desc">
            在侧栏提交插件、首页模板或广告申请。目录按应用隔离。ZIP 可上传到本站，也可以登记外部 HTTPS；外链会在提交审核时校验。
          </p>
          <div class="welcome-actions">
            <el-button type="primary" @click="router.push('/developer-panel/plugins?create=1')">
              登记插件
            </el-button>
            <el-button @click="router.push('/developer-panel/templates?create=1')">登记模板</el-button>
            <el-button @click="router.push('/developer-panel/ads')">申请广告</el-button>
            <el-button text type="primary" @click="router.push('/developer-panel/guide')">
              开发文档
            </el-button>
          </div>
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
          <iconify-icon icon="ri:time-line" width="22" color="var(--el-color-warning)" />
        </div>
        <div class="stat-info">
          <span class="stat-num">{{ pendingCount }}</span>
          <span class="stat-label">待审核</span>
        </div>
      </div>
      <div class="art-card p-5 stat-card">
        <div class="stat-icon" style="background: var(--el-color-info-light-9)">
          <iconify-icon icon="ri:advertisement-line" width="22" color="var(--el-color-info)" />
        </div>
        <div class="stat-info">
          <span class="stat-num">{{ pendingAds }}</span>
          <span class="stat-label">广告申请待审</span>
        </div>
      </div>
    </div>

    <el-card shadow="never" class="mb-5">
      <template #header>
        <div class="card-header">
          <span class="card-title">我的插件</span>
          <el-button link type="primary" @click="router.push('/developer-panel/plugins')">
            管理
          </el-button>
        </div>
      </template>
      <el-empty v-if="!plugins.length" description="暂无插件草稿。审核通过后，可在此查看已提交的目录项。" />
      <el-table v-else :data="plugins" stripe>
        <el-table-column prop="id" label="标识" min-width="140" show-overflow-tooltip />
        <el-table-column prop="name" label="名称" min-width="140" show-overflow-tooltip />
        <el-table-column label="应用" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">{{ appLabel(row.appId) }}</template>
        </el-table-column>
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
        <div class="card-header">
          <span class="card-title">我的首页模板</span>
          <el-button link type="primary" @click="router.push('/developer-panel/templates')">
            管理
          </el-button>
        </div>
      </template>
      <el-empty v-if="!templates.length" description="暂无模板草稿。审核通过后，可在此查看已提交的目录项。" />
      <el-table v-else :data="templates" stripe>
        <el-table-column prop="id" label="标识" min-width="140" show-overflow-tooltip />
        <el-table-column prop="name" label="名称" min-width="140" show-overflow-tooltip />
        <el-table-column label="应用" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">{{ appLabel(row.appId) }}</template>
        </el-table-column>
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
  import { computed, onMounted, reactive, ref } from 'vue'
  import { useRouter } from 'vue-router'
  import { Icon as IconifyIcon } from '@iconify/vue'
  import { ElMessage } from 'element-plus'
  import { SOURCE_ITEM_STATUS } from '@/api/source-station'
  import {
    DEVELOPER_INFO_KEY,
    DEVELOPER_TOKEN_KEY,
    fetchSourceDeveloperAdApplications,
    fetchSourceDeveloperCatalogApps,
    fetchSourceDeveloperItems,
    fetchSourceDeveloperMe,
    type SourceDeveloperCatalogApp,
    type SourceDeveloperCatalogItem,
    type SourceDeveloperProfile
  } from '@/api/source-developer'

  const router = useRouter()
  const loading = ref(false)
  const plugins = ref<SourceDeveloperCatalogItem[]>([])
  const templates = ref<SourceDeveloperCatalogItem[]>([])
  const catalogApps = ref<SourceDeveloperCatalogApp[]>([])
  const pendingAds = ref(0)
  const profile = reactive<SourceDeveloperProfile>({
    id: 0,
    username: '',
    displayName: '',
    email: '',
    roleCode: ''
  })
  const pendingCount = computed(
    () =>
      plugins.value.filter((item) => item.status === 'review').length +
      templates.value.filter((item) => item.status === 'review').length
  )

  function statusMeta(value: string) {
    return SOURCE_ITEM_STATUS[value] || { label: value || '-', type: 'info' as const }
  }

  function appLabel(appId?: number) {
    if (!appId) return '-'
    const app = catalogApps.value.find((item) => item.id === appId)
    if (!app) return `应用 #${appId}`
    return `${app.name}（${app.appKey}）`
  }

  function logoutToLogin() {
    localStorage.removeItem(DEVELOPER_TOKEN_KEY)
    localStorage.removeItem(DEVELOPER_INFO_KEY)
    router.replace('/developer-panel/login')
  }

  async function loadDashboard() {
    loading.value = true
    try {
      const [meRes, itemsRes, appsRes, adsRes] = await Promise.all([
        fetchSourceDeveloperMe(),
        fetchSourceDeveloperItems(),
        fetchSourceDeveloperCatalogApps(),
        fetchSourceDeveloperAdApplications('pending')
      ])
      if (
        meRes.status === 401 ||
        itemsRes.status === 401 ||
        appsRes.status === 401 ||
        adsRes.status === 401
      ) {
        logoutToLogin()
        return
      }
      if (
        meRes.data.code === 401 ||
        itemsRes.data.code === 401 ||
        appsRes.data.code === 401 ||
        adsRes.data.code === 401
      ) {
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
      if (appsRes.data.code === 200) {
        catalogApps.value = appsRes.data.data?.list || []
      }
      if (itemsRes.data.code === 200) {
        plugins.value = itemsRes.data.data?.plugins || []
        templates.value = itemsRes.data.data?.homeTemplates || []
      }
      if (adsRes.data.code === 200) {
        pendingAds.value = adsRes.data.data?.total || adsRes.data.data?.list?.length || 0
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

  .welcome-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 16px;
  }

  .stats-row {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
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

  .card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  @media (width <= 1100px) {
    .stats-row {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  @media (width <= 800px) {
    .stats-row {
      grid-template-columns: 1fr;
    }
  }
</style>
