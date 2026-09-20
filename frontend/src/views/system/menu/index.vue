<!-- 菜单管理页面 -->
<template>
  <div class="menu-page art-full-height">
    <!-- 搜索栏 -->
    <ArtSearchBar
      v-model="formFilters"
      :items="formItems"
      :showExpand="false"
      @reset="handleReset"
      @search="handleSearch"
    />

    <ElCard class="art-table-card">
      <!-- 表格头部 -->
      <ArtTableHeader
        :showZebra="false"
        :loading="loading"
        v-model:columns="columnChecks"
        @refresh="handleRefresh"
      >
        <template #left>
          <ElButton v-auth="'add'" @click="handleAddMenu" v-ripple> 添加菜单 </ElButton>
          <ElButton @click="toggleExpand" v-ripple>
            {{ isExpanded ? '收起' : '展开' }}
          </ElButton>
        </template>
      </ArtTableHeader>

      <ArtTable
        ref="tableRef"
        rowKey="id"
        :loading="loading"
        :columns="columns"
        :data="filteredTableData"
        :stripe="false"
        :tree-props="{ children: 'children', hasChildren: 'hasChildren' }"
        :default-expand-all="false"
      />

      <!-- 菜单弹窗 -->
      <MenuDialog
        v-model:visible="dialogVisible"
        :type="dialogType"
        :editData="editData"
        :menus="tableData"
        :lockType="lockMenuType"
        @submit="handleSubmit"
      />
    </ElCard>
  </div>
</template>

