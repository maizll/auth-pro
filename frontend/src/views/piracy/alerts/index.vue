<template>
  <div class="alert-center">
    <el-alert
      class="mb-4"
      type="info"
      :closable="false"
      show-icon
      title="通知渠道尚未接入。这里不会发邮件或 Webhook，校验失败也不会自动写入本列表。盗版命中请到盗版追踪查看。"
    />
    <!-- 统计概览 -->
    <el-row :gutter="16" class="mb-4">
      <el-col :xs="12" :sm="6">
        <el-card shadow="never" class="art-card stats-card">
          <div class="stats-title">未处理告警</div>
          <div class="stats-value text-danger">{{ stats.unhandled }}</div>
        </el-card>
      </el-col>
      <el-col :xs="12" :sm="6">
        <el-card shadow="never" class="art-card stats-card">
          <div class="stats-title">今日告警</div>
          <div class="stats-value text-warning">{{ stats.today }}</div>
        </el-card>
      </el-col>
      <el-col :xs="12" :sm="6">
        <el-card shadow="never" class="art-card stats-card">
          <div class="stats-title">本周告警</div>
          <div class="stats-value text-primary">{{ stats.week }}</div>
        </el-card>
      </el-col>
      <el-col :xs="12" :sm="6">
        <el-card shadow="never" class="art-card stats-card">
          <div class="stats-title">已处理</div>
          <div class="stats-value text-success">{{ stats.handled }}</div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 搜索栏 -->
    <el-card shadow="never" class="art-card mb-4 filter-panel">
      <el-form :model="searchForm" inline>
        <el-form-item label="告警类型">
          <el-select v-model="searchForm.type" placeholder="全部" clearable style="width: 140px">
            <el-option label="盗版异常" value="piracy" />
            <el-option label="授权到期" value="expire" />
            <el-option label="余额不足" value="balance" />
            <el-option label="配额耗尽" value="quota" />
            <el-option label="验证异常" value="verify_anomaly" />
          </el-select>
        </el-form-item>
        <el-form-item label="级别">
          <el-select v-model="searchForm.level" placeholder="全部" clearable style="width: 110px">
            <el-option label="紧急" value="critical" />
            <el-option label="警告" value="warning" />
            <el-option label="提示" value="info" />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="searchForm.status" placeholder="全部" clearable style="width: 110px">
            <el-option label="未处理" value="pending" />
            <el-option label="已处理" value="handled" />
            <el-option label="已忽略" value="ignored" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleSearch">查询</el-button>
          <el-button @click="handleReset">重置</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 告警列表 -->
    <el-card shadow="never" class="art-card piracy-panel">
      <template #header>
        <div class="table-header">
          <span class="card-title">告警列表</span>
          <div class="table-actions">
            <el-button plain :disabled="!selectedRows.length" @click="handleBatchHandle">
              批量标记已处理 ({{ selectedRows.length }})
            </el-button>
          </div>
        </div>
      </template>

      <el-table
        :data="tableData"
        stripe
        v-loading="loading"
        @selection-change="handleSelectionChange"
      >
        <el-table-column type="selection" width="45" />
        <el-table-column prop="levelLabel" label="级别" width="70" align="center">
          <template #default="{ row }">
            <el-tag :type="levelTagMap[row.level]" size="small" effect="dark">{{
              row.levelLabel
            }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="typeLabel" label="类型" width="100">
          <template #default="{ row }">
            <el-tag :type="typeTagMap[row.type]" size="small">{{ row.typeLabel }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="title" label="告警内容" min-width="280" show-overflow-tooltip />
        <el-table-column prop="target" label="关联对象" width="160" show-overflow-tooltip />
        <el-table-column prop="statusLabel" label="状态" width="80" align="center">
          <template #default="{ row }">
            <el-tag :type="statusTagMap[row.status]" size="small" effect="plain">{{
              row.statusLabel
            }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="createdAt" label="时间" width="160" />
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button
              link
              type="primary"
              size="small"
              @click="handleMark(row, 'handled')"
              v-if="row.status === 'pending'"
            >
              已处理
            </el-button>
            <el-button
              link
              type="info"
              size="small"
              @click="handleMark(row, 'ignored')"
              v-if="row.status === 'pending'"
            >
              忽略
            </el-button>
            <el-button link type="primary" size="small" @click="handleViewTarget(row)"
              >查看</el-button
            >
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-wrapper">
        <el-pagination
          v-model:current-page="pagination.page"
          v-model:page-size="pagination.pageSize"
          :page-sizes="[20, 50, 100]"
          :total="pagination.total"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handleSearch"
          @current-change="handleSearch"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
  import { ref, reactive, onMounted } from 'vue'
  import { ElMessage } from 'element-plus'
  import request from '@/utils/http'

  const loading = ref(false)
  const selectedRows = ref<any[]>([])

  const stats = reactive({ unhandled: 0, today: 0, week: 0, handled: 0 })

  const searchForm = reactive({ type: '', level: '', status: '' })
  const pagination = reactive({ page: 1, pageSize: 20, total: 0 })

  const levelTagMap: Record<
    string,
    'primary' | 'success' | 'warning' | 'info' | 'danger' | undefined
  > = { critical: 'danger', warning: 'warning', info: 'info' } as const
  const typeTagMap: Record<
    string,
    'primary' | 'success' | 'warning' | 'info' | 'danger' | undefined
  > = {
    piracy: 'danger',
    expire: 'warning',
    balance: undefined,
    quota: 'info',
    verify_anomaly: 'danger'
  } as const
  const statusTagMap: Record<
    string,
    'primary' | 'success' | 'warning' | 'info' | 'danger' | undefined
  > = { pending: 'warning', handled: 'success', ignored: 'info' } as const

  const tableData = ref<any[]>([])

  async function loadStats() {
    const data = await request.get<any>({ url: '/api/piracy/alert/stats' })
    Object.assign(stats, data)
  }

  async function handleSearch() {
    loading.value = true
    try {
      const params: any = { page: pagination.page, pageSize: pagination.pageSize }
      if (searchForm.type) params.type = searchForm.type
      if (searchForm.level) params.level = searchForm.level
      if (searchForm.status) params.status = searchForm.status
      const data = await request.get<any>({ url: '/api/piracy/alert/list', params })
      tableData.value = data.list || []
      pagination.total = data.total || 0
    } finally {
      loading.value = false
    }
  }

  function handleReset() {
    searchForm.type = ''
    searchForm.level = ''
    searchForm.status = ''
    pagination.page = 1
    handleSearch()
  }

  function handleSelectionChange(rows: any[]) {
    selectedRows.value = rows
  }

  async function handleMark(row: any, status: 'handled' | 'ignored') {
    await request.put<any>({ url: `/api/piracy/alert/${row.id}/mark`, data: { status } })
    ElMessage.success(status === 'handled' ? '已标记为处理' : '已忽略')
    handleSearch()
    loadStats()
  }

  async function handleBatchHandle() {
    const ids = selectedRows.value.filter((r) => r.status === 'pending').map((r) => r.id)
    if (!ids.length) return
    await request.post<any>({
      url: '/api/piracy/alert/batch-mark',
      data: { ids, status: 'handled' }
    })
    ElMessage.success(`已批量处理 ${ids.length} 条`)
    handleSearch()
    loadStats()
  }

  function handleViewTarget(row: any) {
    ElMessage.info(`查看关联对象: ${row.target}`)
  }

  onMounted(() => {
    loadStats()
    handleSearch()
  })
</script>

<style scoped lang="scss">
  .alert-center {
    padding-bottom: 8px;

    :deep(.el-card) {
      --el-card-border-color: var(--art-card-border);
      border-radius: calc(var(--custom-radius) + 4px);
      background: var(--default-box-color);
      box-shadow: none;
    }

    :deep(.el-card__header) {
      padding: 20px 22px 14px;
      border-bottom-color: var(--art-card-border);
    }

    :deep(.el-card__body) {
      padding: 20px 22px;
    }

    :deep(.el-table) {
      --el-table-border-color: var(--art-card-border);
      --el-table-header-bg-color: var(--art-gray-100);
      --el-table-row-hover-bg-color: var(--art-gray-100);
      color: var(--art-gray-800);
    }

    :deep(.el-table th.el-table__cell) {
      color: var(--art-gray-600);
      font-weight: 600;
    }

    .filter-panel :deep(.el-form) {
      margin-bottom: -18px;
    }
  }

  .mb-4 {
    margin-bottom: 16px;
  }
  .table-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 12px;
  }
  .table-actions {
    display: flex;
    gap: 8px;
  }
  .card-title {
    font-size: 16px;
    font-weight: 700;
    color: var(--art-gray-900);
  }
  .pagination-wrapper {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
  }

  .stats-card {
    min-height: 102px;
    text-align: center;
    transition:
      border-color 0.2s ease,
      transform 0.2s ease;

    &:hover {
      border-color: color-mix(in srgb, var(--art-primary) 30%, var(--art-card-border));
      transform: translateY(-2px);
    }

    .stats-title {
      margin-bottom: 8px;
      font-size: 13px;
      color: var(--art-gray-600);
    }

    .stats-value {
      font-size: 26px;
      font-weight: 700;
      line-height: 1.2;
    }
  }

  .text-danger {
    color: var(--el-color-danger);
  }
  .text-warning {
    color: var(--el-color-warning);
  }
  .text-primary {
    color: var(--art-primary);
  }
  .text-success {
    color: var(--el-color-success);
  }
</style>
