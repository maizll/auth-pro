<template>
  <div class="source-station-page">
    <el-card
      shadow="never"
      class="art-card placeholder-card"
      :class="{ 'is-collapsed': !placeholderOpen }"
    >
      <template #header>
        <div class="table-header placeholder-header" @click="placeholderOpen = !placeholderOpen">
          <div>
            <span class="card-title">广告位招租占位</span>
            <p class="card-hint">
              {{
                placeholderOpen
                  ? '客户端广告位无投放时会用这段文案补齐。跳转留空则不可点击，不会跳到外部链接。'
                  : `当前：${placeholder.title || '广告位出租'}。默认收起，需要时再展开编辑。`
              }}
            </p>
          </div>
          <div class="placeholder-actions" @click.stop>
            <el-button
              v-if="placeholderOpen"
              type="primary"
              :loading="savingPlaceholder"
              @click="handleSavePlaceholder"
            >
              保存占位
            </el-button>
            <el-button @click="placeholderOpen = !placeholderOpen">
              {{ placeholderOpen ? '收起' : '展开' }}
            </el-button>
          </div>
        </div>
      </template>
      <el-form
        v-show="placeholderOpen"
        :model="placeholder"
        label-width="88px"
        class="placeholder-form"
      >
        <el-form-item label="标题">
          <el-input v-model="placeholder.title" maxlength="120" show-word-limit placeholder="广告位出租" />
        </el-form-item>
        <el-form-item label="简介">
          <el-input
            v-model="placeholder.description"
            type="textarea"
            :rows="2"
            maxlength="500"
            show-word-limit
            placeholder="虚位以待，欢迎联系投放"
          />
        </el-form-item>
        <el-form-item label="跳转 URL">
          <el-input
            v-model="placeholder.linkUrl"
            maxlength="500"
            placeholder="可选，https://... 留空则不可点击"
          />
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" class="art-card mb-4">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">开发者广告申请（共 {{ applications.length }} 条）</span>
            <p class="card-hint">
              通过后会用申请字段创建一条广告投放。拒绝不会写入投放表。公开客户端接口不变。
            </p>
          </div>
          <el-select v-model="appStatus" clearable placeholder="全部状态" style="width: 140px" @change="loadApplications">
            <el-option label="待审核" value="pending" />
            <el-option label="已通过" value="approved" />
            <el-option label="已拒绝" value="rejected" />
          </el-select>
        </div>
      </template>
      <el-table :data="applications" stripe v-loading="appLoading">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="developerUsername" label="开发者" width="120" />
        <el-table-column prop="title" label="标题" min-width="140" show-overflow-tooltip />
        <el-table-column label="广告位" min-width="160">
          <template #default="{ row }">{{ positionLabels(row) }}</template>
        </el-table-column>
        <el-table-column prop="imageUrl" label="图片" min-width="140" show-overflow-tooltip />
        <el-table-column prop="linkUrl" label="跳转" min-width="140" show-overflow-tooltip />
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="appStatusMeta(row.status).type" size="small">
              {{ appStatusMeta(row.status).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="reviewNote" label="审核说明" min-width="120" show-overflow-tooltip />
        <el-table-column prop="createdAt" label="申请时间" width="170" />
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button
              link
              type="success"
              size="small"
              :disabled="row.status !== 'pending'"
              @click="handleApproveApp(row)"
            >
              通过
            </el-button>
            <el-button
              link
              type="warning"
              size="small"
              :disabled="row.status !== 'pending'"
              @click="handleRejectApp(row)"
            >
              拒绝
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card shadow="never" class="art-card">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">广告投放（共 {{ tableData.length }} 条）</span>
            <p class="card-hint"
              >可上传本站图片或粘贴外部网址。广告位可多选：首页横幅、侧栏、弹窗。这些广告面向访客页面，不会出现在管理后台。</p
            >
          </div>
          <el-button type="primary" @click="openEdit()">新增广告</el-button>
        </div>
      </template>
      <el-table :data="tableData" stripe v-loading="loading">
        <el-table-column prop="id" label="标识" width="140" />
        <el-table-column prop="title" label="标题" min-width="140" show-overflow-tooltip />
        <el-table-column label="广告位" min-width="180">
          <template #default="{ row }">
            {{ positionLabels(row) }}
          </template>
        </el-table-column>
        <el-table-column prop="weight" label="权重" width="80" align="center" />
        <el-table-column prop="imageUrl" label="图片" min-width="180" show-overflow-tooltip />
        <el-table-column prop="destinationUrl" label="跳转" min-width="180" show-overflow-tooltip />
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" size="small" @click="handleDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog
      v-model="visible"
      :title="form.id && isEdit ? '编辑广告' : '新增广告'"
      width="560px"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
        <el-form-item label="标识">
          <el-input :model-value="form.id || '保存时自动生成'" disabled />
        </el-form-item>
        <el-form-item label="标题" prop="title">
          <el-input v-model="form.title" />
        </el-form-item>
        <el-form-item label="广告位" prop="positions">
          <el-select
            v-model="form.positions"
            multiple
            collapse-tags
            collapse-tags-tooltip
            placeholder="可多选，至少选一处"
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
        <el-form-item label="权重">
          <el-input-number v-model="form.weight" :min="0" :max="999" />
        </el-form-item>
        <el-form-item label="广告图">
          <div class="image-field">
            <el-upload
              :auto-upload="false"
              :show-file-list="false"
              accept="image/png,image/jpeg,image/gif,image/webp"
              :disabled="uploadingImage"
              :on-change="handleImageFile"
            >
              <el-button :loading="uploadingImage">上传图片</el-button>
            </el-upload>
            <el-input
              v-model="form.imageUrl"
              placeholder="或粘贴 https:// / 本站图片地址"
            />
            <img v-if="form.imageUrl" :src="form.imageUrl" alt="" class="image-preview" />
          </div>
        </el-form-item>
        <el-form-item label="跳转 URL" prop="destinationUrl">
          <el-input v-model="form.destinationUrl" placeholder="付费广告填 https://，可留空" />
        </el-form-item>
        <el-form-item label="开始时间">
          <el-date-picker
            v-model="startAtModel"
            type="datetime"
            placeholder="可选，留空立即开始"
            format="YYYY-MM-DD HH:mm"
            clearable
            teleported
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="结束时间">
          <el-date-picker
            v-model="endAtModel"
            type="datetime"
            placeholder="可选，留空长期投放"
            format="YYYY-MM-DD HH:mm"
            clearable
            teleported
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="说明">
          <el-input v-model="form.description" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="visible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="handleSave">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, reactive, ref } from 'vue'
  import type { FormInstance, FormRules, UploadFile } from 'element-plus'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { DEFAULT_AD_PLACEHOLDER } from '@/api/advertisement'
  import {
    AD_POSITIONS,
    SOURCE_APPLICATION_STATUS,
    advertisementPositionList,
    approveSourceAdApplication,
    deleteSourceAdvertisement,
    fetchSourceAdApplications,
    fetchSourceAdvertisements,
    rejectSourceAdApplication,
    saveSourceAdPlaceholder,
    saveSourceAdvertisement,
    uploadSourceAdvertisementImage,
    type SourceAdApplication,
    type SourceAdPlaceholder,
    type SourceAdvertisement
  } from '@/api/source-station'

  const loading = ref(false)
  const appLoading = ref(false)
  const applications = ref<SourceAdApplication[]>([])
  const appStatus = ref('pending')
  const saving = ref(false)
  const savingPlaceholder = ref(false)
  /** 招租占位默认收起，避免广告投放页被表单占满 */
  const placeholderOpen = ref(false)
  const uploadingImage = ref(false)
  const visible = ref(false)
  const isEdit = ref(false)
  const tableData = ref<SourceAdvertisement[]>([])
  const placeholder = reactive<SourceAdPlaceholder>({ ...DEFAULT_AD_PLACEHOLDER })
  const formRef = ref<FormInstance>()
  const startAtModel = ref<Date>()
  const endAtModel = ref<Date>()
  const form = reactive<SourceAdvertisement>({
    id: '',
    title: '',
    imageUrl: '',
    destinationUrl: '',
    position: 'home-banner',
    positions: ['home-banner'],
    weight: 0,
    startAt: '',
    endAt: '',
    description: ''
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
    ]
  }

  function parseAdDate(raw?: string) {
    const value = String(raw || '').trim()
    if (!value) return undefined
    const parsed = new Date(value)
    return Number.isNaN(parsed.getTime()) ? undefined : parsed
  }

  function positionLabel(value: string) {
    return AD_POSITIONS.find((item) => item.value === value)?.label || value
  }

  function positionLabels(row: { position?: string; positions?: string[] }) {
    const slots = advertisementPositionList(row)
    return slots.length ? slots.map(positionLabel).join('、') : '—'
  }

  function appStatusMeta(value: string) {
    return SOURCE_APPLICATION_STATUS[value] || { label: value || '-', type: 'info' as const }
  }

  async function loadApplications() {
    appLoading.value = true
    try {
      const data = await fetchSourceAdApplications(appStatus.value || undefined)
      applications.value = data.list || []
    } finally {
      appLoading.value = false
    }
  }

  async function handleApproveApp(row: SourceAdApplication) {
    await ElMessageBox.confirm(`通过「${row.title}」并创建广告投放？`, '通过广告申请', {
      type: 'success',
      confirmButtonText: '确认通过'
    })
    await approveSourceAdApplication(row.id)
    ElMessage.success('已通过并创建广告投放')
    await Promise.all([loadApplications(), loadAds()])
  }

  async function handleRejectApp(row: SourceAdApplication) {
    const { value } = await ElMessageBox.prompt('请填写拒绝原因', '拒绝广告申请', {
      inputPlaceholder: '审核说明',
      confirmButtonText: '确定'
    })
    await rejectSourceAdApplication(row.id, value || '未通过')
    ElMessage.success('已拒绝广告申请')
    await loadApplications()
  }

  async function loadAds() {
    loading.value = true
    try {
      const data = await fetchSourceAdvertisements()
      tableData.value = data.records || []
      placeholder.title = data.placeholder?.title || DEFAULT_AD_PLACEHOLDER.title
      placeholder.description = data.placeholder?.description || DEFAULT_AD_PLACEHOLDER.description
      placeholder.linkUrl = data.placeholder?.linkUrl || ''
    } finally {
      loading.value = false
    }
  }

  async function handleSavePlaceholder() {
    savingPlaceholder.value = true
    try {
      const saved = await saveSourceAdPlaceholder({
        title: placeholder.title.trim(),
        description: placeholder.description.trim(),
        linkUrl: placeholder.linkUrl.trim()
      })
      placeholder.title = saved?.title || placeholder.title
      placeholder.description = saved?.description || placeholder.description
      placeholder.linkUrl = saved?.linkUrl || ''
      ElMessage.success('招租占位已保存')
    } finally {
      savingPlaceholder.value = false
    }
  }

  function openEdit(row?: SourceAdvertisement) {
    isEdit.value = Boolean(row)
    form.id = row?.id || ''
    form.title = row?.title || ''
    form.imageUrl = row?.imageUrl || ''
    form.destinationUrl = row?.destinationUrl || ''
    form.positions = row ? advertisementPositionList(row) : ['home-banner']
    form.position = form.positions[0] || 'home-banner'
    form.weight = row?.weight || 0
    form.startAt = row?.startAt || ''
    form.endAt = row?.endAt || ''
    form.description = row?.description || ''
    startAtModel.value = parseAdDate(row?.startAt)
    endAtModel.value = parseAdDate(row?.endAt)
    visible.value = true
  }

  async function handleImageFile(upload: UploadFile) {
    const selected = upload.raw
    if (!selected || uploadingImage.value) return
    uploadingImage.value = true
    try {
      const data = new FormData()
      data.append('file', selected)
      const result = await uploadSourceAdvertisementImage(data)
      form.imageUrl = result.url
      ElMessage.success('图片已上传')
    } finally {
      uploadingImage.value = false
    }
  }

  async function handleSave() {
    await formRef.value?.validate()
    saving.value = true
    try {
      const positions = advertisementPositionList(form)
      const saved = await saveSourceAdvertisement({
        ...form,
        id: isEdit.value ? form.id : '',
        positions,
        position: positions[0] || '',
        startAt: startAtModel.value ? startAtModel.value.toISOString() : '',
        endAt: endAtModel.value ? endAtModel.value.toISOString() : ''
      })
      if (saved?.id) form.id = saved.id
      ElMessage.success('广告已保存')
      visible.value = false
      await loadAds()
    } finally {
      saving.value = false
    }
  }

  async function handleDelete(row: SourceAdvertisement) {
    await ElMessageBox.confirm(`删除广告 ${row.id}？`, '删除确认', { type: 'warning' })
    await deleteSourceAdvertisement(row.id)
    ElMessage.success('广告已删除')
    await loadAds()
  }

  onMounted(() => {
    loadAds()
    loadApplications()
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
  }

  .mb-4 {
    margin-bottom: 16px;
  }

  .placeholder-card {
    margin-bottom: 16px;

    &.is-collapsed {
      :deep(.el-card__body) {
        display: none;
        padding: 0;
      }
    }
  }

  .placeholder-header {
    cursor: pointer;
    user-select: none;
  }

  .placeholder-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .placeholder-form {
    max-width: 720px;
  }

  .image-field {
    display: flex;
    flex-direction: column;
    gap: 10px;
    width: 100%;
  }

  .image-preview {
    max-width: 100%;
    max-height: 160px;
    object-fit: contain;
    border-radius: 8px;
    border: 1px solid var(--el-border-color);
    background: var(--el-fill-color-lighter);
  }
</style>
