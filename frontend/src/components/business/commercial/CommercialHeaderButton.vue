<template>
  <div class="commercial-header-entry">
    <button
      v-if="ready && pending"
      type="button"
      class="commercial-header-entry__pending"
      aria-label="商业版 · 待校验"
      @click="openCommercialLicense"
    >
      <ArtSvgIcon icon="ri:shield-check-line" />
      <span class="commercial-header-entry__full">商业版 · 待校验</span>
      <span class="commercial-header-entry__short" aria-hidden="true">待校验</span>
      <i class="commercial-header-entry__dot" aria-hidden="true" />
    </button>
    <button
      v-else-if="ready && commercial"
      type="button"
      class="commercial-header-entry__badge"
      aria-label="商业版 · 已激活"
      @click="openCommercialLicense"
    >
      <ArtSvgIcon icon="ri:shield-check-fill" />
      <span class="commercial-header-entry__full">商业版 · 已激活</span>
      <span class="commercial-header-entry__short" aria-hidden="true">已激活</span>
    </button>
    <button
      v-else-if="ready"
      type="button"
      class="commercial-header-entry__upgrade"
      aria-label="升级商业版"
      @click="openCommercialUpgrade"
    >
      <ArtSvgIcon icon="ri:rocket-2-line" />
      <span class="commercial-header-entry__full">升级商业版</span>
      <span class="commercial-header-entry__short" aria-hidden="true">升级</span>
    </button>
  </div>
</template>

<script setup lang="ts">
  import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
  import { fetchStoreAccount, type StoreAccount } from '@/api/store'
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

  watch(
    () => commercialUi.account,
    (next) => {
      account.value = next
      if (next) ready.value = true
    }
  )

  async function load() {
    try {
      const next = await fetchStoreAccount()
      account.value = next
      rememberCommercialAccount(next)
    } catch {
      if (!account.value) account.value = null
    } finally {
      ready.value = true
    }
  }

  onMounted(() => {
    load()
    window.addEventListener('store-account-refresh', load)
  })

  onBeforeUnmount(() => {
    window.removeEventListener('store-account-refresh', load)
  })
</script>

<style scoped>
  .commercial-header-entry {
    display: inline-flex;
    flex: none;
    align-items: center;
  }

  .commercial-header-entry__upgrade,
  .commercial-header-entry__badge,
  .commercial-header-entry__pending {
    display: inline-flex;
    gap: 6px;
    align-items: center;
    height: 32px;
    padding: 0 12px;
    font-size: 13px;
    font-weight: 600;
    line-height: 1;
    white-space: nowrap;
    cursor: pointer;
  }

  .commercial-header-entry__upgrade {
    color: var(--el-color-primary);
    background: transparent;
    border: 1px solid var(--el-color-primary);
    border-radius: 8px;
  }

  .commercial-header-entry__upgrade:hover {
    background: var(--el-color-primary-light-9);
  }

  .commercial-header-entry__badge {
    color: #f4f7ff;
    background: #1c2744;
    border: 1px solid #1c2744;
    border-radius: 999px;
  }

  .commercial-header-entry__badge:hover {
    background: #273556;
  }

  .commercial-header-entry__pending {
    color: var(--el-color-warning-dark-2);
    background: var(--el-color-warning-light-9);
    border: 1px solid var(--el-color-warning-light-5);
    border-radius: 8px;
  }

  .commercial-header-entry__pending:hover {
    background: var(--el-color-warning-light-8);
  }

  .commercial-header-entry__dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--el-color-warning);
    box-shadow: 0 0 0 3px var(--el-color-warning-light-7);
  }

  .commercial-header-entry__short {
    display: none;
  }

  .commercial-header-entry :deep(.art-svg-icon) {
    font-size: 16px;
  }

  @media (max-width: 640px) {
    .commercial-header-entry__upgrade,
    .commercial-header-entry__badge,
    .commercial-header-entry__pending {
      height: 30px;
      padding: 0 8px;
      font-size: 12px;
    }

    .commercial-header-entry__full {
      display: none;
    }

    .commercial-header-entry__short {
      display: inline;
    }
  }
</style>

<style>
  html.dark .commercial-header-entry__badge {
    color: #eef3ff;
    background: #24345c;
    border-color: #8eabef;
  }

  html.dark .commercial-header-entry__badge:hover {
    background: #2d4070;
  }

  html.dark .commercial-header-entry__pending {
    color: var(--el-color-warning-light-3);
    background: rgb(230 162 60 / 16%);
    border-color: rgb(230 162 60 / 55%);
  }
</style>
