<!-- 已开通商业版时的授权详情卡，含到期时间和立即刷新。 -->
<template>
  <article
    v-if="account && isCommercialActive(account)"
    class="license-card"
    :class="{ 'is-pending': account.offlineGrace }"
  >
    <header class="license-card__head">
      <ArtSvgIcon :icon="account.offlineGrace ? 'ri:shield-check-line' : 'ri:shield-check-fill'" />
      <div class="license-card__title">
        <h3>{{ account.offlineGrace ? '商业版 · 待校验' : '商业版' }}</h3>
        <p>{{ account.offlineGrace ? '源站暂时连不上，宽限期内可用' : '授权有效' }}</p>
      </div>
      <span class="license-card__pill">{{
        account.permanent || !account.editionExpireAt ? '永久' : '有效期内'
      }}</span>
    </header>
    <dl class="license-card__rows">
      <div v-for="row in rows" :key="row.label" class="license-card__row">
        <dt>{{ row.label }}</dt>
        <dd :title="row.value">{{ row.value }}</dd>
      </div>
    </dl>
    <p v-if="account.offlineGrace" class="license-card__note"
      >源站暂时连不上，商业版处于离线宽限。</p
    >
    <footer class="license-card__foot">
      <span class="license-card__verified">上次校验时间：{{ verifiedText }}</span>
      <ElButton size="small" :loading="refreshing" @click="refreshNow">立即刷新</ElButton>
    </footer>
  </article>
  <div v-else class="license-card__plain">
    <p class="license-card__empty">当前还不是商业版。</p>
    <footer v-if="account" class="license-card__foot">
      <span class="license-card__verified">上次校验时间：{{ verifiedText }}</span>
      <ElButton size="small" :loading="refreshing" @click="refreshNow">立即刷新</ElButton>
    </footer>
  </div>
</template>

<script setup lang="ts">
  import { computed, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import { refreshStoreSnapshot, type StoreAccount } from '@/api/store'
  import {
    commercialExpireText,
    isCommercialActive,
    rememberCommercialAccount
  } from '@/utils/commercial'
  import { showCaughtError } from '@/utils/http/error-toast'

  defineOptions({ name: 'CommercialLicenseCard' })

  const props = defineProps<{ account: StoreAccount | null }>()
  const emit = defineEmits<{ refreshed: [account: StoreAccount] }>()
  const refreshing = ref(false)

  /** 详情行：标签在左、值在右，各占一行；没有的值不显示。 */
  const rows = computed(() => {
    const account = props.account
    if (!account) return []
    return [
      { label: '到期时间', value: commercialExpireText(account) },
      { label: '来源', value: account.editionSource || '' },
      { label: '绑定账号', value: account.account || '' },
      { label: '授权域名', value: account.domain || '' },
      { label: '授权编号', value: account.licenseNo || '' }
    ].filter((row) => row.value)
  })

  const verifiedText = computed(() => {
    const ts = props.account?.verifiedAt || 0
    if (!ts) return '尚未校验'
    const date = new Date(ts * 1000)
    const pad = (value: number) => String(value).padStart(2, '0')
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
  })

  async function refreshNow() {
    refreshing.value = true
    try {
      const next = await refreshStoreSnapshot(true)
      if (next) {
        rememberCommercialAccount(next)
        emit('refreshed', next)
      }
      window.dispatchEvent(new Event('store-account-refresh'))
      ElMessage.success('已刷新')
    } catch (error) {
      showCaughtError(error, '刷新失败')
      window.dispatchEvent(new Event('store-account-refresh'))
    } finally {
      refreshing.value = false
    }
  }
</script>

<style scoped>
  .license-card {
    overflow: hidden;
    border: 1px solid color-mix(in srgb, var(--el-color-primary) 35%, var(--el-border-color));
    border-radius: 12px;
    background:
      radial-gradient(120% 80% at 100% 0%, var(--el-color-primary-light-8), transparent 55%),
      var(--el-bg-color);
  }

  .license-card.is-pending {
    border-color: var(--el-color-primary-light-5);
  }

  .license-card__head {
    display: flex;
    gap: 10px;
    align-items: center;
    padding: 14px 16px 10px;
    color: var(--el-text-color-primary);
  }

  .license-card__title {
    flex: 1 1 auto;
    min-width: 0;
  }

  .license-card__head h3,
  .license-card__head p,
  .license-card__note,
  .license-card__empty {
    margin: 0;
  }

  .license-card__head h3 {
    font-size: 16px;
    line-height: 22px;
    white-space: nowrap;
  }

  .license-card__head p {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .license-card__head p,
  .license-card__note,
  .license-card__empty {
    color: var(--el-text-color-secondary);
    font-size: 13px;
  }

  .license-card__head :deep(.art-svg-icon) {
    flex: none;
    font-size: 26px;
    color: var(--el-color-primary);
  }

  .license-card__pill {
    flex: none;
    margin-left: auto;
    padding: 2px 8px;
    border-radius: 999px;
    background: var(--el-color-primary-light-9);
    color: var(--el-color-primary);
    font-size: 12px;
    line-height: 18px;
    white-space: nowrap;
  }

  .license-card__rows {
    margin: 0;
    padding: 2px 16px 10px;
  }

  .license-card__row {
    display: flex;
    gap: 16px;
    align-items: baseline;
    justify-content: space-between;
    padding: 6px 0;
    border-bottom: 1px dashed var(--el-border-color-lighter);
  }

  .license-card__row:last-child {
    border-bottom: 0;
  }

  .license-card__row dt {
    flex: none;
    color: var(--el-text-color-secondary);
    font-size: 13px;
    white-space: nowrap;
  }

  .license-card__row dd {
    min-width: 0;
    margin: 0;
    overflow: hidden;
    color: var(--el-text-color-primary);
    font-size: 13px;
    text-align: right;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .license-card__note,
  .license-card__empty {
    padding: 0 16px 12px;
  }

  .license-card__foot {
    display: flex;
    gap: 12px;
    align-items: center;
    justify-content: space-between;
    padding: 0 16px 14px;
    color: var(--el-text-color-secondary);
    font-size: 12px;
  }

  .license-card__verified {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .license-card__foot :deep(.el-button) {
    flex: none;
  }

  .license-card__plain .license-card__empty {
    padding-bottom: 8px;
  }

  :global(html.dark) .license-card,
  :global(.dark) .license-card {
    border-color: rgb(96 165 250 / 45%);
    background:
      radial-gradient(120% 80% at 100% 0%, rgb(37 99 235 / 28%), transparent 55%),
      var(--el-bg-color);
  }

  :global(html.dark) .license-card__pill,
  :global(.dark) .license-card__pill {
    background: rgb(37 99 235 / 28%);
    color: var(--el-color-primary-light-3);
  }
</style>
