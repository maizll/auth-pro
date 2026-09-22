<!-- 面板工单中心：用户面板 / 代理商面板共用（列表 + 新建 + 会话） -->
<template>
  <div class="panel-ticket-center">
    <div class="art-card ticket-card">
      <div class="ticket-toolbar">
        <div class="toolbar-filters">
          <el-select
            v-model="searchStatus"
            placeholder="全部状态"
            clearable
            class="filter-select"
            @change="handleSearch"
          >
            <el-option label="待处理" value="pending" />
            <el-option label="已回复" value="replied" />
            <el-option label="已关闭" value="closed" />
          </el-select>
        </div>
        <el-button type="primary" class="create-btn" @click="openCreateDialog">
          <iconify-icon icon="ri:add-line" width="15" />
          新建工单
        </el-button>
      </div>

      <el-table :data="tableData" v-loading="loading" class="ticket-table">
        <el-table-column label="标题" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <div class="title-cell">
              <span v-if="row.unread" class="unread-dot" title="有新回复" />
              <span class="title-text">{{ row.title }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="ticketNo" label="工单编号" width="150">
          <template #default="{ row }">
            <span class="mono">{{ row.ticketNo }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="categoryText" label="分类" width="100" align="center" />
        <el-table-column label="状态" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="statusTagMap[row.status]" size="small" effect="light">
              {{ row.statusText }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="lastReplyAt" label="最近动态" width="150" />
        <el-table-column prop="createdAt" label="创建时间" width="150" />
        <el-table-column label="操作" width="90" fixed="right" align="center">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="openChat(row)">查看</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="暂无工单，点击右上角新建" :image-size="80" />
        </template>
      </el-table>

      <div class="pagination-wrapper">
        <el-pagination
          v-model:current-page="pagination.page"
          v-model:page-size="pagination.pageSize"
          :page-sizes="[10, 20, 50]"
          :total="pagination.total"
          layout="total, sizes, prev, pager, next"
          @current-change="fetchList"
          @size-change="fetchList"
        />
      </div>
    </div>

    <el-dialog v-model="createDialog.visible" title="新建工单" width="520px" destroy-on-close>
      <el-form label-width="72px" @submit.prevent>
        <el-form-item label="分类">
          <el-select v-model="createDialog.category" class="full-width">
            <el-option
              v-for="option in TICKET_CATEGORY_OPTIONS"
              :key="option.value"
              :label="option.label"
              :value="option.value"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="标题">
          <el-input
            v-model="createDialog.title"
            placeholder="一句话描述问题"
            maxlength="60"
            show-word-limit
            clearable
          />
        </el-form-item>
        <el-form-item label="问题描述">
          <el-input
            v-model="createDialog.content"
            type="textarea"
            :rows="5"
            maxlength="2000"
            show-word-limit
            placeholder="请描述遇到的问题，附上授权编号 / 订单号等信息可以更快定位"
            resize="none"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createDialog.visible = false">取消</el-button>
        <el-button type="primary" :loading="createDialog.submitting" @click="submitCreate">
          提交工单
        </el-button>
      </template>
    </el-dialog>

    <TicketChatPanel
      ref="chatPanelRef"
      mode="panel"
      :api-prefix="apiPrefix"
      :token-key="tokenKey"
      :self-type="selfType"
      @updated="handleChatUpdated"
    />
  </div>
</template>

<script setup lang="ts">
  import { ref, reactive, onMounted } from 'vue'
  import { ElMessage } from 'element-plus'
  import { Icon as IconifyIcon } from '@iconify/vue'
  import axios from 'axios'
  import TicketChatPanel from '@/components/core/panels/TicketChatPanel.vue'
  import { TICKET_CATEGORY_OPTIONS, type TicketItem, type TicketSenderType } from '@/api/ticket'

  interface Props {
    /** 接口前缀：/api/user-panel 或 /api/agent-panel */
    apiPrefix: string
    /** token 存储键：user_panel_token 或 agent_panel_token */
    tokenKey: string
    /** 当前角色，会话气泡左右侧判断 */
    selfType: TicketSenderType
  }

  const props = defineProps<Props>()

  type TagType = 'success' | 'warning' | 'info'
  const statusTagMap: Record<string, TagType> = {
    pending: 'warning',
    replied: 'success',
    closed: 'info'
  }

  const loading = ref(false)
  const searchStatus = ref('')
  const pagination = reactive({ page: 1, pageSize: 10, total: 0 })
  const tableData = ref<TicketItem[]>([])
  const chatPanelRef = ref<InstanceType<typeof TicketChatPanel>>()

  const createDialog = reactive({
    visible: false,
    submitting: false,
    category: 'authorization',
    title: '',
    content: ''
  })

  function getToken() {
    return localStorage.getItem(props.tokenKey) || ''
  }

  async function fetchList() {
    loading.value = true
    try {
      const { data } = await axios.get(`${props.apiPrefix}/tickets`, {
        headers: { Authorization: `Bearer ${getToken()}` },
        params: {
          status: searchStatus.value || undefined,
          page: pagination.page,
          pageSize: pagination.pageSize
        }
      })
      if (data.code === 200) {
        tableData.value = data.data.list || []
        pagination.total = data.data.total || 0
      } else {
        ElMessage.error(data.msg || '加载工单失败')
      }
    } catch {
      ElMessage.error('加载工单失败')
    } finally {
      loading.value = false
    }
  }

  function handleSearch() {
    pagination.page = 1
    fetchList()
  }

  function openCreateDialog() {
    createDialog.title = ''
    createDialog.content = ''
    createDialog.category = 'authorization'
    createDialog.visible = true
  }

  async function submitCreate() {
    const title = createDialog.title.trim()
    const content = createDialog.content.trim()
    if (!title) {
      ElMessage.warning('请填写标题')
      return
    }
    if (!content) {
      ElMessage.warning('请填写问题描述')
      return
    }
    createDialog.submitting = true
    try {
      const { data } = await axios.post(
        `${props.apiPrefix}/tickets`,
        { category: createDialog.category, title, content },
        { headers: { Authorization: `Bearer ${getToken()}` } }
      )
      if (data.code === 200) {
        ElMessage.success(data.msg || '工单已提交')
        createDialog.visible = false
        pagination.page = 1
        fetchList()
        notifyUnreadChanged()
      } else {
        ElMessage.error(data.msg || '提交失败')
      }
    } catch {
      ElMessage.error('提交失败，请稍后重试')
    } finally {
      createDialog.submitting = false
    }
  }

  function openChat(row: TicketItem) {
    chatPanelRef.value?.open(row.id)
  }

  function handleChatUpdated() {
    fetchList()
    notifyUnreadChanged()
  }

  /** 通知布局组件立即刷新未读角标（布局内另有 30s 轮询兜底） */
  function notifyUnreadChanged() {
    window.dispatchEvent(new CustomEvent('panel-ticket-unread-refresh'))
  }

  onMounted(fetchList)
</script>

<style scoped lang="scss">
  .panel-ticket-center {
    min-width: 0;
    max-width: 100%;
  }

  .ticket-card {
    padding: 20px;
    overflow: hidden;
    background: var(--el-bg-color);
    border-radius: 12px !important;
  }

  .ticket-toolbar {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 16px;
  }

  .toolbar-filters {
    display: flex;
    gap: 10px;
    align-items: center;
  }

  .filter-select {
    width: 130px;
  }

  .create-btn {
    display: inline-flex;
    gap: 5px;
    align-items: center;
  }

  .ticket-table {
    :deep(.el-table__header th) {
      font-weight: 600;
      color: var(--el-text-color-secondary);
      background: var(--el-fill-color-light);
    }
  }

  .title-cell {
    display: flex;
    gap: 8px;
    align-items: center;
    min-width: 0;
  }

  .unread-dot {
    flex-shrink: 0;
    width: 8px;
    height: 8px;
    background: var(--el-color-danger);
    border-radius: 50%;
  }

  .title-text {
    overflow: hidden;
    color: var(--el-text-color-primary);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .mono {
    font-family: 'Roboto Mono', monospace;
    font-size: 12px;
  }

  .full-width {
    width: 100%;
  }

  .pagination-wrapper {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
  }

  @media (width <= 768px) {
    .ticket-card {
      padding: 14px;
    }

    .toolbar-filters,
    .filter-select {
      width: 100%;
    }

    .create-btn {
      justify-content: center;
      width: 100%;
    }

    .pagination-wrapper {
      justify-content: center;
    }
  }
</style>
