<!-- 工单管理页面：左列表 + 右会话的聊天式布局 -->
<template>
  <div class="tickets-page art-full-height">
    <div class="chat-container art-card">
      <!-- 左侧工单列表 -->
      <aside class="ticket-list-col" v-show="!isMobile || !chatVisible">
        <div class="list-header">
          <span class="list-title">工单列表</span>
          <span v-if="stats.pending > 0" class="pending-tag">待处理 {{ stats.pending }}</span>
        </div>

        <div class="list-filters">
          <ElInput
            v-model.trim="searchForm.keyword"
            placeholder="搜索编号 / 标题 / 创建人"
            clearable
            @input="handleKeywordInput"
          >
            <template #prefix>
              <ArtSvgIcon icon="ri:search-line" />
            </template>
          </ElInput>
          <ElSelect
            v-model="searchForm.status"
            placeholder="全部状态"
            clearable
            class="status-filter"
            @change="handleFilter"
          >
            <ElOption label="待处理" value="pending" />
            <ElOption label="已回复" value="replied" />
            <ElOption label="已关闭" value="closed" />
          </ElSelect>
        </div>

        <div class="ticket-list" v-loading="listLoading">
          <div
            v-for="item in ticketList"
            :key="item.id"
            class="ticket-item"
            :class="{ active: item.id === currentTicketId, closed: item.status === 'closed' }"
            @click="selectTicket(item)"
          >
            <span class="item-avatar" :class="`avatar-${item.creatorType}`">
              {{ avatarText(item.creatorName) }}
            </span>
            <div class="item-main">
              <div class="item-top">
                <span class="item-title">{{ item.title }}</span>
                <span class="item-time">{{ shortTime(item.lastReplyAt || item.createdAt) }}</span>
              </div>
              <div class="item-bottom">
                <span class="item-sub">{{ item.categoryText }}</span>
                <span class="item-status" :class="`status-${item.status}`">{{
                  item.statusText
                }}</span>
                <span v-if="item.unread" class="unread-dot" title="有新消息" />
              </div>
            </div>
          </div>
          <ElEmpty
            v-if="!ticketList.length && !listLoading"
            description="暂无工单"
            :image-size="72"
          />
        </div>

        <div class="list-footer">
          <ElPagination
            v-model:current-page="searchForm.page"
            :page-size="searchForm.pageSize"
            :total="total"
            layout="prev, pager, next"
            small
            @current-change="fetchList"
          />
        </div>
      </aside>

      <!-- 右侧会话区 -->
      <section class="chat-col" v-show="!isMobile || chatVisible">
        <template v-if="currentTicket">
          <header class="chat-header">
            <div class="chat-header-left">
              <ArtSvgIcon
                v-if="isMobile"
                icon="ri:arrow-left-line"
                class="back-btn"
                @click="chatVisible = false"
              />
              <span class="chat-avatar" :class="`avatar-${currentTicket.creatorType}`">
                {{ avatarText(currentTicket.creatorName) }}
              </span>
              <div class="chat-info">
                <div class="chat-title-line">
                  <span class="chat-title" :title="currentTicket.title">{{
                    currentTicket.title
                  }}</span>
                  <ElTag :type="statusTagMap[currentTicket.status]" size="small" effect="light">
                    {{ currentTicket.statusText }}
                  </ElTag>
                </div>
                <div class="chat-meta">
                  <span class="mono">{{ currentTicket.ticketNo }}</span>
                  <span class="divider">·</span>
                  <span>{{ currentTicket.categoryText }}</span>
                  <span class="divider">·</span>
                  <span class="creator-info">
                    {{ currentTicket.creatorName }}
                    <em class="creator-badge" :class="`badge-${currentTicket.creatorType}`">
                      {{ currentTicket.creatorType === 'agent' ? '代理商' : '用户' }}
                    </em>
                  </span>
                </div>
              </div>
            </div>
            <div class="chat-actions">
              <ElButton
                v-if="currentTicket.status !== 'closed'"
                size="small"
                plain
                :loading="actionLoading"
                @click="handleClose"
              >
                关闭工单
              </ElButton>
              <ElButton
                v-else
                size="small"
                type="primary"
                plain
                :loading="actionLoading"
                @click="handleReopen"
              >
                重新打开
              </ElButton>
            </div>
          </header>

          <div ref="messageContainer" class="chat-messages" v-loading="detailLoading">
            <template v-for="message in messages" :key="message.id">
              <div class="message-row" :class="{ 'is-self': message.senderType === 'admin' }">
                <span class="message-avatar" :class="`avatar-${message.senderType}`">
                  {{ avatarText(message.senderName) }}
                </span>
                <div class="message-body">
                  <div class="message-meta">
                    <span class="message-sender">
                      {{ message.senderName || senderRoleText(message.senderType) }}
                      <em v-if="message.senderType === 'admin'" class="sender-badge">客服</em>
                    </span>
                    <span class="message-time">{{ message.createdAt }}</span>
                  </div>
                  <div class="message-bubble">{{ message.content }}</div>
                </div>
              </div>
            </template>
            <ElEmpty
              v-if="!messages.length && !detailLoading"
              description="暂无对话"
              :image-size="60"
            />
          </div>

          <footer v-if="currentTicket.status !== 'closed'" class="chat-input">
            <ElInput
              v-model="draft"
              type="textarea"
              :rows="3"
              :maxlength="2000"
              placeholder="输入回复内容，Enter 发送，Shift+Enter 换行"
              resize="none"
              @keyup.enter.exact.prevent="sendReply"
            />
            <div class="chat-input-bar">
              <span class="input-tip">{{ draft.length }}/2000</span>
              <ElButton
                type="primary"
                :loading="sending"
                :disabled="!draft.trim()"
                @click="sendReply"
              >
                发送
              </ElButton>
            </div>
          </footer>
          <footer v-else class="chat-closed-tip">
            <ArtSvgIcon icon="ri:lock-line" />
            工单已关闭，可点击右上角「重新打开」后继续回复
          </footer>
        </template>

        <div v-else class="chat-empty">
          <ElEmpty description="从左侧选择一个工单开始处理" :image-size="120" />
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { showCaughtError } from '@/utils/http/error-toast'
  import {
    fetchTicketList,
    fetchTicketDetail,
    fetchTicketReply,
    fetchTicketStatus,
    type TicketItem,
    type TicketMessage,
    type TicketStats,
    type TicketStatus
  } from '@/api/ticket'

  defineOptions({ name: 'TicketManage' })

  type TagType = 'success' | 'warning' | 'info'

  const statusTagMap: Record<string, TagType> = {
    pending: 'warning',
    replied: 'success',
    closed: 'info'
  }

  // ==================== 布局 ====================

  const { width } = useWindowSize()
  const isMobile = computed(() => width.value < 768)
  /** 移动端下是否展示会话列 */
  const chatVisible = ref(false)

  // ==================== 工单列表 ====================

  const listLoading = ref(false)
  const ticketList = ref<TicketItem[]>([])
  const total = ref(0)
  const stats = ref<TicketStats>({ pending: 0, today: 0, closed: 0 })

  const searchForm = reactive({
    keyword: '',
    status: '' as TicketStatus | '',
    page: 1,
    pageSize: 15
  })

  let keywordTimer: ReturnType<typeof setTimeout> | null = null

  async function fetchList() {
    listLoading.value = true
    try {
      const res = await fetchTicketList({
        page: searchForm.page,
        pageSize: searchForm.pageSize,
        keyword: searchForm.keyword || undefined,
        status: searchForm.status || undefined
      })
      ticketList.value = res.list || []
      total.value = res.total || 0
      if (res.stats) stats.value = res.stats
    } catch {
      /* 拦截器已提示错误 */
    } finally {
      listLoading.value = false
    }
  }

  function handleKeywordInput() {
    if (keywordTimer) clearTimeout(keywordTimer)
    keywordTimer = setTimeout(handleFilter, 300)
  }

  function handleFilter() {
    searchForm.page = 1
    fetchList()
  }

  // ==================== 会话 ====================

  const currentTicketId = ref(0)
  const currentTicket = ref<TicketItem | null>(null)
  const messages = ref<TicketMessage[]>([])
  const detailLoading = ref(false)
  const sending = ref(false)
  const actionLoading = ref(false)
  const draft = ref('')
  const messageContainer = ref<HTMLElement | null>(null)

  function avatarText(name: string) {
    return (name || '?').trim().slice(0, 1).toUpperCase()
  }

  function senderRoleText(senderType: string) {
    return { user: '用户', agent: '代理商', admin: '客服' }[senderType] || senderType
  }

  /** 列表时间：今天显示时分，否则显示月-日 */
  function shortTime(value: string) {
    if (!value) return ''
    const [date, time] = value.split(' ')
    const now = new Date()
    const pad = (n: number) => String(n).padStart(2, '0')
    const today = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`
    return date === today ? (time || '').slice(0, 5) : (date || '').slice(5)
  }

  async function selectTicket(item: TicketItem) {
    if (item.id === currentTicketId.value) return
    currentTicketId.value = item.id
    chatVisible.value = true
    await loadDetail()
    // 打开即被服务端标记已读，同步左侧红点与角标
    item.unread = false
    window.dispatchEvent(new CustomEvent('panel-ticket-unread-refresh'))
  }

  async function loadDetail(silent = false) {
    if (!currentTicketId.value) return
    if (!silent) detailLoading.value = true
    try {
      const res = await fetchTicketDetail(currentTicketId.value)
      currentTicket.value = res.ticket
      messages.value = res.messages || []
      if (!silent) scrollToBottom()
    } catch (error) {
      if (!silent) showCaughtError(error, '加载工单失败')
    } finally {
      detailLoading.value = false
    }
  }

  async function sendReply() {
    const content = draft.value.trim()
    if (!content || !currentTicket.value || sending.value) return
    sending.value = true
    try {
      await fetchTicketReply(currentTicket.value.id, content)
      draft.value = ''
      await loadDetail(true)
      scrollToBottom()
      fetchList()
    } catch {
      /* 拦截器已提示错误 */
    } finally {
      sending.value = false
    }
  }

  async function handleClose() {
    if (!currentTicket.value) return
    try {
      await ElMessageBox.confirm('关闭后双方将不能继续回复，确定关闭该工单？', '关闭工单', {
        type: 'warning'
      })
    } catch {
      return
    }
    actionLoading.value = true
    try {
      await fetchTicketStatus(currentTicket.value.id, 'close')
      ElMessage.success('工单已关闭')
      await loadDetail(true)
      fetchList()
    } finally {
      actionLoading.value = false
    }
  }

  async function handleReopen() {
    if (!currentTicket.value) return
    actionLoading.value = true
    try {
      await fetchTicketStatus(currentTicket.value.id, 'reopen')
      ElMessage.success('工单已重新打开')
      await loadDetail(true)
      fetchList()
    } finally {
      actionLoading.value = false
    }
  }

  function scrollToBottom() {
    nextTick(() => {
      if (messageContainer.value) {
        messageContainer.value.scrollTop = messageContainer.value.scrollHeight
      }
    })
  }

  // ==================== 轮询 ====================

  let pollTimer: ReturnType<typeof setInterval> | null = null

  onMounted(() => {
    fetchList()
    pollTimer = setInterval(() => {
      fetchList()
      loadDetail(true)
    }, 15000)
  })

  onBeforeUnmount(() => {
    if (pollTimer) clearInterval(pollTimer)
    if (keywordTimer) clearTimeout(keywordTimer)
  })
</script>

<style scoped lang="scss">
  .tickets-page {
    .chat-container {
      flex: 1;
      display: flex;
      min-height: 0;
      overflow: hidden;
    }

    // ==================== 左侧列表 ====================

    .ticket-list-col {
      width: 300px;
      flex-shrink: 0;
      display: flex;
      flex-direction: column;
      border-right: 1px solid var(--art-card-border);
      min-height: 0;
    }

    .list-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 16px 16px 10px;

      .list-title {
        font-size: 15px;
        font-weight: 600;
        color: var(--art-gray-900);
      }

      .pending-tag {
        font-size: 12px;
        color: var(--el-color-warning);
        background: var(--el-color-warning-light-9);
        border-radius: 10px;
        padding: 2px 8px;
      }
    }

    .list-filters {
      display: flex;
      flex-direction: column;
      gap: 8px;
      padding: 0 16px 12px;
      border-bottom: 1px solid var(--art-card-border);

      .status-filter {
        width: 100%;
      }
    }

    .ticket-list {
      flex: 1;
      overflow-y: auto;
      min-height: 0;
    }

    .ticket-item {
      display: flex;
      align-items: center;
      gap: 10px;
      padding: 12px 16px;
      cursor: pointer;
      transition: background 0.15s;

      &:hover {
        background: var(--el-fill-color-light);
      }

      &.active {
        background: var(--el-color-primary-light-9);
      }

      &.closed {
        opacity: 0.6;
      }

      .item-avatar {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 38px;
        height: 38px;
        border-radius: 50%;
        font-size: 14px;
        font-weight: 600;
        color: #fff;
        flex-shrink: 0;
      }

      .item-main {
        flex: 1;
        min-width: 0;
      }

      .item-top {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
      }

      .item-title {
        font-size: 13.5px;
        font-weight: 500;
        color: var(--art-gray-900);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }

      .item-time {
        font-size: 11px;
        color: var(--art-gray-500);
        flex-shrink: 0;
      }

      .item-bottom {
        display: flex;
        align-items: center;
        gap: 8px;
        margin-top: 4px;
      }

      .item-sub {
        font-size: 12px;
        color: var(--art-gray-500);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }

      .item-status {
        display: inline-flex;
        align-items: center;
        gap: 4px;
        font-size: 12px;
        font-weight: 500;
        flex-shrink: 0;

        &::before {
          content: '';
          width: 6px;
          height: 6px;
          border-radius: 50%;
          background: currentColor;
        }

        &.status-pending {
          color: var(--el-color-warning);
        }

        &.status-replied {
          color: var(--el-color-success);
        }

        &.status-closed {
          color: var(--art-gray-500);
        }
      }

      .unread-dot {
        margin-left: auto;
      }
    }

    .unread-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: var(--el-color-danger);
      flex-shrink: 0;
    }

    .list-footer {
      display: flex;
      justify-content: center;
      padding: 10px 0;
      border-top: 1px solid var(--art-card-border);
    }

    // ==================== 右侧会话 ====================

    .chat-col {
      flex: 1;
      display: flex;
      flex-direction: column;
      min-width: 0;
      min-height: 0;
    }

    .chat-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      padding: 14px 20px;
      border-bottom: 1px solid var(--art-card-border);
      flex-shrink: 0;
    }

    .chat-header-left {
      display: flex;
      align-items: center;
      gap: 12px;
      min-width: 0;
    }

    .back-btn {
      font-size: 18px;
      cursor: pointer;
      color: var(--art-gray-600);
    }

    .chat-avatar {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 40px;
      height: 40px;
      border-radius: 50%;
      font-size: 15px;
      font-weight: 600;
      color: #fff;
      flex-shrink: 0;
    }

    .chat-info {
      min-width: 0;
    }

    .chat-title-line {
      display: flex;
      align-items: center;
      gap: 8px;
    }

    .chat-title {
      font-size: 15px;
      font-weight: 600;
      color: var(--art-gray-900);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      max-width: 320px;
    }

    .chat-meta {
      margin-top: 4px;
      font-size: 12px;
      color: var(--art-gray-500);

      .mono {
        font-family: 'Roboto Mono', monospace;
      }

      .divider {
        margin: 0 6px;
        color: var(--art-gray-400);
      }

      .creator-info {
        display: inline-flex;
        align-items: center;
        gap: 4px;
      }
    }

    .chat-actions {
      flex-shrink: 0;
    }

    .chat-messages {
      flex: 1;
      overflow-y: auto;
      min-height: 0;
      padding: 20px;
    }

    .message-row {
      display: flex;
      align-items: flex-start;
      gap: 10px;
      margin-bottom: 20px;

      &.is-self {
        flex-direction: row-reverse;

        .message-body {
          align-items: flex-end;
        }

        .message-meta {
          flex-direction: row-reverse;
        }

        .message-bubble {
          background: var(--el-color-primary-light-8);
        }
      }
    }

    .message-avatar {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 32px;
      height: 32px;
      border-radius: 50%;
      font-size: 13px;
      font-weight: 600;
      color: #fff;
      flex-shrink: 0;
    }

    .avatar-user {
      background: var(--el-color-success);
    }

    .avatar-agent {
      background: var(--el-color-warning);
    }

    .avatar-admin {
      background: var(--el-color-primary);
    }

    .creator-badge {
      font-style: normal;
      font-size: 10px;
      font-weight: 500;
      border-radius: 4px;
      padding: 1px 5px;
      line-height: 1.5;

      &.badge-user {
        color: var(--el-color-success);
        background: var(--el-color-success-light-9);
        border: 1px solid var(--el-color-success-light-7);
      }

      &.badge-agent {
        color: var(--el-color-warning);
        background: var(--el-color-warning-light-9);
        border: 1px solid var(--el-color-warning-light-7);
      }
    }

    .message-body {
      display: flex;
      flex-direction: column;
      max-width: 70%;
    }

    .message-meta {
      display: flex;
      align-items: center;
      gap: 8px;
      margin-bottom: 4px;
      font-size: 12px;
    }

    .message-sender {
      font-weight: 500;
      color: var(--art-gray-700);
    }

    .sender-badge {
      font-style: normal;
      font-size: 10px;
      color: var(--el-color-primary);
      background: var(--el-color-primary-light-9);
      border-radius: 4px;
      padding: 0 4px;
      margin-left: 2px;
    }

    .message-time {
      color: var(--art-gray-400);
    }

    .message-bubble {
      padding: 9px 12px;
      border-radius: 8px;
      background: var(--el-fill-color-light);
      font-size: 13px;
      line-height: 1.55;
      color: var(--art-gray-900);
      white-space: pre-wrap;
      word-break: break-word;
    }

    .chat-input {
      flex-shrink: 0;
      border-top: 1px solid var(--art-card-border);
      padding: 12px 20px 14px;
    }

    .chat-input-bar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-top: 10px;
    }

    .input-tip {
      font-size: 12px;
      color: var(--art-gray-400);
    }

    .chat-closed-tip {
      flex-shrink: 0;
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 6px;
      padding: 16px 0;
      border-top: 1px solid var(--art-card-border);
      font-size: 12px;
      color: var(--art-gray-400);
    }

    .chat-empty {
      flex: 1;
      display: flex;
      align-items: center;
      justify-content: center;
    }

    // ==================== 移动端 ====================

    @media (max-width: 768px) {
      .ticket-list-col {
        width: 100%;
        border-right: none;
      }

      .chat-col {
        width: 100%;
      }
    }
  }
</style>
