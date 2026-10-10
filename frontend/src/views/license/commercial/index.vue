<!-- 商业版：按应用查看商业版订单、授权和已绑定站点 -->
<template>
  <div class="license-commercial-page art-full-height">
    <ElAlert
      class="refund-policy"
      :closable="false"
      type="info"
      show-icon
      title="虚拟产品，付款后不退款。如需作废权益，请在授权列表吊销或作废商业版。"
    />
    <!-- 每个应用单独提示待补发的订单 -->
    <ElAlert
      v-for="gap in gapGroups"
      :key="gap.appId"
      class="gap-alert"
      type="error"
      :closable="false"
      show-icon
    >
      <template #title>
        <span>{{ gap.appName }} 有 {{ gap.count }} 笔已付款订单还没开通商业版。</span>
        <ElButton
          class="gap-btn"
          size="small"
          type="primary"
          :loading="reissuing === gap.appId"
          @click="reissue(gap)"
        >
          补发 {{ gap.appName }}
        </ElButton>
      </template>
    </ElAlert>

    <ElCard class="art-table-card" shadow="never">
      <div class="toolbar">
        <ElRadioGroup v-model="tab" @change="onFilterChange">
          <ElRadioButton value="orders">订单</ElRadioButton>
          <ElRadioButton value="licenses">授权</ElRadioButton>
          <ElRadioButton value="bindings">绑定站点</ElRadioButton>
        </ElRadioGroup>
        <div class="filters">
          <ElSelect
            v-model="appId"
            class="filter-app"
            placeholder="全部商业版应用"
            clearable
            :disabled="!apps.length"
            @change="onFilterChange"
          >
            <ElOption
              v-for="app in apps"
              :key="app.id"
              :label="app.mode === 'stopped' ? `${app.name}（已停售）` : app.name"
              :value="app.id"
            />
          </ElSelect>
          <ElSelect
            v-if="statusOptions.length"
            v-model="status"
            class="filter-status"
            placeholder="全部状态"
            clearable
            @change="onFilterChange"
          >
            <ElOption
              v-for="opt in statusOptions"
              :key="opt.value"
              :label="opt.label"
              :value="opt.value"
            />
          </ElSelect>
          <ElButton :loading="loading" @click="loadList">刷新</ElButton>
        </div>
      </div>

      <!-- 加载中 -->
      <ElSkeleton v-if="loading && !rows.length" :rows="6" animated class="state-box" />

      <!-- 出错 -->
      <ElAlert v-else-if="error" type="error" :closable="false" show-icon class="state-box">
        <template #title>
          <span>{{ error }}</span>
          <ElButton class="gap-btn" size="small" @click="reload">重试</ElButton>
        </template>
      </ElAlert>

      <!-- 还没有出售商业版的应用 -->
      <ElEmpty v-else-if="appsLoaded && !apps.length" class="state-box">
        <template #description>
          <span>还没有出售商业版的应用。去</span>
          <ElButton link type="primary" @click="router.push('/license/apps')">应用管理</ElButton>
          <span>打开「出售商业版」。</span>
        </template>
      </ElEmpty>

      <!-- 筛选后没有数据 -->
      <ElEmpty v-else-if="!loading && !rows.length" :description="emptyText" class="state-box" />

      <template v-else>
        <!-- 手机：卡片列表 -->
        <div v-if="narrow" v-loading="loading" class="card-list">
          <div v-for="row in rows" :key="rowKey(row)" class="item-card">
            <div class="item-head">
              <span class="app-tag" :class="appColor(row.appId)">{{ row.appName || '--' }}</span>
              <ElTag :type="statusTag(row).type" size="small">{{ statusTag(row).label }}</ElTag>
            </div>
            <div class="item-title">{{ rowTitle(row) }}</div>
            <div v-for="line in rowLines(row)" :key="line.label" class="item-line">
              <span class="item-label">{{ line.label }}</span>
              <span>{{ line.value }}</span>
            </div>
          </div>
        </div>

        <!-- 电脑：表格 -->
        <ElTable v-else v-loading="loading" :data="rows" :row-key="rowKey" class="data-table">
          <ElTableColumn label="应用" min-width="140">
            <template #default="{ row }">
              <span class="app-tag" :class="appColor(row.appId)">{{ row.appName || '--' }}</span>
            </template>
          </ElTableColumn>
          <ElTableColumn :label="titleLabel" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">{{ rowTitle(row) }}</template>
          </ElTableColumn>
          <ElTableColumn
            v-for="col in extraColumns"
            :key="col.label"
            :label="col.label"
            :min-width="col.width"
            show-overflow-tooltip
          >
            <template #default="{ row }">{{ col.value(row) }}</template>
          </ElTableColumn>
          <ElTableColumn label="状态" width="100" align="center">
            <template #default="{ row }">
              <ElTag :type="statusTag(row).type" size="small">{{ statusTag(row).label }}</ElTag>
            </template>
          </ElTableColumn>
        </ElTable>

        <ElPagination
          v-if="total > size"
          class="pager"
          :layout="narrow ? 'prev, pager, next' : 'total, prev, pager, next'"
          :total="total"
          :page-size="size"
          :current-page="page"
          :pager-count="narrow ? 5 : 7"
          @current-change="onPage"
        />
      </template>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
  import { ElMessage } from 'element-plus'
  import { useRouter } from 'vue-router'
  import { formatLocalDateTime } from '@/utils/local-time'
  import { useNarrowScreen } from '@/hooks/core/useNarrowScreen'
  import {
    fetchLicenseAppList,
    fetchCommercialPurchaseGaps,
    reissueCommercialPurchases,
    fetchStoreAdminOrders,
    fetchStoreAdminLicenses,
    fetchStoreAdminBindings,
    type AppCommercialMode,
    type StoreAdminOrder,
    type StoreAdminLicense,
    type StoreAdminBinding,
    type StoreAdminQuery
  } from '@/api/license-manage'

  defineOptions({ name: 'LicenseCommercial' })

  type Tab = 'orders' | 'licenses' | 'bindings'
  type Row = StoreAdminOrder | StoreAdminLicense | StoreAdminBinding
  type TagType = 'primary' | 'success' | 'info' | 'danger'

  interface CommercialApp {
    id: number
    name: string
    mode: AppCommercialMode
  }

  interface GapGroup {
    appId: number
    appName: string
    count: number
  }

  const router = useRouter()
  const narrow = useNarrowScreen()

  const tab = ref<Tab>('orders')
  const appId = ref<number>()
  const status = ref<string>()
  const page = ref(1)
  const size = 20
  const total = ref(0)
  const rows = ref<Row[]>([])
  const loading = ref(false)
  const error = ref('')
  const apps = ref<CommercialApp[]>([])
  const appsLoaded = ref(false)
  const gapGroups = ref<GapGroup[]>([])
  const reissuing = ref(0)

  const statusText: Record<Tab, Record<string, [string, TagType]>> = {
    orders: {
      pending: ['待付款', 'info'],
      paid: ['已付款', 'success'],
      refunded: ['已退款', 'danger'],
      expired: ['已过期', 'info'],
      closed: ['已关闭', 'info']
    },
    licenses: {
      active: ['生效中', 'success'],
      expired: ['已过期', 'info'],
      revoked: ['已作废', 'danger'],
      disabled: ['已停用', 'danger']
    },
    bindings: {
      active: ['正常', 'success'],
      revoked: ['已撤销', 'danger'],
      replaced: ['已更换', 'info']
    }
  }

  // 授权接口不支持按状态筛选，只给订单和绑定显示状态下拉。
  const statusOptions = computed(() =>
    tab.value === 'licenses'
      ? []
      : Object.entries(statusText[tab.value]).map(([value, [label]]) => ({ value, label }))
  )

  const titleLabel = computed(
    () => ({ orders: '订单', licenses: '授权', bindings: '站点' })[tab.value]
  )

  const emptyText = computed(
    () =>
      ({
        orders: '还没有商业版订单',
        licenses: '还没有商业版授权',
        bindings: '还没有绑定的站点'
      })[tab.value]
  )

  // 应用标签只用蓝、紫、绿三种颜色轮换。
  const appColor = (id: number) => {
    const index = apps.value.findIndex((app) => app.id === id)
    return ['tag-blue', 'tag-purple', 'tag-green'][(index < 0 ? id : index) % 3]
  }

  const money = (cents: number) => `¥${(cents / 100).toFixed(2)}`
  const day = (value?: string) => formatLocalDateTime(value, false) || '--'
  const periodText = (value?: string) =>
    ({ permanent: '永久', yearly: '按年', monthly: '按月' })[value || ''] || value || '--'

  const rowKey = (row: Row) =>
    'orderNo' in row ? row.orderNo : 'bindingId' in row ? row.bindingId : String(row.id)

  const rowTitle = (row: Row) => {
    if ('orderNo' in row) return row.title || row.orderNo
    if ('bindingId' in row) return row.domain || row.bindingId
    return row.licenseNo
  }

  const statusTag = (row: Row): { label: string; type: TagType } => {
    const value =
      'licenseStatus' in row
        ? row.editionStatus || row.licenseStatus
        : (row as StoreAdminOrder).status
    const hit = statusText[tab.value][value]
    return hit ? { label: hit[0], type: hit[1] } : { label: value || '--', type: 'info' }
  }

  interface Column {
    label: string
    width: number
    value: (row: Row) => string
  }

  const extraColumns = computed<Column[]>(() => {
    if (tab.value === 'orders') {
      return [
        { label: '订单号', width: 190, value: (r) => (r as StoreAdminOrder).orderNo },
        { label: '金额', width: 100, value: (r) => money((r as StoreAdminOrder).amountCents) },
        { label: '站点', width: 160, value: (r) => r.domain || '--' },
        {
          label: '时间',
          width: 150,
          value: (r) => day((r as StoreAdminOrder).paidAt || (r as StoreAdminOrder).createdAt)
        }
      ]
    }
    if (tab.value === 'licenses') {
      return [
        { label: '站点', width: 160, value: (r) => r.domain || '--' },
        { label: '周期', width: 90, value: (r) => periodText((r as StoreAdminLicense).period) },
        {
          label: '到期',
          width: 150,
          value: (r) => day((r as StoreAdminLicense).editionExpireAt) || '永久'
        }
      ]
    }
    return [
      { label: '授权码', width: 170, value: (r) => (r as StoreAdminBinding).licenseNo || '--' },
      { label: '版本', width: 90, value: (r) => (r as StoreAdminBinding).appVersion || '--' },
      { label: '最近在线', width: 150, value: (r) => day((r as StoreAdminBinding).lastSeenAt) }
    ]
  })

  const rowLines = (row: Row) =>
    extraColumns.value.map((col) => ({ label: col.label, value: col.value(row) }))

  async function loadApps() {
    try {
      const res = await fetchLicenseAppList()
      apps.value = (res || [])
        .filter((app) => app.commercial && app.commercial.mode !== 'off')
        .map((app) => ({ id: app.id, name: app.name, mode: app.commercial!.mode }))
    } catch {
      apps.value = []
    } finally {
      appsLoaded.value = true
    }
  }

  async function loadGaps() {
    try {
      const res = await fetchCommercialPurchaseGaps(0)
      const groups = new Map<number, GapGroup>()
      for (const order of res?.orders || []) {
        const group = groups.get(order.appId) || {
          appId: order.appId,
          appName: order.appName,
          count: 0
        }
        group.count++
        groups.set(order.appId, group)
      }
      gapGroups.value = [...groups.values()]
    } catch {
      gapGroups.value = []
    }
  }

  async function loadList() {
    loading.value = true
    error.value = ''
    const params: StoreAdminQuery = { page: page.value, size }
    if (appId.value) params.appId = appId.value
    if (status.value) params.status = status.value
    try {
      const api =
        tab.value === 'orders'
          ? fetchStoreAdminOrders({ ...params, kind: 'edition' })
          : tab.value === 'licenses'
            ? fetchStoreAdminLicenses(params)
            : fetchStoreAdminBindings(params)
      const res = await api
      rows.value = (res?.list || []) as Row[]
      total.value = res?.total || 0
    } catch (err) {
      rows.value = []
      total.value = 0
      error.value = err instanceof Error && err.message ? err.message : '加载失败，请稍后重试'
    } finally {
      loading.value = false
    }
  }

  function onFilterChange() {
    if (tab.value === 'licenses') status.value = undefined
    page.value = 1
    loadList()
  }

  function onPage(value: number) {
    page.value = value
    loadList()
  }

  async function reissue(gap: GapGroup) {
    reissuing.value = gap.appId
    try {
      const res = await reissueCommercialPurchases(gap.appId)
      ElMessage.success(res?.msg || `已补发 ${res?.granted || 0} 笔`)
      await Promise.all([loadGaps(), loadList()])
    } catch {
      // 出错提示由请求层弹出
    } finally {
      reissuing.value = 0
    }
  }

  async function reload() {
    await Promise.all([loadApps(), loadGaps(), loadList()])
  }

  onMounted(reload)
