<!-- 通知详情：顶栏铃铛和通知中心共用。只有链接有意义时才给「前往处理」。 -->
<template>
  <AppDialog
    :model-value="modelValue"
    :title="title"
    size="sm"
    dialog-class="notice-detail-dialog"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div v-if="item" class="notice-detail">
      <div class="notice-detail__head">
        <span class="notice-detail__icon" :class="style.iconClass">
          <ArtSvgIcon :icon="style.icon" class="text-lg !bg-transparent" />
        </span>
        <div class="notice-detail__meta">
          <div class="notice-detail__title">{{ item.title }}</div>
          <div class="notice-detail__time">{{ formatNoticeTime(item.createdAt) }}</div>
        </div>
      </div>
      <p v-if="item.body" class="notice-detail__body">{{ item.body }}</p>
    </div>
    <template #footer>
      <ElButton @click="emit('update:modelValue', false)">关闭</ElButton>
      <ElButton v-if="actionable" type="primary" @click="go">前往处理</ElButton>
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
  import { computed } from 'vue'
  import AppDialog from '@/components/core/dialog/AppDialog.vue'
  import type { InAppNotification } from '@/api/notifications'
  import { formatNoticeTime, hasActionLink, noticeStyle } from './notice-meta'

  defineOptions({ name: 'NoticeDetailDialog' })

  const props = defineProps<{
    modelValue: boolean
    item: InAppNotification | null
  }>()

  const emit = defineEmits<{
    'update:modelValue': [value: boolean]
    go: [link: string]
  }>()

  const tabTitle: Record<string, string> = { notice: '通知', message: '消息', todo: '待办' }
  const title = computed(() => `${tabTitle[props.item?.category || 'notice'] || '通知'}详情`)
  const style = computed(() => noticeStyle(props.item?.eventType, props.item?.category))
  const actionable = computed(() => hasActionLink(props.item?.link))

  function go() {
    const link = props.item?.link?.trim() || ''
    emit('update:modelValue', false)
    if (hasActionLink(link)) emit('go', link)
  }
</script>

<style scoped>
  .notice-detail__head {
    display: flex;
    gap: 12px;
    align-items: flex-start;
  }

  .notice-detail__icon {
    display: inline-flex;
    flex: none;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border-radius: 8px;
  }

  .notice-detail__meta {
    min-width: 0;
  }

  .notice-detail__title {
    font-size: 15px;
    font-weight: 600;
    line-height: 22px;
    color: var(--el-text-color-primary);
    word-break: break-word;
  }

  .notice-detail__time {
    margin-top: 2px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
    white-space: nowrap;
  }

  .notice-detail__body {
    margin: 14px 0 0;
    font-size: 14px;
    line-height: 22px;
    color: var(--el-text-color-regular);
    word-break: break-word;
    white-space: pre-wrap;
  }
</style>
