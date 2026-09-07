<template>
  <div class="license-dashboard">
    <!-- 统计卡片 -->
    <el-row :gutter="16" class="mb-4">
      <el-col :xs="12" :sm="6" v-for="item in statsCards" :key="item.title">
        <el-card shadow="never" class="art-card stats-card">
          <div class="stats-card-inner">
            <div class="stats-info">
              <span class="stats-title">{{ item.title }}</span>
              <span class="stats-value">{{ item.value }}</span>
            </div>
            <div class="stats-icon" :style="{ backgroundColor: item.bgColor, color: item.color }">
              <ArtSvgIcon :icon="item.icon" />
            </div>
          </div>
          <div class="stats-footer">
            <span :class="item.trend > 0 ? 'trend-up' : 'trend-down'">
              {{ item.trend > 0 ? '+' : '' }}{{ item.trend }}%
            </span>
            <span class="trend-label">较昨日</span>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 最近授权 + 到期提醒 -->
    <el-row :gutter="16">
      <el-col :xs="24" :lg="14">
        <el-card shadow="never" class="art-card dashboard-panel">
          <template #header>
            <span class="card-title">最近授权</span>
          </template>
          <el-table :data="recentLicenses" stripe>
            <el-table-column prop="domain" label="域名/IP" min-width="180" />
            <el-table-column prop="appName" label="应用" width="120" />
            <el-table-column prop="type" label="类型" width="100">
              <template #default="{ row }">
                <el-tag :type="typeTagMap[row.type]" size="small">{{ row.typeLabel }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="createdAt" label="授权时间" width="160" />
          </el-table>
        </el-card>
      </el-col>
      <el-col :xs="24" :lg="10">
        <el-card shadow="never" class="art-card dashboard-panel">
          <template #header>
            <span class="card-title">即将到期</span>
          </template>
          <div class="expire-list">
            <div v-for="item in expiringSoon" :key="item.id" class="expire-item">
              <div class="expire-info">
                <span class="expire-domain">{{ item.domain }}</span>
                <span class="expire-app">{{ item.appName }}</span>
              </div>
              <el-tag type="warning" size="small">{{ item.daysLeft }}天后到期</el-tag>
            </div>
            <el-empty v-if="expiringSoon.length === 0" description="暂无即将到期的授权" />
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
  import { ref, onMounted } from 'vue'
  import ArtSvgIcon from '@/components/core/base/art-svg-icon/index.vue'
  import request from '@/utils/http'

  interface StatsItem {
    title: string
    value: number
    trend: number
    icon: string
    color: string
    bgColor: string
  }

  interface RecentLicense {
    domain: string
    appName: string
    type: string
    typeLabel: string
    createdAt: string
  }

  interface ExpireItem {
    id: number
    domain: string
    appName: string
    daysLeft: number
  }

  interface DashboardData {
    stats: { title: string; value: number; trend: number }[]
    recentLicenses: RecentLicense[]
    expiringSoon: ExpireItem[]
  }

  const iconMap: Record<string, { icon: string; color: string; bgColor: string }> = {
    总授权数: { icon: 'ri-shield-keyhole-line', color: '#409eff', bgColor: '#ecf5ff' },
    活跃授权: { icon: 'ri-check-double-line', color: '#67c23a', bgColor: '#f0f9eb' },
    已过期: { icon: 'ri-time-line', color: '#e6a23c', bgColor: '#fdf6ec' },
    今日验证: { icon: 'ri-radar-line', color: '#909399', bgColor: '#f4f4f5' }
  }

  const statsCards = ref<StatsItem[]>([])
  const recentLicenses = ref<RecentLicense[]>([])
  const expiringSoon = ref<ExpireItem[]>([])

  const typeTagMap: Record<
    string,
    'primary' | 'success' | 'warning' | 'info' | 'danger' | undefined
  > = {
    domain: undefined,
    wildcard: 'success',
    ip: 'warning',
    key: 'info'
  } as const

  const fetchDashboard = async () => {
    try {
      const data = await request.get<DashboardData>({ url: '/api/license/dashboard' })
      statsCards.value = data.stats.map((s) => ({
        ...s,
        ...(iconMap[s.title] || {
          icon: 'ri-shield-keyhole-line',
          color: '#409eff',
          bgColor: '#ecf5ff'
        })
      }))
      recentLicenses.value = data.recentLicenses
      expiringSoon.value = data.expiringSoon
    } catch (e) {
      console.error('[LicenseDashboard] 加载失败:', e)
    }
  }

  onMounted(() => {
    fetchDashboard()
  })
</script>

<style scoped lang="scss">
  .license-dashboard {
    padding: 0;

    :deep(.el-card) {
      --el-card-border-color: var(--art-card-border);
      border-radius: calc(var(--custom-radius) + 4px);
      background: var(--default-box-color);
      box-shadow: none;
    }

    :deep(.el-card__header) {
      padding: 20px 22px 14px;
      border-bottom-color: var(--art-card-border);
    }

    :deep(.el-card__body) {
      padding: 20px 22px;
    }
  }

  .stats-card {
    min-height: 154px;
    transition:
      border-color 0.2s ease,
      transform 0.2s ease;

    &:hover {
      border-color: color-mix(in srgb, var(--art-primary) 30%, var(--art-card-border));
      transform: translateY(-2px);
    }

    .stats-card-inner {
      display: flex;
      align-items: center;
      justify-content: space-between;
    }

    .stats-info {
      display: flex;
      flex-direction: column;
      gap: 8px;
    }

    .stats-title {
      font-size: 13px;
      color: var(--art-gray-600);
    }

    .stats-value {
      font-size: 28px;
      font-weight: 700;
      line-height: 1.2;
      color: var(--art-gray-900);
    }

    .stats-icon {
      width: 44px;
      height: 44px;
      border-radius: 13px;
      display: flex;
      align-items: center;
      justify-content: center;
    }

    .stats-footer {
      display: flex;
      align-items: center;
      gap: 5px;
      margin-top: 16px;
      padding-top: 12px;
      border-top: 1px solid var(--art-card-border);
      font-size: 12px;

      .trend-up {
        color: var(--el-color-success);
        font-weight: 600;
      }

      .trend-down {
        color: var(--el-color-danger);
        font-weight: 600;
      }

      .trend-label {
        color: var(--art-gray-500);
      }
    }
  }

  .dashboard-panel {
    height: 100%;

    :deep(.el-table) {
      --el-table-border-color: var(--art-card-border);
      --el-table-header-bg-color: var(--art-gray-100);
      --el-table-row-hover-bg-color: var(--art-gray-100);
      color: var(--art-gray-800);
    }

    :deep(.el-table th.el-table__cell) {
      color: var(--art-gray-600);
      font-weight: 600;
    }
  }

  .mb-4 {
    margin-bottom: 16px;
  }

  .card-title {
    font-size: 16px;
    font-weight: 700;
    color: var(--art-gray-900);
  }

  .expire-list {
    .expire-item {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 13px 0;
      border-bottom: 1px solid var(--art-card-border);

      &:last-child {
        border-bottom: none;
      }
    }

    .expire-info {
      display: flex;
      flex-direction: column;
      min-width: 0;
      gap: 5px;
      margin-right: 12px;
    }

    .expire-domain {
      overflow: hidden;
      font-size: 14px;
      font-weight: 600;
      color: var(--art-gray-800);
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .expire-app {
      overflow: hidden;
      font-size: 12px;
      color: var(--art-gray-500);
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  @media (max-width: 768px) {
    .license-dashboard {
      :deep(.el-card__header),
      :deep(.el-card__body) {
        padding-right: 16px;
        padding-left: 16px;
      }
    }
  }
</style>
