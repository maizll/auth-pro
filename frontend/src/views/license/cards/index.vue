<!-- 卡密管理页面：按应用、套餐和授权类型生成一次性兑换卡 -->
<!-- art-full-height 自动计算出页面剩余高度 -->
<!-- art-table-card 一个符合系统样式的 class，同时自动撑满剩余高度 -->
<template>
  <div class="license-cards-page art-full-height">
    <!-- 搜索栏 -->
    <CardSearch v-model="searchForm" @search="handleSearch" @reset="resetSearchParams" />

    <ElCard class="art-table-card" shadow="never">
      <!-- 表格头部 -->
      <ArtTableHeader v-model:columns="columnChecks" :loading="loading" @refresh="refreshData">
        <template #left>
          <ElSpace wrap>
            <ElButton @click="openCreateDialog" v-ripple>生成卡密</ElButton>
          </ElSpace>
        </template>
      </ArtTableHeader>

      <!-- 表格 -->
      <ArtTable
        :loading="loading"
        :data="data"
        :columns="columns"
        :pagination="pagination"
        @pagination:size-change="handleSizeChange"
        @pagination:current-change="handleCurrentChange"
      >
        <!-- 批次号 -->
        <template #batchNo="{ row }">
          <span class="card-code">{{ row.batchNo }}</span>
        </template>

        <!-- 套餐 -->
        <template #planName="{ row }">
          <div>{{ row.planName }}</div>
          <span class="muted">
            {{ row.durationDays ? `${row.durationDays} 天` : '永久' }} / ¥{{
              Number(row.price).toFixed(2)
            }}
          </span>
        </template>

        <!-- 授权类型 -->
        <template #typeLabel="{ row }">
          <ElTag size="small">{{ row.typeLabel }}</ElTag>
        </template>

        <!-- 库存 -->
        <template #stock="{ row }">
          <div class="stock-row">
            <span
              >未兑换 <b>{{ row.unusedCount }}</b></span
            >
            <span
              >已兑换 <b>{{ row.redeemedCount }}</b></span
            >
            <span
              >已禁用 <b>{{ row.disabledCount }}</b></span
            >
          </div>
        </template>

        <!-- 批次状态 -->
        <template #status="{ row }">
          <ElTag :type="row.status === 'active' ? 'success' : 'info'" size="small">
            {{ row.status === 'active' ? '启用' : '已暂停' }}
          </ElTag>
        </template>

        <!-- 操作 -->
        <template #operation="{ row }">
          <ElButton link type="primary" @click="openCards(row)">明细</ElButton>
          <ElDropdown trigger="click" @command="(status: string) => exportCards(row, status)">
            <ElButton link type="primary">
              导出<ElIcon class="el-icon--right"><ArrowDown /></ElIcon>
            </ElButton>
            <template #dropdown>
              <ElDropdownMenu>
                <ElDropdownItem command="unused">仅未兑换</ElDropdownItem>
                <ElDropdownItem command="all">全部卡密</ElDropdownItem>
              </ElDropdownMenu>
            </template>
          </ElDropdown>
          <ElButton link type="danger" @click="deleteBatch(row)">删除</ElButton>
        </template>
      </ArtTable>
    </ElCard>

    <!-- 生成卡密弹窗 -->
    <ElDialog v-model="createDialog.visible" title="生成卡密" width="560px" destroy-on-close>
      <ElAlert
        title="卡密永久有效且只能兑换一次。套餐时长从兑换成功时开始计算。"
        type="info"
        show-icon
        :closable="false"
        class="mb-16"
      />
      <ElForm ref="createFormRef" :model="createForm" :rules="createRules" label-width="90px">
        <ElFormItem label="应用" prop="appId">
          <ElSelect
            v-model="createForm.appId"
            placeholder="请选择应用"
            style="width: 100%"
            @change="handleCreateAppChange"
          >
            <ElOption v-for="app in enabledApps" :key="app.id" :label="app.name" :value="app.id" />
          </ElSelect>
        </ElFormItem>
        <ElFormItem label="套餐" prop="planId">
          <ElSelect
            v-model="createForm.planId"
            placeholder="请选择套餐"
            style="width: 100%"
            :disabled="!createForm.appId"
          >
            <ElOption
              v-for="plan in availablePlans"
              :key="plan.id"
              :label="`${plan.name}（${plan.durationText} / ¥${Number(plan.price).toFixed(2)}）`"
              :value="plan.id"
            />
          </ElSelect>
        </ElFormItem>
        <ElFormItem label="授权类型" prop="type">
          <ElSelect
            v-model="createForm.type"
            placeholder="请选择授权类型"
            style="width: 100%"
            :disabled="!createForm.appId"
          >
            <ElOption
              v-for="type in availableTypes"
              :key="type"
              :label="typeMeta[type]"
              :value="type"
            />
          </ElSelect>
        </ElFormItem>
        <ElFormItem label="生成数量" prop="quantity">
          <ElInputNumber
            v-model="createForm.quantity"
            :min="1"
            :max="5000"
            :step="10"
            controls-position="right"
          />
          <span class="form-tip">单批最多 5000 张</span>
        </ElFormItem>
        <ElFormItem label="备注">
          <ElInput
            v-model="createForm.remark"
            type="textarea"
            :rows="2"
            maxlength="255"
            show-word-limit
          />
        </ElFormItem>
      </ElForm>
      <template #footer>
        <ElSpace :size="10">
          <ElButton @click="createDialog.visible = false">取消</ElButton>
          <ElButton type="primary" :loading="createDialog.submitting" @click="submitCreate">
            生成
          </ElButton>
        </ElSpace>
      </template>
    </ElDialog>

    <!-- 生成结果弹窗 -->
    <ElDialog v-model="resultDialog.visible" title="卡密生成成功" width="720px" destroy-on-close>
      <ElAlert
        title="完整卡密已保存，可在批次中重复导出。请妥善保管，避免泄露。"
        type="success"
        show-icon
        :closable="false"
        class="mb-16"
      />
      <ElInput v-model="resultDialog.text" type="textarea" :rows="12" readonly />
      <template #footer>
        <ElSpace :size="10">
          <ElButton @click="copyGeneratedCards">复制全部</ElButton>
          <ElButton type="primary" @click="resultDialog.visible = false">完成</ElButton>
        </ElSpace>
      </template>
    </ElDialog>

    <!-- 批次明细抽屉 -->
    <ElDrawer
      v-model="cardsDrawer.visible"
      :title="`批次明细 · ${cardsDrawer.batchNo}`"
      size="min(960px, 92vw)"
      destroy-on-close
    >
      <div class="drawer-toolbar">
        <ElSelect
          v-model="cardsDrawer.status"
          placeholder="全部状态"
          clearable
          style="width: 140px"
          @change="fetchCards"
        >
          <ElOption label="未兑换" value="unused" />
          <ElOption label="已兑换" value="redeemed" />
          <ElOption label="已禁用" value="disabled" />
        </ElSelect>
        <span class="muted">明细展示完整卡密，可直接复制</span>
      </div>
      <ElTable v-loading="cardsDrawer.loading" :data="cardsDrawer.list" stripe>
        <ElTableColumn label="卡密" min-width="270">
          <template #default="{ row }">
            <ElSpace :size="6">
              <span class="card-code">{{ row.cardCode }}</span>
              <ElButton plain type="primary" size="small" @click="copyCardCode(row.cardCode)">
                复制
              </ElButton>
            </ElSpace>
          </template>
        </ElTableColumn>
        <ElTableColumn prop="status" label="状态" width="100">
          <template #default="{ row }">
            <ElTag :type="cardStatusType(row.status)" size="small">
              {{ cardStatusLabel(row.status) }}
            </ElTag>
          </template>
        </ElTableColumn>
        <ElTableColumn label="兑换主体" min-width="190">
          <template #default="{ row }">
            {{
              row.redeemedByAccount
                ? `${row.redeemedByType === 'user' ? '用户' : '代理'} · ${row.redeemedByAccount}`
                : '-'
            }}
          </template>
        </ElTableColumn>
        <ElTableColumn prop="licenseId" label="授权ID" width="90" />
        <ElTableColumn prop="redeemedAt" label="兑换时间" width="165" />
        <ElTableColumn label="操作" width="90" fixed="right">
          <template #default="{ row }">
            <ElButton
              v-if="row.status !== 'redeemed'"
              plain
              :type="row.status === 'unused' ? 'danger' : 'success'"
              size="small"
              @click="toggleCard(row)"
            >
              {{ row.status === 'unused' ? '禁用' : '恢复' }}
            </ElButton>
          </template>
        </ElTableColumn>
      </ElTable>
      <div class="drawer-pagination">
        <ElPagination
          v-model:current-page="cardsDrawer.page"
          v-model:page-size="cardsDrawer.pageSize"
          :total="cardsDrawer.total"
          layout="total, prev, pager, next"
          @current-change="fetchCards"
        />
      </div>
    </ElDrawer>
  </div>
