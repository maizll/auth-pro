<!-- 存储检查：定时任务的结果。出问题只提醒，不改目录上下架。 -->
<template>
  <div class="storage-monitor art-full-height">
    <ElCard class="art-table-card" shadow="never">
      <ArtTableHeader :loading="loading" @refresh="loadHealth">
        <template #left>
          <ElSpace wrap>
            <ElButton type="primary" :loading="running" @click="runNow">立即检查</ElButton>
            <span class="hint">{{ intervalText }}</span>
          </ElSpace>
        </template>
      </ArtTableHeader>
      <ElAlert class="page-alert" type="info" :closable="false" show-icon :title="summary" />
      <ArtTable :loading="loading" :data="rows" :columns="columns">
        <template #level="{ row }">
          <ElTag :type="levelTag(row.level)" size="small">{{ levelText(row.level) }}</ElTag>
        </template>
        <template #locationName="{ row }">
          <span class="cell-one-line">{{ row.locationName || '-' }}</span>
        </template>
        <template #target="{ row }">
          <span class="cell-one-line">{{ row.target || '-' }}</span>
        </template>
        <template #message="{ row }">
          <div class="message-cell">
            <span class="cell-one-line" :title="row.message">{{ row.message }}</span>
            <ElButton
              v-if="row.action === 'make_private'"
              v-roles="'R_SUPER'"
              class="message-action"
              size="small"
              :type="row.official ? 'default' : 'primary'"
              plain
              @click="openMakePrivate(row)"
            >
              改为私有
            </ElButton>
          </div>
        </template>
      </ArtTable>
    </ElCard>

    <AppDialog v-model="privateVisible" :title="dialogTitle" size="md" flow="long" destroy-on-close>
      <div v-loading="checking" class="private-body">
        <template v-if="check">
          <p class="repo-line">
            <span class="repo-kind">{{ check.kind === 'gitee' ? 'Gitee' : 'GitHub' }}</span>
            <span class="repo-name">{{ fullName }}</span>
          </p>

          <template v-if="check.official">
            <ElAlert type="info" :closable="false" show-icon :title="check.reason" />
            <p class="section-title">改私有步骤</p>
            <ol class="plain-list">
              <li v-for="item in check.steps" :key="item">{{ item }}</li>
            </ol>
            <p class="section-title">现有令牌改私有后能不能读</p>
            <ul v-if="check.tokens?.length" class="token-list">
              <li v-for="item in check.tokens" :key="item.label">
                <ElTag :type="tokenTag(item.state)" size="small">{{ tokenText(item.state) }}</ElTag>
                <span class="token-label">{{ item.label }}</span>
                <span class="token-note">{{ item.note }}</span>
              </li>
            </ul>
            <p v-else class="muted">存储管理里还没有 GitHub 令牌。</p>
            <p class="muted">{{ check.summary }}</p>
          </template>

          <template v-else>
            <p class="section-title">改为私有后</p>
            <ul class="plain-list">
              <li v-for="item in check.consequences" :key="item">{{ item }}</li>
            </ul>
            <p v-if="check.allowed" class="muted">将使用{{ check.tokenLabel }}修改。</p>
            <ElAlert v-else type="error" :closable="false" show-icon :title="check.reason" />
            <ElForm v-if="check.allowed" label-position="top" class="confirm-form" @submit.prevent>
              <ElFormItem :label="`输入仓库名 ${fullName} 确认`">
                <ElInput v-model="confirmText" :placeholder="fullName" autocomplete="off" />
              </ElFormItem>
            </ElForm>
            <ElAlert v-if="errorText" type="error" :closable="false" show-icon :title="errorText" />
          </template>
        </template>
      </div>
      <template #footer>
        <template v-if="check && !check.official && check.allowed">
          <ElButton @click="privateVisible = false">取消</ElButton>
          <ElButton
            type="danger"
            :disabled="confirmText.trim() !== fullName"
            :loading="submitting"
            @click="submitMakePrivate"
          >
            改为私有
          </ElButton>
        </template>
        <template v-else>
          <ElButton v-if="check?.official" :loading="checking" @click="reloadCheck"
            >重新检查令牌</ElButton
          >
          <ElButton type="primary" @click="privateVisible = false">知道了</ElButton>
        </template>
      </template>
    </AppDialog>
  </div>
</template>

