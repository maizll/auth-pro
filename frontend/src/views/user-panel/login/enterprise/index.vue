<!-- 企业蓝首页。文案来自 template.json，四个公开页面不在这里写。 -->
<template>
  <div class="enterprise-home" :style="themeStyle">
    <header class="bar">
      <div class="bar-inner">
        <button class="brand" type="button" @click="goHome">
          <img v-if="logo" :src="logo" alt="" />
          <strong>{{ siteName }}</strong>
        </button>
        <PublicSiteNav :items="items" />
        <div class="actions">
          <RouterLink v-if="loggedIn" class="login" to="/user/dashboard">个人中心</RouterLink>
          <button v-else class="login" type="button" @click="emit('login', 'login')">登录</button>
        </div>
      </div>
    </header>

    <section class="hero-band">
      <div class="hero">
        <div class="hero-copy">
          <p v-if="document.hero.badge" class="badge">{{ document.hero.badge }}</p>
          <h1>
            {{ document.hero.title }}
            <span v-if="document.hero.highlight">{{ document.hero.highlight }}</span>
          </h1>
          <p class="desc">{{ document.hero.description || siteSubtitle }}</p>
          <div class="hero-actions">
            <button class="primary" type="button" @click="emit('login', 'login')">
              {{ document.hero.primaryAction?.label || '进入用户中心' }}
            </button>
            <RouterLink v-if="secondaryPath" class="secondary" :to="secondaryPath">
              {{ document.hero.secondaryAction?.label || '授权购买' }}
            </RouterLink>
            <button
              v-else-if="document.hero.secondaryAction"
              class="secondary"
              type="button"
              @click="emit('login', 'register')"
            >
              {{ document.hero.secondaryAction.label || '注册' }}
            </button>
          </div>
        </div>
        <div class="hero-art" aria-hidden="true">
          <div class="panel">
            <div class="panel-top"><i /><i /><i /><span>授权服务</span></div>
            <div class="bars"><span /><span /><span /><span /><span /></div>
            <div class="chips"><em /><em /><em /></div>
          </div>
          <div class="float-card">域名授权</div>
        </div>
      </div>
    </section>

    <section v-if="features.length" class="features">
      <h2>开箱即用的能力</h2>
      <div class="feature-grid">
        <article v-for="feature in features" :key="feature.title">
          <span class="feature-icon" aria-hidden="true">
            <IconifyIcon :icon="feature.icon || 'ri:checkbox-circle-line'" />
          </span>
          <h3>{{ feature.title }}</h3>
          <p>{{ feature.description }}</p>
        </article>
      </div>
    </section>

    <section v-if="scenes.length" class="scenes">
      <h2>适用场景</h2>
      <div class="scene-grid">
        <article v-for="scene in scenes" :key="scene.title">
          <h3>{{ scene.title }}</h3>
          <p>{{ scene.description }}</p>
          <ul>
            <li v-for="point in scene.points || []" :key="point">{{ point }}</li>
          </ul>
        </article>
      </div>
    </section>

    <footer>
      <div class="footer-grid">
        <div class="footer-brand">
          <strong>{{ siteName }}</strong>
          <p>{{ document.footer?.text || `© ${year} ${siteName}` }}</p>
          <p v-if="footerAuthor">{{ footerAuthor }}</p>
          <p v-if="footerContact">{{ footerContact }}</p>
        </div>
        <nav class="footer-links" aria-label="页脚">
          <RouterLink v-for="item in footerLinks" :key="item.key" :to="item.href">
            {{ item.label }}
          </RouterLink>
        </nav>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, ref } from 'vue'
  import { useRouter } from 'vue-router'
  import { Icon as IconifyIcon } from '@iconify/vue'
  import PublicSiteNav from '@/components/site/PublicSiteNav.vue'
  import {
    type HomeTemplateDocument,
    homeTemplateThemeStyle,
    templateRoutePath
  } from '../home-template'
  import { siteUserLoggedIn, usePublicNav } from '@/utils/public-site'

  defineOptions({ name: 'EnterpriseHome' })

  const props = defineProps<{
    document: HomeTemplateDocument
    siteName: string
    siteSubtitle: string
    logo: string
  }>()
  const emit = defineEmits<{ login: [mode: 'login' | 'register'] }>()

  const router = useRouter()
  const { items } = usePublicNav()
  const loggedIn = ref(false)
  const year = new Date().getFullYear()
  const themeStyle = computed(() => homeTemplateThemeStyle(props.document))
  const features = computed(() => (props.document.features || []).slice(0, 12))
  const scenes = computed(() => props.document.scenes || [])
  const secondaryPath = computed(() => templateRoutePath(props.document.hero.secondaryAction))
  const footerLinks = computed(() => items.value.filter((item) => !item.external))
  const footerAuthor = computed(() => (props.document.footer?.author || '').trim())
  const footerContact = computed(() => (props.document.footer?.contact || '').trim())

  function goHome() {
    void router.push('/user/login')
  }

  onMounted(() => {
    loggedIn.value = siteUserLoggedIn()
  })
