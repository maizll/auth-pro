<!-- 工单会话面板：用户/代理/管理三端共用，聊天气泡 + 回复 + 关闭/重开 -->
<template>
  <el-drawer
    v-model="visible"
    :size="drawerSize"
    :with-header="false"
    destroy-on-close
    class="ticket-chat-drawer"
    @closed="handleClosed"
  >
    <div class="chat-panel" v-loading="loading">
      <template v-if="ticket">
        <header class="chat-header">
          <div class="chat-title">
            <div class="chat-title-line">
              <span class="chat-title-text" :title="ticket.title">{{ ticket.title }}</span>
              <el-tag :type="statusTagType" size="small" effect="light">{{
                ticket.statusText
              }}</el-tag>
            </div>
            <div class="chat-meta">
              <span class="mono">{{ ticket.ticketNo }}</span>
              <span class="divider">·</span>
              <span>{{ ticket.categoryText }}</span>
              <template v-if="mode === 'admin'">
                <span class="divider">·</span>
                <span class="creator-info">
                  {{ ticket.creatorName }}
                  <em class="creator-badge" :class="`badge-${ticket.creatorType}`">
                    {{ ticket.creatorType === 'agent' ? '代理商' : '用户' }}
                  </em>
                </span>
              </template>
            </div>
          </div>
          <div class="chat-actions">
            <el-button
              v-if="ticket.status !== 'closed'"
              size="small"
              plain
              :loading="actionLoading"
              @click="handleClose"
            >
              关闭工单
            </el-button>
            <el-button
              v-else-if="mode === 'admin'"
              size="small"
              type="primary"
              plain
              :loading="actionLoading"
              @click="handleReopen"
            >
              重新打开
            </el-button>
            <el-icon class="close-btn" :size="18" @click="visible = false">
              <iconify-icon icon="ri:close-line" />
            </el-icon>
          </div>
        </header>

        <div ref="messageContainer" class="chat-messages">
          <template v-for="message in messages" :key="message.id">
            <div class="message-row" :class="{ 'is-self': message.senderType === selfType }">
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
          <el-empty v-if="!messages.length && !loading" description="暂无对话" :image-size="60" />
        </div>

        <footer v-if="ticket.status !== 'closed'" class="chat-input">
          <el-input
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
            <el-button
              type="primary"
              :loading="sending"
              :disabled="!draft.trim()"
              @click="sendReply"
            >
              发送
            </el-button>
          </div>
        </footer>
        <footer v-else class="chat-closed-tip">
          <iconify-icon icon="ri:lock-line" width="14" />
          工单已关闭，{{ mode === 'admin' ? '可重新打开后继续回复' : '如有新问题请新建工单' }}
        </footer>
      </template>
    </div>
  </el-drawer>
</template>

