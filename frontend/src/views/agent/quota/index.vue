<!-- 开码配额管理页面 -->
<!-- art-full-height 自动计算出页面剩余高度 -->
<!-- art-table-card 一个符合系统样式的 class，同时自动撑满剩余高度 -->
<template>
  <div class="agent-quota-page art-full-height">
    <!-- 搜索栏 -->
    <QuotaSearch v-model="searchForm" @search="handleSearch" @reset="resetSearchParams" />

    <ElCard class="art-table-card" shadow="never">
      <!-- 表格头部 -->
      <ArtTableHeader v-model:columns="columnChecks" :loading="loading" @refresh="refreshData">
        <template #left>
          <ElSpace wrap>
            <ElButton @click="handleAdd" v-ripple>分配配额</ElButton>
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
        <!-- 剩余 -->
        <template #remaining="{ row }">
          <span
            :class="{ 'text-danger': row.totalQuota !== -1 && row.totalQuota - row.usedQuota <= 5 }"
          >
            {{ row.totalQuota === -1 ? '无限' : row.totalQuota - row.usedQuota }}
          </span>
        </template>

        <!-- 使用率 -->
        <template #usage="{ row }">
          <ElProgress
            v-if="row.totalQuota !== -1"
            :percentage="Math.round((row.usedQuota / row.totalQuota) * 100)"
            :color="getProgressColor(row.usedQuota / row.totalQuota)"
            :stroke-width="8"
          />
          <ElTag v-else type="success" size="small">不限</ElTag>
        </template>

        <!-- 单价 -->
        <template #price="{ row }"> ¥{{ Number(row.price || 0).toFixed(2) }} </template>

        <!-- 操作 -->
        <template #operation="{ row }">
          <ElButton link type="primary" @click="handleEdit(row)">调整</ElButton>
          <ElButton link type="danger" @click="handleDelete(row)">移除</ElButton>
        </template>
      </ArtTable>
    </ElCard>

    <!-- 分配/调整弹窗 -->
    <ElDialog v-model="dialogVisible" :title="dialogTitle" width="480px" destroy-on-close>
      <ElForm :model="formData" :rules="formRules" ref="formRef" label-width="90px">
        <ElFormItem label="代理商" prop="agentId">
          <ElSelect
            v-model="formData.agentId"
            placeholder="请输入代理商名称搜索"
            filterable
            style="width: 100%"
            :disabled="isEdit"
          >
            <ElOption v-for="a in agentList" :key="a.id" :label="a.name" :value="a.id" />
          </ElSelect>
        </ElFormItem>
        <ElFormItem label="应用" prop="appId">
          <ElSelect
            v-model="formData.appId"
            placeholder="请选择"
            style="width: 100%"
            :disabled="isEdit"
          >
            <ElOption v-for="app in appList" :key="app.id" :label="app.name" :value="app.id" />
          </ElSelect>
        </ElFormItem>
        <ElFormItem label="总配额" prop="totalQuota">
          <ElInputNumber
            v-model="formData.totalQuota"
            :min="0"
            :max="999999"
            :step="10"
            style="width: 200px"
          />
        </ElFormItem>
        <ElFormItem label="单价(元)" prop="price">
          <ElInputNumber
            v-model="formData.price"
            :min="0"
            :max="999999"
            :step="10"
            :precision="2"
            style="width: 200px"
          />
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
    fetchQuotaList,
    fetchCreateQuota,
    fetchUpdateQuota,
    fetchDeleteQuota,
    fetchAgentSelectList,
    type AgentSelectOption,
    type QuotaItem
  } from '@/api/agent-manage'
  import { fetchLicenseAppOptions, type LicenseAppOption } from '@/api/license-manage'
  import QuotaSearch from './modules/quota-search.vue'

  defineOptions({ name: 'AgentQuota' })

  interface QuotaSearchForm {
    agentId?: string
    appId?: string
  }

  // 弹窗相关
  const dialogVisible = ref(false)
  const isEdit = ref(false)
  const dialogTitle = computed(() => (isEdit.value ? '调整配额' : '分配配额'))

  // 搜索表单
  const searchForm = ref<QuotaSearchForm>({
    agentId: undefined,
    appId: undefined
  })

  // 弹窗下拉选项
  const agentList = ref<AgentSelectOption[]>([])
  const appList = ref<LicenseAppOption[]>([])

  const formRef = ref()
  const formData = reactive({ id: 0, agentId: '', appId: '', totalQuota: 0, price: 0.0 })
  const formRules = {
    agentId: [{ required: true, message: '请选择代理商', trigger: 'change' }],
    appId: [{ required: true, message: '请选择应用', trigger: 'change' }],
    totalQuota: [{ required: true, message: '请输入配额', trigger: 'blur' }],
    price: [{ required: true, message: '请输入单价', trigger: 'blur' }]
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
      apiFn: fetchQuotaList,
      apiParams: {
        page: 1,
        pageSize: 20,
        ...searchForm.value
      },
      // 后端配额接口使用 page / pageSize 分页字段
      paginationKey: {
        current: 'page',
        size: 'pageSize'
      },
      columnsFactory: () => [
        { type: 'index', width: 60, label: '序号' }, // 序号
        { prop: 'agentName', label: '代理商', minWidth: 120, showOverflowTooltip: true },
        { prop: 'appName', label: '应用', width: 120 },
        { prop: 'totalQuota', label: '总配额', width: 100, align: 'center' },
        { prop: 'usedQuota', label: '已使用', width: 100, align: 'center' },
        { prop: 'remaining', label: '剩余', width: 100, align: 'center', useSlot: true },
        { prop: 'usage', label: '使用率', width: 160, useSlot: true },
        { prop: 'price', label: '单价(元)', width: 100, align: 'right', useSlot: true },
        { prop: 'updatedAt', label: '更新时间', width: 160 },
        {
          prop: 'operation',
          label: '操作',
          width: 130,
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
  const handleSearch = (params: QuotaSearchForm) => {
    replaceSearchParams(params)
    getData()
  }

  const getProgressColor = (ratio: number) => {
    if (ratio >= 0.9) return '#f56c6c'
    if (ratio >= 0.7) return '#e6a23c'
    return '#67c23a'
  }

  const fetchAgentOptions = async () => {
    try {
      const data = await fetchAgentSelectList()
      agentList.value = (data || []).map((a) => ({ id: String(a.id), name: a.name }))
    } catch {
      agentList.value = []
    }
  }

  const fetchAppOptions = async () => {
    try {
      const data = await fetchLicenseAppOptions()
      appList.value = (data || []).map((a) => ({ id: String(a.id), name: a.name }))
    } catch {
      appList.value = []
    }
  }

  const handleAdd = () => {
    isEdit.value = false
    Object.assign(formData, { id: 0, agentId: '', appId: '', totalQuota: 0, price: 0.0 })
    dialogVisible.value = true
  }

  const handleEdit = (row: QuotaItem) => {
    isEdit.value = true
    Object.assign(formData, {
      id: row.id,
      agentId: String(row.agentId),
      appId: String(row.appId),
      totalQuota: row.totalQuota,
      price: row.price
    })
    dialogVisible.value = true
  }

  const handleDelete = async (row: QuotaItem) => {
    try {
      await ElMessageBox.confirm(`移除「${row.agentName}」的「${row.appName}」配额？`, '提示', {
        type: 'warning'
      })
      await fetchDeleteQuota(row.id)
      ElMessage.success('已移除')
      refreshRemove()
    } catch {
      return
    }
  }

  const handleSubmit = async () => {
    const valid = await formRef.value?.validate().catch(() => false)
    if (!valid) return

    try {
      if (isEdit.value) {
        await fetchUpdateQuota(formData.id, {
          totalQuota: formData.totalQuota,
          price: formData.price
        })
        ElMessage.success('调整成功')
      } else {
        await fetchCreateQuota({
          agentId: Number(formData.agentId),
          appId: Number(formData.appId),
          totalQuota: formData.totalQuota,
          price: formData.price
        })
        ElMessage.success('分配成功')
      }
      dialogVisible.value = false
      if (isEdit.value) {
        refreshUpdate()
      } else {
        refreshCreate()
      }
    } catch (e) {
      console.error('[Quota] 提交失败:', e)
    }
  }

  onMounted(() => {
    fetchAgentOptions()
    fetchAppOptions()
  })
</script>

<style scoped lang="scss">
  .agent-quota-page {
    .text-danger {
      font-weight: 600;
      color: var(--el-color-danger);
    }
  }
</style>
