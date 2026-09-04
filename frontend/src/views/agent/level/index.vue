<!-- 代理商等级管理页面 -->
<!-- art-full-height 自动计算出页面剩余高度 -->
<!-- art-table-card 一个符合系统样式的 class，同时自动撑满剩余高度 -->
<template>
  <div class="agent-level-page art-full-height">
    <!-- 统计卡片 -->
    <div class="stats-cards">
      <div class="art-card stat-card">
        <span class="stat-label">等级总数</span>
        <strong class="stat-value text-primary">{{ stats.total }}</strong>
      </div>
      <div class="art-card stat-card">
        <span class="stat-label">当前页启用</span>
        <strong class="stat-value text-success">{{ stats.enabled }}</strong>
      </div>
      <div class="art-card stat-card">
        <span class="stat-label">最低折扣</span>
        <strong class="stat-value text-warning">
          {{ stats.minDiscount ? `${stats.minDiscount}折` : '-' }}
        </strong>
      </div>
      <div class="art-card stat-card">
        <span class="stat-label">绑定代理商</span>
        <strong class="stat-value">{{ stats.agentCount }}</strong>
      </div>
    </div>

    <!-- 搜索栏 -->
    <LevelSearch v-model="searchForm" @search="handleSearch" @reset="resetSearchParams" />

    <ElCard class="art-table-card" shadow="never">
      <!-- 表格头部 -->
      <ArtTableHeader v-model:columns="columnChecks" :loading="loading" @refresh="refreshData">
        <template #left>
          <ElSpace wrap>
            <ElButton @click="handleAdd" v-ripple>新增等级</ElButton>
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
        <!-- 等级 -->
        <template #name="{ row }">
          <ElTag :type="levelTagType(row.discount)" size="small">{{ row.name }}</ElTag>
        </template>

        <!-- 折扣 -->
        <template #discount="{ row }">
          <span class="discount-text">{{ formatDiscount(row.discount) }}折</span>
        </template>

        <!-- 用户自助开通 -->
        <template #selfServiceEnabled="{ row }">
          <ElTag :type="row.selfServiceEnabled ? 'success' : 'info'" size="small" effect="plain">
            {{ row.selfServiceEnabled ? '已开放' : '未开放' }}
          </ElTag>
        </template>

        <!-- 开通价格 -->
        <template #upgradePrice="{ row }">
          <span v-if="row.selfServiceEnabled" class="price-text">
            ¥{{ Number(row.upgradePrice).toFixed(2) }}
          </span>
          <span v-else class="remark-text">-</span>
        </template>

        <!-- 开通赠送 -->
        <template #openingBonus="{ row }">
          <span v-if="Number(row.openingBonus) > 0" class="bonus-text">
            + ¥{{ Number(row.openingBonus).toFixed(2) }}
          </span>
          <span v-else class="remark-text">-</span>
        </template>

        <!-- 绑定代理商 -->
        <template #agentCount="{ row }">
          <ElTag :type="row.agentCount > 0 ? 'primary' : 'info'" size="small" effect="plain">
            {{ row.agentCount }} 个
          </ElTag>
        </template>

        <!-- 状态 -->
        <template #enabled="{ row }">
          <ElTag :type="row.enabled ? 'success' : 'info'" size="small">
            {{ row.enabled ? '启用' : '禁用' }}
          </ElTag>
        </template>

        <!-- 备注 -->
        <template #remark="{ row }">
          <span class="remark-text">{{ row.remark || '-' }}</span>
        </template>

        <!-- 操作 -->
        <template #operation="{ row }">
          <ElButton link type="primary" @click="handleEdit(row)">编辑</ElButton>
          <ElTooltip
            :disabled="row.enabled || row.agentCount === 0"
            content="已有代理商使用，不能禁用"
            placement="top"
          >
            <span>
              <ElButton
                link
                type="primary"
                :disabled="row.enabled && row.agentCount > 0"
                @click="handleToggle(row)"
              >
                {{ row.enabled ? '禁用' : '启用' }}
              </ElButton>
            </span>
          </ElTooltip>
          <ElTooltip
            :disabled="row.agentCount === 0"
            content="已有代理商使用，不能删除"
            placement="top"
          >
            <span>
              <ElButton
                link
                type="danger"
                :disabled="row.agentCount > 0"
                @click="handleDelete(row)"
              >
                删除
              </ElButton>
            </span>
          </ElTooltip>
        </template>
      </ArtTable>
    </ElCard>

    <!-- 新增/编辑弹窗 -->
    <ElDialog
      v-model="dialogVisible"
      :title="dialogTitle"
      width="560px"
      destroy-on-close
      @closed="resetFormValidate"
    >
      <ElForm :model="formData" :rules="formRules" ref="formRef" label-width="120px">
        <ElFormItem label="等级名称" prop="name">
          <ElInput
            v-model.trim="formData.name"
            placeholder="例如 金牌代理"
            maxlength="50"
            show-word-limit
          />
        </ElFormItem>
        <ElFormItem label="等级折扣" prop="discount">
          <ElInputNumber
            v-model="formData.discount"
            :min="1"
            :max="10"
            :step="0.1"
            :precision="1"
          />
          <span class="form-unit">折</span>
          <span class="form-tip inline">数值越小，代理商拿货价格越低。</span>
        </ElFormItem>
        <ElFormItem label="用户自助开通">
          <ElSwitch v-model="formData.selfServiceEnabled" />
          <div class="form-tip">默认关闭；仅开启后，该等级才会出现在用户端开通代理页面。</div>
        </ElFormItem>
        <ElFormItem label="开通价格" prop="upgradePrice">
          <ElInputNumber
            v-model="formData.upgradePrice"
            :min="0"
            :max="9999999999.99"
            :step="100"
            :precision="2"
            :disabled="!formData.selfServiceEnabled"
          />
          <span class="form-unit">元</span>
        </ElFormItem>
        <ElFormItem label="开通赠送余额" prop="openingBonus">
          <ElInputNumber
            v-model="formData.openingBonus"
            :min="0"
            :max="9999999999.99"
            :step="100"
            :precision="2"
          />
          <span class="form-unit">元</span>
          <div class="form-tip">用户成功自助开通后一次性发放，修改不影响已有订单。</div>
        </ElFormItem>
        <ElFormItem label="等级权益">
          <ElInput
            v-model="formData.benefits"
            type="textarea"
            :rows="4"
            maxlength="1000"
            show-word-limit
            placeholder="每行填写一项权益，将展示在用户端等级卡片中"
          />
        </ElFormItem>
        <ElFormItem label="排序" prop="sort">
          <ElInputNumber v-model="formData.sort" :min="0" :max="999" />
        </ElFormItem>
        <ElFormItem label="启用状态">
          <ElSwitch
            v-model="formData.enabled"
            :disabled="isEdit && formData.agentCount > 0"
            active-text="启用"
            inactive-text="禁用"
          />
          <div v-if="isEdit && formData.agentCount > 0" class="form-tip">
            当前等级已绑定 {{ formData.agentCount }} 个代理商，不能直接禁用。
          </div>
        </ElFormItem>
        <ElFormItem label="备注">
          <ElInput
            v-model="formData.remark"
            type="textarea"
            :rows="3"
            maxlength="255"
            show-word-limit
            placeholder="可选备注"
          />
        </ElFormItem>
      </ElForm>
      <template #footer>
        <ElButton @click="dialogVisible = false">取消</ElButton>
        <ElButton type="primary" :loading="submitLoading" @click="handleSubmit">确定</ElButton>
      </template>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import type { FormInstance, FormRules } from 'element-plus'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { useTable } from '@/hooks/core/useTable'
  import {
    fetchAgentLevelList,
    fetchCreateAgentLevel,
    fetchUpdateAgentLevel,
    fetchDeleteAgentLevel,
    type AgentLevelItem,
    type AgentLevelPayload
  } from '@/api/agent-manage'
  import LevelSearch from './modules/level-search.vue'

  defineOptions({ name: 'AgentLevel' })

  interface LevelSearchForm {
    keyword?: string
    status?: string
  }

  type LevelForm = {
    id: number
    name: string
    discount: number
    selfServiceEnabled: boolean
    upgradePrice: number
    openingBonus: number
    benefits: string
    sort: number
    enabled: boolean
    remark: string
    agentCount: number
  }

  const defaultForm: LevelForm = {
    id: 0,
    name: '',
    discount: 9,
    selfServiceEnabled: false,
    upgradePrice: 0,
    openingBonus: 0,
    benefits: '',
    sort: 0,
    enabled: true,
    remark: '',
    agentCount: 0
  }

  // 弹窗相关
  const submitLoading = ref(false)
  const dialogVisible = ref(false)
  const isEdit = ref(false)
  const formRef = ref<FormInstance>()
  const dialogTitle = computed(() => (isEdit.value ? '编辑等级' : '新增等级'))

  // 搜索表单
  const searchForm = ref<LevelSearchForm>({
    keyword: undefined,
    status: undefined
  })

  const formData = reactive<LevelForm>({ ...defaultForm })

  const formRules: FormRules<LevelForm> = {
    name: [{ required: true, message: '请输入等级名称', trigger: 'blur' }],
    discount: [{ required: true, message: '请输入折扣', trigger: 'change' }],
    upgradePrice: [
      {
        validator: (_rule, value, callback) => {
          if (formData.selfServiceEnabled && Number(value) < 0.01) {
            callback(new Error('允许用户自助开通时，价格不能低于 0.01 元'))
            return
          }
          callback()
        },
        trigger: 'change'
      }
    ],
    sort: [{ required: true, message: '请输入排序', trigger: 'change' }]
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
    refreshData,
    refreshCreate,
    refreshUpdate,
    refreshRemove
  } = useTable({
    // 核心配置
    core: {
      apiFn: fetchAgentLevelList,
      apiParams: {
        page: 1,
        pageSize: 20,
        ...searchForm.value
      },
      // 后端等级接口使用 page / pageSize 分页字段
      paginationKey: {
        current: 'page',
        size: 'pageSize'
      },
      columnsFactory: () => [
        { type: 'index', width: 60, label: '序号' }, // 序号
        { prop: 'name', label: '等级', minWidth: 150, useSlot: true },
        { prop: 'discount', label: '折扣', width: 100, align: 'center', useSlot: true },
        {
          prop: 'selfServiceEnabled',
          label: '用户自助开通',
          width: 120,
          align: 'center',
          useSlot: true
        },
        {
          prop: 'upgradePrice',
          label: '开通价格',
          width: 110,
          align: 'right',
          useSlot: true
        },
        {
          prop: 'openingBonus',
          label: '开通赠送',
          width: 110,
          align: 'right',
          useSlot: true
        },
        {
          prop: 'agentCount',
          label: '绑定代理商',
          width: 110,
          align: 'center',
          useSlot: true
        },
        { prop: 'sort', label: '排序', width: 80, align: 'center' },
        { prop: 'enabled', label: '状态', width: 90, align: 'center', useSlot: true },
        {
          prop: 'remark',
          label: '备注',
          minWidth: 160,
          showOverflowTooltip: true,
          useSlot: true
        },
        { prop: 'updatedAt', label: '更新时间', width: 160 },
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
        // 规范化数值与布尔字段，避免后端返回字符串导致展示异常
        const normalized = (records as unknown as AgentLevelItem[]).map((item) => ({
          ...item,
          discount: Number(item.discount || 0),
          selfServiceEnabled: Boolean(item.selfServiceEnabled),
          upgradePrice: Number(item.upgradePrice || 0),
          openingBonus: Number(item.openingBonus || 0),
          benefits: item.benefits || '',
          sort: Number(item.sort || 0),
          agentCount: Number(item.agentCount || 0),
          remark: item.remark || ''
        }))
        return normalized as unknown as typeof records
      }
    }
  })

  /**
   * 当前页统计数据
   */
  const stats = computed(() => {
    const records = data.value as unknown as AgentLevelItem[]
    const discounts = records.map((item) => Number(item.discount || 0)).filter(Boolean)
    return {
      total: pagination.total,
      enabled: records.filter((item) => item.enabled).length,
      minDiscount: discounts.length ? Math.min(...discounts) : 0,
      agentCount: records.reduce((total, item) => total + Number(item.agentCount || 0), 0)
    }
  })

  /**
   * 搜索处理
   */
  const handleSearch = (params: LevelSearchForm) => {
    replaceSearchParams(params)
    getData()
  }

  const formatDiscount = (discount: number) => {
    return Number(discount || 0)
      .toFixed(1)
      .replace(/\.0$/, '')
  }

  const levelTagType = (discount: number) => {
    if (discount <= 7) return 'warning'
    if (discount <= 8) return 'success'
    return 'info'
  }

  const resetFormValidate = () => {
    formRef.value?.clearValidate()
  }

  const handleAdd = () => {
    isEdit.value = false
    Object.assign(formData, defaultForm)
    dialogVisible.value = true
  }

  const handleEdit = (row: AgentLevelItem) => {
    isEdit.value = true
    Object.assign(formData, {
      id: row.id,
      name: row.name,
      discount: Number(row.discount),
      selfServiceEnabled: row.selfServiceEnabled,
      upgradePrice: Number(row.upgradePrice || 0),
      openingBonus: Number(row.openingBonus || 0),
      benefits: row.benefits || '',
      sort: row.sort,
      enabled: row.enabled,
      remark: row.remark,
      agentCount: row.agentCount
    })
    dialogVisible.value = true
  }

  const buildPayload = (): AgentLevelPayload => {
    return {
      name: formData.name.trim(),
      discount: Number(formData.discount),
      selfServiceEnabled: formData.selfServiceEnabled,
      upgradePrice: Number(formData.upgradePrice || 0),
      openingBonus: Number(formData.openingBonus || 0),
      benefits: formData.benefits.trim(),
      sort: Number(formData.sort || 0),
      enabled: formData.enabled,
      remark: formData.remark.trim()
    }
  }

  const handleSubmit = async () => {
    const valid = await formRef.value?.validate().catch(() => false)
    if (!valid) return

    submitLoading.value = true
    try {
      const payload = buildPayload()
      if (isEdit.value) {
        await fetchUpdateAgentLevel(formData.id, payload)
        ElMessage.success('编辑成功，已同步该等级下的代理商折扣')
      } else {
        await fetchCreateAgentLevel(payload)
        ElMessage.success('新增成功')
      }
      dialogVisible.value = false
      if (isEdit.value) {
        refreshUpdate()
      } else {
        refreshCreate()
      }
    } finally {
      submitLoading.value = false
    }
  }

  const handleToggle = async (row: AgentLevelItem) => {
    if (row.enabled && row.agentCount > 0) {
      ElMessage.warning('该等级已有代理商使用，不能禁用')
      return
    }

    const action = row.enabled ? '禁用' : '启用'
    try {
      await ElMessageBox.confirm(`确定${action}等级「${row.name}」？`, '提示', { type: 'warning' })
      await fetchUpdateAgentLevel(row.id, {
        name: row.name,
        discount: Number(row.discount),
        selfServiceEnabled: row.selfServiceEnabled,
        upgradePrice: Number(row.upgradePrice || 0),
        openingBonus: Number(row.openingBonus || 0),
        benefits: row.benefits || '',
        sort: row.sort,
        enabled: !row.enabled,
        remark: row.remark
      })
      ElMessage.success(`${action}成功`)
      refreshUpdate()
    } catch {
      return
    }
  }

  const handleDelete = async (row: AgentLevelItem) => {
    if (row.agentCount > 0) {
      ElMessage.warning('该等级已有代理商使用，不能删除')
      return
    }

    try {
      await ElMessageBox.confirm(`删除等级「${row.name}」后不可恢复，确定继续？`, '危险操作', {
        type: 'error',
        confirmButtonText: '确认删除',
        confirmButtonClass: 'el-button--danger'
      })
      await fetchDeleteAgentLevel(row.id)
      ElMessage.success('删除成功')
      refreshRemove()
    } catch {
      return
    }
  }
</script>

<style scoped lang="scss">
  .agent-level-page {
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
      font-size: 24px;
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
    }

    .discount-text {
      font-weight: 700;
      color: var(--el-color-primary);
    }

    .bonus-text {
      font-weight: 600;
      color: var(--el-color-success);
    }

    .remark-text {
      color: var(--el-text-color-regular);
    }

    .form-unit {
      margin-left: 8px;
      color: var(--el-text-color-secondary);
    }

    .form-tip {
      margin-top: 4px;
      font-size: 12px;
      line-height: 1.4;
      color: var(--el-text-color-secondary);

      &.inline {
        margin-top: 0;
        margin-left: 12px;
      }
    }
  }
</style>
