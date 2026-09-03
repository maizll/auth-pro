<!-- 广告位容器：按位置拉取投放内容，多条轮播，无投放时显示占位 -->
<template>
  <div class="ad-slot" :style="{ height }">
    <ElCarousel
      v-if="items.length > 1"
      height="100%"
      indicator-position="none"
      arrow="never"
      :interval="interval"
      autoplay
    >
      <ElCarouselItem v-for="item in items" :key="item.id">
        <ArtAdCreative :item="item" :fit="fit" />
      </ElCarouselItem>
    </ElCarousel>
    <ArtAdCreative v-else-if="items.length === 1" :item="items[0]" :fit="fit" />
  </div>
</template>

<script setup lang="ts">
  import type { AdPosition } from '@/api/advertisement'
  import { useAdvertisement } from '@/hooks'

  defineOptions({ name: 'ArtAdSlot' })

  const props = withDefaults(
    defineProps<{
      position: AdPosition
      /** 必须是确定值：轮播项为绝对定位，靠内容撑不起高度 */
      height?: string
      fit?: 'cover' | 'contain'
      interval?: number
    }>(),
    { height: '120px', fit: 'cover', interval: 6000 }
  )

  const { items } = useAdvertisement(props.position)
</script>

<style lang="scss" scoped>
  .ad-slot {
    width: 100%;
    overflow: hidden;
    border-radius: 10px;

    :deep(.el-carousel),
    :deep(.el-carousel__container) {
      height: 100%;
    }

    :deep(.el-carousel__item) {
      overflow: hidden;
      border-radius: 10px;
    }
  }
</style>