<script setup lang="ts">
  import ArtButtonTable from '@/components/core/forms/art-button-table/index.vue'
  import { useTableColumns } from '@/hooks/core/useTableColumns'
  import MenuDialog from './modules/menu-dialog.vue'
  import { fetchMenuManageList, fetchCreateMenu, fetchUpdateMenu, fetchDeleteMenu } from '@/api/system-manage'
  import { toMenuSavePayload } from '@/utils/form/menu-form'
  import { formatManageMenuName, resolveManageMenuTree, stripDemoMenus } from '@/utils/form/menu-title'
  import { reloadDynamicMenus } from '@/router/guards/beforeEach'
  import { ElTag, ElMessage, ElMessageBox } from 'element-plus'
  import { useRouter } from 'vue-router'

  defineOptions({ name: 'Menus' })

  const router = useRouter()

  interface MenuItem {
    id: number
    parentId: number
    name: string
    path: string
    component: string
    redirect: string
    title: string
    icon: string
    sort: number
    isHide: boolean
    isHideTab: boolean
    isFullPage: boolean
    keepAlive: boolean
    fixedTab: boolean
    enabled: boolean
    children?: MenuItem[]
  }

  const loading = ref(false)
  const isExpanded = ref(false)
  const tableRef = ref()

  const dialogVisible = ref(false)
  const dialogType = ref<'menu' | 'button'>('menu')
  const editData = ref<MenuItem | null>(null)
  const lockMenuType = ref(false)

  const initialSearchState = { name: '', route: '' }
  const formFilters = reactive({ ...initialSearchState })
  const appliedFilters = reactive({ ...initialSearchState })

  const formItems = computed(() => [
    { label: '菜单名称', key: 'name', type: 'input', props: { clearable: true } },
    { label: '路由地址', key: 'route', type: 'input', props: { clearable: true } }
  ])

  onMounted(() => {
    getMenuList()
  })

  const getMenuList = async (): Promise<void> => {
    loading.value = true
    try {
      const res = await fetchMenuManageList()
      tableData.value = resolveManageMenuTree(stripDemoMenus(res || []))
    } finally {
      loading.value = false
    }
  }

  const getMenuTypeTag = (row: MenuItem): 'primary' | 'info' => {
    if (row.children?.length) return 'info'
    return 'primary'
  }

  const getMenuTypeText = (row: MenuItem): string => {
    if (row.children?.length) return '目录'
    return '菜单'
  }

  const { columnChecks, columns } = useTableColumns(() => [
    {
      prop: 'title',
      label: '菜单名称',
      minWidth: 160,
      formatter: (row: MenuItem) => formatManageMenuName(row)
    },
    {
      prop: 'type',
      label: '类型',
      width: 80,
      formatter: (row: MenuItem) =>
        h(ElTag, { type: getMenuTypeTag(row) }, () => getMenuTypeText(row))
    },
    { prop: 'icon', label: '图标', width: 80, formatter: (row: MenuItem) => row.icon || '-' },
    { prop: 'path', label: '路由', minWidth: 140, formatter: (row: MenuItem) => row.path },
    {
      prop: 'component',
      label: '组件',
      minWidth: 160,
      formatter: (row: MenuItem) => row.component || '-'
    },
    { prop: 'sort', label: '排序', width: 70 },
    {
      prop: 'isHide',
      label: '侧栏',
      width: 80,
      formatter: (row: MenuItem) =>
        h(ElTag, { type: row.isHide ? 'info' : 'success' }, () => (row.isHide ? '隐藏' : '显示'))
    },
    {
      prop: 'enabled',
      label: '状态',
      width: 80,
      formatter: (row: MenuItem) =>
        h(ElTag, { type: row.enabled ? 'success' : 'danger' }, () =>
          row.enabled ? '启用' : '禁用'
        )
    },
    {
      prop: 'operation',
      label: '操作',
      width: 160,
      align: 'right',
      formatter: (row: MenuItem) => {
        return h('div', { style: 'text-align: right' }, [
          h(ArtButtonTable, { type: 'edit', onClick: () => handleEditMenu(row) }),
          h(ArtButtonTable, { type: 'delete', onClick: () => handleDeleteMenu(row) })
        ])
      }
    }
  ])

  const tableData = ref<MenuItem[]>([])

  const handleReset = (): void => {
    Object.assign(formFilters, { ...initialSearchState })
    Object.assign(appliedFilters, { ...initialSearchState })
    getMenuList()
  }

  const handleSearch = (): void => {
    Object.assign(appliedFilters, { ...formFilters })
  }

  const handleRefresh = (): void => {
    getMenuList()
  }

  const searchMenu = (items: MenuItem[]): MenuItem[] => {
    const results: MenuItem[] = []
    for (const item of items) {
      const searchName = appliedFilters.name?.toLowerCase().trim() || ''
      const searchRoute = appliedFilters.route?.toLowerCase().trim() || ''
      const menuTitle = formatManageMenuName(item).toLowerCase()
      const menuPath = (item.path || '').toLowerCase()
      const nameMatch = !searchName || menuTitle.includes(searchName)
      const routeMatch = !searchRoute || menuPath.includes(searchRoute)
      if (item.children?.length) {
        const matchedChildren = searchMenu(item.children)
        if (matchedChildren.length > 0) {
          results.push({ ...item, children: matchedChildren })
          continue
        }
      }
      if (nameMatch && routeMatch) results.push({ ...item })
    }
    return results
  }

  const filteredTableData = computed(() => searchMenu(tableData.value))

  const handleAddMenu = (): void => {
    dialogType.value = 'menu'
    editData.value = null
    dialogVisible.value = true
  }
  const handleEditMenu = (row: MenuItem): void => {
    dialogType.value = 'menu'
    editData.value = {
      ...row,
      title: formatManageMenuName(row)
    }
    dialogVisible.value = true
  }

  const refreshSidebar = async (): Promise<void> => {
    try {
      await reloadDynamicMenus(router)
    } catch (error) {
      console.warn('[菜单管理] 侧栏刷新失败，请硬刷新一次', error)
    }
  }

  const handleSubmit = async (data: Record<string, any>): Promise<void> => {
    const title = String(data.name || '').trim()
    if (!title) {
      ElMessage.error('请输入菜单名称')
      return
    }
    try {
      if (data.id) {
        await fetchUpdateMenu(data.id, toMenuSavePayload(data))
      } else {
        await fetchCreateMenu(toMenuSavePayload(data))
      }
      dialogVisible.value = false
      await getMenuList()
      await refreshSidebar()
    } catch {
      /* request already toasts */
    }
  }

  const handleDeleteMenu = async (row: MenuItem): Promise<void> => {
    try {
      await ElMessageBox.confirm('确定要删除该菜单吗？删除后无法恢复', '提示', {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      })
      await fetchDeleteMenu(row.id)
      await getMenuList()
      await refreshSidebar()
    } catch (error: any) {
      if (error === 'cancel' || error === 'close') return
      /* request already toasts API errors such as 该菜单下有子菜单 */
    }
  }

  const toggleExpand = (): void => {
    isExpanded.value = !isExpanded.value
    nextTick(() => {
      if (tableRef.value?.elTableRef && filteredTableData.value) {
        const processRows = (rows: MenuItem[]) => {
          rows.forEach((row) => {
            if (row.children?.length) {
              tableRef.value.elTableRef.toggleRowExpansion(row, isExpanded.value)
              processRows(row.children)
            }
          })
        }
        processRows(filteredTableData.value)
      }
    })
  }
</script>