<script setup lang="ts">
  import AppDialog from '@/components/core/dialog/AppDialog.vue'
  import { useTableColumns } from '@/hooks/core/useTableColumns'
  import {
    checkMakeRepoPrivate,
    fetchStorageHealth,
    makeRepoPrivate,
    runStorageHealth,
    type MakePrivateCheck,
    type StorageHealthRow
  } from '@/api/source-station'
  import { formatLocalDateTime } from '@/utils/local-time'

  defineOptions({ name: 'SourceStationStorageMonitor' })

  const loading = ref(false)
  const running = ref(false)
  const rows = ref<StorageHealthRow[]>([])
  const checkedAt = ref('')
  const intervalText = ref('每小时检查一次。出问题只提醒，不会自动下架。')

  const summary = computed(() => {
    const when = formatLocalDateTime(checkedAt.value)
    const problems = rows.value.filter((row) => row.level === 'problem').length
    if (!when) return '还没有检查记录。点「立即检查」会连上各存储并确认安装包还在。'
    if (!problems) return `${when} 检查完成，存储可以连接，已登记的安装包都在。`
    return `${when} 发现 ${problems} 个问题。目录不会因此下架，请按说明处理。`
  })

  const { columns } = useTableColumns<StorageHealthRow>(() => [
    { prop: 'level', label: '结果', width: 90, useSlot: true },
    { prop: 'locationName', label: '存储', minWidth: 140, useSlot: true },
    { prop: 'target', label: '对象', minWidth: 140, useSlot: true },
    { prop: 'message', label: '说明', minWidth: 240, useSlot: true }
  ])

  function apply(data: { checkedAt?: string; list: StorageHealthRow[] | null; interval?: string }) {
    rows.value = data.list || []
    checkedAt.value = data.checkedAt || ''
    if (data.interval) intervalText.value = data.interval
  }

  function levelTag(level: string) {
    if (level === 'problem') return 'danger'
    if (level === 'notice') return 'info'
    return 'success'
  }

  function levelText(level: string) {
    if (level === 'problem') return '有问题'
    if (level === 'notice') return '提醒'
    return '正常'
  }

  function tokenTag(state: string) {
    if (state === 'yes') return 'success'
    if (state === 'no') return 'danger'
    return 'info'
  }

  function tokenText(state: string) {
    if (state === 'yes') return '能读'
    if (state === 'no') return '读不到'
    return '待确认'
  }

  // 改为私有：先问服务端后果和要用的令牌，再要求输入完整仓库名确认。
  const privateVisible = ref(false)
  const checking = ref(false)
  const submitting = ref(false)
  const target = ref<StorageHealthRow | null>(null)
  const check = ref<MakePrivateCheck | null>(null)
  const confirmText = ref('')
  const errorText = ref('')
  const fullName = computed(() =>
    target.value ? `${target.value.owner}/${target.value.repo}` : ''
  )
  const dialogTitle = computed(() =>
    check.value?.official ? '官网更新来源改为私有' : '把仓库改为私有'
  )

  function targetPayload() {
    const row = target.value
    return { kind: row?.kind || '', owner: row?.owner || '', repo: row?.repo || '' }
  }

  async function reloadCheck() {
    checking.value = true
    try {
      check.value = await checkMakeRepoPrivate(targetPayload())
    } finally {
      checking.value = false
    }
  }

  function openMakePrivate(row: StorageHealthRow) {
    target.value = row
    check.value = null
    confirmText.value = ''
    errorText.value = ''
    privateVisible.value = true
    reloadCheck().catch(() => {
      privateVisible.value = false
    })
  }

  async function submitMakePrivate() {
    submitting.value = true
    errorText.value = ''
    try {
      apply(await makeRepoPrivate(targetPayload(), confirmText.value.trim()))
      privateVisible.value = false
    } catch (error) {
      errorText.value = error instanceof Error ? error.message : '修改失败，请稍后再试'
    } finally {
      submitting.value = false
    }
  }

  async function loadHealth() {
    loading.value = true
    try {
      apply(await fetchStorageHealth())
    } finally {
      loading.value = false
    }
  }

  async function runNow() {
    running.value = true
    try {
      apply(await runStorageHealth())
    } finally {
      running.value = false
    }
  }

  onMounted(loadHealth)
</script>

<style scoped lang="scss">
  .page-alert {
    margin-bottom: 12px;
  }

  .hint {
    color: var(--art-gray-600);
    font-size: 13px;
  }

  .message-cell {
    display: flex;
    gap: 8px;
    align-items: center;
    min-width: 0;
  }

  .message-cell .cell-one-line {
    flex: 1;
    min-width: 0;
  }

  .message-action {
    flex-shrink: 0;
  }

  .private-body {
    min-height: 120px;
    font-size: 14px;
    line-height: 1.7;
  }

  .repo-line {
    display: flex;
    gap: 8px;
    align-items: center;
    margin: 0 0 12px;
  }

  .repo-kind {
    padding: 0 8px;
    color: var(--art-gray-600);
    font-size: 12px;
    border: 1px solid var(--default-border);
    border-radius: 4px;
  }

  .repo-name {
    font-weight: 600;
    word-break: break-all;
  }

  .section-title {
    margin: 16px 0 6px;
    font-weight: 600;
  }

  .plain-list {
    padding-left: 20px;
    margin: 0;
    color: var(--art-gray-700);
    list-style: disc;
  }

  ol.plain-list {
    list-style: decimal;
  }

  .token-list {
    padding: 0;
    margin: 0;
    list-style: none;

    li {
      display: flex;
      flex-wrap: wrap;
      gap: 6px 8px;
      align-items: baseline;
      padding: 8px 0;
      border-bottom: 1px dashed var(--default-border);
    }
  }

  .token-label {
    font-weight: 500;
  }

  .token-note,
  .muted {
    color: var(--art-gray-600);
    font-size: 13px;
  }

  .confirm-form {
    margin-top: 12px;
  }

  .cell-one-line {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
