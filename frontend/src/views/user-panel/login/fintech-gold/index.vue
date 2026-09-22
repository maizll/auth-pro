<template>
  <div class="gold-home" :style="themeStyle">
    <header class="site-header">
      <div class="container header-inner">
        <a class="brand" href="#top" aria-label="返回首页">
          <span class="brand-mark" aria-hidden="true"
            ><span></span><span></span><span></span><span></span
          ></span>
          <span class="brand-name">{{ siteName }}</span>
        </a>
        <nav class="desktop-nav" aria-label="主导航">
          <a class="active" href="#top">首页</a>
          <a href="#query">快速查询</a>
          <a href="#services">平台能力</a>
          <a href="#guide">使用链路</a>
        </nav>
        <div class="header-actions">
          <button class="search-trigger" type="button" aria-label="搜索" @click="focusQuery">
            <AppIcon name="search" :size="16" /><span class="search-label">搜索</span>
          </button>
          <button class="login-link" type="button" data-testid="template-login" @click="openLogin">
            登录
          </button>
          <button class="button button-small" type="button" @click="openLogin">注册</button>
        </div>
        <button
          class="mobile-menu"
          type="button"
          :aria-label="menuOpen ? '关闭菜单' : '打开菜单'"
          :aria-expanded="menuOpen"
          aria-controls="mobile-nav"
          @click="menuOpen = !menuOpen"
        >
          <AppIcon :name="menuOpen ? 'x' : 'menu'" :size="24" />
        </button>
      </div>
      <nav
        v-show="menuOpen"
        id="mobile-nav"
        class="mobile-nav"
        aria-label="移动端主导航"
        @keydown.esc="menuOpen = false"
      >
        <a href="#top" @click="menuOpen = false">首页</a>
        <a href="#query" @click="menuOpen = false">快速查询</a>
        <a href="#services" @click="menuOpen = false">平台能力</a>
        <a href="#guide" @click="menuOpen = false">使用链路</a>
      </nav>
    </header>

    <main id="top">
      <section class="hero-section">
        <div class="hero-grid container">
          <div class="hero-copy">
            <p class="eyebrow"
              ><span class="eyebrow-line" aria-hidden="true"></span>{{ template.hero.badge }}</p
            >
            <h1
              >{{ template.hero.title }}<strong>{{ template.hero.highlight }}</strong></h1
            >
            <p class="hero-description">{{ template.hero.description || siteSubtitle }}</p>
            <div class="hero-actions">
              <button class="button button-primary" type="button" @click="openLogin">
                {{ template.hero.primaryAction?.label || '进入用户中心' }}
                <AppIcon name="arrow-right" class="icon-arrow" />
              </button>
              <button class="button button-outline" type="button" @click="focusQuery">
                <AppIcon name="search" />快速查询服务
              </button>
            </div>
            <div class="trust-row">
              <span><AppIcon name="circle-check" :size="14" />授权状态实时同步</span>
              <span><AppIcon name="shield-check" :size="14" />账户信息安全保障</span>
              <span><AppIcon name="headphones" :size="14" />7×24 小时在线服务</span>
            </div>
          </div>

          <div class="hero-visual" aria-label="授权数据概览（示例）">
            <div class="visual-grid" aria-hidden="true"></div>
            <span class="orbit orbit-one" aria-hidden="true"></span>
            <span class="orbit orbit-two" aria-hidden="true"></span>
            <span class="spark spark-one" aria-hidden="true"></span>
            <span class="spark spark-two" aria-hidden="true"></span>
            <span class="spark spark-three" aria-hidden="true"></span>
            <div class="cube cube-large" aria-hidden="true"><span></span></div>
            <div class="cube cube-small" aria-hidden="true"><span></span></div>
            <GlassCard as="div" class="license-window">
              <div class="window-bar">
                <span class="window-dots" aria-hidden="true"><i></i><i></i><i></i></span>
                <small>授权管理中心 <span class="preview-tag">示例</span></small>
                <AppIcon name="lock-keyhole" :size="13" />
              </div>
              <div class="license-title">
                <span class="mini-shield"><AppIcon name="shield-check" :size="22" /></span>
                <div><b>软件授权</b><small>LICENSE CONTROL</small></div>
                <span class="status-pill"><AppIcon name="check" :size="12" />启用</span>
              </div>
              <div class="license-rule"></div>
              <div class="license-details">
                <div><small>授权类型</small><b>专业版</b></div>
                <div><small>有效状态</small><b class="gold-text">正常使用</b></div>
              </div>
              <div class="license-details">
                <div><small>授权用户</small><b>账户持有人</b></div>
                <div><small>有效期限</small><b>2025.12.31</b></div>
              </div>
              <div class="license-footer">
                <span><AppIcon name="refresh-cw" :size="12" />最后同步：刚刚</span>
                <button class="text-link" type="button" @click="openLogin">
                  授权详情<AppIcon name="arrow-up-right" class="icon-arrow" :size="14" />
                </button>
              </div>
            </GlassCard>
            <div class="visual-corner" aria-hidden="true">SECURE<br /><b>ACCESS</b></div>
          </div>
        </div>
      </section>

      <section id="query" class="query-section">
        <div class="container">
          <div class="section-heading query-heading">
            <div><span class="section-kicker">QUERY CENTER</span><h2>快速查询服务</h2></div>
            <div class="query-tabs" role="group" aria-label="查询类型">
              <button class="selected" type="button" aria-current="true" @click="focusQuery">
                <AppIcon name="key-round" :size="15" />授权查询
              </button>
              <button type="button" @click="openLogin"
                ><AppIcon name="file-key" :size="15" />许可证查询</button
              >
              <button type="button" @click="openLogin"
                ><AppIcon name="globe" :size="15" />域名 / IP 查询</button
              >
            </div>
          </div>
          <GlassCard as="form" class="query-box" :interactive="false" @submit.prevent="handleQuery">
            <AppIcon name="search" :size="20" />
            <label class="sr-only" for="query-input">用户账号、许可证 ID 或授权码</label>
            <input
              id="query-input"
              ref="queryInput"
              v-model="queryValue"
              type="text"
              placeholder="请输入用户账号 / 许可证 ID / 授权码"
              :aria-describedby="queryMessage ? 'query-message' : undefined"
              :aria-invalid="queryMessage && !queryValue.trim() ? true : undefined"
            />
            <button class="button button-primary" type="submit">
              立即查询<AppIcon name="arrow-right" class="icon-arrow" :size="16" />
            </button>
          </GlassCard>
          <p v-if="queryMessage" id="query-message" class="query-message" role="status">{{
            queryMessage
          }}</p>
        </div>
      </section>

      <section id="services" class="services-section">
        <div class="container">
          <div class="section-heading">
            <div
              ><span class="section-kicker">PLATFORM SERVICES</span
              ><h2>围绕软件授权打造的一站式服务</h2></div
            >
            <p>把授权生命周期放在一个清晰、可靠的服务入口中</p>
          </div>
          <div class="service-grid">
            <GlassCard
              v-for="(feature, index) in features"
              :key="feature.title"
              class="service-card"
            >
              <div class="card-top">
                <span class="service-icon"
                  ><AppIcon :name="feature.icon || 'shield-check'" :size="24"
                /></span>
                <span class="card-index" aria-hidden="true">0{{ index + 1 }}</span>
              </div>
              <h3>{{ feature.title }}</h3>
              <p>{{ feature.description }}</p>
              <a
                class="text-link"
                href="#query"
                :aria-label="`详细了解${feature.title}`"
                @click="queryMessage = '请输入授权信息开始查询'"
              >
                详细了解<AppIcon name="arrow-up-right" class="icon-arrow" :size="17" />
              </a>
            </GlassCard>
          </div>
        </div>
      </section>

      <section id="guide" class="guide-section">
        <div class="container guide-grid">
          <div class="guide-copy">
            <span class="section-kicker">HOW IT WORKS</span>
            <h2>三步开启授权服务</h2>
            <p>从账户注册到授权管理，把复杂流程收纳进清晰的操作路径。</p>
            <div class="guide-line" aria-hidden="true"></div>
          </div>
          <ol class="steps">
            <GlassCard v-for="(step, index) in steps" :key="step.title" as="li" class="step">
              <span class="step-number" aria-hidden="true">0{{ index + 1 }}</span>
              <span class="step-icon"><AppIcon :name="step.icon" :size="20" /></span>
              <div
                ><h3>{{ step.title }}</h3
                ><p>{{ step.description }}</p></div
              >
            </GlassCard>
          </ol>
        </div>
      </section>

      <section class="cta-section">
        <GlassCard as="div" class="container cta-panel">
          <div>
            <span class="section-kicker">GET STARTED</span>
            <h2>开始管理您的<span>软件授权</span></h2>
            <p>连接用户中心，集中管理授权状态，有效对接您的业务。</p>
          </div>
          <div class="cta-actions">
            <button class="button button-primary" type="button" @click="openLogin">
              立即授权<AppIcon name="arrow-right" class="icon-arrow" :size="17" />
            </button>
            <button class="button button-outline" type="button" @click="openLogin">马上登录</button>
          </div>
        </GlassCard>
      </section>
    </main>

    <footer class="site-footer">
      <div class="container footer-inner">
        <div class="brand footer-brand">
          <span class="brand-mark" aria-hidden="true"
            ><span></span><span></span><span></span><span></span
          ></span>
          <span class="brand-name">{{ siteName }}</span>
        </div>
        <p>{{ template.footer?.text || '安全 · 稳定 · 可验证的软件授权服务' }}</p>
        <span>© {{ currentYear }} {{ siteName }}</span>
      </div>
    </footer>

    <dialog
      ref="loginDialog"
      class="login-dialog remote-login-dialog remote-login-dialog--fintech-gold"
      :style="themeStyle"
      aria-labelledby="login-title"
      aria-describedby="login-description"
      @click.self="closeLogin"
    >
      <GlassCard as="section" class="login-modal" :interactive="false">
        <button class="modal-close" type="button" aria-label="关闭登录窗口" @click="closeLogin"
          ><AppIcon name="x" :size="20"
        /></button>
        <div class="modal-brand">
          <span class="brand-mark" aria-hidden="true"
            ><span></span><span></span><span></span><span></span
          ></span>
          <span>{{ siteName }}</span>
        </div>
        <span class="section-kicker">ACCOUNT ACCESS</span>
        <h2 id="login-title" data-testid="login-host-hint">登录用户中心</h2>
        <p id="login-description">
          {{
            preview
              ? '示意：启用后由宿主打开。外观跟随 stylePreset 与 theme.primaryColor、backgroundColor、textColor。模板不提交密码。'
              : '登录后查看并管理您的授权'
          }}
        </p>
        <form :aria-busy="loginLoading" @submit.prevent="handleLogin">
          <label for="login-account">账号</label>
          <div class="input-field">
            <AppIcon name="user-round" :size="17" />
            <input
              id="login-account"
              v-model.trim="loginForm.account"
              required
              autofocus
              autocomplete="username"
              type="text"
              placeholder="手机号 / 邮箱 / 用户ID"
            />
          </div>
          <label for="login-password">密码</label>
          <div class="input-field">
            <AppIcon name="lock-keyhole" :size="17" />
            <input
              id="login-password"
              v-model="loginForm.password"
              required
              autocomplete="current-password"
              type="password"
              placeholder="登录密码"
            />
          </div>
          <button class="button button-primary login-submit" type="submit" :disabled="loginLoading">
            <AppIcon v-if="loginLoading" name="loader-circle" class="icon-spin" />
            {{ loginLoading ? '登录中...' : '登录用户中心' }}
            <AppIcon v-if="!loginLoading" name="arrow-right" class="icon-arrow" />
          </button>
        </form>
        <p v-if="loginMessage" class="login-message" role="alert">{{ loginMessage }}</p>
      </GlassCard>
    </dialog>
  </div>
