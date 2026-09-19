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
          <h2 class="form-title">开发者登录</h2>
          <p class="form-subtitle">请输入已通过审核的开发者账号</p>
          <el-alert
            v-if="approvedNotice"
            class="approved-notice"
            title="申请已通过，可登录开发者端"
            description="请使用入驻时设置的用户名和密码登录。开发者账号与代理商账号相互独立。"
            type="success"
            :closable="false"
            show-icon
          />

          <el-form
            ref="loginFormRef"
            :model="loginForm"
            :rules="loginRules"
            class="login-form"
            @keyup.enter="handleLogin"
          >
            <el-form-item prop="username">
              <el-input
                v-model="loginForm.username"
                placeholder="开发者用户名"
                size="large"
                prefix-icon="ri-user-line"
              />
            </el-form-item>
            <el-form-item prop="password">
              <el-input
                v-model="loginForm.password"
                type="password"
                placeholder="开发者登录密码"
                size="large"
                show-password
                prefix-icon="ri-lock-line"
              />
            </el-form-item>
            <el-form-item>
              <el-button
                type="primary"
                size="large"
                :loading="loading"
                class="login-btn"
                @click="handleLogin"
              >
                登 录
              </el-button>
            </el-form-item>
          </el-form>

          <div class="login-footer">
            <span>还没有开发者账号？</span>
            <el-link type="primary" :underline="false" @click="goApply">前往代理商面板申请入驻</el-link>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, reactive, ref } from 'vue'
  import { useRoute, useRouter } from 'vue-router'
  import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
  import { useSystemConfigStore } from '@/store/modules/system-config'
  import PanelThemeToggle from '@/components/core/theme/PanelThemeToggle.vue'
  import {
    DEVELOPER_INFO_KEY,
    DEVELOPER_TOKEN_KEY,
    loginSourceDeveloper
  } from '@/api/source-developer'

  const router = useRouter()
  const route = useRoute()
  const systemConfigStore = useSystemConfigStore()
  const { siteName } = storeToRefs(systemConfigStore)
  const loading = ref(false)
  const loginFormRef = ref<FormInstance>()
  const approvedNotice = ref(route.query.approved === '1')

  const loginForm = reactive({
    username: typeof route.query.username === 'string' ? route.query.username : '',
    password: ''
  })

  const loginRules: FormRules = {
    username: [{ required: true, message: '请输入开发者用户名', trigger: 'blur' }],
    password: [{ required: true, message: '请输入开发者登录密码', trigger: 'blur' }]
  }

  function goApply() {
    router.push('/agent-panel/become-developer')
  }

  function handleLogin() {
    loginFormRef.value?.validate(async (valid: boolean) => {
      if (!valid) return
      loading.value = true
      try {
        const { data } = await loginSourceDeveloper(
          loginForm.username.trim().toLowerCase(),
          loginForm.password
        )
        if (data.code === 200 && data.data?.token) {
          localStorage.setItem(DEVELOPER_TOKEN_KEY, data.data.token)
          localStorage.setItem(
            DEVELOPER_INFO_KEY,
            JSON.stringify({
              username: data.data.username,
              displayName: data.data.displayName
            })
          )
          ElMessage.success('登录成功')
          const redirect =
            typeof route.query.redirect === 'string' &&
            route.query.redirect.startsWith('/developer-panel')
              ? route.query.redirect
              : '/developer-panel/dashboard'
          router.push(redirect)
          return
        }
        ElMessage.error(data.msg || '登录失败')
      } catch {
        ElMessage.error('请求失败，请重试')
      } finally {
        loading.value = false
      }
    })
  }

  onMounted(() => {
    if (approvedNotice.value) return
    if (!localStorage.getItem(DEVELOPER_TOKEN_KEY)) return
    const redirect =
      typeof route.query.redirect === 'string' &&
      route.query.redirect.startsWith('/developer-panel')
        ? route.query.redirect
        : '/developer-panel/dashboard'
    router.replace(redirect)
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

    .login-form {
      :deep(.el-input__wrapper) {
        min-height: 46px;
        background: var(--el-fill-color-lighter);
        border-radius: 12px;
        box-shadow: 0 0 0 1px var(--el-border-color-lighter) inset;
      }

      :deep(.el-input__wrapper.is-focus) {
        background: var(--el-bg-color);
        box-shadow: 0 0 0 1px var(--el-color-primary) inset;
      }

      .login-btn {
        width: 100%;
        height: 46px;
        font-size: 16px;
        font-weight: 600;
        border-radius: 12px;
      }
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
