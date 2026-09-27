<!-- 授权购买公开页。只展示套餐和购买步骤，不在官网下单。 -->
<template>
  <PublicSiteShell>
    <section class="buy-hero">
      <p class="kicker">授权购买</p>
      <h1>选择免费版或商业版</h1>
      <p class="lead">价格来自套餐接口。官网不能直接购买商业版，付款在你自己的后台完成。</p>
    </section>

    <div v-if="loading" class="state">正在读取套餐…</div>
    <div v-else class="plan-grid">
      <article class="plan-card">
        <h2>免费版</h2>
        <p class="price">{{ freePrice }}</p>
        <p class="plan-note">可下载当前发布的安装包</p>
        <a class="plan-link" :href="downloadUrl" target="_blank" rel="noopener noreferrer">
          下载最新版本
        </a>
      </article>
      <article v-for="plan in plans" :key="plan.id" class="plan-card plan-card--commercial">
        <h2>{{ plan.name }}</h2>
        <p class="price">{{ yuan(plan.priceCents) }}</p>
        <p class="plan-note">{{ periodText(plan.period) }}</p>
        <ul>
          <li>{{ changeText(plan) }}</li>
          <li>{{ changePriceText(plan) }}</li>
        </ul>
      </article>
      <article v-if="!plans.length" class="plan-card">
        <h2>商业版</h2>
        <p class="plan-note">当前没有在售套餐。价格在套餐管理里配置后会出现在这里。</p>
      </article>
    </div>

    <section class="steps">
      <h2>购买商业版</h2>
      <ol>
        <li>
          <strong>注册或登录</strong>
          <span>在本站打开登录注册窗口，使用这里的账号。</span>
          <button type="button" @click="register">打开登录 / 注册</button>
        </li>
        <li>
          <strong>绑定本站账号</strong>
          <span>到自己的 auth-pro 后台，点顶栏「升级商业版」，绑定刚才的账号。</span>
        </li>
        <li>
          <strong>扫码支付</strong>
          <span>绑定完成后在购买窗口选择套餐并付款。这一步不在官网进行。</span>
        </li>
      </ol>
    </section>

    <section class="rules">
      <h2>域名规则</h2>
      <ul>
        <li v-for="rule in rules" :key="rule">{{ rule }}</li>
      </ul>
    </section>
  </PublicSiteShell>
</template>

<script setup lang="ts">
  import axios from 'axios'
  import { computed, onMounted, ref } from 'vue'
  import { useRouter } from 'vue-router'
  import PublicSiteShell from '@/components/site/PublicSiteShell.vue'
  import { commercialPeriodText, commercialYuanText } from '@/utils/commercial'
  import { openSiteAuth } from '@/utils/public-site'

  defineOptions({ name: 'SitePurchase' })

  interface SalePlan {
    id: number
    name: string
    period?: string
    priceCents: number
    free_site_changes: number
    site_change_price: number | null
  }

  const router = useRouter()
  const loading = ref(true)
  const plans = ref<SalePlan[]>([])
  const rules = ref<string[]>([])
  const downloadUrl = ref('')
  const freeCents = ref(0)
  const freePrice = computed(() => commercialYuanText(freeCents.value))

  function yuan(cents: number) {
    return commercialYuanText(cents)
  }

  function periodText(period?: string) {
    return commercialPeriodText(period) || '按套餐时长'
  }

  function changeText(plan: SalePlan) {
    if (plan.free_site_changes < 0) return '免费更换次数不限'
    if (plan.free_site_changes === 0) return '不含免费更换'
    return `含 ${plan.free_site_changes} 次免费更换`
  }

  function changePriceText(plan: SalePlan) {
    if (plan.site_change_price == null) return '超出后不能付费更换'
    const price = Number(plan.site_change_price)
    if (!Number.isFinite(price)) return '超出后不能付费更换'
    return `超出后每次 ${price.toFixed(2)} 元`
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
  .rules li {
    color: rgb(28 39 64 / 72%);
    line-height: 1.7;
  }

  .plan-grid,
  .steps ol {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 260px), 1fr));
    gap: 16px;
  }

  .plan-grid {
    margin-top: 28px;
  }

  .plan-card,
  .steps,
  .rules {
    padding: 22px;
    background: #fff;
    border: 1px solid rgb(47 111 237 / 10%);
    border-radius: 16px;
  }

  .price {
    margin: 12px 0 4px;
    color: var(--remote-primary, #2f6fed);
    font-size: 28px;
    font-weight: 700;
    white-space: nowrap;
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

  .plan-card ul,
  .rules ul {
    margin: 12px 0 0;
    padding-left: 18px;
  }

  .steps,
  .rules {
    margin-top: 20px;
  }

  .steps ol {
    margin: 16px 0 0;
    padding: 0;
    list-style: none;
  }

  .steps li {
    min-width: 0;
  }

  .steps strong,
  .steps span {
    display: block;
  }

  .steps strong {
    white-space: nowrap;
  }
</style>
