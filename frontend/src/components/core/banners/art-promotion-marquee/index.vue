<!-- 广告位跑马灯：所有投放项首尾相接匀速横向滚动，无分页概念 -->
<template>
  <div class="art-card promotion-marquee" :style="{ height: height }">
    <div class="marquee-header">
      <div>
        <h3>{{ title }}</h3>
        <p v-if="subtitle">{{ subtitle }}</p>
      </div>
      <slot name="extra"></slot>
    </div>

    <div class="marquee-viewport">
      <div class="marquee-track" :style="{ '--repeat': repeat, animationDuration: duration }">
        <component
          :is="card.link.is"
          v-for="card in loopCards"
          :key="card.key"
          v-bind="card.link.props"
          class="marquee-card"
          :class="{ 'is-clickable': !!card.item.linkUrl }"
          :style="{ width: cardWidth }"
        >
          <div class="marquee-cover" :style="coverStyle(card.item)">
            <img v-if="card.item.imageUrl" :src="card.item.imageUrl" :alt="card.item.title" />
            <span v-else>{{ card.item.title.charAt(0) }}</span>
          </div>
          <div class="marquee-body">
            <div class="marquee-heading">
              <p class="marquee-title">{{ card.item.title }}</p>
              <span class="marquee-tag">{{ card.item.tag }}</span>
            </div>
            <small class="marquee-summary">{{ card.item.summary }}</small>
          </div>
        </component>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
  import { computed } from 'vue'
  import type { PromotionItem } from '@/api/promotion'
  import { usePromotion } from '@/hooks'
  import type { PromotionLink } from '@/hooks'

  defineOptions({ name: 'ArtPromotionMarquee' })

  const props = withDefaults(
    defineProps<{
      items: PromotionItem[]
      title?: string
      subtitle?: string
      height?: string
      cardWidth?: string
      /** 滚动一整轮的秒数按投放数量摊算，保证卡片多时不会变快 */
      secondsPerItem?: number
    }>(),
    {
      title: '精选推荐',
      subtitle: '',
      height: '200px',
      cardWidth: '240px',
      secondsPerItem: 5
    }
  )

  const { resolvePromotionLink, coverStyle } = usePromotion()

  /** 单轮内容必须比视口宽，否则滚动到尾部会露白；投放太少时多复制几轮 */
  const MIN_CARDS_PER_ROUND = 8

  const repeat = computed(() => {
    const count = props.items.length
    if (count === 0) return 2
    return Math.max(2, Math.ceil(MIN_CARDS_PER_ROUND / count))
  })

  const loopCards = computed(() => {
    const cards: { key: string; item: PromotionItem; link: PromotionLink }[] = []
    for (let round = 0; round < repeat.value; round++) {
      props.items.forEach((item, index) => {
        cards.push({
          key: `${item.id}-${round}-${index}`,
          item,
          link: resolvePromotionLink(item)
        })
      })
    }
    return cards
  })

  const duration = computed(() => `${Math.max(props.items.length * props.secondsPerItem, 20)}s`)
</script>

<style lang="scss" scoped>
  .promotion-marquee {
    display: flex;
    flex-direction: column;
    width: 100%;
    margin-bottom: 16px;
    padding: 20px;
  }

  .marquee-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 14px;

    h3 {
      margin: 0;
      font-size: 17px;
      font-weight: 700;
    }

    p {
      margin-top: 6px;
      font-size: 13px;
      color: var(--art-gray-500);
    }
  }

  .marquee-viewport {
    flex: 1;
    min-height: 0;
    overflow: hidden;

    &:hover .marquee-track {
      animation-play-state: paused;
    }
  }

  .marquee-track {
    display: flex;
    width: max-content;
    height: 100%;
    animation: promotion-marquee-scroll linear infinite;
  }

  .marquee-card {
    display: flex;
    flex-shrink: 0;
    gap: 10px;
    height: 100%;
    margin-right: 12px;
    padding: 10px;
    overflow: hidden;
    color: inherit;
    text-decoration: none;
    background: var(--art-gray-100);
    border: 1px solid transparent;
    border-radius: 12px;
    transition:
      border-color 0.2s ease,
      box-shadow 0.2s ease;

    &.is-clickable {
      cursor: pointer;

      &:hover {
        border-color: rgba(var(--art-primary-rgb), 0.24);
        box-shadow: 0 6px 16px rgba(0, 0, 0, 0.08);
      }
    }
  }

  .marquee-cover {
    display: flex;
    flex: 0 0 62px;
    align-items: center;
    justify-content: center;
    overflow: hidden;
    background: var(--art-primary);
    border-radius: 8px;

    img {
      width: 100%;
      height: 100%;
      object-fit: cover;
    }

    span {
      font-size: 22px;
      font-weight: 700;
      color: rgba(255, 255, 255, 0.92);
    }
  }

  .marquee-body {
    display: flex;
    flex: 1;
    flex-direction: column;
    justify-content: center;
    gap: 4px;
    min-width: 0;
  }

  .marquee-heading {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }

  .marquee-tag {
    flex-shrink: 0;
    padding: 1px 5px;
    font-size: 10px;
    line-height: 1.6;
    color: var(--art-primary);
    background: rgba(var(--art-primary-rgb), 0.1);
    border-radius: 5px;
  }

  .marquee-title {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    font-size: 13px;
    font-weight: 600;
    color: var(--art-gray-900);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .marquee-summary {
    display: -webkit-box;
    overflow: hidden;
    font-size: 11px;
    line-height: 1.5;
    color: var(--art-gray-500);
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
  }

  @keyframes promotion-marquee-scroll {
    from {
      transform: translateX(0);
    }

    to {
      transform: translateX(calc(-100% / var(--repeat)));
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .marquee-track {
      animation: none;
    }
  }
</style>
