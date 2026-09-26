<template>
  <div class="home-template-manage">
    <ElCard shadow="never" class="art-table-card">
      <div class="panel-header">
        <div>
          <h2 class="panel-title">首页模板管理</h2>
          <p class="panel-subtitle">
            管理 <code>/user/login</code> 的首页展示模板，启用后访问路径保持不变
          </p>
        </div>
        <div class="panel-header-actions">
          <ElButton
            :disabled="loading || defaultEnabled"
            :loading="togglingId === 'default'"
            @click="handleRestoreDefault"
          >
            恢复默认模板
          </ElButton>
          <ElButton :icon="Refresh" circle :loading="loading" @click="loadAll(true)" />
        </div>
      </div>

      <div class="source-bar">
        <ArtSvgIcon icon="ri:archive-2-line" class="source-bar-icon" />
        <template v-if="sourceError">
          <span class="source-bar-name">模板分发中心</span>
          <ElText type="danger" size="small">{{ sourceError }}</ElText>
        </template>
        <template v-else-if="softwareSource">
          <span class="source-bar-name">{{ softwareSource.name }}</span>
          <ElTag type="success" size="small" effect="plain">内置于后端</ElTag>
          <ElText type="info" size="small">
            提供 {{ softwareSource.homeTemplates.length }} 个首页模板 ·
            {{ softwareSource.plugins.length }} 个插件
          </ElText>
        </template>
        <ElText v-else type="info" size="small">正在读取模板分发中心…</ElText>
      </div>

      <div v-loading="loading" class="panel-body">
        <ElAlert
          v-if="loadError"
          :title="loadError"
          type="error"
          show-icon
          :closable="false"
          class="panel-error"
        />
        <ElAlert
          v-if="refreshWarning"
          :title="refreshWarning"
          type="warning"
          show-icon
          :closable="false"
          class="panel-error"
        />

        <ElEmpty
          v-if="!loading && !loadError && !templates.length"
          description="暂无可用的首页模板"
        />

        <div v-else-if="templates.length" class="template-grid">
          <article
            v-for="template in templates"
            :key="String(template.id)"
            class="template-card"
            :class="{ 'is-enabled': template.enabled }"
          >
            <div class="template-preview">
              <img
                v-if="previewSrc(template)"
                :src="previewSrc(template)"
                :alt="`${template.name} 示例图片`"
                loading="lazy"
                @error="markPreviewFailed(template)"
              />
              <div v-else class="preview-placeholder">
                <ArtSvgIcon icon="ri:image-line" />
                <span>暂无示例图片</span>
              </div>
              <ElTag v-if="template.enabled" class="preview-badge" type="success" size="small">
                已启用
              </ElTag>
            </div>

            <div class="template-body">
              <div class="template-name">
                <strong>{{ template.name }}</strong>
                <ElTag
                  v-if="template.sourceType === 'builtin' || template.id === 'default'"
                  type="primary"
                  size="small"
                  effect="plain"
                >
                  内置
                </ElTag>
                <ElTag type="info" size="small" effect="plain">v{{ template.version }}</ElTag>
              </div>

              <p class="template-desc">{{ template.description || '暂无模板简介' }}</p>

              <dl class="template-meta">
                <div>
                  <dt>作者</dt>
                  <dd>
                    <a
                      v-if="template.author?.name && template.author?.url"
                      :href="template.author.url"
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {{ template.author.name }}
                    </a>
                    <span v-else>{{ template.author?.name || '未提供' }}</span>
                    <ElText v-if="template.author?.email" type="info" size="small">
                      {{ template.author.email }}
                    </ElText>
                  </dd>
                </div>
                <div>
                  <dt>来源</dt>
                  <dd :title="template.sourceUrl || template.source">{{ template.source }}</dd>
                </div>
              </dl>
            </div>

            <div class="template-footer">
              <div class="template-status">
                <ElTag v-if="template.updateAvailable" type="warning" size="small">待更新</ElTag>
                <ElTag v-if="!template.available" type="danger" size="small" effect="light">
                  源中已移除
                </ElTag>
                <ElTag v-else-if="template.installed" type="info" size="small" effect="plain">
                  已安装
                </ElTag>
                <ElTag v-else type="warning" size="small" effect="light">未安装</ElTag>
              </div>

              <TemplateActions :template="template" @changed="loadAll()" />
            </div>
          </article>
        </div>
      </div>
    </ElCard>

    <ElBacktop target="#app-main" :right="32" :bottom="32" />
  </div>
