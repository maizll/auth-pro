<!-- 关闭一个应用的商业版出售。默认只停新售；作废全部权益只有超级管理员能选，并且要输入应用名确认。 -->
<template>
  <AppDialog
    :model-value="modelValue"
    size="md"
    flow="short"
    :close-on-click-modal="false"
    @update:model-value="emit('update:modelValue', $event)"
    @open="reset"
  >
    <template #header>
      <span class="close-dialog__title">关闭「{{ appName }}」的商业版出售</span>
    </template>
    <div class="close-dialog__head">
      <span class="close-dialog__icon">!</span>
      <div>该应用还有已售出的商业版。请先选择关闭方式，已售出的权益不会被自动作废。</div>
    </div>
    <div class="close-dialog__impact">
      <div
        ><b>{{ stats.activeLicenses }}</b
        ><span>有效商业版授权</span></div
      >
      <div
        ><b>{{ stats.boundSites }}</b
        ><span>已绑定站点</span></div
      >
      <div
        ><b>{{ stats.pendingOrders }}</b
        ><span>待支付订单</span></div
      >
    </div>
    <label class="close-dialog__opt" :class="{ 'is-on': action === 'stop' }">
      <ElRadio v-model="action" value="stop">仅停止新售（推荐）</ElRadio>
      <div class="close-dialog__desc">
        顶栏不再显示购买入口，新下单会提示已停售，待支付订单到期作废。已售授权继续有效、照常刷新，已绑定站点不受影响。
      </div>
    </label>
    <label
      v-if="isSuper"
      class="close-dialog__opt is-danger"
      :class="{ 'is-on': action === 'revoke' }"
    >
      <ElRadio v-model="action" value="revoke">停止新售并作废全部商业版权益</ElRadio>
      <div class="close-dialog__desc">
        {{ stats.activeLicenses }}
        个授权下次刷新后回到免费版，绑定保留。此操作不能撤销，已付款的订单需要另行退款。
      </div>
    </label>
    <ElForm label-position="top" class="close-dialog__form" @submit.prevent>
      <ElFormItem v-if="action === 'revoke'" :label="`请输入应用名称「${appName}」以确认`">
        <ElInput v-model="confirmName" :placeholder="appName" />
      </ElFormItem>
      <ElFormItem v-if="legacyDefault" label="老客户端以后的新绑定改到">
        <ElSelect v-model="legacyTo" class="close-dialog__select">
          <ElOption label="不改，仍由本应用接收（已停售）" :value="-1" />
          <ElOption
            v-for="item in legacyTargets"
            :key="item.id"
            :label="item.name"
            :value="item.id"
          />
          <ElOption label="不再接收老客户端" :value="0" />
        </ElSelect>
        <div class="close-dialog__desc">已绑定的站点不受影响。</div>
      </ElFormItem>
    </ElForm>
    <ElAlert v-if="error" :title="error" type="error" :closable="false" show-icon />
    <template #footer>
      <ElButton :disabled="saving" @click="emit('update:modelValue', false)">取消</ElButton>
      <ElButton v-if="action === 'stop'" type="primary" :loading="saving" @click="submit">
        停止新售
      </ElButton>
      <ElButton
        v-else
        type="danger"
        :loading="saving"
        :disabled="confirmName.trim() !== appName.trim()"
        @click="submit"
      >
        作废并关闭
      </ElButton>
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
  import { ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import AppDialog from '@/components/core/dialog/AppDialog.vue'
  import { closeAppCommercial, type AppCommercialStats } from '@/api/license-manage'

  defineOptions({ name: 'AppCommercialCloseDialog' })

  const props = defineProps<{
    modelValue: boolean
    appId: number
    appName: string
    stats: AppCommercialStats
    isSuper: boolean
    legacyDefault: boolean
    /** 可以接收老客户端的其他出售中应用 */
    legacyTargets: { id: number; name: string }[]
  }>()

  const emit = defineEmits<{
    (e: 'update:modelValue', value: boolean): void
    (e: 'done'): void
  }>()

  const action = ref<'stop' | 'revoke'>('stop')
  const confirmName = ref('')
  const legacyTo = ref(-1)
  const saving = ref(false)
  const error = ref('')

  function reset() {
    action.value = 'stop'
    confirmName.value = ''
    legacyTo.value = props.legacyTargets[0]?.id ?? -1
    error.value = ''
  }

  async function submit() {
    saving.value = true
    error.value = ''
    try {
      const result = await closeAppCommercial(props.appId, {
        action: action.value,
        confirmName: action.value === 'revoke' ? confirmName.value.trim() : undefined,
        legacyTo: props.legacyDefault && legacyTo.value >= 0 ? legacyTo.value : undefined
      })
      ElMessage.success(result?.msg || '已停止新售')
      emit('update:modelValue', false)
      emit('done')
    } catch (e) {
      error.value = e instanceof Error && e.message ? e.message : '关闭失败，请稍后重试'
    } finally {
      saving.value = false
    }
  }
</script>

<style scoped lang="scss">
  .close-dialog {
    &__title {
      font-size: 16px;
      font-weight: 600;
    }

    &__head {
      display: flex;
      gap: 10px;
      align-items: flex-start;
      padding: 10px 12px;
      margin-bottom: 12px;
      color: var(--el-color-danger);
      background: var(--el-color-danger-light-9);
      border-radius: 6px;
    }

    &__icon {
      display: inline-flex;
      flex: none;
      align-items: center;
      justify-content: center;
      width: 20px;
      height: 20px;
      font-weight: 700;
      color: #fff;
      background: var(--el-color-danger);
      border-radius: 50%;
    }

    &__impact {
      display: grid;
      grid-template-columns: repeat(3, 1fr);
      gap: 8px;
      margin-bottom: 12px;

      div {
        display: flex;
        flex-direction: column;
        align-items: center;
        padding: 8px 4px;
        background: var(--el-fill-color-light);
        border-radius: 6px;
      }

      b {
        font-size: 20px;
      }

      span {
        font-size: 12px;
        color: var(--el-text-color-secondary);
      }
    }

    &__opt {
      display: block;
      padding: 8px 12px;
      margin-bottom: 8px;
      cursor: pointer;
      border: 1px solid var(--el-border-color);
      border-radius: 6px;

      &.is-on {
        border-color: var(--el-color-primary);
      }

      &.is-danger.is-on {
        border-color: var(--el-color-danger);
      }
    }

    &__desc {
      font-size: 12px;
      line-height: 1.5;
      color: var(--el-text-color-secondary);
    }

    &__form {
      margin-top: 8px;
    }

    &__select {
      width: 100%;
    }
  }
</style>
