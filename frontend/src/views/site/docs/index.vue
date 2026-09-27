<!-- 系统文档：分类、文章和目录。正文来自后台，Markdown 用站点共用渲染。 -->
<template>
  <PublicSiteShell>
    <div v-if="!slug" class="docs-home">
      <p class="kicker">系统文档</p>
      <h1>文档</h1>
      <div v-for="category in categories" :key="category.id" class="category">
        <h2>{{ category.name }}</h2>
        <RouterLink
          v-for="article in articlesOf(category.id)"
          :key="article.slug"
          class="article-link"
          :to="`/docs/${article.slug}`"
          :title="article.title"
        >
          <strong>{{ article.title }}</strong>
          <span>{{ article.summary }}</span>
        </RouterLink>
      </div>
      <p v-if="!categories.length" class="muted">还没有公开文档。</p>
    </div>
    <article v-else class="doc-detail">
      <RouterLink class="back" to="/docs">返回文档</RouterLink>
      <p class="kicker">{{ current?.category }}</p>
      <h1>{{ current?.title || '文档' }}</h1>
      <nav v-if="toc.length" class="toc" aria-label="目录">
        <a
          v-for="item in toc"
          :key="item.id"
          :href="`#${item.id}`"
          :class="`toc-l${item.level}`"
          :title="item.text"
        >
          {{ item.text }}
        </a>
      </nav>
      <div class="markdown-body" v-html="html" />
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
  .kicker {
    margin: 0 0 8px;
    color: var(--remote-primary, #2f6fed);
    font-weight: 700;
  }

  h1,
  h2 {
    margin: 0 0 12px;
  }

  .muted,
  .article-link span {
    color: rgb(28 39 64 / 68%);
  }

  .category {
    margin-top: 22px;
  }

  .article-link,
  .back,
  .neighbors a,
  .toc a {
    display: block;
    overflow: hidden;
    color: inherit;
    white-space: nowrap;
    text-decoration: none;
    text-overflow: ellipsis;
  }

  .article-link {
    padding: 12px 0;
    border-bottom: 1px solid rgb(28 39 64 / 8%);
  }

  .article-link strong {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .doc-detail,
  .docs-home {
    min-width: 0;
  }

  .toc {
    margin: 8px 0 20px;
    padding: 12px 14px;
    background: #fff;
    border-radius: 12px;
  }

  .toc-l2 {
    padding-left: 12px;
  }

  .toc-l3 {
    padding-left: 24px;
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

  .back {
    margin-bottom: 12px;
    color: var(--remote-primary, #2f6fed);
  }
</style>
