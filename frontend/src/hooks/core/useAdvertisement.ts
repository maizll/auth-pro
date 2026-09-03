import { onMounted, ref } from 'vue'
import { fetchAdvertisements } from '@/api/advertisement'
import type { AdPosition, AdvertisementItem } from '@/api/advertisement'

/** 无投放数据时的占位项，让广告位显示「广告位出租」而不是留一块空白 */
const placeholderItem = (position: AdPosition): AdvertisementItem => ({
  id: `placeholder-${position}`,
  title: '广告位出租',
  imageUrl: '',
  destinationUrl: '',
  position,
  weight: 0,
  startAt: '',
  endAt: '',
  description: '虚位以待，欢迎联系投放',
  isPlaceholder: true
})

/**
 * 拉取某个广告位的投放内容。
 * 接口失败一律当作无投放处理：广告不是业务功能，不该把错误抛到后台界面上。
 */
export function useAdvertisement(position: AdPosition) {
  const items = ref<AdvertisementItem[]>([])
  const loading = ref(true)
  /** 是否只剩占位内容，弹窗一类的场景据此决定不打扰用户 */
  const isPlaceholderOnly = ref(true)

  const load = async () => {
    loading.value = true
    try {
      const result = await fetchAdvertisements(position)
      const records = result?.records ?? []
      isPlaceholderOnly.value = records.length === 0
      items.value = records.length > 0 ? records : [placeholderItem(position)]
    } catch {
      isPlaceholderOnly.value = true
      items.value = [placeholderItem(position)]
    } finally {
      loading.value = false
    }
  }

  onMounted(load)

  return { items, loading, isPlaceholderOnly, load }
}
