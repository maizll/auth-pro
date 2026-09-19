<template>
  <div class="source-station-page">
    <el-card shadow="never" class="art-card placeholder-card">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">广告位招租占位</span>
            <p class="card-hint">
              跑马灯 / 九宫格空位会用这段文案补齐。跳转留空则不可点击，不会跳到外部链接。
            </p>
          </div>
          <el-button type="primary" :loading="savingPlaceholder" @click="handleSavePlaceholder">
            保存占位
          </el-button>
        </div>
      </template>
      <el-form :model="placeholder" label-width="88px" class="placeholder-form">
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

    <el-card shadow="never" class="art-card">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">广告投放（共 {{ tableData.length }} 条）</span>
            <p class="card-hint">
              图片可本站上传或粘贴 https 地址。付费跳转仍填 https://。广告位：首页横幅 / 侧栏 /
              弹窗。
            </p>
          </div>
          <el-button type="primary" @click="openEdit()">新增广告</el-button>
        </div>
      </template>
      <el-table :data="tableData" stripe v-loading="loading">
        <el-table-column prop="id" label="标识" width="140" />
        <el-table-column prop="title" label="标题" min-width="140" show-overflow-tooltip />
        <el-table-column label="广告位" width="120">
          <template #default="{ row }">
            {{ positionLabel(row.position) }}
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
        <el-form-item label="广告位" prop="position">
          <el-select v-model="form.position" style="width: 100%">
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
    deleteSourceAdvertisement,
    fetchSourceAdvertisements,
    saveSourceAdPlaceholder,
    saveSourceAdvertisement,
    uploadSourceAdvertisementImage,
    type SourceAdPlaceholder,
    type SourceAdvertisement
  } from '@/api/source-station'

  const loading = ref(false)
  const saving = ref(false)
  const savingPlaceholder = ref(false)
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
    weight: 0,
    startAt: '',
    endAt: '',
    description: ''
  })
  const rules: FormRules = {
    title: [{ required: true, message: '请填写标题', trigger: 'blur' }],
    position: [{ required: true, message: '请选择广告位', trigger: 'change' }]
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
    form.position = row?.position || 'home-banner'
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
      const saved = await saveSourceAdvertisement({
        ...form,
        id: isEdit.value ? form.id : '',
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

  onMounted(loadAds)
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

  .placeholder-card {
    margin-bottom: 16px;
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
    max-height: 140px;
    object-fit: contain;
    border: 1px solid var(--el-border-color);
    border-radius: 8px;
    background: var(--el-fill-color-lighter);
  }
</style>