</script>

<style scoped>
  .enterprise-home {
    min-height: 100vh;
    overflow-x: clip;
    color: var(--remote-text);
    background: linear-gradient(180deg, #eef3ff 0%, var(--remote-background) 28%, #fff 100%);
  }

  .bar {
    position: sticky;
    top: 0;
    z-index: 20;
    background: rgb(255 255 255 / 92%);
    border-bottom: 1px solid rgb(28 39 64 / 6%);
  }

  .bar-inner,
  .hero,
  .features,
  .scenes,
  .footer-grid {
    width: min(1120px, calc(100% - 24px));
    margin: 0 auto;
  }

  .hero-band {
    background:
      radial-gradient(circle at 85% 20%, rgb(255 255 255 / 28%), transparent 26%),
      linear-gradient(120deg, #1d4ed8 0%, #2f6fed 46%, #60a5fa 100%);
    color: #fff;
  }

  .bar-inner {
    display: flex;
    align-items: center;
    gap: 8px;
    height: 64px;
  }

  .brand {
    display: flex;
    align-items: center;
    min-width: 0;
    padding: 0;
    color: inherit;
    background: transparent;
    border: 0;
    cursor: pointer;
    gap: 8px;
  }

  .brand img {
    width: 28px;
    height: 28px;
  }

  .brand strong {
    overflow: hidden;
    font-size: 15px;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .login {
    display: inline-flex;
    align-items: center;
    height: 32px;
    padding: 0 12px;
    color: #fff;
    font-size: 13px;
    white-space: nowrap;
    text-decoration: none;
    background: var(--remote-primary);
    border: 0;
    border-radius: 8px;
    cursor: pointer;
  }

  .hero {
    display: grid;
    grid-template-columns: minmax(0, 1.1fr) minmax(0, 0.9fr);
    gap: 32px;
    align-items: center;
    padding: 56px 0 28px;
  }

  .badge {
    display: inline-block;
    margin: 0 0 12px;
    padding: 4px 10px;
    color: #fff;
    font-size: 13px;
    white-space: nowrap;
    background: rgb(255 255 255 / 16%);
    border-radius: 999px;
  }

  h1 {
    margin: 0;
    font-size: 44px;
    line-height: 1.2;
    overflow-wrap: anywhere;
  }

  h1 span {
    display: block;
    color: #dbe7ff;
  }

  .desc {
    max-width: 36em;
    color: rgb(255 255 255 / 88%);
    font-size: 16px;
    line-height: 1.7;
  }

  .hero-actions {
    display: flex;
    flex-wrap: nowrap;
    gap: 10px;
  }

  .primary,
  .secondary {
    display: inline-flex;
    align-items: center;
    height: 42px;
    padding: 0 16px;
    white-space: nowrap;
    text-decoration: none;
    border-radius: 10px;
    cursor: pointer;
  }

  .primary {
    color: var(--remote-primary);
    background: #fff;
    border: 0;
  }

  .secondary {
    color: #fff;
    background: transparent;
    border: 1px solid rgb(255 255 255 / 70%);
  }

  .hero-art {
    position: relative;
    min-width: 0;
  }

  .panel {
    padding: 18px;
    background: #fff;
    border-radius: 20px;
    box-shadow: 0 20px 50px rgb(47 111 237 / 12%);
  }

  .panel-top,
  .bars,
  .chips {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .panel-top i,
  .chips em {
    display: block;
    width: 10px;
    height: 10px;
    background: #dbe6ff;
    border-radius: 50%;
  }

  .panel-top span {
    margin-left: auto;
    color: rgb(28 39 64 / 55%);
    font-size: 12px;
    white-space: nowrap;
  }

  .bars {
    height: 120px;
    margin-top: 18px;
    align-items: flex-end;
  }

  .bars span {
    flex: 1;
    background: linear-gradient(180deg, var(--remote-primary), #9db7ff);
    border-radius: 8px 8px 0 0;
  }

  .bars span:nth-child(1) {
    height: 40%;
  }
  .bars span:nth-child(2) {
    height: 70%;
  }
  .bars span:nth-child(3) {
    height: 55%;
  }
  .bars span:nth-child(4) {
    height: 86%;
  }
  .bars span:nth-child(5) {
    height: 62%;
  }

  .chips {
    margin-top: 14px;
  }

  .chips em {
    width: 48px;
    height: 14px;
    border-radius: 7px;
  }

  .float-card {
    position: absolute;
    right: 8px;
    bottom: -12px;
    padding: 8px 12px;
    color: var(--remote-primary);
    font-size: 13px;
    white-space: nowrap;
    background: #fff;
    border-radius: 10px;
    box-shadow: 0 8px 24px rgb(28 39 64 / 8%);
  }

  .features,
  .scenes {
    padding: 28px 0;
  }

  h2 {
    margin: 0 0 16px;
    font-size: 28px;
  }

  .feature-grid,
  .scene-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 14px;
  }

  .scene-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }

  .feature-grid article,
  .scene-grid article {
    min-width: 0;
    padding: 18px;
    background: #fff;
    border: 1px solid rgb(47 111 237 / 8%);
    border-radius: 16px;
    box-shadow: 0 10px 28px rgb(47 111 237 / 6%);
  }

  .feature-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    margin-bottom: 10px;
    color: var(--remote-primary);
    font-size: 20px;
    background: rgb(47 111 237 / 10%);
    border-radius: 10px;
  }

  h3 {
    margin: 0 0 8px;
    font-size: 16px;
    overflow-wrap: anywhere;
  }

  article p,
  article li {
    color: rgb(28 39 64 / 72%);
    line-height: 1.6;
  }

  article ul {
    margin: 8px 0 0;
    padding-left: 18px;
  }

  footer {
    padding: 28px 0 40px;
    background: #fff;
    border-top: 1px solid rgb(28 39 64 / 6%);
  }

  .footer-grid {
    display: grid;
    grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr);
    gap: 24px;
  }

  .footer-brand strong {
    display: block;
    margin-bottom: 8px;
    overflow-wrap: anywhere;
  }

  .footer-links {
    display: flex;
    flex-wrap: wrap;
    gap: 12px 16px;
    align-content: start;
    justify-content: flex-end;
  }

  .footer-links a,
  footer p {
    color: rgb(28 39 64 / 70%);
    font-size: 13px;
    line-height: 1.6;
    overflow-wrap: anywhere;
  }

  .footer-links a {
    color: inherit;
    text-decoration: none;
  }

  @media (max-width: 900px) {
    .hero,
    .feature-grid,
    .scene-grid,
    .footer-grid {
      grid-template-columns: 1fr;
    }

    .footer-links {
      justify-content: flex-start;
    }

    h1 {
      font-size: 32px;
    }

    .float-card {
      position: static;
      display: inline-block;
      margin-top: 12px;
    }
  }
</style>