</template>

<script setup lang="ts">
  import TemplateActions from './TemplateActions.vue'
  import { computed, onMounted, ref } from 'vue'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { showCaughtError } from '@/utils/http/error-toast'
  import { Refresh } from '@element-plus/icons-vue'
  import {
    fetchEnableHomeTemplate,
    fetchHomeTemplateList,
    fetchSoftwareSourcePlugins,
    type HomeTemplateInfo,
    type SoftwareSourceData
  } from '@/api/system-manage'

  defineOptions({ name: 'HomeTemplate' })

  const loading = ref(false)
  const loadError = ref('')
  const refreshWarning = ref('')
  const sourceError = ref('')
  const togglingId = ref('')
  const templates = ref<HomeTemplateInfo[]>([])
  const softwareSource = ref<SoftwareSourceData | null>(null)
  // 示例图片加载失败的模板，改用占位图，避免出现破图
  const failedPreviews = ref(new Set<string>())

  const defaultEnabled = computed(() =>
    templates.value.some((item) => item.id === 'default' && item.enabled)
  )

  const previewSrc = (template: HomeTemplateInfo): string => {
    if (failedPreviews.value.has(String(template.id))) return ''
    if (template.previewUrl) return template.previewUrl
    // 数据库未记录示例图片时，回退到模板分发中心目录里的同名条目
    const fromSource = softwareSource.value?.homeTemplates.find(
      (item) => item.id === template.templateId
    )
    return fromSource?.previewUrl || ''
  }

  const markPreviewFailed = (template: HomeTemplateInfo) => {
    failedPreviews.value = new Set(failedPreviews.value).add(String(template.id))
  }

  /** 模板分发中心为公开接口，单独失败时不影响模板列表展示 */
  const loadSoftwareSource = async () => {
    sourceError.value = ''
    try {
      softwareSource.value = await fetchSoftwareSourcePlugins()
    } catch (error: any) {
      softwareSource.value = null
      const message = String(error?.message || '').trim()
      sourceError.value = message.includes('未配置远程软件源')
        ? ''
        : message || '模板分发中心读取失败'
    }
  }

  const loadTemplates = async (refresh = false) => {
    loadError.value = ''
    refreshWarning.value = ''
    try {
      const data = await fetchHomeTemplateList(refresh)
      templates.value = data.list || []
      failedPreviews.value = new Set()
      refreshWarning.value = String(data.warning || '').trim()
    } catch (error: any) {
      templates.value = []
      loadError.value = error?.message || '首页模板列表加载失败，请稍后重试'
      showCaughtError(error, loadError.value)
    }
  }

  const loadAll = async (refresh = false) => {
    loading.value = true
    try {
      await loadTemplates(refresh)
      await loadSoftwareSource()
    } finally {
      loading.value = false
    }
  }

  const applyTemplate = async (id: HomeTemplateInfo['id'], successText: string) => {
    togglingId.value = String(id)
    try {
      await fetchEnableHomeTemplate(id)
      ElMessage.success(successText)
      await loadAll()
    } catch (error: any) {
      showCaughtError(error, '操作失败，请稍后重试')
    } finally {
      togglingId.value = ''
    }
  }

  const handleRestoreDefault = async () => {
    try {
      await ElMessageBox.confirm(
        '确认恢复默认首页模板？当前启用的自定义模板将被停用。',
        '恢复默认模板',
        { confirmButtonText: '恢复', cancelButtonText: '取消', type: 'warning' }
      )
    } catch {
      return
    }
    await applyTemplate('default', '已恢复默认首页模板')
  }

  onMounted(loadAll)
</script>

