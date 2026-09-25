<template>
  <div
    class="user-panel-layout"
    :class="{ 'is-nav-open': isMobile && navOpen, 'is-collapsed': menuCollapsed }"
  >
    <button
      v-if="isMobile"
      type="button"
      class="panel-nav-mask"
      :class="{ 'is-visible': navOpen }"
      aria-label="关闭导航"
      :tabindex="navOpen ? 0 : -1"
      @click="closeNav"
    />
    <aside
      id="panel-sidebar"
      class="panel-sidebar"
      :inert="isMobile && !navOpen"
      :aria-hidden="isMobile && !navOpen"
    >
      <div class="sidebar-header">
        <img :src="resolvedLogo" class="brand-logo" alt="logo" />
        <span class="brand-text" v-show="!menuCollapsed">{{ siteName }}</span>
        <button
          v-if="isMobile"
          type="button"
          class="sidebar-close"
          aria-label="关闭导航"
          @click="closeNav"
        >
          <el-icon :size="18"><iconify-icon icon="ri:close-line" /></el-icon>
        </button>
      </div>

      <el-menu
        :default-active="currentRoute"
        :collapse="menuCollapsed"
        router
        class="sidebar-menu"
        @select="closeNav"
      >
        <el-menu-item index="/user/dashboard">
          <el-icon><iconify-icon icon="ri:dashboard-line" /></el-icon>
          <template #title>概览</template>
        </el-menu-item>
        <el-menu-item index="/user/licenses">
          <el-icon><iconify-icon icon="ri:shield-check-line" /></el-icon>
          <template #title>我的授权</template>
        </el-menu-item>
        <el-menu-item index="/user/tickets">
          <el-icon><iconify-icon icon="ri:customer-service-2-line" /></el-icon>
          <template #title>
            <span class="menu-label">
              <span>我的工单</span>
              <el-badge v-if="ticketUnread" :value="ticketUnread" :max="99" class="menu-badge" />
            </span>
          </template>
        </el-menu-item>
        <el-menu-item v-if="selfPurchaseEnabled" index="/user/purchase">
          <el-icon><iconify-icon icon="ri:shopping-cart-2-line" /></el-icon>
          <template #title>购买授权</template>
        </el-menu-item>
        <el-menu-item index="/user/store">
          <el-icon><iconify-icon icon="ri:vip-crown-line" /></el-icon>
          <template #title>已绑定站点</template>
        </el-menu-item>
        <el-menu-item index="/user/profile">
          <el-icon><iconify-icon icon="ri:settings-3-line" /></el-icon>
          <template #title>个人设置</template>
        </el-menu-item>
        <el-menu-item index="/user/become-agent">
          <el-icon><iconify-icon icon="ri:user-star-line" /></el-icon>
          <template #title>开通代理商</template>
        </el-menu-item>
      </el-menu>
      <div v-show="!menuCollapsed" class="sidebar-ad">
        <ArtAdSlot position="sidebar" height="120px" />
      </div>
    </aside>

    <div class="panel-main">
      <header class="panel-header">
        <div class="header-left">
          <button
            type="button"
            class="collapse-btn"
            :aria-label="toggleLabel"
            :aria-expanded="navExpanded"
            aria-controls="panel-sidebar"
            @click="toggleNav"
          >
            <el-icon :size="18">
              <iconify-icon :icon="toggleIcon" />
            </el-icon>
          </button>
          <el-breadcrumb separator="/">
            <el-breadcrumb-item>{{ siteName }}</el-breadcrumb-item>
            <el-breadcrumb-item>{{ currentTitle }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <div class="header-right">
          <ArtNotificationBell />
          <PanelThemeToggle scope="user" />
          <span class="header-balance">
            <iconify-icon icon="ri:wallet-3-line" width="16" />
            ¥{{ balance.toFixed(2) }}
          </span>
          <span class="user-name">{{ nickname }}</span>
          <el-dropdown trigger="click">
            <el-avatar :size="32" class="avatar-btn">
              <iconify-icon icon="ri:user-3-fill" width="18" />
            </el-avatar>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item @click="handleLogout">
                  <el-icon><iconify-icon icon="ri:logout-box-r-line" /></el-icon>
                  退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </header>

      <main
        class="panel-content"
        :class="{
          'is-panel-surface': ['/user/dashboard', '/user/profile', '/user/become-agent'].includes(
            currentRoute
          )
        }"
      >
        <router-view />
      </main>
      <footer v-if="icpNumber" class="panel-footer">
        <a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer">
          {{ icpNumber }}
        </a>
      </footer>
    </div>

    <ArtAdPopup />
  </div>
