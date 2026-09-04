<!-- 验证日志搜索栏 -->
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
  import { fetchLicenseAppOptions, type LicenseAppOption } from '@/api/license-manage'

  interface VerifyLogSearchForm {
    keyword?: string
    appId?: number | string
    result?: string
    dateRange?: [string, string]
  }

  interface Props {
    modelValue: VerifyLogSearchForm
  }

  interface Emits {
    (e: 'update:modelValue', value: VerifyLogSearchForm): void
    (e: 'search', params: VerifyLogSearchForm): void
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

  const appList = ref<LicenseAppOption[]>([])

  const fetchAppOptions = async () => {
    try {
      const data = await fetchLicenseAppOptions()
      appList.value = data || []
    } catch {
      appList.value = []
    }
  }

  /**
   * 验证结果选项
   */
  const resultOptions = [
    { label: '通过', value: 'pass' },
    { label: '拒绝', value: 'reject' }
  ]

  /**
   * 搜索表单配置项
   */
  const formItems = computed(() => [
    {
      label: '域名/IP/密钥',
      key: 'keyword',
      type: 'input',
      labelWidth: '96px',
      placeholder: '请输入',
      clearable: true
    },
    {
      label: '应用',
      key: 'appId',
      type: 'select',
      props: {
        placeholder: '全部',
        options: appList.value.map((item) => ({ label: item.name, value: item.id })),
        clearable: true
      }
    },
    {
      label: '验证结果',
      key: 'result',
      type: 'select',
      props: {
        placeholder: '全部',
        options: resultOptions,
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
  const handleSearch = (params: VerifyLogSearchForm) => {
    emit('search', params)
  }

  onMounted(fetchAppOptions)
</script>
