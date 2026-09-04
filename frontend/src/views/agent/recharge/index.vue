<!-- 代理商财务流水页面 -->
<!-- art-full-height 自动计算出页面剩余高度 -->
<!-- art-table-card 一个符合系统样式的 class，同时自动撑满剩余高度 -->
<template>
  <div class="agent-recharge-page art-full-height">
    <!-- 统计卡片 -->
    <div class="stats-cards">
      <div class="art-card stat-card">
        <span class="stat-label">总充值</span>
        <strong class="stat-value text-primary">¥{{ stats.totalRecharge.toFixed(2) }}</strong>
      </div>
      <div class="art-card stat-card">
        <span class="stat-label">总消费</span>
        <strong class="stat-value text-danger">¥{{ stats.totalConsume.toFixed(2) }}</strong>
      </div>
      <div class="art-card stat-card">
        <span class="stat-label">本月充值</span>
        <strong class="stat-value text-success">¥{{ stats.monthRecharge.toFixed(2) }}</strong>
      </div>
      <div class="art-card stat-card">
        <span class="stat-label">本月消费</span>
        <strong class="stat-value text-warning">¥{{ stats.monthConsume.toFixed(2) }}</strong>
      </div>
    </div>

    <!-- 搜索栏 -->
    <RechargeSearch v-model="searchForm" @search="handleSearch" @reset="resetSearchParams" />

    <ElCard class="art-table-card" shadow="never">
      <!-- 表格头部 -->
      <ArtTableHeader v-model:columns="columnChecks" :loading="loading" @refresh="refreshAll" />

      <!-- 表格 -->
      <ArtTable
        :loading="loading"
        :data="data"
        :columns="columns"
        :pagination="pagination"
        @pagination:size-change="handleSizeChange"
        @pagination:current-change="handleCurrentChange"
      >
        <!-- 流水号 -->
        <template #orderNo="{ row }">
          <span class="mono">{{ row.orderNo }}</span>
        </template>

        <!-- 类型 -->
        <template #typeLabel="{ row }">
          <ElTag :type="typeTagMap[row.type]" size="small">{{ row.typeLabel }}</ElTag>
        </template>

        <!-- 金额 -->
        <template #amount="{ row }">
          <span :class="row.type === 'consume' ? 'text-danger' : 'text-success'">
            {{ row.type === 'consume' ? '-' : '+' }}¥{{ Number(row.amount || 0).toFixed(2) }}
          </span>
        </template>

        <!-- 余额 -->
        <template #balanceAfter="{ row }">
          ¥{{ Number(row.balanceAfter || 0).toFixed(2) }}
        </template>
      </ArtTable>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
  import { useTable } from '@/hooks/core/useTable'
  import {
    fetchTransactionList,
    fetchTransactionStats,
    type TransactionSearchParams
  } from '@/api/agent-manage'
  import RechargeSearch from './modules/recharge-search.vue'

  defineOptions({ name: 'AgentRecharge' })

  interface TransactionSearchForm {
    agentId?: string
    type?: string
    dateRange?: [string, string]
  }

  // 统计数据
  const stats = reactive({
    totalRecharge: 0,
    totalConsume: 0,
    monthRecharge: 0,
    monthConsume: 0
  })

  // 搜索表单
  const searchForm = ref<TransactionSearchForm>({
    agentId: undefined,
    type: undefined,
    dateRange: undefined
  })

  const typeTagMap: Record<
    string,
    'primary' | 'success' | 'warning' | 'info' | 'danger' | undefined
  > = {
    recharge: 'success',
    consume: 'danger',
    refund: 'warning',
    transfer: 'info',
    bonus: 'success'
  } as const

  const {
    columns,
    columnChecks,
    data,
    loading,
    pagination,
    getData,
    replaceSearchParams,
    resetSearchParams,
    handleSizeChange,
    handleCurrentChange,
    refreshData
  } = useTable({
    // 核心配置
    core: {
      apiFn: fetchTransactionList,
      apiParams: {
        page: 1,
        pageSize: 20
      },
      // 后端流水接口使用 page / pageSize 分页字段
      paginationKey: {
        current: 'page',
        size: 'pageSize'
      },
      columnsFactory: () => [
        { type: 'index', width: 60, label: '序号' }, // 序号
        {
          prop: 'orderNo',
          label: '流水号',
          minWidth: 180,
          showOverflowTooltip: true,
          useSlot: true
        },
        { prop: 'agentName', label: '代理商', width: 120 },
        { prop: 'typeLabel', label: '类型', width: 90, align: 'center', useSlot: true },
        { prop: 'amount', label: '金额(元)', width: 120, align: 'right', useSlot: true },
        { prop: 'balanceAfter', label: '余额(元)', width: 110, align: 'right', useSlot: true },
        { prop: 'remark', label: '备注', minWidth: 150, showOverflowTooltip: true },
        { prop: 'createdAt', label: '时间', width: 170 }
      ]
    },
    // 数据处理
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
  const fetchStats = async () => {
    try {
      const data = await fetchTransactionStats()
      Object.assign(stats, data)
    } catch {
      return
    }
  }

  /**
   * 搜索处理：时间范围映射为 startDate / endDate
   */
  const handleSearch = (params: TransactionSearchForm) => {
    const { dateRange, ...rest } = params
    const query: Partial<TransactionSearchParams> = { ...rest }
    if (dateRange && dateRange.length === 2) {
      query.startDate = dateRange[0]
      query.endDate = dateRange[1]
    }
    replaceSearchParams(query)
    getData()
  }

  /**
   * 刷新统计数据与列表
   */
  const refreshAll = () => {
    fetchStats()
    refreshData()
  }

  onMounted(fetchStats)
</script>

<style scoped lang="scss">
  .agent-recharge-page {
    .stats-cards {
      display: grid;
      grid-template-columns: repeat(4, 1fr);
      gap: 16px;
      margin-bottom: 12px;

      @media (width <= 992px) {
        grid-template-columns: repeat(2, 1fr);
      }
    }

    .stat-card {
      display: flex;
      align-items: center;
      justify-content: space-between;
      min-height: 64px;
      padding: 16px 20px;
    }

    .stat-label {
      font-size: 13px;
      color: var(--el-text-color-secondary);
    }

    .stat-value {
      font-size: 22px;
      color: var(--el-text-color-primary);

      &.text-primary {
        color: var(--el-color-primary);
      }

      &.text-success {
        color: var(--el-color-success);
      }

      &.text-danger {
        color: var(--el-color-danger);
      }

      &.text-warning {
        color: var(--el-color-warning);
      }
    }

    .mono {
      font-family: 'Roboto Mono', monospace;
    }

    .text-success {
      color: var(--el-color-success);
    }

    .text-danger {
      color: var(--el-color-danger);
    }
  }
</style>
