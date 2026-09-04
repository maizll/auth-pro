<!-- 验证日志页面 -->
<!-- art-full-height 自动计算出页面剩余高度 -->
<!-- art-table-card 一个符合系统样式的 class，同时自动撑满剩余高度 -->
<template>
  <div class="license-logs-page art-full-height">
    <!-- 搜索栏 -->
    <LogSearch v-model="searchForm" @search="handleSearch" @reset="resetSearchParams" />

    <ElCard class="art-table-card" shadow="never">
      <!-- 表格头部 -->
      <ArtTableHeader v-model:columns="columnChecks" :loading="loading" @refresh="refreshData">
        <template #left>
          <ElSpace wrap>
            <ElButton type="danger" plain @click="handleClear" v-ripple>清空日志</ElButton>
          </ElSpace>
        </template>
      </ArtTableHeader>

      <!-- 表格 -->
      <ArtTable
        :loading="loading"
        :data="data"
        :columns="columns"
        :pagination="pagination"
        @pagination:size-change="handleSizeChange"
        @pagination:current-change="handleCurrentChange"
      >
        <!-- 结果 -->
        <template #result="{ row }">
          <ElTag :type="row.result === 'pass' ? 'success' : 'danger'" size="small">
            {{ row.result === 'pass' ? '通过' : '拒绝' }}
          </ElTag>
        </template>
      </ArtTable>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { useTable } from '@/hooks/core/useTable'
  import {
    fetchVerifyLogList,
    fetchClearVerifyLogs,
    type VerifyLogSearchParams
  } from '@/api/license-manage'
  import LogSearch from './modules/log-search.vue'

  defineOptions({ name: 'LicenseLogs' })

  interface VerifyLogSearchForm {
    keyword?: string
    appId?: number | string
    result?: string
    dateRange?: [string, string]
  }

  // 搜索表单
  const searchForm = ref<VerifyLogSearchForm>({
    keyword: undefined,
    appId: undefined,
    result: undefined,
    dateRange: undefined
  })

  const {
    columns,
    columnChecks,
    data,
    loading,
    pagination,
    getData,
    replaceSearchParams,
    resetSearchParams,
    handleSizeChange,
    handleCurrentChange,
    refreshData
  } = useTable({
    // 核心配置
    core: {
      apiFn: fetchVerifyLogList,
      apiParams: {
        page: 1,
        pageSize: 20
      },
      // 后端日志接口使用 page / pageSize 分页字段
      paginationKey: {
        current: 'page',
        size: 'pageSize'
      },
      columnsFactory: () => [
        { type: 'index', width: 60, label: '序号' }, // 序号
        {
          prop: 'requestDomain',
          label: '请求域名/IP',
          minWidth: 180,
          showOverflowTooltip: true
        },
        { prop: 'appName', label: '应用', width: 110 },
        { prop: 'result', label: '结果', width: 80, align: 'center', useSlot: true },
        { prop: 'reason', label: '原因', minWidth: 150, showOverflowTooltip: true },
        { prop: 'clientIp', label: '来源IP', width: 140 },
        { prop: 'serverIp', label: '本机IP', width: 140 },
        { prop: 'responseTime', label: '响应(ms)', width: 90, align: 'center' },
        { prop: 'createdAt', label: '时间', width: 170 }
      ]
    },
    // 数据处理
    transform: {
      dataTransformer: (records) => {
        if (!Array.isArray(records)) {
          return []
        }
        return records
      }
    }
  })

  /**
   * 搜索处理：时间范围映射为 startDate / endDate
   */
  const handleSearch = (params: VerifyLogSearchForm) => {
    const { dateRange, ...rest } = params
    const query: Partial<VerifyLogSearchParams> = { ...rest }
    if (dateRange && dateRange.length === 2) {
      query.startDate = dateRange[0]
      query.endDate = dateRange[1]
    }
    replaceSearchParams(query)
    getData()
  }

  /**
   * 清空所有验证日志
   */
  const handleClear = async () => {
    try {
      await ElMessageBox.confirm('确定清空所有验证日志？此操作不可撤销', '警告', { type: 'error' })
      await fetchClearVerifyLogs()
      ElMessage.success('日志已清空')
      refreshData()
    } catch {
      return
    }
  }
</script>
