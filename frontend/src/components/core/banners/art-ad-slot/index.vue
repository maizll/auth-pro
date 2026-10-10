<!-- 固定槽广告：不轮播、可关闭、空位塌掉、官网不可达则隐藏 -->
<template>
  <div v-if="visible" class="ad-slot" :class="[`ad-slot--${position}`]" :style="slotStyle">
    <div class="ad-slot__bar">
      <span class="ad-slot__chip">广告</span>
      <span class="ad-slot__grow" />
      <button class="ad-slot__close" type="button" aria-label="关闭广告" @click="close">
        <ArtSvgIcon icon="ri:close-line" />
      </button>
    </div>
    <ArtAdCreative v-if="items.length === 1" :item="items[0]" :fit="fit" :show-info="showInfo" />
  </div>
</template>

<script setup lang="ts">
  import { computed, ref, onMounted } from 'vue'
  import type { AdPosition } from '@/api/advertisement'
  import { useAdvertisement } from '@/hooks'
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

  const { items, loading, isPlaceholderOnly, unreachable } = useAdvertisement(props.position)

  const allowed = ref(false)
  onMounted(() => {
    allowed.value = claimAdSlot(props.position)
  })

  const visible = computed(
    () =>
      !closed.value &&
      allowed.value &&
      !loading.value &&
      !unreachable.value &&
      !isPlaceholderOnly.value &&
      items.value.length > 0
  )

  const slotStyle = computed(() => {
    if (props.position === 'console-topbar' || props.position === 'list-footer') {
      return { height: 'auto', minHeight: '28px' }
    }
    return { height: props.height }
  })

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

  .ad-slot--console-topbar,
  .ad-slot--list-footer {
    border: none;
    background: #f1f3f7;
    border-radius: 14px;
  }
</style>
