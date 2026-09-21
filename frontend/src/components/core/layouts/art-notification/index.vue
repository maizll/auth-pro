<!-- 四端共用站内通知：通知 / 消息 / 待办 -->
<template>
  <Teleport to="body">
  <div
    class="art-notification-panel"
    :style="panelStyle"
    v-show="visible"
    @click.stop
  >
    <div class="flex-cb px-3.5 mt-3.5">
      <span class="text-base font-medium text-g-800">{{ $t('notice.title') }}</span>
      <span
        class="text-xs text-g-800 px-1.5 py-1 c-p select-none rounded hover:bg-g-200"
        @click="handleReadAll"
      >
        {{ $t('notice.btnRead') }}
      </span>
    </div>

    <ul class="box-border flex items-end w-full h-12.5 px-3.5 border-b-d">
      <li
        v-for="(item, index) in barList"
        :key="index"
        class="h-12 leading-12 mr-5 overflow-hidden text-[13px] text-g-700 c-p select-none"
        :class="{ 'bar-active': barActiveIndex === index }"
        @click="changeBar(index)"
      >
        {{ item.name }} ({{ item.num }})
      </li>
    </ul>

    <div class="notice-body">
      <div ref="scrollRef" class="notice-scroll scrollbar-thin">
        <ul v-if="barActiveIndex === 0">
          <li
            v-for="item in noticeList"
            :key="item.id || item.eventType + item.createdAt"
            class="box-border flex-c px-3.5 py-3.5 c-p last:border-b-0 hover:bg-g-200/60"
            :class="{ 'is-unread': !item.read && !item.derived }"
            @click="handleItemClick(item)"
          >
            <div
              class="size-9 leading-9 text-center rounded-lg flex-cc"
              :class="[getNoticeStyle(noticeTypeOf(item)).iconClass]"
            >
              <ArtSvgIcon
                class="text-lg !bg-transparent"
                :icon="getNoticeStyle(noticeTypeOf(item)).icon"
              />
            </div>
            <div class="w-[calc(100%-45px)] ml-3.5">
              <h4 class="text-sm font-normal leading-5.5 text-g-900">{{ item.title }}</h4>
              <p v-if="item.body" class="mt-1 text-xs text-g-600 line-clamp-2">{{ item.body }}</p>
              <p class="mt-1.5 text-xs text-g-500">{{ formatNoticeTime(item.createdAt) }}</p>
            </div>
          </li>
        </ul>

        <ul v-else-if="barActiveIndex === 1">
          <li
            v-for="item in msgList"
            :key="item.id || item.eventType + item.createdAt"
            class="box-border flex-c px-3.5 py-3.5 c-p last:border-b-0 hover:bg-g-200/60"
            :class="{ 'is-unread': !item.read && !item.derived }"
            @click="handleItemClick(item)"
          >
            <div
              class="size-9 leading-9 text-center rounded-lg flex-cc bg-success/12 text-success"
            >
              <ArtSvgIcon class="text-lg !bg-transparent" icon="ri:message-3-line" />
            </div>
            <div class="w-[calc(100%-45px)] ml-3.5">
              <h4 class="text-sm font-normal leading-5.5 text-g-900">{{ item.title }}</h4>
              <p v-if="item.body" class="mt-1 text-xs text-g-600 line-clamp-2">{{ item.body }}</p>
              <p class="mt-1.5 text-xs text-g-500">{{ formatNoticeTime(item.createdAt) }}</p>
            </div>
          </li>
        </ul>

        <ul v-else>
          <li
            v-for="item in pendingList"
            :key="item.eventType + item.title"
            class="box-border px-3.5 py-3.5 c-p last:border-b-0 hover:bg-g-200/60"
            @click="handleItemClick(item)"
          >
            <h4 class="text-sm font-medium leading-5.5 text-g-900">{{ item.title }}</h4>
            <p v-if="item.body" class="mt-1 text-xs text-g-600">{{ item.body }}</p>
          </li>
        </ul>

        <div v-if="currentTabIsEmpty" class="notice-empty text-g-500 text-center">
          <ArtSvgIcon icon="system-uicons:inbox" class="text-5xl" />
          <p class="mt-3.5 text-xs">{{ $t('notice.text[0]') }}{{ barList[barActiveIndex].name }}</p>
        </div>
      </div>

      <div class="notice-footer box-border w-full px-3.5">
        <ElButton class="w-full" @click="handleViewAll" v-ripple>
          {{ $t('notice.viewAll') }}
        </ElButton>
      </div>
    </div>
  </div>
  </Teleport>
</template>