</template>

<script setup lang="ts">
  import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
  import { useRoute, useRouter } from 'vue-router'
  import { Icon as IconifyIcon } from '@iconify/vue'
  import axios from 'axios'
  import { useSystemConfigStore } from '@/store/modules/system-config'
  import PanelThemeToggle from '@/components/core/theme/PanelThemeToggle.vue'
  import ArtNotificationBell from '@/components/core/layouts/art-notification/bell.vue'
  import { usePanelMobileNav } from '@/hooks/core/usePanelMobileNav'

  const route = useRoute()
  const router = useRouter()
  const systemConfigStore = useSystemConfigStore()
  const { siteName, resolvedLogo, selfPurchaseEnabled, icpNumber } = storeToRefs(systemConfigStore)
  const {
    isMobile,
    navOpen,
    menuCollapsed,
    navExpanded,
    toggleIcon,
    toggleLabel,
    toggleNav,
    closeNav
  } = usePanelMobileNav()
  const balance = ref(0)
  const nickname = ref('')
  const ticketUnread = ref(0)
  let ticketUnreadTimer: ReturnType<typeof setInterval> | null = null

  function getToken() {
    return localStorage.getItem('user_panel_token') || ''
  }

  async function fetchTicketUnread() {
    try {
      const { data } = await axios.get('/api/user-panel/tickets/unread-count', {
        headers: { Authorization: `Bearer ${getToken()}` }
      })
      if (data.code === 200) ticketUnread.value = data.data.count || 0
    } catch {
      /* Ignore unread request errors. */
    }
  }

  function loadUserInfo() {
    try {
      const info = JSON.parse(localStorage.getItem('user_panel_info') || '{}')
      nickname.value = info.nickname || info.email || '用户'
    } catch {
      nickname.value = '用户'
    }
  }

  async function fetchBalance() {
    try {
      const { data } = await axios.get('/api/user-panel/balance', {
        headers: { Authorization: `Bearer ${getToken()}` }
      })
      if (data.code === 200) {
        balance.value = data.data.balance || 0
      }
    } catch {
      /* Ignore balance request errors. */
    }
  }

  onMounted(() => {
    loadUserInfo()
    fetchBalance()
    fetchTicketUnread()
    ticketUnreadTimer = setInterval(fetchTicketUnread, 30000)
    window.addEventListener('user-panel-balance-refresh', fetchBalance)
    window.addEventListener('panel-ticket-unread-refresh', fetchTicketUnread)
  })

  onBeforeUnmount(() => {
    if (ticketUnreadTimer) clearInterval(ticketUnreadTimer)
    window.removeEventListener('user-panel-balance-refresh', fetchBalance)
    window.removeEventListener('panel-ticket-unread-refresh', fetchTicketUnread)
  })

  watch(
    selfPurchaseEnabled,
    (enabled) => {
      if (!enabled && route.path === '/user/purchase') router.replace('/user/dashboard')
    },
    { immediate: true }
  )

  const currentRoute = computed(() => route.path)

  const titleMap: Record<string, string> = {
    '/user/dashboard': '概览',
    '/user/licenses': '我的授权',
    '/user/tickets': '我的工单',
    '/user/purchase': '购买授权',
    '/user/store': '已绑定站点',
    '/user/profile': '个人设置',
    '/user/become-agent': '开通代理商'
  }

  const currentTitle = computed(() => titleMap[route.path] || '概览')

  function handleLogout() {
    localStorage.removeItem('user_panel_token')
    localStorage.removeItem('user_panel_info')
    router.push('/user/login')
  }
