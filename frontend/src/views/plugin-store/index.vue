<template>
  <div class="plugin-store">
    <ElCard shadow="never" class="art-table-card">
      <div class="store-header">
        <div>
          <h2 class="store-title">应用商店</h2>
          <p class="store-subtitle">
            按软件源 index.categories 筛选（含自定义分类）；首页模板单独分区
          </p>
        </div>
        <div class="store-header-actions">
          <ElButton :icon="FolderAdd" @click="sourceDialogVisible = true">软件源管理</ElButton>
          <ElButton :icon="Refresh" circle :loading="loading" @click="handleRefresh" />
        </div>
      </div>

      <div class="store-toolbar">
        <ElTabs v-model="activeTab" class="store-tabs" @tab-change="loadPlugins">
          <ElTabPane label="全部插件" name="all" />
          <ElTabPane
            v-for="tab in storeTabs"
            :key="tab.name"
            :label="tab.label"
            :name="tab.name"
          />
        </ElTabs>
        <ElInput
          v-model="searchText"
          placeholder="搜索插件名称或描述"
          :prefix-icon="Search"
          clearable
          class="store-search"
          @input="handleSearchInput"
          @clear="loadPlugins"
        />
      </div>

      <div v-loading="loading" class="store-body">
        <ElAlert
          v-if="loadError"
          :title="loadError"
          type="error"
          show-icon
          :closable="false"
          class="store-error"
        />

        <ElAlert
          v-if="templateLoadError && showHomeTemplates"
          :title="templateLoadError"
          type="warning"
          show-icon
          :closable="false"
          class="store-error"
        />

        <section v-if="showHomeTemplates && filteredHomeTemplates.length" class="plugin-section">
          <div class="section-header">
            <strong>首页模板</strong>
            <ElText type="info" size="small">{{ filteredHomeTemplates.length }} 个模板</ElText>
          </div>
          <div class="plugin-grid">
            <div
              v-for="template in filteredHomeTemplates"
              :key="template.id"
              class="plugin-card template-card"
              :class="{ 'is-enabled': template.enabled }"
            >
              <div class="plugin-card-top">
                <div class="plugin-icon template-thumb">
                  <img
                    v-if="templatePreviewSrc(template)"
                    :src="templatePreviewSrc(template)"
                    :alt="`${template.name} 示例图片`"
                    loading="lazy"
                    @error="markTemplatePreviewFailed(template)"
                  />
                  <ArtSvgIcon v-else icon="ri:layout-4-line" />
                </div>
                <div class="plugin-meta">
                  <div class="plugin-name">
                    <strong>{{ template.name }}</strong>
                    <ElTag
                      v-if="template.id === 'default' || template.sourceType === 'builtin'"
                      type="primary"
                      size="small"
                      effect="plain"
                      >内置</ElTag
                    >
                    <ElTag type="info" size="small" effect="plain">v{{ template.version }}</ElTag>
                  </div>
                  <p class="plugin-desc">{{ template.description || '暂无模板描述' }}</p>
                  <div class="template-source" :title="template.sourceUrl || template.source">
                    来源：{{ template.source }} · 作者：{{ template.author?.name || '未提供' }}
                  </div>
                </div>
              </div>
              <div class="plugin-card-bottom">
                <div class="plugin-status">
                  <ElTag v-if="template.updateAvailable" type="warning" size="small">待更新</ElTag>
                  <ElTag v-if="template.enabled" type="success" size="small">已启用</ElTag>
                  <ElTag v-else-if="!template.available" type="danger" size="small">{{
                    template.sourceType === 'upload' ? '安装文件损坏' : '源中已移除'
                  }}</ElTag>
                  <ElTag v-else-if="template.installed" type="info" size="small">已安装</ElTag>
                  <ElTag v-else type="warning" size="small">未安装</ElTag>
                  <ElText v-if="template.sourceType" type="info" size="small">
                    {{
                      template.format === 'zip' || template.sourceType === 'upload'
                        ? 'ZIP 模板'
                        : 'JSON 模板'
                    }}
                  </ElText>
                </div>
                <TemplateActions :template="template" @changed="loadPlugins()" />
              </div>
            </div>
          </div>
        </section>

        <ElEmpty
          v-if="!loadError && !templateLoadError && !hasVisibleContent"
          description="没有匹配的应用或首页模板"
        />
        <section v-for="group in visibleGroups" :key="group.category" class="plugin-section">
          <div class="section-header">
            <strong>{{ group.title }}</strong>
            <ElText type="info" size="small">{{ group.plugins.length }} 个插件</ElText>
          </div>

          <div class="plugin-grid">
            <div
              v-for="plugin in group.plugins"
              :key="plugin.id"
              class="plugin-card"
              :class="{ 'is-enabled': plugin.enabled }"
            >
              <div class="plugin-card-top">
                <div class="plugin-icon">
                  <ArtSvgIcon :icon="plugin.icon" />
                </div>
                <div class="plugin-meta">
                  <div class="plugin-name">
                    <strong>{{ plugin.name }}</strong>
                    <ElTag v-if="plugin.official" type="primary" size="small" effect="plain"
                      >官方</ElTag
                    >
                    <ElTag type="info" size="small" effect="plain">v{{ plugin.version }}</ElTag>
                  </div>
                  <p class="plugin-desc">
                    <template v-if="plugin.homepage">
                      {{ pluginDescriptionPrefix(plugin) }}
                      <a
                        :href="plugin.homepage"
                        target="_blank"
                        rel="noopener noreferrer"
                        @click.stop
                        >{{ plugin.homepage }}</a
                      >
                    </template>
                    <template v-else>{{ plugin.description }}</template>
                  </p>
                </div>
              </div>

              <div class="plugin-card-bottom">
                <div class="plugin-status">
                  <template v-if="plugin.remote">
                    <ElTag type="warning" size="small" effect="light">未安装</ElTag>
                    <ElText type="info" size="small">来源：{{ plugin.source }}</ElText>
                  </template>
                  <template v-else-if="plugin.canEnable === false">
                    <ElTag type="success" size="small" effect="plain">已安装</ElTag>
                    <ElText type="info" size="small">资源包已解压</ElText>
                  </template>
                  <template v-else>
                    <ElTag v-if="plugin.enabled" type="success" size="small" effect="light"
                      >已启用</ElTag
                    >
                    <ElTag v-else type="info" size="small" effect="plain">未启用</ElTag>
                    <ElText
                      :type="plugin.configured ? 'success' : 'warning'"
                      size="small"
                      class="config-status"
                    >
                      <ArtSvgIcon
                        :icon="
                          plugin.configured ? 'ri:checkbox-circle-line' : 'ri:error-warning-line'
                        "
                      />
                      {{ plugin.configured ? '已配置' : '未配置' }}
                    </ElText>
                  </template>
                </div>
                <div class="plugin-actions">
                  <template v-if="plugin.remote">
                    <ElButton
                      type="primary"
                      size="small"
                      :loading="downloadingId === plugin.id"
                      @click="handleDownload(plugin)"
                    >
                      下载并安装
                    </ElButton>
                  </template>
                  <template v-else>
                    <ElButton
                      v-if="configTarget(plugin)"
                      size="small"
                      :disabled="!plugin.enabled"
                      @click="goConfig(plugin)"
                    >
                      配置
                    </ElButton>
                    <ElButton
                      v-if="plugin.canEnable !== false"
                      :type="plugin.enabled ? 'default' : 'primary'"
                      size="small"
                      :loading="togglingId === plugin.id"
                      @click="handleToggle(plugin)"
                    >
                      {{ plugin.enabled ? '停用' : '启用' }}
                    </ElButton>
                    <ElText
                      v-else
                      type="info"
                      size="small"
                      title="启用此插件需部署对应的服务端实现；不会执行 ZIP 中的脚本"
                      >需运行实现</ElText
                    >
                  </template>
                </div>
              </div>
            </div>
          </div>
        </section>
      </div>
    </ElCard>

    <ElDialog v-model="sourceDialogVisible" title="软件源管理" width="560px">
      <div class="source-add">
        <ElInput v-model="newSourceUrl" placeholder="JSON 清单或 Git 仓库地址" />
        <ElInput
          v-model="newSourceName"
          placeholder="名称（可选，留空自动获取）"
          class="source-name-input"
        />
        <ElButton type="primary" :loading="addingSource" @click="handleAddSource">添加</ElButton>
      </div>
      <ElTable :data="sources" size="small" class="source-table">
        <ElTableColumn label="状态" width="70" align="center">
          <template #default="{ row }">
            <span class="source-dot" :class="`is-${row.state}`" />
          </template>
        </ElTableColumn>
        <ElTableColumn prop="name" label="名称" width="130" show-overflow-tooltip />
        <ElTableColumn prop="url" label="地址" show-overflow-tooltip />
        <ElTableColumn label="操作" width="140" align="center">
          <template #default="{ row }">
            <ElButton
              link
              type="primary"
              size="small"
              :loading="refreshingSourceId === row.id"
              @click="handleRefreshSource(row)"
              >刷新</ElButton
            >
            <ElButton link type="danger" size="small" @click="handleDeleteSource(row)"
              >删除</ElButton
            >
          </template>
        </ElTableColumn>
        <template #empty>
          <ElEmpty description="暂无软件源" :image-size="60" />
        </template>
      </ElTable>
      <div class="source-tip">
        支持 JSON 清单 URL（本站源站请用
        <code>/software-source/{app_key}/index.json</code>，每个应用一条，避免串目录）或 HTTP(S)
        Git 仓库地址；Git 仓库根目录需包含 index.json。
      </div>
    </ElDialog>

    <ElBacktop target="#app-main" :right="32" :bottom="32" />
  </div>
