<!-- 在线更新页的「历史版本」：按大版本分组、折叠、搜索、滚到底加载更多、回到顶部。官网和客户站共用。 -->
<template>
  <div ref="sectionRef" class="update-section history-section">
    <div class="history-head">
      <div class="history-title">
        <strong>历史版本</strong>
        <span v-if="releases.length">共 {{ releases.length }} 个版本</span>
      </div>
      <ElButton link type="primary" :loading="loading" @click="emit('refresh')">刷新记录</ElButton>
    </div>
    <div ref="searchRef" class="history-search">
      <ElInput
        v-model="query"
        :prefix-icon="Search"
        clearable
        placeholder="搜索版本号或更新内容，如 1.8 或 签名"
      />
    </div>
    <ElAlert
      v-if="error"
      :title="error"
      type="info"
      show-icon
      :closable="false"
      class="history-alert"
    />
    <ElSkeleton v-if="loading && !releases.length" :rows="4" animated />
    <template v-else-if="views.length">
      <section v-for="view in views" :key="view.group.key" class="release-group">
        <button
          type="button"
          class="release-group__head"
          :aria-expanded="view.open"
          @click="toggleGroup(view.group, view.index)"
        >
          <ElIcon class="chevron" :class="{ open: view.open }">
            <ArrowRight />
          </ElIcon>
          <strong>{{ view.group.key }}</strong>
          <span class="release-group__count">{{ view.group.releases.length }} 个版本</span>
        </button>
        <div v-if="view.shown" class="release-group__body">
          <article
            v-for="release in view.group.releases.slice(0, view.shown)"
            :key="release.version"
            class="release-item"
            :class="{ open: releaseOpen(release.version) }"
          >
            <button
              type="button"
              class="release-row"
              :aria-expanded="releaseOpen(release.version)"
              @click="toggleRelease(release.version)"
            >
              <ElIcon class="chevron" :class="{ open: releaseOpen(release.version) }">
                <ArrowRight />
              </ElIcon>
              <span class="release-row__main">
                <strong>v{{ release.version }}</strong>
                <ElTag v-if="isLatest(release.version)" size="small" type="success" effect="plain">
                  最新版本
                </ElTag>
                <ElTag v-if="isCurrent(release.version)" size="small" type="info" effect="plain">
                  当前版本
                </ElTag>
                <ElTag
                  v-if="channelLabel(release.channel) !== '正式版'"
                  size="small"
                  effect="plain"
                >
                  {{ channelLabel(release.channel) }}
                </ElTag>
                <span class="release-row__date">{{ releaseDate(release.releasedAt) }}</span>
              </span>
              <span v-if="!releaseOpen(release.version)" class="release-row__summary">
                {{ release.notes[0] || '该版本未记录更新日志' }}
              </span>
            </button>
            <div v-if="releaseOpen(release.version)" class="release-notes">
              <div
                v-for="(note, noteIndex) in release.notes"
                :key="`${release.version}-${noteIndex}`"
                class="release-note"
              >
                <span class="release-note-dot" />
                <span>{{ note }}</span>
              </div>
              <ElText v-if="!release.notes.length" type="info" size="small"
                >该版本未记录更新日志</ElText
              >
            </div>
          </article>
          <div
            v-if="view.hidden"
            ref="sentinelRef"
            class="history-more"
            :data-group="view.group.key"
          >
            <ElButton link type="primary" @click="loadMore(view.group.key)">
              还有 {{ view.hidden }} 个版本，继续加载
            </ElButton>
          </div>
        </div>
      </section>
    </template>
    <ElEmpty
      v-else-if="!loading"
      :description="query.trim() ? '没有找到匹配的版本' : '暂无历史版本记录'"
      :image-size="72"
    />
    <Transition name="el-fade-in">
      <button
        v-show="showBackTop"
        type="button"
        class="history-backtop"
        aria-label="回到顶部"
        @click="backToTop"
      >
        <ElIcon><Top /></ElIcon>
        <span>回到顶部</span>
      </button>
    </Transition>
  </div>
</template>

