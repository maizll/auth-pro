<template>
  <ElAlert v-if="count > 0" class="commercial-reissue" type="warning" :closable="false" show-icon>
    <template #title>
      有 {{ count }} 笔已支付订单是在授权购买页买了商业版套餐，但没有开通商业版。买家站点仍按免费版。
    </template>
    <ElButton type="primary" size="small" :loading="acting" @click="reissue">一键补发商业版授权</ElButton>
  </ElAlert>
</template>

<script setup lang="ts">
  import { onMounted, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import { fetchCommercialPurchaseGaps, reissueCommercialPurchases } from '@/api/license-manage'

  defineOptions({ name: 'CommercialReissueBanner' })

  const count = ref(0)
  const acting = ref(false)

  async function load() {
    try {
      const data = await fetchCommercialPurchaseGaps()
      count.value = data?.count || 0
    } catch {
      count.value = 0
    }
  }

  async function reissue() {
    acting.value = true
    try {
      const data = await reissueCommercialPurchases()
      ElMessage.success(data?.msg || '已补发商业版授权。已经开通过的不会重复发放。')
      await load()
    } catch {
      ElMessage.error('补发商业版授权失败')
    } finally {
      acting.value = false
    }
  }

  onMounted(load)
</script>

<style scoped>
  .commercial-reissue {
    margin-bottom: 12px;
  }

  .commercial-reissue :deep(.el-alert__description) {
    margin-top: 8px;
  }
</style>
