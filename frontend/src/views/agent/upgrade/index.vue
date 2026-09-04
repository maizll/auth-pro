<!-- 代理升级审计页面：追踪余额开通订单、账户主体转换、余额与授权迁移结果 -->
<!-- 当前页面仅提供查询，不会重新执行转换 -->
<template>
  <div class="agent-upgrade-page art-full-height">
    <!-- 统计卡片 -->
    <div class="stats-cards">
      <div class="art-card stat-card">
        <span class="stat-label">升级订单</span>
        <strong class="stat-value">{{ stats.totalOrders }}</strong>
      </div>
      <div class="art-card stat-card">
        <span class="stat-label">待处理</span>
        <strong class="stat-value text-warning">{{ stats.pendingOrders }}</strong>
      </div>
      <div class="art-card stat-card">
        <span class="stat-label">完成转换</span>
        <strong class="stat-value text-success">{{ stats.completedConversions }}</strong>
      </div>
      <div class="art-card stat-card">
        <span class="stat-label">开通费合计</span>
        <strong class="stat-value text-primary">¥{{ money(stats.completedAmount) }}</strong>
      </div>
      <div class="art-card stat-card">
        <span class="stat-label">迁移余额</span>
        <strong class="stat-value">¥{{ money(stats.transferredBalance) }}</strong>
      </div>
      <div class="art-card stat-card">
        <span class="stat-label">失败订单</span>
        <strong class="stat-value text-danger">{{ stats.failedOrders }}</strong>
      </div>
    </div>

    <ElCard class="art-table-card" shadow="never">
      <ElTabs v-model="activeTab" class="audit-tabs" @tab-change="handleTabChange">
        <!-- 升级订单 -->
        <ElTabPane label="升级订单" name="orders">
          <UpgradeOrderSearch
            v-model="orderSearchForm"
            @search="handleOrderSearch"
            @reset="resetOrderSearchParams"
          />

          <ArtTableHeader
            v-model:columns="orderColumnChecks"
            :loading="orderLoading"
            @refresh="refreshOrders"
          />

          <ArtTable
            :loading="orderLoading"
            :data="orderData"
            :columns="orderColumns"
            :pagination="orderPagination"
            @pagination:size-change="handleOrderSizeChange"
            @pagination:current-change="handleOrderCurrentChange"
          >
            <!-- 升级订单号 -->
            <template #orderNo="{ row }">
              <span class="mono">{{ row.orderNo }}</span>
            </template>

            <!-- 原用户 -->
            <template #user="{ row }">
              <div class="subject-cell">
                <strong>{{ row.userName || row.userEmail }}</strong>
                <span>#{{ row.userId }} · {{ row.userEmail }}</span>
              </div>
            </template>

            <!-- 代理等级 -->
            <template #level="{ row }">
              <div class="subject-cell">
                <strong>{{ row.levelName }}</strong>
                <span>{{ row.discount }} 折</span>
              </div>
            </template>

            <!-- 开通费 -->
            <template #amount="{ row }">
              <span class="amount">¥{{ money(row.amount) }}</span>
            </template>

            <!-- 开通赠送 -->
            <template #openingBonus="{ row }">
              <span :class="{ amount: row.openingBonus > 0 }"
                >+ ¥{{ money(row.openingBonus) }}</span
              >
            </template>

            <!-- 支付渠道 -->
            <template #payChannel="{ row }">
              {{ payChannelLabel(row.payChannel) }}
            </template>

            <!-- 状态 -->
            <template #status="{ row }">
              <ElTag :type="orderStatusType(row.status)" size="small">
                {{ orderStatusLabel(row.status) }}
              </ElTag>
            </template>

            <!-- 目标代理 -->
            <template #agent="{ row }">
              {{ row.agentId ? `${row.agentName || '代理'} #${row.agentId}` : '-' }}
            </template>

            <!-- 失败原因 -->
            <template #errorMessage="{ row }">
              <span :class="{ 'error-text': row.errorMessage }">{{ row.errorMessage || '-' }}</span>
            </template>

            <!-- 完成时间 -->
            <template #completedAt="{ row }">
              {{ row.completedAt || '-' }}
            </template>
          </ArtTable>
        </ElTabPane>

        <!-- 转换记录 -->
        <ElTabPane label="转换记录" name="conversions">
          <ConversionSearch
            v-model="conversionSearchForm"
            @search="handleConversionSearch"
            @reset="resetConversionSearchParams"
          />

          <ArtTableHeader
            v-model:columns="conversionColumnChecks"
            :loading="conversionLoading"
            @refresh="refreshConversions"
          />

          <ArtTable
            :loading="conversionLoading"
            :data="conversionData"
            :columns="conversionColumns"
            :pagination="conversionPagination"
            @pagination:size-change="handleConversionSizeChange"
            @pagination:current-change="handleConversionCurrentChange"
          >
            <!-- 转换流水号 -->
            <template #conversionNo="{ row }">
              <span class="mono">{{ row.conversionNo }}</span>
            </template>

            <!-- 升级订单号 -->
            <template #orderNo="{ row }">
              <span class="mono">{{ row.orderNo }}</span>
            </template>

            <!-- 主体迁移 -->
            <template #migration="{ row }">
              <div class="migration-cell">
                <span>用户 #{{ row.userId }} · {{ row.userEmail }}</span>
                <ArtSvgIcon icon="ri:arrow-right-line" class="migration-arrow" />
                <span>代理 #{{ row.agentId }} · {{ row.agentName || row.agentEmail }}</span>
              </div>
            </template>

            <!-- 开通费 -->
            <template #openingFee="{ row }"> ¥{{ money(row.openingFee) }} </template>

            <!-- 迁移余额 -->
            <template #transferredBalance="{ row }">
              <span class="amount">¥{{ money(row.transferredBalance) }}</span>
            </template>

            <!-- 开通赠送 -->
            <template #openingBonus="{ row }"> + ¥{{ money(row.openingBonus) }} </template>

            <!-- 代理余额 -->
            <template #finalBalance="{ row }">
              <span class="amount">¥{{ money(row.finalBalance) }}</span>
            </template>

            <!-- 状态 -->
            <template #status="{ row }">
              <ElTag :type="conversionStatusType(row.status)" size="small">
                {{ conversionStatusLabel(row.status) }}
              </ElTag>
            </template>

            <!-- 异常原因 -->
            <template #errorMessage="{ row }">
              <span :class="{ 'error-text': row.errorMessage }">{{ row.errorMessage || '-' }}</span>
            </template>

            <!-- 完成时间 -->
            <template #completedAt="{ row }">
              {{ row.completedAt || '-' }}
            </template>

            <!-- 操作 -->
            <template #operation="{ row }">
              <ElButton link type="primary" @click="openConversionDetail(row)">详情</ElButton>
            </template>
          </ArtTable>
        </ElTabPane>
      </ElTabs>
    </ElCard>

    <!-- 转换审计快照弹窗 -->
    <ElDialog v-model="detailVisible" title="账户转换审计快照" width="680px">
      <ElDescriptions v-if="detail" :column="1" border>
        <ElDescriptionsItem label="转换流水号">
          <span class="mono">{{ detail.conversionNo }}</span>
        </ElDescriptionsItem>
        <ElDescriptionsItem label="升级订单号">
          <span class="mono">{{ detail.orderNo }}</span>
        </ElDescriptionsItem>
        <ElDescriptionsItem label="转换状态">
          {{ conversionStatusLabel(detail.status) }}
        </ElDescriptionsItem>
        <ElDescriptionsItem label="异常原因">
          {{ detail.errorMessage || '-' }}
        </ElDescriptionsItem>
        <ElDescriptionsItem label="原用户快照">
          <pre>{{ snapshotText(detail.sourceSnapshot) }}</pre>
        </ElDescriptionsItem>
        <ElDescriptionsItem label="转换结果快照">
          <pre>{{ snapshotText(detail.resultSnapshot) }}</pre>
        </ElDescriptionsItem>
      </ElDescriptions>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import ArtSvgIcon from '@/components/core/base/art-svg-icon/index.vue'
  import { useTable } from '@/hooks/core/useTable'
  import {
    fetchAgentUpgradeStats,
    fetchAgentUpgradeOrders,
    fetchAgentUpgradeConversions,
    fetchAgentUpgradeConversionDetail,
    type AgentUpgradeStats,
    type AccountConversionItem,
    type ConversionDetail
  } from '@/api/agent-manage'
  import UpgradeOrderSearch from './modules/upgrade-order-search.vue'
  import ConversionSearch from './modules/conversion-search.vue'

  defineOptions({ name: 'AgentUpgrade' })

  type TagType = 'primary' | 'success' | 'warning' | 'info' | 'danger'

  interface UpgradeOrderSearchForm {
    keyword?: string
    status?: string
    payChannel?: string
  }

  interface ConversionSearchForm {
    keyword?: string
    status?: string
  }

  const emptyStats: AgentUpgradeStats = {
    totalOrders: 0,
    pendingOrders: 0,
    completedOrders: 0,
    failedOrders: 0,
    completedAmount: 0,
    completedConversions: 0,
    transferredBalance: 0,
    openingBonus: 0,
    migratedLicenses: 0
  }

  const activeTab = ref('orders')
  const stats = reactive<AgentUpgradeStats>({ ...emptyStats })
  const detailVisible = ref(false)
  const detail = ref<ConversionDetail>()
  /** 转换记录是否已加载过（保持懒加载语义） */
  const conversionsLoaded = ref(false)

  // 搜索表单
  const orderSearchForm = ref<UpgradeOrderSearchForm>({
    keyword: undefined,
    status: undefined,
    payChannel: undefined
  })
  const conversionSearchForm = ref<ConversionSearchForm>({
    keyword: undefined,
    status: undefined
  })

  const money = (value: unknown) => Number(value || 0).toFixed(2)

  const orderStatusLabel = (status: string) => {
    return (
      {
        pending: '待支付',
        paid: '已支付',
        processing: '转换中',
        completed: '已完成',
        failed: '失败',
        cancelled: '已取消'
      }[status] || status
    )
  }

  const orderStatusType = (status: string): TagType => {
    return (
      (
        {
          pending: 'warning',
          paid: 'primary',
          processing: 'warning',
          completed: 'success',
          failed: 'danger',
          cancelled: 'info'
        } as Record<string, TagType>
      )[status] || 'info'
    )
  }

  const conversionStatusLabel = (status: string) => {
    return { processing: '处理中', completed: '已完成', failed: '失败' }[status] || status
  }

  const conversionStatusType = (status: string): TagType => {
    return (
      (
        { processing: 'warning', completed: 'success', failed: 'danger' } as Record<string, TagType>
      )[status] || 'info'
    )
  }

  const payChannelLabel = (channel: string) => {
    if (channel === 'balance') return '用户余额'
    return channel || '-'
  }

  const snapshotText = (value: unknown) => {
    if (value === null || value === undefined) return '-'
    if (typeof value === 'string') return value
    return JSON.stringify(value, null, 2)
  }

  // ==================== 升级订单表格 ====================

  const {
    columns: orderColumns,
    columnChecks: orderColumnChecks,
    data: orderData,
    loading: orderLoading,
    pagination: orderPagination,
    getData: getOrderData,
    replaceSearchParams: replaceOrderSearchParams,
    resetSearchParams: resetOrderSearchParams,
    handleSizeChange: handleOrderSizeChange,
    handleCurrentChange: handleOrderCurrentChange,
    refreshData: refreshOrderData
  } = useTable({
    core: {
      apiFn: fetchAgentUpgradeOrders,
      apiParams: {
        page: 1,
        pageSize: 20,
        ...orderSearchForm.value
      },
      paginationKey: {
        current: 'page',
        size: 'pageSize'
      },
      columnsFactory: () => [
        { type: 'index', width: 60, label: '序号' }, // 序号
        {
          prop: 'orderNo',
          label: '升级订单号',
          minWidth: 190,
          showOverflowTooltip: true,
          useSlot: true
        },
        { prop: 'user', label: '原用户', minWidth: 190, showOverflowTooltip: true, useSlot: true },
        { prop: 'level', label: '代理等级', minWidth: 130, useSlot: true },
        { prop: 'amount', label: '开通费', width: 110, align: 'right', useSlot: true },
        { prop: 'openingBonus', label: '开通赠送', width: 110, align: 'right', useSlot: true },
        { prop: 'payChannel', label: '支付渠道', width: 100, align: 'center', useSlot: true },
        { prop: 'status', label: '状态', width: 100, align: 'center', useSlot: true },
        {
          prop: 'agent',
          label: '目标代理',
          minWidth: 130,
          showOverflowTooltip: true,
          useSlot: true
        },
        {
          prop: 'errorMessage',
          label: '失败原因',
          minWidth: 180,
          showOverflowTooltip: true,
          useSlot: true
        },
        { prop: 'createdAt', label: '创建时间', width: 165 },
        { prop: 'completedAt', label: '完成时间', width: 165, useSlot: true }
      ]
    },
    transform: {
      dataTransformer: (records) => {
        if (!Array.isArray(records)) {
          return []
        }
        return records
      }
    }
  })

  // ==================== 转换记录表格 ====================

  const {
    columns: conversionColumns,
    columnChecks: conversionColumnChecks,
    data: conversionData,
    loading: conversionLoading,
    pagination: conversionPagination,
    getData: getConversionData,
    replaceSearchParams: replaceConversionSearchParams,
    resetSearchParams: resetConversionSearchParams,
    handleSizeChange: handleConversionSizeChange,
    handleCurrentChange: handleConversionCurrentChange,
    refreshData: refreshConversionData
  } = useTable({
    core: {
      // 转换记录切到对应 Tab 时才加载
      immediate: false,
      apiFn: fetchAgentUpgradeConversions,
      apiParams: {
        page: 1,
        pageSize: 20,
        ...conversionSearchForm.value
      },
      paginationKey: {
        current: 'page',
        size: 'pageSize'
      },
      columnsFactory: () => [
        { type: 'index', width: 60, label: '序号' }, // 序号
        {
          prop: 'conversionNo',
          label: '转换流水号',
          minWidth: 190,
          showOverflowTooltip: true,
          useSlot: true
        },
        {
          prop: 'orderNo',
          label: '升级订单号',
          minWidth: 190,
          showOverflowTooltip: true,
          useSlot: true
        },
        { prop: 'migration', label: '主体迁移', minWidth: 240, useSlot: true },
        { prop: 'levelName', label: '代理等级', width: 120 },
        { prop: 'openingFee', label: '开通费', width: 110, align: 'right', useSlot: true },
        {
          prop: 'transferredBalance',
          label: '迁移余额',
          width: 110,
          align: 'right',
          useSlot: true
        },
        { prop: 'openingBonus', label: '开通赠送', width: 110, align: 'right', useSlot: true },
        { prop: 'finalBalance', label: '代理余额', width: 110, align: 'right', useSlot: true },
        { prop: 'migratedLicenseCount', label: '迁移授权', width: 100, align: 'center' },
        { prop: 'status', label: '状态', width: 100, align: 'center', useSlot: true },
        {
          prop: 'errorMessage',
          label: '异常原因',
          minWidth: 170,
          showOverflowTooltip: true,
          useSlot: true
        },
        { prop: 'completedAt', label: '完成时间', width: 165, useSlot: true },
        {
          prop: 'operation',
          label: '操作',
          width: 90,
          fixed: 'right',
          useSlot: true
        }
      ]
    },
    transform: {
      dataTransformer: (records) => {
        if (!Array.isArray(records)) {
          return []
        }
        return records
      }
    }
  })

  /**
   * 加载统计数据
   */
  const loadStats = async () => {
    try {
      const data = await fetchAgentUpgradeStats()
      Object.assign(stats, emptyStats, data || {})
    } catch {
      return
    }
  }

  /**
   * 升级订单搜索
   */
  const handleOrderSearch = (params: UpgradeOrderSearchForm) => {
    replaceOrderSearchParams(params)
    getOrderData()
  }

  /**
   * 转换记录搜索
   */
  const handleConversionSearch = (params: ConversionSearchForm) => {
    conversionsLoaded.value = true
    replaceConversionSearchParams(params)
    getConversionData()
  }

  /**
   * 刷新升级订单（同时刷新统计）
   */
  const refreshOrders = () => {
    loadStats()
    refreshOrderData()
  }

  /**
   * 刷新转换记录（同时刷新统计）
   */
  const refreshConversions = () => {
    loadStats()
    refreshConversionData()
  }

  const handleTabChange = (name: string | number) => {
    if (name === 'conversions' && !conversionsLoaded.value) {
      conversionsLoaded.value = true
      getConversionData()
    }
  }

  const openConversionDetail = async (row: AccountConversionItem) => {
    detail.value = await fetchAgentUpgradeConversionDetail(row.id)
    detailVisible.value = true
  }

  onMounted(loadStats)
