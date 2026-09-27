<!-- 顶栏商业版入口。形态按宝塔首页右上角的胶囊：浅色底是未开通，实心主题蓝是已开通。 -->
<template>
  <div class="commercial-header-entry">
    <button
      v-if="ready && pending"
      type="button"
      class="commercial-header-entry__hit"
      aria-label="商业版 · 待校验"
      @click="openCommercialLicense"
    >
      <span class="commercial-header-entry__pill commercial-header-entry__pill--pending">
        <ArtSvgIcon icon="ri:error-warning-line" />
        <span>商业版</span>
        <span>待校验</span>
      </span>
    </button>
    <button
      v-else-if="ready && commercial"
      type="button"
      class="commercial-header-entry__hit"
      :aria-label="`商业版 ${term}`"
      @click="openCommercialLicense"
    >
      <span class="commercial-header-entry__pill commercial-header-entry__pill--on">
        <ArtSvgIcon icon="ri:vip-crown-2-fill" />
        <span>商业版</span>
      </span>
      <span class="commercial-header-entry__expire">{{ term }}</span>
    </button>
    <button
      v-else-if="ready"
      type="button"
      class="commercial-header-entry__hit"
      aria-label="免费版，升级商业版"
      @click="openCommercialUpgrade"
    >
      <span class="commercial-header-entry__pill commercial-header-entry__pill--free">
        <ArtSvgIcon icon="ri:vip-diamond-line" />
        <span>免费版</span>
      </span>
    </button>
  </div>
</template>

<script setup lang="ts">
  import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
  import { fetchStoreAccount, refreshStoreSnapshot, type StoreAccount } from '@/api/store'
  import {
    commercialUi,
    isCommercialActive,
    openCommercialLicense,
    openCommercialUpgrade,
    rememberCommercialAccount
  } from '@/utils/commercial'

  defineOptions({ name: 'CommercialHeaderButton' })

  const account = ref<StoreAccount | null>(commercialUi.account)
  const ready = ref(!!commercialUi.account)
  const commercial = computed(() => isCommercialActive(account.value))
  const pending = computed(() => commercial.value && !!account.value?.offlineGrace)
  const term = computed(() => headerTerm(account.value))

  function headerTerm(current: StoreAccount | null) {
    if (!current) return ''
    if (current.permanent) return '永久授权'
    if (!current.editionExpireAt) return '未设置到期时间'
    const date = new Date(current.editionExpireAt * 1000)
    if (Number.isNaN(date.getTime())) return '未设置到期时间'
    const pad = (value: number) => String(value).padStart(2, '0')
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} 到期`
  }

  watch(
    () => commercialUi.account,
    (next) => {
      account.value = next
      if (next) ready.value = true
    }
  )

  async function load(pull: boolean) {
    try {
      const next = pull
        ? await refreshStoreSnapshot().catch(() => fetchStoreAccount())
        : await fetchStoreAccount()
      account.value = next
      rememberCommercialAccount(next)
    } catch {
      if (!account.value) account.value = null
    } finally {
      ready.value = true
    }
  }

  function onAccountRefresh() {
    void load(false)
  }

  onMounted(() => {
    void load(true)
    window.addEventListener('store-account-refresh', onAccountRefresh)
  })

  onBeforeUnmount(() => {
    window.removeEventListener('store-account-refresh', onAccountRefresh)
  })
</script>

<style scoped>
  .commercial-header-entry {
    display: inline-flex;
    flex: none;
    align-items: center;
  }

  .commercial-header-entry__hit {
    display: inline-flex;
    gap: 8px;
    align-items: center;
    min-height: 36px;
    padding: 0;
    font: inherit;
    color: inherit;
    white-space: nowrap;
    cursor: pointer;
    background: transparent;
    border: 0;
  }

  /* 宝塔 10 首页右上角是全圆角小胶囊：高约 26px，字号 12px，图标在左。 */
  .commercial-header-entry__pill {
    display: inline-flex;
    gap: 4px;
    align-items: center;
    height: 26px;
    padding: 0 10px;
    font-size: 12px;
    font-weight: 600;
    line-height: 1;
    border-radius: 999px;
  }

  .commercial-header-entry__pill--free {
    color: var(--el-color-primary);
    background: var(--el-color-primary-light-9);
    border: 1px solid var(--el-color-primary-light-5);
  }

  .commercial-header-entry__hit:hover .commercial-header-entry__pill--free {
    background: var(--el-color-primary-light-8);
  }

  .commercial-header-entry__pill--on {
    color: #fff;
    background: var(--el-color-primary);
    border: 1px solid var(--el-color-primary);
  }

  .commercial-header-entry__hit:hover .commercial-header-entry__pill--on {
    background: var(--el-color-primary-dark-2);
    border-color: var(--el-color-primary-dark-2);
  }

  .commercial-header-entry__expire {
    color: var(--el-text-color-secondary);
    font-size: 12px;
    font-weight: 400;
    line-height: 1;
  }

  .commercial-header-entry__pill--pending {
    color: var(--el-color-warning-dark-2);
    background: var(--el-color-warning-light-9);
    border: 1px solid var(--el-color-warning-light-5);
  }

  .commercial-header-entry :deep(.art-svg-icon) {
    font-size: 14px;
  }

  @media (max-width: 767px) {
    .commercial-header-entry__pill {
      height: 28px;
      padding: 0 8px;
    }

    .commercial-header-entry__expire {
      font-size: 11px;
    }
  }
</style>

<style>
  html.dark .commercial-header-entry__pill--free {
    color: var(--el-color-primary-light-3);
    background: color-mix(in srgb, var(--el-color-primary) 18%, transparent);
    border-color: color-mix(in srgb, var(--el-color-primary) 48%, transparent);
  }

  html.dark .commercial-header-entry__pill--pending {
    color: var(--el-color-warning-light-3);
    background: color-mix(in srgb, var(--el-color-warning) 16%, transparent);
    border-color: color-mix(in srgb, var(--el-color-warning) 48%, transparent);
  }
</style>
