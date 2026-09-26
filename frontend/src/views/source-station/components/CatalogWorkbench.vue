<template>
  <div class="source-station-page">
    <el-card shadow="never" class="art-card mb-4 filter-panel">
      <el-form :model="searchForm" inline>
        <el-form-item label="应用">
          <el-select
            v-model="searchForm.appId"
            placeholder="请选择应用"
            style="width: 240px"
            @change="onAppChange"
          >
            <el-option
              v-for="app in apps"
              :key="app.id"
              :label="appLabel(app)"
              :value="app.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="searchForm.status" placeholder="默认（不含已弃用）" clearable style="width: 180px">
            <el-option label="草稿" value="draft" />
            <el-option label="待审核" value="review" />
            <el-option label="已通过" value="approved" />
            <el-option label="已上架" value="published" />
            <el-option label="已下架" value="hidden" />
            <el-option label="已驳回" value="rejected" />
            <el-option label="已弃用" value="deprecated" />
          </el-select>
        </el-form-item>
        <el-form-item label="分类">
          <el-select v-model="searchForm.category" placeholder="全部" clearable style="width: 180px">
            <el-option
              v-for="item in categories"
              :key="item.key"
              :label="item.label"
              :value="item.key"
            />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="loadItems">查询</el-button>
          <el-button @click="resetSearch">重置</el-button>
          <el-button @click="openCategoryManager">管理分类</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" class="art-card">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">软件目录（共 {{ tableData.length }} 条）</span>
            <p class="card-hint">
              先选择应用，再按分类筛选。源站只保存说明和下载地址，不保存源码。上架后会自动出现在该应用的软件源里。下架后商店不再显示，已经安装的不会被远程卸掉。弃用后会从公开目录移除，默认列表也不再显示；需要查看时，把状态筛成「已弃用」。
            </p>
          </div>
          <div class="table-actions">
            <el-button :disabled="!searchForm.appId" @click="openRegister">登记外部地址</el-button>
            <el-button type="primary" :disabled="!searchForm.appId" @click="openUpload"
              >上传压缩包</el-button
            >
          </div>
        </div>
      </template>

      <el-table :data="tableData" stripe v-loading="loading">
        <el-table-column prop="id" label="标识" min-width="140" show-overflow-tooltip />
        <el-table-column prop="name" label="名称" min-width="140" show-overflow-tooltip />
        <el-table-column label="分类" width="120">
          <template #default="{ row }">
            {{ row.categoryLabel || categoryLabel(row.category) }}
          </template>
        </el-table-column>
        <el-table-column label="当前版本" width="110">
          <template #default="{ row }">
            {{ row.latestVersion || row.version || '-' }}
          </template>
        </el-table-column>
        <el-table-column label="售价" width="100">
          <template #default="{ row }">{{ formatCatalogPriceLabel(row.priceCents) }}</template>
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
            {{ itemLocation(row) }}
          </template>
        </el-table-column>
        <el-table-column label="来源外链" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ row.originUrl || '-' }}</span>
            <p v-if="row.originHint" class="card-hint">{{ row.originHint }}</p>
          </template>
        </el-table-column>
        <el-table-column prop="sha256" label="校验码" min-width="160" show-overflow-tooltip />
        <el-table-column prop="updatedAt" label="更新时间" width="170" />
        <el-table-column label="操作" width="420" fixed="right">
          <template #default="{ row }">
            <el-button
              v-if="canEditItem(row.status)"
              link
              type="primary"
              size="small"
              @click="openEdit(row)"
              >编辑</el-button
            >
            <el-button
              v-if="row.originUrl"
              link
              type="primary"
              size="small"
              :loading="pullingId === row.id"
              @click="handlePull(row)"
              >重新拉取</el-button
            >
            <el-button
              v-if="canAction(row.status, 'approve')"
              link
              type="primary"
              size="small"
              @click="runStatus(row, 'approve')"
              >通过</el-button
            >
            <el-button
              v-if="canAction(row.status, 'reject')"
              link
              type="warning"
              size="small"
              @click="runStatus(row, 'reject')"
              >拒绝</el-button
            >
            <el-button
              v-if="canAction(row.status, 'shelf')"
              link
              type="success"
              size="small"
              @click="runStatus(row, 'shelf')"
              >上架</el-button
            >
            <el-button
              v-if="canAction(row.status, 'unshelf')"
              link
              type="info"
              size="small"
              @click="runStatus(row, 'unshelf')"
              >下架</el-button
            >
            <el-button
              v-if="canAction(row.status, 'deprecate')"
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

    <el-dialog v-model="uploadVisible" title="上传安装包" width="640px" destroy-on-close>
      <el-alert
        type="warning"
        :closable="false"
        show-icon
        title="校验不通过就会拒绝：压缩包里要有合法的插件或模板清单，不能包含越界路径。失败不会保存，也不会推送到发布页。"
        class="mb-3"
      />
      <p class="card-hint mb-3">
        压缩包和外部地址二选一。只填 https 时，本站下载并校验，自动填写校验码。付费条目私有托管，买家看不到外链。
      </p>
      <el-form label-width="120px">
        <el-form-item label="应用" required>
          <el-select v-model="uploadForm.appId" placeholder="请选择应用" style="width: 100%">
            <el-option
              v-for="app in apps"
              :key="app.id"
              :label="appLabel(app)"
              :value="app.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="压缩包">
          <el-upload
            drag
            :auto-upload="false"
            :show-file-list="false"
            accept=".zip,application/zip"
            :on-change="selectUploadFile"
          >
            <div>{{ uploadFile ? uploadFile.name : '点击或拖拽压缩包（不超过 20 MB）' }}</div>
          </el-upload>
          <p class="card-hint">与外部地址二选一。推送 Release 时必须选择压缩包。</p>
        </el-form-item>
        <el-form-item label="分类">
          <el-select
            v-model="uploadForm.category"
            clearable
            placeholder="可留空，按包内清单自动识别"
            style="width: 100%"
          >
            <el-option
              v-for="item in uploadCategoryOptions"
              :key="item.key"
              :label="`${item.label}（${item.kind === 'template' ? '模板清单' : '插件清单'}）`"
              :value="item.key"
            />
          </el-select>
        </el-form-item>
        <el-form-item v-if="parsedManifest" label="解析结果">
          <el-descriptions :column="1" border size="small">
            <el-descriptions-item label="分类">{{
              categoryLabel(parsedManifest.category || uploadForm.category)
            }}</el-descriptions-item>
            <el-descriptions-item label="ID">{{ parsedManifest.id }}</el-descriptions-item>
            <el-descriptions-item label="名称">{{ parsedManifest.name }}</el-descriptions-item>
            <el-descriptions-item label="版本">{{ parsedManifest.version }}</el-descriptions-item>
            <el-descriptions-item label="作者">{{
              parsedManifest.author?.name
            }}</el-descriptions-item>
            <el-descriptions-item label="校验码">{{ parsedManifest.sha256 }}</el-descriptions-item>
          </el-descriptions>
        </el-form-item>
        <el-form-item label="更新说明">
          <el-input v-model="uploadForm.changelog" type="textarea" :rows="2" placeholder="可选" />
        </el-form-item>
        <el-form-item label="售价（元）">
          <el-input v-model="uploadForm.priceYuan" placeholder="0" />
          <p class="card-hint">
            填 0 表示免费，校验后仍由外部地址提供下载。大于 0
            时本站立即拉取并私有托管，买家看不到外链。
          </p>
        </el-form-item>
        <el-form-item label="选项">
          <el-checkbox v-model="uploadForm.push">推送 GitHub/Gitee Release</el-checkbox>
        </el-form-item>
        <el-form-item v-if="!uploadForm.push" label="外部地址">
          <el-input
            v-model="uploadForm.location"
            placeholder="https://... 与压缩包二选一"
            @input="parsedManifest = null"
          />
          <p class="card-hint">
            与压缩包二选一。只填 https 网址时，本站下载、校验并自动填写校验码。GitHub Release
            的跳转也会跟着走，每一跳都拒绝内网地址。
          </p>
        </el-form-item>
      </el-form>
      <p v-if="uploadBlockReason" class="card-hint">{{ uploadBlockReason }}</p>
      <template #footer>
        <el-button @click="uploadVisible = false">取消</el-button>
        <el-tooltip :disabled="!uploadBlockReason" :content="uploadBlockReason" placement="top">
          <span>
            <el-button :disabled="!!uploadBlockReason" :loading="parsing" @click="handleParse"
              >仅校验解析</el-button
            >
          </span>
        </el-tooltip>
        <el-tooltip :disabled="!uploadBlockReason" :content="uploadBlockReason" placement="top">
          <span>
            <el-button
              type="primary"
              :disabled="!!uploadBlockReason"
              :loading="publishing"
              @click="handlePublish"
            >
              校验并保存元数据
            </el-button>
          </span>
        </el-tooltip>
      </template>
    </el-dialog>

    <el-dialog v-model="editVisible" title="编辑目录项" width="560px" destroy-on-close>
      <el-alert
        type="info"
        :closable="false"
        show-icon
        class="mb-3"
        title="已上架条目可直接改名称、地址、校验码等元数据，不会自动退回待审核。标识与所属应用创建后不可改。"
      />
      <el-form ref="editRef" :model="editForm" :rules="editRules" label-width="110px">
        <el-form-item label="应用">
          <el-input :model-value="editApp ? appLabel(editApp) : '-'" disabled />
        </el-form-item>
        <el-form-item label="标识">
          <el-input v-model="editForm.id" disabled />
        </el-form-item>
        <el-form-item label="分类" prop="category">
          <el-select v-model="editForm.category" style="width: 100%">
            <el-option
              v-for="item in editCategoryOptions"
              :key="item.key"
              :label="item.label"
              :value="item.key"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="名称" prop="name">
          <el-input v-model="editForm.name" maxlength="100" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="editForm.description" type="textarea" :rows="2" maxlength="500" />
        </el-form-item>
        <el-form-item label="售价（元）">
          <el-input v-model="editForm.priceYuan" placeholder="0" />
          <p class="card-hint">填 0 表示免费。大于 0 为买断：可上传本站托管的压缩包，或填写 https 网址由本站立即拉取并私有托管。付费上架尚未开放。</p>
        </el-form-item>
        <el-form-item label="下载地址" prop="location">
          <el-input v-model="editForm.location" placeholder="https://..." />
        </el-form-item>
        <el-form-item v-if="editingItem?.originUrl" label="来源外链">
          <el-input :model-value="editingItem.originUrl" disabled />
          <p class="card-hint">
            {{ editingItem.originHint || '本站已拉取并私有托管。买家看不到这条外链。' }}
          </p>
        </el-form-item>
        <el-form-item label="校验码" prop="sha256">
          <el-input v-model="editForm.sha256" />
        </el-form-item>
        <el-form-item label="作者">
          <el-input v-model="editForm.authorName" placeholder="作者名称" />
        </el-form-item>
        <el-form-item v-if="!editIsTemplate" label="图标">
          <el-input v-model="editForm.icon" placeholder="ri:puzzle-line" />
        </el-form-item>
        <el-form-item label="更新说明">
          <el-input v-model="editForm.changelog" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item label="审计备注">
          <el-input v-model="editForm.note" placeholder="可选，写入审计日志" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" :loading="editing" @click="handleEdit">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="registerVisible" title="登记外部地址" width="560px" destroy-on-close>
      <el-form ref="registerRef" :model="registerForm" :rules="registerRules" label-width="110px">
        <el-form-item label="应用" prop="appId">
          <el-select v-model="registerForm.appId" placeholder="请选择应用" style="width: 100%">
            <el-option
              v-for="app in apps"
              :key="app.id"
              :label="appLabel(app)"
              :value="app.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="分类" prop="category">
          <el-select v-model="registerForm.category" style="width: 100%">
            <el-option
              v-for="item in categories"
              :key="item.key"
              :label="item.label"
              :value="item.key"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="标识" prop="id">
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
        <el-form-item label="售价（元）">
          <el-input v-model="registerForm.priceYuan" placeholder="0" />
          <p class="card-hint">填 0 表示免费。大于 0 可上传本站托管的压缩包，或填写 https 网址由本站立即拉取并私有托管。付费上架尚未开放。</p>
        </el-form-item>
        <el-form-item label="下载地址" prop="location">
          <el-input v-model="registerForm.location" placeholder="https://..." />
        </el-form-item>
        <el-form-item label="校验码" prop="sha256">
          <el-input v-model="registerForm.sha256" />
        </el-form-item>
        <el-form-item label="作者">
          <el-input v-model="registerForm.authorName" placeholder="作者名称" />
        </el-form-item>
        <el-form-item label="更新说明">
          <el-input v-model="registerForm.changelog" />
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="registerForm.shelf"
            >登记后直接上架（须同时有下载地址和校验码）</el-checkbox
          >
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="registerVisible = false">取消</el-button>
        <el-button type="primary" :loading="registering" @click="handleRegister">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="categoryVisible" title="目录分类" width="640px" destroy-on-close>
      <p class="card-hint mb-3">
        内置分类包含插件和首页模板，也可作为应用里的二级筛选。可以自行添加或删除额外分类。应用商店会按这些分类生成筛选页签。
      </p>
      <el-table :data="categories" size="small" class="mb-3">
        <el-table-column prop="label" label="名称" min-width="120" />
        <el-table-column prop="key" label="标识" min-width="140" />
        <el-table-column label="清单类型" width="110">
          <template #default="{ row }">
            {{ row.kind === 'template' ? '首页模板' : '插件' }}
          </template>
        </el-table-column>
        <el-table-column label="来源" width="80">
          <template #default="{ row }">{{ row.builtin ? '内置' : '自定义' }}</template>
        </el-table-column>
        <el-table-column label="操作" width="80" align="center">
          <template #default="{ row }">
            <el-button
              v-if="canDeleteCatalogCategory(row)"
              link
              type="danger"
              size="small"
              :disabled="savingCategories"
              @click="handleDeleteCategory(row)"
              >删除</el-button
            >
          </template>
        </el-table-column>
      </el-table>
      <el-form :model="extraForm" inline>
        <el-form-item label="新分类标识">
          <el-input v-model="extraForm.key" placeholder="theme" />
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="extraForm.label" placeholder="主题" />
        </el-form-item>
        <el-form-item label="清单">
          <el-select v-model="extraForm.kind" style="width: 140px">
            <el-option label="插件" value="plugin" />
            <el-option label="首页模板" value="template" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="savingCategories" @click="handleAddCategory"
            >添加</el-button
          >
        </el-form-item>
      </el-form>
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
        <el-table-column label="地址" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            {{ currentIsTemplate ? row.templateUrl : row.downloadUrl }}
          </template>
        </el-table-column>
        <el-table-column prop="changelog" label="说明" min-width="140" show-overflow-tooltip />
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
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
              >驳回</el-button
            >
            <el-button
              v-if="canVersionAction(row.status, 'latest')"
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
          <el-form-item label="下载地址" required>
            <el-input v-model="versionForm.location" placeholder="https://..." />
          </el-form-item>
          <el-form-item label="校验码">
            <el-input v-model="versionForm.sha256" placeholder="付费外链可留空，由本站拉取后计算" />
            <p class="card-hint">免费外链仍须填写 64 位校验码。付费条目填写 https 网址时，保存时本站拉取并自动计算。</p>
          </el-form-item>
          <el-form-item label="更新说明">
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
  import { useRoute } from 'vue-router'
  import type { FormInstance, FormRules, UploadFile } from 'element-plus'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import {
    SOURCE_ITEM_STATUS,
    SOURCE_VERSION_STATUS,
    fetchSourceCatalogCategories,
    fetchSourceCatalogItems,
    fetchSourceCatalogApps,
    parseSourcePackage,
    publishSourcePackage,
    registerSourcePlugin,
    registerSourceTemplate,
    updateSourcePlugin,
    updateSourceTemplate,
    saveSourceCatalogCategories,
    pullSourcePlugin,
    pullSourceTemplate,
    setSourcePluginStatus,
    setSourceTemplateStatus,
    fetchSourcePluginVersions,
    fetchSourceTemplateVersions,
    registerSourcePluginVersion,
    registerSourceTemplateVersion,
    setSourcePluginVersionStatus,
    setSourceTemplateVersionStatus,
    type SourceCatalogCategory,
    type SourceCatalogItem,
    type SourceCatalogApp,
    type SourcePackageManifest,
    type SourceVersion
  } from '@/api/source-station'
  import {
    canDeleteCatalogCategory,
    catalogCategoryUsageCount,
    deleteCatalogCategoryConfirmMessage,
    extrasAfterDeletingCategory
  } from '@/utils/form/catalog-category'
  import { catalogUploadBlockReason } from '@/utils/form/catalog-package-source'
  import {
    formatCatalogPriceLabel,
    formatCatalogPriceYuan,
    isHttpsLocation,
    isPaidHttpsImportLocation,
    isPrivatePackageLocation,
    isStationPackageLocation,
    parseCatalogPriceYuan,
    resolveCatalogPriceCents
  } from '@/utils/form/catalog-slug'

  const props = withDefaults(
    defineProps<{
      initialCategory?: string
    }>(),
    { initialCategory: '' }
  )

  const route = useRoute()
  const CATEGORY_APP_STORAGE_KEY = 'source-station-catalog-app-id'
  const categories = ref<SourceCatalogCategory[]>([])
  const apps = ref<SourceCatalogApp[]>([])
  const loading = ref(false)
  const pullingId = ref('')
  const tableData = ref<SourceCatalogItem[]>([])
  const searchForm = reactive({
    appId: 0,
    status: '',
    category: String(route.query.category || props.initialCategory || '')
  })

  const uploadVisible = ref(false)
  const parsing = ref(false)
  const publishing = ref(false)
  const uploadFile = ref<File | null>(null)
  const parsedManifest = ref<SourcePackageManifest | null>(null)
  const uploadForm = reactive({
    appId: 0,
    category: '',
    changelog: '',
    location: '',
    priceYuan: '0',
    push: true
  })
  const uploadBlockReason = computed(() =>
    catalogUploadBlockReason({
      appId: uploadForm.appId,
      push: uploadForm.push,
      hasFile: !!uploadFile.value,
      location: uploadForm.location
    })
  )

  const registerVisible = ref(false)
  const registering = ref(false)
  const registerRef = ref<FormInstance>()
  const registerForm = reactive({
    appId: 0,
    id: '',
    name: '',
    version: '1.0.0',
    description: '',
    location: '',
    sha256: '',
    priceYuan: '0',
    authorName: '',
    changelog: '',
    category: 'other',
    shelf: false
  })
  const editVisible = ref(false)
  const editing = ref(false)
  const editRef = ref<FormInstance>()
  const editingItem = ref<SourceCatalogItem | null>(null)
  const editForm = reactive({
    id: '',
    name: '',
    description: '',
    location: '',
    sha256: '',
    priceYuan: '0',
    authorName: '',
    changelog: '',
    category: '',
    icon: '',
    note: ''
  })
  const editRules: FormRules = {
    category: [{ required: true, message: '请选择分类', trigger: 'change' }],
    name: [{ required: true, message: '请填写名称', trigger: 'blur' }],
    location: [{ required: true, message: '请填写外部地址', trigger: 'blur' }],
    sha256: [optionalPaidShaRule(() => editForm.priceYuan, () => editForm.location)]
  }

  const registerRules: FormRules = {
    appId: [{ required: true, type: 'number', min: 1, message: '请选择应用', trigger: 'change' }],
    category: [{ required: true, message: '请选择分类', trigger: 'change' }],
    id: [{ required: true, message: '请填写标识', trigger: 'blur' }],
    name: [{ required: true, message: '请填写名称', trigger: 'blur' }],
    version: [{ required: true, message: '请填写版本', trigger: 'blur' }],
    location: [{ required: true, message: '请填写外部地址', trigger: 'blur' }],
    sha256: [optionalPaidShaRule(() => registerForm.priceYuan, () => registerForm.location)]
  }

  function optionalPaidShaRule(yuan: () => string, location: () => string) {
    return {
      validator: (_: unknown, value: string, callback: (error?: Error) => void) => {
        const raw = String(value || '').trim()
        const cents = parseCatalogPriceYuan(yuan())
        if (!raw && cents !== null && isPaidHttpsImportLocation(location(), cents)) {
          callback()
          return
        }
        if (!/^[a-fA-F0-9]{64}$/.test(raw)) {
          callback(new Error(raw ? '校验码须为 64 位十六进制' : '请填写校验码'))
          return
        }
        callback()
      },
      trigger: 'blur' as const
    }
  }

  const categoryVisible = ref(false)
  const savingCategories = ref(false)
  const extraForm = reactive({ key: '', label: '', kind: 'plugin' as 'plugin' | 'template' })

  const versionVisible = ref(false)
  const versionLoading = ref(false)
  const versionSaving = ref(false)
  const versionFormVisible = ref(false)
  const versions = ref<SourceVersion[]>([])
  const currentItem = ref<SourceCatalogItem | null>(null)
  const versionForm = reactive({ version: '', location: '', sha256: '', changelog: '' })

  const registerIsTemplate = computed(
    () => categoryKind(registerForm.category) === 'template'
  )
  const editIsTemplate = computed(
    () => editingItem.value?.kind === 'template' || categoryKind(editForm.category) === 'template'
  )
  const editCategoryOptions = computed(() => {
    const kind = editingItem.value?.kind || categoryKind(editForm.category)
    return categories.value.filter((item) => item.kind === kind)
  })
  const editApp = computed(
    () => apps.value.find((item) => item.id === editingItem.value?.appId) || apps.value[0]
  )
  const currentIsTemplate = computed(() => currentItem.value?.kind === 'template')
  const uploadCategoryOptions = computed(() => {
    const kind = parsedManifest.value?.kind
    if (!kind) return categories.value
    return categories.value.filter((item) => item.kind === kind)
  })
  function appLabel(app: SourceCatalogApp) {
    return app.enabled ? `${app.name}（${app.appKey}）` : `${app.name}（${app.appKey}）· 已停用`
  }

  function categoryKind(key: string): 'plugin' | 'template' {
    return categories.value.find((item) => item.key === key)?.kind || 'plugin'
  }

  function categoryLabel(key?: string) {
    if (!key) return '-'
    return categories.value.find((item) => item.key === key)?.label || key
  }

  function itemLocation(row: SourceCatalogItem) {
    return row.location || row.downloadUrl || row.templateUrl || ''
  }

  function isTemplateItem(row: SourceCatalogItem) {
    return row.kind === 'template' || categoryKind(row.category) === 'template'
  }

  function canVersionAction(
    status: string,
    action: 'approve' | 'reject' | 'latest' | 'deprecate'
  ): boolean {
    switch (action) {
      case 'approve':
        return status === 'pending' || status === 'draft'
      case 'reject':
        return status === 'pending'
      case 'latest':
        return status === 'published'
      case 'deprecate':
        return status === 'published'
      default:
        return false
    }
  }

  function canEditItem(status: string) {
    return ['draft', 'review', 'approved', 'published', 'hidden'].includes(status)
  }

  function canAction(
    status: string,
    action: 'approve' | 'reject' | 'shelf' | 'unshelf' | 'deprecate'
  ): boolean {
    const target = {
      approve: 'approved',
      reject: 'rejected',
      shelf: 'published',
      unshelf: 'hidden',
      deprecate: 'deprecated'
    } as const
    const to = target[action]
    if (status === to) return false
    switch (to) {
      case 'approved':
      case 'rejected':
        return status === 'review' || status === 'draft'
      case 'published':
        return status === 'approved' || status === 'hidden'
      case 'hidden':
        return status === 'published'
      case 'deprecated':
        return status === 'published' || status === 'hidden' || status === 'approved'
      default:
        return false
    }
  }

  function statusMeta(status: string) {
    return SOURCE_ITEM_STATUS[status] || { label: status, type: 'info' as const }
  }

  function versionStatusMeta(status: string) {
    return SOURCE_VERSION_STATUS[status] || { label: status, type: 'info' as const }
  }

  async function loadCategories() {
    const data = await fetchSourceCatalogCategories()
    categories.value = data.list || []
  }

  async function loadApps() {
    const data = await fetchSourceCatalogApps()
    apps.value = data.list || []
    const stored = Number(localStorage.getItem(CATEGORY_APP_STORAGE_KEY) || 0)
    const exists = apps.value.some((item) => item.id === stored)
    if (exists) {
      searchForm.appId = stored
      return
    }
    searchForm.appId = apps.value[0]?.id || 0
    if (searchForm.appId) {
      localStorage.setItem(CATEGORY_APP_STORAGE_KEY, String(searchForm.appId))
    }
  }

  function onAppChange() {
    if (searchForm.appId) {
      localStorage.setItem(CATEGORY_APP_STORAGE_KEY, String(searchForm.appId))
    }
    loadItems()
  }

  async function handlePull(row: SourceCatalogItem) {
    if (!row.originUrl || pullingId.value) return
    pullingId.value = row.id
    try {
      if (isTemplateItem(row)) await pullSourceTemplate(row.id)
      else await pullSourcePlugin(row.id)
      ElMessage.success('已重新拉取并更新托管包')
      await loadItems()
    } finally {
      pullingId.value = ''
    }
  }

  async function loadItems() {
    if (!searchForm.appId) {
      tableData.value = []
      return
    }
    loading.value = true
    try {
      const data = await fetchSourceCatalogItems(
        searchForm.status,
        searchForm.category,
        searchForm.appId
      )
      tableData.value = data.list || []
    } finally {
      loading.value = false
    }
  }

  function resetSearch() {
    searchForm.status = ''
    searchForm.category = ''
    loadItems()
  }

  function openUpload() {
    if (!searchForm.appId) {
      ElMessage.warning('请先选择应用')
      return
    }
    uploadFile.value = null
    parsedManifest.value = null
    uploadForm.appId = searchForm.appId
    uploadForm.category = searchForm.category
    uploadForm.changelog = ''
    uploadForm.location = ''
    uploadForm.priceYuan = '0'
    uploadForm.push = true
    uploadVisible.value = true
  }

  function selectUploadFile(file: UploadFile) {
    uploadFile.value = file.raw || null
    parsedManifest.value = null
  }

  function buildPackageForm() {
    const form = new FormData()
    if (uploadFile.value) form.append('file', uploadFile.value)
    if (uploadForm.category) form.append('category', uploadForm.category)
    if (uploadForm.changelog) form.append('changelog', uploadForm.changelog)
    if (!uploadForm.push && uploadForm.location.trim()) {
      const kind = parsedManifest.value?.kind || categoryKind(uploadForm.category)
      form.append(kind === 'template' ? 'templateUrl' : 'downloadUrl', uploadForm.location.trim())
    }
    const priced = resolveCatalogPriceCents(
      uploadForm.priceYuan,
      uploadForm.push ? '' : uploadForm.location
    )
    if (!priced.error && priced.cents > 0) form.append('priceCents', String(priced.cents))
    if (uploadForm.push) form.append('push', 'true')
    if (uploadForm.appId) form.append('appId', String(uploadForm.appId))
    return form
  }

  async function handleParse() {
    if (uploadBlockReason.value) {
      ElMessage.warning(uploadBlockReason.value)
      return
    }
    parsing.value = true
    try {
      const form = new FormData()
      if (uploadFile.value) form.append('file', uploadFile.value)
      else {
        const kind = categoryKind(uploadForm.category)
        form.append(kind === 'template' ? 'templateUrl' : 'downloadUrl', uploadForm.location.trim())
      }
      if (uploadForm.category) form.append('category', uploadForm.category)
      parsedManifest.value = await parseSourcePackage(form)
      if (parsedManifest.value.category) {
        uploadForm.category = parsedManifest.value.category
      }
      ElMessage.success('已通过校验（包未落盘、未入库）')
    } finally {
      parsing.value = false
    }
  }

  async function handlePublish() {
    if (uploadBlockReason.value) {
      ElMessage.warning(uploadBlockReason.value)
      return
    }
    const priced = resolveCatalogPriceCents(
      uploadForm.priceYuan,
      uploadForm.push ? '' : uploadForm.location
    )
    if (priced.error) {
      ElMessage.warning(priced.error)
      return
    }
    publishing.value = true
    try {
      const result = await publishSourcePackage(buildPackageForm())
      const origin = (result.item as { originUrl?: string } | undefined)?.originUrl
      ElMessage.success(
        result.pushed
          ? '校验通过，已推送 Release 并保存元数据'
          : origin
            ? '校验通过，已拉取外链并私有托管，校验码已自动填写'
            : uploadFile.value
              ? '校验通过，已保存元数据（包已丢弃）'
              : '校验通过，已保存元数据并自动填写校验码'
      )
      uploadVisible.value = false
      await loadItems()
    } finally {
      publishing.value = false
    }
  }

  function openEdit(row: SourceCatalogItem) {
    editingItem.value = row
    editForm.id = row.id
    editForm.name = row.name
    editForm.description = row.description || ''
    editForm.location = itemLocation(row)
    editForm.sha256 = row.sha256 || ''
    editForm.priceYuan = formatCatalogPriceYuan(row.priceCents)
    editForm.authorName = row.author?.name || ''
    editForm.changelog = row.changelog || ''
    editForm.category = row.category
    editForm.icon = row.icon || ''
    editForm.note = ''
    editVisible.value = true
  }

  async function handleEdit() {
    if (!editingItem.value) return
    await editRef.value?.validate()
    const priced = resolveCatalogPriceCents(editForm.priceYuan, editForm.location)
    if (priced.error) {
      ElMessage.warning(priced.error)
      return
    }
    editing.value = true
    try {
      const note = editForm.note.trim() || '管理员编辑目录元数据（保持原状态）'
      if (editIsTemplate.value) {
        await updateSourceTemplate(editingItem.value.id, {
          id: editingItem.value.id,
          appId: editingItem.value.appId,
          templateKey: editingItem.value.templateKey || editingItem.value.id,
          name: editForm.name,
          description: editForm.description,
          version: editingItem.value.version || '1.0.0',
          schemaVersion: editingItem.value.schemaVersion || 1,
          templateUrl: editForm.location,
          sha256: editForm.sha256,
          priceCents: priced.cents,
          changelog: editForm.changelog,
          category: editForm.category,
          author: { name: editForm.authorName },
          note
        })
      } else {
        await updateSourcePlugin(editingItem.value.id, {
          id: editingItem.value.id,
          appId: editingItem.value.appId,
          name: editForm.name,
          description: editForm.description,
          version: editingItem.value.version || '1.0.0',
          downloadUrl: editForm.location,
          sha256: editForm.sha256,
          priceCents: priced.cents,
          changelog: editForm.changelog,
          category: editForm.category,
          icon: editForm.icon,
          author: { name: editForm.authorName },
          note
        })
      }
      ElMessage.success('已更新目录元数据')
      editVisible.value = false
      await loadItems()
    } finally {
      editing.value = false
    }
  }

  function openRegister() {
    if (!searchForm.appId) {
      ElMessage.warning('请先选择应用')
      return
    }
    registerForm.appId = searchForm.appId
    registerForm.id = ''
    registerForm.name = ''
    registerForm.version = '1.0.0'
    registerForm.description = ''
    registerForm.location = ''
    registerForm.sha256 = ''
    registerForm.priceYuan = '0'
    registerForm.authorName = ''
    registerForm.changelog = ''
    registerForm.category = searchForm.category || 'other'
    registerForm.shelf = false
    registerVisible.value = true
  }

  async function handleRegister() {
    await registerRef.value?.validate()
    const priced = resolveCatalogPriceCents(registerForm.priceYuan, registerForm.location)
    if (priced.error) {
      ElMessage.warning(priced.error)
      return
    }
    registering.value = true
    try {
      if (registerIsTemplate.value) {
        await registerSourceTemplate({
          id: registerForm.id,
          appId: registerForm.appId,
          templateKey: registerForm.id,
          name: registerForm.name,
          version: registerForm.version,
          description: registerForm.description,
          templateUrl: registerForm.location,
          sha256: registerForm.sha256,
          priceCents: priced.cents,
          changelog: registerForm.changelog,
          schemaVersion: 1,
          category: registerForm.category,
          author: { name: registerForm.authorName },
          shelf: registerForm.shelf
        })
      } else {
        await registerSourcePlugin({
          id: registerForm.id,
          appId: registerForm.appId,
          name: registerForm.name,
          version: registerForm.version,
          description: registerForm.description,
          downloadUrl: registerForm.location,
          sha256: registerForm.sha256,
          priceCents: priced.cents,
          changelog: registerForm.changelog,
          category: registerForm.category,
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

  function openCategoryManager() {
    extraForm.key = ''
    extraForm.label = ''
    extraForm.kind = 'plugin'
    categoryVisible.value = true
  }

  async function handleAddCategory() {
    const key = extraForm.key.trim().toLowerCase()
    const label = extraForm.label.trim()
    if (!key || !label) {
      ElMessage.warning('请填写分类标识和名称')
      return
    }
    savingCategories.value = true
    try {
      const extras = extrasAfterDeletingCategory(categories.value, '')
      extras.push({ key, label, kind: extraForm.kind })
      const data = await saveSourceCatalogCategories(extras)
      categories.value = data.list || []
      extraForm.key = ''
      extraForm.label = ''
      ElMessage.success('已添加分类')
    } finally {
      savingCategories.value = false
    }
  }

  async function handleDeleteCategory(row: SourceCatalogCategory) {
    if (!canDeleteCatalogCategory(row)) return
    let usedCount = catalogCategoryUsageCount(tableData.value, row.key)
    if (usedCount === 0 && searchForm.appId) {
      const data = await fetchSourceCatalogItems('', '', searchForm.appId)
      usedCount = catalogCategoryUsageCount(data.list || [], row.key)
    }
    try {
      await ElMessageBox.confirm(
        deleteCatalogCategoryConfirmMessage(row.label, usedCount),
        '删除分类',
        { type: 'warning' }
      )
    } catch {
      return
    }
    savingCategories.value = true
    try {
      const extras = extrasAfterDeletingCategory(categories.value, row.key)
      const data = await saveSourceCatalogCategories(extras)
      categories.value = data.list || []
      if (searchForm.category === row.key) {
        searchForm.category = ''
        await loadItems()
      }
      ElMessage.success('已删除分类')
    } finally {
      savingCategories.value = false
    }
  }

  async function runStatus(
    row: SourceCatalogItem,
    action: 'approve' | 'reject' | 'shelf' | 'unshelf' | 'deprecate'
  ) {
    if (action === 'unshelf') {
      await ElMessageBox.confirm(
        '下架后，该应用的公开软件源里不再显示这一条，已经安装的不会被远程卸掉。确认继续？',
        '下架确认',
        { type: 'warning' }
      )
    }
    let note = ''
    if (action === 'reject' || action === 'deprecate') {
      const { value } = await ElMessageBox.prompt(
        '备注（可选）',
        action === 'reject' ? '驳回' : '弃用',
        {
          inputPlaceholder: '审核说明',
          confirmButtonText: '确定',
          cancelButtonText: '取消'
        }
      )
      note = value || ''
    }
    if (isTemplateItem(row)) {
      await setSourceTemplateStatus(row.id, action, note)
    } else {
      await setSourcePluginStatus(row.id, action, note)
    }
    ElMessage.success('已更新状态')
    await loadItems()
  }

  async function openVersions(row: SourceCatalogItem) {
    currentItem.value = row
    versionVisible.value = true
    await loadVersions()
  }

  async function loadVersions() {
    if (!currentItem.value) return
    versionLoading.value = true
    try {
      const data = isTemplateItem(currentItem.value)
        ? await fetchSourceTemplateVersions(currentItem.value.id)
        : await fetchSourcePluginVersions(currentItem.value.id)
      versions.value = data.list || []
    } finally {
      versionLoading.value = false
    }
  }

  async function handleAddVersion() {
    if (!currentItem.value) return
    const cents = currentItem.value.priceCents || 0
    const location = versionForm.location.trim()
    if (!versionForm.version.trim() || !location) {
      ElMessage.warning('请填写版本和地址')
      return
    }
    if (
      cents > 0 &&
      !isStationPackageLocation(location) &&
      !isPrivatePackageLocation(location) &&
      !isHttpsLocation(location)
    ) {
      ElMessage.warning('付费条目请上传压缩包，或填写 https 网址由本站拉取托管')
      return
    }
    if (!isPaidHttpsImportLocation(location, cents) && !/^[a-fA-F0-9]{64}$/.test(versionForm.sha256.trim())) {
      ElMessage.warning('请填写 64 位校验码')
      return
    }
    versionSaving.value = true
    try {
      const payload = {
        version: versionForm.version,
        sha256: versionForm.sha256,
        changelog: versionForm.changelog,
        ...(isTemplateItem(currentItem.value)
          ? { templateUrl: versionForm.location }
          : { downloadUrl: versionForm.location })
      }
      if (isTemplateItem(currentItem.value)) {
        await registerSourceTemplateVersion(currentItem.value.id, payload)
      } else {
        await registerSourcePluginVersion(currentItem.value.id, payload)
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
    if (isTemplateItem(currentItem.value)) {
      await setSourceTemplateVersionStatus(currentItem.value.id, row.version, action)
    } else {
      await setSourcePluginVersionStatus(currentItem.value.id, row.version, action)
    }
    ElMessage.success('已更新版本')
    await loadVersions()
    await loadItems()
  }

  onMounted(async () => {
    await loadCategories()
    await loadApps()
    await loadItems()
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
</style>