</template>

<script setup lang="ts">
  import TemplateActions from '@/views/home-template/TemplateActions.vue'
  import { computed, onMounted, ref } from 'vue'
  import { useRouter } from 'vue-router'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { FolderAdd, Refresh, Search } from '@element-plus/icons-vue'
  import {
    fetchAddPluginSource,
    fetchDeletePluginSource,
    fetchDownloadPlugin,
    fetchHomeTemplateList,
    fetchPluginList,
    fetchRefreshPluginSource,
    fetchTogglePlugin,
    HomeTemplateInfo,
    PluginCategoryGroup,
    PluginInfo,
    PluginSource
  } from '@/api/system-manage'

  defineOptions({ name: 'PluginStore' })

  const router = useRouter()
  const loading = ref(false)
  const togglingId = ref('')
  const downloadingId = ref('')
  const refreshingSourceId = ref<number | null>(null)
  const loadError = ref('')
  const templateLoadError = ref('')
  const categories = ref<PluginCategoryGroup[]>([])
  const sources = ref<PluginSource[]>([])
  const homeTemplates = ref<HomeTemplateInfo[]>([])

  const activeTab = ref('all')
  const searchText = ref('')
  let searchTimer: ReturnType<typeof setTimeout> | undefined

  const sourceDialogVisible = ref(false)
  const addingSource = ref(false)
  const newSourceUrl = ref('')
  const newSourceName = ref('')

  const storeTabs = computed(() => {
    const tabs: { name: string; label: string }[] = []
    const seen = new Set<string>(['all'])
    for (const group of categories.value) {
      if (!group.category || seen.has(group.category)) continue
      seen.add(group.category)
      tabs.push({ name: group.category, label: group.title || group.category })
    }
    if (!seen.has('home-template')) {
      tabs.push({ name: 'home-template', label: '首页模板' })
    }
    return tabs
  })

  const visibleGroups = computed(() => {
    if (activeTab.value === 'all') return categories.value.filter((g) => g.plugins.length)
    return categories.value.filter((g) => g.category === activeTab.value && g.plugins.length)
  })

  const showHomeTemplates = computed(
    () => activeTab.value === 'all' || activeTab.value === 'home-template'
  )
  const filteredHomeTemplates = computed(() => {
    if (!showHomeTemplates.value) return []
    const keyword = searchText.value.trim().toLowerCase()
    if (!keyword) return homeTemplates.value
    return homeTemplates.value.filter((item) =>
      [item.name, item.description, item.templateId, item.source].some((value) =>
        String(value || '')
          .toLowerCase()
          .includes(keyword)
      )
    )
  })
  const hasVisibleContent = computed(
    () => visibleGroups.value.length > 0 || filteredHomeTemplates.value.length > 0
  )

  // 示例图片加载失败的模板改用图标占位，避免出现破图
  const failedTemplatePreviews = ref(new Set<string>())
  const templatePreviewSrc = (template: HomeTemplateInfo): string =>
    failedTemplatePreviews.value.has(String(template.id)) ? '' : template.previewUrl || ''
  const markTemplatePreviewFailed = (template: HomeTemplateInfo) => {
    failedTemplatePreviews.value = new Set(failedTemplatePreviews.value).add(String(template.id))
  }

  const pluginDescriptionPrefix = (plugin: PluginInfo): string => {
    if (!plugin.homepage) return plugin.description
    return plugin.description.replace(plugin.homepage, '')
  }

  // 需要独立配置页的插件；其余按类别归到统一配置页
  const pluginConfigPaths: Record<string, string> = {
    epay: '/system/epay-config',
    'epay-v2': '/system/epay-config'
  }

  // 配置入口；返回空串表示无独立配置页。
  // 实名服务商共用系统配置的实名页签，新增服务商无需再改这里。
  const configTarget = (plugin: PluginInfo): string => {
    if (plugin.canEnable === false) return ''
    const mapped = pluginConfigPaths[plugin.id]
    if (mapped) return mapped
    if (plugin.category === 'realname') return '/system/config?tab=realname'
    return ''
  }

  const loadPlugins = async (refreshTemplates: unknown = false) => {
    loading.value = true
    loadError.value = ''
    templateLoadError.value = ''
    try {
      const [pluginResult, templateResult] = await Promise.allSettled([
        fetchPluginList({ q: searchText.value || undefined }),
        fetchHomeTemplateList(refreshTemplates === true)
      ])

      if (pluginResult.status === 'fulfilled') {
        categories.value = pluginResult.value.categories || []
        sources.value = pluginResult.value.sources || []
      } else {
        categories.value = []
        sources.value = []
        loadError.value = pluginResult.reason?.message || '插件商店加载失败，请稍后重试'
        ElMessage.error(loadError.value)
      }

      if (templateResult.status === 'fulfilled') {
        homeTemplates.value = templateResult.value.list || []
        const warning = String(templateResult.value.warning || '').trim()
        templateLoadError.value = warning
        failedTemplatePreviews.value = new Set()
      } else {
        homeTemplates.value = []
        const reason = templateResult.reason?.message || '模板服务暂时不可用'
        templateLoadError.value = `首页模板加载失败：${reason}。插件商店不受影响`
      }
    } finally {
      loading.value = false
    }
  }

  const handleRefresh = async () => {
    loading.value = true
    try {
      await Promise.all(sources.value.map((source) => fetchRefreshPluginSource(source.id)))
      ElMessage.success('软件源已刷新')
    } catch (error: any) {
      ElMessage.error(error?.message || '部分软件源刷新失败')
    } finally {
      loading.value = false
      await loadPlugins(true)
    }
  }

  const handleRefreshSource = async (source: PluginSource) => {
    refreshingSourceId.value = source.id
    try {
      await fetchRefreshPluginSource(source.id)
      ElMessage.success(`「${source.name}」刷新成功`)
      await loadPlugins()
    } catch (error: any) {
      ElMessage.error(error?.message || '软件源刷新失败')
    } finally {
      refreshingSourceId.value = null
    }
  }

  const handleSearchInput = () => {
    if (searchTimer) clearTimeout(searchTimer)
    searchTimer = setTimeout(loadPlugins, 400)
  }

  const handleToggle = async (plugin: PluginInfo) => {
    if (!plugin.enabled) {
      try {
        await ElMessageBox.confirm(
          `启用「${plugin.name}」后，同分区其他插件将自动停用，确认启用？`,
          '启用插件',
          { confirmButtonText: '启用', cancelButtonText: '取消', type: 'warning' }
        )
      } catch {
        return
      }
    }
    togglingId.value = plugin.id
    try {
      await fetchTogglePlugin(plugin.id, !plugin.enabled)
      ElMessage.success(plugin.enabled ? '插件已停用' : `已启用「${plugin.name}」`)
      await loadPlugins()
    } catch {
      ElMessage.error('操作失败')
    } finally {
      togglingId.value = ''
    }
  }

  const handleDownload = async (plugin: PluginInfo) => {
    downloadingId.value = plugin.id
    try {
      await fetchDownloadPlugin(plugin.id)
      ElMessage.success(`「${plugin.name}」已下载、解压并安装`)
      await loadPlugins()
    } catch (error: any) {
      ElMessage.error(error?.message || '插件下载安装失败')
    } finally {
      downloadingId.value = ''
    }
  }

  const handleAddSource = async () => {
    const url = newSourceUrl.value.trim()
    if (!url) {
      ElMessage.warning('请输入软件源清单地址')
      return
    }
    addingSource.value = true
    try {
      await fetchAddPluginSource(newSourceName.value.trim(), url)
      ElMessage.success('软件源已添加')
      newSourceUrl.value = ''
      newSourceName.value = ''
      await loadPlugins()
    } catch (e: any) {
      ElMessage.error(e?.message || '软件源添加失败，请检查清单地址')
    } finally {
      addingSource.value = false
    }
  }

  const handleDeleteSource = async (row: PluginSource) => {
    try {
      await ElMessageBox.confirm(
        `确认删除软件源「${row.name}」？已下载到本地的插件不受影响。`,
        '删除软件源',
        {
          confirmButtonText: '删除',
          cancelButtonText: '取消',
          type: 'warning'
        }
      )
    } catch {
      return
    }
    try {
      await fetchDeletePluginSource(row.id)
      ElMessage.success('软件源已删除')
      await loadPlugins()
    } catch {
      ElMessage.error('删除失败')
    }
  }

  const goConfig = (plugin: PluginInfo) => {
    const target = configTarget(plugin)
    if (target) router.push(target)
  }

  onMounted(() => {
    loadPlugins()
  })
