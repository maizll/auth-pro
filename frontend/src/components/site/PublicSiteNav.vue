<!-- 官网顶栏导航。和电脑同一行，放不下的收进「更多」，不用汉堡菜单和侧栏。 -->
<template>
  <nav ref="host" class="public-nav" aria-label="官网导航">
    <div class="public-nav__row">
      <component
        :is="item.external ? 'a' : 'RouterLink'"
        v-for="item in shown"
        :key="item.key"
        class="public-nav__link"
        :class="{ 'is-active': active(item) }"
        :to="item.external ? undefined : item.href"
        :href="item.external ? item.href : undefined"
        :target="item.external ? '_blank' : undefined"
        :rel="item.external ? 'noopener noreferrer' : undefined"
        :title="item.label"
      >
        {{ item.label }}
      </component>
      <button
        v-if="overflow.length"
        class="public-nav__more"
        type="button"
        :aria-expanded="open"
        @click.stop="open = !open"
      >
        更多
      </button>
    </div>
    <div v-if="overflow.length && open" class="public-nav__menu" @click.stop>
      <component
        :is="item.external ? 'a' : 'RouterLink'"
        v-for="item in overflow"
        :key="item.key"
        class="public-nav__menu-link"
        :to="item.external ? undefined : item.href"
        :href="item.external ? item.href : undefined"
        :target="item.external ? '_blank' : undefined"
        :rel="item.external ? 'noopener noreferrer' : undefined"
        @click="open = false"
      >
        {{ item.label }}
      </component>
    </div>
    <div ref="ruler" class="public-nav__ruler" aria-hidden="true">
      <span v-for="item in items" :key="item.key" data-nav-item>{{ item.label }}</span>
      <span data-nav-more>更多</span>
    </div>
  </nav>
</template>

<script setup lang="ts">
  import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
  import { useRoute } from 'vue-router'
  import { navItemActive, type PublicNavItem } from '@/utils/public-site'

  defineOptions({ name: 'PublicSiteNav' })

  const props = defineProps<{ items: PublicNavItem[] }>()
  const route = useRoute()
  const host = ref<HTMLElement>()
  const ruler = ref<HTMLElement>()
  const shown = ref<PublicNavItem[]>([])
  const overflow = ref<PublicNavItem[]>([])
  const open = ref(false)

  function active(item: PublicNavItem) {
    return navItemActive(route.path, item)
  }

  function measure() {
    const items = props.items || []
    const box = host.value
    const rule = ruler.value
    if (!box || !rule || items.length === 0) {
      shown.value = items
      overflow.value = []
      return
    }
    const available = box.clientWidth
    const widths = Array.from(rule.querySelectorAll<HTMLElement>('[data-nav-item]')).map(
      (node) => node.offsetWidth
    )
    const moreWidth = rule.querySelector<HTMLElement>('[data-nav-more]')?.offsetWidth || 52
    const gap = 8
    let used = 0
    let count = 0
    for (let index = 0; index < items.length; index += 1) {
      const width = widths[index] || 0
      const needsMore = index < items.length - 1
      const limit = needsMore ? available - moreWidth - gap : available
      if (count > 0 && used + width > limit) break
      used += width + gap
      count += 1
    }
    if (count < 1) count = 1
    shown.value = items.slice(0, count)
    overflow.value = items.slice(count)
    if (!overflow.value.length) open.value = false
  }

  function onResize() {
    measure()
  }

  function onDocumentClick() {
    open.value = false
  }

  onMounted(() => {
    measure()
    window.addEventListener('resize', onResize)
    document.addEventListener('click', onDocumentClick)
  })
  onBeforeUnmount(() => {
    window.removeEventListener('resize', onResize)
    document.removeEventListener('click', onDocumentClick)
  })
  watch(
    () => props.items,
    () => {
      open.value = false
      requestAnimationFrame(measure)
    }
  )
</script>

<style scoped>
  .public-nav {
    position: relative;
    flex: 1 1 auto;
    min-width: 0;
    height: 40px;
  }

  .public-nav__row {
    display: flex;
    flex-wrap: nowrap;
    align-items: center;
    min-width: 0;
    height: 100%;
    overflow: hidden;
    gap: 4px;
  }

  .public-nav__link,
  .public-nav__more,
  .public-nav__menu-link {
    flex: 0 0 auto;
    max-width: 8em;
    padding: 0 8px;
    overflow: hidden;
    color: inherit;
    font-size: 14px;
    line-height: 32px;
    white-space: nowrap;
    text-decoration: none;
    text-overflow: ellipsis;
    cursor: pointer;
    background: transparent;
    border: 0;
    border-radius: 8px;
  }

  .public-nav__link.is-active,
  .public-nav__link:hover,
  .public-nav__more:hover,
  .public-nav__menu-link:hover {
    color: var(--remote-primary, var(--el-color-primary));
  }

  .public-nav__link.is-active {
    box-shadow: inset 0 -2px 0 var(--remote-primary, var(--el-color-primary));
  }

  .public-nav__more {
    font-weight: 600;
  }

  .public-nav__menu {
    position: absolute;
    top: calc(100% + 6px);
    right: 0;
    z-index: 30;
    display: flex;
    flex-direction: column;
    width: max-content;
    max-width: min(240px, 70vw);
    padding: 6px;
    background: #fff;
    border: 1px solid rgb(28 39 64 / 8%);
    border-radius: 12px;
    box-shadow: 0 12px 32px rgb(28 39 64 / 12%);
  }

  .public-nav__menu-link {
    max-width: none;
    line-height: 36px;
    color: #1c2740;
  }

  .public-nav__ruler {
    position: absolute;
    top: 0;
    left: 0;
    visibility: hidden;
    display: flex;
    gap: 4px;
    height: 0;
    overflow: hidden;
    pointer-events: none;
  }

  .public-nav__ruler span {
    padding: 0 8px;
    font-size: 14px;
    line-height: 32px;
    white-space: nowrap;
  }
</style>
