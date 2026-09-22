<template>
  <div class="developer-docs-page">
    <el-card shadow="never" class="docs-hero mb-4">
      <div class="docs-hero-row">
        <div>
          <h1 class="docs-title">源站开发章程</h1>
          <p class="docs-lead">
            按章程生成可登记的插件包和整站模板。源站只登记外链和校验码，不保存
            ZIP。模板是一份 template.json。启用后，宿主登录弹窗跟随
            stylePreset，以及 theme.primaryColor、theme.backgroundColor、theme.textColor。
          </p>
        </div>
        <div class="header-actions">
          <el-button :loading="downloadingStarter" @click="handleDownloadStarter">
            下载入门包 ZIP
          </el-button>
          <el-button type="primary" :loading="downloadingSkill" @click="handleDownloadSkill">
            下载 AI Skill
          </el-button>
        </div>
      </div>
      <el-alert
        type="warning"
        :closable="false"
        show-icon
        class="mt-4"
        title="登记模板只上传 template.json。必须写 kind: template、schemaVersion: 1、hero.title，以及 hero.primaryAction.type = login。启用后登录弹窗跟随 stylePreset 与 theme 三色（primaryColor、backgroundColor、textColor），不要写 login.html，也不要把 index.html 打进同一个 ZIP。"
      />
    </el-card>

    <el-card shadow="never" class="docs-shell">
      <div class="docs-layout">
        <aside class="docs-sidebar" aria-label="开发文档目录">
          <div class="sidebar-title">文档目录</div>
          <nav class="section-nav">
            <button
              v-for="chapter in chapters"
              :key="chapter.id"
              type="button"
              class="section-nav-item"
              :class="{ active: activeId === chapter.id }"
              @click="activeId = chapter.id"
            >
              <span class="nav-index">{{ chapter.index }}</span>
              {{ chapter.title }}
            </button>
          </nav>
        </aside>
        <main class="docs-content">
          <article v-if="activeChapter" class="markdown-body" v-html="activeChapter.html"></article>
        </main>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
  import { computed, ref } from 'vue'
  import { marked } from 'marked'
  import { ElMessage } from 'element-plus'
  import charterDoc from '@developer-docs/charter.md?raw'
  import pluginDoc from '@developer-docs/plugin-package.md?raw'
  import templateDoc from '@developer-docs/template-package.md?raw'
  import packagingDoc from '@developer-docs/packaging.md?raw'
  import validationDoc from '@developer-docs/validation.md?raw'
  import versionsDoc from '@developer-docs/versions.md?raw'
  import reviewDoc from '@developer-docs/review-and-catalog.md?raw'
  import {
    downloadSourceDeveloperSkill,
    downloadSourceDeveloperStarter
  } from '@/api/source-developer'

  defineOptions({ name: 'DeveloperPanelGuide' })

  interface DocChapter {
    id: string
    index: string
    title: string
    html: string
  }

  function chapter(id: string, index: string, title: string, source: string): DocChapter {
    return {
      id,
      index,
      title,
      html: String(marked.parse(source, { gfm: true, breaks: false }))
    }
  }

  const chapters: DocChapter[] = [
    chapter('charter', '00', '开发者章程', charterDoc),
    chapter('plugin', '01', '插件清单', pluginDoc),
    chapter('template', '02', '整站模板', templateDoc),
    chapter('packaging', '03', '打包与登记', packagingDoc),
    chapter('validation', '04', '拒绝与改法', validationDoc),
    chapter('versions', '05', '新版本', versionsDoc),
    chapter('review', '06', '审核之后', reviewDoc)
  ]

  const activeId = ref(chapters[0].id)
  const activeChapter = computed(() => chapters.find((item) => item.id === activeId.value))

  const downloadingStarter = ref(false)
  const downloadingSkill = ref(false)

  async function handleDownloadStarter() {
    downloadingStarter.value = true
    try {
      await downloadSourceDeveloperStarter()
      ElMessage.success('已开始下载入门包')
    } catch (error: unknown) {
      ElMessage.error(error instanceof Error ? error.message : '下载入门包失败')
    } finally {
      downloadingStarter.value = false
    }
  }

  async function handleDownloadSkill() {
    downloadingSkill.value = true
    try {
      await downloadSourceDeveloperSkill()
      ElMessage.success('已开始下载 SKILL.md')
    } catch (error: unknown) {
      ElMessage.error(error instanceof Error ? error.message : '下载 Skill 失败')
    } finally {
      downloadingSkill.value = false
    }
  }
