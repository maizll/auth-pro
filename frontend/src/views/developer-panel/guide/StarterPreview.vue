<template>
  <el-card shadow="never" class="starter-preview">
    <div class="preview-head">
      <div>
        <h2>标准示例预览</h2>
        <p>
          下面用入门包里的 <code>template.json</code>、<code>template.fintech-gold.json</code> 和
          <code>plugin.json</code> 直接渲染。不用先登记，也不是另一套字段。
        </p>
      </div>
      <el-radio-group v-model="preset" size="small" aria-label="模板外观">
        <el-radio-button value="cartoon-blue">cartoon-blue</el-radio-button>
        <el-radio-button value="fintech-gold">fintech-gold</el-radio-button>
      </el-radio-group>
    </div>

    <el-alert
      v-if="!activeDocument"
      type="error"
      :closable="false"
      show-icon
      title="标准模板示例不是合法的 schemaVersion 1 文档"
    />

    <section v-else class="preview-block" aria-label="模板预览" data-testid="starter-template-preview">
      <div class="preview-caption">
        <strong>{{ activeDocumentName }}</strong>
        <span>{{ activePreset }} · {{ activeTemplateId }} · v{{ activeTemplateVersion }}</span>
      </div>
      <div
        class="home-stage"
        :class="`home-stage--${activePreset}`"
        :style="themeStyle"
        data-testid="template-stage"
      >
        <header class="home-header">
          <div class="home-brand">
            <span class="brand-mark" aria-hidden="true"></span>
            <strong>{{ activeDocumentName }}</strong>
          </div>
          <button type="button" class="login-chip" data-testid="template-login" @click="loginHint = true">
            登录
          </button>
        </header>

        <div class="hero-grid">
          <div class="hero-copy">
            <span v-if="activeDocument.hero.badge" class="hero-badge">{{ activeDocument.hero.badge }}</span>
            <h3>
              {{ activeDocument.hero.title }}
              <span v-if="activeDocument.hero.highlight">{{ activeDocument.hero.highlight }}</span>
            </h3>
            <p>{{ activeDocument.hero.description }}</p>
            <div class="hero-actions">
              <button type="button" class="action action-primary" @click="loginHint = true">
                {{ activeDocument.hero.primaryAction?.label || '进入用户中心' }}
              </button>
              <button
                v-if="activeDocument.hero.secondaryAction"
                type="button"
                class="action action-secondary"
                @click="loginHint = true"
              >
                {{ activeDocument.hero.secondaryAction.label || '用户登录' }}
              </button>
            </div>
          </div>
          <div class="hero-art" aria-hidden="true">
            <IconifyIcon icon="ri:shield-check-line" />
          </div>
        </div>

        <div v-if="features.length" class="feature-grid">
          <article v-for="feature in features" :key="feature.title">
            <IconifyIcon :icon="safeTemplateIcon(feature.icon)" />
            <h4>{{ feature.title }}</h4>
            <p>{{ feature.description }}</p>
          </article>
        </div>

        <footer class="home-footer">{{ activeDocument.footer?.text }}</footer>

        <div
          v-if="loginHint"
          class="login-hint"
          role="dialog"
          aria-modal="true"
          aria-labelledby="starter-login-title"
          data-testid="login-host-hint"
        >
          <div class="login-hint-card">
            <p class="hint-kicker">宿主登录框</p>
            <h3 id="starter-login-title">用户登录</h3>
            <p>
              这是示意。启用模板后由宿主打开登录弹窗，圆角和颜色跟随
              <code>{{ activePreset }}</code>
              与 theme.primaryColor、backgroundColor、textColor。模板不提交密码，也不要写 login.html。
            </p>
            <label>
              账号
              <input type="text" disabled placeholder="手机号 / 邮箱 / 用户ID" />
            </label>
            <label>
              密码
              <input type="password" disabled placeholder="登录密码" />
            </label>
            <button type="button" class="action action-primary" disabled>登录用户中心</button>
            <button type="button" class="hint-close" @click="loginHint = false">知道了</button>
          </div>
        </div>
      </div>
      <p v-if="activePreset === 'fintech-gold'" class="host-note">
        查询区、使用步骤和底部按钮由宿主按 fintech-gold 写死，template.json 里没有这些字段。
      </p>
    </section>

    <section class="preview-block" aria-label="插件预览" data-testid="starter-plugin-preview">
      <div class="preview-caption">
        <strong>商店里的插件卡片</strong>
        <span>{{ pluginExample.id }} · {{ categoryLabel }}</span>
      </div>
      <div class="store-frame">
        <div class="store-tabs" aria-hidden="true">
          <span>全部插件</span>
          <span class="is-active">{{ categoryLabel }}</span>
        </div>
        <article class="store-card">
          <div class="store-icon">
            <IconifyIcon :icon="safeTemplateIcon(pluginExample.icon)" />
          </div>
          <div class="store-meta">
            <div class="store-name">
              <strong>{{ pluginExample.name }}</strong>
              <span class="pill">{{ categoryLabel }}</span>
              <span class="pill">v{{ pluginExample.version }}</span>
            </div>
            <p>{{ pluginExample.description }}</p>
            <p class="store-author">作者：{{ pluginExample.author?.name || '未提供' }} · 标识 {{ pluginExample.id }}</p>
          </div>
          <div class="store-status">
            <span class="pill pill-warn">未安装</span>
          </div>
        </article>
      </div>
    </section>
  </el-card>
