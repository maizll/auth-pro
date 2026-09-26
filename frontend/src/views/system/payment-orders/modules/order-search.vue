<!-- 订单列表搜索栏 -->
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
  import type { PaymentOrderSearchParams } from '@/api/system-manage'

  type OrderSearchFormParams = Partial<
    Pick<PaymentOrderSearchParams, 'orderNo' | 'subjectType' | 'payMethod' | 'status'>
  >

  interface Props {
    modelValue: OrderSearchFormParams
  }

  interface Emits {
    (e: 'update:modelValue', value: OrderSearchFormParams): void
    (e: 'search', params: OrderSearchFormParams): void
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
   * 订单类型选项
   */
  const subjectTypeOptions = ref([
    { label: '用户充值', value: 'user' },
    { label: '代理充值', value: 'agent' },
    { label: '支付测试', value: 'test' },
    { label: '商业版', value: 'store_edition' },
    { label: '付费插件', value: 'store_plugin' },
    { label: '付费模板', value: 'store_template' }
  ])

  /**
   * 支付方式选项
   */
  const payMethodOptions = ref([
    { label: '支付宝', value: 'alipay' },
    { label: '微信支付', value: 'wxpay' },
    { label: 'QQ 钱包', value: 'qqpay' },
    { label: '余额支付', value: 'balance' },
    { label: '配额支付', value: 'quota' }
  ])

  /**
   * 支付状态选项
   */
  const statusOptions = ref([
    { label: '待支付', value: 'pending' },
    { label: '已支付', value: 'paid' },
    { label: '失败', value: 'failed' },
    { label: '已取消', value: 'cancelled' }
  ])

  /**
   * 搜索表单配置项
   */
  const formItems = computed(() => [
    {
      label: '订单号',
      key: 'orderNo',
      type: 'input',
      placeholder: '支持模糊搜索',
      clearable: true
    },
    {
      label: '订单类型',
      key: 'subjectType',
      type: 'select',
      props: {
        placeholder: '全部',
        options: subjectTypeOptions.value,
        clearable: true
      }
    },
    {
      label: '支付方式',
      key: 'payMethod',
      type: 'select',
      props: {
        placeholder: '全部',
        options: payMethodOptions.value,
        clearable: true
      }
    },
    {
      label: '支付状态',
      key: 'status',
      type: 'select',
      props: {
        placeholder: '全部',
        options: statusOptions.value,
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
  const handleSearch = (params: OrderSearchFormParams) => {
    emit('search', params)
  }
</script>
