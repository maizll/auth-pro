<!-- 授权购买公开页。只展示套餐和购买步骤，不在官网下单。 -->
<template>
  <PublicSiteShell>
    <section class="buy-hero">
      <p class="kicker">授权购买</p>
      <h1>选择免费版或商业版</h1>
      <p class="lead"
        >免费版可以直接下载。商业版请先在本站注册，再到自己的后台绑定账号并扫码支付。</p
      >
    </section>

    <div v-if="loading" class="state">正在读取套餐…</div>
    <div v-else class="plan-grid">
      <article class="plan-card">
        <h2>免费版</h2>
        <p class="price">{{ freePrice }}</p>
        <ul class="benefits">
          <li v-for="row in commercialCompareRows" :key="row.label">
            <span class="tick" aria-hidden="true">✓</span>
            <span>{{ row.label }}：{{ row.free }}</span>
          </li>
        </ul>
        <a class="plan-link" :href="downloadUrl" target="_blank" rel="noopener noreferrer">
          下载最新版本
        </a>
      </article>
      <article
        v-for="(plan, index) in plans"
        :key="plan.id"
        class="plan-card plan-card--commercial"
      >
        <span v-if="index === 0" class="recommend">推荐</span>
        <h2>{{ plan.name }}</h2>
        <p class="price">
          {{ yuan(plan.priceCents)
          }}<span v-if="periodSuffix(plan.period)" class="unit">{{
            periodSuffix(plan.period)
          }}</span>
        </p>
        <ul class="benefits">
          <li v-for="row in commercialCompareRows" :key="row.label">
            <span class="tick" aria-hidden="true">✓</span>
            <span>{{ row.label }}：{{ row.commercial }}</span>
          </li>
        </ul>
      </article>
      <article v-if="!plans.length" class="plan-card plan-card--commercial">
        <span class="recommend">推荐</span>
        <h2>商业版</h2>
        <p class="plan-note">暂未开放购买</p>
      </article>
    </div>

    <section class="steps">
      <h2>购买商业版</h2>
      <ol class="step-bar">
        <li>
          <span class="num">1</span>
          <div>
            <strong>注册或登录</strong>
            <span>在本站打开登录注册窗口，使用这里的账号。</span>
            <button type="button" @click="register">打开登录 / 注册</button>
          </div>
        </li>
        <li>
          <span class="num">2</span>
          <div>
            <strong>绑定本站账号</strong>
            <span>到自己的 auth-pro 后台，点顶栏「升级商业版」，绑定刚才的账号。</span>
          </div>
        </li>
        <li>
          <span class="num">3</span>
          <div>
            <strong>扫码支付</strong>
            <span>绑定完成后在购买窗口选择套餐并付款。这一步不在官网进行。</span>
          </div>
        </li>
      </ol>
    </section>

    <section class="rules">
      <h2>域名规则</h2>
      <ol>
        <li v-for="rule in displayRules" :key="rule">{{ rule }}</li>
      </ol>
    </section>
  </PublicSiteShell>
</template>

<script setup lang="ts">
  import axios from 'axios'
  import { computed, onMounted, ref } from 'vue'
  import { useRouter } from 'vue-router'
  import PublicSiteShell from '@/components/site/PublicSiteShell.vue'
  import {
    commercialCompareRows,
    commercialPeriodText,
    commercialYuanText
  } from '@/utils/commercial'
  import { openSiteAuth } from '@/utils/public-site'

  defineOptions({ name: 'SitePurchase' })

  interface SalePlan {
    id: number
    name: string
    period?: string
    priceCents: number
    free_site_changes: number
    site_change_price: number | string | null
  }

  const router = useRouter()
  const loading = ref(true)
  const plans = ref<SalePlan[]>([])
  const rules = ref<string[]>([])
  const downloadUrl = ref('')
  const freeCents = ref(0)
  const freePrice = computed(() => commercialYuanText(freeCents.value))
  const displayRules = computed(() => [...rules.value, ...planRules(plans.value)])

  function yuan(cents: number) {
    return commercialYuanText(cents)
  }

  function periodSuffix(period?: string) {
    const text = commercialPeriodText(period)
    if (text === '一年') return '/ 年'
    if (text === '永久') return '/ 永久'
    if (text.endsWith('天')) return `/ ${text}`
    return ''
  }

  function money(value: number) {
    if (Number.isInteger(value)) return `¥${value}`
    return `¥${value.toFixed(2)}`
  }

  function planRules(list: SalePlan[]) {
    const many = list.length > 1
    return list
      .map((plan) => {
        const parts: string[] = []
        const free = Number(plan.free_site_changes)
        if (Number.isFinite(free) && free < 0) parts.push('含不限次数免费更换')
        else if (Number.isFinite(free) && free > 0) parts.push(`含 ${free} 次免费更换`)
        const raw = plan.site_change_price
        const price = raw == null || raw === '' ? Number.NaN : Number(raw)
        if (Number.isFinite(price)) parts.push(`超出后每次 ${money(price)}`)
        if (!parts.length) return ''
        const sentence = `${parts.join('，')}。`
        if (many) return `${plan.name}：${sentence}`
        if (parts[0].startsWith('含')) return `套餐${sentence}`
        return sentence
      })
      .filter((line) => line.length > 0)
  }

  function register() {
    openSiteAuth('register', router)
  }

  onMounted(async () => {
    try {
      const [info, edition] = await Promise.all([
        axios.get('/api/v1/site/purchase-info', { timeout: 8000 }),
        axios.get('/api/v1/store/edition-plans', { timeout: 8000 })
      ])
      if (info.data?.code === 200) {
        downloadUrl.value = String(info.data.data?.freeDownloadUrl || '')
        freeCents.value = Number(info.data.data?.freePriceCents || 0)
        rules.value = Array.isArray(info.data.data?.rules) ? info.data.data.rules : []
      }
      if (edition.data?.code === 200 && Array.isArray(edition.data.data?.list)) {
        plans.value = edition.data.data.list
      }
    } finally {
      loading.value = false
    }
  })
