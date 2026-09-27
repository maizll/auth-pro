import { computed } from 'vue'
import { useWindowSize } from '@vueuse/core'

/** 手机宽度，和列表样式的 767px 断点一致。 */
export function useNarrowScreen(maxWidth = 767) {
  const { width } = useWindowSize()
  return computed(() => width.value > 0 && width.value <= maxWidth)
}
