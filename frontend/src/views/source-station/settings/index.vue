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

    <el-card v-loading="storeLoading" shadow="never" class="art-card store-card">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">商店预留</span>
            <p class="card-hint">
              只保存产品应用标识和免费套餐 ID，供后续购买与交付使用。这里不校验应用或套餐是否存在，也不开放付费上架。
            </p>
          </div>
          <div class="table-actions">
            <el-button type="primary" :loading="storeSaving" @click="handleStoreSave">保存</el-button>
          </div>
        </div>
      </template>
      <el-form :model="storeForm" label-width="140px" class="settings-form">
        <el-form-item label="产品应用标识">
          <el-input v-model.trim="storeForm.productAppKey" placeholder="可留空" />
        </el-form-item>
        <el-form-item label="免费套餐 ID">
          <el-input v-model.trim="storeForm.freePlanId" placeholder="可留空，正整数" />
        </el-form-item>
        <el-form-item label="离线宽限天数">
          <el-input-number v-model="storeForm.graceDays" :min="1" :max="30" />
        </el-form-item>
        <el-form-item label="改密撤销绑定">
          <el-switch v-model="storeForm.revokeOnPasswordChange" />
        </el-form-item>
        <el-form-item label="商业版功能键">
          <el-input v-model.trim="storeForm.commercialFeatures" placeholder="multi_app" />
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
    fetchSourceStoreSettings,
    saveSourceReleaseSettings,
    saveSourceStoreSettings,
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
  const storeLoading = ref(false)
  const storeSaving = ref(false)
  const storeForm = reactive({
    productAppKey: '',
    freePlanId: '',
    graceDays: 7,
    revokeOnPasswordChange: true,
    commercialFeatures: 'multi_app'
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

  async function loadStoreSettings() {
    storeLoading.value = true
    try {
      const data = await fetchSourceStoreSettings()
      storeForm.productAppKey = data.productAppKey || ''
      storeForm.freePlanId = data.freePlanId || ''
      storeForm.graceDays = data.graceDays || 7
      storeForm.revokeOnPasswordChange = data.revokeOnPasswordChange !== false
      storeForm.commercialFeatures = (data.commercialFeatures || ['multi_app']).join(',')
    } finally {
      storeLoading.value = false
    }
  }

  async function handleStoreSave() {
    const appKey = storeForm.productAppKey.trim()
    const plan = storeForm.freePlanId.trim()
    if (appKey && !/^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/.test(appKey)) {
      ElMessage.warning('产品应用标识不合法')
      return
    }
    if (plan && !/^[1-9]\d*$/.test(plan)) {
      ElMessage.warning('免费套餐 ID 须为正整数')
      return
    }
    storeSaving.value = true
    try {
      const data = await saveSourceStoreSettings({
        productAppKey: appKey,
        freePlanId: plan,
        graceDays: storeForm.graceDays,
        revokeOnPasswordChange: storeForm.revokeOnPasswordChange,
        commercialFeatures: storeForm.commercialFeatures.split(',').map((item) => item.trim()).filter(Boolean)
      })
      storeForm.productAppKey = data.productAppKey || ''
      storeForm.freePlanId = data.freePlanId || ''
      storeForm.graceDays = data.graceDays || 7
      storeForm.revokeOnPasswordChange = data.revokeOnPasswordChange !== false
      storeForm.commercialFeatures = (data.commercialFeatures || ['multi_app']).join(',')
      ElMessage.success('已保存商店设置')
    } finally {
      storeSaving.value = false
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
    void loadStoreSettings()
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

  .store-card {
    margin-top: 16px;
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