</script>

<style scoped>
  .kicker {
    margin: 0 0 8px;
    color: var(--remote-primary, #2f6fed);
    font-size: 13px;
    font-weight: 700;
  }

  h1,
  h2 {
    margin: 0;
    font-weight: 700;
  }

  h1 {
    font-size: 32px;
    line-height: 1.25;
  }

  .lead,
  .plan-note,
  .steps span,
  .rules li,
  .benefits span {
    color: rgb(28 39 64 / 72%);
    line-height: 1.7;
  }

  .plan-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 260px), 1fr));
    gap: 16px;
    margin-top: 28px;
  }

  .plan-card,
  .steps,
  .rules {
    min-width: 0;
    padding: 22px;
    background: #fff;
    border: 1px solid rgb(47 111 237 / 10%);
    border-radius: 16px;
  }

  .plan-card--commercial {
    position: relative;
    border: 2px solid var(--remote-primary, #2f6fed);
    box-shadow: 0 12px 32px rgb(47 111 237 / 10%);
  }

  .recommend {
    position: absolute;
    top: 14px;
    right: 14px;
    padding: 2px 8px;
    color: #fff;
    font-size: 12px;
    line-height: 20px;
    white-space: nowrap;
    background: var(--remote-primary, #2f6fed);
    border-radius: 999px;
  }

  .price {
    display: flex;
    align-items: baseline;
    gap: 6px;
    margin: 12px 0 4px;
    color: var(--remote-primary, #2f6fed);
    font-size: 32px;
    font-weight: 700;
    white-space: nowrap;
  }

  .unit {
    color: rgb(28 39 64 / 62%);
    font-size: 16px;
    font-weight: 600;
  }

  .benefits {
    margin: 14px 0 0;
    padding: 0;
    list-style: none;
  }

  .benefits li {
    display: flex;
    gap: 8px;
    align-items: flex-start;
    min-width: 0;
    margin-bottom: 8px;
  }

  .tick {
    flex: 0 0 auto;
    color: var(--remote-primary, #2f6fed);
    font-weight: 700;
    line-height: 1.7;
  }

  .benefits span:last-child {
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .plan-link,
  .steps button {
    display: inline-flex;
    margin-top: 12px;
    padding: 8px 14px;
    color: #fff;
    white-space: nowrap;
    text-decoration: none;
    background: var(--remote-primary, #2f6fed);
    border: 0;
    border-radius: 8px;
    cursor: pointer;
  }

  .steps,
  .rules {
    margin-top: 20px;
  }

  .step-bar {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 12px;
    margin: 16px 0 0;
    padding: 0;
    list-style: none;
  }

  .step-bar li {
    display: flex;
    gap: 12px;
    min-width: 0;
    padding: 16px;
    background: #f7f9ff;
    border-radius: 14px;
  }

  .num {
    display: inline-flex;
    flex: 0 0 auto;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    color: #fff;
    font-weight: 700;
    background: var(--remote-primary, #2f6fed);
    border-radius: 50%;
  }

  .steps strong,
  .steps span {
    display: block;
  }

  .rules ol {
    margin: 12px 0 0;
    padding-left: 1.4em;
  }

  .rules li {
    white-space: normal;
    overflow-wrap: anywhere;
  }

  @media (max-width: 800px) {
    .step-bar {
      grid-template-columns: 1fr;
    }

    h1 {
      font-size: 28px;
    }
  }
</style>
