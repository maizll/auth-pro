<template>
  <IconifyIcon
    class="app-icon"
    :icon="icon"
    :width="size ?? 18"
    :height="size ?? 18"
    :style="size ? { '--icon-size': `${size}px` } : undefined"
    :aria-hidden="!label"
    :aria-label="label"
    :role="label ? 'img' : undefined"
    focusable="false"
  />
</template>

<script setup lang="ts">
  import { computed } from 'vue'
  import { Icon as IconifyIcon } from '@iconify/vue'
  import riIcons from '@iconify-json/ri/icons.json'

  const props = defineProps<{ name: string; size?: number; label?: string }>()
  // Reuse auth-pro's existing offline icon set; no CDN or new dependency.
  const icons = {
    'arrow-right': 'arrow-right-line',
    'arrow-up-right': 'arrow-right-up-line',
    check: 'check-line',
    'circle-check': 'checkbox-circle-line',
    'file-key': 'file-lock-line',
    globe: 'global-line',
    headphones: 'customer-service-2-line',
    'key-round': 'key-2-line',
    'layers-3': 'stack-line',
    'layout-grid': 'layout-grid-line',
    'loader-circle': 'loader-4-line',
    'lock-keyhole': 'lock-2-line',
    menu: 'menu-line',
    'refresh-cw': 'refresh-line',
    rocket: 'rocket-2-line',
    search: 'search-line',
    'search-check': 'search-eye-line',
    'shield-check': 'shield-check-line',
    'shopping-bag': 'shopping-bag-line',
    'user-round': 'user-line',
    'user-round-plus': 'user-add-line',
    x: 'close-line',
    'ri:search-eye-line': 'search-eye-line',
    'ri:shield-keyhole-line': 'shield-check-line',
    'ri:customer-service-2-line': 'customer-service-2-line'
  } as const
  const icon = computed(() => {
    let name: keyof typeof riIcons.icons = 'shield-check-line'
    if (Object.prototype.hasOwnProperty.call(icons, props.name)) {
      name = icons[props.name as keyof typeof icons]
    } else if (
      props.name.startsWith('ri:') &&
      Object.prototype.hasOwnProperty.call(riIcons.icons, props.name.slice(3))
    ) {
      name = props.name.slice(3) as keyof typeof riIcons.icons
    }
    return { ...riIcons.icons[name], width: riIcons.width, height: riIcons.height }
  })
</script>

<style scoped>
  .app-icon {
    display: inline-block;
    flex: 0 0 auto;
    width: var(--icon-size, 18px);
    height: var(--icon-size, 18px);
    vertical-align: middle;
    transition:
      color var(--motion-duration) var(--motion-ease),
      transform var(--motion-duration) var(--motion-ease);
  }
  @media (prefers-reduced-motion: reduce) {
    .app-icon {
      transition: none;
    }
  }
</style>
