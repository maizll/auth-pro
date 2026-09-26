<template>
  <div class="commercial-header-entry">
    <button
      v-if="ready && commercial"
      type="button"
      class="commercial-header-entry__badge"
      @click="openCommercialLicense"
    >
      <ArtSvgIcon icon="ri:vip-crown-fill" />
      <span>商业版</span>
    </button>
    <button
      v-else-if="ready"
      type="button"
      class="commercial-header-entry__upgrade"
      @click="openCommercialUpgrade"
    >
      <ArtSvgIcon icon="ri:vip-crown-fill" />
      <span>升级商业版</span>
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
  .commercial-header-entry__badge {
    display: inline-flex;
    gap: 4px;
    align-items: center;
    height: 32px;
    padding: 0 10px;
    border-radius: 6px;
    font-size: 13px;
    line-height: 1;
    white-space: nowrap;
    cursor: pointer;
  }

  .commercial-header-entry__upgrade {
    color: var(--el-color-warning-dark-2);
    background: var(--el-color-warning-light-9);
    border: 1px solid var(--el-color-warning-light-5);
  }

  .commercial-header-entry__upgrade:hover {
    background: var(--el-color-warning-light-8);
  }

  .commercial-header-entry__badge {
    color: var(--el-color-success-dark-2);
    background: var(--el-color-success-light-9);
    border: 1px solid var(--el-color-success-light-5);
  }

  .commercial-header-entry__badge:hover {
    background: var(--el-color-success-light-8);
  }

  .commercial-header-entry :deep(.art-svg-icon) {
    font-size: 16px;
  }

  @media (max-width: 640px) {
    .commercial-header-entry__upgrade,
    .commercial-header-entry__badge {
      height: 30px;
      padding: 0 8px;
      font-size: 12px;
    }
  }
</style>
