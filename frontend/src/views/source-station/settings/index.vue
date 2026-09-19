<template>
  <div class="source-station-page">
    <el-card v-loading="loading" shadow="never" class="art-card">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">Release / 仓库 Token</span>
            <p class="card-hint">
              校验通过的 ZIP 可推送到 GitHub/Gitee Release，源站只保存附件 https 地址与
              sha256。令牌仅保存在服务端，GET 只返回掩码。
            </p>
          </div>
          <el-button type="primary" :loading="saving" @click="handleSave">保存设置</el-button>
        </div>
      </template>

      <el-alert
        :title="
          settings.configured
            ? '已配置仓库与令牌，上传时默认推送 Release'
            : '尚未配置完整，上传时需粘贴外部 https 地址'
        "
        :type="settings.configured ? 'success' : 'warning'"
        :closable="false"
        show-icon
        class="mb-4"
      />

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
  import { onMounted, reactive, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import {
    fetchSourceReleaseSettings,
    saveSourceReleaseSettings,
    type SourceReleaseSettings
  } from '@/api/source-station'

  const loading = ref(false)
  const saving = ref(false)
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

  async function handleSave() {
    saving.value = true
    try {
      const data = await saveSourceReleaseSettings({
        provider: form.provider,
        owner: form.owner,
        repo: form.repo,
        token: form.token,
        tagStrategy: form.tagStrategy,
        branch: form.branch
      })
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