</script>

<style scoped lang="scss">
  .docs-hero-row {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    flex-wrap: wrap;
  }

  .docs-title {
    margin: 0;
    font-size: 24px;
    font-weight: 700;
    color: var(--el-text-color-primary);
  }

  .docs-lead {
    margin: 10px 0 0;
    max-width: 720px;
    font-size: 14px;
    line-height: 1.7;
    color: var(--el-text-color-regular);
  }

  .header-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .mt-4 {
    margin-top: 16px;
  }

  .mb-4 {
    margin-bottom: 16px;
  }

  .docs-layout {
    display: grid;
    grid-template-columns: 240px minmax(0, 1fr);
    gap: 24px;
    align-items: start;
  }

  .docs-sidebar {
    position: sticky;
    top: 16px;
    padding: 14px 10px;
    background: var(--el-fill-color-lighter);
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 10px;
  }

  .sidebar-title {
    padding: 0 10px 10px;
    color: var(--el-text-color-primary);
    font-size: 14px;
    font-weight: 700;
    border-bottom: 1px solid var(--el-border-color-lighter);
  }

  .section-nav {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding-top: 10px;
  }

  .section-nav-item {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 9px 10px;
    color: var(--el-text-color-regular);
    font-size: 13px;
    line-height: 1.5;
    text-align: left;
    cursor: pointer;
    background: transparent;
    border: 0;
    border-radius: 6px;

    &:hover {
      color: var(--el-color-primary);
      background: var(--el-color-primary-light-9);
    }

    &.active {
      color: var(--el-color-primary);
      font-weight: 600;
      background: var(--el-color-primary-light-9);
      box-shadow: inset 3px 0 0 var(--el-color-primary);
    }
  }

  .nav-index {
    flex-shrink: 0;
    font-size: 11px;
    color: var(--el-text-color-secondary);
  }

  .docs-content {
    min-width: 0;
  }

  .markdown-body {
    max-width: 100%;
    min-height: 520px;
    overflow-x: auto;
    color: var(--el-text-color-regular);
    line-height: 1.8;
  }

  :deep(.markdown-body h1),
  :deep(.markdown-body h2),
  :deep(.markdown-body h3),
  :deep(.markdown-body h4) {
    margin: 24px 0 12px;
    color: var(--el-text-color-primary);
    line-height: 1.4;
  }

  :deep(.markdown-body h1) {
    padding-bottom: 12px;
    margin-top: 0;
    font-size: 24px;
    border-bottom: 1px solid var(--el-border-color-lighter);
  }

  :deep(.markdown-body h2) {
    padding-bottom: 8px;
    font-size: 20px;
    border-bottom: 1px solid var(--el-border-color-extra-light);
  }

  :deep(.markdown-body table) {
    width: 100%;
    margin: 14px 0;
    border-collapse: collapse;
  }

  :deep(.markdown-body th),
  :deep(.markdown-body td) {
    padding: 8px 12px;
    text-align: left;
    border: 1px solid var(--el-border-color-lighter);
  }

  :deep(.markdown-body th) {
    background: var(--el-fill-color-light);
  }

  :deep(.markdown-body pre) {
    padding: 16px;
    margin: 14px 0;
    overflow-x: auto;
    color: #d1d5db;
    line-height: 1.7;
    background: #111827;
    border-radius: 10px;
  }

  :deep(.markdown-body code) {
    padding: 2px 6px;
    color: var(--el-color-danger);
    font-family: 'Roboto Mono', monospace;
    font-size: 0.9em;
    background: var(--el-fill-color);
    border-radius: 4px;
  }

  :deep(.markdown-body pre code) {
    padding: 0;
    color: inherit;
    background: transparent;
  }

  :deep(.markdown-body ul),
  :deep(.markdown-body ol) {
    padding-left: 24px;
  }

  @media (max-width: 768px) {
    .docs-layout {
      grid-template-columns: 1fr;
    }

    .docs-sidebar {
      position: static;
    }

    .section-nav {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
</style>
