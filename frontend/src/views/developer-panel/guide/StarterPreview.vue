<template>
  <el-card shadow="never" class="starter-preview">
    <div class="preview-head">
      <div>
        <h2>标准示例预览</h2>
        <p>
          下面用入门包里的 <code>template.json</code>、<code>template.fintech-gold.json</code> 和
          <code>plugin.json</code> 直接渲染正式首页和商店卡片。不用先登记，也不是另一套字段。
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
      <div class="live-frame" data-testid="template-stage">
        <RemoteHomeTemplate :document="activeDocument" preview />
      </div>
      <p class="host-note">
        点登录打开的是宿主登录框。预览不提交密码。启用后圆角和颜色跟随 stylePreset 与
        theme.primaryColor、backgroundColor、textColor。
      </p>
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
  import RemoteHomeTemplate from '@/views/user-panel/login/RemoteHomeTemplate.vue'
  import {
    type HomeTemplateDocument,
    type HomeTemplateStylePreset,
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

  const documents: Record<HomeTemplateStylePreset, HomeTemplateDocument | null> = {
    'cartoon-blue': isHomeTemplateDocument(cartoonTemplate) ? cartoonTemplate : null,
    'fintech-gold': isHomeTemplateDocument(goldTemplate) ? goldTemplate : null
  }

  const activeDocument = computed(() => documents[preset.value])
  const activePreset = computed(() => preset.value)
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
      max-width: 720px;
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

  .live-frame {
    max-height: 720px;
    overflow: auto;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 16px;
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
    .live-frame {
      max-height: 640px;
    }
  }
</style>
