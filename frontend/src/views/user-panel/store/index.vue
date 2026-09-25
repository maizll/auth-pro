<template>
  <div>
    <el-card shadow="never" class="block">
      <template #header>已绑定站点</template>
      <el-table :data="stations" v-loading="loading">
        <el-table-column prop="bindingId" label="绑定" />
        <el-table-column prop="domain" label="域名" />
        <el-table-column prop="status" label="状态" />
        <el-table-column prop="createdAt" label="时间" />
      </el-table>
    </el-card>
    <el-card shadow="never">
      <template #header>已购项目</template>
      <el-table :data="purchases">
        <el-table-column prop="orderNo" label="订单" />
        <el-table-column prop="title" label="项目" />
        <el-table-column prop="amountCents" label="金额（分）" />
        <el-table-column prop="status" label="状态" />
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
  import axios from 'axios'
  import { onMounted, ref } from 'vue'
  import { useRoute } from 'vue-router'

  const route = useRoute()
  const loading = ref(false)
  const stations = ref<any[]>([])
  const purchases = ref<any[]>([])
  const prefix = route.path.startsWith('/agent-panel') ? '/api/agent-panel' : '/api/user-panel'
  const tokenKey = route.path.startsWith('/agent-panel') ? 'agent_panel_token' : 'user_panel_token'

  onMounted(async () => {
    loading.value = true
    const headers = { Authorization: `Bearer ${localStorage.getItem(tokenKey) || ''}` }
    try {
      const [site, bought] = await Promise.all([
        axios.get(`${prefix}/store/stations`, { headers }),
        axios.get(`${prefix}/store/purchases`, { headers })
      ])
      stations.value = site.data?.data?.list || []
      purchases.value = bought.data?.data?.list || []
    } finally {
      loading.value = false
    }
  })
</script>

<style scoped>
  .block {
    margin-bottom: 16px;
  }
</style>
