<!-- 单条广告创意：真实 a 标签包一张投放图，无图或图片失效时退回「广告位出租」占位 -->
<template>
  <component
    :is="link.is"
    v-bind="link.props"
    class="ad-creative"
    :class="{ 'is-clickable': clickable }"
  >
    <img
      v-if="showImage"
      :src="item.imageUrl"
      :alt="item.title"
      :style="{ objectFit: fit }"
      loading="lazy"
      @error="imageFailed = true"
    />
    <div v-else class="ad-vacancy">
      <span class="ad-vacancy-title">广告位出租</span>
      <small class="ad-vacancy-desc">{{ item.description || '虚位以待，欢迎联系投放' }}</small>
    </div>
  </component>
</template>

<script setup lang="ts">
  import { computed, ref, watch } from 'vue'
  import type { AdvertisementItem } from '@/api/advertisement'
  import { usePromotion } from '@/hooks'

  defineOptions({ name: 'ArtAdCreative' })

  const props = withDefaults(
    defineProps<{
      item: AdvertisementItem
      /** 横幅铺满用 cover，弹窗要看全图用 contain */
      fit?: 'cover' | 'contain'
    }>(),
    { fit: 'cover' }
  )

  const { resolvePromotionLink } = usePromotion()

  /** 投放图是带签名的临时地址，过期后不能留一个破图，直接退回占位 */
  const imageFailed = ref(false)
  watch(
    () => props.item.imageUrl,
    () => {
      imageFailed.value = false
    }
  )

  const showImage = computed(() => !!props.item.imageUrl && !imageFailed.value)
  const clickable = computed(() => showImage.value && !!props.item.destinationUrl)
  const link = computed(() =>
    resolvePromotionLink(clickable.value ? props.item.destinationUrl : '')
  )
</script>

<style lang="scss" scoped>
  .ad-creative {
    display: block;
    width: 100%;
    height: 100%;
    overflow: hidden;
    color: inherit;
    text-decoration: none;
    border-radius: 10px;

    &.is-clickable {
      cursor: pointer;

      &:hover img {
        transform: scale(1.02);
      }
    }

    img {
      display: block;
      width: 100%;
      height: 100%;
      transition: transform 0.3s ease;
    }
  }

  /* 占位块用 DOM 绘制而非固定尺寸图片：横幅宽扁、侧边栏窄条、弹窗近方形，一张图三处必然变形 */
  .ad-vacancy {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 6px;
    width: 100%;
    height: 100%;
    padding: 12px;
    text-align: center;
    background: var(--el-fill-color-lighter);
    border: 1px dashed var(--el-border-color);
    border-radius: 10px;
  }

  .ad-vacancy-title {
    font-size: 15px;
    font-weight: 600;
    letter-spacing: 2px;
    color: var(--el-text-color-secondary);
  }

  .ad-vacancy-desc {
    font-size: 12px;
    color: var(--el-text-color-placeholder);
  }
</style>
