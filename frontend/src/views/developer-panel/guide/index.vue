<template>
  <div class="developer-docs-page">
    <el-card shadow="never" class="docs-hero mb-4">
      <div class="docs-hero-row">
        <div>
          <h1 class="docs-title">源站开发章程</h1>
          <p class="docs-lead">
            按章程生成可登记的插件包和整站模板。登记包根目录是
            <code>plugin.json</code> 或 <code>template.json</code>（schemaVersion 1，kind 为
            template）。源站只登记外链和校验码，不保存 ZIP。启用模板后，宿主登录弹窗跟随
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
        title='登记模板只上传 template.json。必须写 kind: "template"、schemaVersion: 1、hero.title，以及 hero.primaryAction.type = "login"。启用后登录弹窗跟随 stylePreset（cartoon-blue / fintech-gold）与 theme.primaryColor、theme.backgroundColor、theme.textColor。不要写 login.html，也不要把 index.html 打进同一个 ZIP。插件登记包是 plugin.json。'
      />
    </el-card>

    <StarterPreview />

    <el-card shadow="never" class="docs-shell">
      <div class="docs-layout">
        <aside class="docs-sidebar" aria-label="开发文档目录">
          <div class="sidebar-title">文档目录</div>
          <nav class="section-nav">
            <div v-for="chapter in chapters" :key="chapter.id" class="chapter-block">
              <button
                type="button"
                class="section-nav-item"
                :class="{ active: activeChapterId === chapter.id }"
                @click="selectChapter(chapter.id)"
              >
                <span class="nav-index">{{ chapter.index }}</span>
                {{ chapter.title }}
              </button>
              <div
                v-if="activeChapterId === chapter.id && chapter.sections.length > 1"
                class="subsection-nav"
              >
                <button
                  v-for="section in chapter.sections"
                  :key="`${chapter.id}-${section.id}`"
                  type="button"
                  class="subsection-nav-item"
                  :class="{ active: activeSectionId === section.id }"
                  @click="selectSection(section.id)"
                >
                  {{ section.title }}
                </button>
              </div>
            </div>
          </nav>
        </aside>
        <main class="docs-content">
          <article v-if="activeSection" class="markdown-body" v-html="activeSection.html"></article>
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
  import StarterPreview from './StarterPreview.vue'

  defineOptions({ name: 'DeveloperPanelGuide' })

  interface DocumentSection {
    id: string
    title: string
    html: string
  }

  interface DocChapter {
    id: string
    index: string
    title: string
    sections: DocumentSection[]
  }

  function parseSections(source: string, idPrefix: string): DocumentSection[] {
    const headingPattern = /^##\s+(.+)$/gm
    const headings = [...source.matchAll(headingPattern)]
    const sections: DocumentSection[] = []

    if (headings.length > 0 && headings[0].index !== undefined) {
      const intro = source.slice(0, headings[0].index).trim()
      if (intro) {
        const heading = intro.match(/^#\s+(.+)$/m)
        const title = (heading?.[1] || '包格式').replace(/^重要提示：/, '').trim()
        sections.push({
          id: `${idPrefix}-intro`,
          title,
          html: String(marked.parse(intro, { gfm: true, breaks: false }))
        })
      }
    }

    headings.forEach((heading, index) => {
      const start = heading.index ?? 0
      const end = headings[index + 1]?.index ?? source.length
      const sectionMarkdown = source.slice(start, end).trim()
      sections.push({
        id: `${idPrefix}-section-${index + 1}`,
        title: heading[1].trim(),
        html: String(marked.parse(sectionMarkdown, { gfm: true, breaks: false }))
      })
    })

    if (!sections.length && source.trim()) {
      sections.push({
        id: `${idPrefix}-content`,
        title: '文档内容',
        html: String(marked.parse(source, { gfm: true, breaks: false }))
      })
    }

    return sections
  }

  function chapter(id: string, index: string, title: string, source: string): DocChapter {
    return { id, index, title, sections: parseSections(source, id) }
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

  const activeChapterId = ref(chapters[0].id)
  const activeSectionId = ref(chapters[0].sections[0]?.id || '')
  const activeChapter = computed(() => chapters.find((item) => item.id === activeChapterId.value))
  const activeSection = computed(() =>
    activeChapter.value?.sections.find((section) => section.id === activeSectionId.value)
  )

  function selectChapter(chapterId: string) {
    if (activeChapterId.value === chapterId) return
    activeChapterId.value = chapterId
    const next = chapters.find((item) => item.id === chapterId)
    activeSectionId.value = next?.sections[0]?.id || ''
  }

  function selectSection(sectionId: string) {
    activeSectionId.value = sectionId
  }

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

    code {
      padding: 1px 4px;
      font-family: 'Roboto Mono', monospace;
      font-size: 0.92em;
      background: var(--el-fill-color);
      border-radius: 4px;
    }
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
    max-height: calc(100vh - 32px);
    padding: 14px 10px;
    overflow: auto;
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

  .chapter-block {
    display: flex;
    flex-direction: column;
    gap: 2px;
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

  .subsection-nav {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 2px 0 6px 8px;
  }

  .subsection-nav-item {
    width: 100%;
    padding: 6px 8px 6px 12px;
    color: var(--el-text-color-secondary);
    font-size: 12px;
    line-height: 1.45;
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
    margin-top: 0;
    font-size: 20px;
    border-bottom: 1px solid var(--el-border-color-extra-light);
  }

  :deep(.markdown-body h3) {
    font-size: 17px;
  }

  :deep(.markdown-body h4) {
    font-size: 15px;
  }

  :deep(.markdown-body p) {
    margin: 10px 0;
  }

  :deep(.markdown-body ul),
  :deep(.markdown-body ol) {
    padding-left: 24px;
    margin: 10px 0;
  }

  :deep(.markdown-body li) {
    margin: 4px 0;
  }

  :deep(.markdown-body a) {
    color: var(--el-color-primary);
  }

  :deep(.markdown-body blockquote) {
    padding: 8px 16px;
    margin: 12px 0;
    color: var(--el-text-color-secondary);
    background: var(--el-fill-color-lighter);
    border-left: 4px solid var(--el-color-primary);
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

  :deep(.markdown-body hr) {
    margin: 24px 0;
    border: 0;
    border-top: 1px solid var(--el-border-color-lighter);
  }

  @media (max-width: 768px) {
    .docs-layout {
      grid-template-columns: 1fr;
      gap: 16px;
    }

    .docs-sidebar {
      position: static;
      max-height: none;
    }

    .section-nav {
      display: flex;
      flex-direction: column;
    }

    .subsection-nav {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    :deep(.markdown-body h1) {
      font-size: 21px;
    }

    :deep(.markdown-body h2) {
      font-size: 18px;
    }

    :deep(.markdown-body pre) {
      padding: 12px;
      font-size: 12px;
    }
  }
</style>