<script setup lang="ts">
  import { computed, onBeforeUnmount, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRouter } from 'vue-router'
  import {
    fetchNotifications,
    markAllNotificationsRead,
    markNotificationRead,
    notificationDefaultLink,
    type InAppNotification,
    type NotificationTab
  } from '@/api/notifications'

  defineOptions({ name: 'ArtNotification' })

  type NoticeType = 'email' | 'message' | 'collection' | 'user' | 'notice'

  const { t } = useI18n()
  const router = useRouter()

  const props = defineProps<{
    value: boolean
    anchor?: HTMLElement | null
  }>()

  const emit = defineEmits<{
    'update:value': [value: boolean]
    unread: [count: number]
  }>()

  const show = ref(false)
  const visible = ref(false)
  const barActiveIndex = ref(0)
  const scrollRef = ref<HTMLElement | null>(null)
  const placement = ref({ top: 64, left: 8, width: 360, maxHeight: 480 })
  const noticeList = ref<InAppNotification[]>([])
  const msgList = ref<InAppNotification[]>([])
  const pendingList = ref<InAppNotification[]>([])

  const barList = computed(() => [
    { name: t('notice.bar[0]'), num: noticeList.value.length, tab: 'notice' as NotificationTab },
    { name: t('notice.bar[1]'), num: msgList.value.length, tab: 'message' as NotificationTab },
    { name: t('notice.bar[2]'), num: pendingList.value.length, tab: 'todo' as NotificationTab }
  ])

  const currentTabIsEmpty = computed(() => {
    return [noticeList.value, msgList.value, pendingList.value][barActiveIndex.value]?.length === 0
  })

  const noticeStyleMap: Record<NoticeType, { icon: string; iconClass: string }> = {
    email: { icon: 'ri:mail-line', iconClass: 'bg-warning/12 text-warning' },
    message: { icon: 'ri:volume-down-line', iconClass: 'bg-success/12 text-success' },
    collection: { icon: 'ri:heart-3-line', iconClass: 'bg-danger/12 text-danger' },
    user: { icon: 'ri:user-add-line', iconClass: 'bg-info/12 text-info' },
    notice: { icon: 'ri:notification-3-line', iconClass: 'bg-theme/12 text-theme' }
  }

  const getNoticeStyle = (type: NoticeType) => noticeStyleMap[type] || noticeStyleMap.notice

  function noticeTypeOf(item: InAppNotification): NoticeType {
    if (item.eventType.includes('password') || item.eventType.includes('expir')) return 'email'
    if (item.eventType.includes('apply') || item.eventType.includes('ticket')) return 'user'
    if (item.eventType.includes('reject') || item.eventType.includes('deprecat')) return 'collection'
    if (item.eventType.includes('approved') || item.eventType.includes('paid')) return 'message'
    return 'notice'
  }

  function formatNoticeTime(raw?: string) {
    if (!raw) return ''
    const date = new Date(raw)
    if (Number.isNaN(date.getTime())) return raw
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
  }

  async function loadTab(tab: NotificationTab) {
    try {
      const { data } = await fetchNotifications(tab)
      if (data.code !== 200) {
        console.warn('[notifications] 通知列表拉取失败', tab, data.msg || data.code)
        return
      }
      const list = data.data?.list || []
      if (tab === 'notice') noticeList.value = list
      if (tab === 'message') msgList.value = list
      if (tab === 'todo') pendingList.value = list
    } catch (error) {
      console.warn('[notifications] 通知列表拉取失败', tab, error)
    }
  }

  async function refreshAll() {
    await Promise.all([loadTab('notice'), loadTab('message'), loadTab('todo')])
    emit('unread', unreadNow())
  }

  function changeBar(index: number) {
    barActiveIndex.value = index
    if (scrollRef.value) scrollRef.value.scrollTop = 0
  }

  function updatePlacement() {
    const margin = 8
    const viewportWidth = window.innerWidth
    const viewportHeight = window.innerHeight
    const width = Math.min(360, Math.max(220, viewportWidth - margin * 2))
    const anchor = props.anchor?.getBoundingClientRect()
    let left = viewportWidth - width - margin
    let top = margin
    let maxHeight = Math.min(520, viewportHeight - margin * 2)
    if (anchor) {
      left = anchor.right - width
      const belowTop = anchor.bottom + margin
      const spaceBelow = viewportHeight - belowTop - margin
      const spaceAbove = anchor.top - margin * 2
      if (spaceBelow >= 200 || spaceBelow >= spaceAbove) {
        top = belowTop
        maxHeight = Math.min(520, spaceBelow)
      } else {
        maxHeight = Math.min(520, spaceAbove)
        top = Math.max(margin, anchor.top - margin - maxHeight)
      }
    }
    left = Math.min(Math.max(margin, left), Math.max(margin, viewportWidth - width - margin))
    top = Math.max(margin, Math.min(top, viewportHeight - margin))
    maxHeight = Math.min(maxHeight, viewportHeight - top - margin)
    if (maxHeight < 120) {
      top = margin
      maxHeight = viewportHeight - margin * 2
    }
    placement.value = { top, left, width, maxHeight: Math.max(80, maxHeight) }
  }

  const panelStyle = computed(() => ({
    transform: show.value ? 'scaleY(1)' : 'scaleY(0.9)',
    opacity: show.value ? 1 : 0,
    top: `${placement.value.top}px`,
    left: `${placement.value.left}px`,
    width: `${placement.value.width}px`,
    height: `${placement.value.maxHeight}px`,
    maxHeight: `${placement.value.maxHeight}px`
  }))

  let placementBound = false

  function bindPlacement(active: boolean) {
    if (active === placementBound) return
    placementBound = active
    if (active) {
      window.addEventListener('resize', updatePlacement)
      window.addEventListener('scroll', updatePlacement, true)
      return
    }
    window.removeEventListener('resize', updatePlacement)
    window.removeEventListener('scroll', updatePlacement, true)
  }

  async function handleReadAll() {
    try {
      await markAllNotificationsRead()
    } catch (error) {
      console.warn('[notifications] 全部已读失败', error)
    }
    await refreshAll()
  }

  async function handleItemClick(item: InAppNotification) {
    if (item.id && !item.derived && !item.read) {
      try {
        await markNotificationRead(item.id)
        item.read = true
      } catch (error) {
        console.warn('[notifications] 标记已读失败', error)
      }
    }
    emit('unread', unreadNow())
    if (item.link) {
      emit('update:value', false)
      router.push(item.link)
    }
  }

  function unreadNow() {
    const persisted =
      noticeList.value.filter((item) => !item.read && !item.derived).length +
      msgList.value.filter((item) => !item.read && !item.derived).length
    return persisted + pendingList.value.length
  }

  function handleViewAll() {
    const tab = barList.value[barActiveIndex.value]?.tab || 'notice'
    const first = [noticeList.value, msgList.value, pendingList.value][barActiveIndex.value]?.[0]
    emit('update:value', false)
    router.push(first?.link || notificationDefaultLink(tab))
  }

  function showNotice(open: boolean) {
    if (open) {
      updatePlacement()
      bindPlacement(true)
      visible.value = true
      void refreshAll()
      setTimeout(() => {
        show.value = true
      }, 5)
    } else {
      show.value = false
      bindPlacement(false)
      setTimeout(() => {
        visible.value = false
      }, 350)
    }
  }

  onBeforeUnmount(() => {
    bindPlacement(false)
  })

  watch(
    () => props.value,
    (newValue) => {
      showNotice(newValue)
    }
  )
