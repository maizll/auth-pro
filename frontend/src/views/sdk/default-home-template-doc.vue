<template>
  <div class="template-doc-page">
    <ElCard shadow="never" class="template-doc-card">
      <template #header>
        <div class="template-doc-header">
          <div>
            <h2>首页模版文档</h2>
            <p>首页的页面功能、接口说明和开发约定。</p>
          </div>
          <ElTag type="primary" size="large">Home Template</ElTag>
        </div>
      </template>

      <div class="template-doc-layout">
        <aside class="template-doc-sidebar" aria-label="首页模版文档目录">
          <div class="sidebar-title">文档目录</div>
          <nav class="section-nav">
            <button
              v-for="section in sections"
              :key="section.id"
              type="button"
              class="section-nav-item"
              :class="{ active: activeSectionId === section.id }"
              @click="selectSection(section.id)"
            >
              {{ section.title }}
            </button>
          </nav>
        </aside>

        <main class="template-doc-content">
          <article v-if="activeSection" class="markdown-body" v-html="activeSection.html"></article>
        </main>
      </div>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
  import { computed, ref } from 'vue'
  import { marked } from 'marked'
  import markdown from './default-home-template.md?raw'

  defineOptions({ name: 'DefaultHomeTemplateDoc' })

  interface DocumentSection {
    id: string
    title: string
    html: string
  }

  function parseSections(source: string): DocumentSection[] {
    const headingPattern = /^##\s+(.+)$/gm
    const headings = [...source.matchAll(headingPattern)]
    const sections: DocumentSection[] = []

    if (headings.length > 0 && headings[0].index !== undefined) {
      const intro = source.slice(0, headings[0].index).trim()
      if (intro) {
        sections.push({
          id: 'template-package-format',
          title: '模版压缩包格式',
          html: String(marked.parse(intro, { gfm: true, breaks: false }))
        })
      }
    }

    headings.forEach((heading, index) => {
      const start = heading.index ?? 0
      const end = headings[index + 1]?.index ?? source.length
      const sectionMarkdown = source.slice(start, end).trim()
      const title = heading[1].trim()
      sections.push({
        id: `template-doc-section-${index + 1}`,
        title,
        html: String(marked.parse(sectionMarkdown, { gfm: true, breaks: false }))
      })
    })

    if (!sections.length && source.trim()) {
      sections.push({
        id: 'template-doc-content',
        title: '文档内容',
        html: String(marked.parse(source, { gfm: true, breaks: false }))
      })
    }

    return sections
  }

  const sections = parseSections(markdown)
  const activeSectionId = ref(sections[0]?.id || '')
  const activeSection = computed(() =>
    sections.find((section) => section.id === activeSectionId.value)
  )

  function selectSection(sectionId: string) {
    activeSectionId.value = sectionId
  }
</script>

<style scoped lang="scss">
  .template-doc-page {
    padding: 16px;
  }

  .template-doc-card {
    max-width: 1180px;
  }

  .template-doc-header {
    display: flex;
    gap: 16px;
    align-items: center;
    justify-content: space-between;

    h2 {
      margin: 0 0 8px;
      font-size: 22px;
      font-weight: 700;
    }

    p {
      margin: 0;
      color: var(--el-text-color-secondary);
    }
  }

  .template-doc-layout {
    display: grid;
    grid-template-columns: 220px minmax(0, 1fr);
    gap: 24px;
    align-items: start;
  }

  .template-doc-sidebar {
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
    transition: 0.2s ease;

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

  .template-doc-content {
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

  :deep(.markdown-body code) {
    padding: 2px 6px;
    color: var(--el-color-danger);
    font-family: 'Roboto Mono', monospace;
    font-size: 0.9em;
    background: var(--el-fill-color);
    border-radius: 4px;
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

  :deep(.markdown-body pre code) {
    padding: 0;
    color: inherit;
    background: transparent;
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

  :deep(.markdown-body hr) {
    margin: 24px 0;
    border: 0;
    border-top: 1px solid var(--el-border-color-lighter);
  }

  @media (max-width: 768px) {
    .template-doc-header {
      align-items: flex-start;
      flex-direction: column;
    }

    .template-doc-layout {
      grid-template-columns: 1fr;
      gap: 16px;
    }

    .template-doc-sidebar {
      position: static;
    }

    .section-nav {
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
