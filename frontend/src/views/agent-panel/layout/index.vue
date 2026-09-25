<template>
  <div
    class="agent-panel-layout"
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
        <el-menu-item index="/agent-panel/dashboard">
          <el-icon><iconify-icon icon="ri:dashboard-line" /></el-icon>
          <template #title>概览</template>
        </el-menu-item>
        <el-menu-item index="/agent-panel/licenses">
          <el-icon><iconify-icon icon="ri:file-list-3-line" /></el-icon>
          <template #title>我的授权</template>
        </el-menu-item>
        <el-menu-item index="/agent-panel/purchase">
          <el-icon><iconify-icon icon="ri:add-circle-line" /></el-icon>
          <template #title>开通授权</template>
        </el-menu-item>
        <el-menu-item index="/agent-panel/finance">
          <el-icon><iconify-icon icon="ri:money-cny-circle-line" /></el-icon>
          <template #title>我的财务</template>
        </el-menu-item>
        <el-menu-item index="/agent-panel/tickets">
          <el-icon><iconify-icon icon="ri:customer-service-2-line" /></el-icon>
          <template #title>
            <span class="menu-label">
              <span>我的工单</span>
              <el-badge v-if="ticketUnread" :value="ticketUnread" :max="99" class="menu-badge" />
            </span>
          </template>
        </el-menu-item>
        <el-menu-item index="/agent-panel/store">
          <el-icon><iconify-icon icon="ri:vip-crown-line" /></el-icon>
          <template #title>已绑定站点</template>
        </el-menu-item>
        <el-menu-item index="/agent-panel/profile">
          <el-icon><iconify-icon icon="ri:settings-3-line" /></el-icon>
          <template #title>个人设置</template>
        </el-menu-item>
        <el-menu-item index="/agent-panel/become-developer">
          <el-icon><iconify-icon icon="ri:code-s-slash-line" /></el-icon>
          <template #title>开发者入驻</template>
        </el-menu-item>
      </el-menu>
      <div v-show="!menuCollapsed" class="sidebar-ad">
        <ArtAdSlot position="sidebar" height="120px" />
      </div>
    </aside>

    <!-- 主内容区 -->
    <div class="panel-main">
      <!-- 顶栏 -->
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
          <PanelThemeToggle scope="agent" />
          <el-button
            v-if="developerApproved"
            type="primary"
            text
            class="developer-entry"
            @click="enterDeveloper"
          >
            <el-icon><iconify-icon icon="ri:code-s-slash-line" /></el-icon>
            <span class="developer-entry-label">进入开发者端</span>
          </el-button>
          <span class="agent-name">{{ agentName }}</span>
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

      <!-- 页面内容 -->
      <main
        class="panel-content"
        :class="{
          'is-panel-surface': [
            '/agent-panel/dashboard',
            '/agent-panel/profile',
            '/agent-panel/become-developer'
          ].includes(currentRoute)
        }"
      >
        <router-view />
      </main>
    </div>

    <ArtAdPopup />
  </div>
</template>

<script setup lang="ts">
  import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
  import { useRoute, useRouter } from 'vue-router'
  import { Icon as IconifyIcon } from '@iconify/vue'
  import axios from 'axios'
  import { useSystemConfigStore } from '@/store/modules/system-config'
  import PanelThemeToggle from '@/components/core/theme/PanelThemeToggle.vue'
  import ArtNotificationBell from '@/components/core/layouts/art-notification/bell.vue'
  import { usePanelMobileNav } from '@/hooks/core/usePanelMobileNav'
  import {
    enterDeveloperSessionFromAgent,
    fetchSourceDeveloperApplyStatus
  } from '@/api/source-developer'

  const route = useRoute()
  const router = useRouter()
  const systemConfigStore = useSystemConfigStore()
  const { siteName, resolvedLogo } = storeToRefs(systemConfigStore)
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
  const ticketUnread = ref(0)
  const developerApproved = ref(false)
  let ticketUnreadTimer: ReturnType<typeof setInterval> | null = null

  async function fetchTicketUnread() {
    try {
      const { data } = await axios.get('/api/agent-panel/tickets/unread-count', {
        headers: {
          Authorization: `Bearer ${localStorage.getItem('agent_panel_token') || ''}`
        }
      })
      if (data.code === 200) ticketUnread.value = data.data.count || 0
    } catch {
      /* Ignore unread request errors. */
    }
  }

  async function fetchDeveloperStatus() {
    try {
      const { data } = await fetchSourceDeveloperApplyStatus()
      developerApproved.value = data.code === 200 && data.data?.status === 'approved'
    } catch {
      developerApproved.value = false
    }
  }

  function enterDeveloper() {
    enterDeveloperSessionFromAgent()
    router.push('/developer-panel/dashboard')
  }

  onMounted(() => {
    fetchTicketUnread()
    fetchDeveloperStatus()
    ticketUnreadTimer = setInterval(fetchTicketUnread, 30000)
    window.addEventListener('panel-ticket-unread-refresh', fetchTicketUnread)
  })

  onBeforeUnmount(() => {
    if (ticketUnreadTimer) clearInterval(ticketUnreadTimer)
    window.removeEventListener('panel-ticket-unread-refresh', fetchTicketUnread)
  })

  const currentRoute = computed(() => route.path)

  const titleMap: Record<string, string> = {
    '/agent-panel/dashboard': '概览',
    '/agent-panel/licenses': '我的授权',
    '/agent-panel/purchase': '开通授权',
    '/agent-panel/finance': '我的财务',
    '/agent-panel/tickets': '我的工单',
    '/agent-panel/store': '已绑定站点',
    '/agent-panel/profile': '个人设置',
    '/agent-panel/become-developer': '开发者入驻'
  }

  const currentTitle = computed(() => titleMap[route.path] || '概览')
  const agentName = computed(() => {
    try {
      const info = JSON.parse(localStorage.getItem('agent_panel_info') || '{}')
      return info.name || info.email || '代理商'
    } catch {
      return '代理商'
    }
  })

  function handleLogout() {
    localStorage.removeItem('agent_panel_token')
    localStorage.removeItem('agent_panel_info')
    router.push('/agent-panel/login')
  }
</script>

<style scoped lang="scss">
  @use '@/assets/styles/core/panel-shell' as panel;

  .agent-panel-layout {
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

      .agent-name {
        font-size: 14px;
        color: var(--el-text-color-primary);
      }

      .developer-entry {
        margin-right: 4px;
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
</style>
