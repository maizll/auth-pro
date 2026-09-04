<!-- 套餐管理搜索栏 -->
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

  interface PlanSearchForm {
    appId?: number
    keyword?: string
    status?: string
  }

  interface Props {
    modelValue: PlanSearchForm
  }

  interface Emits {
    (e: 'update:modelValue', value: PlanSearchForm): void
    (e: 'search', params: PlanSearchForm): void
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
    { label: '启用', value: 'enabled' },
    { label: '禁用', value: 'disabled' }
  ]

  /**
   * 搜索表单配置项
   */
  const formItems = computed(() => [
    {
      label: '所属应用',
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
      label: '关键词',
      key: 'keyword',
      type: 'input',
      placeholder: '套餐名/应用名',
      clearable: true
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
  const handleSearch = (params: PlanSearchForm) => {
    emit('search', params)
  }

  onMounted(fetchAppOptions)

  defineExpose({ appOptions })
</script>
