import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  PANEL_MOBILE_MAX_WIDTH,
  panelNavExpanded,
  panelToggleIcon,
  panelToggleLabel,
  reducePanelNav,
  syncPanelNavToViewport,
  type PanelNavEvent,
  type PanelNavState
} from './panelNavState'

const MOBILE_QUERY = `(max-width: ${PANEL_MOBILE_MAX_WIDTH}px)`

/**
 * 用户端 / 代理端共用的侧栏开关。
 * 窄屏抽屉打开时锁住 body 滚动，并在路由变化、Esc、遮罩点击时关闭。
 */
export function usePanelMobileNav() {
  const route = useRoute()
  const isMobile = ref(readMobile())
  const navOpen = ref(false)
  const collapsed = ref(false)

  function snapshot(): PanelNavState {
    return {
      isMobile: isMobile.value,
      navOpen: navOpen.value,
      collapsed: collapsed.value
    }
  }

  function commit(next: PanelNavState) {
    isMobile.value = next.isMobile
    navOpen.value = next.navOpen
    collapsed.value = next.collapsed
  }

  function dispatch(event: PanelNavEvent) {
    commit(reducePanelNav(snapshot(), event))
  }

  function onViewportChange(matches: boolean) {
    commit(syncPanelNavToViewport(snapshot(), matches))
  }

  const menuCollapsed = computed(() => !isMobile.value && collapsed.value)
  const navExpanded = computed(() => panelNavExpanded(snapshot()))
  const toggleIcon = computed(() => panelToggleIcon(snapshot()))
  const toggleLabel = computed(() => panelToggleLabel(snapshot()))

  let media: MediaQueryList | null = null

  function handleMedia(event: MediaQueryListEvent) {
    onViewportChange(event.matches)
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') dispatch('escape')
  }

  watch(
    () => route.fullPath,
    () => dispatch('route')
  )

  watch(
    [isMobile, navOpen],
    ([mobile, open]) => {
      document.body.classList.toggle('panel-nav-scroll-lock', Boolean(mobile && open))
    },
    { immediate: true }
  )

  onMounted(() => {
    media = window.matchMedia(MOBILE_QUERY)
    onViewportChange(media.matches)
    media.addEventListener('change', handleMedia)
    window.addEventListener('keydown', handleKeydown)
  })

  onBeforeUnmount(() => {
    media?.removeEventListener('change', handleMedia)
    window.removeEventListener('keydown', handleKeydown)
    document.body.classList.remove('panel-nav-scroll-lock')
  })

  return {
    isMobile,
    navOpen,
    menuCollapsed,
    navExpanded,
    toggleIcon,
    toggleLabel,
    toggleNav: () => dispatch('toggle'),
    closeNav: () => dispatch('close')
  }
}

function readMobile() {
  return typeof window !== 'undefined' && window.matchMedia(MOBILE_QUERY).matches
}
