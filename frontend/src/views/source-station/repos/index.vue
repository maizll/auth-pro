<!-- 每个应用一行。状态和最近一次检查分开。本站在线更新不在这张表里。 -->
<template>
  <div class="repo-page">
    <ElAlert v-if="note" type="info" :closable="false" :title="note" />
    <ElAlert v-if="problem" class="gap" type="error" :closable="false" :title="problem" />
    <ElCard>
      <template #header>
        <strong>仓库绑定</strong>
      </template>
      <ElEmpty v-if="loaded && !list.length" :description="emptyText" />
      <ElTable v-else :data="list">
        <ElTableColumn prop="name" label="应用" min-width="120" />
        <ElTableColumn label="仓库" min-width="180">
          <template #default="{ row }">{{ row.repo || '未绑定' }}</template>
        </ElTableColumn>
        <ElTableColumn label="状态" width="100">
          <template #default="{ row }">
            <ElTag :type="tagType(row.status)">{{ statusText(row.status) }}</ElTag>
          </template>
        </ElTableColumn>
        <ElTableColumn prop="health" label="检查" min-width="180" />
        <ElTableColumn label="" width="160" align="right">
          <template #default="{ row }">
            <ElButton v-if="row.status === 'unbound'" type="primary" @click="openBind(row)"
              >绑定</ElButton
            >
            <template v-else>
              <ElButton @click="openRebind(row)">更换</ElButton>
              <ElButton @click="openUnbind(row)">解除</ElButton>
            </template>
          </template>
        </ElTableColumn>
      </ElTable>
      <p class="foot">本站程序的在线更新不在这里配置。</p>
    </ElCard>

    <AppDialog v-model="bindOpen" title="绑定仓库" size="md" flow="long">
      <ElAlert v-if="!tokenReady" type="error" :closable="false" :title="tokenMessage" />
      <ElRadioGroup v-model="action" class="choices">
        <ElRadio value="create" border :disabled="!tokenReady">自动创建私有仓库</ElRadio>
        <ElRadio value="bind" border :disabled="!tokenReady">绑定已有仓库</ElRadio>
      </ElRadioGroup>
      <ElInput v-model.trim="repo" class="gap" placeholder="所有者/仓库" :disabled="!tokenReady" />
      <template #footer>
        <ElButton @click="bindOpen = false">取消</ElButton>
        <ElButton type="primary" :disabled="!tokenReady" :loading="saving" @click="submitBind"
          >绑定</ElButton
        >
      </template>
    </AppDialog>

    <AppDialog v-model="previewOpen" title="更换仓库" size="lg" flow="long">
      <p>{{ current?.name }} · 从 {{ current?.repo }} 到 {{ repo || '新仓库' }}</p>
      <ElDescriptions :column="narrow ? 1 : 2" border>
        <ElDescriptionsItem v-for="group in groups" :key="group.prefix" :label="group.prefix">
          {{ group.text }}
        </ElDescriptionsItem>
      </ElDescriptions>
      <ElAlert
        class="gap"
        type="info"
        :closable="false"
        title="只切换时，已发布文件不移动。复制并核对通过后才改地址。"
      />
      <template #footer>
        <ElButton :disabled="saving" @click="submitRebind('switch')">只切换</ElButton>
        <ElButton
          type="primary"
          :disabled="!!previewEmpty"
          :loading="saving"
          @click="submitRebind('copy')"
          >复制并核对</ElButton
        >
      </template>
    </AppDialog>

    <AppDialog v-model="unbindOpen" title="解除绑定" size="sm" flow="short">
      <ElDescriptions :column="1" border>
        <ElDescriptionsItem label="已发布">{{ impact.published }}</ElDescriptionsItem>
        <ElDescriptionsItem label="草稿">{{ impact.drafts }}</ElDescriptionsItem>
      </ElDescriptions>
      <p v-if="blocked" class="gap">已发布的版本还依赖这个仓库，请先迁到新仓库。</p>
      <p v-else-if="impact.drafts > 0" class="gap"
        >还有未发布的草稿。解除后草稿留在远程，本地不再写入新文件。</p
      >
      <p v-else class="gap">解除后远程仓库仍保留。新的导入和上传不再写入它。</p>
      <template #footer>
        <ElButton @click="unbindOpen = false">取消</ElButton>
        <ElButton v-if="blocked" type="primary" @click="moveInstead">先迁到新仓库</ElButton>
        <ElButton
          v-else
          type="danger"
          :loading="saving"
          :disabled="impact.busy"
          @click="submitUnbind"
          >解除绑定</ElButton
        >
      </template>
    </AppDialog>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import AppDialog from '@/components/core/dialog/AppDialog.vue'
  import {
    bindAppRepo,
    fetchAppRepoImpact,
    fetchAppRepos,
    fetchAppRepoToken,
    previewAppRepo,
    rebindAppRepo,
    unbindAppRepo,
    type AppRepoItem
  } from '@/api/app-repo'

  defineOptions({ name: 'SourceStationRepos' })

  const list = ref<AppRepoItem[]>([])
  const note = ref('')
  const problem = ref('')
  const emptyText = ref('还没有应用。请先创建应用。')
  const loaded = ref(false)
  const tokenReady = ref(false)
  const tokenMessage = ref('还没有可用的令牌。请到存储管理添加。也可以先创建应用，稍后再绑定。')
  const bindOpen = ref(false)
  const previewOpen = ref(false)
  const unbindOpen = ref(false)
  const blocked = ref(false)
  const saving = ref(false)
  const action = ref<'create' | 'bind'>('create')
  const repo = ref('')
  const current = ref<AppRepoItem | null>(null)
  const groups = ref<{ prefix: string; text: string }[]>([])
  const previewEmpty = ref('')
  const narrow = ref(window.innerWidth < 768)
  const impact = ref({ published: 0, drafts: 0, busy: false })

  function statusText(status: string) {
    if (status === 'ready') return '正常'
    if (status === 'degraded') return '异常'
    return '未绑定'
  }

  function tagType(status: string) {
    if (status === 'ready') return 'success'
    if (status === 'degraded') return 'danger'
    return 'info'
  }

  async function load() {
    const data = await fetchAppRepos()
    list.value = data?.list || []
    note.value = data?.note || ''
    emptyText.value = data?.empty || '还没有应用。请先创建应用。'
    problem.value = list.value.find((item) => item.status === 'degraded')
      ? `${list.value.find((item) => item.status === 'degraded')?.name}的令牌已失效。绑定仍保留，请到存储管理更新。`
      : ''
    loaded.value = true
    const token = await fetchAppRepoToken()
    tokenReady.value = !!token?.ready
    if (token?.message) tokenMessage.value = token.message
  }

  function openBind(row: AppRepoItem) {
    current.value = row
    action.value = tokenReady.value ? 'create' : 'bind'
    repo.value = ''
    bindOpen.value = true
  }

  async function openRebind(row: AppRepoItem) {
    current.value = row
    repo.value = ''
    const data = await previewAppRepo(row.appId, row.repo)
    groups.value = data?.groups || []
    previewEmpty.value = data?.empty || ''
    previewOpen.value = true
  }

  async function openUnbind(row: AppRepoItem) {
    current.value = row
    blocked.value = false
    impact.value = { published: 0, drafts: 0, busy: false }
    try {
      const data = await fetchAppRepoImpact(row.appId)
      impact.value = {
        published: data?.published || 0,
        drafts: data?.drafts || 0,
        busy: !!data?.busy
      }
      blocked.value = !!data?.blocked || impact.value.published > 0
    } catch (error) {
      const message = error instanceof Error ? error.message : '暂时无法检查依赖'
      ElMessage.error(message)
      return
    }
    unbindOpen.value = true
  }

  function moveInstead() {
    unbindOpen.value = false
    if (current.value) void openRebind(current.value)
  }

  async function submitBind() {
    if (!current.value) return
    saving.value = true
    try {
      await bindAppRepo({ appId: current.value.appId, action: action.value, repo: repo.value })
      bindOpen.value = false
      await load()
    } finally {
      saving.value = false
    }
  }

  async function submitRebind(mode: 'switch' | 'copy') {
    if (!current.value) return
    saving.value = true
    try {
      await rebindAppRepo({ appId: current.value.appId, repo: repo.value, mode })
      previewOpen.value = false
      await load()
    } finally {
      saving.value = false
    }
  }

  async function submitUnbind() {
    if (!current.value) return
    saving.value = true
    try {
      await unbindAppRepo(current.value.appId)
      ElMessage.success('已解除绑定。远程仓库仍保留。')
      unbindOpen.value = false
      await load()
    } finally {
      saving.value = false
    }
  }

  onMounted(load)
</script>

<style scoped>
  .repo-page {
    padding: 16px;
  }

  .gap {
    margin-top: 12px;
  }

  .choices {
    display: flex;
    flex-direction: column;
    gap: 8px;
    align-items: stretch;
    margin-top: 12px;
  }

  .foot {
    margin: 8px 0 0;
    color: #6b7686;
    font-size: 12px;
  }
</style>
