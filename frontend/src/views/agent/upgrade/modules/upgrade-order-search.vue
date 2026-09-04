<!-- 升级订单搜索栏 -->
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
  interface UpgradeOrderSearchForm {
    keyword?: string
    status?: string
    payChannel?: string
  }

  interface Props {
    modelValue: UpgradeOrderSearchForm
  }

  interface Emits {
    (e: 'update:modelValue', value: UpgradeOrderSearchForm): void
    (e: 'search', params: UpgradeOrderSearchForm): void
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
   * 订单状态选项
   */
  const statusOptions = [
    { label: '待支付', value: 'pending' },
    { label: '已支付', value: 'paid' },
    { label: '转换中', value: 'processing' },
    { label: '已完成', value: 'completed' },
    { label: '失败', value: 'failed' },
    { label: '已取消', value: 'cancelled' }
  ]

  /**
   * 支付渠道选项
   */
  const payChannelOptions = [{ label: '用户余额', value: 'balance' }]

  /**
   * 搜索表单配置项
   */
  const formItems = computed(() => [
    {
      label: '关键词',
      key: 'keyword',
      type: 'input',
      placeholder: '订单号 / 用户 / 代理',
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
    },
    {
      label: '支付渠道',
      key: 'payChannel',
      type: 'select',
      props: {
        placeholder: '全部渠道',
        options: payChannelOptions,
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
  const handleSearch = (params: UpgradeOrderSearchForm) => {
    emit('search', params)
  }
</script>
