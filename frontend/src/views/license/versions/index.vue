<template>
  <div class="license-versions-hub art-full-height">
    <ElCard class="art-table-card no-search-card" shadow="never">
      <ArtTableHeader v-model:columns="columnChecks" :loading="loading" @refresh="refreshData">
        <template #left>
          <ElSpace wrap>
            <ElText>先选择应用，再管理版本。没有应用时请先去创建。</ElText>
            <ElButton @click="goCreateApp" v-ripple>创建应用</ElButton>
          </ElSpace>
        </template>
      </ArtTableHeader>

      <ElEmpty v-if="!loading && !data.length" description="还没有应用，请先创建应用再发布版本">
        <ElButton type="primary" @click="goCreateApp">去应用管理</ElButton>
      </ElEmpty>

      <ArtTable v-else :loading="loading" :data="data" :columns="columns">
        <template #version="{ row }">
          <span>{{ row.recentVersion || '未发布' }}</span>
          <span v-if="row.versionCount" class="version-count">{{ row.versionCount }}</span>
        </template>
        <template #enabled="{ row }">
          <ElTag :type="row.enabled ? 'success' : 'info'" size="small">
            {{ row.enabled ? '启用' : '禁用' }}
          </ElTag>
        </template>
        <template #operation="{ row }">
          <ElButton link type="primary" @click="handleVersions(row)">管理版本</ElButton>
        </template>
      </ArtTable>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
  import { useRouter } from 'vue-router'
  import { useTable } from '@/hooks/core/useTable'
  import { fetchLicenseAppList, type LicenseAppItem } from '@/api/license-manage'

  defineOptions({ name: 'LicenseVersionsHub' })

  const router = useRouter()

  const { columns, columnChecks, data, loading, refreshData } = useTable({
    core: {
      apiFn: fetchLicenseAppList,
      apiParams: {},
      columnsFactory: () => [
        { type: 'index', width: 60, label: '序号' },
        { prop: 'name', label: '应用名称', minWidth: 180, showOverflowTooltip: true },
        { prop: 'appKey', label: 'AppKey', minWidth: 220, showOverflowTooltip: true },
        { prop: 'version', label: '最新版本', minWidth: 140, useSlot: true },
        { prop: 'enabled', label: '状态', width: 90, align: 'center', useSlot: true },
        { prop: 'operation', label: '操作', width: 120, fixed: 'right', useSlot: true }
      ]
    }
  })

  const goCreateApp = () => {
    router.push('/license/apps')
  }

  const handleVersions = (row: LicenseAppItem) => {
    router.push(`/license/apps/${row.id}/versions`)
  }

  let activatedOnce = false
  onActivated(() => {
    if (activatedOnce) {
      refreshData()
    }
    activatedOnce = true
  })
</script>

<style scoped lang="scss">
  .license-versions-hub {
    .no-search-card {
      margin-top: 0;
    }

    .version-count {
      margin-left: 6px;
      font-size: 12px;
      color: var(--art-gray-600);
    }
  }
</style>
