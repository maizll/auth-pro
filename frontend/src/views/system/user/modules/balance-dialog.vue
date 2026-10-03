<!-- 调整用户余额：按增减量改，结果不能小于 0，每次都记流水和操作日志。编辑用户资料不再带余额。 -->
<template>
  <AppDialog v-model="visible" title="调整余额" size="sm" flow="short">
    <div class="balance-current">
      <span>{{ userLabel }} 当前余额</span>
      <strong>¥{{ current.toFixed(2) }}</strong>
    </div>
    <ElForm label-position="top">
      <ElFormItem label="方式">
        <ElRadioGroup v-model="direction">
          <ElRadioButton value="add">增加</ElRadioButton>
          <ElRadioButton value="deduct">扣减</ElRadioButton>
        </ElRadioGroup>
      </ElFormItem>
      <ElFormItem label="金额（元）">
        <ElInputNumber
          v-model="amount"
          :min="0.01"
          :max="direction === 'deduct' ? Math.max(current, 0.01) : 1000000"
          :precision="2"
          :step="10"
          controls-position="right"
          class="balance-amount"
        />
      </ElFormItem>
      <ElFormItem label="原因（会记进流水）">
        <ElInput
          v-model="remark"
          maxlength="100"
          show-word-limit
          placeholder="例如：线下转账充值"
        />
      </ElFormItem>
    </ElForm>
    <p class="balance-after" :class="{ invalid: after < 0 }">
      调整后余额 ¥{{ after.toFixed(2) }}{{ after < 0 ? '，不能小于 0' : '' }}
    </p>
    <template #footer>
      <ElButton @click="visible = false">取消</ElButton>
      <ElButton type="primary" :loading="saving" :disabled="!canSubmit" @click="submit">
        确认调整
      </ElButton>
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
  import AppDialog from '@/components/core/dialog/AppDialog.vue'
  import { fetchAdjustUserBalance } from '@/api/system-manage'

  const props = defineProps<{
    user: { userId?: number; userName?: string; userEmail?: string; balance?: number }
  }>()
  const visible = defineModel<boolean>({ required: true })
  const emit = defineEmits<{ done: [] }>()

  const direction = ref<'add' | 'deduct'>('add')
  const amount = ref<number | undefined>(undefined)
  const remark = ref('')
  const saving = ref(false)

  const current = computed(() => Number(props.user?.balance || 0))
  const userLabel = computed(() => props.user?.userName || props.user?.userEmail || '该用户')
  const delta = computed(() => {
    const value = Number(amount.value || 0)
    return direction.value === 'add' ? value : -value
  })
  const after = computed(() => Math.round((current.value + delta.value) * 100) / 100)
  const canSubmit = computed(
    () => Number(amount.value || 0) > 0 && remark.value.trim() !== '' && after.value >= 0
  )

  watch(visible, (open) => {
    if (!open) return
    direction.value = 'add'
    amount.value = undefined
    remark.value = ''
  })

  async function submit() {
    if (!props.user?.userId || !canSubmit.value) return
    saving.value = true
    try {
      await fetchAdjustUserBalance(props.user.userId, delta.value, remark.value.trim())
      ElMessage.success('余额已调整')
      visible.value = false
      emit('done')
    } catch {
      // 接口错误由请求层提示
    } finally {
      saving.value = false
    }
  }
</script>

<style scoped lang="scss">
  .balance-current {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    padding: 12px 14px;
    margin-bottom: 16px;
    border-radius: 8px;
    background: var(--el-color-primary-light-9);
    color: var(--el-text-color-regular);

    strong {
      font-size: 20px;
      color: var(--el-text-color-primary);
    }
  }

  .balance-amount {
    width: 100%;
  }

  .balance-after {
    margin: 4px 0 0;
    font-size: 13px;
    color: var(--el-text-color-secondary);

    &.invalid {
      color: var(--el-color-danger);
    }
  }
</style>
