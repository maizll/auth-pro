<template>
  <div class="source-station-page">
    <el-card v-loading="loading" shadow="never" class="art-card">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">Release / 仓库 Token</span>
            <p class="card-hint">
              校验通过的 ZIP 可推送到 GitHub/Gitee Release，源站只保存附件 https 地址与
              sha256。令牌仅保存在服务端，GET 只返回掩码。「测试连接」验证仓库可达与令牌权限（不创建
              Release）。
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
        <el-form-item label="Provider">
          <el-radio-group v-model="form.provider">
            <el-radio value="github">GitHub</el-radio>
            <el-radio value="gitee">Gitee</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="Owner">
          <el-input v-model.trim="form.owner" placeholder="组织或用户名" />
        </el-form-item>
        <el-form-item label="仓库">
          <el-input v-model.trim="form.repo" placeholder="仓库名" />
        </el-form-item>
        <el-form-item label="Token">
          <el-input
            v-model="form.token"
            type="password"
            show-password
            :placeholder="
              settings.hasToken ? `已设置 ${settings.tokenMasked}，留空不修改` : '仅服务端保存'
            "
          />
        </el-form-item>
        <el-form-item label="Tag 策略">
          <el-input v-model.trim="form.tagStrategy" placeholder="{id}-{version}" />
        </el-form-item>
        <el-form-item v-if="form.provider === 'gitee'" label="Gitee 分支">
          <el-input v-model.trim="form.branch" placeholder="master" />
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, reactive, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import {
    fetchSourceReleaseSettings,
    saveSourceReleaseSettings,
    testSourceReleaseSettings,
    type SourceReleaseSettings
  } from '@/api/source-station'

  const loading = ref(false)
  const saving = ref(false)
  const testing = ref(false)
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
    if (provider !== 'github' && provider !== 'gitee') missing.push('Provider')
    if (!form.owner.trim()) missing.push('Owner')
    if (!form.repo.trim()) missing.push('仓库')
    if (!formHasToken.value) missing.push('Token')
    return missing
  })

  const formReady = computed(() => missingFields.value.length === 0)

  const alertTitle = computed(() => {
    if (formReady.value) {
      return '当前表单已齐：上传时可推送 Release（保存后生效）'
    }
    return `当前表单不完整，缺少：${missingFields.value.join('、')}`
  })

  const alertDescription = computed(() => {
    if (settings.value.configured) {
      return '服务端已保存完整配置。表单改动需点击「保存设置」后才会写入服务端。'
    }
    if (formReady.value) {
      return '服务端尚未保存完整配置，请点击「保存设置」后上传才会推送 Release。'
    }
    return '尚未配置完整，上传时需粘贴外部 https 地址。'
  })

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

  onMounted(loadSettings)
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

  .settings-form {
    max-width: 560px;
  }
</style>
