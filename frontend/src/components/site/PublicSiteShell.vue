<!-- 四个公开页面的外壳：顶栏、页脚和当前模板的颜色。正文由页面自己填。 -->
<template>
  <div class="site-shell" :class="shellClass" :style="themeStyle">
    <header class="site-shell__header">
      <div class="site-shell__bar">
        <RouterLink class="site-shell__brand" to="/user/login" :title="siteName">
          <img v-if="resolvedLogo" :src="resolvedLogo" alt="" />
          <strong>{{ siteName }}</strong>
        </RouterLink>
        <PublicSiteNav :items="items" />
        <div class="site-shell__actions">
          <RouterLink v-if="loggedIn" class="site-shell__login" to="/user/dashboard"
            >个人中心</RouterLink
          >
          <button v-else class="site-shell__login" type="button" @click="login">登录</button>
        </div>
      </div>
    </header>
    <main class="site-shell__main">
      <slot />
    </main>
    <footer class="site-shell__footer">
      <div class="site-shell__footer-links">
        <RouterLink v-for="item in footerLinks" :key="item.key" :to="item.href">
          {{ item.label }}
        </RouterLink>
      </div>
      <p>{{ footerText }}</p>
    </footer>
  </div>
</template>

<script setup lang="ts">
  import axios from 'axios'
  import { computed, onMounted, ref } from 'vue'
  import { useRouter } from 'vue-router'
  import { storeToRefs } from 'pinia'
  import PublicSiteNav from '@/components/site/PublicSiteNav.vue'
  import { useSystemConfigStore } from '@/store/modules/system-config'
  import {
    type HomeTemplateDocument,
    homeTemplateThemeStyle,
    isHomeTemplateDocument,
    resolveHomeTemplatePreset
  } from '@/views/user-panel/login/home-template'
  import { openSiteAuth, siteUserLoggedIn, usePublicNav } from '@/utils/public-site'

  defineOptions({ name: 'PublicSiteShell' })

  const router = useRouter()
  const { items } = usePublicNav()
  const systemConfigStore = useSystemConfigStore()
  const { siteName, resolvedLogo } = storeToRefs(systemConfigStore)
  const loggedIn = ref(false)
  const document = ref<HomeTemplateDocument | null>(null)

  const preset = computed(() => resolveHomeTemplatePreset(document.value?.stylePreset))
  const shellClass = computed(() => `site-shell--${preset.value}`)
  const themeStyle = computed(() =>
    homeTemplateThemeStyle({
      stylePreset: document.value?.stylePreset,
      theme: document.value?.theme
    })
  )
  const footerText = computed(
    () => document.value?.footer?.text || `© ${new Date().getFullYear()} ${siteName.value}`
  )
  const footerLinks = computed(() => items.value.filter((item) => !item.external).slice(0, 5))

  function login() {
    openSiteAuth('login', router)
  }

  onMounted(async () => {
    loggedIn.value = siteUserLoggedIn()
    try {
      const { data } = await axios.get('/api/home-template/active', { timeout: 5000 })
      const result = data?.data
      if (
        data?.code === 200 &&
        result &&
        !result.isDefault &&
        isHomeTemplateDocument(result.document)
      ) {
        document.value = result.document
      }
    } catch {
      document.value = null
    }
  })
</script>

<style scoped>
  .site-shell {
    min-height: 100vh;
    overflow-x: clip;
    color: var(--remote-text, #1c2740);
    background: var(--remote-background, #f4f7fd);
  }

  .site-shell__header {
    position: sticky;
    top: 0;
    z-index: 20;
    background: rgb(255 255 255 / 92%);
    border-bottom: 1px solid rgb(28 39 64 / 8%);
    backdrop-filter: blur(10px);
  }

  .site-shell--fintech-gold .site-shell__header {
    background: rgb(11 14 17 / 92%);
  }

  .site-shell__bar {
    display: flex;
    align-items: center;
    gap: 8px;
    width: min(1120px, calc(100% - 24px));
    height: 64px;
    margin: 0 auto;
  }

  .site-shell__brand {
    display: flex;
    flex: 0 1 auto;
    align-items: center;
    min-width: 0;
    color: inherit;
    text-decoration: none;
    gap: 8px;
  }

  .site-shell__brand img {
    width: 28px;
    height: 28px;
    object-fit: contain;
  }

  .site-shell__brand strong {
    overflow: hidden;
    font-size: 15px;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .site-shell__actions {
    flex: 0 0 auto;
  }

  .site-shell__login {
    display: inline-flex;
    align-items: center;
    height: 32px;
    padding: 0 12px;
    color: #fff;
    font-size: 13px;
    line-height: 32px;
    white-space: nowrap;
    text-decoration: none;
    background: var(--remote-primary, #2f6fed);
    border: 0;
    border-radius: 8px;
    cursor: pointer;
  }

  .site-shell__main {
    width: min(1120px, calc(100% - 24px));
    margin: 0 auto;
    padding: 28px 0 48px;
  }

  .site-shell__footer {
    padding: 28px 0 36px;
    color: rgb(28 39 64 / 72%);
    background: #fff;
    border-top: 1px solid rgb(28 39 64 / 6%);
  }

  .site-shell__footer-links,
  .site-shell__footer p {
    width: min(1120px, calc(100% - 24px));
    margin: 0 auto;
  }

  .site-shell__footer-links {
    display: flex;
    flex-wrap: nowrap;
    gap: 12px;
    overflow: hidden;
    margin-bottom: 8px;
  }

  .site-shell__footer-links a {
    flex: 0 1 auto;
    overflow: hidden;
    color: inherit;
    font-size: 13px;
    white-space: nowrap;
    text-decoration: none;
    text-overflow: ellipsis;
  }

  .site-shell__footer p {
    overflow: hidden;
    font-size: 13px;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
</style>
