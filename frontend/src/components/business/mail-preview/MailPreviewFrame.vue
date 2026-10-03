<!-- 邮件内容预览：放进不带任何权限的沙箱 iframe，邮件里的脚本、表单、外链都不能动后台页面和登录信息。 -->
<template>
  <iframe
    class="mail-preview-frame"
    :style="{ height }"
    sandbox=""
    referrerpolicy="no-referrer"
    title="邮件预览"
    :srcdoc="srcdoc"
  ></iframe>
</template>

<script setup lang="ts">
  import { computed } from 'vue'
  import { mailPreviewDocument } from './mail-preview'

  const props = withDefaults(
    defineProps<{ content: string; contentType?: 'text' | 'html'; height?: string }>(),
    { contentType: 'html', height: '360px' }
  )

  const srcdoc = computed(() => mailPreviewDocument(props.content || '', props.contentType))
</script>

<style scoped lang="scss">
  .mail-preview-frame {
    display: block;
    width: 100%;
    border: 1px solid var(--art-border-color);
    border-radius: 10px;
    background: #fff;
  }
</style>
