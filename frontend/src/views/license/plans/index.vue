<!-- 套餐管理页面 -->
<!-- art-full-height 自动计算出页面剩余高度 -->
<!-- art-table-card 一个符合系统样式的 class，同时自动撑满剩余高度 -->
<template>
  <div class="license-plans-page art-full-height">
    <!-- 搜索栏 -->
    <PlanSearch v-model="searchForm" @search="handleSearch" @reset="resetSearchParams" />

    <ElCard class="art-table-card" shadow="never">
      <!-- 表格头部 -->
      <ArtTableHeader v-model:columns="columnChecks" :loading="loading" @refresh="refreshData">
        <template #left>
          <ElSpace wrap>
            <ElButton @click="handleAdd" v-ripple>新增套餐</ElButton>
          </ElSpace>
        </template>
      </ArtTableHeader>

      <!-- 表格（套餐接口不分页，展示全部结果） -->
      <ArtTable :loading="loading" :data="data" :columns="columns">
        <template #appName="{ row }">
          <span>{{ row.appName }}</span>
          <ElTag v-if="row.commercialProduct" class="commercial-plan-tag" type="warning" size="small" effect="dark">
            商业版
          </ElTag>
        </template>

        <!-- 授权方式 -->
        <template #licenseType="{ row }">
          <ElTag v-if="row.licenseType" size="small" effect="plain">
            {{ licenseTypeLabel(row.licenseType) }}
          </ElTag>
          <span v-else class="text-secondary">通用</span>
        </template>

        <!-- 价格 -->
        <template #price="{ row }"> ¥{{ Number(row.price || 0).toFixed(2) }} </template>

        <!-- 最大站点数 -->
        <template #maxSites="{ row }">
          <span v-if="row.licenseType === 'key'">{{ Number(row.maxSites || 0) || '不限' }}</span>
          <span v-else class="text-secondary">--</span>
        </template>

        <!-- 状态 -->
        <template #enabled="{ row }">
          <ElTag :type="row.enabled ? 'success' : 'info'" size="small">
            {{ row.enabled ? '启用' : '禁用' }}
          </ElTag>
        </template>

        <!-- 操作 -->
        <template #operation="{ row }">
          <ElButton link type="primary" @click="handleEdit(row)">编辑</ElButton>
          <ElButton link type="primary" @click="handleToggle(row)">
            {{ row.enabled ? '禁用' : '启用' }}
          </ElButton>
          <ElButton link type="danger" @click="handleDelete(row)">删除</ElButton>
        </template>
      </ArtTable>
    </ElCard>

    <!-- 新增/编辑弹窗 -->
    <ElDialog v-model="dialogVisible" :title="dialogTitle" width="520px" destroy-on-close>
      <ElForm ref="formRef" :model="formData" :rules="formRules" label-width="90px">
        <ElFormItem label="所属应用" prop="appId">
          <ElSelect
            v-model="formData.appId"
            placeholder="请选择应用"
            style="width: 100%"
            @change="handleAppChange"
          >
            <ElOption v-for="app in appOptions" :key="app.id" :label="app.name" :value="app.id" />
          </ElSelect>
        </ElFormItem>
        <ElFormItem label="套餐名称" prop="name">
          <ElInput v-model="formData.name" placeholder="例如：1个月、3个月、1年、永久" />
        </ElFormItem>
        <ElFormItem label="授权方式" prop="licenseType">
          <ElSelect
            v-model="formData.licenseType"
            placeholder="请选择授权方式"
            clearable
            style="width: 100%"
            :disabled="!formData.appId || availableLicenseTypes.length === 0"
          >
            <ElOption label="通用（当前应用全部授权方式）" value="" />
            <ElOption
              v-for="licenseType in availableLicenseTypes"
              :key="licenseType"
              :label="licenseTypeLabels[licenseType] || licenseType"
              :value="licenseType"
            />
          </ElSelect>
          <div class="form-tip block">{{ licenseTypeTip }}</div>
        </ElFormItem>
        <ElFormItem label="快捷时长">
          <ElSpace wrap>
            <ElButton size="small" @click="applyPreset('1个月', 30)">1个月</ElButton>
            <ElButton size="small" @click="applyPreset('3个月', 90)">3个月</ElButton>
            <ElButton size="small" @click="applyPreset('1年', 365)">1年</ElButton>
            <ElButton size="small" @click="applyPreset('永久', 0)">永久</ElButton>
          </ElSpace>
        </ElFormItem>
        <ElFormItem label="授权天数" prop="durationDays">
          <ElInputNumber
            v-model="formData.durationDays"
            :min="0"
            :precision="0"
            :step="30"
            controls-position="right"
          />
          <span class="form-tip">0 表示永久</span>
        </ElFormItem>
        <ElFormItem label="价格" prop="price">
          <ElInputNumber
            v-model="formData.price"
            :min="0"
            :precision="2"
            :step="10"
            controls-position="right"
          />
        </ElFormItem>
        <ElFormItem v-if="formData.licenseType === 'key'" label="最大站点数">
          <ElInputNumber
            v-model="formData.maxSites"
            :min="0"
            :precision="0"
            :step="1"
            controls-position="right"
          />
          <span class="form-tip">0 表示不限制</span>
        </ElFormItem>
        <ElFormItem label="排序">
          <ElInputNumber
            v-model="formData.sort"
            :precision="0"
            :step="10"
            controls-position="right"
          />
        </ElFormItem>
        <ElFormItem label="状态">
          <ElSwitch v-model="formData.enabled" active-text="启用" inactive-text="禁用" />
        </ElFormItem>
        <ElFormItem label="备注">
          <ElInput v-model="formData.remark" type="textarea" :rows="2" placeholder="可选" />
        </ElFormItem>
      </ElForm>
      <template #footer>
        <ElButton @click="dialogVisible = false">取消</ElButton>
        <ElButton type="primary" @click="handleSubmit">确定</ElButton>
      </template>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { useTable } from '@/hooks/core/useTable'
  import {
    fetchPlanList,
    fetchCreatePlan,
    fetchUpdatePlan,
    fetchTogglePlan,
    fetchDeletePlan,
    fetchLicenseAppList,
    type LicenseAppItem,
    type PlanItem,
    type PlanPayload
  } from '@/api/license-manage'
  import PlanSearch from './modules/plan-search.vue'

  defineOptions({ name: 'LicensePlans' })

  interface PlanSearchForm {
    appId?: number
    keyword?: string
    status?: string
  }

  // 弹窗相关
  const dialogVisible = ref(false)
  const isEdit = ref(false)
  const formRef = ref()
  const dialogTitle = computed(() => (isEdit.value ? '编辑套餐' : '新增套餐'))

  // 搜索表单
  const searchForm = ref<PlanSearchForm>({
    appId: undefined,
    keyword: undefined,
    status: undefined
  })

  // 弹窗应用选项（需要 purchaseLicenseTypes 联动授权方式）
  const appOptions = ref<LicenseAppItem[]>([])

  const formData = reactive({
    id: 0,
    appId: undefined as number | undefined,
    name: '',
    licenseType: '',
    durationDays: 30,
    price: 0,
    maxSites: 0,
    sort: 0,
    enabled: true,
    remark: ''
  })

  const licenseTypeLabels: Record<string, string> = {
    domain: '单域名授权',
    wildcard: '泛域名授权',
    ip: 'IP 授权',
    key: '密钥授权'
  }

  const selectedApp = computed(() => appOptions.value.find((app) => app.id === formData.appId))
  const availableLicenseTypes = computed<string[]>(() =>
    Array.isArray(selectedApp.value?.purchaseLicenseTypes)
      ? selectedApp.value.purchaseLicenseTypes
      : []
  )
  const licenseTypeTip = computed(() => {
    if (!formData.appId) return '请先选择所属应用'
    if (availableLicenseTypes.value.length === 0)
      return '该应用未配置可用授权方式，请先在应用管理中配置'
    return `仅显示当前应用已配置的授权方式：${availableLicenseTypes.value.map(licenseTypeLabel).join('、')}`
  })

  const licenseTypeLabel = (value: string): string => {
    return licenseTypeLabels[value] || value
  }

  const formRules = {
    appId: [{ required: true, message: '请选择应用', trigger: 'change' }],
    name: [{ required: true, message: '请输入套餐名称', trigger: 'blur' }],
    durationDays: [{ required: true, message: '请输入授权天数', trigger: 'blur' }],
    price: [{ required: true, message: '请输入价格', trigger: 'blur' }]
  }

  const {
    columns,
    columnChecks,
    data,
    loading,
    getData,
    replaceSearchParams,
    resetSearchParams,
    refreshData,
    refreshRemove
  } = useTable({
    // 核心配置
    core: {
      apiFn: fetchPlanList,
      apiParams: {
        page: 1,
        pageSize: 20,
        ...searchForm.value
      },
      // 后端套餐接口暂不分页，分页参数会被忽略
      paginationKey: {
        current: 'page',
        size: 'pageSize'
      },
      columnsFactory: () => [
        { type: 'index', width: 60, label: '序号' }, // 序号
        { prop: 'appName', label: '应用', minWidth: 180, useSlot: true, showOverflowTooltip: true },
        { prop: 'name', label: '套餐名称', minWidth: 140, showOverflowTooltip: true },
        { prop: 'licenseType', label: '授权方式', width: 110, align: 'center', useSlot: true },
        { prop: 'durationText', label: '授权时长', width: 120 },
        { prop: 'price', label: '价格', width: 120, align: 'right', useSlot: true },
        { prop: 'maxSites', label: '最大站点数', width: 110, align: 'center', useSlot: true },
        { prop: 'sort', label: '排序', width: 80, align: 'center' },
        { prop: 'enabled', label: '状态', width: 90, align: 'center', useSlot: true },
        { prop: 'remark', label: '备注', minWidth: 160, showOverflowTooltip: true },
        { prop: 'createdAt', label: '创建时间', width: 160 },
        {
          prop: 'operation',
          label: '操作',
          width: 170,
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
  const handleSearch = (params: PlanSearchForm) => {
    replaceSearchParams(params)
    getData()
  }

  const fetchApps = async () => {
    try {
      const data = await fetchLicenseAppList()
      appOptions.value = data || []
    } catch {
      appOptions.value = []
    }
  }

  const resetForm = () => {
    Object.assign(formData, {
      id: 0,
      appId: undefined,
      name: '',
      licenseType: '',
      durationDays: 30,
      price: 0,
      maxSites: 0,
      sort: 0,
      enabled: true,
      remark: ''
    })
  }

  const handleAppChange = () => {
    if (formData.licenseType && !availableLicenseTypes.value.includes(formData.licenseType)) {
      formData.licenseType = ''
    }
    formRef.value?.clearValidate('licenseType')
  }

  const applyPreset = (name: string, days: number) => {
    formData.name = name
    formData.durationDays = days
  }

  const handleAdd = () => {
    isEdit.value = false
    resetForm()
    dialogVisible.value = true
  }

  const handleEdit = (row: PlanItem) => {
    isEdit.value = true
    Object.assign(formData, {
      id: row.id,
      appId: row.appId,
      name: row.name,
      licenseType: row.licenseType || '',
      durationDays: row.durationDays,
      price: Number(row.price || 0),
      maxSites: Number(row.maxSites || 0),
      sort: row.sort || 0,
      enabled: row.enabled,
      remark: row.remark || ''
    })
    handleAppChange()
    dialogVisible.value = true
  }

  const handleSubmit = async () => {
    const valid = await formRef.value?.validate().catch(() => false)
    if (!valid) return

    const payload: PlanPayload = {
      appId: formData.appId,
      name: formData.name,
      licenseType: formData.licenseType,
      durationDays: formData.durationDays,
      price: formData.price,
      maxSites: formData.maxSites,
      sort: formData.sort,
      enabled: formData.enabled,
      remark: formData.remark
    }

    if (isEdit.value) {
      await fetchUpdatePlan(formData.id, payload)
      ElMessage.success('编辑成功')
    } else {
      await fetchCreatePlan(payload)
      ElMessage.success('新增成功')
    }
    dialogVisible.value = false
    refreshData()
  }

  const handleToggle = async (row: PlanItem) => {
    await fetchTogglePlan(row.id)
    ElMessage.success('操作成功')
    refreshData()
  }

  const handleDelete = async (row: PlanItem) => {
    try {
      await ElMessageBox.confirm(`确定删除套餐「${row.name}」？`, '删除套餐', { type: 'warning' })
      await fetchDeletePlan(row.id)
      ElMessage.success('删除成功')
      refreshRemove()
    } catch {
      return
    }
  }

  onMounted(fetchApps)
</script>

<style scoped lang="scss">
  .license-plans-page {
    .form-tip {
      margin-left: 12px;
      font-size: 12px;
      color: var(--el-text-color-secondary);

      &.block {
        width: 100%;
        margin-top: 4px;
        margin-left: 0;
      }
    }

    .commercial-plan-tag {
      margin-left: 6px;
    }

    .text-secondary {
      font-size: 13px;
      color: var(--el-text-color-secondary);
    }
  }
</style>
