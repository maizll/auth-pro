<template>
  <div class="source-station-page">
    <el-card shadow="never" class="art-card mb-4 filter-panel">
      <el-form :model="searchForm" inline>
        <el-form-item :label="isPlugin ? '插件状态' : '模板状态'">
          <el-select v-model="searchForm.status" placeholder="全部" clearable style="width: 140px">
            <el-option label="草稿" value="draft" />
            <el-option label="待审核" value="review" />
            <el-option label="已通过" value="approved" />
            <el-option label="已上架" value="published" />
            <el-option label="已下架" value="hidden" />
            <el-option label="已驳回" value="rejected" />
            <el-option label="已弃用" value="deprecated" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="loadItems">查询</el-button>
          <el-button @click="resetSearch">重置</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" class="art-card">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">{{ title }}（共 {{ tableData.length }} 条）</span>
            <p class="card-hint">
              源站只保存元数据与外部 HTTPS 地址，从不存储源码。下架只从公开
              <code>/software-source/index.json</code> 隐藏，不会远程卸载已安装实例。
            </p>
          </div>
          <div class="table-actions">
            <el-button @click="openRegister">登记外部地址</el-button>
            <el-button type="primary" @click="openUpload">上传 ZIP（硬校验）</el-button>
          </div>
        </div>
      </template>

      <el-table :data="tableData" stripe v-loading="loading">
        <el-table-column
          prop="id"
          :label="isPlugin ? '插件 ID' : '模板 ID'"
          min-width="140"
          show-overflow-tooltip
        />
        <el-table-column prop="name" label="名称" min-width="140" show-overflow-tooltip />
        <el-table-column label="当前版本" width="110">
          <template #default="{ row }">
            {{ row.latestVersion || row.version || '-' }}
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="statusMeta(row.status).type" size="small">
              {{ statusMeta(row.status).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="下载地址" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            {{ isPlugin ? row.downloadUrl : row.templateUrl }}
          </template>
        </el-table-column>
        <el-table-column prop="sha256" label="SHA256" min-width="160" show-overflow-tooltip />
        <el-table-column prop="updatedAt" label="更新时间" width="170" />
        <el-table-column label="操作" width="300" fixed="right">
          <template #default="{ row }">
            <span v-if="isDeprecatedCatalogStatus(row.status)" class="deprecated-hint"
              >已弃用，请重新上传</span
            >
            <el-button
              v-if="canCatalogItemAction(row.status, 'approve')"
              link
              type="primary"
              size="small"
              @click="runStatus(row, 'approve')"
              >通过</el-button
            >
            <el-button
              v-if="canCatalogItemAction(row.status, 'reject')"
              link
              type="warning"
              size="small"
              @click="runStatus(row, 'reject')"
              >拒绝</el-button
            >
            <el-button
              v-if="canCatalogItemAction(row.status, 'shelf')"
              link
              type="success"
              size="small"
              @click="runStatus(row, 'shelf')"
              >上架</el-button
            >
            <el-button
              v-if="canCatalogItemAction(row.status, 'unshelf')"
              link
              type="info"
              size="small"
              @click="runStatus(row, 'unshelf')"
              >下架</el-button
            >
            <el-button
              v-if="canCatalogItemAction(row.status, 'deprecate')"
              link
              type="danger"
              size="small"
              @click="runStatus(row, 'deprecate')"
              >弃用</el-button
            >
            <el-button link type="primary" size="small" @click="openVersions(row)">版本</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog
      v-model="uploadVisible"
      :title="`上传${kindLabel} ZIP`"
      width="640px"
      destroy-on-close
    >
      <el-alert
        type="warning"
        :closable="false"
        show-icon
        title="失败即拒绝：ZIP 必须含 plugin.json / template.json，必填字段合法，禁止路径穿越。失败不写库、不推 Release、不留临时文件。"
        class="mb-3"
      />
      <el-form label-width="120px">
        <el-form-item label="ZIP 文件" required>
          <el-upload
            drag
            :auto-upload="false"
            :show-file-list="false"
            accept=".zip,application/zip"
            :on-change="selectUploadFile"
          >
            <div>{{ uploadFile ? uploadFile.name : '点击或拖拽 ZIP（≤ 20 MiB）' }}</div>
          </el-upload>
        </el-form-item>
        <el-form-item v-if="parsedManifest" label="解析结果">
          <el-descriptions :column="1" border size="small">
            <el-descriptions-item label="ID">{{ parsedManifest.id }}</el-descriptions-item>
            <el-descriptions-item label="名称">{{ parsedManifest.name }}</el-descriptions-item>
            <el-descriptions-item label="版本">{{ parsedManifest.version }}</el-descriptions-item>
            <el-descriptions-item label="作者">{{
              parsedManifest.author?.name
            }}</el-descriptions-item>
            <el-descriptions-item label="SHA256">{{ parsedManifest.sha256 }}</el-descriptions-item>
          </el-descriptions>
        </el-form-item>
        <el-form-item label="changelog">
          <el-input v-model="uploadForm.changelog" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item label="外部地址">
          <el-input
            v-model="uploadForm.location"
            :placeholder="
              isPlugin ? 'https://... 下载地址（未配置 Release 时必填）' : 'https://... 或相对路径'
            "
          />
        </el-form-item>
        <el-form-item label="minVersion">
          <el-input v-model="uploadForm.minVersion" placeholder="可选" />
        </el-form-item>
        <el-form-item label="选项">
          <el-checkbox v-model="uploadForm.push">推送 GitHub/Gitee Release</el-checkbox>
          <el-checkbox v-model="uploadForm.submit">保存后提交审核</el-checkbox>
          <el-checkbox v-model="uploadForm.shelf">保存后直接上架</el-checkbox>
          <el-checkbox v-model="uploadForm.forceUpdate">forceUpdate</el-checkbox>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="uploadVisible = false">取消</el-button>
        <el-button :disabled="!uploadFile" :loading="parsing" @click="handleParse"
          >仅校验解析</el-button
        >
        <el-button
          type="primary"
          :disabled="!uploadFile"
          :loading="publishing"
          @click="handlePublish"
        >
          校验并保存元数据
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="registerVisible"
      :title="`登记外部${kindLabel}`"
      width="560px"
      destroy-on-close
    >
      <el-form ref="registerRef" :model="registerForm" :rules="registerRules" label-width="110px">
        <el-form-item :label="isPlugin ? '插件 ID' : '模板 ID'" prop="id">
          <el-input v-model="registerForm.id" />
        </el-form-item>
        <el-form-item label="名称" prop="name">
          <el-input v-model="registerForm.name" />
        </el-form-item>
        <el-form-item label="版本" prop="version">
          <el-input v-model="registerForm.version" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="registerForm.description" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item :label="isPlugin ? 'downloadUrl' : 'templateUrl'" prop="location">
          <el-input v-model="registerForm.location" placeholder="https://..." />
        </el-form-item>
        <el-form-item label="sha256" prop="sha256">
          <el-input v-model="registerForm.sha256" />
        </el-form-item>
        <el-form-item label="作者">
          <el-input v-model="registerForm.authorName" placeholder="作者名称" />
        </el-form-item>
        <el-form-item label="changelog">
          <el-input v-model="registerForm.changelog" />
        </el-form-item>
        <el-form-item v-if="isPlugin" label="分类">
          <el-select v-model="registerForm.category" style="width: 100%">
            <el-option label="支付" value="payment" />
            <el-option label="实名" value="realname" />
            <el-option label="其他" value="other" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="registerForm.shelf"
            >登记后直接上架（须同时有 URL 与 sha256）</el-checkbox
          >
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="registerVisible = false">取消</el-button>
        <el-button type="primary" :loading="registering" @click="handleRegister">保存</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="versionVisible" :title="`${currentItem?.name || ''} 多版本`" size="720px">
      <div class="table-actions mb-3">
        <el-button type="primary" @click="versionFormVisible = true">登记新版本</el-button>
      </div>
      <el-table :data="versions" v-loading="versionLoading" stripe>
        <el-table-column prop="version" label="版本" width="110" />
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="versionStatusMeta(row.status).type" size="small">
              {{ versionStatusMeta(row.status).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="latest" width="80" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.version === currentItem?.latestVersion" type="success" size="small"
              >latest</el-tag
            >
          </template>
        </el-table-column>
        <el-table-column
          :prop="isPlugin ? 'downloadUrl' : 'templateUrl'"
          label="地址"
          min-width="180"
          show-overflow-tooltip
        />
        <el-table-column prop="changelog" label="说明" min-width="140" show-overflow-tooltip />
        <el-table-column label="操作" width="240" fixed="right">
          <template #default="{ row }">
            <span v-if="isDeprecatedCatalogStatus(row.status)" class="deprecated-hint">已弃用</span>
            <el-button
              v-if="canVersionAction(row.status, 'approve')"
              link
              type="success"
              size="small"
              @click="runVersion(row, 'approve')"
              >通过</el-button
            >
            <el-button
              v-if="canVersionAction(row.status, 'reject')"
              link
              type="warning"
              size="small"
              @click="runVersion(row, 'reject')"
              >拒绝</el-button
            >
            <el-button
              v-if="
                canVersionAction(
                  row.status,
                  'latest',
                  row.version === currentItem?.latestVersion
                )
              "
              link
              type="primary"
              size="small"
              @click="runVersion(row, 'latest')"
              >设为 latest</el-button
            >
            <el-button
              v-if="canVersionAction(row.status, 'deprecate')"
              link
              type="danger"
              size="small"
              @click="runVersion(row, 'deprecate')"
              >弃用</el-button
            >
          </template>
        </el-table-column>
      </el-table>

      <el-dialog
        v-model="versionFormVisible"
        title="登记新版本"
        width="480px"
        append-to-body
        destroy-on-close
      >
        <el-form :model="versionForm" label-width="110px">
          <el-form-item label="版本" required>
            <el-input v-model="versionForm.version" placeholder="1.0.1" />
          </el-form-item>
          <el-form-item :label="isPlugin ? 'downloadUrl' : 'templateUrl'" required>
            <el-input v-model="versionForm.location" placeholder="https://..." />
          </el-form-item>
          <el-form-item label="sha256" required>
            <el-input v-model="versionForm.sha256" />
          </el-form-item>
          <el-form-item label="changelog">
            <el-input v-model="versionForm.changelog" type="textarea" :rows="2" />
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="versionFormVisible = false">取消</el-button>
          <el-button type="primary" :loading="versionSaving" @click="handleAddVersion"
            >保存</el-button
          >
        </template>
      </el-dialog>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, reactive, ref } from 'vue'
  import type { FormInstance, FormRules, UploadFile } from 'element-plus'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import {
    SOURCE_ITEM_STATUS,
    SOURCE_VERSION_STATUS,
    fetchSourcePlugins,
    fetchSourceTemplates,
    parseSourcePackage,
    publishSourcePackage,
    registerSourcePlugin,
    registerSourceTemplate,
    setSourcePluginStatus,
    setSourceTemplateStatus,
    fetchSourcePluginVersions,
    fetchSourceTemplateVersions,
    registerSourcePluginVersion,
    registerSourceTemplateVersion,
    setSourcePluginVersionStatus,
    setSourceTemplateVersionStatus,
    type SourcePackageManifest,
    type SourcePlugin,
    type SourceTemplate,
    type SourceVersion
  } from '@/api/source-station'
  import {
    canCatalogItemAction,
    canVersionAction,
    isDeprecatedCatalogStatus
  } from './catalog-actions'

  const props = defineProps<{ kind: 'plugin' | 'template' }>()

  const isPlugin = computed(() => props.kind === 'plugin')
  const kindLabel = computed(() => (isPlugin.value ? '插件' : '首页模板'))
  const title = computed(() => (isPlugin.value ? '插件目录' : '首页模板目录'))

  type CatalogItem = SourcePlugin & SourceTemplate
  const loading = ref(false)
  const tableData = ref<CatalogItem[]>([])
  const searchForm = reactive({ status: '' })

  const uploadVisible = ref(false)
  const parsing = ref(false)
  const publishing = ref(false)
  const uploadFile = ref<File | null>(null)
  const parsedManifest = ref<SourcePackageManifest | null>(null)
  const uploadForm = reactive({
    changelog: '',
    location: '',
    minVersion: '',
    push: true,
    submit: false,
    shelf: false,
    forceUpdate: false
  })

  const registerVisible = ref(false)
  const registering = ref(false)
  const registerRef = ref<FormInstance>()
  const registerForm = reactive({
    id: '',
    name: '',
    version: '1.0.0',
    description: '',
    location: '',
    sha256: '',
    authorName: '',
    changelog: '',
    category: 'other',
    shelf: false
  })
  const registerRules: FormRules = {
    id: [{ required: true, message: '请填写标识', trigger: 'blur' }],
    name: [{ required: true, message: '请填写名称', trigger: 'blur' }],
    version: [{ required: true, message: '请填写版本', trigger: 'blur' }],
    location: [{ required: true, message: '请填写外部地址', trigger: 'blur' }],
    sha256: [{ required: true, message: '请填写 sha256', trigger: 'blur' }]
  }

  const versionVisible = ref(false)
  const versionLoading = ref(false)
  const versionSaving = ref(false)
  const versionFormVisible = ref(false)
  const versions = ref<SourceVersion[]>([])
  const currentItem = ref<CatalogItem | null>(null)
  const versionForm = reactive({ version: '', location: '', sha256: '', changelog: '' })

  function statusMeta(status: string) {
    return SOURCE_ITEM_STATUS[status] || { label: status, type: 'info' as const }
  }

  function versionStatusMeta(status: string) {
    return SOURCE_VERSION_STATUS[status] || { label: status, type: 'info' as const }
  }

  async function loadItems() {
    loading.value = true
    try {
      const data = isPlugin.value
        ? await fetchSourcePlugins(searchForm.status)
        : await fetchSourceTemplates(searchForm.status)
      tableData.value = (data.list || []) as CatalogItem[]
    } finally {
      loading.value = false
    }
  }

  function resetSearch() {
    searchForm.status = ''
    loadItems()
  }

  function openUpload() {
    uploadFile.value = null
    parsedManifest.value = null
    uploadForm.changelog = ''
    uploadForm.location = ''
    uploadForm.minVersion = ''
    uploadForm.push = true
    uploadForm.submit = false
    uploadForm.shelf = false
    uploadForm.forceUpdate = false
    uploadVisible.value = true
  }

  function selectUploadFile(file: UploadFile) {
    uploadFile.value = file.raw || null
    parsedManifest.value = null
  }

  function buildPackageForm() {
    const form = new FormData()
    if (uploadFile.value) form.append('file', uploadFile.value)
    form.append('kind', props.kind)
    if (uploadForm.changelog) form.append('changelog', uploadForm.changelog)
    if (uploadForm.minVersion) form.append('minVersion', uploadForm.minVersion)
    if (uploadForm.location) {
      form.append(isPlugin.value ? 'downloadUrl' : 'templateUrl', uploadForm.location)
    }
    if (uploadForm.push) form.append('push', 'true')
    if (uploadForm.submit) form.append('submit', 'true')
    if (uploadForm.shelf) form.append('shelf', 'true')
    if (uploadForm.forceUpdate) form.append('forceUpdate', 'true')
    return form
  }

  async function handleParse() {
    if (!uploadFile.value) return
    parsing.value = true
    try {
      const form = new FormData()
      form.append('file', uploadFile.value)
      form.append('kind', props.kind)
      parsedManifest.value = await parseSourcePackage(form)
      ElMessage.success('已通过校验（包未落盘、未入库）')
    } finally {
      parsing.value = false
    }
  }

  async function handlePublish() {
    if (!uploadFile.value) return
    publishing.value = true
    try {
      const result = await publishSourcePackage(buildPackageForm())
      ElMessage.success(
        result.pushed
          ? '校验通过，已推送 Release 并保存元数据'
          : '校验通过，已保存元数据（包已丢弃）'
      )
      uploadVisible.value = false
      await loadItems()
    } finally {
      publishing.value = false
    }
  }

  function openRegister() {
    registerForm.id = ''
    registerForm.name = ''
    registerForm.version = '1.0.0'
    registerForm.description = ''
    registerForm.location = ''
    registerForm.sha256 = ''
    registerForm.authorName = ''
    registerForm.changelog = ''
    registerForm.category = 'other'
    registerForm.shelf = false
    registerVisible.value = true
  }

  async function handleRegister() {
    await registerRef.value?.validate()
    registering.value = true
    try {
      if (isPlugin.value) {
        await registerSourcePlugin({
          id: registerForm.id,
          name: registerForm.name,
          version: registerForm.version,
          description: registerForm.description,
          downloadUrl: registerForm.location,
          sha256: registerForm.sha256,
          changelog: registerForm.changelog,
          category: registerForm.category,
          author: { name: registerForm.authorName },
          shelf: registerForm.shelf
        })
      } else {
        await registerSourceTemplate({
          id: registerForm.id,
          templateKey: registerForm.id,
          name: registerForm.name,
          version: registerForm.version,
          description: registerForm.description,
          templateUrl: registerForm.location,
          sha256: registerForm.sha256,
          changelog: registerForm.changelog,
          schemaVersion: 1,
          author: { name: registerForm.authorName },
          shelf: registerForm.shelf
        })
      }
      ElMessage.success('已登记外部地址（未上传源码）')
      registerVisible.value = false
      await loadItems()
    } finally {
      registering.value = false
    }
  }

  async function runStatus(
    row: CatalogItem,
    action: 'approve' | 'reject' | 'shelf' | 'unshelf' | 'deprecate'
  ) {
    if (action === 'unshelf') {
      await ElMessageBox.confirm(
        '下架只会从公开 index.json 隐藏，不会远程卸载已安装实例。确认继续？',
        '下架确认',
        { type: 'warning' }
      )
    }
    let note = ''
    if (action === 'reject' || action === 'deprecate') {
      const { value } = await ElMessageBox.prompt(
        '备注（可选）',
        action === 'reject' ? '拒绝' : '弃用',
        {
          inputPlaceholder: '审核说明',
          confirmButtonText: '确定',
          cancelButtonText: '取消'
        }
      )
      note = value || ''
    }
    if (isPlugin.value) {
      await setSourcePluginStatus(row.id, action, note)
    } else {
      await setSourceTemplateStatus(row.id, action, note)
    }
    ElMessage.success('已更新状态')
    await loadItems()
  }

  async function openVersions(row: CatalogItem) {
    currentItem.value = row
    versionVisible.value = true
    await loadVersions()
  }

  async function loadVersions() {
    if (!currentItem.value) return
    versionLoading.value = true
    try {
      const data = isPlugin.value
        ? await fetchSourcePluginVersions(currentItem.value.id)
        : await fetchSourceTemplateVersions(currentItem.value.id)
      versions.value = data.list || []
    } finally {
      versionLoading.value = false
    }
  }

  async function handleAddVersion() {
    if (!currentItem.value) return
    versionSaving.value = true
    try {
      const payload = {
        version: versionForm.version,
        sha256: versionForm.sha256,
        changelog: versionForm.changelog,
        ...(isPlugin.value
          ? { downloadUrl: versionForm.location }
          : { templateUrl: versionForm.location })
      }
      if (isPlugin.value) {
        await registerSourcePluginVersion(currentItem.value.id, payload)
      } else {
        await registerSourceTemplateVersion(currentItem.value.id, payload)
      }
      ElMessage.success('已登记外部版本地址')
      versionFormVisible.value = false
      versionForm.version = ''
      versionForm.location = ''
      versionForm.sha256 = ''
      versionForm.changelog = ''
      await loadVersions()
      await loadItems()
    } finally {
      versionSaving.value = false
    }
  }

  async function runVersion(
    row: SourceVersion,
    action: 'approve' | 'reject' | 'deprecate' | 'latest'
  ) {
    if (!currentItem.value) return
    if (isPlugin.value) {
      await setSourcePluginVersionStatus(currentItem.value.id, row.version, action)
    } else {
      await setSourceTemplateVersionStatus(currentItem.value.id, row.version, action)
    }
    ElMessage.success('已更新版本')
    await loadVersions()
    await loadItems()
  }

  onMounted(loadItems)
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

    :deep(.el-card__header) {
      padding: 20px 22px 14px;
      border-bottom-color: var(--art-card-border);
    }

    :deep(.el-card__body) {
      padding: 20px 22px;
    }
  }

  .mb-4 {
    margin-bottom: 16px;
  }

  .mb-3 {
    margin-bottom: 12px;
  }

  .table-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 12px;
  }

  .table-actions {
    display: flex;
    gap: 8px;
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

  .filter-panel :deep(.el-form) {
    margin-bottom: -18px;
  }

  .deprecated-hint {
    margin-right: 8px;
    font-size: 12px;
    color: var(--art-gray-500);
  }
</style>
