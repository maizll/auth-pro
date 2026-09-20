<template>
  <div class="developer-panel-layout">
    <aside class="panel-sidebar">
      <div class="sidebar-header">
        <img :src="resolvedLogo" class="brand-logo" alt="logo" />
        <span class="brand-text" v-show="!collapsed">源站开发者</span>
      </div>

      <el-menu :default-active="currentRoute" :collapse="collapsed" router class="sidebar-menu">
        <el-menu-item v-for="item in menuItems" :key="item.path" :index="item.path">
          <el-icon><iconify-icon :icon="item.icon" /></el-icon>
          <template #title>{{ item.title }}</template>
        </el-menu-item>
      </el-menu>
    </aside>

    <div class="panel-main">
      <header class="panel-header">
        <div class="header-left">
          <el-icon class="collapse-btn" :size="18" @click="collapsed = !collapsed">
            <iconify-icon :icon="collapsed ? 'ri:menu-unfold-line' : 'ri:menu-fold-line'" />
          </el-icon>
          <el-breadcrumb separator="/">
            <el-breadcrumb-item>{{ siteName }}</el-breadcrumb-item>
            <el-breadcrumb-item>{{ currentTitle }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <div class="header-right">
          <ArtNotificationBell />
          <PanelThemeToggle scope="agent" />
          <span class="developer-name">{{ developerName }}</span>
          <el-dropdown trigger="click">
            <el-avatar :size="32" class="avatar-btn">
              <iconify-icon icon="ri:code-s-slash-line" width="18" />
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

      <main class="panel-content is-panel-surface">
        <router-view />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, ref } from 'vue'
  import { useRoute, useRouter } from 'vue-router'
  import { Icon as IconifyIcon } from '@iconify/vue'
  import { useSystemConfigStore } from '@/store/modules/system-config'
  import PanelThemeToggle from '@/components/core/theme/PanelThemeToggle.vue'
  import ArtNotificationBell from '@/components/core/layouts/art-notification/bell.vue'
  import {
    DEVELOPER_INFO_KEY,
    DEVELOPER_TOKEN_KEY,
    fetchSourceDeveloperMe
  } from '@/api/source-developer'

  const route = useRoute()
  const router = useRouter()
  const systemConfigStore = useSystemConfigStore()
  const { siteName, resolvedLogo } = storeToRefs(systemConfigStore)
  const collapsed = ref(false)

  const currentRoute = computed(() => route.path)
  const menuItems = [
    { path: '/developer-panel/dashboard', title: '概览', icon: 'ri:dashboard-line' },
    { path: '/developer-panel/plugins', title: '我的插件', icon: 'ri:puzzle-2-line' },
    { path: '/developer-panel/templates', title: '我的模板', icon: 'ri:layout-3-line' },
    { path: '/developer-panel/ads', title: '申请广告', icon: 'ri:advertisement-line' },
    { path: '/developer-panel/guide', title: '开发文档', icon: 'ri:book-open-line' }
  ]
  const titleMap: Record<string, string> = Object.fromEntries(
    menuItems.map((item) => [item.path, item.title])
  )
  const currentTitle = computed(() => titleMap[route.path] || '开发者工作台')
  const developerName = computed(() => {
    try {
      const info = JSON.parse(localStorage.getItem(DEVELOPER_INFO_KEY) || '{}')
      return info.displayName || info.username || '开发者'
    } catch {
      return '开发者'
    }
  })

  function handleLogout() {
    localStorage.removeItem(DEVELOPER_TOKEN_KEY)
    localStorage.removeItem(DEVELOPER_INFO_KEY)
    if (localStorage.getItem('agent_panel_token')) {
      router.push('/agent-panel/become-developer')
      return
    }
    router.push('/agent-panel/login')
  }

  onMounted(async () => {
    try {
      const { data, status } = await fetchSourceDeveloperMe()
      if (status === 401 || data.code === 401 || data.code === 403) {
        router.replace('/agent-panel/become-developer')
      }
    } catch {
      /* Keep the shell; page requests will handle auth errors. */
    }
  })
</script>

<style scoped lang="scss">
  .developer-panel-layout {
    display: flex;
    height: 100vh;
    overflow: hidden;
    background: var(--el-bg-color);
  }

  .panel-sidebar {
    width: 220px;
    background: var(--el-bg-color);
    display: flex;
    flex-direction: column;
    transition: width 0.3s;
    flex-shrink: 0;
    border-right: 1px solid var(--el-border-color-lighter);
    box-shadow: 2px 0 8px rgb(0 0 0 / 3%);

    .sidebar-header {
      display: flex;
      gap: 10px;
      align-items: center;
      height: 56px;
      padding: 0 16px;
      border-bottom: 1px solid var(--el-border-color-lighter);

      .brand-logo {
        width: 32px;
        height: 32px;
        border-radius: 6px;
        flex-shrink: 0;
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
      border-right: none;

      :deep(.el-menu-item) {
        height: 44px;
        margin: 2px 8px;
        border-radius: 8px;

        &.is-active {
          color: var(--el-color-primary);
          background: var(--el-color-primary-light-9);
        }
      }
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

      .developer-name {
        font-size: 14px;
        color: var(--el-text-color-primary);
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
