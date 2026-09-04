<!-- 开码配额搜索栏 -->
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
  import { fetchLicenseAppOptions, type LicenseAppOption } from '@/api/license-manage'

  interface QuotaSearchForm {
    agentId?: string
    appId?: string
  }

  interface Props {
    modelValue: QuotaSearchForm
  }

  interface Emits {
    (e: 'update:modelValue', value: QuotaSearchForm): void
    (e: 'search', params: QuotaSearchForm): void
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
  const appList = ref<LicenseAppOption[]>([])

  const fetchAgentOptions = async () => {
    try {
      const data = await fetchAgentSelectList()
      agentList.value = data || []
    } catch {
      agentList.value = []
    }
  }

  const fetchAppOptions = async () => {
    try {
      const data = await fetchLicenseAppOptions()
      appList.value = data || []
    } catch {
      appList.value = []
    }
  }

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
      label: '应用',
      key: 'appId',
      type: 'select',
      props: {
        placeholder: '全部',
        options: appList.value.map((item) => ({ label: item.name, value: String(item.id) })),
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
  const handleSearch = (params: QuotaSearchForm) => {
    emit('search', params)
  }

  onMounted(() => {
    fetchAgentOptions()
    fetchAppOptions()
  })
</script>