</template>

<script setup lang="ts">
  import { computed, ref } from 'vue'
  import { Icon as IconifyIcon } from '@iconify/vue'
  import cartoonTemplate from '@developer-docs/starter/template-example/template.json'
  import goldTemplate from '@developer-docs/starter/template-example/template.fintech-gold.json'
  import pluginExample from '@developer-docs/starter/plugin-example/plugin.json'
  import {
    type HomeTemplateDocument,
    type HomeTemplateStylePreset,
    homeTemplateThemeStyle,
    isHomeTemplateDocument,
    safeTemplateIcon
  } from '@/views/user-panel/login/home-template'

  defineOptions({ name: 'DeveloperStarterPreview' })

  const categoryLabels: Record<string, string> = {
    other: '其他',
    payment: '支付',
    realname: '实名认证',
    'home-template': '首页模板'
  }

  const preset = ref<HomeTemplateStylePreset>('cartoon-blue')
  const loginHint = ref(false)

  const documents: Record<HomeTemplateStylePreset, HomeTemplateDocument | null> = {
    'cartoon-blue': isHomeTemplateDocument(cartoonTemplate) ? cartoonTemplate : null,
    'fintech-gold': isHomeTemplateDocument(goldTemplate) ? goldTemplate : null
  }

  const activeDocument = computed(() => documents[preset.value])
  const activePreset = computed(() => preset.value)
  const themeStyle = computed(() =>
    activeDocument.value ? homeTemplateThemeStyle(activeDocument.value) : {}
  )
  const features = computed(() => (activeDocument.value?.features || []).slice(0, 3))
  const activeTemplateRecord = computed(() =>
    preset.value === 'fintech-gold' ? goldTemplate : cartoonTemplate
  )
  const activeDocumentName = computed(
    () => activeTemplateRecord.value.name || activeDocument.value?.hero.title || '模板'
  )
  const activeTemplateId = computed(() => activeTemplateRecord.value.id)
  const activeTemplateVersion = computed(() => activeTemplateRecord.value.version)
  const categoryLabel = computed(
    () => categoryLabels[pluginExample.category] || pluginExample.category || '其他'
  )
</script>