</template>

<script setup lang="ts">
  import axios from 'axios'
  import { ArrowDown } from '@element-plus/icons-vue'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { useTable } from '@/hooks/core/useTable'
  import { useUserStore } from '@/store/modules/user'
  import {
    fetchCardBatches,
    fetchCreateCardBatch,
    fetchDeleteCardBatch,
    fetchBatchCards,
    fetchUpdateCardStatus,
    fetchLicenseAppList,
    fetchPlanList,
    type CardBatchItem,
    type CardBatchSearchParams,
    type LicenseAppItem,
    type PlanItem
  } from '@/api/license-manage'
  import CardSearch from './modules/card-search.vue'

  defineOptions({ name: 'LicenseCards' })

  interface CardBatchSearchForm {
    keyword?: string
    appId?: number
    status?: string
  }

  const typeMeta: Record<string, string> = {
    domain: '单域名',
    wildcard: '泛域名',
    ip: 'IP',
    key: '密钥'
  }

  // 搜索表单
  const searchForm = ref<CardBatchSearchForm>({
    keyword: undefined,
    appId: undefined,
    status: undefined
  })

  // 生成卡密弹窗选项
  const appOptions = ref<LicenseAppItem[]>([])
  const planOptions = ref<PlanItem[]>([])

  const createFormRef = ref()
  const createDialog = reactive({ visible: false, submitting: false })
  const createForm = reactive({
    appId: undefined as number | undefined,
    planId: undefined as number | undefined,
    type: '',
    quantity: 100,
    remark: ''
  })
  const createRules = {
    appId: [{ required: true, message: '请选择应用', trigger: 'change' }],
    planId: [{ required: true, message: '请选择套餐', trigger: 'change' }],
    type: [{ required: true, message: '请选择授权类型', trigger: 'change' }],
    quantity: [{ required: true, message: '请输入生成数量', trigger: 'blur' }]
  }
  const resultDialog = reactive({ visible: false, text: '' })
  const cardsDrawer = reactive({
    visible: false,
    loading: false,
    batchId: 0,
    batchNo: '',
    status: '',
    list: [] as any[],
    page: 1,
    pageSize: 20,
    total: 0
  })

  const enabledApps = computed(() => appOptions.value.filter((app) => app.enabled !== false))
  const selectedApp = computed(() => appOptions.value.find((app) => app.id === createForm.appId))
  const availablePlans = computed(() =>
    planOptions.value.filter((plan) => plan.appId === createForm.appId && plan.enabled !== false)
  )
  const availableTypes = computed(() =>
    Array.isArray(selectedApp.value?.purchaseLicenseTypes)
      ? selectedApp.value.purchaseLicenseTypes
      : []
  )

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
    refreshData,
    refreshRemove
  } = useTable({
    // 核心配置
    core: {
      apiFn: fetchCardBatches,
      apiParams: {
        page: 1,
        pageSize: 20,
        ...searchForm.value
      },
      // 后端卡密接口使用 page / pageSize 分页字段
      paginationKey: {
        current: 'page',
        size: 'pageSize'
      },
      columnsFactory: () => [
        { type: 'index', width: 60, label: '序号' }, // 序号
        { prop: 'batchNo', label: '批次号', minWidth: 190, useSlot: true },
        { prop: 'appName', label: '应用', minWidth: 130, showOverflowTooltip: true },
        { prop: 'planName', label: '套餐', minWidth: 130, useSlot: true },
        { prop: 'typeLabel', label: '授权类型', width: 100, align: 'center', useSlot: true },
        { prop: 'stock', label: '库存', minWidth: 210, useSlot: true },
        { prop: 'status', label: '批次状态', width: 100, align: 'center', useSlot: true },
        { prop: 'createdAt', label: '生成时间', width: 160 },
        {
          prop: 'operation',
          label: '操作',
          width: 190,
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
  const handleSearch = (params: CardBatchSearchForm) => {
    replaceSearchParams(params as Partial<CardBatchSearchParams>)
    getData()
  }

  /**
   * 加载应用与套餐选项（生成卡密弹窗使用）
   */
  const loadOptions = async () => {
    const [apps, plans] = await Promise.all([fetchLicenseAppList(), fetchPlanList({})])
    appOptions.value = apps || []
    planOptions.value = plans || []
  }

  const openCreateDialog = () => {
    Object.assign(createForm, {
      appId: undefined,
      planId: undefined,
      type: '',
      quantity: 100,
      remark: ''
    })
    createDialog.visible = true
  }

  const handleCreateAppChange = () => {
    createForm.planId = undefined
    createForm.type = ''
  }

  const submitCreate = async () => {
    const valid = await createFormRef.value?.validate().catch(() => false)
    if (!valid) return
    await ElMessageBox.confirm(`确认生成 ${createForm.quantity} 张卡密？`, '生成卡密', {
      type: 'warning'
    })
    createDialog.submitting = true
    try {
      const data = await fetchCreateCardBatch(createForm)
      createDialog.visible = false
      resultDialog.text = (data?.cards || []).join('\n')
      resultDialog.visible = true
      getData()
    } finally {
      createDialog.submitting = false
    }
  }

  const deleteBatch = async (row: CardBatchItem) => {
    await ElMessageBox.confirm(
      `确认删除批次「${row.batchNo}」？这将删除该批次及其所有卡密，此操作不可恢复！`,
      '确认删除',
      {
        type: 'error',
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        confirmButtonClass: 'el-button--danger'
      }
    )
    await fetchDeleteCardBatch(row.id)
    ElMessage.success('批次已删除')
    refreshRemove()
  }

  // ==================== 批次明细抽屉 ====================

  const openCards = (row: CardBatchItem) => {
    Object.assign(cardsDrawer, {
      visible: true,
      batchId: row.id,
      batchNo: row.batchNo,
      status: '',
      page: 1
    })
    fetchCards()
  }

  const fetchCards = async () => {
    cardsDrawer.loading = true
    try {
      const data = await fetchBatchCards(cardsDrawer.batchId, {
        status: cardsDrawer.status || undefined,
        page: cardsDrawer.page,
        pageSize: cardsDrawer.pageSize
      })
      cardsDrawer.list = data?.list || []
      cardsDrawer.total = data?.total || 0
    } finally {
      cardsDrawer.loading = false
    }
  }

  const toggleCard = async (row: any) => {
    const status = row.status === 'unused' ? 'disabled' : 'unused'
    await fetchUpdateCardStatus(row.id, status)
    ElMessage.success('卡密状态已更新')
    fetchCards()
    refreshData()
  }

  /**
   * 导出卡密 CSV（携带鉴权头，走浏览器下载）
   */
  const exportCards = async (row: CardBatchItem, status: string) => {
    const token = useUserStore().accessToken
    const response = await axios.get(`/api/license/cards/batches/${row.id}/export`, {
      params: { status },
      responseType: 'blob',
      headers: { Authorization: `Bearer ${token}` }
    })
    const url = URL.createObjectURL(response.data)
    const link = document.createElement('a')
    link.href = url
    link.download = `${row.batchNo}.csv`
    link.click()
    URL.revokeObjectURL(url)
  }

  const copyGeneratedCards = async () => {
    await navigator.clipboard.writeText(resultDialog.text)
    ElMessage.success('已复制全部卡密')
  }

  const copyCardCode = async (cardCode: string) => {
    await navigator.clipboard.writeText(cardCode)
    ElMessage.success('卡密已复制')
  }

  const cardStatusLabel = (status: string) => {
    return { unused: '未兑换', redeemed: '已兑换', disabled: '已禁用' }[status] || status
  }

  const cardStatusType = (status: string): 'success' | 'info' | 'danger' => {
    return status === 'unused' ? 'success' : status === 'redeemed' ? 'info' : 'danger'
  }

  onMounted(loadOptions)
</script>

<style scoped lang="scss">
  .license-cards-page {
    .muted {
      margin-top: 4px;
      font-size: 12px;
      color: var(--el-text-color-secondary);
    }

    .stock-row {
      display: flex;
      gap: 12px;
      font-size: 12px;

      b {
        color: var(--el-text-color-primary);
      }
    }

    .drawer-toolbar {
      display: flex;
      gap: 10px;
      align-items: center;
      margin-bottom: 16px;
    }

    .drawer-pagination {
      display: flex;
      justify-content: flex-end;
      margin-top: 16px;
    }

    .form-tip {
      margin-left: 10px;
      font-size: 12px;
      color: var(--el-text-color-secondary);
    }

    .card-code {
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      white-space: nowrap;
    }

    .mb-16 {
      margin-bottom: 16px;
    }
  }
</style>
