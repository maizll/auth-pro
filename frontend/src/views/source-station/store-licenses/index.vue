<template>
  <div class="art-full-height">
    <ElCard shadow="never">
      <template #header>主授权与权益</template>
      <ElTable :data="list" v-loading="loading">
        <ElTableColumn prop="licenseNo" label="授权号" />
        <ElTableColumn prop="ownerType" label="归属" />
        <ElTableColumn prop="ownerId" label="账号" />
        <ElTableColumn prop="edition" label="版本" />
        <ElTableColumn prop="licenseStatus" label="授权状态" />
        <ElTableColumn label="操作" min-width="220">
          <template #default="{ row }">
            <ElButton link type="primary" @click="grant(row.id)">授予商业版</ElButton>
            <ElButton link type="danger" @click="revoke(row.id)">吊销</ElButton>
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
      const data = await request.get<{ list: any[] }>({ url: '/api/v1/source/admin/store/licenses' })
      list.value = data.list || []
    } finally {
      loading.value = false
    }
  }

  async function grant(id: number) {
    await request.post({ url: `/api/v1/source/admin/store/licenses/${id}/grant`, data: { period: 'permanent' } })
    ElMessage.success('已授予')
    load()
  }

  async function revoke(id: number) {
    const { value } = await ElMessageBox.prompt('请填写吊销原因', '吊销商业版')
    await request.post({ url: `/api/v1/source/admin/store/licenses/${id}/revoke`, data: { reason: value } })
    ElMessage.success('已吊销，付费能力立即停止')
    load()
  }

  onMounted(load)
</script>