</template>

<script setup lang="ts">
  import { computed, reactive, ref } from 'vue'
  import AppIcon from './AppIcon.vue'
  import GlassCard from './GlassCard.vue'
  import { homeTemplateThemeStyle, type HomeTemplateDocument } from '../home-template'

  defineOptions({ name: 'FintechGoldHome' })
  const props = defineProps<{
    document: HomeTemplateDocument
    siteName: string
    siteSubtitle: string
    loginAction: (account: string, password: string) => Promise<void>
    preview?: boolean
  }>()
  const template = computed(() => props.document)
  const preview = computed(() => props.preview === true)
  const menuOpen = ref(false)
  const queryValue = ref('')
  const queryMessage = ref('')
  const queryInput = ref<HTMLInputElement | null>(null)
  const loginDialog = ref<HTMLDialogElement | null>(null)
  const loginLoading = ref(false)
  const loginMessage = ref('')
  const loginForm = reactive({ account: '', password: '' })
  const currentYear = new Date().getFullYear()
  const fallbackFeatures = [
    {
      icon: 'layers-3',
      title: '多类型授权',
      description: '支持单站授权、IP、域名等多种授权方式，满足不同业务场景。'
    },
    {
      icon: 'search-check',
      title: '授权状态查询',
      description: '快速查看授权状态与有效期限，让关键记录始终清晰可查。'
    },
    {
      icon: 'shopping-bag',
      title: '在线购买授权',
      description: '从授权开通到周期管理，提供稳定、直观的一站式服务体验。'
    }
  ]
  const features = computed(() =>
    Array.isArray(props.document.features) && props.document.features.length
      ? props.document.features.slice(0, 3)
      : fallbackFeatures
  )
  const themeStyle = computed(() => homeTemplateThemeStyle(props.document))
  const steps = [
    {
      icon: 'user-round-plus',
      title: '创建用户账号',
      description: '注册并登录用户中心，建立安全的账户与授权管理身份。'
    },
    {
      icon: 'layout-grid',
      title: '选择授权方案',
      description: '根据业务场景选择合适的授权类型与服务方案。'
    },
    {
      icon: 'rocket',
      title: '开始使用服务',
      description: '完成授权配置后，即可连接您的应用并持续管理。'
    }
  ]

  function openLogin() {
    loginMessage.value = ''
    if (!loginDialog.value?.open) loginDialog.value?.showModal()
  }
  function closeLogin() {
    loginDialog.value?.close()
  }
  function focusQuery() {
    queryInput.value?.focus({ preventScroll: true })
    window.document.getElementById('query')?.scrollIntoView({
      behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'
    })
  }
  function handleQuery() {
    if (!queryValue.value.trim()) {
      queryMessage.value = '请输入用户账号、许可证 ID 或授权码'
      queryInput.value?.focus({ preventScroll: true })
      return
    }
    queryMessage.value = '授权查询需要登录用户中心后继续'
    openLogin()
  }
  async function handleLogin() {
    if (preview.value) {
      loginMessage.value = '预览不提交密码。启用后由宿主按 stylePreset 与 theme 打开这个登录框。'
      return
    }
    if (loginLoading.value) return
    loginLoading.value = true
    loginMessage.value = ''
    try {
      // The host owns login, token persistence, agent conversion and navigation.
      await props.loginAction(loginForm.account.trim(), loginForm.password)
      closeLogin()
    } catch (error) {
      loginMessage.value = error instanceof Error ? error.message : '登录失败，请稍后重试'
    } finally {
      loginLoading.value = false
    }
  }