<style lang="scss" scoped>
  .home-template-manage {
    .panel-header {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      margin-bottom: 16px;

      .panel-title {
        margin: 0;
        font-size: 20px;
        color: var(--art-gray-900);
      }

      .panel-subtitle {
        margin: 6px 0 0;
        font-size: 13px;
        color: var(--art-gray-600);

        code {
          padding: 1px 5px;
          font-size: 12px;
          background: var(--el-fill-color-light);
          border-radius: 4px;
        }
      }

      .panel-header-actions {
        display: flex;
        flex-shrink: 0;
        gap: 10px;
        align-items: center;
      }
    }

    .source-bar {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
      align-items: center;
      padding: 10px 14px;
      margin-bottom: 18px;
      background: var(--el-fill-color-lighter);
      border-radius: 8px;

      .source-bar-icon {
        font-size: 16px;
        color: var(--el-color-primary);
      }

      .source-bar-name {
        font-size: 13px;
        font-weight: 500;
        color: var(--art-gray-900);
      }
    }

    .panel-body {
      min-height: 240px;
    }

    .panel-error {
      margin-bottom: 18px;
    }

    .template-grid {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
      gap: 18px;
    }

    .template-card {
      display: flex;
      flex-direction: column;
      overflow: hidden;
      border: 1px solid var(--art-border-color);
      border-radius: 10px;
      transition:
        border-color 0.15s,
        box-shadow 0.15s;

      &:hover {
        box-shadow: 0 4px 16px rgb(0 0 0 / 6%);
      }

      &.is-enabled {
        border-color: var(--el-color-primary-light-5);
      }
    }

    .template-preview {
      position: relative;
      aspect-ratio: 16 / 10;
      background: var(--el-fill-color-lighter);
      border-bottom: 1px solid var(--art-border-color);

      img {
        display: block;
        width: 100%;
        height: 100%;
        object-fit: cover;
      }

      .preview-placeholder {
        display: flex;
        flex-direction: column;
        gap: 8px;
        align-items: center;
        justify-content: center;
        height: 100%;
        font-size: 12px;
        color: var(--art-gray-500);

        svg {
          width: 28px;
          height: 28px;
        }
      }

      .preview-badge {
        position: absolute;
        top: 10px;
        right: 10px;
      }
    }

    .template-body {
      flex: 1;
      padding: 16px 18px 0;

      .template-name {
        display: flex;
        flex-wrap: wrap;
        gap: 8px;
        align-items: center;

        strong {
          font-size: 15px;
          color: var(--art-gray-900);
        }
      }

      .template-desc {
        display: -webkit-box;
        margin: 8px 0 0;
        overflow: hidden;
        font-size: 12px;
        line-height: 1.7;
        color: var(--art-gray-600);
        -webkit-box-orient: vertical;
        -webkit-line-clamp: 2;
      }
    }

    .template-meta {
      margin: 14px 0 0;

      div {
        display: flex;
        gap: 8px;
        align-items: baseline;
        margin-bottom: 6px;
      }

      dt {
        flex-shrink: 0;
        width: 34px;
        font-size: 12px;
        color: var(--art-gray-500);
      }

      dd {
        display: flex;
        gap: 8px;
        align-items: baseline;
        min-width: 0;
        margin: 0;
        overflow: hidden;
        font-size: 12px;
        color: var(--art-gray-800);
        text-overflow: ellipsis;
        white-space: nowrap;

        a {
          color: var(--el-color-primary);
          text-decoration: none;

          &:hover {
            text-decoration: underline;
          }
        }
      }
    }

    .template-footer {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 14px 18px;
      margin-top: 14px;
      border-top: 1px dashed var(--art-border-color);

      .template-status {
        display: flex;
        gap: 8px;
        align-items: center;
      }

      :deep(.el-button.el-button--small) {
        min-width: 64px;
        height: 24px !important;
        padding: 5px 12px;
        margin-left: 0;
      }
    }

    @media (width <= 768px) {
      .panel-header {
        flex-direction: column;
        gap: 12px;
      }

      .template-grid {
        grid-template-columns: 1fr;
      }
    }
  }
</style>
