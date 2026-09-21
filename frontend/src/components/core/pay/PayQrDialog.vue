<template>
  <ElDialog
    :model-value="visible"
    :title="title || '请使用支付宝扫码支付'"
    width="420px"
    append-to-body
    @close="emit('close')"
  >
    <div class="pay-qr-dialog">
      <p v-if="amount" class="pay-qr-amount">应付 ¥{{ amount }}</p>
      <div class="pay-qr-canvas">
        <QrcodeVue v-if="qrCode" :value="qrCode" :size="220" />
      </div>
      <p class="pay-qr-hint">请使用支付宝扫描二维码完成支付，到账后本页会自动刷新。</p>
      <p v-if="orderNo" class="pay-qr-order">订单号 {{ orderNo }}</p>
    </div>
    <template #footer>
      <ElButton @click="emit('close')">关闭</ElButton>
    </template>
  </ElDialog>
</template>

<script setup lang="ts">
  import QrcodeVue from 'qrcode.vue'

  defineProps<{
    visible: boolean
    qrCode: string
    amount?: string | number
    orderNo?: string
    title?: string
  }>()

  const emit = defineEmits<{
    close: []
  }>()
</script>

<style scoped>
  .pay-qr-dialog {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    padding: 8px 0 4px;
  }
  .pay-qr-amount {
    margin: 0;
    font-size: 22px;
    font-weight: 600;
    color: var(--el-color-danger);
  }
  .pay-qr-canvas {
    padding: 12px;
    background: #fff;
    border-radius: 12px;
    box-shadow: 0 0 0 1px var(--el-border-color-lighter);
  }
  .pay-qr-hint,
  .pay-qr-order {
    margin: 0;
    color: var(--el-text-color-secondary);
    font-size: 13px;
    text-align: center;
  }
</style>
