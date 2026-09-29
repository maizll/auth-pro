<template>
  <AppDialog v-model="appConfirmState.open" :title="appConfirmState.title" size="sm" flow="short">
    <p class="app-confirm__text">{{ appConfirmState.message }}</p>
    <ElInput v-if="appConfirmState.prompt" v-model="appConfirmState.value" />
    <template #footer>
      <ElButton @click="settleAppConfirm(false)">{{ appConfirmState.cancelText }}</ElButton>
      <ElButton
        :type="appConfirmState.danger ? 'danger' : 'primary'"
        @click="settleAppConfirm(true)"
      >
        {{ appConfirmState.confirmText }}
      </ElButton>
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
  import AppDialog from '@/components/core/dialog/AppDialog.vue'
  import { appConfirmState, settleAppConfirm } from '@/utils/app-confirm'

  defineOptions({ name: 'AppConfirmHost' })
</script>

<style scoped>
  .app-confirm__text {
    margin: 0 0 12px;
    line-height: 1.6;
  }
</style>
