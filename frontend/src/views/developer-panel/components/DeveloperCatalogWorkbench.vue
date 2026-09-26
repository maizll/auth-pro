<template>
  <div v-loading="loading" class="developer-catalog">
    <el-card v-loading="githubLoading" shadow="never" class="token-card">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">收费安装包只读令牌</span>
            <p class="card-hint">
              收费插件和模板放在你自己的私有 GitHub
              仓库时，只用这一枚只读令牌。令牌按你的开发者账号加密保存，只用于你登记的条目，页面不回显明文。
            </p>
          </div>
          <div class="table-actions">
            <el-button :loading="githubTesting" @click="handleGitHubTest">测试令牌</el-button>
            <el-button type="primary" :loading="githubSaving" @click="handleGitHubSave">
              保存令牌
            </el-button>
          </div>
        </div>
      </template>
      <el-alert
        :title="githubConfigured ? '已配置只读令牌' : '尚未配置只读令牌'"
        :type="githubConfigured ? 'success' : 'warning'"
        :closable="false"
        show-icon
      />
      <el-form label-width="118px" class="token-form">
        <el-form-item label="只读令牌">
          <el-input
            v-model="githubToken"
            type="password"
            show-password
            :placeholder="githubConfigured ? '已配置，留空不修改' : '粘贴只读令牌'"
          />
          <p class="field-help">
            在 GitHub 创建 fine-grained personal access token，只授权你的私有仓库，Contents 选
            Read-only。不要用可推送的令牌。
          </p>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">{{ title }}（共 {{ items.length }} 条）</span>
            <p class="card-hint">
              来源三选一：上传压缩包（收费时由本站托管）、公开地址（仅免费）、私有 GitHub
              仓库（收费推荐）。公开地址不能用来收费。旧的收费公开地址会标注「需要改成私有仓库来源或上传
              zip 才能继续收费出售」。
            </p>
          </div>
          <el-button type="primary" @click="openEdit()">{{ createLabel }}</el-button>
        </div>
      </template>

      <el-empty v-if="!items.length" :description="emptyText" />
      <el-table v-else :data="items" stripe>
        <el-table-column prop="id" label="标识" min-width="140" show-overflow-tooltip />
        <el-table-column label="名称" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ row.name }}</span>
            <p v-if="row.fulfillmentHint" class="field-help">{{ row.fulfillmentHint }}</p>
          </template>
        </el-table-column>
        <el-table-column label="应用" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">{{ appLabel(row.appId) }}</template>
        </el-table-column>
        <el-table-column label="分类" width="120">
          <template #default="{ row }">{{ categoryLabel(row.category) }}</template>
        </el-table-column>
        <el-table-column label="版本" width="110">
          <template #default="{ row }">{{ row.latestVersion || row.version || '-' }}</template>
        </el-table-column>
        <el-table-column label="售价" width="100">
          <template #default="{ row }">{{ formatCatalogPriceLabel(row.priceCents) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="110" align="center">
          <template #default="{ row }">
            <el-tag :type="statusMeta(row.status).type" size="small">
              {{ statusMeta(row.status).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="来源" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ sourceLabel(row) }}</span>
            <p v-if="row.originHint" class="field-help">{{ row.originHint }}</p>
          </template>
        </el-table-column>
        <el-table-column label="审核说明" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">{{ row.reviewNote || '-' }}</template>
        </el-table-column>
        <el-table-column prop="updatedAt" label="更新时间" width="180" />
        <el-table-column label="操作" width="300" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="openEdit(row)">
              {{ canEditItem(row) ? '编辑' : '查看' }}
            </el-button>
            <el-button
              v-if="canSubmitItem(row)"
              link
              type="success"
              size="small"
              @click="handleSubmit(row)"
            >
              提交审核
            </el-button>
            <el-button
              v-if="row.originUrl"
              link
              type="primary"
              size="small"
              :loading="pullingId === row.id"
              @click="handlePull(row)"
            >
              重新拉取
            </el-button>
            <el-button link type="primary" size="small" @click="openVersions(row)">版本</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-drawer
      v-model="formVisible"
      class="catalog-sheet"
      :title="formTitle"
      :size="formDrawerSize"
      destroy-on-close
    >
      <el-form
        ref="formRef"
        :model="form"
        :rules="formRules"
        label-width="118px"
        :label-position="formLabelPosition"
      >
        <el-form-item label="应用" prop="appId">
          <el-select
            v-model="form.appId"
            placeholder="请选择会出现在哪个应用里"
            style="width: 100%"
            :disabled="isEdit"
          >
            <el-option
              v-for="app in apps"
              :key="app.id"
              :label="appLabel(app.id)"
              :value="app.id"
            />
          </el-select>
          <p class="field-help">必选。这个条目只会出现在所选应用的目录中。</p>
        </el-form-item>
        <el-form-item label="分类" prop="category">
          <el-select
            v-model="form.category"
            placeholder="请选择分类"
            style="width: 100%"
            :disabled="!canEditMeta"
          >
            <el-option
              v-for="item in categoryOptions"
              :key="item.key"
              :label="item.label"
              :value="item.key"
            />
          </el-select>
          <p class="field-help">必选。方便用户按类查找。</p>
        </el-form-item>
        <el-form-item label="名称" prop="name">
          <el-input
            v-model="form.name"
            :disabled="!canEditMeta"
            maxlength="100"
            placeholder="用户看到的名字，例如：微信支付"
          />
        </el-form-item>
        <el-form-item class="is-secondary" label="标识" prop="id">
          <el-input
            v-model="form.id"
            :disabled="isEdit"
            placeholder="会根据名称自动生成"
            @input="onIdInput"
          />
          <p class="field-help">一般不用改。提交后不能再改。</p>
        </el-form-item>
        <el-form-item label="版本" prop="version">
          <el-input v-model="form.version" :disabled="!canEditPackage" placeholder="1.0.0" />
          <p class="field-help">首次发布用 1.0.0 即可，以后改这里或在「版本」里新增。</p>
        </el-form-item>
        <el-form-item label="售价（元）">
          <el-input v-model="form.priceYuan" :disabled="!canEditMeta" placeholder="0" />
          <p class="field-help">
            填 0 表示免费，请使用公开地址。大于 0 为买断：推荐私有 GitHub
            仓库，也可以上传压缩包由本站托管。付费上架尚未开放。
          </p>
        </el-form-item>
        <el-form-item label="包来源">
          <el-radio-group
            v-model="form.packageSource"
            :disabled="!canEditPackage"
            @change="onPackageSourceChange('form')"
          >
            <el-radio value="upload">上传压缩包</el-radio>
            <el-radio value="public">公开地址（仅免费）</el-radio>
            <el-radio value="github">私有 GitHub 仓库（收费推荐）</el-radio>
          </el-radio-group>
          <p class="field-help">{{ packageSourceHelp }}</p>
          <p v-if="draftBlockReason" class="field-help">{{ draftBlockReason }}</p>
          <p v-else-if="submitBlockReason" class="field-help">{{ submitBlockReason }}</p>
        </el-form-item>
        <el-form-item v-if="form.packageSource === 'upload'" label="压缩包">
          <el-upload
            :auto-upload="false"
            :show-file-list="false"
            accept=".zip,application/zip"
            :disabled="!canEditPackage || uploading"
            :on-change="(file) => handlePackageFile('form', file)"
          >
            <el-button :loading="uploading" :disabled="!canEditPackage">选择压缩包</el-button>
          </el-upload>
          <p class="field-help">上传后自动填写地址和校验码。不合规的包不会保存。</p>
        </el-form-item>
        <el-form-item :label="locationLabel" prop="location">
          <el-input
            v-model="form.location"
            :disabled="!canEditPackage || form.packageSource === 'upload'"
            :placeholder="locationPlaceholderFor(form.packageSource)"
          />
          <p class="field-help">{{ locationHelp }}</p>
        </el-form-item>
        <el-form-item v-if="currentItem?.originUrl" label="来源外链">
          <el-input :model-value="currentItem.originUrl" disabled />
          <p class="field-help">
            {{ currentItem.originHint || '本站已拉取并私有托管。买家看不到这条外链。' }}
          </p>
        </el-form-item>
        <el-form-item label="校验码" prop="sha256">
          <div class="checksum-row">
            <el-input
              v-model="form.sha256"
              :disabled="!canEditPackage || form.packageSource !== 'public'"
              placeholder="64 位十六进制，可稍后补"
            />
            <el-button
              v-if="form.packageSource === 'public' && formPriceCents <= 0"
              :disabled="!canEditPackage || !canAutoHash(form.location)"
              :loading="hashing === 'form'"
              @click="handleAutoHash('form')"
            >
              自动计算
            </el-button>
          </div>
          <p class="field-help">
            {{
              form.packageSource === 'upload'
                ? '由本站按压缩包内容计算，不能手改。'
                : form.packageSource === 'github'
                  ? '私有仓库由本站核对后自动计算，不用手填。临时文件不会留下。'
                  : '64 位。粘贴下载地址后也可以稍后补上'
            }}
          </p>
        </el-form-item>
        <el-form-item label="简介" prop="description">
          <el-input
            v-model="form.description"
            type="textarea"
            :rows="2"
            maxlength="500"
            show-word-limit
            :disabled="!canEditMeta"
            placeholder="一句话说明做什么，可不填"
          />
        </el-form-item>

        <el-collapse v-model="advancedOpen" class="advanced-collapse">
          <el-collapse-item name="advanced" title="高级选项">
            <el-form-item label="作者">
              <el-input v-model="form.authorName" disabled />
              <p class="field-help">默认使用当前登录开发者名称，一般不用改。</p>
            </el-form-item>
            <el-form-item v-if="kind === 'plugin'" label="图标">
              <el-input v-model="form.icon" :disabled="!canEditMeta" placeholder="ri:puzzle-line" />
              <p class="field-help">图标名称，不填则使用默认拼图图标。</p>
            </el-form-item>
            <el-form-item label="更新说明">
              <el-input
                v-model="form.changelog"
                type="textarea"
                :rows="2"
                maxlength="2000"
                :disabled="!canEditPackage"
                placeholder="这次改了什么，可不填"
              />
            </el-form-item>
          </el-collapse-item>
        </el-collapse>

        <el-alert
          v-if="currentItem?.latestVersion"
          type="info"
          :closable="false"
          show-icon
          class="mb-3"
          title="已有正式版本后，包地址请通过「版本」新增，不要改当前草稿字段。"
        />
        <el-alert
          v-if="currentItem?.reviewNote"
          type="warning"
          :closable="false"
          show-icon
          :title="`审核说明：${currentItem.reviewNote}`"
        />
      </el-form>
      <template #footer>
        <el-button @click="formVisible = false">取消</el-button>
        <el-button
          :loading="saving"
          :disabled="!canEditMeta || Boolean(draftBlockReason)"
          @click="handleSave(false)"
        >
          保存草稿
        </el-button>
        <el-button
          type="primary"
          :loading="saving"
          :disabled="!canSubmitCurrent || Boolean(submitBlockReason)"
          @click="handleSave(true)"
        >
          提交审核
        </el-button>
      </template>
    </el-drawer>

    <el-drawer
      v-model="versionVisible"
      class="catalog-sheet"
      :title="`${currentItem?.name || ''} 版本`"
      :size="versionDrawerSize"
    >
      <div class="table-actions mb-3">
        <el-button type="primary" @click="openVersionForm">新增版本</el-button>
      </div>
      <el-table v-loading="versionLoading" :data="versions" stripe>
        <el-table-column prop="version" label="版本" width="110" />
        <el-table-column label="状态" width="110" align="center">
          <template #default="{ row }">
            <el-tag :type="versionStatusMeta(row.status).type" size="small">
              {{ versionStatusMeta(row.status).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="当前版本" width="100" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.version === currentItem?.latestVersion" type="success" size="small">
              当前
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="地址" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            {{ versionLocationLabel(row) }}
          </template>
        </el-table-column>
        <el-table-column label="来源外链" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">{{ row.originUrl || '-' }}</template>
        </el-table-column>
        <el-table-column prop="changelog" label="说明" min-width="140" show-overflow-tooltip />
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <el-button
              v-if="row.status === 'draft'"
              link
              type="success"
              size="small"
              @click="handleSubmitVersion(row)"
            >
              提交审核
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-dialog
        v-model="versionFormVisible"
        class="catalog-sheet"
        title="新增版本"
        :width="versionDialogWidth"
        append-to-body
        destroy-on-close
      >
        <el-form :model="versionForm" label-width="118px" :label-position="formLabelPosition">
          <el-form-item label="版本" required>
            <el-input v-model="versionForm.version" placeholder="1.0.1" />
          </el-form-item>
          <el-form-item label="包来源">
            <el-radio-group
              v-model="versionForm.packageSource"
              @change="onPackageSourceChange('version')"
            >
              <el-radio value="upload">上传压缩包</el-radio>
              <el-radio value="public">公开地址（仅免费）</el-radio>
              <el-radio value="github">私有 GitHub 仓库（收费推荐）</el-radio>
            </el-radio-group>
            <p v-if="versionBlockReason" class="field-help">{{ versionBlockReason }}</p>
          </el-form-item>
          <el-form-item v-if="versionForm.packageSource === 'upload'" label="压缩包">
            <el-upload
              :auto-upload="false"
              :show-file-list="false"
              accept=".zip,application/zip"
              :disabled="uploading"
              :on-change="(file) => handlePackageFile('version', file)"
            >
              <el-button :loading="uploading">选择压缩包</el-button>
            </el-upload>
          </el-form-item>
          <el-form-item :label="locationLabel" required>
            <el-input
              v-model="versionForm.location"
              :disabled="versionForm.packageSource === 'upload'"
              :placeholder="locationPlaceholderFor(versionForm.packageSource)"
            />
            <p class="field-help">{{ versionLocationHelp }}</p>
          </el-form-item>
          <el-form-item label="校验码" required>
            <div class="checksum-row">
              <el-input
                v-model="versionForm.sha256"
                :disabled="versionForm.packageSource !== 'public'"
                placeholder="64 位十六进制"
              />
              <el-button
                v-if="versionForm.packageSource === 'public' && (currentItem?.priceCents || 0) <= 0"
                :disabled="!canAutoHash(versionForm.location)"
                :loading="hashing === 'version'"
                @click="handleAutoHash('version')"
              >
                自动计算
              </el-button>
            </div>
            <p class="field-help">
              {{
                versionForm.packageSource === 'upload'
                  ? '由本站按压缩包内容计算。'
                  : versionForm.packageSource === 'github'
                    ? '私有仓库由本站核对后自动计算，不用手填。'
                    : '64 位。提交审核时会核对文件。'
              }}
            </p>
          </el-form-item>
          <el-form-item label="更新说明">
            <el-input
              v-model="versionForm.changelog"
              type="textarea"
              :rows="2"
              placeholder="可不填"
            />
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="versionFormVisible = false">取消</el-button>
          <el-button
            type="primary"
            :loading="versionSaving"
            :disabled="Boolean(versionBlockReason)"
            @click="handleAddVersion"
          >
            保存草稿
          </el-button>
        </template>
      </el-dialog>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
  import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
  import { useRoute, useRouter } from 'vue-router'
  import type { FormInstance, FormRules, UploadFile } from 'element-plus'
  import { ElMessage } from 'element-plus'
  import { SOURCE_ITEM_STATUS, SOURCE_VERSION_STATUS } from '@/api/source-station'
  import {
    DEVELOPER_INFO_KEY,
    DEVELOPER_TOKEN_KEY,
    fetchSourceDeveloperCatalogApps,
    fetchSourceDeveloperCategories,
    fetchSourceDeveloperGitHubPaidToken,
    fetchSourceDeveloperItems,
    fetchSourceDeveloperMe,
    fetchSourceDeveloperPluginVersions,
    fetchSourceDeveloperTemplateVersions,
    pullSourceDeveloperPlugin,
    pullSourceDeveloperTemplate,
    submitSourceDeveloperPlugin,
    submitSourceDeveloperPluginVersion,
    submitSourceDeveloperTemplate,
    submitSourceDeveloperTemplateVersion,
    saveSourceDeveloperGitHubPaidToken,
    testSourceDeveloperGitHubPaidToken,
    upsertSourceDeveloperPlugin,
    upsertSourceDeveloperPluginVersion,
    upsertSourceDeveloperTemplate,
    upsertSourceDeveloperTemplateVersion,
    uploadSourceDeveloperPackage,
    type SourceDeveloperCatalogApp,
    type SourceDeveloperCatalogItem,
    type SourceDeveloperCategory,
    type SourceDeveloperVersion
  } from '@/api/source-developer'
  import {
    CATALOG_ID_PATTERN,
    SHA256_HEX_PATTERN,
    formatCatalogPriceLabel,
    formatCatalogPriceYuan,
    parseCatalogPriceYuan,
    isHttpsLocation,
    isPrivatePackageLocation,
    isStationPackageLocation,
    isTemplateLocation,
    resolveCatalogPriceCents,
    suggestCatalogSlug
  } from '@/utils/form/catalog-slug'
  import {
    developerCatalogBlockReason,
    isGitHubReleaseAssetURL,
    type CatalogPackageSource
  } from '@/utils/form/catalog-package-source'

  const props = defineProps<{
    kind: 'plugin' | 'template'
  }>()

  const NARROW_QUERY = '(max-width: 768px)'
  const route = useRoute()
  const router = useRouter()
  const isNarrow = ref(typeof window !== 'undefined' && window.matchMedia(NARROW_QUERY).matches)
  let narrowMedia: MediaQueryList | null = null

  function syncNarrow(event?: MediaQueryListEvent) {
    isNarrow.value = event ? event.matches : !!narrowMedia?.matches
  }

  const formDrawerSize = computed(() => (isNarrow.value ? '100%' : '560px'))
  const versionDrawerSize = computed(() => (isNarrow.value ? '100%' : '720px'))
  const versionDialogWidth = computed(() => (isNarrow.value ? '92%' : '480px'))
  const formLabelPosition = computed(() => (isNarrow.value ? 'top' : 'right'))
  const loading = ref(false)
  const saving = ref(false)
  const pullingId = ref('')
  const hashing = ref<'form' | 'version' | ''>('')
  const uploading = ref(false)
  const githubLoading = ref(false)
  const githubSaving = ref(false)
  const githubTesting = ref(false)
  const githubConfigured = ref(false)
  const githubToken = ref('')
  const items = ref<SourceDeveloperCatalogItem[]>([])
  const apps = ref<SourceDeveloperCatalogApp[]>([])
  const categories = ref<SourceDeveloperCategory[]>([])
  const formVisible = ref(false)
  const isEdit = ref(false)
  const idManuallyEdited = ref(false)
  const requirePackageFields = ref(false)
  const advancedOpen = ref<string[]>([])
  const formRef = ref<FormInstance>()
  const currentItem = ref<SourceDeveloperCatalogItem | null>(null)
  const versionVisible = ref(false)
  const versionLoading = ref(false)
  const versionSaving = ref(false)
  const versionFormVisible = ref(false)
  const versions = ref<SourceDeveloperVersion[]>([])
  const form = reactive({
    appId: 0,
    category: '',
    id: '',
    name: '',
    version: '1.0.0',
    description: '',
    icon: 'ri:puzzle-line',
    location: '',
    sha256: '',
    priceYuan: '0',
    packageSource: 'public' as CatalogPackageSource,
    savedGitHubRef: '',
    savedGitHubURL: '',
    authorName: '',
    changelog: ''
  })
  const versionForm = reactive({
    version: '',
    location: '',
    sha256: '',
    changelog: '',
    packageSource: 'public' as CatalogPackageSource,
    savedGitHubRef: '',
    savedGitHubURL: ''
  })

  const title = computed(() => (props.kind === 'template' ? '我的模板' : '我的插件'))
  const createLabel = computed(() => (props.kind === 'template' ? '登记模板' : '登记插件'))
  const emptyText = computed(() =>
    props.kind === 'template'
      ? '还没有模板。点右上角「登记模板」添加第一条。'
      : '还没有插件。点右上角「登记插件」添加第一条。'
  )
  const locationLabel = computed(() => (props.kind === 'template' ? '模板地址' : '下载地址'))
  const formPriceCents = computed(() => parseCatalogPriceYuan(form.priceYuan) || 0)
  const packageSourceHelp = computed(() => {
    if (form.packageSource === 'github') {
      return '粘贴私有仓库的 Release 资产链接。本站用你自己的只读令牌核对一次并记下校验码，不保存压缩包。买家付款后拿到几分钟有效的临时地址。'
    }
    if (form.packageSource === 'upload') {
      return '收费条目可以上传压缩包，由本站托管。免费条目请改用公开地址。'
    }
    return props.kind === 'template'
      ? '只用于免费。可以是 https 网址，或本站上已有文件的相对路径。'
      : '只用于免费。必须是 https 网址。提交审核时会下载并核对压缩包与校验码。'
  })
  function locationPlaceholderFor(source: CatalogPackageSource) {
    if (source === 'upload') return '上传压缩包后自动填写'
    if (source === 'github') {
      return 'https://github.com/所有者/仓库/releases/download/标签/文件名.zip'
    }
    return props.kind === 'template' ? 'https://你的文件地址.zip' : 'https://你的文件地址.zip'
  }
  function locationHelpFor(source: CatalogPackageSource, cents: number) {
    if (source === 'upload') return '本站地址由上传结果填入，提交时只核对本地文件。'
    if (source === 'github') {
      return '保存时本站用你的只读令牌下载核对，自动填写校验码，然后删除临时文件。失败不会保存。'
    }
    if (cents > 0) {
      return '公开地址只能用于免费条目。收费请改用私有 GitHub 仓库，或上传压缩包。'
    }
    return props.kind === 'template'
      ? '填写 https 网址，或本站上已有文件的相对路径。提交审核时会下载并核对压缩包与校验码。'
      : '填写 https 网址。提交审核时会下载并核对是不是压缩包、校验码是否一致。'
  }
  const locationHelp = computed(() => locationHelpFor(form.packageSource, formPriceCents.value))
  const versionLocationHelp = computed(() =>
    locationHelpFor(versionForm.packageSource, currentItem.value?.priceCents || 0)
  )
  const draftBlockReason = computed(() =>
    developerCatalogBlockReason({
      kind: props.kind,
      source: form.packageSource,
      location: form.location,
      sha256: form.sha256,
      priceYuan: form.priceYuan,
      requirePackage: false
    })
  )
  const submitBlockReason = computed(() =>
    developerCatalogBlockReason({
      kind: props.kind,
      source: form.packageSource,
      location: form.location,
      sha256: form.sha256,
      priceYuan: form.priceYuan,
      requirePackage: true
    })
  )
  const versionBlockReason = computed(() =>
    developerCatalogBlockReason({
      kind: props.kind,
      source: versionForm.packageSource,
      location: versionForm.location,
      sha256: versionForm.sha256,
      priceYuan: ((currentItem.value?.priceCents || 0) / 100).toFixed(2),
      requirePackage: true
    })
  )
  const categoryOptions = computed(() =>
    categories.value.filter((item) => item.kind === props.kind)
  )
  const formTitle = computed(() => {
    if (!isEdit.value) return createLabel.value
    const noun = props.kind === 'template' ? '模板' : '插件'
    return canEditMeta.value ? `编辑${noun}` : `查看${noun}`
  })
  const canEditMeta = computed(() => {
    const status = currentItem.value?.status || 'draft'
    return !isEdit.value || status === 'draft' || status === 'rejected'
  })
  const canEditPackage = computed(() => canEditMeta.value && !currentItem.value?.latestVersion)
  const canSubmitCurrent = computed(() => {
    const status = currentItem.value?.status || 'draft'
    return status === 'draft' || status === 'rejected'
  })

  const formRules: FormRules = {
    appId: [{ required: true, type: 'number', min: 1, message: '请选择应用', trigger: 'change' }],
    category: [{ required: true, message: '请选择分类', trigger: 'change' }],
    id: [
      {
        required: true,
        validator: (_: unknown, value: string, callback: (error?: Error) => void) => {
          if (!CATALOG_ID_PATTERN.test(String(value || '').trim())) {
            callback(new Error('标识只能用小写字母、数字和连字符，至少 2 位'))
            return
          }
          callback()
        },
        trigger: 'blur'
      }
    ],
    name: [{ required: true, message: '请填写名称', trigger: 'blur' }],
    version: [{ required: true, message: '请填写版本', trigger: 'blur' }],
    location: [
      {
        validator: (_: unknown, value: string, callback: (error?: Error) => void) => {
          const raw = String(value || '').trim()
          if (!raw) {
            if (requirePackageFields.value) {
              callback(new Error(`提交审核前请填写${locationLabel.value}`))
              return
            }
            callback()
            return
          }
          if (isStationPackageLocation(raw) || isPrivatePackageLocation(raw)) {
            callback()
            return
          }
          if (form.packageSource === 'github') {
            if (!isGitHubReleaseAssetURL(raw)) {
              callback(new Error('请粘贴 GitHub Release 资产链接'))
              return
            }
            callback()
            return
          }
          if (form.packageSource === 'public' && formPriceCents.value > 0) {
            callback(
              new Error('公开地址只能用于免费条目。收费请改用私有 GitHub 仓库，或上传压缩包')
            )
            return
          }
          if (form.packageSource === 'upload') {
            callback(new Error('请先上传压缩包'))
            return
          }
          const ok = props.kind === 'template' ? isTemplateLocation(raw) : isHttpsLocation(raw)
          if (!ok) {
            callback(
              new Error(
                props.kind === 'template'
                  ? '模板地址须为 https 开头，或相对路径如 templates/demo-home.json'
                  : '下载地址须为 https 开头的外链'
              )
            )
            return
          }
          callback()
        },
        trigger: 'blur'
      }
    ],
    sha256: [
      {
        validator: (_: unknown, value: string, callback: (error?: Error) => void) => {
          const raw = String(value || '').trim()
          if (!raw) {
            if (form.packageSource === 'github' || form.packageSource === 'upload') {
              callback()
              return
            }
            if (requirePackageFields.value) {
              callback(new Error('提交审核前请填写校验码'))
              return
            }
            callback()
            return
          }
          if (!SHA256_HEX_PATTERN.test(raw)) {
            callback(new Error('校验码须为 64 位十六进制'))
            return
          }
          callback()
        },
        trigger: 'blur'
      }
    ]
  }

  watch(
    () => form.name,
    () => {
      if (!formVisible.value || isEdit.value || idManuallyEdited.value) return
      if (!form.name.trim()) {
        form.id = ''
        return
      }
      form.id = suggestCatalogSlug(form.name, props.kind, form.id)
    }
  )

  function defaultIcon() {
    return props.kind === 'template' ? 'ri:layout-3-line' : 'ri:puzzle-line'
  }

  function currentDeveloperName() {
    try {
      const info = JSON.parse(localStorage.getItem(DEVELOPER_INFO_KEY) || '{}') as {
        displayName?: string
        username?: string
      }
      return String(info.displayName || info.username || '').trim()
    } catch {
      return ''
    }
  }

  function onIdInput() {
    if (isEdit.value) return
    if (!form.id.trim()) {
      idManuallyEdited.value = false
      if (form.name.trim()) {
        form.id = suggestCatalogSlug(form.name, props.kind, '')
      }
      return
    }
    idManuallyEdited.value = true
  }

  function canAutoHash(location: string) {
    return isHttpsLocation(location)
  }

  function statusMeta(value: string) {
    return SOURCE_ITEM_STATUS[value] || { label: value || '-', type: 'info' as const }
  }

  function versionStatusMeta(value: string) {
    return SOURCE_VERSION_STATUS[value] || { label: value || '-', type: 'info' as const }
  }

  function appLabel(appId?: number) {
    if (!appId) return '-'
    const app = apps.value.find((item) => item.id === appId)
    if (!app) return `应用 #${appId}`
    return `${app.name}（${app.appKey}）`
  }

  function categoryLabel(key?: string) {
    if (!key) return '-'
    return categories.value.find((item) => item.key === key)?.label || key
  }

  function canEditItem(row: SourceDeveloperCatalogItem) {
    return row.status === 'draft' || row.status === 'rejected'
  }

  function canSubmitItem(row: SourceDeveloperCatalogItem) {
    return row.status === 'draft' || row.status === 'rejected'
  }

  function logoutToLogin() {
    localStorage.removeItem(DEVELOPER_TOKEN_KEY)
    localStorage.removeItem(DEVELOPER_INFO_KEY)
    router.replace('/developer-panel/login')
  }

  function unwrapCode<T extends { code?: number; msg?: string; data?: unknown }>(res: {
    status?: number
    data?: T
  }) {
    if (res.status === 401 || res.data?.code === 401) {
      logoutToLogin()
      return null
    }
    return res.data || null
  }

  async function refreshDeveloperProfile() {
    try {
      const res = await fetchSourceDeveloperMe()
      const body = unwrapCode(res)
      if (!body || body.code !== 200 || !body.data) return
      const profile = body.data
      localStorage.setItem(
        DEVELOPER_INFO_KEY,
        JSON.stringify({
          username: profile.username,
          displayName: profile.displayName
        })
      )
      form.authorName = profile.displayName || profile.username || currentDeveloperName()
    } catch {
      form.authorName = currentDeveloperName()
    }
  }

  async function loadAll() {
    loading.value = true
    try {
      const [itemsRes, appsRes, catRes] = await Promise.all([
        fetchSourceDeveloperItems(),
        fetchSourceDeveloperCatalogApps(),
        fetchSourceDeveloperCategories()
      ])
      const itemsBody = unwrapCode(itemsRes)
      const appsBody = unwrapCode(appsRes)
      const catBody = unwrapCode(catRes)
      if (!itemsBody || !appsBody || !catBody) return
      if (appsBody.code === 200) apps.value = appsBody.data?.list || []
      if (catBody.code === 200) categories.value = catBody.data?.list || []
      if (itemsBody.code === 200) {
        items.value =
          props.kind === 'template'
            ? itemsBody.data?.homeTemplates || []
            : itemsBody.data?.plugins || []
      } else {
        ElMessage.error(itemsBody.msg || '加载失败')
      }
    } catch (error: unknown) {
      if ((error as { response?: { status?: number } })?.response?.status === 401) {
        logoutToLogin()
        return
      }
      ElMessage.error('加载开发者目录失败')
    } finally {
      loading.value = false
    }
  }

  function resetForm() {
    requirePackageFields.value = false
    idManuallyEdited.value = false
    advancedOpen.value = []
    form.appId = apps.value[0]?.id || 0
    form.category =
      categoryOptions.value[0]?.key || (props.kind === 'template' ? 'home-template' : 'other')
    form.id = ''
    form.name = ''
    form.version = '1.0.0'
    form.description = ''
    form.icon = defaultIcon()
    form.location = ''
    form.sha256 = ''
    form.priceYuan = '0'
    form.packageSource = 'public'
    form.savedGitHubRef = ''
    form.savedGitHubURL = ''
    form.authorName = currentDeveloperName()
    form.changelog = ''
  }

  function openEdit(row?: SourceDeveloperCatalogItem) {
    isEdit.value = Boolean(row)
    currentItem.value = row || null
    requirePackageFields.value = false
    advancedOpen.value = []
    if (row) {
      idManuallyEdited.value = true
      form.appId = row.appId || 0
      form.category =
        row.category ||
        categoryOptions.value[0]?.key ||
        (props.kind === 'template' ? 'home-template' : 'other')
      form.id = row.id
      form.name = row.name
      form.version = row.version || '1.0.0'
      form.description = row.description || ''
      form.icon = row.icon || defaultIcon()
      form.location = (props.kind === 'template' ? row.templateUrl : row.downloadUrl) || ''
      form.sha256 = row.sha256 || ''
      form.priceYuan = formatCatalogPriceYuan(row.priceCents)
      applyStoredPackageSource(form, row)
      form.authorName = row.author?.name || currentDeveloperName()
      form.changelog = row.changelog || ''
    } else {
      resetForm()
      void refreshDeveloperProfile()
    }
    formVisible.value = true
  }

  function openVersionForm() {
    versionForm.version = ''
    versionForm.location = ''
    versionForm.sha256 = ''
    versionForm.changelog = ''
    versionForm.packageSource = (currentItem.value?.priceCents || 0) > 0 ? 'github' : 'public'
    versionForm.savedGitHubRef = ''
    versionForm.savedGitHubURL = ''
    versionFormVisible.value = true
  }

  function githubReleaseURL(row: {
    githubOwner?: string
    githubRepo?: string
    githubTag?: string
    githubAsset?: string
  }) {
    if (!row.githubOwner || !row.githubRepo || !row.githubTag || !row.githubAsset) return ''
    const tag = encodeURIComponent(row.githubTag)
    const asset = encodeURIComponent(row.githubAsset)
    return `https://github.com/${row.githubOwner}/${row.githubRepo}/releases/download/${tag}/${asset}`
  }

  function sourceLabel(row: {
    packageSource?: string
    githubOwner?: string
    githubRepo?: string
    githubTag?: string
    githubAsset?: string
    originUrl?: string
  }) {
    if (row.packageSource === 'github' && row.githubOwner && row.githubRepo) {
      return `私有仓库 ${row.githubOwner}/${row.githubRepo} @ ${row.githubTag} / ${row.githubAsset}`
    }
    return row.originUrl || '-'
  }

  function versionLocationLabel(row: SourceDeveloperVersion) {
    const labeled = sourceLabel(row)
    if (labeled !== '-') return labeled
    return (props.kind === 'template' ? row.templateUrl : row.downloadUrl) || '-'
  }

  function applyStoredPackageSource(
    bucket: {
      location: string
      packageSource: CatalogPackageSource
      savedGitHubRef: string
      savedGitHubURL: string
    },
    row: SourceDeveloperCatalogItem
  ) {
    const raw = bucket.location
    if (row.packageSource === 'github' || raw.startsWith('github:')) {
      bucket.packageSource = 'github'
      bucket.savedGitHubRef = raw.startsWith('github:') ? raw : ''
      const restored = githubReleaseURL(row)
      bucket.savedGitHubURL = restored
      bucket.location = restored || (isGitHubReleaseAssetURL(raw) ? raw : '')
      return
    }
    bucket.savedGitHubRef = ''
    bucket.savedGitHubURL = ''
    bucket.packageSource =
      isStationPackageLocation(raw) || isPrivatePackageLocation(raw) ? 'upload' : 'public'
  }

  function packageLocationToSave(bucket: {
    packageSource: CatalogPackageSource
    location: string
    savedGitHubRef: string
    savedGitHubURL: string
  }) {
    const location = bucket.location.trim()
    if (
      bucket.packageSource === 'github' &&
      bucket.savedGitHubRef &&
      location === bucket.savedGitHubURL
    ) {
      return bucket.savedGitHubRef
    }
    return location
  }

  function onPackageSourceChange(target: 'form' | 'version') {
    const bucket = target === 'form' ? form : versionForm
    const hosted =
      isStationPackageLocation(bucket.location) || isPrivatePackageLocation(bucket.location)
    const githubish =
      bucket.location.startsWith('github:') || isGitHubReleaseAssetURL(bucket.location)
    if (bucket.packageSource === 'upload' && !hosted) {
      bucket.location = ''
      bucket.sha256 = ''
    }
    if (bucket.packageSource === 'public' && (hosted || bucket.location.startsWith('github:'))) {
      bucket.location = ''
      bucket.sha256 = ''
    }
    if (bucket.packageSource === 'github' && hosted && !githubish) {
      bucket.location = ''
      bucket.sha256 = ''
    }
  }

  async function handlePackageFile(target: 'form' | 'version', upload: UploadFile) {
    const selected = upload.raw
    if (!selected || uploading.value) return
    uploading.value = true
    try {
      const data = new FormData()
      data.append('file', selected)
      data.append('kind', props.kind)
      const res = await uploadSourceDeveloperPackage(data)
      const body = unwrapCode(res)
      if (!body) return
      if (body.code !== 200 || !body.data?.url || !body.data.sha256) {
        ElMessage.error(body.msg || '上传失败')
        return
      }
      const manifest = body.data
      if (target === 'form') {
        form.packageSource = 'upload'
        form.location = manifest.url
        form.sha256 = manifest.sha256
        if (!isEdit.value) {
          if (!form.name.trim() && manifest.name) form.name = manifest.name
          if (!idManuallyEdited.value && manifest.id) {
            form.id = manifest.id
            idManuallyEdited.value = true
          }
          if (manifest.version) form.version = manifest.version
          if (!form.description.trim() && manifest.description) {
            form.description = manifest.description
          }
          if (manifest.category) form.category = manifest.category
          if (props.kind === 'plugin' && manifest.icon) form.icon = manifest.icon
        }
      } else {
        versionForm.packageSource = 'upload'
        versionForm.location = manifest.url
        versionForm.sha256 = manifest.sha256
        if (!versionForm.version.trim() && manifest.version) versionForm.version = manifest.version
      }
      ElMessage.success('已上传，地址和校验码已填入')
    } catch {
      ElMessage.error('上传失败')
    } finally {
      uploading.value = false
    }
  }

  async function handleAutoHash(target: 'form' | 'version') {
    const location = target === 'form' ? form.location.trim() : versionForm.location.trim()
    if (!isHttpsLocation(location)) {
      ElMessage.warning(`请先填写有效的${locationLabel.value}（https 外链）`)
      return
    }
    hashing.value = target
    try {
      const res = await fetch(location, { mode: 'cors' })
      if (!res.ok) {
        throw new Error(`读取失败（${res.status}）`)
      }
      const payload = await res.arrayBuffer()
      const digest = await crypto.subtle.digest('SHA-256', payload)
      const hex = [...new Uint8Array(digest)]
        .map((byte) => byte.toString(16).padStart(2, '0'))
        .join('')
      if (target === 'form') form.sha256 = hex
      else versionForm.sha256 = hex
      ElMessage.success('校验码已填入')
    } catch {
      ElMessage.warning(
        '浏览器无法直接读取该地址（多为跨域限制）。请在本地用 sha256sum 计算后粘贴，也可先保存草稿稍后补。'
      )
    } finally {
      hashing.value = ''
    }
  }

  async function handleSave(submitAfter: boolean) {
    requirePackageFields.value = submitAfter
    try {
      await formRef.value?.validate()
    } catch (invalid) {
      if (submitAfter) {
        const keys = invalid && typeof invalid === 'object' ? Object.keys(invalid) : []
        if (keys.includes('location') || keys.includes('sha256')) {
          ElMessage.warning(`提交审核前请先填写${locationLabel.value}和校验码`)
        }
      }
      return
    }
    const blocked = submitAfter ? submitBlockReason.value : draftBlockReason.value
    if (blocked) {
      ElMessage.warning(blocked)
      return
    }
    if (!form.id.trim()) {
      form.id = suggestCatalogSlug(form.name, props.kind, '')
    }
    const location = packageLocationToSave(form)
    const priced = resolveCatalogPriceCents(form.priceYuan, location)
    if (priced.error) {
      ElMessage.warning(priced.error)
      return
    }
    saving.value = true
    try {
      const authorName = form.authorName.trim() || currentDeveloperName()
      const saveRes =
        props.kind === 'template'
          ? await upsertSourceDeveloperTemplate({
              id: form.id.trim(),
              templateKey: form.id.trim(),
              appId: form.appId,
              category: form.category,
              name: form.name.trim(),
              description: form.description.trim(),
              version: form.version.trim() || '1.0.0',
              schemaVersion: 1,
              sha256: form.sha256.trim(),
              templateUrl: location,
              packageSource: form.packageSource,
              priceCents: priced.cents,
              changelog: form.changelog.trim(),
              author: authorName ? { name: authorName } : undefined
            })
          : await upsertSourceDeveloperPlugin({
              id: form.id.trim(),
              appId: form.appId,
              category: form.category,
              name: form.name.trim(),
              description: form.description.trim(),
              icon: form.icon.trim() || defaultIcon(),
              version: form.version.trim() || '1.0.0',
              sha256: form.sha256.trim(),
              downloadUrl: location,
              packageSource: form.packageSource,
              priceCents: priced.cents,
              changelog: form.changelog.trim(),
              author: authorName ? { name: authorName } : undefined
            })
      const saveBody = unwrapCode(saveRes)
      if (!saveBody) return
      if (saveBody.code !== 200) {
        ElMessage.error(saveBody.msg || '保存失败')
        return
      }
      const saved = saveBody.data
      if (submitAfter) {
        const submitRes =
          props.kind === 'template'
            ? await submitSourceDeveloperTemplate(saved.id)
            : await submitSourceDeveloperPlugin(saved.id)
        const submitBody = unwrapCode(submitRes)
        if (!submitBody) return
        if (submitBody.code !== 200) {
          ElMessage.error(submitBody.msg || '提交失败')
          return
        }
        ElMessage.success('已提交审核')
      } else {
        ElMessage.success(saveBody.msg || '草稿已保存')
      }
      formVisible.value = false
      await loadAll()
    } finally {
      saving.value = false
      requirePackageFields.value = false
    }
  }

  async function handleSubmit(row: SourceDeveloperCatalogItem) {
    const location = props.kind === 'template' ? row.templateUrl : row.downloadUrl
    if (!row.sha256 || !location) {
      ElMessage.warning(`提交审核前请先填写${locationLabel.value}和校验码`)
      openEdit(row)
      return
    }
    const res =
      props.kind === 'template'
        ? await submitSourceDeveloperTemplate(row.id)
        : await submitSourceDeveloperPlugin(row.id)
    const body = unwrapCode(res)
    if (!body) return
    if (body.code !== 200) {
      ElMessage.error(body.msg || '提交失败')
      return
    }
    ElMessage.success('已提交审核')
    await loadAll()
  }

  async function handlePull(row: SourceDeveloperCatalogItem) {
    if (!row.originUrl || pullingId.value) return
    pullingId.value = row.id
    try {
      const res =
        props.kind === 'template'
          ? await pullSourceDeveloperTemplate(row.id)
          : await pullSourceDeveloperPlugin(row.id)
      const body = unwrapCode(res)
      if (!body) return
      if (body.code !== 200) {
        ElMessage.error(body.msg || '重新拉取失败，原托管包未改动')
        await loadAll()
        return
      }
      ElMessage.success(body.msg || '已重新拉取并更新托管包')
      await loadAll()
    } finally {
      pullingId.value = ''
    }
  }

  async function openVersions(row: SourceDeveloperCatalogItem) {
    currentItem.value = row
    versionVisible.value = true
    versionLoading.value = true
    try {
      const res =
        props.kind === 'template'
          ? await fetchSourceDeveloperTemplateVersions(row.id)
          : await fetchSourceDeveloperPluginVersions(row.id)
      const body = unwrapCode(res)
      if (!body) return
      versions.value = body.data?.list || []
    } finally {
      versionLoading.value = false
    }
  }

  async function handleAddVersion() {
    if (!currentItem.value) return
    if (!versionForm.version.trim()) {
      ElMessage.warning('请填写版本')
      return
    }
    if (versionBlockReason.value) {
      ElMessage.warning(versionBlockReason.value)
      return
    }
    const location = packageLocationToSave(versionForm)
    versionSaving.value = true
    try {
      const payload =
        props.kind === 'template'
          ? {
              version: versionForm.version.trim(),
              templateUrl: location,
              sha256: versionForm.sha256.trim(),
              packageSource: versionForm.packageSource,
              changelog: versionForm.changelog.trim()
            }
          : {
              version: versionForm.version.trim(),
              downloadUrl: location,
              sha256: versionForm.sha256.trim(),
              packageSource: versionForm.packageSource,
              changelog: versionForm.changelog.trim()
            }
      const res =
        props.kind === 'template'
          ? await upsertSourceDeveloperTemplateVersion(currentItem.value.id, payload)
          : await upsertSourceDeveloperPluginVersion(currentItem.value.id, payload)
      const body = unwrapCode(res)
      if (!body) return
      if (body.code !== 200) {
        ElMessage.error(body.msg || '保存版本失败')
        return
      }
      ElMessage.success(body.msg || '版本草稿已保存')
      versionFormVisible.value = false
      versionForm.version = ''
      versionForm.location = ''
      versionForm.sha256 = ''
      versionForm.changelog = ''
      versionForm.packageSource = 'public'
      versionForm.savedGitHubRef = ''
      versionForm.savedGitHubURL = ''
      await openVersions(currentItem.value)
    } finally {
      versionSaving.value = false
    }
  }

  async function handleSubmitVersion(row: SourceDeveloperVersion) {
    if (!currentItem.value) return
    const res =
      props.kind === 'template'
        ? await submitSourceDeveloperTemplateVersion(currentItem.value.id, row.version)
        : await submitSourceDeveloperPluginVersion(currentItem.value.id, row.version)
    const body = unwrapCode(res)
    if (!body) return
    if (body.code !== 200) {
      ElMessage.error(body.msg || '提交版本失败')
      return
    }
    ElMessage.success('版本已提交审核')
    await openVersions(currentItem.value)
  }

  async function loadGitHubToken() {
    githubLoading.value = true
    try {
      const res = await fetchSourceDeveloperGitHubPaidToken()
      const body = unwrapCode(res)
      const data = body?.data as { configured?: boolean } | undefined
      if (body?.code === 200) githubConfigured.value = Boolean(data?.configured)
    } catch {
      githubConfigured.value = false
    } finally {
      githubLoading.value = false
    }
  }

  async function handleGitHubSave() {
    githubSaving.value = true
    try {
      const res = await saveSourceDeveloperGitHubPaidToken(githubToken.value.trim())
      const body = unwrapCode(res)
      if (!body) return
      if (body.code !== 200) {
        ElMessage.error(body.msg || '保存令牌失败')
        return
      }
      githubConfigured.value = true
      githubToken.value = ''
      ElMessage.success(body.msg || '已保存只读令牌')
    } catch {
      ElMessage.error('保存令牌失败')
    } finally {
      githubSaving.value = false
    }
  }

  async function handleGitHubTest() {
    githubTesting.value = true
    try {
      const res = await testSourceDeveloperGitHubPaidToken(githubToken.value.trim())
      const body = unwrapCode(res)
      if (!body) return
      if (body.code !== 200) {
        ElMessage.error(body.msg || '令牌不可用')
        return
      }
      ElMessage.success(body.msg || '令牌可用')
    } catch {
      ElMessage.error('令牌不可用')
    } finally {
      githubTesting.value = false
    }
  }

  onMounted(async () => {
    narrowMedia = window.matchMedia(NARROW_QUERY)
    syncNarrow()
    narrowMedia.addEventListener('change', syncNarrow)
    await Promise.all([loadAll(), loadGitHubToken()])
    if (route.query.create === '1') {
      openEdit()
    }
  })

  onBeforeUnmount(() => {
    narrowMedia?.removeEventListener('change', syncNarrow)
  })
</script>

<style scoped lang="scss">
  .table-header {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    align-items: flex-start;
    justify-content: space-between;
  }

  .card-title {
    font-weight: 600;
  }

  .token-card {
    margin-bottom: 16px;
  }

  .token-form {
    margin-top: 16px;
  }

  .card-hint {
    margin: 6px 0 0;
    font-size: 13px;
    line-height: 1.5;
    color: var(--el-text-color-secondary);
  }

  .field-help {
    margin: 6px 0 0;
    font-size: 12px;
    line-height: 1.5;
    color: var(--el-text-color-secondary);
  }

  :deep(.el-radio) {
    height: auto;
    margin-right: 16px;
    white-space: normal;
  }

  .checksum-row {
    display: flex;
    gap: 8px;
    width: 100%;

    .el-input {
      flex: 1;
      min-width: 0;
    }
  }

  @media (width <= 768px) {
    .checksum-row {
      flex-wrap: wrap;
    }

    :deep(.el-form-item) {
      margin-bottom: 8px;
    }

    :deep(.el-form-item__content) {
      min-width: 0;
    }

    .field-help {
      margin-top: 2px;
      line-height: 1.35;
    }
  }

  .advanced-collapse {
    margin: 4px 0 16px;
    border: none;

    :deep(.el-collapse-item__header) {
      height: 40px;
      font-size: 13px;
      color: var(--el-text-color-regular);
      background: transparent;
      border: none;
    }

    :deep(.el-collapse-item__wrap) {
      background: transparent;
      border: none;
    }

    :deep(.el-collapse-item__content) {
      padding-bottom: 4px;
    }
  }

  .is-secondary {
    :deep(.el-form-item__label) {
      color: var(--el-text-color-secondary);
    }
  }

  .table-actions {
    display: flex;
    justify-content: flex-end;
  }

  .mb-3 {
    margin-bottom: 12px;
  }
</style>

<style lang="scss">
  /* 登记插件 / 登记模板抽屉，以及新增版本信息弹框。弹层挂在 body 上，手机宽度写在这里。 */
  @media (width <= 768px) {
    .el-drawer.catalog-sheet {
      display: flex;
      flex-direction: column;
      width: 100% !important;
      max-width: 100vw !important;
      height: 100dvh !important;
      max-height: 100dvh !important;
    }

    .el-drawer.catalog-sheet .el-drawer__body {
      flex: 1 1 auto;
      min-height: 0;
      overflow-y: auto;
    }

    .el-dialog.catalog-sheet {
      display: flex;
      flex-direction: column;
      width: 92% !important;
      max-width: calc(100vw - 16px) !important;
      max-height: calc(100dvh - 32px);
      margin: 16px auto !important;
      overflow: hidden;
    }

    .el-dialog.catalog-sheet .el-dialog__body {
      flex: 1 1 auto;
      min-height: 0;
      max-height: calc(100dvh - 180px);
      overflow-y: auto;
    }

    .el-drawer.catalog-sheet .el-drawer__footer,
    .el-dialog.catalog-sheet .el-dialog__footer {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      justify-content: flex-end;
    }
  }
</style>
