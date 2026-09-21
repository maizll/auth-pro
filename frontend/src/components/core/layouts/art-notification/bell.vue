<template>
  <div ref="rootRef" class="art-notification-bell" @click.stop>
    <ElBadge :value="unread" :hidden="unread <= 0" :max="99" class="notice-badge">
      <button type="button" class="bell-btn" :title="$t('notice.title')" @click="toggle">
        <IconifyIcon icon="ri:notification-3-line" width="18" />
      </button>
    </ElBadge>
    <ArtNotification
      :value="open"
      :anchor="rootRef"
      @update:value="open = $event"
      @unread="unread = $event"
    />
  </div>
</template>

<script setup lang="ts">
  import { onBeforeUnmount, onMounted, ref } from 'vue'
  import { Icon as IconifyIcon } from '@iconify/vue'
  import { fetchNotificationUnreadCount, notificationBadgeCount } from '@/api/notifications'
  import ArtNotification from './index.vue'

  defineOptions({ name: 'ArtNotificationBell' })

  const open = ref(false)
  const unread = ref(0)
  const rootRef = ref<HTMLElement | null>(null)
  let timer: ReturnType<typeof setInterval> | null = null

  async function refreshUnread() {
    try {
      const { data } = await fetchNotificationUnreadCount()
      if (data.code !== 200) {
        console.warn('[notifications] 未读数拉取失败', data.msg || data.code)
        return
      }
      unread.value = notificationBadgeCount(data.data?.count, data.data?.todo)
    } catch (error) {
      console.warn('[notifications] 未读数拉取失败', error)
    }
  }

  function toggle() {
    open.value = !open.value
  }

  function onDocumentClick(event: MouseEvent) {
    if (!open.value) return
    const target = event.target as Node | null
    if (target && rootRef.value && !rootRef.value.contains(target)) {
      open.value = false
    }
  }

  onMounted(() => {
    void refreshUnread()
    timer = setInterval(refreshUnread, 60000)
    document.addEventListener('click', onDocumentClick)
  })

  onBeforeUnmount(() => {
    if (timer) clearInterval(timer)
    document.removeEventListener('click', onDocumentClick)
  })
</script>

<style scoped lang="scss">
  .art-notification-bell {
    position: relative;
    display: inline-flex;
    align-items: center;
  }

  .bell-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    padding: 0;
    color: var(--el-text-color-regular);
    cursor: pointer;
    background: transparent;
    border: 0;
    border-radius: 8px;
    transition: color 0.2s, background 0.2s;

    &:hover {
      color: var(--el-color-primary);
      background: var(--el-fill-color-light);
    }
  }

  :deep(.notice-badge .el-badge__content) {
    z-index: 1;
  }
</style>
