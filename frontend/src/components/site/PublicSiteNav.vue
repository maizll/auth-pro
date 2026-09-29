<!-- 电脑横排，当前项是直线下划线。手机把菜单收到左侧抽屉。 -->
<template>
  <nav ref="host" class="public-nav" :class="{ 'is-narrow': narrow }" aria-label="官网导航">
    <button
      v-if="narrow"
      class="public-nav__menu-btn"
      type="button"
      :aria-label="drawer ? '关闭菜单' : '打开菜单'"
      @click="drawer = !drawer"
    >
      <span />
      <span />
      <span />
    </button>
    <div v-if="!narrow" class="public-nav__row">
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
    <ElDrawer
      v-if="narrow"
      v-model="drawer"
      direction="ltr"
      size="232px"
      :with-header="false"
      append-to-body
    >
      <component
        :is="item.external ? 'a' : 'RouterLink'"
        v-for="item in items"
        :key="item.key"
        class="public-nav__drawer-link"
        :class="{ 'is-active': active(item) }"
        :to="item.external ? undefined : item.href"
        :href="item.external ? item.href : undefined"
        @click="drawer = false"
      >
        {{ item.label }}
      </component>
    </ElDrawer>
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
  const drawer = ref(false)
  const narrow = ref(false)

  function syncNarrow() {
    narrow.value = window.innerWidth < 768
    if (!narrow.value) drawer.value = false
  }

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
    const nodes = Array.from(rule.querySelectorAll<HTMLElement>('[data-nav-item]'))
    const widthOf = new Map<string, number>()
    items.forEach((item, index) => widthOf.set(item.key, nodes[index]?.offsetWidth || 0))
    const moreWidth = rule.querySelector<HTMLElement>('[data-nav-more]')?.offsetWidth || 52
    const gap = 8
    // 内置项优先留在这一行。外链最多露 2 个，多出来的直接进「更多」。
    let externalShown = 0
    const preferred: PublicNavItem[] = []
    const forced: PublicNavItem[] = []
    for (const item of items) {
      if (item.external) {
        externalShown += 1
        if (externalShown > 2) {
          forced.push(item)
          continue
        }
      }
      preferred.push(item)
    }
    let used = 0
    let count = 0
    for (let index = 0; index < preferred.length; index += 1) {
      const width = widthOf.get(preferred[index].key) || 0
      const needsMore = forced.length > 0 || index < preferred.length - 1
      const limit = needsMore ? available - moreWidth - gap : available
      if (count > 0 && used + width > limit) break
      used += width + gap
      count += 1
    }
    if (count < 1 && preferred.length) count = 1
    shown.value = preferred.slice(0, count)
    overflow.value = preferred.slice(count).concat(forced)
    if (!overflow.value.length) open.value = false
  }

  function onResize() {
    measure()
  }

  function onDocumentClick() {
    open.value = false
  }

  onMounted(() => {
    syncNarrow()
    measure()
    window.addEventListener('resize', onResize)
    window.addEventListener('resize', syncNarrow)
    document.addEventListener('click', onDocumentClick)
  })
  onBeforeUnmount(() => {
    window.removeEventListener('resize', onResize)
    window.removeEventListener('resize', syncNarrow)
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

  .public-nav.is-narrow {
    flex: 0 0 auto;
    order: -1;
    width: auto;
    height: auto;
  }

  .public-nav__menu-btn {
    display: grid;
    gap: 4px;
    width: 36px;
    height: 36px;
    padding: 8px;
    background: transparent;
    border: 0;
  }

  .public-nav__menu-btn span {
    display: block;
    height: 2px;
    background: currentcolor;
  }

  .public-nav__drawer-link {
    display: block;
    padding: 12px 16px;
    color: inherit;
    font-size: 14px;
    text-decoration: none;
  }

  .public-nav__drawer-link.is-active {
    color: var(--remote-primary, var(--el-color-primary));
    background: #eef3ff;
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
