<!-- 绑定已有仓库：从令牌能访问的仓库里搜。创建应用和更换仓库共用这一份。 -->
<template>
  <div class="repo-search">
    <div v-if="status === 'token'" class="repo-search__banner is-bad">
      <p>{{ message }}</p>
      <ElButton link type="primary" @click="goStorage">去存储管理</ElButton>
    </div>
    <div v-else-if="status === 'timeout'" class="repo-search__banner is-bad">
      <p>{{ message }}</p>
      <ElButton link type="primary" @click="reload">重试</ElButton>
    </div>
    <template v-else>
      <ElSelect
        :model-value="modelValue"
        class="repo-search__select"
        filterable
        remote
        reserve-keyword
        clearable
        placeholder="搜索仓库"
        loading-text="正在搜索…"
        no-data-text="没有找到仓库"
        popper-class="repo-search-popper"
        :loading="loading"
        :remote-method="search"
        @update:model-value="pick"
        @visible-change="onOpen"
      >
        <ElOption
          v-for="item in options"
          :key="item.repo"
          :label="item.repo"
          :value="item.repo"
          :disabled="!!item.boundApp"
        >
          <div class="repo-line" :class="{ 'is-taken': item.boundApp }">
            <span class="repo-line__name">{{ item.repo }}</span>
            <span class="repo-line__tag" :class="{ 'is-public': !item.private }">{{
              item.private ? '私有' : '公开'
            }}</span>
            <span class="repo-line__time">{{ formatUpdated(item.updatedAt) }}</span>
          </div>
          <p v-if="item.boundApp" class="repo-line__extra">已被应用「{{ item.boundApp }}」占用</p>
          <p v-else-if="!item.private" class="repo-line__extra">建议改为私有</p>
        </ElOption>
      </ElSelect>
      <p v-if="status === 'empty'" class="repo-search__hint"
        >没有可访问的仓库。可以在下面手动填写。</p
      >
      <p v-else-if="showMiss" class="repo-search__hint">没有找到这个仓库，可以手动填写。</p>
      <p v-if="publicHint" class="repo-search__hint">这是公开仓库，可以绑定，建议改为私有。</p>
      <label v-if="showManual" class="repo-search__manual">
        <span>手动填写</span>
        <ElInput :model-value="manual" placeholder="所有者/仓库" @update:model-value="onManual" />
        <span v-if="manualError" class="repo-search__error">{{ manualError }}</span>
      </label>
    </template>
  </div>
</template>

