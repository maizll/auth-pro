<template>
  <div class="art-full-height">
    <ElCard shadow="never">
      <template #header>商店订单</template>
      <ElTable :data="list" v-loading="loading">
        <ElTableColumn prop="orderNo" label="订单号" min-width="180" />
        <ElTableColumn prop="title" label="项目" />
        <ElTableColumn prop="amountCents" label="金额（分）" />
        <ElTableColumn prop="status" label="状态" />
        <ElTableColumn label="操作" width="120">
          <template #default="{ row }">
            <ElButton v-if="row.status === 'paid'" link type="danger" @click="refund(row.orderNo)">退款</ElButton>
          </template>
        </ElTableColumn>
      </ElTable>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, ref } from 'vue'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import request from '@/utils/http'

  const loading = ref(false)
  const list = ref<any[]>([])

  async function load() {
    loading.value = true
    try {
      const data = await request.get<{ list: any[] }>({ url: '/api/v1/source/admin/store/orders' })
      list.value = data.list || []
    } finally {
      loading.value = false
    }
  }

  async function refund(orderNo: string) {
    await ElMessageBox.confirm(`确认退款 ${orderNo}？退款后商业版立即失效。`, '退款')
    await request.post({ url: `/api/v1/source/admin/store/orders/${orderNo}/refund`, data: { reason: 'admin refund' } })
    ElMessage.success('已退款')
    load()
  }

  onMounted(load)
</script>
