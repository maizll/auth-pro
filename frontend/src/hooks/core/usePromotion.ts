import { RouterLink } from 'vue-router'
import type { Component } from 'vue'
import type { PromotionItem } from '@/api/promotion'

/** 渲染广告卡片外壳所需的标签与属性；无投放链接时退化为普通容器 */
export interface PromotionLink {
  is: Component | string
  props: Record<string, unknown>
}

const isExternalUrl = (url: string) => /^https?:\/\//i.test(url)

/** 广告投放项的通用行为：链接解析与无图时的渐变底色 */
export function usePromotion() {
  /**
   * 站内地址交给路由链接，避免整页刷新；两种情况都会渲染成真实的 a 标签，
   * 用户可以右键新开、中键打开、悬停查看目标。
   */
  const resolvePromotionLink = (url: string): PromotionLink => {
    if (!url) return { is: 'div', props: {} }
    if (isExternalUrl(url)) {
      return {
        is: 'a',
        props: { href: url, target: '_blank', rel: 'noopener noreferrer' }
      }
    }
    return { is: RouterLink, props: { to: url } }
  }

  const coverStyle = (item: PromotionItem) => {
    const accent = item.accent
    if (!accent) return {}
    const fade = /^#[0-9a-f]{6}$/i.test(accent) ? `${accent}b3` : accent
    return { background: `linear-gradient(135deg, ${accent}, ${fade})` }
  }

  return { resolvePromotionLink, coverStyle }
}
