<!-- 卡密批次搜索栏 -->
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
  import { fetchLicenseAppList, type LicenseAppItem } from '@/api/license-manage'

  interface CardBatchSearchForm {
    keyword?: string
    appId?: number
    status?: string
  }

  interface Props {
    modelValue: CardBatchSearchForm
  }

  interface Emits {
    (e: 'update:modelValue', value: CardBatchSearchForm): void
    (e: 'search', params: CardBatchSearchForm): void
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

  const appOptions = ref<LicenseAppItem[]>([])

  const fetchAppOptions = async () => {
    try {
      const data = await fetchLicenseAppList()
      appOptions.value = data || []
    } catch {
      appOptions.value = []
    }
  }

  /**
   * 状态选项
   */
  const statusOptions = [
    { label: '启用', value: 'active' },
    { label: '已暂停', value: 'disabled' }
  ]

  /**
   * 搜索表单配置项
   */
  const formItems = computed(() => [
    {
      label: '关键词',
      key: 'keyword',
      type: 'input',
      placeholder: '批次号/应用/套餐',
      clearable: true
    },
    {
      label: '应用',
      key: 'appId',
      type: 'select',
      props: {
        placeholder: '全部应用',
        options: appOptions.value.map((item) => ({ label: item.name, value: item.id })),
        clearable: true,
        filterable: true
      }
    },
    {
      label: '状态',
      key: 'status',
      type: 'select',
      props: {
        placeholder: '全部状态',
        options: statusOptions,
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
  const handleSearch = (params: CardBatchSearchForm) => {
    emit('search', params)
  }

  onMounted(fetchAppOptions)
</script>
