<template>
  <div v-loading="loading" class="developer-ads">
    <el-card shadow="never" class="mb-5">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">申请广告投放</span>
            <p class="card-hint">
              广告面向客户端，不按应用拆公开目录。提交后由管理员审核；通过后才会生成真实投放记录。图片支持本地上传（PNG
              / JPEG / GIF / WebP，小于 2MB）或粘贴 https:// 外链。
            </p>
          </div>
        </div>
      </template>
      <el-form ref="formRef" :model="form" :rules="rules" label-width="100px" class="ad-form">
        <el-form-item label="关联应用">
          <el-select
            v-model="form.appId"
            clearable
            placeholder="可选，广告本身为全站投放"
            style="width: 100%"
          >
            <el-option
              v-for="app in apps"
              :key="app.id"
              :label="`${app.name}（${app.appKey}）`"
              :value="app.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="标题" prop="title">
          <el-input v-model="form.title" maxlength="120" show-word-limit />
        </el-form-item>
        <el-form-item label="广告位" prop="positions">
          <el-select
            v-model="form.positions"
            multiple
            collapse-tags
            placeholder="至少选一处"
            style="width: 100%"
          >
            <el-option
              v-for="item in AD_POSITIONS"
              :key="item.value"
              :label="item.label"
              :value="item.value"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="广告图" prop="imageUrl">
          <div class="image-field">
            <el-upload
              :auto-upload="false"
              :show-file-list="false"
              accept="image/png,image/jpeg,image/gif,image/webp"
              :disabled="uploadingImage"
              :on-change="handleImageFile"
            >
              <el-button :loading="uploadingImage">本地上传</el-button>
            </el-upload>
            <el-input v-model="form.imageUrl" placeholder="或粘贴 https:// 外链 / 本站已上传地址" />
            <p class="field-hint">支持本地上传或 https 外链，不接受 http:// 或本地路径。</p>
            <img v-if="form.imageUrl" :src="form.imageUrl" alt="" class="image-preview" />
          </div>
        </el-form-item>
        <el-form-item label="跳转 URL">
          <el-input v-model="form.linkUrl" placeholder="https://... 可留空" />
        </el-form-item>
        <el-form-item label="说明">
          <el-input v-model="form.note" type="textarea" :rows="2" maxlength="500" show-word-limit />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="handleSubmit">提交申请</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never">
      <template #header>
        <span class="card-title">我的申请（共 {{ applications.length }} 条）</span>
      </template>
      <el-empty v-if="!applications.length" description="还没有广告申请。" />
      <el-table v-else :data="applications" stripe>
        <el-table-column prop="title" label="标题" min-width="140" show-overflow-tooltip />
        <el-table-column label="广告位" min-width="160">
          <template #default="{ row }">{{ positionLabels(row.positions) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="110" align="center">
          <template #default="{ row }">
            <BizStatusTag domain="ad" :status="row.status" size="small" />
          </template>
        </el-table-column>
        <el-table-column prop="reviewNote" label="审核说明" min-width="160" show-overflow-tooltip />
        <el-table-column prop="advertisementId" label="投放 ID" width="140" show-overflow-tooltip />
        <el-table-column prop="createdAt" label="申请时间" width="180" />
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, reactive, ref } from 'vue'
  import { useRouter } from 'vue-router'
  import type { FormInstance, FormRules, UploadFile } from 'element-plus'
  import { ElMessage } from 'element-plus'
  import { AD_POSITIONS } from '@/api/source-station'
  import {
    DEVELOPER_INFO_KEY,
    DEVELOPER_TOKEN_KEY,
    createSourceDeveloperAdApplication,
    fetchSourceDeveloperAdApplications,
    fetchSourceDeveloperCatalogApps,
    uploadSourceDeveloperAdvertisementImage,
    type SourceDeveloperAdApplication,
    type SourceDeveloperCatalogApp
  } from '@/api/source-developer'

  const router = useRouter()
  const loading = ref(false)
  const saving = ref(false)
  const uploadingImage = ref(false)
  const formRef = ref<FormInstance>()
  const apps = ref<SourceDeveloperCatalogApp[]>([])
  const applications = ref<SourceDeveloperAdApplication[]>([])
  const form = reactive({
    appId: undefined as number | undefined,
    title: '',
    positions: ['home-banner'] as string[],
    imageUrl: '',
    linkUrl: '',
    note: ''
  })
  const rules: FormRules = {
    title: [{ required: true, message: '请填写标题', trigger: 'blur' }],
    positions: [
      {
        type: 'array',
        required: true,
        min: 1,
        message: '请至少选择一个广告位',
        trigger: 'change'
      }
    ],
    imageUrl: [
      {
        validator: (_rule, value, callback) => {
          const raw = String(value || '').trim()
          if (!raw) {
            callback()
            return
          }
          if (/^https:\/\//i.test(raw) || raw.startsWith('/api/v1/public/advertisement-files/')) {
            callback()
            return
          }
          callback(new Error('广告图片地址不合法，支持本地上传或 https 外链'))
        },
        trigger: 'blur'
      }
    ]
  }

  function positionLabels(values?: string[]) {
    if (!values?.length) return '-'
    return values
      .map((value) => AD_POSITIONS.find((item) => item.value === value)?.label || value)
      .join('、')
  }

  function logoutToLogin() {
    localStorage.removeItem(DEVELOPER_TOKEN_KEY)
    localStorage.removeItem(DEVELOPER_INFO_KEY)
    router.replace('/developer-panel/login')
  }

  function unwrap(res: { status?: number; data?: { code?: number; msg?: string } }) {
    if (res.status === 401 || res.data?.code === 401) {
      logoutToLogin()
      return null
    }
    return res.data
  }

  async function loadPage() {
    loading.value = true
    try {
      const [appsRes, listRes] = await Promise.all([
        fetchSourceDeveloperCatalogApps(),
        fetchSourceDeveloperAdApplications()
      ])
      const appsBody = unwrap(appsRes)
      const listBody = unwrap(listRes)
      if (!appsBody || !listBody) return
      if (appsBody.code === 200) {
        apps.value =
          (appsBody as { data?: { list?: SourceDeveloperCatalogApp[] } }).data?.list || []
      }
      if (listBody.code === 200) {
        applications.value =
          (listBody as { data?: { list?: SourceDeveloperAdApplication[] } }).data?.list || []
      }
    } catch (error: unknown) {
      if ((error as { response?: { status?: number } })?.response?.status === 401) {
        logoutToLogin()
        return
      }
      ElMessage.error('加载广告申请失败')
    } finally {
      loading.value = false
    }
  }

  async function handleImageFile(upload: UploadFile) {
    const selected = upload.raw
    if (!selected || uploadingImage.value) return
    uploadingImage.value = true
    try {
      const data = new FormData()
      data.append('file', selected)
      const res = await uploadSourceDeveloperAdvertisementImage(data)
      const body = unwrap(res)
      if (!body) return
      if (body.code !== 200) {
        ElMessage.error(body.msg || '图片上传失败')
        return
      }
      const url = (body as { data?: { url?: string } }).data?.url || ''
      if (!url) {
        ElMessage.error('图片上传失败')
        return
      }
      form.imageUrl = url
      ElMessage.success(body.msg || '图片已上传')
    } catch (error: unknown) {
      if ((error as { response?: { status?: number } })?.response?.status === 401) {
        logoutToLogin()
        return
      }
      ElMessage.error('图片上传失败')
    } finally {
      uploadingImage.value = false
    }
  }

  async function handleSubmit() {
    await formRef.value?.validate()
    saving.value = true
    try {
      const res = await createSourceDeveloperAdApplication({
        appId: form.appId || undefined,
        title: form.title.trim(),
        positions: form.positions,
        imageUrl: form.imageUrl.trim() || undefined,
        linkUrl: form.linkUrl.trim() || undefined,
        note: form.note.trim() || undefined
      })
      const body = unwrap(res)
      if (!body) return
      if (body.code !== 200) {
        ElMessage.error(body.msg || '提交失败')
        return
      }
      ElMessage.success(body.msg || '广告申请已提交')
      form.title = ''
      form.imageUrl = ''
      form.linkUrl = ''
      form.note = ''
      form.positions = ['home-banner']
      await loadPage()
    } finally {
      saving.value = false
    }
  }

  onMounted(loadPage)
</script>

<style scoped lang="scss">
  .table-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
  }

  .card-title {
    font-weight: 600;
  }

  .card-hint {
    margin: 6px 0 0;
    font-size: 13px;
    line-height: 1.5;
    color: var(--el-text-color-secondary);
  }

  .ad-form {
    max-width: 640px;
  }

  .image-field {
    display: flex;
    flex-direction: column;
    gap: 10px;
    width: 100%;
  }

  .field-hint {
    margin: 0;
    font-size: 12px;
    line-height: 1.4;
    color: var(--el-text-color-secondary);
  }

  .image-preview {
    max-width: 100%;
    max-height: 160px;
    object-fit: contain;
    border-radius: 8px;
    border: 1px solid var(--el-border-color);
    background: var(--el-fill-color-lighter);
  }

  .mb-5 {
    margin-bottom: 20px;
  }
</style>
