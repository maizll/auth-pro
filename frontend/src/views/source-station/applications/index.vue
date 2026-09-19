<template>
  <div class="source-station-page">
    <el-card shadow="never" class="art-card mb-4 filter-panel">
      <el-form inline>
        <el-form-item label="申请状态">
          <el-select v-model="status" style="width: 140px">
            <el-option label="待审核" value="pending" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="loadApplications">查询</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" class="art-card mb-4">
      <template #header>
        <div class="table-header">
          <span class="card-title">开发者入驻申请（待审核 {{ applications.length }} 条）</span>
        </div>
      </template>
      <el-table :data="applications" stripe v-loading="loading">
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="username" label="用户名" width="140" />
        <el-table-column prop="displayName" label="显示名" width="140" />
        <el-table-column prop="email" label="邮箱" min-width="160" show-overflow-tooltip />
        <el-table-column prop="reason" label="申请说明" min-width="180" show-overflow-tooltip />
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="statusMeta(row.status).type" size="small">
              {{ statusMeta(row.status).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="reviewNote" label="审核说明" min-width="140" show-overflow-tooltip />
        <el-table-column prop="createdAt" label="申请时间" width="170" />
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button
              link
              type="success"
              size="small"
              :disabled="row.status !== 'pending'"
              @click="handleApprove(row)"
            >
              通过
            </el-button>
            <el-button
              link
              type="warning"
              size="small"
              :disabled="row.status !== 'pending'"
              @click="handleReject(row)"
            >
              拒绝
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card shadow="never" class="art-card">
      <template #header>
        <div class="table-header">
          <span class="card-title">开发者账号（共 {{ developers.length }} 条）</span>
        </div>
      </template>
      <el-table :data="developers" stripe v-loading="devLoading">
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="username" label="用户名" width="160" />
        <el-table-column prop="displayName" label="显示名" width="160" />
        <el-table-column prop="email" label="邮箱" min-width="180" show-overflow-tooltip />
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag type="success" size="small">正常</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="createdAt" label="创建时间" width="170" />
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button
              link
              type="danger"
              size="small"
              @click="handleCancelDev(row)"
            >
              取消资格
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, ref } from 'vue'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import {
    SOURCE_APPLICATION_STATUS,
    approveSourceApplication,
    cancelSourceDeveloper,
    fetchSourceApplications,
    fetchSourceDevelopers,
    rejectSourceApplication,
    type SourceApplication,
    type SourceDeveloper
  } from '@/api/source-station'

  const status = ref('pending')
  const loading = ref(false)
  const devLoading = ref(false)
  const applications = ref<SourceApplication[]>([])
  const developers = ref<SourceDeveloper[]>([])

  function statusMeta(value: string) {
    return SOURCE_APPLICATION_STATUS[value] || { label: value, type: 'info' as const }
  }

  async function loadApplications() {
    loading.value = true
    try {
      const data = await fetchSourceApplications(status.value || 'pending')
      applications.value = data.list || []
    } finally {
      loading.value = false
    }
  }

  async function loadDevelopers() {
    devLoading.value = true
    try {
      const data = await fetchSourceDevelopers()
      developers.value = data.list || []
    } finally {
      devLoading.value = false
    }
  }

  async function handleApprove(row: SourceApplication) {
    await ElMessageBox.confirm(`通过 ${row.username} 的入驻申请？通过后将创建开发者账号。`, '通过入驻', {
      type: 'success',
      confirmButtonText: '确认通过',
      cancelButtonText: '返回'
    })
    await approveSourceApplication(row.id)
    ElMessage.success('已通过入驻并创建开发者账号')
    await Promise.all([loadApplications(), loadDevelopers()])
  }

  async function promptNote(title: string) {
    const { value } = await ElMessageBox.prompt('请填写原因', title, {
      inputPlaceholder: '审核说明',
      confirmButtonText: '确定',
      cancelButtonText: '返回'
    })
    return value || title
  }

  async function handleReject(row: SourceApplication) {
    const note = await promptNote('拒绝入驻')
    await rejectSourceApplication(row.id, note)
    ElMessage.success('已拒绝入驻申请')
    await loadApplications()
  }

  async function confirmCancel(name: string) {
    await ElMessageBox.confirm(
      `确认取消「${name}」的开发者资格？将删除该账号并释放用户名/邮箱，对方可重新申请入驻。已有目录项会保留，但不再挂在该开发者名下。`,
      '取消开发者资格',
      {
        type: 'warning',
        confirmButtonText: '确认取消',
        cancelButtonText: '返回',
        confirmButtonClass: 'el-button--danger',
        distinguishCancelAndClose: true
      }
    )
  }

  async function handleCancelDev(row: SourceDeveloper) {
    await confirmCancel(row.username)
    await cancelSourceDeveloper(row.id)
    ElMessage.success('已删除开发者账号，该用户名可重新申请')
    await Promise.all([loadApplications(), loadDevelopers()])
  }

  onMounted(() => {
    loadApplications()
    loadDevelopers()
  })
</script>

<style scoped lang="scss">
  .source-station-page {
    padding-bottom: 8px;

    :deep(.el-card) {
      --el-card-border-color: var(--art-card-border);
      border-radius: calc(var(--custom-radius) + 4px);
      background: var(--default-box-color);
      box-shadow: none;
    }
  }

  .mb-4 {
    margin-bottom: 16px;
  }

  .table-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .card-title {
    font-size: 16px;
    font-weight: 700;
    color: var(--art-gray-900);
  }

  .filter-panel :deep(.el-form) {
    margin-bottom: -18px;
  }
</style>