<script setup lang="ts">
  import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
  import { ArrowRight, Search, Top } from '@element-plus/icons-vue'
  import type { OnlineUpdateRelease } from '@/api/update'
  import { updateChannelLabel } from './status-label'
  import {
    RELEASE_PAGE_SIZE,
    filterReleases,
    groupOpenByDefault,
    groupReleases,
    layoutGroups,
    normalizeVersion,
    releaseOpenByDefault,
    type ReleaseGroup
  } from './release-history'

  defineOptions({ name: 'ReleaseHistory' })

  const props = defineProps<{
    releases: OnlineUpdateRelease[]
    currentVersion?: string
    latestVersion?: string
    loading?: boolean
    error?: string
  }>()
  const emit = defineEmits<{ refresh: [] }>()

  const query = ref('')
  // 每组已加载的页数上限，没记的按一页
  const groupLimit = ref<Record<string, number>>({})
  // 用户点过的展开状态，覆盖默认规则
  const releaseToggled = ref<Record<string, boolean>>({})
  const groupToggled = ref<Record<string, boolean>>({})
  const showBackTop = ref(false)
  const sectionRef = ref<HTMLElement>()
  const searchRef = ref<HTMLElement>()
  // 没显示完的组末尾各有一个「加载更多」，v-for 里的 ref 是数组
  const sentinelRef = ref<HTMLElement[]>([])

  const searching = computed(() => query.value.trim() !== '')
  const filtered = computed(() => filterReleases(props.releases, query.value))
  const groups = computed(() => groupReleases(filtered.value))
  const views = computed(() =>
    layoutGroups(
      groups.value,
      groupOpen,
      (group) => groupLimit.value[group.key] ?? RELEASE_PAGE_SIZE
    )
  )

  const channelLabel = updateChannelLabel
  const isLatest = (version: string) =>
    normalizeVersion(version) === normalizeVersion(props.latestVersion)
  const isCurrent = (version: string) =>
    normalizeVersion(version) === normalizeVersion(props.currentVersion)

  const releaseOpen = (version: string) =>
    releaseToggled.value[version] ??
    releaseOpenByDefault(version, props.latestVersion, props.currentVersion)
  const toggleRelease = (version: string) => {
    releaseToggled.value = { ...releaseToggled.value, [version]: !releaseOpen(version) }
  }
  function groupOpen(group: ReleaseGroup, index: number) {
    return (
      groupToggled.value[group.key] ??
      groupOpenByDefault(group, index, props.latestVersion, props.currentVersion, searching.value)
    )
  }
  const toggleGroup = (group: ReleaseGroup, index: number) => {
    groupToggled.value = { ...groupToggled.value, [group.key]: !groupOpen(group, index) }
  }

  const releaseDate = (value?: string) => {
    if (!value) return ''
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
  }

  // 换关键词后从头显示，分组展开状态回到默认
  watch(query, () => {
    groupLimit.value = {}
    groupToggled.value = {}
  })

  // 给一组再加一页；加完「加载更多」还在屏幕里就接着加
  const nearViewport = (el?: HTMLElement) =>
    !!el && el.getBoundingClientRect().top < window.innerHeight + 120
  const loadMore = async (key: string) => {
    groupLimit.value = {
      ...groupLimit.value,
      [key]: (groupLimit.value[key] ?? RELEASE_PAGE_SIZE) + RELEASE_PAGE_SIZE
    }
    await nextTick()
    const el = sentinelRef.value.find((item) => item.dataset.group === key)
    if (el && nearViewport(el)) void loadMore(key)
  }

  let moreObserver: IntersectionObserver | undefined
  // 「回到顶部」：搜索框滚到顶栏下面看不见、列表还在屏幕里时出现。
  // 后台布局的滚动容器不固定，在捕获阶段监听所有滚动；快速跳动时交叉观察会漏掉，所以直接量位置。
  let backTopFrame = 0
  const updateBackTop = () => {
    backTopFrame = 0
    const search = searchRef.value?.getBoundingClientRect()
    const section = sectionRef.value?.getBoundingClientRect()
    // 顶栏加标签栏约 120px，搜索框被它挡住就算滚出去了
    showBackTop.value = !!search && !!section && search.bottom < 120 && section.bottom > 240
  }
  const onScroll = () => {
    if (!backTopFrame) backTopFrame = window.requestAnimationFrame(updateBackTop)
  }

  onMounted(() => {
    document.addEventListener('scroll', onScroll, { capture: true, passive: true })
    if (typeof IntersectionObserver === 'undefined') return
    moreObserver = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const key = (entry.target as HTMLElement).dataset.group
          if (entry.isIntersecting && key) void loadMore(key)
        }
      },
      { rootMargin: '0px 0px 120px 0px' }
    )
    sentinelRef.value.forEach((el) => moreObserver?.observe(el))
  })

  // 「加载更多」增减时重新观察
  watch(
    views,
    () => {
      moreObserver?.disconnect()
      sentinelRef.value.forEach((el) => moreObserver?.observe(el))
    },
    { flush: 'post' }
  )

  onBeforeUnmount(() => {
    moreObserver?.disconnect()
    document.removeEventListener('scroll', onScroll, { capture: true })
    if (backTopFrame) window.cancelAnimationFrame(backTopFrame)
  })

  const backToTop = () => {
    sectionRef.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }
