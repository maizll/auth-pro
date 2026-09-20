<template>
  <div class="developer-login">
    <div class="theme-toggle">
      <PanelThemeToggle scope="agent" />
    </div>
    <div class="login-container">
      <div class="login-left">
        <div class="brand">
          <h1 class="brand-title">{{ siteName }}</h1>
          <p class="brand-desc">源站开发者工作台</p>
        </div>
        <div class="features">
          <div class="feature-item">
            <el-icon :size="20" color="#67c23a"><i class="ri-check-line" /></el-icon>
            <span>提交插件元数据</span>
          </div>
          <div class="feature-item">
            <el-icon :size="20" color="#67c23a"><i class="ri-check-line" /></el-icon>
            <span>管理首页模板</span>
          </div>
          <div class="feature-item">
            <el-icon :size="20" color="#67c23a"><i class="ri-check-line" /></el-icon>
            <span>跟踪审核与上架状态</span>
          </div>
        </div>
      </div>

      <div class="login-right">
        <div class="login-form-wrapper">
          <h2 class="form-title">进入开发者端</h2>
          <p class="form-subtitle">请使用代理商账号登录后进入开发者端</p>
          <el-alert
            class="approved-notice"
            title="开发者端复用代理商登录"
            description="不再使用独立的开发者用户名和密码。请先登录代理商面板，审核通过后即可一键进入。"
            type="info"
            :closable="false"
            show-icon
          />
          <el-button type="primary" size="large" class="login-btn" :loading="loading" @click="goAgentLogin">
            前往代理商登录
          </el-button>
          <div class="login-footer">
            <span>已是代理商？</span>
            <el-link type="primary" :underline="false" @click="goApply">前往开发者入驻</el-link>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, ref } from 'vue'
  import { useRoute, useRouter } from 'vue-router'
  import { useSystemConfigStore } from '@/store/modules/system-config'
  import PanelThemeToggle from '@/components/core/theme/PanelThemeToggle.vue'
  import {
    AGENT_TOKEN_KEY,
    DEVELOPER_TOKEN_KEY,
    enterDeveloperSessionFromAgent,
    fetchSourceDeveloperApplyStatus
  } from '@/api/source-developer'

  const router = useRouter()
  const route = useRoute()
  const systemConfigStore = useSystemConfigStore()
  const { siteName } = storeToRefs(systemConfigStore)
  const loading = ref(false)

  function developerRedirect() {
    return typeof route.query.redirect === 'string' &&
      route.query.redirect.startsWith('/developer-panel')
      ? route.query.redirect
      : '/developer-panel/dashboard'
  }

  function goAgentLogin() {
    router.push({
      path: '/agent-panel/login',
      query: { redirect: developerRedirect() }
    })
  }

  function goApply() {
    router.push('/agent-panel/become-developer')
  }

  async function tryEnterFromAgent() {
    const agentToken = localStorage.getItem(AGENT_TOKEN_KEY)
    if (!agentToken) return false
    loading.value = true
    try {
      const { data } = await fetchSourceDeveloperApplyStatus()
      if (data.code === 200 && data.data?.status === 'approved') {
        enterDeveloperSessionFromAgent()
        router.replace(developerRedirect())
        return true
      }
      router.replace('/agent-panel/become-developer')
      return true
    } catch {
      return false
    } finally {
      loading.value = false
    }
  }

  onMounted(async () => {
    if (await tryEnterFromAgent()) return
    if (localStorage.getItem(DEVELOPER_TOKEN_KEY)) {
      router.replace(developerRedirect())
    }
  })
</script>

<style scoped lang="scss">
  .developer-login {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 100%;
    min-height: 100vh;
    padding: 32px;
    background:
      radial-gradient(circle at 20% 20%, rgb(64 158 255 / 8%), transparent 28%),
      linear-gradient(180deg, var(--el-bg-color) 0%, var(--el-bg-color-page) 100%);
  }

  .theme-toggle {
    position: absolute;
    top: 20px;
    right: 24px;
    z-index: 2;
  }

  .login-container {
    display: flex;
    width: 880px;
    min-height: 540px;
    overflow: hidden;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 24px;
    box-shadow: 0 24px 70px rgb(30 41 59 / 10%);
  }

  .login-left {
    display: flex;
    flex-direction: column;
    justify-content: center;
    width: 360px;
    padding: 52px 40px;
    color: var(--el-text-color-primary);
    background: var(--el-fill-color-light);
    border-right: 1px solid var(--el-border-color-lighter);

    .brand {
      margin-bottom: 36px;

      .brand-title {
        margin-bottom: 10px;
        font-size: 25px;
        font-weight: 700;
        color: var(--el-text-color-primary);
        letter-spacing: 0.2px;
      }

      .brand-desc {
        font-size: 14px;
        color: var(--el-text-color-secondary);
      }
    }

    .features {
      display: grid;
      gap: 14px;

      .feature-item {
        display: flex;
        gap: 10px;
        align-items: center;
        padding: 12px 14px;
        font-size: 14px;
        color: var(--el-text-color-regular);
        background: var(--el-bg-color);
        border: 1px solid var(--el-border-color-lighter);
        border-radius: 14px;
      }
    }
  }

  .login-right {
    display: flex;
    flex: 1;
    align-items: center;
    justify-content: center;
    padding: 52px;
    background: var(--el-bg-color);

    .login-form-wrapper {
      width: 100%;
      max-width: 352px;
    }

    .form-title {
      margin-bottom: 8px;
      font-size: 24px;
      font-weight: 700;
      color: var(--el-text-color-primary);
    }

    .form-subtitle {
      margin-bottom: 30px;
      font-size: 14px;
      color: var(--el-text-color-secondary);
    }

    .approved-notice {
      margin: -12px 0 22px;
    }

    .login-btn {
      width: 100%;
      height: 46px;
      font-size: 16px;
      font-weight: 600;
      border-radius: 12px;
    }

    .login-footer {
      margin-top: 24px;
      font-size: 13px;
      color: var(--el-text-color-secondary);
      text-align: center;
    }
  }

  @media (max-width: 768px) {
    .developer-login {
      padding: 20px;
    }

    .login-left {
      display: none;
    }

    .login-container {
      width: 100%;
      min-height: auto;
    }

    .login-right {
      padding: 36px 24px;
    }
  }
</style>
