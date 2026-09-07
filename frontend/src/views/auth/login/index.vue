<!-- 登录页面 -->
<template>
  <div class="auth-login-page">
    <div class="flex w-full h-screen items-center justify-center">
      <div class="relative">
        <div class="auth-right-wrap">
          <div class="form">
            <h3 class="title">{{ $t('login.title') }}</h3>
            <p class="sub-title">{{ $t('login.subTitle') }}</p>
            <ElForm
              ref="formRef"
              :model="formData"
              :rules="rules"
              :key="formKey"
              @keyup.enter="handleSubmit"
              style="margin-top: 25px"
            >
              <ElFormItem prop="username">
                <ElInput
                  class="custom-height"
                  :placeholder="$t('login.placeholder.username')"
                  v-model.trim="formData.username"
                />
              </ElFormItem>
              <ElFormItem prop="password">
                <ElInput
                  class="custom-height"
                  :placeholder="$t('login.placeholder.password')"
                  v-model.trim="formData.password"
                  type="password"
                  autocomplete="off"
                  show-password
                />
              </ElFormItem>

              <!-- 推拽验证（未启用极验时使用本地滑块） -->
              <div v-if="!captchaEnabled" class="relative pb-5 mt-6">
                <div
                  class="relative z-[2] overflow-hidden select-none rounded-lg border border-transparent tad-300"
                  :class="{ '!border-[#FF4E4F]': !isPassing && isClickPass }"
                >
                  <ArtDragVerify
                    ref="dragVerify"
                    v-model:value="isPassing"
                    :text="$t('login.sliderText')"
                    textColor="var(--art-gray-700)"
                    :successText="$t('login.sliderSuccessText')"
                    progressBarBg="var(--main-color)"
                    :background="isDark ? '#26272F' : '#F1F1F4'"
                    handlerBg="var(--default-box-color)"
                  />
                </div>
                <p
                  class="absolute top-0 z-[1] px-px mt-2 text-xs text-[#f56c6c] tad-300"
                  :class="{ 'translate-y-10': !isPassing && isClickPass }"
                >
                  {{ $t('login.placeholder.slider') }}
                </p>
              </div>

              <div class="flex-cb mt-2 text-sm">
                <ElCheckbox v-model="formData.rememberPassword">{{
                  $t('login.rememberPwd')
                }}</ElCheckbox>
                <RouterLink class="text-theme" :to="{ name: 'ForgetPassword' }">{{
                  $t('login.forgetPwd')
                }}</RouterLink>
              </div>

              <div style="margin-top: 30px">
                <ElButton
                  class="w-full custom-height"
                  type="primary"
                  @click="handleSubmit"
                  :loading="loading"
                  v-ripple
                >
                  {{ $t('login.btnText') }}
                </ElButton>
              </div>
            </ElForm>
          </div>
        </div>
      </div>
    </div>
    <footer v-if="icpNumber" class="auth-site-footer">
      <a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer">
        {{ icpNumber }}
      </a>
    </footer>
  </div>
</template>

