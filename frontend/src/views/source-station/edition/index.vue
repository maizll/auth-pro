<template>
  <div class="art-full-height">
    <ElCard shadow="never">
      <template #header>商业版设置</template>
      <ElForm inline>
        <ElFormItem label="名称"><ElInput v-model="form.name" /></ElFormItem>
        <ElFormItem label="周期">
          <ElSelect v-model="form.period" style="width: 120px">
            <ElOption label="永久" value="permanent" />
            <ElOption label="年付（暂不开放购买）" value="yearly" />
          </ElSelect>
        </ElFormItem>
        <ElFormItem label="价格（分）"><ElInputNumber v-model="form.priceCents" :min="1" /></ElFormItem>
        <ElFormItem label="启用"><ElSwitch v-model="form.enabled" /></ElFormItem>
        <ElButton type="primary" @click="save">保存套餐</ElButton>
      </ElForm>
      <ElTable :data="list" v-loading="loading">
        <ElTableColumn prop="name" label="名称" />
        <ElTableColumn prop="period" label="周期" />
        <ElTableColumn prop="priceCents" label="价格（分）" />
        <ElTableColumn prop="enabled" label="启用" />
      </ElTable>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, reactive, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import request from '@/utils/http'

  const loading = ref(false)
  const list = ref<any[]>([])
  const form = reactive({ name: '商业版', period: 'permanent', priceCents: 9900, enabled: true, sort: 1 })

  async function load() {
    loading.value = true
    try {
      const data = await request.get<{ list: any[] }>({ url: '/api/v1/source/admin/store/plans' })
      list.value = data.list || []
    } finally {
      loading.value = false
    }
  }

  async function save() {
    await request.post({ url: '/api/v1/source/admin/store/plans', data: form })
    ElMessage.success('已保存')
    load()
  }

  onMounted(load)
</script>
