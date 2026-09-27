<!-- 官网 Markdown 只有这一份：代码高亮在 renderSiteMarkdown，复制按钮在这里。 -->
<template>
  <div class="site-markdown" @click="onCopyClick" v-html="viewHtml" />
</template>

<script setup lang="ts">
  import { computed } from 'vue'
  import { renderSiteMarkdown } from '@/utils/markdown'

  defineOptions({ name: 'SiteMarkdown' })

  const props = defineProps<{
    source?: string
    pageTitle?: string
    html?: string
  }>()

  const viewHtml = computed(() => {
    if (props.html != null) return props.html
    return renderSiteMarkdown(props.source || '', props.pageTitle || '').html
  })

  const copyTimers = new WeakMap<HTMLButtonElement, number>()

  function onCopyClick(event: MouseEvent) {
    const target = event.target
    if (!(target instanceof Element)) return
    const button = target.closest('.site-code__copy')
    if (!(button instanceof HTMLButtonElement)) return
    const code = button.parentElement?.querySelector('pre code')
    const text = code?.textContent || ''
    void copyPlainText(text).then((ok) => {
      if (ok) markCopied(button)
    })
  }

  function markCopied(button: HTMLButtonElement) {
    const previous = copyTimers.get(button)
    if (previous) window.clearTimeout(previous)
    button.textContent = '已复制'
    button.classList.add('is-copied')
    copyTimers.set(
      button,
      window.setTimeout(() => {
        button.textContent = '复制'
        button.classList.remove('is-copied')
        copyTimers.delete(button)
      }, 2000)
    )
  }

  // 优先用剪贴板接口。非 HTTPS 或接口不可用时，退回选中文本框再 execCommand。
  async function copyPlainText(text: string) {
    const clipboard = navigator.clipboard
    if (window.isSecureContext && clipboard && typeof clipboard.writeText === 'function') {
      try {
        await clipboard.writeText(text)
        return true
      } catch {
        // 权限被拒绝时继续走下面的兼容写法。
      }
    }
    const area = document.createElement('textarea')
    area.value = text
    area.setAttribute('readonly', '')
    area.style.position = 'fixed'
    area.style.top = '0'
    area.style.left = '0'
    area.style.opacity = '0'
    document.body.appendChild(area)
    area.focus()
    area.select()
    let ok = false
    try {
      ok = document.execCommand('copy')
    } catch {
      ok = false
    }
    area.remove()
    return ok
  }
</script>

<style scoped>
  .site-markdown {
    min-width: 0;
    max-width: 100%;
  }

  .site-markdown :deep(code) {
    padding: 0.1em 0.35em;
    font-size: 0.92em;
    background: rgb(47 111 237 / 8%);
    border-radius: 4px;
  }

  .site-markdown :deep(.site-code) {
    position: relative;
    max-width: 100%;
    min-width: 0;
    margin: 0 0 0.9em;
  }

  .site-markdown :deep(.site-code__copy) {
    position: absolute;
    top: 8px;
    right: 8px;
    z-index: 1;
    min-width: 72px;
    min-height: 36px;
    padding: 0 12px;
    color: #1d4ed8;
    font-size: 14px;
    line-height: 34px;
    cursor: pointer;
    background: #fff;
    border: 1px solid rgb(29 78 216 / 35%);
    border-radius: 8px;
  }

  .site-markdown :deep(.site-code__copy.is-copied) {
    color: #fff;
    background: #1d4ed8;
    border-color: #1d4ed8;
  }

  .site-markdown :deep(.site-code pre) {
    max-width: 100%;
    margin: 0;
    padding: 44px 14px 12px;
    overflow-x: auto;
    background: #f3f7ff;
    border: 1px solid rgb(47 111 237 / 18%);
    border-radius: 10px;
  }

  .site-markdown :deep(.site-code pre code) {
    display: block;
    padding: 0;
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 13px;
    line-height: 1.6;
    color: #1c2740;
    white-space: pre;
    word-break: normal;
    overflow-wrap: normal;
    background: transparent;
  }

  .site-markdown :deep(.hljs-keyword),
  .site-markdown :deep(.hljs-selector-tag),
  .site-markdown :deep(.hljs-literal),
  .site-markdown :deep(.hljs-built_in),
  .site-markdown :deep(.hljs-type) {
    color: #1d4ed8;
  }

  .site-markdown :deep(.hljs-string),
  .site-markdown :deep(.hljs-attr),
  .site-markdown :deep(.hljs-attribute) {
    color: #0f766e;
  }

  .site-markdown :deep(.hljs-number),
  .site-markdown :deep(.hljs-symbol) {
    color: #1e3a8a;
  }

  .site-markdown :deep(.hljs-comment),
  .site-markdown :deep(.hljs-quote) {
    color: #64748b;
  }

  .site-markdown :deep(.hljs-title),
  .site-markdown :deep(.hljs-function),
  .site-markdown :deep(.hljs-name),
  .site-markdown :deep(.hljs-section) {
    color: #2563eb;
  }

  .site-markdown :deep(.hljs-variable),
  .site-markdown :deep(.hljs-params),
  .site-markdown :deep(.hljs-meta) {
    color: #334155;
  }

  :global(html.dark) .site-markdown :deep(.site-code__copy) {
    color: #bfdbfe;
    background: #0f172a;
    border-color: rgb(147 197 253 / 45%);
  }

  :global(html.dark) .site-markdown :deep(.site-code__copy.is-copied) {
    color: #0f172a;
    background: #93c5fd;
    border-color: #93c5fd;
  }

  :global(html.dark) .site-markdown :deep(.site-code pre) {
    background: #0b1220;
    border-color: rgb(147 197 253 / 22%);
  }

  :global(html.dark) .site-markdown :deep(.site-code pre code) {
    color: #e2e8f0;
  }

  :global(html.dark) .site-markdown :deep(.hljs-keyword),
  :global(html.dark) .site-markdown :deep(.hljs-selector-tag),
  :global(html.dark) .site-markdown :deep(.hljs-literal),
  :global(html.dark) .site-markdown :deep(.hljs-built_in),
  :global(html.dark) .site-markdown :deep(.hljs-type),
  :global(html.dark) .site-markdown :deep(.hljs-title),
  :global(html.dark) .site-markdown :deep(.hljs-function),
  :global(html.dark) .site-markdown :deep(.hljs-name) {
    color: #93c5fd;
  }

  :global(html.dark) .site-markdown :deep(.hljs-string),
  :global(html.dark) .site-markdown :deep(.hljs-attr) {
    color: #5eead4;
  }

  :global(html.dark) .site-markdown :deep(.hljs-number) {
    color: #bfdbfe;
  }

  :global(html.dark) .site-markdown :deep(.hljs-comment) {
    color: #94a3b8;
  }

  :global(html.dark) .site-markdown :deep(.hljs-variable),
  :global(html.dark) .site-markdown :deep(.hljs-params),
  :global(html.dark) .site-markdown :deep(.hljs-meta) {
    color: #cbd5e1;
  }
</style>