<script setup lang="ts">
  import { useUserStore } from '@/store/modules/user'
  import { useSystemConfigStore } from '@/store/modules/system-config'
  import { useI18n } from 'vue-i18n'
  import { HttpError } from '@/utils/http/error'
  import { fetchLogin } from '@/api/auth'
  import { ElMessage, ElNotification, type FormInstance, type FormRules } from 'element-plus'
  import { useSettingStore } from '@/store/modules/setting'
  import { useGeetestLoginCaptcha } from '@/utils/geetest'

  defineOptions({ name: 'Login' })

  const systemConfigStore = useSystemConfigStore()
  const settingStore = useSettingStore()
  const { isDark } = storeToRefs(settingStore)
  const { siteName, icpNumber } = storeToRefs(systemConfigStore)
  const { t, locale } = useI18n()
  const formKey = ref(0)

  // 监听语言切换，重置表单
  watch(locale, () => {
    formKey.value++
  })

  const dragVerify = ref()

  // 极验行为验证：启用后替代本地滑块，点击登录时弹出验证
  const {
    captchaEnabled,
    preload: preloadCaptcha,
    verify: verifyCaptcha,
    reset: resetCaptcha
  } = useGeetestLoginCaptcha()
  onMounted(preloadCaptcha)

  const userStore = useUserStore()
  const router = useRouter()
  const route = useRoute()
  const isPassing = ref(false)
  const isClickPass = ref(false)

  const formRef = ref<FormInstance>()

  const formData = reactive({
    username: '',
    password: '',
    rememberPassword: true
  })

  const rules = computed<FormRules>(() => ({
    username: [{ required: true, message: t('login.placeholder.username'), trigger: 'blur' }],
    password: [{ required: true, message: t('login.placeholder.password'), trigger: 'blur' }]
  }))

  const loading = ref(false)

  const isAppStorePath = (path: string) =>
    path === '/admin/app-store' || path.startsWith('/admin/app-store/')

  const resolveLoginRedirect = () => {
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect.trim() : ''
    if (!redirect.startsWith('/') || redirect.startsWith('//')) return '/dashboard/console'

    const targetPath = redirect.split(/[?#]/, 1)[0]
    const isAppStoreRedirect = isAppStorePath(targetPath)
    if (
      targetPath === '/' ||
      targetPath === '/install' ||
      targetPath.startsWith('/install/') ||
      targetPath === '/admin' ||
      (targetPath.startsWith('/admin/') && !isAppStoreRedirect) ||
      targetPath === '/user' ||
      targetPath.startsWith('/user/') ||
      targetPath === '/agent-panel' ||
      targetPath.startsWith('/agent-panel/')
    ) {
      return '/dashboard/console'
    }

    return redirect
  }

  // 登录
  const handleSubmit = async () => {
    if (!formRef.value) return

    let loginAttempted = false
    let loginCompleted = false

    try {
      // 表单验证
      const valid = await formRef.value.validate()
      if (!valid) return

      // 拖拽验证（极验启用时改为提交前弹出极验滑块）
      let captchaParams = {}
      if (captchaEnabled.value) {
        let result
        try {
          result = await verifyCaptcha()
        } catch (error) {
          ElMessage.error(error instanceof Error ? error.message : '行为验证初始化失败')
          return
        }
        // null = 用户关闭或组件出错，中止提交
        if (!result) return
        captchaParams = result
      } else if (!isPassing.value) {
        isClickPass.value = true
        return
      }

      loginAttempted = true
      loading.value = true

      // 登录请求
      const { username, password } = formData

      const { token, refreshToken } = await fetchLogin({
        userName: username,
        password,
        ...captchaParams
      })

      // 验证token
      if (!token) {
        throw new Error('Login failed - no token received')
      }

      // 存储 token 和登录状态
      userStore.setToken(token, refreshToken)
      userStore.setLoginStatus(true)

      // 独立应用商店需要重新请求 Go 静态入口，不能交给主后台 Router 接管。
      const redirectTarget = resolveLoginRedirect()
      if (isAppStorePath(redirectTarget.split(/[?#]/, 1)[0])) {
        loginCompleted = true
        window.location.replace(redirectTarget)
        return
      }

      // 登录页不接受安装向导或外部地址作为回跳目标
      const navigationFailure = await router.replace(redirectTarget)
      if (navigationFailure) {
        throw navigationFailure
      }

      loginCompleted = true
      showLoginSuccessNotice()
    } catch (error) {
      // 处理 HttpError
      if (error instanceof HttpError) {
        // console.log(error.code)
      } else {
        // 处理非 HttpError
        // ElMessage.error('登录失败，请稍后重试')
        console.error('[Login] Unexpected error:', error)
      }
    } finally {
      loading.value = false
      if (loginAttempted && !loginCompleted) {
        resetDragVerify()
        resetCaptcha()
      }
    }
  }

  // 重置拖拽验证
  const resetDragVerify = () => {
    dragVerify.value?.reset()
    isClickPass.value = false
  }

  // 登录成功提示
  const showLoginSuccessNotice = () => {
    setTimeout(() => {
      ElNotification({
        title: t('login.success.title'),
        type: 'success',
        duration: 2500,
        zIndex: 10000,
        message: `${t('login.success.message')}, ${siteName.value}!`
      })
    }, 1000)
  }
</script>

<style scoped>
  @import './style.css';

  .auth-login-page {
    position: relative;
    min-height: 100vh;
  }

  .auth-site-footer {
    position: absolute;
    right: 0;
    bottom: 14px;
    left: 0;
    z-index: 2;
    font-size: 12px;
    text-align: center;
  }

  .auth-site-footer a {
    color: var(--art-gray-500);
    text-decoration: none;
  }

  .auth-site-footer a:hover {
    color: var(--main-color);
  }
</style>
