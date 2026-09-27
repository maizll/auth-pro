<!-- 已开通商业版时的授权详情卡，含到期时间和立即刷新。 -->
<template>
  <article
    v-if="account && isCommercialActive(account)"
    class="license-card"
    :class="{ 'is-pending': account.offlineGrace }"
  >
    <header class="license-card__head">
      <ArtSvgIcon :icon="account.offlineGrace ? 'ri:shield-check-line' : 'ri:shield-check-fill'" />
      <div>
        <h3>{{ account.offlineGrace ? '商业版 · 待校验' : '商业版' }}</h3>
        <p>{{
          account.offlineGrace
            ? '源站暂时连不上，宽限期内仍可使用。'
            : '当前授权有效，无需再次升级。'
        }}</p>
      </div>
      <span class="license-card__pill">{{ account.permanent ? '永久授权' : '有效期内' }}</span>
    </header>
    <dl class="license-card__grid">
      <div>
        <dt>到期时间</dt>
        <dd>{{ commercialExpireText(account) }}</dd>
      </div>
      <div v-if="account.account">
        <dt>绑定账号</dt>
        <dd>{{ account.account }}</dd>
      </div>
      <div v-if="account.domain">
        <dt>授权域名</dt>
        <dd>{{ account.domain }}</dd>
      </div>
      <div v-if="account.licenseNo">
        <dt>授权编号</dt>
        <dd>{{ account.licenseNo }}</dd>
      </div>
    </dl>
    <p v-if="account.offlineGrace" class="license-card__note"
      >源站暂时连不上，商业版处于离线宽限。</p
    >
    <footer class="license-card__foot">
      <span>上次校验时间：{{ verifiedText }}</span>
      <ElButton size="small" :loading="refreshing" @click="refreshNow">立即刷新</ElButton>
    </footer>
  </article>
  <div v-else class="license-card__plain">
    <p class="license-card__empty">当前还不是商业版。</p>
    <footer v-if="account" class="license-card__foot">
      <span>上次校验时间：{{ verifiedText }}</span>
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
      const next = await refreshStoreSnapshot()
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
    border-radius: 14px;
    background:
      radial-gradient(120% 80% at 100% 0%, var(--el-color-primary-light-8), transparent 55%),
      var(--el-bg-color);
  }

  .license-card.is-pending {
    border-color: var(--el-color-warning-light-5);
    background:
      radial-gradient(120% 80% at 100% 0%, var(--el-color-warning-light-8), transparent 55%),
      var(--el-bg-color);
  }

  .license-card__head {
    display: flex;
    gap: 10px;
    align-items: center;
    padding: 16px 16px 8px;
    color: var(--el-text-color-primary);
  }

  .license-card__head h3,
  .license-card__head p,
  .license-card__note,
  .license-card__empty {
    margin: 0;
  }

  .license-card__head h3 {
    font-size: 18px;
  }

  .license-card__head p,
  .license-card__note,
  .license-card__empty {
    color: var(--el-text-color-secondary);
    font-size: 13px;
  }

  .license-card__head :deep(.art-svg-icon) {
    font-size: 28px;
    color: var(--el-color-primary);
  }

  .license-card.is-pending .license-card__head :deep(.art-svg-icon) {
    color: var(--el-color-warning);
  }

  .license-card__pill {
    margin-left: auto;
    padding: 2px 8px;
    border-radius: 999px;
    background: var(--el-color-primary-light-9);
    color: var(--el-color-primary);
    font-size: 12px;
  }

  .license-card.is-pending .license-card__pill {
    background: var(--el-color-warning-light-9);
    color: var(--el-color-warning-dark-2);
  }

  .license-card__grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px 16px;
    margin: 0;
    padding: 8px 16px 16px;
  }

  .license-card__grid dt {
    color: var(--el-text-color-secondary);
    font-size: 12px;
  }

  .license-card__grid dd {
    margin: 2px 0 0;
    color: var(--el-text-color-primary);
    font-size: 14px;
    word-break: break-all;
  }

  .license-card__note,
  .license-card__empty {
    padding: 0 16px 16px;
  }

  .license-card__foot {
    display: flex;
    gap: 12px;
    align-items: center;
    justify-content: space-between;
    padding: 0 16px 16px;
    color: var(--el-text-color-secondary);
    font-size: 12px;
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

  :global(html.dark) .license-card.is-pending,
  :global(.dark) .license-card.is-pending {
    border-color: rgb(230 162 60 / 55%);
    background:
      radial-gradient(120% 80% at 100% 0%, rgb(230 162 60 / 18%), transparent 55%),
      var(--el-bg-color);
  }

  :global(html.dark) .license-card__pill,
  :global(.dark) .license-card__pill {
    background: rgb(37 99 235 / 28%);
    color: var(--el-color-primary-light-3);
  }

  :global(html.dark) .license-card.is-pending .license-card__pill,
  :global(.dark) .license-card.is-pending .license-card__pill {
    background: rgb(230 162 60 / 18%);
    color: var(--el-color-warning-light-3);
  }

  @media (max-width: 640px) {
    .license-card__grid {
      grid-template-columns: 1fr;
    }
  }
</style>