<style scoped lang="scss">
  .starter-preview {
    margin-bottom: 16px;
  }

  .preview-head {
    display: grid;
    gap: 12px;
    margin-bottom: 16px;

    h2 {
      margin: 0;
      font-size: 18px;
      color: var(--el-text-color-primary);
    }

    p {
      max-width: 640px;
      margin: 6px 0 0;
      font-size: 13px;
      line-height: 1.6;
      color: var(--el-text-color-regular);
    }

    code {
      font-family: 'Roboto Mono', monospace;
      font-size: 0.92em;
    }
  }

  .preview-block + .preview-block {
    margin-top: 18px;
  }

  .preview-caption {
    display: flex;
    flex-wrap: wrap;
    gap: 8px 12px;
    align-items: baseline;
    margin-bottom: 8px;

    strong {
      color: var(--el-text-color-primary);
    }

    span {
      font-size: 12px;
      color: var(--el-text-color-secondary);
    }
  }

  .home-stage {
    position: relative;
    overflow: hidden;
    color: var(--remote-text);
    background: var(--remote-background);
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 16px;
  }

  .home-stage--cartoon-blue {
    background:
      radial-gradient(circle at 12% 18%, rgb(22 143 229 / 16%) 0 90px, transparent 92px),
      linear-gradient(145deg, var(--remote-background), #e7f7ff 62%, #fff8f4);

    .home-header,
    .home-footer,
    .feature-grid article {
      background: rgb(255 255 255 / 82%);
    }

    .action,
    .login-chip,
    .hero-badge {
      border-radius: 999px;
    }

    .login-hint-card,
    .feature-grid article {
      border-radius: 22px;
    }

    .hero-art {
      color: #fff;
      background: var(--remote-primary);
      border: 10px solid rgb(255 255 255 / 80%);
      border-radius: 50%;
    }

    h3 span {
      color: #e64a4a;
    }
  }

  .home-stage--fintech-gold {
    background-color: var(--remote-background);
    background-image:
      linear-gradient(rgb(240 185 11 / 8%) 1px, transparent 1px),
      linear-gradient(90deg, rgb(240 185 11 / 8%) 1px, transparent 1px);
    background-size: 28px 28px;

    .home-header,
    .home-footer {
      background: rgb(9 11 16 / 92%);
      border-color: #252a34;
    }

    .feature-grid article,
    .login-hint-card {
      color: var(--remote-text);
      background: #11151c;
      border-color: #2b313d;
      border-radius: 6px;
    }

    .action,
    .login-chip,
    .hero-badge {
      border-radius: 3px;
    }

    .action-primary {
      color: #090b10;
    }

    .action-secondary {
      color: #f4f6fa;
      background: transparent;
      border-color: #3a404c;
    }

    .hero-badge {
      padding-left: 10px;
      letter-spacing: 0.06em;
      background: rgb(240 185 11 / 12%);
      border-left: 3px solid var(--remote-primary);
      border-radius: 0;
    }

    .hero-art {
      color: var(--remote-primary);
      background: #11151c;
      border-color: #343b47;
      border-radius: 6px;
      clip-path: polygon(12% 0, 100% 0, 100% 86%, 86% 100%, 0 100%, 0 14%);
    }

    .hero-copy > p,
    .feature-grid p,
    .home-footer {
      color: #aeb4bf;
    }
  }

  .home-header,
  .hero-grid,
  .feature-grid,
  .home-footer {
    width: min(920px, calc(100% - 28px));
    margin-right: auto;
    margin-left: auto;
  }

  .home-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    min-height: 58px;
    border-bottom: 1px solid rgb(255 255 255 / 8%);
  }

  .home-brand {
    display: flex;
    gap: 8px;
    align-items: center;
    font-size: 14px;
  }

  .brand-mark {
    width: 28px;
    height: 28px;
    background: var(--remote-primary);
    border-radius: 50%;
  }

  .home-stage--fintech-gold .brand-mark {
    border-radius: 4px;
  }

  .login-chip,
  .action {
    padding: 8px 14px;
    font: inherit;
    color: #fff;
    cursor: pointer;
    background: var(--remote-primary);
    border: 1px solid var(--remote-primary);
  }

  .action-secondary {
    color: var(--remote-text);
    background: transparent;
  }

  .hero-grid {
    display: grid;
    grid-template-columns: minmax(0, 1.2fr) 180px;
    gap: 24px;
    align-items: center;
    padding: 28px 0 8px;
  }

  .hero-badge {
    display: inline-flex;
    padding: 4px 10px;
    font-size: 12px;
    color: var(--remote-primary);
    background: color-mix(in srgb, var(--remote-primary) 12%, white);
  }

  h3 {
    margin: 12px 0;
    font-size: 32px;
    line-height: 1.15;
    letter-spacing: -1px;

    span {
      display: block;
      color: var(--remote-primary);
    }
  }

  .hero-copy > p,
  .feature-grid p {
    margin: 0;
    font-size: 14px;
    line-height: 1.7;
    color: color-mix(in srgb, var(--remote-text) 72%, transparent);
  }

  .hero-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 16px;
  }

  .hero-art {
    display: grid;
    place-items: center;
    width: 160px;
    height: 160px;
    margin-left: auto;
    border: 1px solid transparent;

    svg {
      width: 64px;
      height: 64px;
    }
  }

  .feature-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 12px;
    padding-bottom: 22px;

    article {
      padding: 16px;
      border: 1px solid rgb(21 51 74 / 8%);
    }

    svg {
      width: 22px;
      height: 22px;
      color: var(--remote-primary);
    }

    h4 {
      margin: 10px 0 6px;
      font-size: 15px;
    }
  }

  .home-footer {
    padding: 14px 0 16px;
    font-size: 12px;
    text-align: center;
    border-top: 1px solid rgb(255 255 255 / 8%);
  }

  .login-hint {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    padding: 16px;
    background: rgb(9 11 16 / 46%);
  }

  .login-hint-card {
    display: grid;
    gap: 10px;
    width: min(380px, 100%);
    padding: 18px;
    background: #fff;
    border: 1px solid color-mix(in srgb, var(--remote-text) 12%, transparent);
    box-shadow: 0 18px 50px rgb(0 0 0 / 18%);

    h3,
    p {
      margin: 0;
    }

    label {
      display: grid;
      gap: 4px;
      font-size: 12px;
    }

    input {
      padding: 8px 10px;
      color: var(--remote-text);
      background: transparent;
      border: 1px solid color-mix(in srgb, var(--remote-text) 18%, transparent);
      border-radius: inherit;
    }
  }

  .home-stage--cartoon-blue .login-hint-card {
    border-radius: 22px;
  }

  .hint-kicker {
    font-size: 12px;
    color: var(--remote-primary);
  }

  .hint-close {
    justify-self: start;
    padding: 0;
    font: inherit;
    color: var(--remote-text);
    cursor: pointer;
    background: transparent;
    border: 0;
  }

  .host-note {
    margin: 8px 0 0;
    font-size: 12px;
    color: var(--el-text-color-secondary);
  }

  .store-frame {
    padding: 14px;
    background: var(--el-fill-color-blank);
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 12px;
  }

  .store-tabs {
    display: flex;
    gap: 8px;
    margin-bottom: 12px;
    font-size: 13px;
    color: var(--el-text-color-secondary);

    .is-active {
      padding-bottom: 4px;
      font-weight: 600;
      color: var(--el-color-primary);
      border-bottom: 2px solid var(--el-color-primary);
    }
  }

  .store-card {
    display: grid;
    grid-template-columns: 46px minmax(0, 1fr);
    gap: 12px 14px;
    padding: 16px;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color);
    border-radius: 8px;
  }

  .store-icon {
    display: grid;
    place-items: center;
    width: 46px;
    height: 46px;
    color: var(--el-color-primary);
    background: var(--el-color-primary-light-9);
    border-radius: 10px;

    svg {
      width: 24px;
      height: 24px;
    }
  }

  .store-name {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    align-items: center;

    strong {
      color: var(--el-text-color-primary);
    }
  }

  .store-meta p {
    margin: 6px 0 0;
    font-size: 12px;
    line-height: 1.6;
    color: var(--el-text-color-regular);
  }

  .store-author {
    color: var(--el-text-color-secondary) !important;
  }

  .store-status {
    grid-column: 1 / -1;
    padding-top: 10px;
    border-top: 1px dashed var(--el-border-color);
  }

  .pill {
    display: inline-flex;
    padding: 1px 6px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
    border: 1px solid var(--el-border-color);
    border-radius: 4px;
  }

  .pill-warn {
    color: var(--el-color-warning);
    border-color: var(--el-color-warning-light-5);
  }

  @media (max-width: 768px) {
    .hero-grid,
    .feature-grid {
      grid-template-columns: 1fr;
    }

    .hero-art {
      width: 120px;
      height: 120px;
      margin: 0;
    }

    h3 {
      font-size: 26px;
    }

    .store-card {
      grid-template-columns: 46px minmax(0, 1fr);
    }
  }
</style>
