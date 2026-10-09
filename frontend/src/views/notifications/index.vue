<!-- 通知中心：后台、用户面板、代理商面板、开发者面板共用。顶栏铃铛「查看全部」进入。 -->
<template>
  <div class="notice-center art-full-height">
    <ElCard class="notice-center__card" shadow="never">
      <template #header>
        <div class="notice-center__header">
          <h2>通知中心</h2>
          <div class="notice-center__actions">
            <ElCheckbox v-if="tab !== 'todo'" v-model="unreadOnly" @change="reload">
              只看未读
            </ElCheckbox>
            <ElButton
              v-if="tab !== 'todo'"
              :loading="readingAll"
              :disabled="!hasUnread"
              @click="handleReadAll"
            >
              全部已读
            </ElButton>
          </div>
        </div>
      </template>

      <ElTabs v-model="tab" class="notice-center__tabs" @tab-change="onTabChange">
        <ElTabPane v-for="item in tabs" :key="item.value" :name="item.value" :label="item.label" />
      </ElTabs>

      <div v-loading="loading" class="notice-center__list">
        <button
          v-for="item in list"
          :key="item.id || item.eventType + item.title"
          type="button"
          class="notice-row"
          :class="{ 'is-unread': isUnread(item) }"
          @click="openDetail(item)"
        >
          <span class="notice-row__icon" :class="noticeStyle(item.eventType, tab).iconClass">
            <ArtSvgIcon
              :icon="noticeStyle(item.eventType, tab).icon"
              class="text-lg !bg-transparent"
            />
          </span>
          <span class="notice-row__main">
            <span class="notice-row__title">
              <i v-if="isUnread(item)" class="notice-row__dot" aria-label="未读" />
              {{ item.title }}
            </span>
            <span v-if="item.body" class="notice-row__body">{{ item.body }}</span>
          </span>
          <span v-if="item.createdAt && !item.derived" class="notice-row__time">
            {{ formatNoticeTime(item.createdAt) }}
          </span>
        </button>

        <ElEmpty
          v-if="!loading && list.length === 0"
          :image-size="72"
          :description="unreadOnly && tab !== 'todo' ? '没有未读的' + tabLabel : '暂无' + tabLabel"
        />

        <div v-if="hasMore" class="notice-center__more">
          <ElButton :loading="loadingMore" @click="loadMore">加载更多</ElButton>
        </div>
      </div>
    </ElCard>

    <NoticeDetailDialog v-model="detailOpen" :item="detailItem" @go="(link) => router.push(link)" />
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, ref, watch } from 'vue'
  import { useRoute, useRouter } from 'vue-router'
  import { ElMessage } from 'element-plus'
  import {
    fetchNotifications,
    markAllNotificationsRead,
    markNotificationRead,
    type InAppNotification
  } from '@/api/notifications'
  import NoticeDetailDialog from '@/components/core/layouts/art-notification/NoticeDetailDialog.vue'
  import {
    formatNoticeTime,
    normalizeNoticeTab,
    noticeStyle,
    type NoticeTab
  } from '@/components/core/layouts/art-notification/notice-meta'

  defineOptions({ name: 'NotificationCenter' })

  const PAGE_SIZE = 20
  const route = useRoute()
  const router = useRouter()

  const tabs: { value: NoticeTab; label: string }[] = [
    { value: 'notice', label: '通知' },
    { value: 'message', label: '消息' },
    { value: 'todo', label: '待办' }
  ]
  const tab = ref<NoticeTab>(normalizeNoticeTab(route.query.tab))
  const unreadOnly = ref(false)
  const list = ref<InAppNotification[]>([])
  const page = ref(1)
  const hasMore = ref(false)
  const loading = ref(false)
  const loadingMore = ref(false)
  const readingAll = ref(false)
  const detailOpen = ref(false)
  const detailItem = ref<InAppNotification | null>(null)

  const tabLabel = computed(() => tabs.find((item) => item.value === tab.value)?.label || '通知')
  const hasUnread = computed(() => list.value.some((item) => isUnread(item)))

  function isUnread(item: InAppNotification) {
    return !item.read && !item.derived
  }

  async function fetchPage(target: number) {
    // 待办是按当前数据实时算出来的，没有分页和已读。
    const paging = tab.value === 'todo' ? undefined : { page: target, size: PAGE_SIZE }
    const { data } = await fetchNotifications(tab.value, unreadOnly.value, paging)
    if (data.code !== 200) throw new Error(data.msg || '读取通知失败')
    return { items: data.data?.list || [], more: Boolean(data.data?.hasMore) }
  }

  async function reload() {
    loading.value = true
    try {
      const { items, more } = await fetchPage(1)
      list.value = items
      page.value = 1
      hasMore.value = more
    } catch (error) {
      list.value = []
      hasMore.value = false
      ElMessage.error(error instanceof Error ? error.message : '读取通知失败')
    } finally {
      loading.value = false
    }
  }

  async function loadMore() {
    loadingMore.value = true
    try {
      const { items, more } = await fetchPage(page.value + 1)
      list.value = [...list.value, ...items]
      page.value += 1
      hasMore.value = more
    } catch (error) {
      ElMessage.error(error instanceof Error ? error.message : '读取通知失败')
    } finally {
      loadingMore.value = false
    }
  }

  function onTabChange(name: string | number) {
    const next = normalizeNoticeTab(name)
    if (route.query.tab !== next) router.replace({ query: { ...route.query, tab: next } })
    void reload()
  }

  async function openDetail(item: InAppNotification) {
    detailItem.value = item
    detailOpen.value = true
    if (!item.id || item.derived || item.read) return
    try {
      await markNotificationRead(item.id)
      item.read = true
    } catch (error) {
      console.warn('[notifications] 标记已读失败', error)
    }
  }

  async function handleReadAll() {
    readingAll.value = true
    try {
      await markAllNotificationsRead()
      ElMessage.success('已全部标为已读')
      await reload()
    } catch {
      ElMessage.error('全部已读失败，请稍后再试')
    } finally {
      readingAll.value = false
    }
  }

  watch(
    () => route.query.tab,
    (value) => {
      const next = normalizeNoticeTab(value)
      if (next !== tab.value) {
        tab.value = next
        void reload()
      }
    }
  )

  onMounted(reload)
