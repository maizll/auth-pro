<template>
  <el-dialog
    :model-value="visible"
    title="付费更换站点"
    width="min(420px, 92vw)"
    destroy-on-close
    @close="emit('close')"
  >
    <p class="pay-lead">免费更换次数已用完。支付 ¥{{ Number(price || 0).toFixed(2) }} 后自动完成这次更换。</p>
    <el-radio-group v-model="payMethod" class="pay-methods">
      <el-radio-button v-for="item in options" :key="item.code" :label="item.code">
        {{ item.label }}
      </el-radio-button>
    </el-radio-group>
    <p v-if="options.length === 0" class="pay-empty">支付通道未开启，请联系管理员</p>
    <p v-if="errorText" class="pay-error">{{ errorText }}</p>
    <template #footer>
      <el-button @click="emit('close')">取消</el-button>
      <el-button type="primary" :loading="submitting" :disabled="options.length === 0" @click="submit">
        去支付
      </el-button>
    </template>
  </el-dialog>
  <PayQrDialog
    :visible="qr.visible"
    :qr-code="qr.qrCode"
    :amount="qr.amount"
    :order-no="qr.orderNo"
    @close="qr.visible = false"
  />
</template>

<script setup lang="ts">
  import { onBeforeUnmount, reactive, ref, watch } from 'vue'
  import axios from 'axios'
  import { ElMessage } from 'element-plus'
  import PayQrDialog from '@/components/core/pay/PayQrDialog.vue'
  import { isQrCheckout } from '@/utils/checkout'

  const props = defineProps<{
    visible: boolean
    price: number
    panel: 'user' | 'agent'
    licenseId: number
    action: 'replace' | 'unbind'
    siteId?: number
    target?: string
  }>()
  const emit = defineEmits<{ close: []; paid: [] }>()

  const options = ref<Array<{ code: string; label: string }>>([])
  const payMethod = ref('balance')
  const submitting = ref(false)
  const errorText = ref('')
  const qr = reactive({ visible: false, qrCode: '', amount: '' as string | number, orderNo: '' })
  let pollTimer: ReturnType<typeof setInterval> | undefined

  function token() {
    const key = props.panel === 'agent' ? 'agent_panel_token' : 'user_panel_token'
    return localStorage.getItem(key) || ''
  }
  function base() {
    return props.panel === 'agent' ? '/api/agent-panel' : '/api/user-panel'
  }
  function stopPoll() {
    if (pollTimer) {
      clearInterval(pollTimer)
      pollTimer = undefined
    }
  }

  async function loadOptions() {
    errorText.value = ''
    const { data } = await axios.get(`${base()}/purchase/pay-options`, {
      headers: { Authorization: `Bearer ${token()}` }
    })
    const list = Array.isArray(data.data) ? data.data : data.data?.options || data.data?.list || []
    options.value = list
      .map((item: any) => ({ code: item.code || item.payType, label: item.label || item.name || item.code }))
      .filter((item: { code: string }) => item.code)
    if (!options.value.some((item) => item.code === 'balance')) {
      options.value.unshift({ code: 'balance', label: '余额支付' })
    }
    payMethod.value = options.value[0]?.code || 'balance'
  }

  function startPoll(orderNo: string) {
    stopPoll()
    pollTimer = setInterval(async () => {
      try {
        const { data } = await axios.get(
          `${base()}/licenses/${props.licenseId}/site-change/orders/${orderNo}`,
          { headers: { Authorization: `Bearer ${token()}` } }
        )
        if (data.data?.status === 'paid') {
          stopPoll()
          qr.visible = false
          ElMessage.success('已支付并完成更换')
          emit('paid')
          emit('close')
        }
      } catch {
        stopPoll()
      }
    }, 2000)
  }

  async function submit() {
    submitting.value = true
    errorText.value = ''
    try {
      const { data } = await axios.post(
        `${base()}/licenses/${props.licenseId}/site-change/pay`,
        {
          action: props.action,
          siteId: props.siteId || 0,
          target: props.target || '',
          payMethod: payMethod.value
        },
        { headers: { Authorization: `Bearer ${token()}` } }
      )
      if (data.code === 200 && data.data?.status === 'paid') {
        ElMessage.success(data.msg || '已支付并完成更换')
        emit('paid')
        emit('close')
        return
      }
      if (data.code === 200 && (data.data?.payUrl || isQrCheckout(data.data))) {
        if (isQrCheckout(data.data)) {
          qr.qrCode = data.data.qrCode
          qr.amount = data.data.amount
          qr.orderNo = data.data.orderNo
          qr.visible = true
        } else if (data.data.payUrl) {
          window.open(data.data.payUrl, '_blank', 'noopener')
        }
        if (data.data.orderNo) startPoll(data.data.orderNo)
        return
      }
      errorText.value = data.msg || '支付失败'
    } catch {
      errorText.value = '支付失败，请稍后重试'
    } finally {
      submitting.value = false
    }
  }

  watch(
    () => props.visible,
    (open) => {
      if (open) loadOptions().catch(() => {
        errorText.value = '读取支付方式失败'
      })
      else stopPoll()
    }
  )
  onBeforeUnmount(stopPoll)
</script>

<style scoped>
  .pay-lead {
    margin: 0 0 12px;
    line-height: 1.6;
  }
  .pay-methods {
    display: flex;
    flex-wrap: wrap;
  }
  .pay-empty,
  .pay-error {
    margin: 8px 0 0;
    color: var(--el-color-danger);
    font-size: 13px;
  }
</style>
