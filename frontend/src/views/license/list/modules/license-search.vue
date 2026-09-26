<!-- 授权列表搜索栏 -->
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

  interface LicenseSearchForm {
    keyword?: string
    type?: string
    status?: string
    source?: string
    appId?: number | string
  }

  interface Props {
    modelValue: LicenseSearchForm
  }

  interface Emits {
    (e: 'update:modelValue', value: LicenseSearchForm): void
    (e: 'search', params: LicenseSearchForm): void
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
   * 授权类型选项
   */
  const typeOptions = [
    { label: '单域名', value: 'domain' },
    { label: '泛域名', value: 'wildcard' },
    { label: 'IP', value: 'ip' },
    { label: '密钥', value: 'key' }
  ]

  /**
   * 状态选项
   */
  const statusOptions = [
    { label: '正常', value: 'active' },
    { label: '已过期', value: 'expired' },
    { label: '已禁用', value: 'disabled' }
  ]

  const sourceOptions = [
    { label: '管理员', value: 'admin' },
    { label: '代理商', value: 'agent' },
    { label: '用户购买', value: 'user_purchase' },
    { label: '卡密', value: 'card' },
    { label: '商店绑定', value: 'store_bind' },
    { label: '商店购买', value: 'store_purchase' }
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
      label: '授权类型',
      key: 'type',
      type: 'select',
      props: {
        placeholder: '全部',
        options: typeOptions,
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
  const handleSearch = (params: LicenseSearchForm) => {
    emit('search', params)
  }

  onMounted(fetchAppOptions)
</script>
