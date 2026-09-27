<!-- 更新日志时间线。日期是北京时间的发布日，标签区分新增、优化和修复。 -->
<template>
  <PublicSiteShell>
    <p class="kicker">更新日志</p>
    <h1>版本记录</h1>
    <p v-if="!groups.length" class="muted">还没有公开的更新记录。</p>
    <ol v-else class="timeline">
      <li v-for="group in groups" :key="group.version">
        <div class="version">
          <strong>V{{ group.version }}</strong>
          <time v-if="group.releasedOn">{{ group.releasedOn }}</time>
          <time v-else>日期待补充</time>
        </div>
        <ul>
          <li v-for="item in group.items" :key="item.id">
            <span class="tag" :class="`tag-${item.tag}`">{{ tagLabel(item.tag) }}</span>
            <p>{{ item.body }}</p>
          </li>
        </ul>
      </li>
    </ol>
  </PublicSiteShell>
</template>

<script setup lang="ts">
  import axios from 'axios'
  import { onMounted, ref } from 'vue'
  import PublicSiteShell from '@/components/site/PublicSiteShell.vue'

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
  const labels: Record<string, string> = { added: '新增', improved: '优化', fixed: '修复' }

  function tagLabel(tag: string) {
    return labels[tag] || '新增'
  }

  onMounted(async () => {
    try {
      const { data } = await axios.get('/api/v1/site/changelog', { timeout: 8000 })
      if (data?.code === 200 && Array.isArray(data.data?.list)) {
        groups.value = data.data.list
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
    margin: 0 0 20px;
  }

  .muted {
    color: rgb(28 39 64 / 68%);
  }

  .timeline {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .timeline > li {
    display: grid;
    grid-template-columns: 140px 1fr;
    gap: 16px;
    padding: 16px 0;
    border-top: 1px solid rgb(28 39 64 / 8%);
  }

  .version strong,
  .version time {
    display: block;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .version time {
    color: rgb(28 39 64 / 60%);
    font-size: 13px;
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

  p {
    min-width: 0;
    margin: 0;
    line-height: 1.7;
    overflow-wrap: anywhere;
  }

  @media (max-width: 640px) {
    .timeline > li {
      grid-template-columns: 1fr;
      gap: 8px;
    }
  }
</style>
