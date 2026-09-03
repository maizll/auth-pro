<!-- 弹出广告：每个会话只打扰一次，没有投放内容时不弹 -->
<template>
  <ElDialog
    v-model="visible"
    :width="width"
    :show-close="false"
    align-center
    append-to-body
    destroy-on-close
    class="ad-popup-dialog"
  >
    <div v-if="items.length > 0" class="ad-popup-body" :style="{ height }">
      <ArtAdCreative :item="items[0]" fit="contain" />
      <button class="ad-popup-close" type="button" aria-label="关闭广告" @click="visible = false">
        <ArtSvgIcon icon="ri:close-line" />
      </button>
    </div>
  </ElDialog>
</template>

<script setup lang="ts">
  import { ref, watch } from 'vue'
  import type { AdPosition } from '@/api/advertisement'
  import { useAdvertisement } from '@/hooks'

  defineOptions({ name: 'ArtAdPopup' })

  const props = withDefaults(
    defineProps<{
      position?: AdPosition
      width?: string
      height?: string
    }>(),
    { position: 'popup', width: '420px', height: '315px' }
  )

  /** 三端共用同一个键：一个会话里在后台和代理端之间切换也不会重复弹 */
  const SHOWN_KEY = 'auth-pro:ad-popup-shown'

  const visible = ref(false)
  const { items, loading, isPlaceholderOnly } = useAdvertisement(props.position)

  watch(loading, (isLoading) => {
    // 只有占位内容时不弹——给用户看一个「广告位出租」毫无意义
    if (isLoading || isPlaceholderOnly.value) return
    if (sessionStorage.getItem(SHOWN_KEY)) return
    sessionStorage.setItem(SHOWN_KEY, '1')
    visible.value = true
  })
</script>

<style lang="scss" scoped>
  .ad-popup-body {
    position: relative;
    width: 100%;
    overflow: hidden;
  }

  .ad-popup-close {
    position: absolute;
    top: 8px;
    right: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    font-size: 15px;
    color: #fff;
    cursor: pointer;
    background: rgb(0 0 0 / 45%);
    border: none;
    border-radius: 50%;
    transition: background-color 0.2s ease;

    &:hover {
      background: rgb(0 0 0 / 65%);
    }
  }
</style>

<!-- 弹窗被 teleport 到 body，且全局样式用 !important 固定了 body 内边距，只能用非 scoped 覆盖 -->
<style lang="scss">
  .ad-popup-dialog {
    .el-dialog__header {
      display: none;
    }

    .el-dialog__body {
      padding: 0 !important;
    }
  }
</style>
