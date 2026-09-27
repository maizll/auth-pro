<!-- 系统文档：电脑左侧目录、右侧正文；手机上目录默认收起。 -->
<template>
  <PublicSiteShell>
    <div class="docs-layout">
      <button class="toc-toggle" type="button" :aria-expanded="tocOpen" @click="tocOpen = !tocOpen">
        {{ tocOpen ? '收起目录' : '目录' }}
      </button>
      <aside class="docs-side" :class="{ 'is-open': tocOpen }">
        <p class="side-label">目录</p>
        <div v-for="category in categories" :key="category.id" class="side-group">
          <h2>{{ category.name }}</h2>
          <RouterLink
            v-for="article in articlesOf(category.id)"
            :key="article.slug"
            :class="{ 'is-current': article.slug === slug }"
            :to="`/docs/${article.slug}`"
            :title="article.title"
            @click="tocOpen = false"
          >
            {{ article.title }}
          </RouterLink>
        </div>
        <div v-if="slug && toc.length" class="heading-toc">
          <h2>本页</h2>
          <a
            v-for="item in toc"
            :key="item.id"
            :href="`#${item.id}`"
            :class="`toc-l${item.level}`"
            :title="item.text"
            @click="tocOpen = false"
          >
            {{ item.text }}
          </a>
        </div>
        <p v-if="!categories.length" class="muted">还没有公开文档。</p>
      </aside>

      <div class="docs-main">
        <div v-if="!slug" class="docs-home">
          <p class="kicker">系统文档</p>
          <h1>文档</h1>
          <p class="lead">从左侧目录选择一篇说明。手机上先点「目录」。</p>
          <RouterLink
            v-for="article in articles"
            :key="article.slug"
            class="article-link"
            :to="`/docs/${article.slug}`"
            :title="article.title"
          >
            <strong>{{ article.title }}</strong>
            <span>{{ article.summary }}</span>
          </RouterLink>
        </div>
        <article v-else class="doc-detail">
          <p class="kicker">{{ current?.category || '系统文档' }}</p>
          <h1>{{ current?.title || '文档' }}</h1>
          <div class="markdown-body" v-html="html" />
          <p v-if="current && !html" class="muted">这篇文档还没有正文。</p>
          <div class="neighbors">
            <RouterLink
              v-if="current?.prev"
              :to="`/docs/${current.prev.slug}`"
              :title="current.prev.title"
            >
              上一篇 {{ current.prev.title }}
            </RouterLink>
            <RouterLink
              v-if="current?.next"
              :to="`/docs/${current.next.slug}`"
              :title="current.next.title"
            >
              下一篇 {{ current.next.title }}
            </RouterLink>
          </div>
        </article>
      </div>
    </div>
  </PublicSiteShell>
</template>

<script setup lang="ts">
  import axios from 'axios'
  import { computed, ref, watch } from 'vue'
  import { useRoute } from 'vue-router'
  import PublicSiteShell from '@/components/site/PublicSiteShell.vue'
  import { renderSiteMarkdown, type SiteTocItem } from '@/utils/markdown'

  defineOptions({ name: 'SiteDocs' })

  interface DocCategory {
    id: number
    name: string
  }
  interface DocArticle {
    id: number
    categoryId: number
    category?: string
    title: string
    slug: string
    summary?: string
    body?: string
    prev?: { slug: string; title: string } | null
    next?: { slug: string; title: string } | null
  }

  const route = useRoute()
  const categories = ref<DocCategory[]>([])
  const articles = ref<DocArticle[]>([])
  const current = ref<DocArticle | null>(null)
  const html = ref('')
  const toc = ref<SiteTocItem[]>([])
  const tocOpen = ref(false)
  const slug = computed(() => (typeof route.params.slug === 'string' ? route.params.slug : ''))

  function articlesOf(categoryId: number) {
    return articles.value.filter((item) => item.categoryId === categoryId)
  }

  async function loadIndex() {
    const { data } = await axios.get('/api/v1/site/docs', { timeout: 8000 })
    if (data?.code !== 200) return
    categories.value = data.data?.categories || []
    articles.value = data.data?.articles || []
  }

  async function loadArticle(value: string) {
    current.value = null
    html.value = ''
    toc.value = []
    tocOpen.value = false
    if (!value) return
    try {
      const { data } = await axios.get(`/api/v1/site/docs/${encodeURIComponent(value)}`, {
        timeout: 8000
      })
      if (data?.code !== 200) return
      current.value = data.data
      const rendered = renderSiteMarkdown(String(data.data?.body || ''))
      html.value = rendered.html
      toc.value = rendered.toc
    } catch {
      current.value = null
    }
  }

  watch(
    slug,
    (value) => {
      void loadArticle(value)
    },
    { immediate: true }
  )
  void loadIndex()
</script>

<style scoped>
  .docs-layout {
    display: grid;
    grid-template-columns: 220px minmax(0, 1fr);
    gap: 28px;
    align-items: start;
    min-width: 0;
  }

  .toc-toggle {
    display: none;
  }

  .docs-side,
  .doc-detail,
  .docs-home {
    min-width: 0;
  }

  .docs-side {
    position: sticky;
    top: 80px;
    padding: 16px;
    background: #fff;
    border-radius: 16px;
  }

  .side-label,
  .kicker {
    margin: 0 0 8px;
    color: var(--remote-primary, #2f6fed);
    font-size: 13px;
    font-weight: 700;
  }

  .side-group,
  .heading-toc {
    margin-top: 14px;
  }

  .docs-side h2 {
    margin: 0 0 6px;
    font-size: 13px;
    color: rgb(28 39 64 / 55%);
  }

  .docs-side a,
  .article-link,
  .neighbors a {
    display: block;
    overflow: hidden;
    color: inherit;
    line-height: 1.6;
    text-decoration: none;
    overflow-wrap: anywhere;
  }

  .docs-side a {
    padding: 4px 0;
  }

  .docs-side a.is-current {
    color: var(--remote-primary, #2f6fed);
    font-weight: 700;
  }

  .toc-l2 {
    padding-left: 12px;
  }

  .toc-l3 {
    padding-left: 24px;
  }

  h1 {
    margin: 0 0 12px;
    overflow-wrap: anywhere;
  }

  .lead,
  .muted,
  .article-link span {
    color: rgb(28 39 64 / 68%);
    line-height: 1.7;
  }

  .article-link {
    padding: 12px 0;
    border-bottom: 1px solid rgb(28 39 64 / 8%);
  }

  .article-link strong {
    display: block;
  }

  .markdown-body {
    min-width: 0;
    padding: 8px 0 20px;
    line-height: 1.75;
    overflow-wrap: anywhere;
  }

  .markdown-body :deep(pre) {
    max-width: 100%;
    overflow-x: auto;
  }

  .markdown-body :deep(img) {
    max-width: 100%;
  }

  .neighbors {
    display: grid;
    gap: 8px;
  }

  .neighbors a {
    color: var(--remote-primary, #2f6fed);
  }

  @media (max-width: 800px) {
    .docs-layout {
      grid-template-columns: minmax(0, 1fr);
      gap: 12px;
    }

    .toc-toggle {
      display: inline-flex;
      align-items: center;
      height: 36px;
      padding: 0 14px;
      color: var(--remote-primary, #2f6fed);
      font-weight: 700;
      background: #fff;
      border: 1px solid rgb(47 111 237 / 25%);
      border-radius: 10px;
      cursor: pointer;
    }

    .docs-side {
      display: none;
      position: static;
    }

    .docs-side.is-open {
      display: block;
    }
  }
</style>
