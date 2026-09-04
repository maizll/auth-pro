<!-- 财务流水搜索栏 -->
<template>
  <ArtSearchBar
    ref="searchBarRef"
    v-model="formData"
    :items="formItems"
    :showExpand="false"
    @reset="handleReset"
    @search="handleSearch"
  >
  </ArtSearchBar>
</template>

<script setup lang="ts">
  import { fetchAgentSelectList, type AgentSelectOption } from '@/api/agent-manage'

  interface TransactionSearchForm {
    agentId?: string
    type?: string
    dateRange?: [string, string]
  }

  interface Props {
    modelValue: TransactionSearchForm
  }

  interface Emits {
    (e: 'update:modelValue', value: TransactionSearchForm): void
    (e: 'search', params: TransactionSearchForm): void
    (e: 'reset'): void
  }

  const props = defineProps<Props>()
  const emit = defineEmits<Emits>()

  const searchBarRef = ref()

  /**
   * 表单数据双向绑定
   */
  const formData = computed({
    get: () => props.modelValue,
    set: (val) => emit('update:modelValue', val)
  })

  const agentList = ref<AgentSelectOption[]>([])

  const fetchAgentOptions = async () => {
    try {
      const data = await fetchAgentSelectList()
      agentList.value = data || []
    } catch {
      agentList.value = []
    }
  }

  /**
   * 流水类型选项
   */
  const typeOptions = [
    { label: '充值', value: 'recharge' },
    { label: '消费', value: 'consume' },
    { label: '退款', value: 'refund' },
    { label: '开通赠送', value: 'bonus' }
  ]

  /**
   * 搜索表单配置项
   */
  const formItems = computed(() => [
    {
      label: '代理商',
      key: 'agentId',
      type: 'select',
      props: {
        placeholder: '全部',
        options: agentList.value.map((item) => ({ label: item.name, value: String(item.id) })),
        clearable: true,
        filterable: true
      }
    },
    {
      label: '类型',
      key: 'type',
      type: 'select',
      props: {
        placeholder: '全部',
        options: typeOptions,
        clearable: true
      }
    },
    {
      label: '时间范围',
      key: 'dateRange',
      type: 'daterange',
      props: {
        type: 'daterange',
        startPlaceholder: '开始日期',
        endPlaceholder: '结束日期',
        valueFormat: 'YYYY-MM-DD',
        clearable: true
      }
    }
  ])

  /**
   * 处理重置事件
   */
  const handleReset = () => {
    emit('reset')
  }

  /**
   * 处理搜索事件
   */
  const handleSearch = (params: TransactionSearchForm) => {
    emit('search', params)
  }

  onMounted(fetchAgentOptions)
</script>