</script>

<style scoped>
  .gold-home {
    min-height: 100vh;
    --gold: #f0b90b;
    --page-bg: #0b0e11;
    --text: #f5f5f5;
    --motion-duration: 300ms;
    --motion-ease: cubic-bezier(0.4, 0, 0.2, 1);
    --icon-size: 18px;
    --icon-stroke-width: 1.75;
    --glass-blur: 20px;
    --card-radius: 20px;
    --card-hover-lift: -4px;
    --card-hover-scale: 1.01;
    --glow-radius: 320px;
    --shadow-opacity: 0.2;
    --shadow-hover-opacity: 0.32;
    --edge-highlight-opacity: 0.12;
  }

  /* Derived here so the API-provided theme colors also reach cards, icons, and the login dialog. */
  .gold-home,
  .login-dialog {
    --text-muted: color-mix(in srgb, var(--text) 66%, var(--page-bg));
    --text-subtle: color-mix(in srgb, var(--text) 54%, var(--page-bg));
    --line: color-mix(in srgb, var(--text) 10%, transparent);
    --section-bg: color-mix(in srgb, var(--page-bg) 98%, var(--text));
    --surface-solid: color-mix(in srgb, var(--page-bg) 94%, var(--text));
    --surface-glass: color-mix(in srgb, var(--surface-solid) 78%, transparent);
    --glass-border: color-mix(in srgb, var(--text) 15%, transparent);
    --glass-border-hover: color-mix(in srgb, var(--gold) 50%, transparent);
    --glass-overlay:
      linear-gradient(135deg, rgb(255 255 255 / 6%), transparent 45%),
      linear-gradient(165deg, transparent 65%, color-mix(in srgb, var(--gold) 3%, transparent));
    --glow-color: color-mix(in srgb, var(--gold) 14%, transparent);
    --gold-soft: color-mix(in srgb, var(--gold) 9%, transparent);
    --gold-border: color-mix(in srgb, var(--gold) 24%, transparent);
    --card-shadow:
      0 2px 4px rgb(0 0 0 / 8%), 0 12px 28px rgb(0 0 0 / var(--shadow-opacity)),
      inset 0 1px 0 rgb(255 255 255 / var(--edge-highlight-opacity)),
      inset 0 -1px 0 rgb(255 255 255 / 2%);
    --card-shadow-hover:
      0 4px 10px rgb(0 0 0 / 12%), 0 24px 48px rgb(0 0 0 / var(--shadow-hover-opacity)),
      0 0 28px color-mix(in srgb, var(--gold) 5%, transparent),
      inset 0 1px 0 rgb(255 255 255 / 20%), inset 0 -1px 0 rgb(255 255 255 / 3%);
  }

  @media (prefers-reduced-motion: reduce) {
    .gold-home {
      --motion-duration: 0ms;
    }
  }

  .gold-home {
    font-family: Inter, 'PingFang SC', 'Microsoft YaHei', Arial, sans-serif;
    color: var(--text);
    background: var(--page-bg);
    font-synthesis: none;
    text-rendering: optimizeLegibility;
    -webkit-font-smoothing: antialiased;
  }

  * {
    box-sizing: border-box;
  }
  .gold-home {
    scroll-behavior: smooth;
    scrollbar-gutter: stable;
  }
  .gold-home {
    margin: 0;
  }
  :global(body:has(.gold-home .login-dialog[open])) {
    overflow: hidden;
  }
  h1,
  h2,
  h3 {
    text-wrap: balance;
  }
  button,
  input {
    font: inherit;
  }
  button,
  a {
    -webkit-tap-highlight-color: transparent;
  }
  button {
    cursor: pointer;
  }
  a {
    color: inherit;
    text-decoration: none;
  }
  button:focus-visible,
  a:focus-visible,
  input:focus-visible {
    outline: 2px solid var(--gold);
    outline-offset: 4px;
  }
  section[id],
  main[id] {
    scroll-margin-top: 96px;
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
    border: 0;
  }

  .gold-home {
    min-width: 0;
    overflow: clip;
    color: var(--text);
    background: var(--page-bg);
  }
  .container {
    width: min(1240px, calc(100% - 80px));
    margin: 0 auto;
  }
  .site-header {
    position: sticky;
    z-index: 10;
    top: 0;
    background: color-mix(in srgb, var(--page-bg) 90%, transparent);
    border-bottom: 1px solid var(--line);
    -webkit-backdrop-filter: blur(20px);
    backdrop-filter: blur(20px);
  }
  .header-inner {
    display: flex;
    align-items: center;
    min-height: 76px;
    gap: 30px;
  }
  .brand {
    display: inline-flex;
    flex: 0 0 auto;
    align-items: center;
    gap: 11px;
  }
  .brand-name {
    color: var(--gold);
    font-size: 16px;
    font-weight: 700;
    letter-spacing: 0.04em;
    white-space: nowrap;
  }
  /* The existing geometric brand mark is intentionally retained. */
  .brand-mark {
    position: relative;
    display: inline-block;
    flex-shrink: 0;
    width: 24px;
    height: 24px;
    transform: rotate(45deg);
  }
  .brand-mark span {
    position: absolute;
    display: block;
    width: 8px;
    height: 8px;
    background: var(--gold);
  }
  .brand-mark span:nth-child(1) {
    top: 0;
    left: 8px;
  }
  .brand-mark span:nth-child(2) {
    top: 8px;
    left: 0;
  }
  .brand-mark span:nth-child(3) {
    top: 8px;
    right: 0;
  }
  .brand-mark span:nth-child(4) {
    bottom: 0;
    left: 8px;
  }
  .desktop-nav {
    display: flex;
    flex: 1;
    align-self: stretch;
    align-items: center;
    gap: 28px;
    margin-left: 18px;
  }
  .desktop-nav a {
    position: relative;
    display: inline-flex;
    align-items: center;
    height: 100%;
    color: var(--text-muted);
    font-size: 13px;
    white-space: nowrap;
    transition: color var(--motion-duration) var(--motion-ease);
  }
  .desktop-nav a:hover,
  .desktop-nav a.active {
    color: var(--text);
  }
  .desktop-nav a.active::after {
    position: absolute;
    right: 0;
    bottom: 0;
    left: 0;
    height: 2px;
    content: '';
    background: var(--gold);
  }
  .header-actions {
    display: flex;
    align-items: center;
    gap: 20px;
  }
  .search-trigger {
    display: flex;
    align-items: center;
    gap: 9px;
    width: 126px;
    height: 36px;
    padding: 0 12px;
    color: var(--text-muted);
    text-align: left;
    background: var(--surface-solid);
    border: 1px solid var(--glass-border);
    border-radius: 10px;
    transition:
      color var(--motion-duration),
      border-color var(--motion-duration);
  }
  .search-trigger:hover {
    color: var(--gold);
    border-color: var(--gold-border);
  }
  .search-label {
    font-size: 12px;
  }
  .login-link {
    padding: 0;
    color: var(--text-muted);
    background: transparent;
    border: 0;
    font-size: 13px;
  }
  .login-link:hover {
    color: var(--gold);
  }
  .mobile-menu,
  .mobile-nav {
    display: none;
  }

  .button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 10px;
    min-height: 46px;
    padding: 0 20px;
    color: #17130a;
    font-size: 14px;
    font-weight: 600;
    border: 1px solid transparent;
    border-radius: 11px;
    transition:
      transform var(--motion-duration) var(--motion-ease),
      background var(--motion-duration) var(--motion-ease),
      border-color var(--motion-duration) var(--motion-ease),
      box-shadow var(--motion-duration) var(--motion-ease),
      color var(--motion-duration) var(--motion-ease);
  }
  .button:disabled {
    cursor: wait;
    opacity: 0.65;
  }
  .button-small {
    min-height: 36px;
    padding: 0 16px;
    font-size: 13px;
    background: var(--gold);
  }
  .button-primary {
    background: linear-gradient(135deg, color-mix(in srgb, var(--gold) 86%, white), var(--gold));
    box-shadow:
      0 6px 20px var(--gold-soft),
      inset 0 1px 0 rgb(255 255 255 / 25%);
  }
  .button-primary:not(:disabled):hover {
    background: color-mix(in srgb, var(--gold) 87%, white);
    box-shadow: 0 10px 28px var(--gold-border);
  }
  .button-outline {
    color: var(--text);
    background: rgb(255 255 255 / 2%);
    border-color: var(--glass-border);
  }
  .button-outline:hover {
    color: var(--gold);
    border-color: var(--gold-border);
    background: var(--gold-soft);
  }
  .text-link {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 0;
    color: var(--gold);
    background: none;
    border: 0;
    font-size: 13px;
  }
  .text-link:hover {
    color: color-mix(in srgb, var(--gold) 80%, white);
  }

  .hero-section {
    position: relative;
    min-height: 590px;
    border-bottom: 1px solid var(--line);
    background:
      radial-gradient(circle at 17% 44%, var(--gold-soft), transparent 35%),
      linear-gradient(115deg, var(--section-bg), var(--page-bg) 62%);
  }
  .hero-section::before {
    position: absolute;
    inset: 0;
    pointer-events: none;
    content: '';
    opacity: 0.26;
    background-image:
      linear-gradient(var(--gold-soft) 1px, transparent 1px),
      linear-gradient(90deg, var(--gold-soft) 1px, transparent 1px);
    background-size: 48px 48px;
    mask-image: linear-gradient(90deg, #000, transparent 80%);
  }
  .hero-grid {
    position: relative;
    display: grid;
    grid-template-columns: minmax(0, 1.1fr) minmax(380px, 0.9fr);
    gap: 52px;
    align-items: center;
    min-height: 590px;
  }
  .hero-copy {
    padding: 48px 0 58px;
  }
  .eyebrow,
  .section-kicker {
    margin: 0;
    color: var(--gold);
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.16em;
  }
  .eyebrow {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .eyebrow-line {
    width: 26px;
    height: 1px;
    background: var(--gold);
  }
  .hero-copy h1 {
    max-width: 590px;
    margin: 24px 0 22px;
    font-size: clamp(38px, 4.1vw, 56px);
    line-height: 1.25;
    font-weight: 700;
    letter-spacing: -0.035em;
  }
  .hero-copy h1 strong {
    display: block;
    color: var(--gold);
    font-weight: 700;
  }
  .hero-description {
    max-width: 530px;
    margin: 0;
    color: var(--text-muted);
    font-size: 15px;
    line-height: 1.9;
  }
  .hero-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    margin-top: 32px;
  }
  .trust-row {
    display: flex;
    flex-wrap: wrap;
    gap: 17px;
    margin-top: 28px;
    color: var(--text-muted);
    font-size: 11px;
  }
  .trust-row > span {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .trust-row .app-icon {
    color: var(--gold);
  }
  .hero-visual {
    position: relative;
    min-height: 470px;
  }
  .hero-visual::before {
    position: absolute;
    inset: 12% -8% 6%;
    content: '';
    pointer-events: none;
    background: radial-gradient(ellipse, var(--gold-soft), transparent 65%);
  }
  .visual-grid {
    position: absolute;
    inset: 70px 0 40px 20px;
    opacity: 0.35;
    background-image:
      linear-gradient(var(--gold-border) 1px, transparent 1px),
      linear-gradient(90deg, var(--gold-border) 1px, transparent 1px);
    background-size: 42px 42px;
    transform: perspective(550px) rotateX(57deg) rotateZ(-16deg);
  }
  .orbit {
    position: absolute;
    display: block;
    border: 1px solid var(--gold-border);
    border-radius: 50%;
    transform: rotate(-27deg);
  }
  .orbit-one {
    top: 64px;
    right: 8px;
    width: 250px;
    height: 105px;
  }
  .orbit-two {
    top: 160px;
    right: 38px;
    width: 185px;
    height: 78px;
    opacity: 0.55;
  }
  .spark {
    position: absolute;
    width: 4px;
    height: 4px;
    background: var(--gold);
    border-radius: 50%;
    box-shadow: 0 0 14px 4px var(--gold-border);
  }
  .spark-one {
    top: 108px;
    right: 58px;
  }
  .spark-two {
    right: 13px;
    bottom: 102px;
  }
  .spark-three {
    bottom: 42px;
    left: 84px;
  }
  .cube {
    position: absolute;
    display: block;
    width: 50px;
    height: 50px;
    background: linear-gradient(135deg, #f5d981 0 48%, #a77a17 49% 100%);
    box-shadow: 16px 16px 0 var(--gold-soft);
    transform: rotate(30deg) skew(-9deg);
  }
  .cube::before,
  .cube::after {
    position: absolute;
    content: '';
    background: #d2a82b;
  }
  .cube::before {
    top: -18px;
    left: 9px;
    width: 50px;
    height: 19px;
    transform: skewX(-40deg);
  }
  .cube::after {
    top: -9px;
    right: -18px;
    width: 19px;
    height: 50px;
    transform: skewY(-41deg);
  }
  .cube span {
    position: absolute;
    inset: 10px;
    z-index: 1;
    border: 1px solid rgb(255 255 255 / 33%);
  }
  .cube-large {
    right: 10px;
    bottom: 35px;
    width: 72px;
    height: 72px;
    opacity: 0.65;
  }
  .cube-small {
    top: 32px;
    left: 30px;
    width: 45px;
    height: 45px;
    opacity: 0.3;
  }
  .license-window {
    --card-radius: 18px;
    --card-rest-transform: perspective(1000px) rotateY(-7deg) rotateX(2deg);
    position: absolute;
    z-index: 2;
    top: 92px;
    right: 30px;
    width: min(390px, calc(100% - 40px));
    padding: 0 22px 20px;
  }
  .window-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: 44px;
    color: var(--text-subtle);
    border-bottom: 1px solid var(--line);
  }
  .window-bar small {
    font-size: 10px;
  }
  .window-bar > .app-icon {
    color: var(--gold);
  }
  .preview-tag {
    margin-left: 5px;
    padding: 2px 5px;
    border: 1px solid var(--line);
    border-radius: 4px;
    font-size: 9px;
  }
  .window-dots {
    display: flex;
    gap: 5px;
  }
  .window-dots i {
    width: 5px;
    height: 5px;
    background: #d4514e;
    border-radius: 50%;
  }
  .window-dots i:nth-child(2) {
    background: #ddb339;
  }
  .window-dots i:nth-child(3) {
    background: #5aad66;
  }
  .license-title {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 22px 0 20px;
  }
  .mini-shield,
  .service-icon,
  .step-icon {
    display: inline-flex;
    flex: 0 0 auto;
    align-items: center;
    justify-content: center;
    color: var(--gold);
    background: linear-gradient(140deg, var(--gold-soft), transparent);
    border: 1px solid var(--gold-border);
    box-shadow: inset 0 1px 0 rgb(255 255 255 / 7%);
    transition:
      border-color var(--motion-duration) var(--motion-ease),
      background var(--motion-duration) var(--motion-ease);
  }
  .mini-shield {
    width: 40px;
    height: 40px;
    border-radius: 12px;
  }
  .license-title b,
  .license-title small,
  .license-details small,
  .license-details b {
    display: block;
  }
  .license-title b {
    font-size: 14px;
    font-weight: 600;
  }
  .license-title small {
    margin-top: 5px;
    color: var(--text-subtle);
    font-size: 8px;
    letter-spacing: 0.13em;
  }
  .status-pill {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    margin-left: auto;
    padding: 5px 8px;
    color: #7cdaa1;
    background: rgb(62 172 107 / 12%);
    border: 1px solid rgb(62 172 107 / 18%);
    border-radius: 20px;
    font-size: 10px;
  }
  .license-rule {
    height: 1px;
    background: var(--line);
  }
  .license-details {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
    padding-top: 17px;
  }
  .license-details small {
    margin-bottom: 7px;
    color: var(--text-subtle);
    font-size: 10px;
  }
  .license-details b {
    font-size: 12px;
    font-weight: 500;
  }
  .gold-text {
    color: var(--gold);
  }
  .license-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    margin-top: 22px;
    padding-top: 15px;
    color: var(--text-subtle);
    border-top: 1px solid var(--line);
    font-size: 10px;
  }
  .license-footer > span {
    display: inline-flex;
    align-items: center;
    gap: 5px;
  }
  .license-footer .text-link {
    font-size: 10px;
  }
  .visual-corner {
    position: absolute;
    bottom: 8px;
    left: 17px;
    color: var(--text-subtle);
    opacity: 0.6;
    font-size: 9px;
    line-height: 1.65;
    letter-spacing: 0.16em;
  }
  .visual-corner b {
    font-weight: 500;
  }

  .query-section {
    padding: 42px 0 48px;
    background: var(--section-bg);
    border-bottom: 1px solid var(--line);
  }
  .section-heading {
    display: flex;
    align-items: end;
    justify-content: space-between;
    gap: 24px;
  }
  .section-heading h2 {
    margin: 10px 0 0;
    font-size: 25px;
    line-height: 1.45;
    font-weight: 600;
  }
  .section-heading > p {
    max-width: 330px;
    text-wrap: balance;
    margin: 0;
    color: var(--text-muted);
    font-size: 13px;
    line-height: 1.8;
    text-align: right;
  }
  .query-tabs {
    display: flex;
    gap: 8px;
    flex-shrink: 0;
  }
  .query-tabs button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 7px;
    min-height: 38px;
    padding: 0 12px;
    color: var(--text-muted);
    background: var(--surface-solid);
    border: 1px solid var(--line);
    border-radius: 10px;
    font-size: 12px;
    transition:
      color var(--motion-duration),
      border-color var(--motion-duration),
      background var(--motion-duration);
  }
  .query-tabs button:hover {
    color: var(--gold);
    border-color: var(--gold-border);
  }
  .query-tabs button.selected {
    color: var(--gold);
    border-color: var(--gold-border);
    background: var(--gold-soft);
  }
  .query-box {
    --card-radius: 16px;
    display: flex;
    align-items: center;
    gap: 14px;
    min-height: 70px;
    margin-top: 23px;
    padding: 10px 10px 10px 22px;
  }
  .query-box > .app-icon {
    color: var(--gold);
  }
  .query-box input {
    min-width: 0;
    min-height: 40px;
    flex: 1;
    color: var(--text);
    background: transparent;
    border: 0;
    outline: 0;
    font-size: 13px;
  }
  .query-box input:focus-visible {
    outline: 0;
  }
  input::placeholder {
    color: var(--text-subtle);
  }
  .query-box .button {
    min-width: 130px;
    min-height: 46px;
    font-size: 13px;
  }
  .query-message {
    margin: 13px 0 0;
    color: var(--gold);
    font-size: 12px;
    line-height: 1.7;
  }

  .services-section {
    padding: 70px 0 82px;
    background:
      radial-gradient(
        ellipse at 18% 0%,
        color-mix(in srgb, var(--gold) 4%, transparent),
        transparent 60%
      ),
      var(--page-bg);
  }
  .service-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 20px;
    margin-top: 32px;
  }
  .service-card {
    display: flex;
    flex-direction: column;
    min-height: 278px;
    padding: 28px;
  }
  .card-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .service-icon {
    width: 48px;
    height: 48px;
    border-radius: 14px;
  }
  .card-index {
    color: var(--text-subtle);
    opacity: 0.7;
    font-size: 12px;
    font-family: 'SFMono-Regular', Consolas, monospace;
    letter-spacing: 0.1em;
  }
  .service-card h3 {
    margin: 26px 0 10px;
    font-size: 18px;
    font-weight: 600;
  }
  .service-card p {
    flex: 1;
    min-height: 50px;
    margin: 0;
    color: var(--text-muted);
    font-size: 13px;
    line-height: 1.9;
  }
  .service-card .text-link {
    justify-content: space-between;
    width: 100%;
    margin-top: 24px;
    padding-top: 18px;
    border-top: 1px solid var(--line);
  }

  .guide-section {
    padding: 64px 0 50px;
    background: var(--section-bg);
    border-top: 1px solid var(--line);
  }
  .guide-grid {
    display: grid;
    grid-template-columns: 0.78fr 1.22fr;
    gap: 72px;
  }
  .guide-copy h2 {
    margin: 12px 0 14px;
    font-size: 30px;
    font-weight: 600;
  }
  .guide-copy p {
    max-width: 290px;
    margin: 0;
    color: var(--text-muted);
    font-size: 13px;
    line-height: 1.9;
  }
  .guide-line {
    width: 100%;
    max-width: 230px;
    height: 1px;
    margin-top: 30px;
    background: linear-gradient(90deg, var(--gold-border), transparent);
  }
  .steps {
    display: grid;
    gap: 14px;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .step {
    --card-radius: 15px;
    display: grid;
    grid-template-columns: 28px 40px 1fr;
    align-items: center;
    gap: 16px;
    min-height: 94px;
    padding: 18px 20px;
  }
  .step-number {
    color: var(--gold);
    opacity: 0.7;
    font-size: 12px;
    font-family: 'SFMono-Regular', Consolas, monospace;
  }
  .step-icon {
    width: 40px;
    height: 40px;
    border-radius: 11px;
  }
  .step h3 {
    margin: 0 0 5px;
    font-size: 14px;
    font-weight: 600;
  }
  .step p {
    margin: 0;
    color: var(--text-muted);
    font-size: 12px;
    line-height: 1.75;
  }
  .cta-section {
    padding: 18px 0 74px;
    background: var(--section-bg);
  }
  .cta-panel {
    --glass-overlay:
      radial-gradient(ellipse at 5% 0%, var(--gold-soft), transparent 70%),
      linear-gradient(135deg, rgb(255 255 255 / 4%), transparent 60%);
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 30px;
    padding: 36px;
  }
  .cta-panel h2 {
    margin: 10px 0 10px;
    font-size: 27px;
    font-weight: 600;
    line-height: 1.4;
  }
  .cta-panel h2 span {
    color: var(--gold);
  }
  .cta-panel p {
    margin: 0;
    color: var(--text-muted);
    font-size: 13px;
    line-height: 1.8;
  }
  .cta-actions {
    display: flex;
    gap: 12px;
    flex: 0 0 auto;
  }
  .site-footer {
    padding: 26px 0;
    color: var(--text-subtle);
    background: var(--page-bg);
    border-top: 1px solid var(--line);
    font-size: 11px;
  }
  .footer-inner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 24px;
  }
  .footer-brand .brand-name {
    font-size: 13px;
  }
  .footer-brand .brand-mark {
    scale: 0.8;
  }
  .footer-inner p {
    margin: 0;
    line-height: 1.8;
  }

  .login-dialog {
    width: min(440px, calc(100% - 32px));
    max-height: calc(100dvh - 32px);
    padding: 0;
    overflow: visible;
    color: var(--text);
    background: transparent;
    border: 0;
    border-radius: var(--card-radius);
  }
  .login-dialog::backdrop {
    background: rgb(0 0 0 / 68%);
    -webkit-backdrop-filter: blur(8px);
    backdrop-filter: blur(8px);
  }
  .login-modal {
    --surface-glass: color-mix(in srgb, var(--surface-solid) 94%, transparent);
    max-height: calc(100dvh - 32px);
    padding: 34px;
    overflow-y: auto;
  }
  .modal-close {
    position: absolute;
    top: 14px;
    right: 14px;
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    padding: 0;
    color: var(--text-muted);
    background: transparent;
    border: 0;
    border-radius: 10px;
  }
  .modal-close:hover {
    color: var(--text);
    background: var(--line);
  }
  .modal-brand {
    display: flex;
    align-items: center;
    gap: 11px;
    margin-bottom: 30px;
    color: var(--gold);
    font-size: 15px;
    font-weight: 600;
  }
  .login-modal h2 {
    margin: 12px 0 9px;
    font-size: 25px;
    font-weight: 600;
  }
  .login-modal > p {
    margin: 0 0 26px;
    color: var(--text-muted);
    font-size: 13px;
  }
  .login-modal label {
    display: block;
    color: var(--text-muted);
    font-size: 12px;
  }
  .input-field {
    display: flex;
    align-items: center;
    gap: 10px;
    height: 48px;
    margin: 8px 0 20px;
    padding: 0 13px;
    color: var(--text-subtle);
    background: color-mix(in srgb, var(--page-bg) 70%, transparent);
    border: 1px solid var(--glass-border);
    border-radius: 11px;
  }
  .input-field:focus-within {
    color: var(--gold);
    border-color: var(--gold);
    box-shadow: 0 0 0 3px var(--gold-soft);
  }
  .input-field input {
    flex: 1;
    min-width: 0;
    width: 100%;
    height: 100%;
    padding: 0;
    color: var(--text);
    background: transparent;
    border: 0;
    outline: 0;
    font-size: 14px;
  }
  .input-field input:focus-visible {
    outline: 0;
  }
  .login-submit {
    width: 100%;
    min-height: 48px;
    margin-top: 4px;
  }
  .login-modal .login-message {
    margin: 18px 0 0;
    color: var(--gold);
    line-height: 1.7;
  }
  .icon-spin {
    animation: icon-spin 1s linear infinite;
  }
  @keyframes icon-spin {
    to {
      transform: rotate(360deg);
    }
  }

  @media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
    .button:not(:disabled):hover {
      transform: translateY(-2px);
    }
    .button:not(:disabled):hover .icon-arrow {
      transform: translateX(3px);
    }
    .text-link:hover .icon-arrow {
      transform: translate(2px, -2px);
    }
    .service-card:hover .service-icon,
    .step:hover .step-icon {
      border-color: var(--glass-border-hover);
      background: var(--gold-soft);
    }
    .service-card:hover .service-icon .app-icon,
    .step:hover .step-icon .app-icon {
      transform: rotate(-6deg) scale(1.08);
    }
    .search-trigger:hover .app-icon,
    .query-tabs button:hover .app-icon {
      transform: scale(1.1);
    }
    .modal-close:hover .app-icon {
      transform: rotate(90deg);
    }
  }

  @media (max-width: 1100px) {
    .header-inner {
      gap: 22px;
    }
    .desktop-nav {
      gap: 20px;
      margin-left: 0;
    }
    .header-actions {
      gap: 16px;
    }
    .search-trigger {
      width: 108px;
    }
  }

  @media (max-width: 900px) {
    .container {
      width: min(calc(100% - 40px), 680px);
    }
    .desktop-nav,
    .header-actions {
      display: none;
    }
    .mobile-menu {
      display: grid;
      place-items: center;
      width: 44px;
      height: 44px;
      margin-left: auto;
      padding: 0;
      color: var(--text);
      background: transparent;
      border: 1px solid var(--line);
      border-radius: 11px;
    }
    .mobile-nav {
      display: flex;
      flex-direction: column;
      padding: 8px 20px 15px;
      border-top: 1px solid var(--line);
    }
    .mobile-nav a {
      padding: 12px 0;
      color: var(--text-muted);
      font-size: 13px;
    }
    .hero-grid {
      grid-template-columns: 1fr;
      gap: 0;
    }
    .hero-section,
    .hero-grid {
      min-height: 0;
    }
    .hero-copy {
      padding: 62px 0 18px;
    }
    .hero-visual {
      width: 100%;
      max-width: 520px;
      min-height: 445px;
      margin: 0 auto;
    }
    .service-grid {
      grid-template-columns: 1fr;
    }
    .service-card {
      min-height: 250px;
      padding: 26px;
    }
    .service-card p {
      min-height: 0;
    }
    .guide-grid {
      grid-template-columns: 1fr;
      gap: 35px;
    }
    .guide-copy p {
      max-width: 430px;
    }
    .cta-panel {
      align-items: flex-start;
      flex-direction: column;
    }
    .footer-inner {
      align-items: flex-start;
      flex-direction: column;
      gap: 13px;
    }
  }

  @media (max-width: 640px) {
    .section-heading {
      align-items: flex-start;
      flex-direction: column;
    }
    .section-heading > p {
      max-width: none;
      text-align: left;
    }
    .query-tabs {
      flex-wrap: wrap;
      gap: 6px;
    }
  }

  @media (max-width: 560px) {
    .container {
      width: calc(100% - 32px);
    }
    .header-inner {
      min-height: 70px;
    }
    .hero-copy h1 {
      font-size: clamp(30px, 8vw, 38px);
    }
    .hero-description {
      font-size: 13px;
    }
    .hero-actions {
      flex-direction: column;
      align-items: stretch;
    }
    .hero-actions .button {
      width: 100%;
      min-height: 48px;
    }
    .trust-row {
      flex-direction: column;
      gap: 10px;
    }
    .hero-visual {
      min-height: 398px;
    }
    .license-window {
      top: 62px;
      right: 9px;
      width: calc(100% - 22px);
      padding-right: 16px;
      padding-left: 16px;
    }
    .cube-large {
      right: 2px;
      bottom: 15px;
    }
    .cube-small {
      top: 26px;
      left: 2px;
    }
    .visual-grid {
      inset: 44px 0 17px;
    }
    .visual-corner {
      bottom: 0;
    }
    .query-section,
    .services-section,
    .guide-section {
      padding-top: 44px;
      padding-bottom: 48px;
    }
    .section-heading h2 {
      font-size: 22px;
    }
    .query-tabs button {
      min-height: 40px;
      padding: 0 10px;
      font-size: 11px;
    }
    .query-box {
      flex-wrap: wrap;
      gap: 10px;
      padding: 10px 12px;
    }
    .query-box input {
      font-size: 12px;
    }
    .query-box .button {
      width: 100%;
    }
    .service-grid {
      gap: 16px;
      margin-top: 26px;
    }
    .service-card {
      padding: 24px;
    }
    .guide-copy h2 {
      font-size: 27px;
    }
    .step {
      grid-template-columns: 22px 34px 1fr;
      gap: 12px;
      padding: 16px;
    }
    .step-icon {
      width: 34px;
      height: 34px;
      border-radius: 10px;
    }
    .step p {
      font-size: 11px;
    }
    .cta-section {
      padding-bottom: 48px;
    }
    .cta-panel {
      padding: 26px 22px;
    }
    .cta-panel h2 {
      font-size: 24px;
    }
    .cta-actions {
      width: 100%;
    }
    .cta-actions .button {
      flex: 1;
      padding: 0 10px;
      font-size: 13px;
    }
    .login-modal {
      padding: 30px 24px;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .gold-home {
      scroll-behavior: auto;
    }
    .icon-spin {
      animation: none;
    }
  }
</style>
