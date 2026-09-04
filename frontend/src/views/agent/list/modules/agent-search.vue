<!-- 代理商列表搜索栏 -->
<template>
  <ArtSearchBar
    ref="searchBarRef"
    v-model="formData"
    :items="formItems"
    @reset="handleReset"
    @search="handleSearch"
  >
  </ArtSearchBar>
</template>

<script setup lang="ts">
  import { fetchAgentLevelOptions, type AgentLevelOption } from '@/api/agent-manage'

  interface AgentSearchForm {
    keyword?: string
    level?: string
    status?: string
    source?: string
  }

  interface Props {
    modelValue: AgentSearchForm
  }

  interface Emits {
    (e: 'update:modelValue', value: AgentSearchForm): void
    (e: 'search', params: AgentSearchForm): void
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

  /**
   * 等级选项（来自等级管理配置）
   */
  const levelOptions = ref<AgentLevelOption[]>([])

  const fetchLevelOptions = async () => {
    try {
      const data = await fetchAgentLevelOptions()
      levelOptions.value = data || []
    } catch {
      levelOptions.value = []
    }
  }

  /**
   * 状态选项
   */
  const statusOptions = [
    { label: '正常', value: 'active' },
    { label: '冻结', value: 'frozen' }
  ]

  /**
   * 来源选项
   */
  const sourceOptions = [
    { label: '后台创建', value: 'admin' },
    { label: '用户自助升级', value: 'user_upgrade' }
  ]

  /**
   * 搜索表单配置项
   */
  const formItems = computed(() => [
    {
      label: '代理商账号',
      key: 'keyword',
      type: 'input',
      placeholder: '账号/手机/邮箱',
      clearable: true
    },
    {
      label: '等级',
      key: 'level',
      type: 'select',
      props: {
        placeholder: '全部',
        options: levelOptions.value.map((item) => ({ label: item.name, value: item.code })),
        clearable: true
      }
    },
    {
      label: '状态',
      key: 'status',
      type: 'select',
      props: {
        placeholder: '全部',
        options: statusOptions,
        clearable: true
      }
    },
    {
      label: '来源',
      key: 'source',
      type: 'select',
      props: {
        placeholder: '全部',
        options: sourceOptions,
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
  const handleSearch = (params: AgentSearchForm) => {
    emit('search', params)
  }

  onMounted(fetchLevelOptions)

  defineExpose({ fetchLevelOptions })
</script>
