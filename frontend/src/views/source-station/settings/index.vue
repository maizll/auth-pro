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
            <span class="card-title">收费插件只读令牌</span>
            <p class="card-hint">
              收费条目放在私有 GitHub
              仓库时，本站只用这一枚只读令牌核对安装包，并在买家付款后换取几分钟有效的下载地址。令牌加密保存在服务器上，页面只显示已配置或未配置，不回显明文。
            </p>
          </div>
          <div class="table-actions">
            <el-button :loading="githubTesting" @click="handleGitHubTest">测试令牌</el-button>
            <el-button type="primary" :loading="githubSaving" @click="handleGitHubSave"
              >保存令牌</el-button
            >
          </div>
        </div>
      </template>
      <el-alert
        :title="githubConfigured ? '已配置只读令牌' : '尚未配置只读令牌'"
        :type="githubConfigured ? 'success' : 'warning'"
        :closable="false"
        show-icon
        class="mb-4"
      >
        在 GitHub 创建 fine-grained personal access token，权限只勾选目标私有仓库的 Contents:
        Read-only。不要用可推送的令牌。
      </el-alert>
      <el-form label-width="140px" class="settings-form">
        <el-form-item label="只读令牌">
          <el-input
            v-model="githubToken"
            type="password"
            show-password
            :placeholder="githubConfigured ? '已配置，留空不修改' : '粘贴只读令牌'"
          />
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, reactive, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import {
    fetchGitHubPaidToken,
    fetchSourceReleaseSettings,
    saveGitHubPaidToken,
    saveSourceReleaseSettings,
    testGitHubPaidToken,
    testSourceReleaseSettings,
    type SourceReleaseSettings
  } from '@/api/source-station'

  const loading = ref(false)
  const saving = ref(false)
  const testing = ref(false)
  const githubLoading = ref(false)
  const githubSaving = ref(false)
  const githubTesting = ref(false)
  const githubConfigured = ref(false)
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
      githubToken.value = ''
    } finally {
      githubLoading.value = false
    }
  }

  async function handleGitHubSave() {
    githubSaving.value = true
    try {
      const data = await saveGitHubPaidToken(githubToken.value.trim())
      githubConfigured.value = Boolean(data.configured)
      githubToken.value = ''
      ElMessage.success(githubConfigured.value ? '已保存只读令牌' : '已保留现有令牌')
    } finally {
      githubSaving.value = false
    }
  }

  async function handleGitHubTest() {
    githubTesting.value = true
    try {
      await testGitHubPaidToken(githubToken.value.trim())
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

  onMounted(() => {
    void loadSettings()
    void loadGitHubToken()
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

  .github-paid-card {
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