</script>

<style lang="scss" scoped>
  .plugin-store {
    .store-error {
      margin-bottom: 18px;
    }

    .template-source {
      margin-top: 8px;
      overflow: hidden;
      font-size: 12px;
      color: var(--art-gray-500);
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .template-thumb {
      overflow: hidden;
      background: var(--el-fill-color-lighter);

      img {
        display: block;
        width: 100%;
        height: 100%;
        object-fit: cover;
      }
    }

    .store-header {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      margin-bottom: 8px;

      .store-title {
        margin: 0;
        font-size: 20px;
        color: var(--art-gray-900);
      }

      .store-subtitle {
        margin: 6px 0 0;
        font-size: 13px;
        color: var(--art-gray-600);
      }

      .store-header-actions {
        display: flex;
        gap: 10px;
        align-items: center;
      }
    }

    .store-toolbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      margin-bottom: 16px;

      .store-tabs {
        flex: 1;
        min-width: 0;

        :deep(.el-tabs__header) {
          margin-bottom: 0;
        }
      }

      .store-search {
        width: 260px;
        flex-shrink: 0;
      }
    }

    .store-body {
      min-height: 200px;
    }

    .plugin-section {
      margin-bottom: 28px;

      &:last-child {
        margin-bottom: 0;
      }

      .section-header {
        display: flex;
        align-items: center;
        gap: 10px;
        padding-bottom: 12px;
        margin-bottom: 16px;
        border-bottom: 1px solid var(--art-border-color);

        strong {
          font-size: 15px;
          color: var(--art-gray-900);
        }
      }
    }

    .plugin-grid {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
      gap: 16px;
    }

    .plugin-card {
      display: flex;
      flex-direction: column;
      justify-content: space-between;
      padding: 18px;
      border: 1px solid var(--art-border-color);
      border-radius: 8px;
      transition:
        border-color 0.15s,
        box-shadow 0.15s;

      &:hover {
        box-shadow: 0 4px 16px rgb(0 0 0 / 6%);
      }

      &.is-enabled {
        border-color: var(--el-color-primary-light-5);
      }

      .plugin-card-top {
        display: flex;
        gap: 14px;
      }

      .plugin-icon {
        display: flex;
        align-items: center;
        justify-content: center;
        width: 46px;
        height: 46px;
        font-size: 24px;
        color: var(--el-color-primary);
        background: var(--el-color-primary-light-9);
        border-radius: 10px;
        flex-shrink: 0;
      }

      .plugin-meta {
        flex: 1;
        min-width: 0;

        .plugin-name {
          display: flex;
          align-items: center;
          gap: 8px;
          flex-wrap: wrap;

          strong {
            font-size: 14px;
            color: var(--art-gray-900);
          }
        }

        .plugin-desc {
          margin: 6px 0 0;
          font-size: 12px;
          line-height: 1.6;
          color: var(--art-gray-600);

          a {
            color: var(--el-color-primary);
            text-decoration: none;
            overflow-wrap: anywhere;

            &:hover {
              text-decoration: underline;
            }
          }
        }
      }

      .plugin-card-bottom {
        display: flex;
        align-items: center;
        justify-content: space-between;
        padding-top: 14px;
        margin-top: 14px;
        border-top: 1px dashed var(--art-border-color);

        .plugin-status {
          display: flex;
          align-items: center;
          gap: 8px;

          .config-status {
            display: inline-flex;
            align-items: center;
            gap: 3px;
          }
        }

        .plugin-actions {
          display: flex;
          gap: 8px;

          :deep(.el-button.el-button--small) {
            min-width: 64px;
            height: 24px !important;
            padding: 5px 12px;
            margin-left: 0;
          }
        }
      }
    }

    .source-add {
      display: flex;
      gap: 10px;
      margin-bottom: 16px;

      .source-name-input {
        width: 180px;
        flex-shrink: 0;
      }
    }

    .source-table {
      margin-bottom: 12px;
    }

    .source-dot {
      display: inline-block;
      width: 8px;
      height: 8px;
      border-radius: 50%;

      &.is-ok {
        background: var(--el-color-success);
      }

      &.is-error {
        background: var(--el-color-danger);
      }

      &.is-unknown {
        background: var(--el-color-info);
      }
    }

    .source-tip {
      font-size: 12px;
      color: var(--art-gray-600);
    }

    @media (max-width: 768px) {
      .store-toolbar {
        flex-direction: column;
        align-items: stretch;

        .store-search {
          width: 100%;
        }
      }

      .source-add {
        flex-direction: column;

        .source-name-input {
          width: 100%;
        }
      }
    }
  }
</style>
