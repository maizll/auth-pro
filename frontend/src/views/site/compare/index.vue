<!-- 免费版和商业版对照。差别行复用 commercialCompareRows，不另写一份。 -->
<template>
  <PublicSiteShell>
    <p class="kicker">系统对比</p>
    <h1>免费版和商业版</h1>
    <p class="lead">{{ commercialCompareNote }}</p>
    <div class="table-wrap">
      <table class="compare-table">
        <thead>
          <tr>
            <th>项目</th>
            <th>免费版</th>
            <th>商业版</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in commercialCompareRows" :key="row.label">
            <td :title="row.label">{{ row.label }}</td>
            <td :title="row.free">{{ row.free }}</td>
            <td :title="row.commercial">{{ row.commercial }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <section class="shared">
      <h2>两个版本都具备</h2>
      <p v-if="!shared.length" class="muted">暂无补充说明。</p>
      <ul v-else>
        <li v-for="item in shared" :key="item.id" :title="item.body">{{ item.body }}</li>
      </ul>
    </section>
  </PublicSiteShell>
</template>

<script setup lang="ts">
  import axios from 'axios'
  import { onMounted, ref } from 'vue'
  import PublicSiteShell from '@/components/site/PublicSiteShell.vue'
  import { commercialCompareNote, commercialCompareRows } from '@/utils/commercial'

  defineOptions({ name: 'SiteCompare' })

  const shared = ref<{ id: number; body: string }[]>([])

  onMounted(async () => {
    try {
      const { data } = await axios.get('/api/v1/site/compare', { timeout: 8000 })
      if (data?.code === 200 && Array.isArray(data.data?.shared)) {
        shared.value = data.data.shared
      }
    } catch {
      shared.value = []
    }
  })
</script>

<style scoped>
  .kicker {
    margin: 0 0 8px;
    color: var(--remote-primary, #2f6fed);
    font-weight: 700;
  }

  h1,
  h2 {
    margin: 0;
  }

  .lead,
  .muted {
    color: rgb(28 39 64 / 72%);
    line-height: 1.7;
  }

  .table-wrap,
  .shared {
    margin-top: 20px;
    padding: 8px 8px 12px;
    overflow: hidden;
    background: #fff;
    border: 1px solid rgb(47 111 237 / 10%);
    border-radius: 16px;
  }

  .compare-table {
    width: 100%;
    table-layout: fixed;
    border-collapse: collapse;
  }

  .compare-table th,
  .compare-table td {
    padding: 12px 8px;
    overflow: hidden;
    font-size: 13px;
    text-align: left;
    white-space: nowrap;
    text-overflow: ellipsis;
    border-bottom: 1px solid rgb(28 39 64 / 8%);
  }

  .compare-table th {
    color: var(--remote-primary, #2f6fed);
  }

  .shared {
    padding: 22px;
  }

  .shared ul {
    margin: 12px 0 0;
    padding-left: 18px;
  }

  .shared li {
    overflow: hidden;
    line-height: 1.8;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
</style>
