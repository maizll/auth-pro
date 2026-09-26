import { computed } from 'vue'
import { useWindowSize } from '@vueuse/core'

/** 手机宽度。后台列表用它收起次要列，让前面的关键信息露出来。 */
export function useNarrowScreen(maxWidth = 768) {
  const { width } = useWindowSize()
  return computed(() => width.value > 0 && width.value <= maxWidth)
}