</script>

<style scoped>
  .art-notification-panel {
    position: fixed;
    z-index: 2100;
    display: flex;
    flex-direction: column;
    box-sizing: border-box;
    overflow: hidden;
    line-height: 1.5;
    color: var(--el-text-color-primary, #303133);
    background-color: var(--default-box-color, #fff) !important;
    border: 1px solid var(--el-border-color-light, rgba(0, 0, 0, 0.08));
    border-radius: 10px;
    box-shadow:
      0 12px 32px rgba(0, 0, 0, 0.16),
      0 2px 8px rgba(0, 0, 0, 0.08);
    transform-origin: top right;
    transition:
      opacity 0.2s ease,
      transform 0.2s ease;
  }

  .notice-body {
    display: flex;
    flex: 1 1 auto;
    flex-direction: column;
    min-height: 0;
  }

  .notice-scroll {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
  }

  .notice-empty {
    padding: 48px 16px 32px;
  }

  .notice-footer {
    flex: none;
    padding-top: 8px;
    padding-bottom: 12px;
    background-color: var(--default-box-color, #fff) !important;
  }

  .bar-active {
    color: var(--theme-color) !important;
    border-bottom: 2px solid var(--theme-color);
  }

  .is-unread h4 {
    font-weight: 600;
  }

  .scrollbar-thin::-webkit-scrollbar {
    width: 5px !important;
  }

  .dark .scrollbar-thin::-webkit-scrollbar-track {
    background-color: var(--default-box-color);
  }

  .dark .scrollbar-thin::-webkit-scrollbar-thumb {
    background-color: #222 !important;
  }
</style>
