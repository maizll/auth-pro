<!-- 布局容器 -->
<template>
  <div class="app-layout">
    <aside id="app-sidebar">
      <ArtSidebarMenu />
    </aside>

    <main id="app-main">
      <div id="app-header">
        <ArtHeaderBar />
      </div>
      <div id="app-content">
        <ArtPageContent />
      </div>
      <footer v-if="icpNumber" id="app-footer">
        <a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer">
          {{ icpNumber }}
        </a>
      </footer>
    </main>

    <div id="app-global">
      <ArtGlobalComponent />
      <!-- 管理端工作台弹窗：与代理/用户面板一样读 ?position=popup，不依赖全局组件开关 -->
      <ArtAdPopup />
    </div>
  </div>
</template>

<script setup lang="ts">
  import { useSystemConfigStore } from '@/store/modules/system-config'
  import { useMenuStore } from '@/store/modules/menu'
  import { fetchTicketUnreadCount } from '@/api/ticket'
  import type { AppRouteRecord } from '@/types/router'

  defineOptions({ name: 'AppLayout' })

  const { icpNumber } = storeToRefs(useSystemConfigStore())
  const menuStore = useMenuStore()

  let ticketBadgeTimer: ReturnType<typeof setInterval> | null = null

  /** 递归查找工单菜单项并同步未读角标 */
  function applyTicketBadge(count: number) {
    const walk = (list: AppRouteRecord[]): boolean => {
      for (const item of list) {
        if (item.name === 'TicketManage') {
          const badge = count > 0 ? String(Math.min(count, 99)) : ''
          if ((item.meta.showTextBadge || '') !== badge) {
            item.meta.showTextBadge = badge
          }
          return true
        }
        if (item.children && walk(item.children)) return true
      }
      return false
    }
    walk(menuStore.menuList)
  }

  async function refreshTicketBadge() {
    try {
      const { count } = await fetchTicketUnreadCount()
      applyTicketBadge(count)
    } catch {
      /* 未登录或接口异常时静默跳过 */
    }
  }

  onMounted(() => {
    refreshTicketBadge()
    ticketBadgeTimer = setInterval(refreshTicketBadge, 30000)
    window.addEventListener('panel-ticket-unread-refresh', refreshTicketBadge)
  })

  onBeforeUnmount(() => {
    if (ticketBadgeTimer) clearInterval(ticketBadgeTimer)
    window.removeEventListener('panel-ticket-unread-refresh', refreshTicketBadge)
  })
</script>

<style lang="scss" scoped>
  @use './style';
</style>
