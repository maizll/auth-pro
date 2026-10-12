import { onMounted, ref } from 'vue'
import {
  fetchAdvertisements,
  DEFAULT_AD_PLACEHOLDER,
  isAdRecruitPosition
} from '@/api/advertisement'
import type { AdPosition, AdvertisementItem, AdvertisementPlaceholder } from '@/api/advertisement'

/**
 * 拉取某个广告位。
 * - 有投放：返回 records（会话内按权重固定一条，禁止轮播）
 * - 槽位开启且空、且为可招租卡片位：recruiting=true（由 ArtAdSlot 画虚线招租）
 * - 槽位关闭 / 不可达 / 非招租位空位：塌掉
 */
export function useAdvertisement(position: AdPosition) {
  const items = ref<AdvertisementItem[]>([])
  const loading = ref(true)
  const unreachable = ref(false)
  /** 兼容旧调用：无真实投放时为 true */
  const isPlaceholderOnly = ref(true)
  /** 官方远程开关：明确为 true 才招租；缺省/false 空位不招租 */
  const slotEnabled = ref(false)
  const placeholder = ref<AdvertisementPlaceholder>({ ...DEFAULT_AD_PLACEHOLDER })
  /** 开启 + 空位 + 卡片招租位 */
  const recruiting = ref(false)

  const sessionKey = `auth-pro:ad-pick:${position}`

  const pickOne = (records: AdvertisementItem[]) => {
    if (records.length <= 1) return records
    const sorted = [...records].sort((a, b) => (b.weight || 0) - (a.weight || 0))
    const cached = sessionStorage.getItem(sessionKey)
    if (cached) {
      const hit = sorted.find((r) => r.id === cached)
      if (hit) return [hit]
    }
    const chosen = sorted[0]
    sessionStorage.setItem(sessionKey, chosen.id)
    return [chosen]
  }

  const load = async () => {
    loading.value = true
    unreachable.value = false
    slotEnabled.value = false
    recruiting.value = false
    try {
      const result = await fetchAdvertisements(position)
      const records = (result?.records ?? []).filter((r) => r && !r.isPlaceholder)
      const enabled = result?.enabled === true
      slotEnabled.value = enabled
      if (result?.placeholder) {
        placeholder.value = {
          title: result.placeholder.title || DEFAULT_AD_PLACEHOLDER.title,
          description: result.placeholder.description || DEFAULT_AD_PLACEHOLDER.description,
          linkUrl: result.placeholder.linkUrl || ''
        }
      } else {
        placeholder.value = { ...DEFAULT_AD_PLACEHOLDER }
      }
      isPlaceholderOnly.value = records.length === 0
      items.value = pickOne(records)
      recruiting.value =
        enabled && records.length === 0 && isAdRecruitPosition(position) && !unreachable.value
    } catch {
      unreachable.value = true
      isPlaceholderOnly.value = true
      slotEnabled.value = false
      recruiting.value = false
      items.value = []
    } finally {
      loading.value = false
    }
  }

  onMounted(load)

  return {
    items,
    loading,
    isPlaceholderOnly,
    unreachable,
    slotEnabled,
    placeholder,
    recruiting,
    load
  }
}