<script setup lang="ts">
  import { useRouter } from 'vue-router'
  import { fetchGitHubRepoChoices, type GitHubRepoChoice } from '@/api/app-repo'

  defineOptions({ name: 'RepoSearchSelect' })

  const props = withDefaults(
    defineProps<{
      modelValue: string
      appId?: number
    }>(),
    { appId: 0 }
  )

  const emit = defineEmits<{
    'update:modelValue': [string]
  }>()

  const router = useRouter()
  const options = ref<GitHubRepoChoice[]>([])
  const loading = ref(false)
  const status = ref<'ok' | 'empty' | 'token' | 'timeout'>('ok')
  const message = ref('')
  const query = ref('')
  const manual = ref('')
  const manualError = ref('')
  let requestSerial = 0

  const showMiss = computed(
    () =>
      status.value === 'ok' && !!query.value.trim() && !loading.value && options.value.length === 0
  )
  const showManual = computed(() => status.value === 'empty' || showMiss.value)
  const publicHint = computed(() => {
    const current = options.value.find((item) => item.repo === props.modelValue)
    return !!current && !current.private && !current.boundApp
  })

  function formatUpdated(value: string) {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return ''
    const month = date.getUTCMonth() + 1
    const day = date.getUTCDate()
    const hour = String(date.getUTCHours()).padStart(2, '0')
    const minute = String(date.getUTCMinutes()).padStart(2, '0')
    return `${month}月${day}日 ${hour}:${minute}`
  }

  function repoFieldError(raw: string) {
    const value = raw.trim()
    if (!value) return ''
    const parts = value.split('/')
    if (parts.length !== 2 || !parts[0] || !parts[1]) return '请填写仓库，格式为 所有者/仓库'
    const name = /^[A-Za-z0-9._-]+$/
    if (
      !name.test(parts[0]) ||
      !name.test(parts[1]) ||
      parts[0].length > 39 ||
      parts[1].length > 100
    ) {
      return '仓库名只允许字母、数字、点、下划线和连字符'
    }
    return ''
  }

  async function load(keyword: string) {
    const serial = ++requestSerial
    loading.value = true
    if (keyword !== query.value) options.value = []
    query.value = keyword
    try {
      const data = await fetchGitHubRepoChoices(keyword, props.appId)
      if (serial !== requestSerial) return
      status.value = data?.status || 'ok'
      message.value = data?.message || ''
      options.value = data?.list || []
      if (status.value === 'token' || status.value === 'timeout') options.value = []
    } catch {
      if (serial !== requestSerial) return
      status.value = 'timeout'
      message.value = '连接超时，请稍后再试。'
      options.value = []
    } finally {
      if (serial === requestSerial) loading.value = false
    }
  }

  function search(keyword: string) {
    void load(keyword)
  }

  function reload() {
    manualError.value = ''
    void load(query.value)
  }

  function onOpen(open: boolean) {
    if (open && !options.value.length && status.value === 'ok' && !loading.value)
      void load(query.value)
  }

  function pick(value: string) {
    manual.value = ''
    manualError.value = ''
    emit('update:modelValue', value || '')
  }

  function onManual(value: string) {
    manual.value = value
    const error = repoFieldError(value)
    manualError.value = error
    emit('update:modelValue', error ? '' : value.trim())
  }

  function goStorage() {
    void router.push('/source-station/storage')
  }

  onMounted(() => {
    void load('')
  })
</script>

<style scoped>
  .repo-search {
    width: 100%;
  }

  .repo-search__select {
    width: 100%;
  }

  .repo-search__banner,
  .repo-search__manual {
    display: flex;
    flex-direction: column;
    gap: 8px;
    align-items: flex-start;
  }

  .repo-search__banner {
    padding: 10px 12px;
    border-radius: 8px;
  }

  .repo-search__banner.is-bad {
    color: #b44848;
    background: #fef0f0;
  }

  .repo-search__banner p,
  .repo-search__hint,
  .repo-search__manual span {
    margin: 0;
    font-size: 13px;
    line-height: 1.5;
  }

  .repo-search__hint,
  .repo-search__manual > span:first-child {
    color: #6b7686;
  }

  .repo-search__hint {
    margin-top: 8px;
  }

  .repo-search__manual {
    margin-top: 10px;
  }

  .repo-search__error {
    color: #b44848;
  }
</style>

<style>
  .repo-search-popper .el-select-dropdown__item {
    height: auto;
    padding-top: 6px;
    padding-bottom: 6px;
    line-height: 1.4;
    white-space: normal;
  }

  .repo-search-popper .el-select-dropdown__item.is-disabled {
    color: #98a2b3;
  }

  .repo-line {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .repo-line.is-taken,
  .repo-line.is-taken .repo-line__time,
  .repo-line.is-taken .repo-line__tag {
    color: #98a2b3;
  }

  .repo-line__name {
    min-width: 0;
    overflow: hidden;
    font-size: 14px;
    text-overflow: ellipsis;
  }

  .repo-line__tag {
    flex: none;
    padding: 0 6px;
    color: #3d4d66;
    font-size: 12px;
    line-height: 20px;
    background: #f2f4f8;
    border-radius: 4px;
  }

  .repo-line__tag.is-public {
    color: #3a5ccc;
    background: #eef2ff;
  }

  .repo-line__time {
    flex: none;
    margin-left: auto;
    color: #6b7686;
    font-size: 12px;
  }

  .repo-line__extra {
    margin: 2px 0 0;
    color: #6b7686;
    font-size: 12px;
    line-height: 1.4;
  }

  .repo-search-popper .is-disabled .repo-line__extra {
    color: #98a2b3;
  }
</style>
