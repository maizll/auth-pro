<!-- 更新日志：顶部最新版本，下面是左侧时间线和版本卡片。 -->
<template>
  <PublicSiteShell>
    <article v-if="latest" class="latest">
      <div>
        <p class="kicker">最新版本</p>
        <h1>V{{ latest.version }}</h1>
        <time>{{ latest.releasedOn || '日期待补充' }}</time>
      </div>
      <a
        v-if="downloadUrl"
        class="download"
        :href="downloadUrl"
        target="_blank"
        rel="noopener noreferrer"
      >
        下载
      </a>
    </article>
    <div v-else>
      <p class="kicker">更新日志</p>
      <h1>版本记录</h1>
      <p class="muted">还没有公开的更新记录。</p>
    </div>

    <ol v-if="groups.length" class="timeline">
      <li v-for="group in groups" :key="group.version">
        <article class="version-card">
          <header>
            <strong>V{{ group.version }}</strong>
            <time>{{ group.releasedOn || '日期待补充' }}</time>
          </header>
          <ul>
            <li v-for="item in group.items" :key="item.id">
              <span class="tag" :class="`tag-${item.tag}`">{{ tagLabel(item.tag) }}</span>
              <SiteMarkdown class="log-body" :source="item.body" />
            </li>
          </ul>
        </article>
      </li>
    </ol>
  </PublicSiteShell>
</template>

<script setup lang="ts">
  import axios from 'axios'
  import { computed, onMounted, ref } from 'vue'
  import PublicSiteShell from '@/components/site/PublicSiteShell.vue'
  import SiteMarkdown from '@/components/site/SiteMarkdown.vue'

  defineOptions({ name: 'SiteChangelog' })

  interface ChangelogItem {
    id: number
    tag: string
    body: string
  }
  interface ChangelogGroup {
    version: string
    releasedOn: string
    items: ChangelogItem[]
  }

  const groups = ref<ChangelogGroup[]>([])
  const downloadUrl = ref('')
  const latest = computed(() => groups.value[0] || null)
  const labels: Record<string, string> = { added: '新增', improved: '优化', fixed: '修复' }

  function tagLabel(tag: string) {
    return labels[tag] || '新增'
  }

  onMounted(async () => {
    try {
      const [log, info] = await Promise.all([
        axios.get('/api/v1/site/changelog', { timeout: 8000 }),
        axios.get('/api/v1/site/purchase-info', { timeout: 8000 })
      ])
      if (log.data?.code === 200 && Array.isArray(log.data.data?.list)) {
        groups.value = log.data.data.list
      }
      if (info.data?.code === 200) {
        downloadUrl.value = String(info.data.data?.freeDownloadUrl || '')
      }
    } catch {
      groups.value = []
    }
  })
</script>

<style scoped>
  .kicker {
    margin: 0 0 8px;
    color: var(--remote-primary, #2f6fed);
    font-weight: 700;
  }

  h1 {
    margin: 0;
    overflow-wrap: anywhere;
  }

  .muted {
    color: rgb(28 39 64 / 68%);
  }

  .latest {
    display: flex;
    gap: 16px;
    align-items: center;
    justify-content: space-between;
    min-width: 0;
    padding: 24px;
    color: #fff;
    background: linear-gradient(135deg, #1d4ed8 0%, #3b82f6 55%, #60a5fa 100%);
    border-radius: 20px;
  }

  .latest .kicker,
  .latest time {
    color: rgb(255 255 255 / 82%);
  }

  .latest h1 {
    font-size: 36px;
  }

  .latest time {
    display: block;
    margin-top: 6px;
  }

  .download {
    flex: 0 0 auto;
    padding: 8px 16px;
    color: #1d4ed8;
    font-weight: 700;
    white-space: nowrap;
    text-decoration: none;
    background: #fff;
    border-radius: 10px;
  }

  .timeline {
    margin: 28px 0 0;
    padding: 0 0 0 22px;
    list-style: none;
    border-left: 2px solid rgb(47 111 237 / 28%);
  }

  .timeline > li {
    position: relative;
    margin: 0 0 16px;
  }

  .timeline > li::before {
    position: absolute;
    top: 22px;
    left: -28px;
    width: 12px;
    height: 12px;
    content: '';
    background: var(--remote-primary, #2f6fed);
    border: 2px solid #fff;
    border-radius: 50%;
    box-shadow: 0 0 0 2px rgb(47 111 237 / 35%);
  }

  .version-card {
    min-width: 0;
    padding: 16px 18px;
    background: #fff;
    border-radius: 16px;
  }

  .version-card header {
    display: flex;
    gap: 12px;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 12px;
  }

  .version-card time {
    color: rgb(28 39 64 / 55%);
    font-size: 13px;
    white-space: nowrap;
  }

  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  ul li {
    display: flex;
    gap: 8px;
    align-items: flex-start;
    min-width: 0;
    margin-bottom: 10px;
  }

  .tag {
    flex: 0 0 auto;
    padding: 0 8px;
    color: #2f6fed;
    font-size: 12px;
    line-height: 22px;
    white-space: nowrap;
    background: rgb(47 111 237 / 10%);
    border-radius: 999px;
  }

  .tag-improved {
    color: #0f766e;
    background: rgb(15 118 110 / 10%);
  }

  .tag-fixed {
    color: #334155;
    background: rgb(51 65 85 / 10%);
  }

  .log-body {
    flex: 1;
    min-width: 0;
  }

  .log-body :deep(p) {
    margin: 0;
    line-height: 1.7;
    overflow-wrap: anywhere;
  }

  @media (max-width: 640px) {
    .latest {
      align-items: flex-start;
      flex-direction: column;
    }

    .latest h1 {
      font-size: 28px;
    }
  }
</style>
