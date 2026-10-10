import { onMounted, ref } from 'vue'
import { fetchAdvertisements } from '@/api/advertisement'
import type { AdPosition, AdvertisementItem } from '@/api/advertisement'

/**
 * 拉取某个广告位。失败 / 空 / 仅占位 → 空列表（由槽位塌掉，不打扰）。
 * 多条时按权重挑一条，会话内固定（禁止轮播）。
 */
export function useAdvertisement(position: AdPosition) {
  const items = ref<AdvertisementItem[]>([])
  const loading = ref(true)
  const unreachable = ref(false)
  const isPlaceholderOnly = ref(true)

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
    try {
      const result = await fetchAdvertisements(position)
      const records = (result?.records ?? []).filter((r) => r && !r.isPlaceholder)
      isPlaceholderOnly.value = records.length === 0
      items.value = pickOne(records)
    } catch {
      unreachable.value = true
      isPlaceholderOnly.value = true
      items.value = []
    } finally {
      loading.value = false
    }
  }

  onMounted(load)

  return { items, loading, isPlaceholderOnly, unreachable, load }
}
