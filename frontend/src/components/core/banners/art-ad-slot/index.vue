<!-- 固定槽广告：有投放显示创意；槽位开启且空（卡片招租位）显示虚线招租；关闭/不可达塌掉 -->
<template>
  <div v-if="visible" class="ad-slot" :class="slotClass" :style="slotStyle">
    <template v-if="recruiting">
      <div class="ad-slot__bar">
        <span class="ad-slot__chip">广告位招租</span>
        <span class="ad-slot__grow" />
        <button class="ad-slot__close" type="button" aria-label="关闭广告" @click="close">
          <ArtSvgIcon icon="ri:close-line" />
        </button>
      </div>
      <button type="button" class="ad-vacant" @click="goPromote">
        <span class="ad-vacant__title">
          <span class="ad-vacant__ico" aria-hidden="true">+</span>
          {{ vacantTitle }}
        </span>
        <small class="ad-vacant__desc">{{ vacantDesc }}</small>
        <span class="ad-vacant__cta">我要投放 →</span>
      </button>
    </template>
    <template v-else>
      <div class="ad-slot__bar">
        <span class="ad-slot__chip">广告</span>
        <button type="button" class="ad-slot__entry" @click.stop="goPromote">我要投放</button>
        <span class="ad-slot__grow" />
        <button class="ad-slot__close" type="button" aria-label="关闭广告" @click="close">
          <ArtSvgIcon icon="ri:close-line" />
        </button>
      </div>
      <ArtAdCreative v-if="items.length === 1" :item="items[0]" :fit="fit" :show-info="showInfo" />
    </template>
  </div>
</template>

<script setup lang="ts">
  import { computed, ref, onMounted } from 'vue'
  import { useRouter } from 'vue-router'
  import type { AdPosition } from '@/api/advertisement'
  import { AD_SLOT_PRICE_YUAN } from '@/api/advertisement'
  import { useAdvertisement } from '@/hooks'
  import { useSystemConfigStore } from '@/store/modules/system-config'
  import { claimAdSlot, releaseAdSlot } from '@/utils/ads/screen-limit'

  defineOptions({ name: 'ArtAdSlot' })

  const props = withDefaults(
    defineProps<{
      position: AdPosition
      height?: string
      fit?: 'cover' | 'contain'
      /** 关闭记忆天数：登录/锁屏 30，其余 7 */
      closeDays?: number
    }>(),
    { height: '120px', fit: 'cover', closeDays: 7 }
  )

  const router = useRouter()
  const isOfficialSite = computed(() => useSystemConfigStore().official === true)
  const closedKey = `auth-pro:ad-closed:${props.position}`
  const closed = ref(false)

  onMounted(() => {
    try {
      const raw = localStorage.getItem(closedKey)
      if (!raw) return
      const until = Number(raw)
      if (Number.isFinite(until) && until > Date.now()) closed.value = true
      else localStorage.removeItem(closedKey)
    } catch {
      /* ignore */
    }
  })

  const showInfo = computed(
    () =>
      props.position === 'sidebar' ||
      props.position === 'console-sidebar' ||
      props.position === 'console-home'
  )

  const { items, loading, unreachable, recruiting, placeholder } = useAdvertisement(props.position)

  const allowed = ref(false)
  onMounted(() => {
    if (isOfficialSite.value) return
    allowed.value = claimAdSlot(props.position)
  })

  const visible = computed(
    () =>
      !isOfficialSite.value &&
      !closed.value &&
      allowed.value &&
      !loading.value &&
      !unreachable.value &&
      (recruiting.value || items.value.length > 0)
  )

  const slotClass = computed(() => [
    `ad-slot--${props.position}`,
    { 'ad-slot--vacant': recruiting.value }
  ])

  const slotStyle = computed(() => {
    if (props.position === 'console-topbar' || props.position === 'list-footer') {
      return { height: 'auto', minHeight: '28px' }
    }
    return { height: props.height }
  })

  const vacantTitle = computed(() => '在这里推广你的网站')
  const vacantDesc = computed(() => {
    const yuan = AD_SLOT_PRICE_YUAN[props.position]
    if (yuan != null) return `展示给其他站长，¥${yuan}/天起，自助下单`
    return placeholder.value.description || '展示给其他站长，自助下单'
  })

  const goPromote = () => {
    router.push({ path: '/ads-promote', query: { slot: props.position } }).catch(() => {
      /* 未登录等路由守卫会自行处理 */
    })
  }

  const close = () => {
    closed.value = true
    releaseAdSlot(props.position)
    const days = props.closeDays ?? 7
    try {
      localStorage.setItem(closedKey, String(Date.now() + days * 86400000))
    } catch {
      /* ignore */
    }
  }
</script>

<style lang="scss" scoped>
  .ad-slot {
    position: relative;
    width: 100%;
    overflow: hidden;
    border-radius: 10px;
    border: 1px solid var(--el-border-color-lighter, #e8ebf2);
    background: var(--el-bg-color, #fff);
  }

  .ad-slot--vacant {
    border-style: dashed;
    border-color: #c9d3ee;
    background: #fafbff;
  }

  .ad-slot__bar {
    position: absolute;
    top: 6px;
    left: 8px;
    right: 6px;
    z-index: 2;
    display: flex;
    align-items: center;
    pointer-events: none;
  }

  .ad-slot__chip {
    display: inline-flex;
    align-items: center;
    height: 18px;
    padding: 0 6px;
    font-size: 11px;
    color: #7a818e;
    background: #f1f3f7;
    border: 1px solid #e3e6ed;
    border-radius: 4px;
  }

  .ad-slot__entry {
    pointer-events: auto;
    margin-left: 8px;
    padding: 0;
    font-size: 12px;
    color: #a0a6b1;
    cursor: pointer;
    background: transparent;
    border: none;

    &:hover {
      color: var(--el-color-primary, #5d87ff);
    }
  }

  .ad-slot__grow {
    flex: 1;
  }

  .ad-slot__close {
    pointer-events: auto;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 22px;
    color: #9aa1ad;
    cursor: pointer;
    background: transparent;
    border: none;
    border-radius: 6px;

    &:hover {
      color: #5c6370;
      background: rgb(0 0 0 / 6%);
    }
  }

  .ad-vacant {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    justify-content: center;
    gap: 6px;
    width: 100%;
    height: 100%;
    padding: 28px 14px 12px;
    text-align: left;
    cursor: pointer;
    background: transparent;
    border: none;
    color: inherit;
  }

  .ad-vacant__title {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    font-weight: 600;
    color: var(--el-text-color-primary, #303133);
  }

  .ad-vacant__ico {
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    font-size: 16px;
    line-height: 1;
    color: var(--el-color-primary, #5d87ff);
    background: var(--el-color-primary-light-9, #ecf2ff);
    border: 1px dashed var(--el-color-primary-light-5, #a8c0ff);
    border-radius: 7px;
  }

  .ad-vacant__desc {
    font-size: 12px;
    color: #8a919f;
    line-height: 1.45;
  }

  .ad-vacant__cta {
    margin-top: 2px;
    font-size: 12px;
    color: var(--el-color-primary, #5d87ff);
  }

  .ad-slot--console-topbar,
  .ad-slot--list-footer {
    border: none;
    background: #f1f3f7;
    border-radius: 14px;
  }
</style>