</script>

<style scoped lang="scss">
  @use '@/assets/styles/core/panel-shell' as panel;

  .user-panel-layout {
    display: flex;
    height: 100vh;
    overflow: hidden;
    background: var(--el-bg-color);

    @include panel.shell;
  }

  .panel-sidebar {
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
    width: 220px;
    background: var(--el-bg-color);
    border-right: 1px solid var(--el-border-color-lighter);
    box-shadow: 2px 0 8px rgb(0 0 0 / 3%);
    transition: width 0.3s;

    .sidebar-header {
      display: flex;
      gap: 10px;
      align-items: center;
      height: 56px;
      padding: 0 16px;
      border-bottom: 1px solid var(--el-border-color-lighter);

      .brand-logo {
        flex-shrink: 0;
        width: 32px;
        height: 32px;
        border-radius: 6px;
      }

      .brand-text {
        font-size: 15px;
        font-weight: 600;
        color: var(--el-text-color-primary);
        white-space: nowrap;
      }
    }

    .sidebar-menu {
      flex: 1;
      padding: 8px 0;
      overflow: visible;
      border-right: none;

      :deep(.el-menu-item) {
        height: 44px;
        margin: 2px 8px;
        overflow: visible;
        line-height: 1;
        border-radius: 8px;

        &.is-active {
          color: var(--el-color-primary);
          background: var(--el-color-primary-light-9);
        }
      }

      .menu-label {
        display: inline-flex;
        gap: 6px;
        align-items: center;
        max-width: 100%;
        overflow: visible;
        line-height: 1;
      }

      .menu-badge {
        display: inline-flex;
        flex: none;
        align-items: center;
        line-height: 1;

        :deep(.el-badge__content.is-fixed),
        :deep(.el-badge__content) {
          position: static;
          top: auto;
          right: auto;
          vertical-align: middle;
          border: none;
          transform: none;
        }
      }
    }

    .sidebar-ad {
      flex-shrink: 0;
      padding: 10px;
    }
  }

  .panel-main {
    display: flex;
    flex: 1;
    flex-direction: column;
    overflow: hidden;
  }

  .panel-header {
    display: flex;
    flex-shrink: 0;
    align-items: center;
    justify-content: space-between;
    height: 56px;
    padding: 0 20px;
    background: var(--el-bg-color);
    border-bottom: 1px solid var(--el-border-color-lighter);

    .header-left {
      display: flex;
      gap: 16px;
      align-items: center;

      .collapse-btn {
        color: var(--el-text-color-secondary);
        cursor: pointer;
        transition: color 0.2s;

        &:hover {
          color: var(--el-color-primary);
        }
      }
    }

    .header-right {
      display: flex;
      gap: 12px;
      align-items: center;

      .user-name {
        font-size: 14px;
        color: var(--el-text-color-primary);
      }

      .header-balance {
        display: inline-flex;
        gap: 4px;
        align-items: center;
        padding: 4px 10px;
        font-family: 'DIN Alternate', 'Roboto Mono', monospace;
        font-size: 14px;
        font-weight: 600;
        color: var(--el-color-success);
        background: var(--el-color-success-light-9);
        border-radius: 6px;
      }

      .avatar-btn {
        color: var(--el-color-primary);
        cursor: pointer;
        background: var(--el-color-primary-light-7);
      }
    }
  }

  .panel-content {
    flex: 1;
    padding: 16px;
    overflow: auto;
    background: var(--el-bg-color-page);

    &.is-panel-surface {
      background: var(--el-bg-color);
    }
  }

  .panel-footer {
    flex-shrink: 0;
    padding: 8px 16px 10px;
    font-size: 12px;
    text-align: center;
    border-top: 1px solid var(--el-border-color-lighter);

    a {
      color: var(--el-text-color-placeholder);
      text-decoration: none;

      &:hover {
        color: var(--el-color-primary);
      }
    }
  }
</style>