</script>

<style scoped lang="scss">
  .license-commercial-page {
    display: flex;
    flex-direction: column;
    gap: 12px;

    .gap-btn {
      margin-left: 12px;
    }

    .toolbar {
      display: flex;
      flex-wrap: wrap;
      gap: 12px;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 16px;
    }

    .filters {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
    }

    .filter-app {
      width: 200px;
    }

    .filter-status {
      width: 130px;
    }

    .state-box {
      margin: 24px 0;
    }

    .pager {
      justify-content: flex-end;
      margin-top: 16px;
    }

    .app-tag {
      display: inline-block;
      padding: 1px 8px;
      font-size: 12px;
      line-height: 20px;
      border-radius: 4px;
    }

    .tag-blue {
      color: #2563eb;
      background: rgb(37 99 235 / 10%);
    }

    .tag-purple {
      color: #7c3aed;
      background: rgb(124 58 237 / 10%);
    }

    .tag-green {
      color: #059669;
      background: rgb(5 150 105 / 10%);
    }

    .card-list {
      display: flex;
      flex-direction: column;
      gap: 10px;
    }

    .item-card {
      padding: 12px;
      border: 1px solid var(--el-border-color-lighter);
      border-radius: 8px;
    }

    .item-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
    }

    .item-title {
      margin: 8px 0 6px;
      font-weight: 500;
      word-break: break-all;
    }

    .item-line {
      display: flex;
      justify-content: space-between;
      font-size: 13px;
      line-height: 22px;
    }

    .item-label {
      color: var(--el-text-color-secondary);
    }
  }

  @media (width <= 767px) {
    .license-commercial-page {
      .toolbar {
        flex-direction: column;
        align-items: stretch;
      }

      .filters {
        flex-wrap: nowrap;
        width: 100%;
      }

      .filter-app,
      .filter-status {
        flex: 1;
        width: auto;
      }
    }
  }

  .refund-policy {
    margin-bottom: 12px;
  }
</style>
