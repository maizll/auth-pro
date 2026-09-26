<template>
  <el-dialog
    :model-value="modelValue"
    title="改为收费"
    :width="narrow ? '92%' : '520px'"
    append-to-body
    @close="emit('update:modelValue', false)"
  >
    <p class="switch-lead">这条目前免费，而且已经公开。保存后会立刻生效：</p>
    <ul class="switch-list">
      <li>公开下载地址作废，不能再直接下载。</li>
      <li>安装包迁入已配置的收费仓库；没配置时改由本站私有托管，下载走付费校验。</li>
      <li>外链会由本站自动拉取后再托管，买家看不到原来的地址。</li>
    </ul>
    <el-radio-group v-model="choice" class="switch-options">
      <el-radio value="grandfather">已下载过的老用户继续免费使用</el-radio>
      <el-radio value="purchase_only">所有人都需购买</el-radio>
    </el-radio-group>
    <p class="card-hint">
      默认保留老用户免费。系统按已有下载记录，给对应授权或账号发放该条目的免费权益，重复保存不会重复发放。选「所有人都需购买」时，已发放的这类免费权益会收回，商业版也不再直接包含这一条。
    </p>
    <p v-if="developer" class="card-hint">开发者的付费条目仍不能上架。保存后会从公开目录下架，并继续走现有审核规则。</p>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" @click="confirm">确认改为收费</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
  import { ref, watch } from 'vue'

  const props = defineProps<{
    modelValue: boolean
    developer?: boolean
    narrow?: boolean
  }>()

  const emit = defineEmits<{
    'update:modelValue': [value: boolean]
    confirm: [policy: 'grandfather' | 'purchase_only']
  }>()

  const choice = ref<'grandfather' | 'purchase_only'>('grandfather')

  watch(
    () => props.modelValue,
    (open) => {
      if (open) choice.value = 'grandfather'
    }
  )

  function confirm() {
    emit('confirm', choice.value)
    emit('update:modelValue', false)
  }
</script>

<style scoped>
  .switch-lead {
    margin: 0 0 8px;
  }

  .switch-list {
    margin: 0 0 12px;
    padding-left: 1.2em;
  }

  .switch-options {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 8px;
    margin-bottom: 8px;
  }
</style>