</script>

<style lang="scss" scoped>
  .history-section {
    scroll-margin-top: 20px;
  }

  .history-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 12px;

    .history-title {
      display: flex;
      gap: 10px;
      align-items: baseline;

      strong {
        font-size: 15px;
        color: var(--art-gray-900);
      }

      span {
        font-size: 12px;
        color: var(--art-gray-500);
      }
    }
  }

  .history-search {
    max-width: 420px;
    margin-bottom: 14px;
  }

  .history-alert {
    margin-bottom: 14px;
  }

  .chevron {
    flex-shrink: 0;
    color: var(--art-gray-500);
    transition: transform 0.2s ease;

    &.open {
      transform: rotate(90deg);
    }
  }

  .release-group + .release-group {
    margin-top: 10px;
  }

  .release-group__head {
    display: flex;
    gap: 8px;
    align-items: center;
    width: 100%;
    min-height: 40px;
    padding: 8px 4px;
    font: inherit;
    color: var(--art-gray-900);
    text-align: left;
    cursor: pointer;
    background: none;
    border: none;

    strong {
      font-size: 14px;
    }

    .release-group__count {
      padding: 1px 8px;
      font-size: 12px;
      color: var(--art-gray-600);
      background: var(--art-gray-100);
      border-radius: 10px;
    }
  }

  .release-group__body {
    display: grid;
    gap: 8px;
    padding-left: 14px;
    margin-left: 10px;
    border-left: 1px dashed var(--art-border-color);
  }

  .release-item {
    background: var(--art-gray-100);
    border: 1px solid var(--art-border-color);
    border-radius: 10px;
    transition: border-color 0.2s ease;

    &.open {
      background: var(--default-box-color);
      border-color: var(--el-color-primary-light-5);
    }
  }

  .release-row {
    display: flex;
    gap: 6px 10px;
    align-items: center;
    width: 100%;
    min-height: 44px;
    padding: 10px 14px;
    font: inherit;
    text-align: left;
    cursor: pointer;
    background: none;
    border: none;
  }

  .release-row__main {
    display: flex;
    flex-shrink: 0;
    flex-wrap: wrap;
    gap: 6px 8px;
    align-items: center;

    strong {
      font-size: 15px;
      color: var(--art-gray-900);
    }
  }

  .release-row__date {
    font-size: 12px;
    color: var(--art-gray-500);
  }

  .release-row__summary {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    font-size: 13px;
    color: var(--art-gray-600);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .release-notes {
    display: grid;
    gap: 8px;
    padding: 0 16px 14px 38px;
  }

  .release-note {
    display: flex;
    gap: 9px;
    align-items: flex-start;
    font-size: 13px;
    line-height: 1.6;
    color: var(--art-gray-700);
  }

  .release-note-dot {
    flex: 0 0 5px;
    width: 5px;
    height: 5px;
    margin-top: 8px;
    background: var(--el-color-primary);
    border-radius: 50%;
  }

  .history-more {
    min-height: 8px;
    padding: 12px 0 4px;
    font-size: 12px;
    color: var(--art-gray-500);
    text-align: center;
  }

  .history-backtop {
    position: fixed;
    right: 28px;
    bottom: 32px;
    z-index: 20;
    display: flex;
    gap: 6px;
    align-items: center;
    height: 40px;
    padding: 0 16px;
    font: inherit;
    font-size: 13px;
    color: #fff;
    cursor: pointer;
    background: var(--el-color-primary);
    border: none;
    border-radius: 20px;
    box-shadow: 0 6px 18px var(--el-color-primary-light-5);
  }

  @media (max-width: 768px) {
    // 手机上顶栏和标签栏固定在上方，回到顶部时让出它们的高度
    .history-section {
      scroll-margin-top: 120px;
    }

    .history-search {
      max-width: none;
    }

    .release-group__body {
      padding-left: 8px;
      margin-left: 6px;
    }

    .release-row {
      flex-wrap: wrap;
      padding: 10px 12px;
    }

    // 版本号和标签放不下时在自己这一块里换行，箭头始终在同一行
    .release-row__main {
      flex: 1 1 0;
      min-width: 0;
    }

    .release-row__summary {
      flex-basis: 100%;
      padding-left: 22px;
      white-space: normal;
      display: -webkit-box;
      -webkit-line-clamp: 2;
      -webkit-box-orient: vertical;
    }

    .release-notes {
      padding: 0 12px 12px 34px;
    }

    .history-backtop {
      right: 16px;
      bottom: 20px;
      justify-content: center;
      width: 44px;
      height: 44px;
      padding: 0;
      border-radius: 50%;

      span {
        display: none;
      }
    }
  }
</style>
