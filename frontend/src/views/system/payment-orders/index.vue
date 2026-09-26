<!-- 订单列表页面 -->
<!-- art-full-height 自动计算出页面剩余高度 -->
<!-- art-table-card 一个符合系统样式的 class，同时自动撑满剩余高度 -->
<template>
  <div class="payment-orders-page art-full-height">
    <!-- 搜索栏 -->
    <OrderSearch v-model="searchForm" @search="handleSearch" @reset="resetSearchParams" />

    <ElCard class="art-table-card" shadow="never">
      <CommercialReissueBanner />
      <ElAlert
        class="commercial-paid"
        :closable="false"
        type="info"
        :title="`商业版已支付合计 ¥${commercialPaidYuan.toFixed(2)}`"
      />
      <!-- 表格头部 -->
      <ArtTableHeader v-model:columns="columnChecks" :loading="loading" @refresh="refreshData" />

      <!-- 表格 -->
      <ArtTable
        :loading="loading"
        :data="data"
        :columns="columns"
        :pagination="pagination"
        @pagination:size-change="handleSizeChange"
        @pagination:current-change="handleCurrentChange"
      >
        <!-- 订单号 -->
        <template #orderNo="{ row }">
          <span class="mono">{{ row.orderNo }}</span>
        </template>

        <!-- 订单类型 -->
        <template #subjectType="{ row }">
          <ElTag :type="subjectTagTypes[row.subjectType] || 'info'" size="small">
            {{ subjectLabels[row.subjectType] || row.subjectType }}
          </ElTag>
        </template>

        <!-- 下单主体 -->
        <template #subjectName="{ row }">
          <span v-if="row.subjectType === 'test'">支付测试</span>
          <span v-else>{{ row.subjectName || `#${row.subjectId}` }}</span>
        </template>

        <!-- 金额 -->
        <template #amount="{ row }">
          <span class="amount-text">¥{{ Number(row.amount || 0).toFixed(2) }}</span>
        </template>

        <!-- 支付方式 -->
        <template #payMethod="{ row }">
          {{ payMethodLabel(row.payMethod) }}
        </template>

        <!-- 支付状态 -->
        <template #status="{ row }">
          <ElTag :type="statusTagTypes[row.status] || 'info'" size="small">
            {{ statusLabels[row.status] || row.status }}
          </ElTag>
        </template>

        <!-- 支付时间 -->
        <template #paidAt="{ row }">
          {{ row.paidAt || '-' }}
        </template>

        <!-- 操作 -->
        <template #operation="{ row }">
          <ElButton link type="primary" @click="openDetail(row)">详情</ElButton>
        </template>
      </ArtTable>
    </ElCard>

    <!-- 订单详情弹窗 -->
    <ElDialog v-model="detailVisible" title="订单详情" width="560px">
      <ElDescriptions v-if="current" :column="1" border>
        <ElDescriptionsItem label="订单号">
          <span class="mono">{{ current.orderNo }}</span>
        </ElDescriptionsItem>
        <ElDescriptionsItem label="订单类型">
          {{ subjectLabels[current.subjectType] || current.subjectType }}
        </ElDescriptionsItem>
        <ElDescriptionsItem label="下单主体">
          {{
            current.subjectType === 'test'
              ? '支付测试'
              : current.subjectName || `#${current.subjectId}`
          }}
        </ElDescriptionsItem>
        <ElDescriptionsItem label="订单金额">
          ¥{{ Number(current.amount || 0).toFixed(2) }}
        </ElDescriptionsItem>
        <ElDescriptionsItem label="实付金额">
          {{ current.paidAmount ? `¥${Number(current.paidAmount).toFixed(2)}` : '-' }}
        </ElDescriptionsItem>
        <ElDescriptionsItem label="支付方式">
          {{ payMethodLabel(current.payMethod) }}
        </ElDescriptionsItem>
        <ElDescriptionsItem label="支付渠道">{{ current.payChannel || '-' }}</ElDescriptionsItem>
        <ElDescriptionsItem label="支付状态">
          <ElTag :type="statusTagTypes[current.status] || 'info'" size="small">
            {{ statusLabels[current.status] || current.status }}
          </ElTag>
        </ElDescriptionsItem>
        <ElDescriptionsItem label="网关交易号">
          {{ current.gatewayTradeNo || '-' }}
        </ElDescriptionsItem>
        <ElDescriptionsItem label="备注">{{ current.remark || '-' }}</ElDescriptionsItem>
        <ElDescriptionsItem label="创建时间">{{ current.createdAt || '-' }}</ElDescriptionsItem>
        <ElDescriptionsItem label="支付时间">{{ current.paidAt || '-' }}</ElDescriptionsItem>
      </ElDescriptions>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { useTable } from '@/hooks/core/useTable'
  import {
    fetchPaymentOrderList,
    type PaymentOrderItem,
    type PaymentOrderSearchParams
  } from '@/api/system-manage'
  import OrderSearch from './modules/order-search.vue'
  import CommercialReissueBanner from '@/components/business/commercial/CommercialReissueBanner.vue'

  defineOptions({ name: 'PaymentOrders' })

  type OrderSearchFormParams = Partial<
    Pick<PaymentOrderSearchParams, 'orderNo' | 'subjectType' | 'payMethod' | 'status'>
  >

  // 订单详情弹窗
  const detailVisible = ref(false)
  const current = ref<PaymentOrderItem>()

  // 搜索表单
  const searchForm = ref<OrderSearchFormParams>({
    orderNo: undefined,
    subjectType: undefined,
    payMethod: undefined,
    status: undefined
  })

  const commercialPaidYuan = ref(0)

  const subjectLabels: Record<string, string> = {
    user: '用户充值',
    agent: '代理充值',
    test: '支付测试',
    store_edition: '商业版',
    store_plugin: '付费插件',
    store_template: '付费模板'
  }

  const subjectTagTypes: Record<string, 'primary' | 'success' | 'warning' | 'info'> = {
    user: 'primary',
    agent: 'success',
    test: 'warning'
  }

  const statusLabels: Record<string, string> = {
    pending: '待支付',
    paid: '已支付',
    failed: '失败',
    cancelled: '已取消',
    refunded: '已退款'
  }

  const statusTagTypes: Record<string, 'success' | 'warning' | 'danger' | 'info'> = {
    pending: 'warning',
    paid: 'success',
    failed: 'danger',
    cancelled: 'info'
  }

  const payMethodOptions = [
    { label: '支付宝', value: 'alipay' },
    { label: '微信支付', value: 'wxpay' },
    { label: 'QQ 钱包', value: 'qqpay' },
    { label: '余额支付', value: 'balance' },
    { label: '配额支付', value: 'quota' }
  ]

  const payMethodLabel = (value: string) => {
    return payMethodOptions.find((item) => item.value === value)?.label || value || '-'
  }

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
      apiFn: async (params: PaymentOrderSearchParams) => {
        const result = await fetchPaymentOrderList(params)
        commercialPaidYuan.value = Number(result?.commercialPaidYuan || 0)
        return result
      },
      apiParams: {
        page: 1,
        pageSize: 20,
        ...searchForm.value
      },
      // 后端订单接口使用 page / pageSize 分页字段
      paginationKey: {
        current: 'page',
        size: 'pageSize'
      },
      columnsFactory: () => [
        { type: 'index', width: 60, label: '序号' }, // 序号
        {
          prop: 'orderNo',
          label: '订单号',
          minWidth: 190,
          showOverflowTooltip: true,
          useSlot: true
        },
        {
          prop: 'subjectType',
          label: '订单类型',
          width: 100,
          align: 'center',
          useSlot: true
        },
        {
          prop: 'subjectName',
          label: '下单主体',
          minWidth: 150,
          showOverflowTooltip: true,
          useSlot: true
        },
        {
          prop: 'amount',
          label: '金额',
          width: 110,
          align: 'right',
          useSlot: true
        },
        {
          prop: 'payMethod',
          label: '支付方式',
          width: 100,
          align: 'center',
          useSlot: true
        },
        {
          prop: 'status',
          label: '支付状态',
          width: 100,
          align: 'center',
          useSlot: true
        },
        {
          prop: 'createdAt',
          label: '创建时间',
          width: 170
        },
        {
          prop: 'paidAt',
          label: '支付时间',
          width: 170,
          useSlot: true
        },
        {
          prop: 'operation',
          label: '操作',
          width: 90,
          fixed: 'right',
          useSlot: true
        }
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
   * 搜索处理
   */
  const handleSearch = (params: OrderSearchFormParams) => {
    replaceSearchParams(params as Partial<PaymentOrderSearchParams>)
    getData()
  }

  /**
   * 打开订单详情
   */
  const openDetail = (row: PaymentOrderItem) => {
    current.value = row
    detailVisible.value = true
  }
</script>

<style scoped lang="scss">
  .payment-orders-page {
    .commercial-paid {
      margin-bottom: 12px;
    }

    .mono {
      font-family: 'Roboto Mono', monospace;
    }

    .amount-text {
      font-weight: 600;
      color: var(--art-gray-900);
    }
  }
</style>
