<!-- 代理商列表页面 -->
<!-- art-full-height 自动计算出页面剩余高度 -->
<!-- art-table-card 一个符合系统样式的 class，同时自动撑满剩余高度 -->
<template>
  <div class="agent-list-page art-full-height">
    <!-- 搜索栏 -->
    <AgentSearch
      ref="searchRef"
      v-model="searchForm"
      @search="handleSearch"
      @reset="resetSearchParams"
    />

    <ElCard class="art-table-card" shadow="never">
      <!-- 表格头部 -->
      <ArtTableHeader v-model:columns="columnChecks" :loading="loading" @refresh="refreshData">
        <template #left>
          <ElSpace wrap>
            <ElButton @click="handleAdd" v-ripple>新增代理商</ElButton>
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
        <template #levelLabel="{ row }">
          <ElTag :type="levelTagType(row.discount)" size="small">{{ row.levelLabel }}</ElTag>
        </template>

        <!-- 账户来源 -->
        <template #sourceLabel="{ row }">
          <ElTag
            :type="row.source === 'user_upgrade' ? 'warning' : 'info'"
            size="small"
            effect="plain"
          >
            {{ row.sourceLabel }}
          </ElTag>
        </template>

        <!-- 折扣 -->
        <template #discount="{ row }">
          <span>{{ row.discount }}折</span>
        </template>

        <!-- 余额 -->
        <template #balance="{ row }">
          <span class="balance-text">¥{{ Number(row.balance || 0).toFixed(2) }}</span>
        </template>

        <!-- 升级迁移 -->
        <template #conversion="{ row }">
          <div v-if="row.source === 'user_upgrade'" class="conversion-summary">
            <span>原用户 #{{ row.originalUserId }}</span>
            <span>
              余额 ¥{{ Number(row.transferredBalance || 0).toFixed(2) }} · 授权
              {{ row.migratedLicenseCount }} 项
            </span>
          </div>
          <span v-else>-</span>
        </template>

        <!-- 状态 -->
        <template #statusLabel="{ row }">
          <ElTag :type="row.status === 'active' ? 'success' : 'danger'" size="small">
            {{ row.statusLabel }}
          </ElTag>
        </template>

        <!-- 操作 -->
        <template #operation="{ row }">
          <ElButton link type="success" @click="loginAsAgent(row)">登录</ElButton>
          <ElButton link type="primary" @click="handleEdit(row)">编辑</ElButton>
          <ElButton link type="primary" @click="handleRecharge(row)">充值</ElButton>
          <ElButton link type="primary" @click="handleToggle(row)">
            {{ row.status === 'active' ? '冻结' : '解冻' }}
          </ElButton>
          <ElTooltip
            :disabled="row.source !== 'user_upgrade'"
            content="用户升级产生的代理需保留审计关联，不能删除"
            placement="top"
          >
            <span>
              <ElButton
                link
                type="danger"
                :disabled="row.source === 'user_upgrade'"
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
    <ElDialog v-model="dialogVisible" :title="dialogTitle" width="520px" destroy-on-close>
      <ElForm :model="formData" :rules="formRules" ref="formRef" label-width="90px">
        <ElFormItem label="账号" prop="name">
          <ElInput v-model="formData.name" placeholder="代理商账号" />
        </ElFormItem>
        <ElFormItem label="联系方式" prop="contact">
          <ElInput v-model="formData.contact" placeholder="手机号或邮箱" />
        </ElFormItem>
        <ElFormItem label="登录密码" prop="password">
          <ElInput
            v-model="formData.password"
            type="password"
            :placeholder="isEdit ? '留空则不修改密码' : '代理商后台登录密码'"
            show-password
          />
        </ElFormItem>
        <ElFormItem label="等级" prop="level">
          <ElSelect v-model="formData.level" style="width: 100%" @change="syncDiscountByLevel">
            <ElOption
              v-for="level in levelOptions"
              :key="level.code"
              :label="`${level.name} (${level.discount}折)`"
              :value="level.code"
            />
          </ElSelect>
        </ElFormItem>
        <ElFormItem label="代理商折扣">
          <ElInputNumber
            v-model="formData.discount"
            :min="1"
            :max="10"
            :step="0.1"
            :precision="1"
          />
          <span class="form-tip inline">默认使用等级折扣，可单独调整</span>
        </ElFormItem>
        <ElFormItem label="备注">
          <ElInput v-model="formData.remark" type="textarea" :rows="2" />
        </ElFormItem>
      </ElForm>
      <template #footer>
        <ElButton @click="dialogVisible = false">取消</ElButton>
        <ElButton type="primary" @click="handleSubmit">确定</ElButton>
      </template>
    </ElDialog>

    <!-- 充值弹窗 -->
    <ElDialog v-model="rechargeVisible" title="代理商充值" width="400px" destroy-on-close>
      <ElForm :model="rechargeForm" label-width="80px">
        <ElFormItem label="代理商">
          <ElInput :model-value="rechargeForm.name" disabled />
        </ElFormItem>
        <ElFormItem label="当前余额">
          <ElInput :model-value="'¥' + rechargeForm.currentBalance.toFixed(2)" disabled />
        </ElFormItem>
        <ElFormItem label="充值金额">
          <ElInputNumber
            v-model="rechargeForm.amount"
            :min="1"
            :max="999999"
            :step="100"
            style="width: 100%"
          />
        </ElFormItem>
        <ElFormItem label="备注">
          <ElInput v-model="rechargeForm.remark" placeholder="充值备注（可选）" />
        </ElFormItem>
      </ElForm>
      <template #footer>
        <ElButton @click="rechargeVisible = false">取消</ElButton>
        <ElButton type="primary" @click="handleRechargeSubmit">确认充值</ElButton>
      </template>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { showCaughtError } from '@/utils/http/error-toast'
  import { useTable } from '@/hooks/core/useTable'
  import {
    fetchAgentList,
    fetchAgentLevelOptions,
    fetchCreateAgent,
    fetchUpdateAgent,
    fetchToggleAgent,
    fetchDeleteAgent,
    fetchRechargeAgent,
    fetchImpersonateAgent,
    type AgentItem,
    type AgentLevelOption
  } from '@/api/agent-manage'
  import AgentSearch from './modules/agent-search.vue'

  defineOptions({ name: 'AgentList' })

  interface AgentSearchForm {
    keyword?: string
    level?: string
    status?: string
    source?: string
  }

  // 弹窗相关
  const dialogVisible = ref(false)
  const rechargeVisible = ref(false)
  const isEdit = ref(false)
  const dialogTitle = computed(() => (isEdit.value ? '编辑代理商' : '新增代理商'))

  // 搜索表单
  const searchForm = ref<AgentSearchForm>({
    keyword: undefined,
    level: undefined,
    status: undefined,
    source: undefined
  })

  const searchRef = ref()

  // 等级选项（新增/编辑弹窗使用）
  const levelOptions = ref<AgentLevelOption[]>([])

  const formRef = ref()
  const formData = reactive({
    id: 0,
    name: '',
    contact: '',
    password: '',
    level: 'bronze',
    discount: 9,
    remark: ''
  })
  const formRules = {
    name: [{ required: true, message: '请输入账号', trigger: 'blur' }],
    contact: [{ required: true, message: '请输入联系方式', trigger: 'blur' }],
    password: [
      {
        validator: (_rule: any, value: string, callback: (error?: Error) => void) => {
          if (!isEdit.value && !value?.trim()) {
            callback(new Error('请输入密码'))
            return
          }
          callback()
        },
        trigger: 'blur'
      }
    ],
    level: [{ required: true, message: '请选择等级', trigger: 'change' }]
  }

  const rechargeForm = reactive({ id: 0, name: '', currentBalance: 0, amount: 100, remark: '' })

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
      apiFn: fetchAgentList,
      apiParams: {
        page: 1,
        pageSize: 20,
        ...searchForm.value
      },
      // 后端代理商接口使用 page / pageSize 分页字段
      paginationKey: {
        current: 'page',
        size: 'pageSize'
      },
      columnsFactory: () => [
        { type: 'index', width: 60, label: '序号' }, // 序号
        { prop: 'name', label: '账号', minWidth: 120, showOverflowTooltip: true },
        { prop: 'contact', label: '联系方式', minWidth: 140, showOverflowTooltip: true },
        {
          prop: 'levelLabel',
          label: '等级',
          width: 90,
          align: 'center',
          useSlot: true
        },
        {
          prop: 'sourceLabel',
          label: '账户来源',
          width: 130,
          align: 'center',
          useSlot: true
        },
        { prop: 'discount', label: '折扣', width: 80, align: 'center', useSlot: true },
        { prop: 'balance', label: '余额(元)', width: 110, align: 'right', useSlot: true },
        { prop: 'totalLicenses', label: '当前授权', width: 90, align: 'center' },
        { prop: 'conversion', label: '升级迁移', minWidth: 180, useSlot: true },
        {
          prop: 'statusLabel',
          label: '状态',
          width: 80,
          align: 'center',
          useSlot: true
        },
        { prop: 'createdAt', label: '注册时间', width: 160 },
        {
          prop: 'operation',
          label: '操作',
          width: 250,
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
  const handleSearch = (params: AgentSearchForm) => {
    replaceSearchParams(params)
    getData()
  }

  /**
   * 加载等级选项（新增/编辑弹窗）
   */
  const fetchLevelOptions = async () => {
    try {
      levelOptions.value = await fetchAgentLevelOptions()
    } catch {
      levelOptions.value = []
    }
  }

  /**
   * 管理员代登录代理商账号（新窗口打开代理端，不影响当前管理员登录态）
   */
  const loginAsAgent = async (row: AgentItem) => {
    if (!row?.id) {
      ElMessage.error('缺少代理商ID')
      return
    }
    try {
      const data = await fetchImpersonateAgent(row.id)
      const info = {
        agentId: data.agentId,
        email: data.email,
        name: data.name,
        balance: data.balance
      }
      sessionStorage.setItem('impersonate_agent_token', data.accessToken)
      sessionStorage.setItem('impersonate_agent_info', JSON.stringify(info))
      window.open(`${location.origin}/agent-panel/login?impersonate=1`, '_blank')
    } catch (error) {
      showCaughtError(error, '登录失败')
    }
  }

  const levelTagType = (discount: number) => {
    if (discount <= 7) return 'warning'
    if (discount <= 8) return 'success'
    return 'info'
  }

  const syncDiscountByLevel = (level = formData.level) => {
    const option = levelOptions.value.find((item) => item.code === level)
    formData.discount = Number(option?.discount || 9)
  }

  const getDefaultLevel = () => {
    return levelOptions.value[0]?.code || ''
  }

  const handleAdd = async () => {
    isEdit.value = false
    await fetchLevelOptions()
    const defaultLevel = getDefaultLevel()
    if (!defaultLevel) {
      ElMessage.warning('暂无可用代理商等级，请先新增并启用等级')
      return
    }
    Object.assign(formData, {
      id: 0,
      name: '',
      contact: '',
      password: '',
      level: defaultLevel,
      discount: 9,
      remark: ''
    })
    syncDiscountByLevel(defaultLevel)
    dialogVisible.value = true
  }

  const handleEdit = async (row: AgentItem) => {
    isEdit.value = true
    await fetchLevelOptions()
    if (!levelOptions.value.length) {
      ElMessage.warning('暂无可用代理商等级，请先新增并启用等级')
    }
    Object.assign(formData, {
      id: row.id,
      name: row.name,
      contact: row.contact,
      password: '',
      level: row.level,
      discount: row.discount,
      remark: row.remark
    })
    dialogVisible.value = true
  }

  const handleRecharge = (row: AgentItem) => {
    Object.assign(rechargeForm, {
      id: row.id,
      name: row.name,
      currentBalance: Number(row.balance || 0),
      amount: 100,
      remark: ''
    })
    rechargeVisible.value = true
  }

  const handleRechargeSubmit = async () => {
    try {
      await fetchRechargeAgent(rechargeForm.id, {
        amount: rechargeForm.amount,
        remark: rechargeForm.remark
      })
      ElMessage.success(`成功充值 ¥${rechargeForm.amount}`)
      rechargeVisible.value = false
      refreshUpdate()
    } catch (e) {
      console.error('[AgentList] 充值失败:', e)
    }
  }

  const handleToggle = async (row: AgentItem) => {
    const newStatus = row.status === 'active' ? 'frozen' : 'active'
    const action = row.status === 'active' ? '冻结' : '解冻'
    try {
      await ElMessageBox.confirm(`确定${action}代理商「${row.name}」？`, '提示', {
        type: 'warning'
      })
      await fetchToggleAgent(row.id, newStatus)
      ElMessage.success(`${action}成功`)
      refreshUpdate()
    } catch {
      return
    }
  }

  const handleDelete = async (row: AgentItem) => {
    if (row.source === 'user_upgrade') {
      ElMessage.warning('用户升级产生的代理需保留审计关联，只能冻结，不能删除')
      return
    }

    try {
      await ElMessageBox.confirm(`删除代理商「${row.name}」将清除其所有数据，确定？`, '危险操作', {
        type: 'error'
      })
      await fetchDeleteAgent(row.id)
      ElMessage.success('删除成功')
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
        await fetchUpdateAgent(formData.id, {
          name: formData.name,
          contact: formData.contact,
          password: formData.password,
          level: formData.level,
          discount: formData.discount,
          remark: formData.remark
        })
        ElMessage.success('编辑成功')
      } else {
        await fetchCreateAgent({
          name: formData.name,
          contact: formData.contact,
          password: formData.password,
          level: formData.level,
          discount: formData.discount,
          remark: formData.remark
        })
        ElMessage.success('新增成功')
      }
      dialogVisible.value = false
      if (isEdit.value) {
        refreshUpdate()
      } else {
        refreshCreate()
      }
    } catch (e) {
      console.error('[AgentList] 提交失败:', e)
    }
  }

  onMounted(fetchLevelOptions)
</script>

<style scoped lang="scss">
  .agent-list-page {
    .balance-text {
      font-weight: 600;
      color: var(--el-color-primary);
    }

    .conversion-summary {
      display: flex;
      flex-direction: column;
      gap: 2px;
      font-size: 12px;
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
