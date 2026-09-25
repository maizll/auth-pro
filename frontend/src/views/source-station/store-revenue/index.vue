<template>
  <div class="art-full-height">
    <ElCard shadow="never">
      <template #header>商业版收入</template>
      <p class="hint">阶段 1 商业版收入全部归平台，账本按 100% 记为已结算。</p>
      <ElTable :data="list" v-loading="loading">
        <ElTableColumn prop="orderId" label="订单" />
        <ElTableColumn prop="grossCents" label="总额（分）" />
        <ElTableColumn prop="netCents" label="净额（分）" />
        <ElTableColumn prop="status" label="状态" />
        <ElTableColumn prop="note" label="备注" />
      </ElTable>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, ref } from 'vue'
  import request from '@/utils/http'

  const loading = ref(false)
  const list = ref<any[]>([])

  onMounted(async () => {
    loading.value = true
    try {
      const data = await request.get<{ list: any[] }>({ url: '/api/v1/source/admin/store/revenue' })
      list.value = data.list || []
    } finally {
      loading.value = false
    }
  })
</script>

<style scoped>
  .hint {
    margin: 0 0 12px;
    color: var(--el-text-color-secondary);
  }
</style>
