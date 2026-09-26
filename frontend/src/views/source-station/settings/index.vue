<template>
  <div class="source-station-page">
    <el-card v-loading="loading" shadow="never" class="art-card">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">发布仓库令牌</span>
            <p class="card-hint">
              校验通过的压缩包可以推送到 GitHub 或 Gitee
              的发布页。源站只保存下载地址和校验码。令牌只存在服务器上，页面只显示掩码。「测试连接」只检查仓库和令牌是否可用，不会创建发布。
            </p>
          </div>
          <div class="table-actions">
            <el-button :loading="testing" @click="handleTest">测试连接</el-button>
            <el-button type="primary" :loading="saving" @click="handleSave">保存设置</el-button>
          </div>
        </div>
      </template>

      <el-alert
        :title="alertTitle"
        :type="formReady ? 'success' : 'warning'"
        :closable="false"
        show-icon
        class="mb-4"
      >
        {{ alertDescription }}
      </el-alert>

      <el-form :model="form" label-width="140px" class="settings-form">
        <el-form-item label="代码平台">
          <el-radio-group v-model="form.provider">
            <el-radio value="github">GitHub</el-radio>
            <el-radio value="gitee">Gitee</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="所有者">
          <el-input v-model.trim="form.owner" placeholder="组织或用户名" />
        </el-form-item>
        <el-form-item label="仓库">
          <el-input v-model.trim="form.repo" placeholder="仓库名" />
        </el-form-item>
        <el-form-item label="访问令牌">
          <el-input
            v-model="form.token"
            type="password"
            show-password
            :placeholder="
              settings.hasToken ? `已设置 ${settings.tokenMasked}，留空不修改` : '仅服务端保存'
            "
          />
        </el-form-item>
        <el-form-item label="版本标签">
          <el-input v-model.trim="form.tagStrategy" placeholder="{id}-{version}" />
          <p class="field-hint">一般保持默认即可，用来给每次发布命名。</p>
        </el-form-item>
        <el-form-item v-if="form.provider === 'gitee'" label="Gitee 分支">
          <el-input v-model.trim="form.branch" placeholder="master" />
        </el-form-item>
      </el-form>
    </el-card>

    <el-card v-loading="githubLoading" shadow="never" class="art-card github-paid-card">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">收费仓库</span>
            <p class="card-hint">
              站长和开发者的收费安装包都放在这一个私有 GitHub 仓库。令牌需要 Contents 读写，用来上传
              Release 资产，并在买家付款后换取几分钟有效的下载地址。令牌加密保存，页面不回显明文。
            </p>
          </div>
          <div class="table-actions">
            <el-button :loading="githubTesting" @click="handleGitHubTest">测试令牌</el-button>
            <el-button type="primary" :loading="githubSaving" @click="handleGitHubSave"
              >保存</el-button
            >
          </div>
        </div>
      </template>
      <el-alert
        :title="githubConfigured ? '已配置收费仓库' : '尚未配置收费仓库'"
        :type="githubConfigured ? 'success' : 'warning'"
        :closable="false"
        show-icon
        class="mb-4"
      >
        {{
          githubReminder ||
          '在 GitHub 创建 fine-grained personal access token，只授权这一个私有仓库，Contents 选 Read and write。'
        }}
      </el-alert>
      <el-form label-width="140px" class="settings-form">
        <el-form-item label="所有者">
          <el-input v-model.trim="githubOwner" placeholder="例如 my-org" />
        </el-form-item>
        <el-form-item label="仓库">
          <el-input v-model.trim="githubRepo" placeholder="例如 paid-plugins" />
        </el-form-item>
        <el-form-item label="读写令牌">
          <el-input
            v-model="githubToken"
            type="password"
            show-password
            :placeholder="githubConfigured ? '已配置，留空不修改' : '粘贴 Contents 读写令牌'"
          />
        </el-form-item>
      </el-form>
    </el-card>

    <el-card v-loading="aliasLoading" shadow="never" class="art-card alias-card">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">旧应用软件源地址</span>
            <p class="card-hint">
              1.6.1 之前已经删掉的应用，可以在这里把旧标识转到还在使用的应用。旧地址
              /software-source/旧标识/index.json
              会返回目标应用的目录，已经订阅的客户端不用改。同一个旧标识再保存一次会更新目标，不会多出一条。正在使用的应用标识不能转走，请先归档。
            </p>
          </div>
          <div class="table-actions">
            <el-button type="primary" :loading="aliasSaving" @click="handleAliasSave">保存映射</el-button>
          </div>
        </div>
      </template>
      <el-form label-width="140px" class="settings-form">
        <el-form-item label="旧应用标识">
          <el-input v-model.trim="aliasForm.oldAppKey" placeholder="例如 app_f93896d80066_5811" />
        </el-form-item>
        <el-form-item label="转到应用">
          <el-select v-model="aliasForm.targetAppId" placeholder="选择还在使用的应用" class="product-app-select">
            <el-option
              v-for="app in liveAliasApps"
              :key="app.id"
              :label="`${app.name}（${app.appKey}）`"
              :value="app.id"
            />
          </el-select>
          <p v-if="!liveAliasApps.length" class="field-hint">还没有可接收地址的应用。</p>
        </el-form-item>
      </el-form>
      <el-table :data="aliases" size="small" empty-text="还没有旧地址映射" @row-click="fillAlias">
        <el-table-column prop="oldAppKey" label="旧标识" min-width="180" show-overflow-tooltip />
        <el-table-column label="目标应用" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            {{ row.targetName || '应用' }}（{{ row.targetAppKey || row.targetAppId }}）
          </template>
        </el-table-column>
        <el-table-column prop="indexUrl" label="仍可用的地址" min-width="220" show-overflow-tooltip />
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, reactive, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import {
    fetchGitHubPaidToken,
    fetchSoftwareSourceAliases,
    fetchSourceCatalogApps,
    fetchSourceReleaseSettings,
    saveGitHubPaidToken,
    saveSoftwareSourceAlias,
    saveSourceReleaseSettings,
    testGitHubPaidToken,
    testSourceReleaseSettings,
    type SoftwareSourceAlias,
    type SourceCatalogApp,
    type SourceReleaseSettings
  } from '@/api/source-station'

  const loading = ref(false)
  const saving = ref(false)
  const testing = ref(false)
  const githubLoading = ref(false)
  const githubSaving = ref(false)
  const githubTesting = ref(false)
  const githubConfigured = ref(false)
  const githubOwner = ref('')
  const githubRepo = ref('')
  const githubReminder = ref('')
  const githubToken = ref('')
  const settings = ref<SourceReleaseSettings>({
    provider: 'github',
    owner: '',
    repo: '',
    tagStrategy: '{id}-{version}',
    branch: 'master',
    hasToken: false,
    tokenMasked: '',
    configured: false
  })
  const form = reactive({
    provider: 'github',
    owner: '',
    repo: '',
    token: '',
    tagStrategy: '{id}-{version}',
    branch: 'master'
  })
  const aliasLoading = ref(false)
  const aliasSaving = ref(false)
  const aliases = ref<SoftwareSourceAlias[]>([])
  const aliasApps = ref<SourceCatalogApp[]>([])
  const aliasForm = reactive({
    oldAppKey: '',
    targetAppId: undefined as number | undefined
  })
  const liveAliasApps = computed(() => aliasApps.value.filter((app) => !app.archived))

  const formHasToken = computed(() => Boolean(form.token.trim()) || settings.value.hasToken)

  const missingFields = computed(() => {
    const missing: string[] = []
    const provider = form.provider.trim().toLowerCase()
    if (provider !== 'github' && provider !== 'gitee') missing.push('代码平台')
    if (!form.owner.trim()) missing.push('所有者')
    if (!form.repo.trim()) missing.push('仓库')
    if (!formHasToken.value) missing.push('访问令牌')
    return missing
  })

  const formReady = computed(() => missingFields.value.length === 0)

  const alertTitle = computed(() => {
    if (formReady.value) {
      return '当前表单已齐：上传时可推送到发布页（保存后生效）'
    }
    return `当前表单不完整，缺少：${missingFields.value.join('、')}`
  })

  const alertDescription = computed(() => {
    if (settings.value.configured) {
      return '服务端已保存完整配置。表单改动需点击「保存设置」后才会写入服务端。'
    }
    if (formReady.value) {
      return '服务端尚未保存完整配置，请点击「保存设置」后上传才会推送到发布页。'
    }
    return '尚未配置完整，上传时需粘贴外部 https 地址。'
  })

  async function loadGitHubToken() {
    githubLoading.value = true
    try {
      const data = await fetchGitHubPaidToken()
      githubConfigured.value = Boolean(data.configured)
      githubOwner.value = data.owner || ''
      githubRepo.value = data.repo || ''
      githubReminder.value = data.reminder || ''
      githubToken.value = ''
    } finally {
      githubLoading.value = false
    }
  }

  async function handleGitHubSave() {
    githubSaving.value = true
    try {
      const data = await saveGitHubPaidToken({
        token: githubToken.value.trim(),
        owner: githubOwner.value.trim(),
        repo: githubRepo.value.trim()
      })
      githubConfigured.value = Boolean(data.configured)
      githubOwner.value = data.owner || githubOwner.value
      githubRepo.value = data.repo || githubRepo.value
      githubReminder.value = data.reminder || ''
      githubToken.value = ''
      ElMessage.success(githubConfigured.value ? '已保存收费仓库' : '已保留现有令牌')
    } finally {
      githubSaving.value = false
    }
  }

  async function handleGitHubTest() {
    githubTesting.value = true
    try {
      await testGitHubPaidToken({
        token: githubToken.value.trim(),
        owner: githubOwner.value.trim(),
        repo: githubRepo.value.trim()
      })
      ElMessage.success('令牌可用')
    } finally {
      githubTesting.value = false
    }
  }

  async function loadSettings() {
    loading.value = true
    try {
      const data = await fetchSourceReleaseSettings()
      settings.value = data
      form.provider = data.provider || 'github'
      form.owner = data.owner || ''
      form.repo = data.repo || ''
      form.token = ''
      form.tagStrategy = data.tagStrategy || '{id}-{version}'
      form.branch = data.branch || 'master'
    } finally {
      loading.value = false
    }
  }

  function releasePayload() {
    return {
      provider: form.provider,
      owner: form.owner,
      repo: form.repo,
      token: form.token,
      tagStrategy: form.tagStrategy,
      branch: form.branch
    }
  }

  async function handleTest() {
    if (!form.owner || !form.repo) {
      ElMessage.warning('请填写 Owner 与仓库')
      return
    }
    testing.value = true
    try {
      const data = await testSourceReleaseSettings(releasePayload())
      ElMessage.success(data.htmlUrl ? `连接成功 ${data.htmlUrl}` : '连接成功')
    } finally {
      testing.value = false
    }
  }

  async function handleSave() {
    saving.value = true
    try {
      const data = await saveSourceReleaseSettings(releasePayload())
      settings.value = data
      form.token = ''
      ElMessage.success('已保存 Release 推送设置')
    } finally {
      saving.value = false
    }
  }

  function fillAlias(row: SoftwareSourceAlias) {
    aliasForm.oldAppKey = row.oldAppKey
    aliasForm.targetAppId = row.targetAppId
  }

  async function loadAliases() {
    aliasLoading.value = true
    try {
      const [aliasData, appData] = await Promise.all([
        fetchSoftwareSourceAliases(),
        fetchSourceCatalogApps()
      ])
      aliases.value = aliasData.list || []
      aliasApps.value = appData.list || []
    } finally {
      aliasLoading.value = false
    }
  }

  async function handleAliasSave() {
    const oldAppKey = aliasForm.oldAppKey.trim()
    if (!oldAppKey || !aliasForm.targetAppId) {
      ElMessage.warning('请填写旧应用标识，并选择要转到的应用')
      return
    }
    aliasSaving.value = true
    try {
      await saveSoftwareSourceAlias({ oldAppKey, targetAppId: aliasForm.targetAppId })
      await loadAliases()
    } finally {
      aliasSaving.value = false
    }
  }

  onMounted(() => {
    void loadSettings()
    void loadGitHubToken()
    void loadAliases()
  })
</script>

<style scoped lang="scss">
  .source-station-page {
    padding-bottom: 8px;

    :deep(.el-card) {
      --el-card-border-color: var(--art-card-border);
      border-radius: calc(var(--custom-radius) + 4px);
      background: var(--default-box-color);
      box-shadow: none;
    }
  }

  .table-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
  }

  .table-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
  }

  .card-title {
    font-size: 16px;
    font-weight: 700;
    color: var(--art-gray-900);
  }

  .card-hint {
    margin: 6px 0 0;
    font-size: 13px;
    color: var(--art-gray-600);
    line-height: 1.5;
    max-width: 720px;
  }

  .mb-4 {
    margin-bottom: 16px;
  }

  .github-paid-card,
  .alias-card {
    margin-top: 16px;
  }

  .settings-form {
    max-width: 560px;
  }

  .product-app-select,
  .free-plan-select {
    width: 100%;
  }

  .field-hint {
    margin: 6px 0 0;
    font-size: 12px;
    line-height: 1.5;
    color: var(--art-gray-600);
  }
</style>