</script>

<style scoped>
  .notice-center__header {
    display: flex;
    gap: 12px;
    align-items: center;
    justify-content: space-between;
  }

  .notice-center__header h2 {
    margin: 0;
    font-size: 16px;
    font-weight: 600;
    white-space: nowrap;
  }

  .notice-center__actions {
    display: flex;
    flex-wrap: nowrap;
    gap: 12px;
    align-items: center;
  }

  .notice-center__tabs :deep(.el-tabs__header) {
    margin-bottom: 4px;
  }

  .notice-center__list {
    min-height: 160px;
  }

  .notice-row {
    display: flex;
    gap: 12px;
    align-items: flex-start;
    width: 100%;
    padding: 12px 8px;
    font: inherit;
    color: inherit;
    text-align: left;
    cursor: pointer;
    background: transparent;
    border: 0;
    border-bottom: 1px solid var(--el-border-color-lighter);
    border-radius: 0;
  }

  .notice-row:hover {
    background: var(--el-fill-color-light);
  }

  .notice-row__icon {
    display: inline-flex;
    flex: none;
    align-items: center;
    justify-content: center;
    width: 34px;
    height: 34px;
    border-radius: 8px;
  }

  .notice-row__main {
    display: flex;
    flex: 1 1 auto;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
  }

  .notice-row__title {
    display: flex;
    gap: 6px;
    align-items: center;
    font-size: 14px;
    line-height: 20px;
    color: var(--el-text-color-primary);
    word-break: break-word;
  }

  .notice-row.is-unread .notice-row__title {
    font-weight: 600;
  }

  .notice-row__dot {
    flex: none;
    width: 6px;
    height: 6px;
    background: var(--el-color-danger);
    border-radius: 50%;
  }

  .notice-row__body {
    display: -webkit-box;
    overflow: hidden;
    -webkit-line-clamp: 2;
    font-size: 13px;
    line-height: 19px;
    color: var(--el-text-color-secondary);
    -webkit-box-orient: vertical;
  }

  .notice-row__time {
    flex: none;
    font-size: 12px;
    line-height: 20px;
    color: var(--el-text-color-secondary);
    white-space: nowrap;
  }

  .notice-center__more {
    display: flex;
    justify-content: center;
    padding: 12px 0 4px;
  }

  @media (width <= 640px) {
    .notice-row {
      flex-wrap: wrap;
    }

    .notice-row__time {
      flex-basis: 100%;
      padding-left: 46px;
      line-height: 16px;
    }
  }
</style>