</script>

<style scoped lang="scss">
  .agent-upgrade-page {
    .stats-cards {
      display: grid;
      grid-template-columns: repeat(6, 1fr);
      gap: 16px;
      margin-bottom: 12px;

      @media (width <= 1400px) {
        grid-template-columns: repeat(3, 1fr);
      }

      @media (width <= 768px) {
        grid-template-columns: repeat(2, 1fr);
      }
    }

    .stat-card {
      display: flex;
      flex-direction: column;
      gap: 8px;
      justify-content: center;
      min-height: 64px;
      padding: 14px 20px;
    }

    .stat-label {
      font-size: 13px;
      color: var(--el-text-color-secondary);
    }

    .stat-value {
      font-size: 22px;
      line-height: 1;
      color: var(--el-text-color-primary);

      &.text-primary {
        color: var(--el-color-primary);
      }

      &.text-success {
        color: var(--el-color-success);
      }

      &.text-warning {
        color: var(--el-color-warning);
      }

      &.text-danger {
        color: var(--el-color-danger);
      }
    }

    // Tab 结构撑满表格卡片，保证 ArtTable 高度自适应
    .audit-tabs {
      display: flex;
      flex-direction: column;
      height: 100%;

      :deep(.el-tabs__content) {
        flex: 1;
        overflow: hidden;
      }

      :deep(.el-tab-pane) {
        display: flex;
        flex-direction: column;
        height: 100%;

        .art-search-bar {
          padding: 0;
          margin-bottom: 12px;
          background: transparent;
          border: none;
        }
      }
    }

    .subject-cell,
    .migration-cell {
      display: flex;
      flex-direction: column;
      gap: 3px;

      span {
        font-size: 12px;
        color: var(--el-text-color-secondary);
      }
    }

    .migration-arrow {
      color: var(--el-color-primary);
    }

    .mono {
      font-family: 'Roboto Mono', monospace;
      font-size: 12px;
    }

    .amount {
      font-weight: 600;
      color: var(--el-color-success);
    }

    .error-text {
      color: var(--el-color-danger);
    }

    pre {
      max-height: 260px;
      padding: 12px;
      margin: 0;
      overflow: auto;
      font-family: 'Roboto Mono', monospace;
      font-size: 12px;
      line-height: 1.6;
      white-space: pre-wrap;
      background: var(--el-fill-color-light);
      border-radius: 6px;
    }
  }
</style>