<script setup lang="ts">
  import { ref, computed, nextTick, onBeforeUnmount } from 'vue'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { Icon as IconifyIcon } from '@iconify/vue'
  import axios from 'axios'
  import {
    fetchTicketDetail,
    fetchTicketReply,
    fetchTicketStatus,
    type TicketItem,
    type TicketMessage,
    type TicketSenderType
  } from '@/api/ticket'

  interface Props {
    /** panel = 用户/代理面板（localStorage token）；admin = 管理端（统一 request） */
    mode: 'panel' | 'admin'
    /** panel 模式接口前缀，如 /api/user-panel */
    apiPrefix?: string
    /** panel 模式 token 存储键 */
    tokenKey?: string
    /** 当前查看者角色，用于区分左右气泡 */
    selfType: TicketSenderType
  }

  const props = withDefaults(defineProps<Props>(), {
    apiPrefix: '',
    tokenKey: ''
  })

  const emit = defineEmits<{ (e: 'updated'): void }>()

  /** 通知父级刷新，并广播给布局角标轮询 */
  function notifyUpdated() {
    emit('updated')
    window.dispatchEvent(new CustomEvent('panel-ticket-unread-refresh'))
  }

  const visible = ref(false)
  const loading = ref(false)
  const sending = ref(false)
  const actionLoading = ref(false)
  const ticket = ref<TicketItem | null>(null)
  const messages = ref<TicketMessage[]>([])
  const draft = ref('')
  const messageContainer = ref<HTMLElement | null>(null)
  let pollTimer: ReturnType<typeof setInterval> | null = null

  const drawerSize = computed(() => (window.innerWidth < 640 ? '100%' : '520px'))
  const statusTagType = computed(() => {
    const map: Record<string, 'warning' | 'success' | 'info'> = {
      pending: 'warning',
      replied: 'success',
      closed: 'info'
    }
    return map[ticket.value?.status || ''] || 'info'
  })

  function authHeaders() {
    return { Authorization: `Bearer ${localStorage.getItem(props.tokenKey) || ''}` }
  }

  function avatarText(name: string) {
    return (name || '?').trim().slice(0, 1).toUpperCase()
  }

  function senderRoleText(senderType: string) {
    return { user: '用户', agent: '代理商', admin: '客服' }[senderType] || senderType
  }

  async function requestDetail(
    id: number
  ): Promise<{ ticket: TicketItem; messages: TicketMessage[] } | null> {
    if (props.mode === 'admin') {
      const data = await fetchTicketDetail(id)
      return data as any
    }
    const { data } = await axios.get(`${props.apiPrefix}/tickets/${id}`, { headers: authHeaders() })
    if (data.code !== 200) {
      ElMessage.error(data.msg || '加载工单失败')
      return null
    }
    return data.data
  }

  async function loadDetail(silent = false) {
    if (!ticket.value) return
    if (!silent) loading.value = true
    try {
      const data = await requestDetail(ticket.value.id)
      if (!data) return
      ticket.value = data.ticket
      messages.value = data.messages || []
      if (!silent) scrollToBottom()
    } catch {
      if (!silent) ElMessage.error('加载工单失败')
    } finally {
      loading.value = false
    }
  }

  async function sendReply() {
    const content = draft.value.trim()
    if (!content || !ticket.value || sending.value) return
    sending.value = true
    try {
      if (props.mode === 'admin') {
        await fetchTicketReply(ticket.value.id, content)
      } else {
        const { data } = await axios.post(
          `${props.apiPrefix}/tickets/${ticket.value.id}/replies`,
          { content },
          { headers: authHeaders() }
        )
        if (data.code !== 200) {
          ElMessage.error(data.msg || '回复失败')
          return
        }
      }
      draft.value = ''
      await loadDetail(true)
      scrollToBottom()
      notifyUpdated()
    } catch (error: any) {
      ElMessage.error(error?.message || '回复失败')
    } finally {
      sending.value = false
    }
  }

  async function handleClose() {
    if (!ticket.value) return
    try {
      await ElMessageBox.confirm('关闭后双方将不能继续回复，确定关闭该工单？', '关闭工单', {
        type: 'warning'
      })
    } catch {
      return
    }
    actionLoading.value = true
    try {
      if (props.mode === 'admin') {
        await fetchTicketStatus(ticket.value.id, 'close')
      } else {
        const { data } = await axios.put(
          `${props.apiPrefix}/tickets/${ticket.value.id}/close`,
          {},
          { headers: authHeaders() }
        )
        if (data.code !== 200) {
          ElMessage.error(data.msg || '关闭失败')
          return
        }
      }
      ElMessage.success('工单已关闭')
      await loadDetail(true)
      notifyUpdated()
    } catch (error: any) {
      ElMessage.error(error?.message || '关闭失败')
    } finally {
      actionLoading.value = false
    }
  }

  async function handleReopen() {
    if (!ticket.value) return
    actionLoading.value = true
    try {
      await fetchTicketStatus(ticket.value.id, 'reopen')
      ElMessage.success('工单已重新打开')
      await loadDetail(true)
      notifyUpdated()
    } catch (error: any) {
      ElMessage.error(error?.message || '操作失败')
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

  function startPolling() {
    stopPolling()
    pollTimer = setInterval(() => loadDetail(true), 15000)
  }

  function stopPolling() {
    if (pollTimer) {
      clearInterval(pollTimer)
      pollTimer = null
    }
  }

  function handleClosed() {
    stopPolling()
    ticket.value = null
    messages.value = []
    draft.value = ''
  }

  function open(ticketId: number) {
    ticket.value = { id: ticketId } as TicketItem
    visible.value = true
    // 打开即被服务端标记已读，完成后同步列表与角标
    loadDetail().finally(() => notifyUpdated())
    startPolling()
  }

  onBeforeUnmount(stopPolling)

  defineExpose({ open })
</script>

<style scoped lang="scss">
  .chat-panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    background: var(--el-bg-color);
  }

  .chat-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    padding: 4px 4px 14px;
    border-bottom: 1px solid var(--el-border-color-lighter);
    flex-shrink: 0;
  }

  .chat-title {
    min-width: 0;
  }

  .chat-title-line {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .chat-title-text {
    font-size: 15px;
    font-weight: 600;
    color: var(--el-text-color-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 260px;
  }

  .chat-meta {
    margin-top: 6px;
    font-size: 12px;
    color: var(--el-text-color-secondary);

    .mono {
      font-family: 'Roboto Mono', monospace;
    }

    .divider {
      margin: 0 6px;
      color: var(--el-text-color-placeholder);
    }

    .creator-info {
      display: inline-flex;
      align-items: center;
      gap: 4px;
    }
  }

  .chat-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
  }

  .close-btn {
    cursor: pointer;
    color: var(--el-text-color-secondary);
    transition: color 0.2s;

    &:hover {
      color: var(--el-color-primary);
    }
  }

  .chat-messages {
    flex: 1;
    overflow-y: auto;
    padding: 18px 4px;
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
        color: var(--el-text-color-primary);
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
    max-width: 72%;
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
    color: var(--el-text-color-regular);
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
    color: var(--el-text-color-placeholder);
  }

  .message-bubble {
    padding: 9px 12px;
    border-radius: 8px;
    background: var(--el-fill-color-light);
    font-size: 13px;
    line-height: 1.55;
    color: var(--el-text-color-primary);
    white-space: pre-wrap;
    word-break: break-word;
  }

  .chat-input {
    flex-shrink: 0;
    border-top: 1px solid var(--el-border-color-lighter);
    padding: 12px 4px 0;
  }

  .chat-input-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: 10px;
  }

  .input-tip {
    font-size: 12px;
    color: var(--el-text-color-placeholder);
  }

  .chat-closed-tip {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    padding: 14px 0 4px;
    border-top: 1px solid var(--el-border-color-lighter);
    font-size: 12px;
    color: var(--el-text-color-placeholder);
  }
</style>
