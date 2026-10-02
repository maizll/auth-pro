<!-- 应用管理页面 -->
<!-- art-full-height 自动计算出页面剩余高度 -->
<!-- art-table-card 一个符合系统样式的 class，同时自动撑满剩余高度 -->
<template>
  <div class="license-apps-page art-full-height">
    <ElCard class="art-table-card no-search-card" shadow="never">
      <!-- 版本状态只在顶栏。这里只在「新增应用」旁留一行说明，超限再点新增才弹升级窗。 -->
      <ArtTableHeader v-model:columns="columnChecks" :loading="loading" @refresh="refreshData">
        <template #left>
          <ElSpace wrap>
            <ElButton @click="handleAdd" v-ripple>新增应用</ElButton>
            <CommercialMark v-if="showCommercialHint" :text="multiAppText" />
          </ElSpace>
        </template>
      </ArtTableHeader>

      <!-- 表格 -->
      <ArtTable :loading="loading" :data="data" :columns="columns">
        <template #name="{ row }">
          <span class="app-name-cell__title">{{ row.name }}</span>
        </template>
        <template #repo="{ row }">
          <span class="app-repo-cell">{{ row.repo || '未绑定仓库' }}</span>
        </template>
        <!-- 授权方式 -->
        <template #purchaseLicenseTypes="{ row }">
          <div v-if="row.purchaseLicenseTypes?.length" class="license-type-tags">
            <ElTag
              v-for="licenseType in orderedPurchaseLicenseTypes(row.purchaseLicenseTypes)"
              :key="licenseType"
              :type="purchaseLicenseTypeMeta[licenseType]?.tagType"
              size="small"
              effect="plain"
            >
              {{ purchaseLicenseTypeMeta[licenseType]?.label || licenseType }}
            </ElTag>
          </div>
          <ElTag v-else type="info" size="small">已关闭购买</ElTag>
        </template>

        <!-- AppSecret -->
        <template #appSecret="{ row }">
          <BizCopySecret
            :value="row.appSecret"
            success-text="已复制"
            fail-text="复制失败，请手动复制"
          />
        </template>

        <!-- 版本 -->
        <template #version="{ row }">
          <ElButton link type="primary" @click="handleVersions(row)">
            {{ row.recentVersion || '未发布' }}
          </ElButton>
          <span v-if="row.versionCount" class="version-count">{{ row.versionCount }}</span>
        </template>

        <template #sale="{ row }">
          <div v-if="row.commercial && row.commercial.mode !== 'off'" class="sale-status">
            <ElTag v-if="row.commercial.mode === 'stopped'" type="info" size="small">已停售</ElTag>
            <ElTag v-else-if="!row.commercial.saleGaps?.length" type="primary" size="small">
              商业版 · 出售中
            </ElTag>
            <ElButton
              v-for="gap in row.commercial.mode === 'selling' ? row.commercial.saleGaps || [] : []"
              :key="gap.code"
              link
              type="danger"
              @click="handleSaleGap(gap)"
            >
              {{ gap.label }}
            </ElButton>
            <ElTag v-if="row.commercial.legacyDefault" type="success" size="small" effect="plain">
              接收老客户端
            </ElTag>
          </div>
          <span v-else class="text-secondary">--</span>
        </template>

        <!-- 状态 -->
        <template #enabled="{ row }">
          <ElTag :type="row.archived ? 'info' : row.enabled ? 'success' : 'info'" size="small">
            {{ row.archived ? '已归档' : row.enabled ? '启用' : '禁用' }}
          </ElTag>
        </template>

        <!-- 授权校验 -->
        <template #licenseRequired="{ row }">
          <ElSwitch
            v-model="row.licenseRequired"
            :loading="row.licenseRequiredChanging"
            :before-change="() => handleLicenseRequiredChange(row)"
          />
        </template>

        <!-- 操作 -->
        <template #operation="{ row }">
          <RowActions
            :primary="appPrimaryActions"
            :more="appMoreActions(row)"
            @click="(action) => onAppAction(row, action)"
          />
        </template>
      </ArtTable>
    </ElCard>

    <!-- 新增/编辑弹窗。创建时第 2 步才选仓库，编辑不出现这一步。 -->
    <AppDialog
      v-model="dialogVisible"
      :title="dialogTitle"
      size="lg"
      flow="long"
      destroy-on-close
      :before-close="beforeCloseCreate"
    >
      <ElSteps v-if="!isEdit && !createResult" :active="step" align-center class="create-steps">
        <ElStep title="应用信息" />
        <ElStep title="仓库" />
      </ElSteps>
      <ElResult
        v-if="createResult"
        :icon="createResult.ok ? 'success' : 'info'"
        :title="createResult.title"
        :sub-title="createResult.sub"
      />
      <ElForm
        v-else-if="isEdit || step === 0"
        :model="formData"
        :rules="formRules"
        ref="formRef"
        :label-width="narrow ? undefined : '96px'"
        :label-position="narrow ? 'top' : 'right'"
      >
        <ElFormItem label="应用名称" prop="name">
          <ElInput v-model="formData.name" placeholder="请输入应用名称" />
        </ElFormItem>
        <ElFormItem label="授权方式">
          <ElCheckboxGroup v-model="formData.purchaseLicenseTypes" class="license-type-options">
            <ElCheckbox
              v-for="licenseType in purchaseLicenseTypeOrder"
              :key="licenseType"
              :value="licenseType"
            >
              {{ purchaseLicenseTypeMeta[licenseType].label }}
            </ElCheckbox>
          </ElCheckboxGroup>
          <div class="form-tip">全部取消后，用户端和代理端将不再显示该应用。</div>
        </ElFormItem>
        <ElFormItem label="回调地址">
          <ElInput v-model="formData.callbackUrl" placeholder="授权验证回调URL（可选）" />
        </ElFormItem>
        <ElFormItem label="状态">
          <ElSwitch v-model="formData.enabled" active-text="启用" inactive-text="禁用" />
        </ElFormItem>
        <AppCommercialSection
          v-if="commercialContext.managed"
          v-model:form="commercialForm"
          :saved-mode="editingRow?.commercial?.mode || 'off'"
          :app-id="formData.id"
          :app-key="editingRow?.appKey || ''"
          :plans="editingRow?.commercial?.plans || []"
          :stats="editingRow?.commercial?.stats"
          :gaps="editingRow?.commercial?.saleGaps || []"
          :legacy-owner="legacyOwnerName"
          @close="openCloseDialog"
          @plans="goPlans"
          @gap="handleSaleGap"
        />
        <ElFormItem label="备注">
          <ElInput v-model="formData.remark" type="textarea" :rows="2" placeholder="可选" />
        </ElFormItem>
      </ElForm>
      <RepoOptionCards
        v-else
        v-model="repoAction"
        v-model:repo="repoName"
        :token-ready="tokenReady"
        :token-message="tokenMessage"
        :hint="repoHint"
        :options="repoOptions"
        @touched="repoTouched = true"
      />
      <template #footer>
        <span v-if="editDirty" class="unsaved-tip">有未保存的修改</span>
        <template v-if="createResult">
          <ElButton @click="finishCreate">稍后处理</ElButton>
          <ElButton v-if="!createResult.ok" type="primary" @click="retryBind">重试绑定</ElButton>
        </template>
        <template v-else-if="!isEdit && step === 0">
          <ElButton @click="requestCloseCreate">取消</ElButton>
          <ElButton type="primary" @click="goRepoStep">下一步</ElButton>
        </template>
        <template v-else-if="!isEdit">
          <ElButton :disabled="saving" @click="step = 0">上一步</ElButton>
          <ElButton type="primary" :loading="saving" @click="handleSubmit">{{
            saving ? '正在创建…' : '创建'
          }}</ElButton>
        </template>
        <template v-else>
          <ElButton @click="requestCloseCreate">取消</ElButton>
          <ElButton type="primary" :loading="saving" @click="handleSubmit">保存</ElButton>
        </template>
      </template>
    </AppDialog>

    <AppCommercialCloseDialog
      v-if="closingRow?.commercial"
      v-model="closeVisible"
      :app-id="closingRow.id"
      :app-name="closingRow.name"
      :stats="closingRow.commercial.stats"
      :is-super="commercialContext.super"
      :legacy-default="closingRow.commercial.legacyDefault"
      :legacy-targets="legacyTargets"
      @done="afterClose"
    />

    <AppDialog v-model="migrateVisible" title="归档应用" size="md" flow="short">
      <p v-if="migrateCount > 0">
        应用「{{ migrateSource?.name }}」下还有
        {{ migrateCount }}
        条软件目录条目。可以迁到另一个应用再归档，也可以直接归档，条目仍挂在这个应用上。授权、套餐和版本都会保留。
      </p>
      <p v-else>
        归档应用「{{
          migrateSource?.name
        }}」后，授权记录和版本都会保留，只是不能再往这个应用登记新的目录条目。
      </p>
      <ElSelect
        v-if="migrateCount > 0 && migrateTargets.length"
        v-model="migrateAppId"
        placeholder="目录条目迁到哪个应用"
        style="width: 100%; margin-top: 12px"
      >
        <ElOption v-for="app in migrateTargets" :key="app.id" :label="app.name" :value="app.id" />
      </ElSelect>
      <p v-else-if="migrateCount > 0">没有其他应用可以接收这些目录条目，请先新建应用。</p>
      <ElCheckbox v-model="redirectSource" style="margin-top: 14px">
        把该应用的软件源地址转到另一个应用
      </ElCheckbox>
      <p class="form-tip">
        勾选后，旧地址 /software-source/{{ migrateSource?.appKey }}/index.json
        会继续返回目标应用的目录，已经订阅这条地址的客户端不用改。
      </p>
      <ElSelect
        v-if="redirectSource"
        v-model="redirectAppId"
        placeholder="软件源地址转到哪个应用"
        style="width: 100%; margin-top: 8px"
      >
        <ElOption
          v-for="app in migrateTargets.filter((item) => !item.archived)"
          :key="app.id"
          :label="app.name"
          :value="app.id"
        />
      </ElSelect>
      <template #footer>
        <ElButton @click="migrateVisible = false">取消</ElButton>
        <ElButton :loading="archiveSaving" @click="confirmArchiveInPlace">直接归档</ElButton>
        <ElButton
          v-if="migrateCount > 0"
          type="danger"
          :loading="migrateSaving"
          :disabled="!migrateTargets.length"
          @click="confirmMigrateAndDelete"
        >
          迁移并归档
        </ElButton>
      </template>
    </AppDialog>
  </div>
</template>

<script setup lang="ts">
  import { useRouter } from 'vue-router'
  import { ElMessage } from 'element-plus'
  import AppDialog from '@/components/core/dialog/AppDialog.vue'
  import { appConfirm } from '@/utils/app-confirm'
  import { fetchAppRepoToken, suggestAppRepo } from '@/api/app-repo'
  import RepoOptionCards from '@/components/business/repo/RepoOptionCards.vue'
  import CommercialMark from '@/components/business/commercial/CommercialMark.vue'
  import AppCommercialSection, {
    type AppCommercialForm
  } from '@/components/business/commercial/AppCommercialSection.vue'
  import AppCommercialCloseDialog from '@/components/business/commercial/AppCommercialCloseDialog.vue'
  import { fetchStoreAccount, type StoreAccount } from '@/api/store'
  import {
    commercialCopy,
    isCommercialActive,
    openCommercialPrompt,
    rememberCommercialAccount
  } from '@/utils/commercial'
  import RowActions, { type RowActionItem } from '@/components/business/row-actions/index.vue'
  import { useNarrowScreen } from '@/hooks/core/useNarrowScreen'
  import { useTable } from '@/hooks/core/useTable'
  import {
    fetchLicenseAppList,
    fetchCreateLicenseApp,
    fetchUpdateLicenseApp,
    fetchDeleteLicenseApp,
    fetchRestoreLicenseApp,
    fetchResetAppSecret,
    fetchUpdateAppLicenseRequired,
    fetchEnsureStoreSnapshotKey,
    fetchAppCommercialContext,
    type AppCommercialInput,
    type AppSaleGap,
    type LicenseAppItem
  } from '@/api/license-manage'
  import { fetchCatalogAppUsage } from '@/api/source-station'

  defineOptions({ name: 'LicenseApps' })

  /** 行级本地状态 */
  type AppRow = LicenseAppItem & {
    licenseRequiredChanging: boolean
  }

  const router = useRouter()

  // 弹窗相关
  const dialogVisible = ref(false)
  const isEdit = ref(false)
  const step = ref(0)
  const saving = ref(false)
  const tokenReady = ref(false)
  const tokenMessage = ref('还没有可用的令牌。请到存储管理添加。也可以先创建应用，稍后再绑定。')
  const repoAction = ref<'create' | 'bind' | 'skip'>('skip')
  const repoName = ref('')
  const repoTouched = ref(false)
  const repoHint = ref('')
  const repoOptions = [
    { value: 'create', title: '自动创建私有仓库', field: '仓库名', placeholder: '所有者/仓库' },
    { value: 'bind', title: '绑定已有仓库', field: '所有者/仓库', placeholder: '所有者/仓库' },
    { value: 'skip', title: '稍后绑定', needsToken: false }
  ]
  const requestId = ref('')
  const createResult = ref<{ ok: boolean; title: string; sub: string; appId: number } | null>(null)
  const dialogTitle = computed(() => {
    if (createResult.value) return '创建结果'
    return isEdit.value ? '编辑应用' : '新增应用'
  })

  const purchaseLicenseTypeOrder = ['domain', 'wildcard', 'ip', 'key'] as const
  const purchaseLicenseTypeMeta: Record<
    string,
    { label: string; tagType: 'primary' | 'success' | 'warning' | 'info' }
  > = {
    domain: { label: '单域名', tagType: 'primary' },
    wildcard: { label: '泛域名', tagType: 'success' },
    ip: { label: 'IP', tagType: 'info' },
    key: { label: '密钥', tagType: 'info' }
  }

  const formRef = ref()
  const formData = reactive({
    id: 0,
    name: '',
    callbackUrl: '',
    enabled: true,
    remark: '',
    purchaseLicenseTypes: [...purchaseLicenseTypeOrder] as string[]
  })

  // 商业版只在官网出售。客户站拿到 managed=false，整节隐藏。
  const commercialContext = reactive({ managed: false, super: false })
  const defaultCommercialForm = (): AppCommercialForm => ({
    mode: 'off',
    legacyDefault: false,
    graceDays: 7,
    revokeOnPasswordChange: true,
    features: 'multi_app'
  })
  const commercialForm = ref<AppCommercialForm>(defaultCommercialForm())
  const editingRow = ref<AppRow | null>(null)
  // 打开弹框时的快照，用来提示未保存的修改。
  const openedSnapshot = ref('')
  const formSnapshot = () => JSON.stringify([formData, commercialForm.value])

  const formRules = {
    name: [{ required: true, message: '请输入应用名称', trigger: 'blur' }]
  }

  const { columns, columnChecks, data, loading, refreshData, refreshRemove, toggleColumn } =
    useTable({
      // 核心配置
      core: {
        apiFn: fetchLicenseAppList,
        apiParams: {},
        columnsFactory: () => [
          { type: 'index', width: 60, label: '序号', mobileHidden: true },
          {
            prop: 'name',
            label: '应用名称',
            mobileLabel: '应用',
            minWidth: 150,
            useSlot: true,
            showOverflowTooltip: true,
            mobilePriority: 1
          },
          {
            prop: 'repo',
            label: '仓库',
            minWidth: 180,
            useSlot: true,
            showOverflowTooltip: true,
            mobileHidden: true
          },
          { prop: 'sale', label: '商业版', minWidth: 220, useSlot: true, mobileHidden: true },
          {
            prop: 'appKey',
            label: 'AppKey',
            minWidth: 220,
            showOverflowTooltip: true,
            mobileHidden: true
          },
          {
            prop: 'purchaseLicenseTypes',
            label: '授权方式',
            minWidth: 250,
            useSlot: true,
            mobileHidden: true
          },
          {
            prop: 'appSecret',
            label: 'AppSecret',
            minWidth: 220,
            useSlot: true,
            mobileHidden: true
          },
          { prop: 'licenseCount', label: '授权数', width: 90, align: 'center', mobileHidden: true },
          { prop: 'version', label: '版本', minWidth: 120, useSlot: true, mobileHidden: true },
          {
            prop: 'enabled',
            label: '状态',
            width: 90,
            align: 'center',
            useSlot: true,
            mobilePriority: 2,
            mobileWidth: 72
          },
          {
            prop: 'licenseRequired',
            label: '授权校验',
            width: 100,
            align: 'center',
            useSlot: true,
            mobileHidden: true
          },
          { prop: 'createdAt', label: '创建时间', width: 160, mobileHidden: true },
          {
            prop: 'operation',
            label: '操作',
            width: 168,
            useSlot: true,
            mobilePriority: 3,
            mobileWidth: 104
          }
        ]
      },
      // 数据处理
      transform: {
        dataTransformer: (records) => {
          if (!Array.isArray(records)) {
            return []
          }
          // 附加行级本地状态：密钥可见性、授权校验切换中
          const normalized = (records as unknown as LicenseAppItem[]).map((item) => ({
            ...item,
            licenseRequired: item.licenseRequired !== false,
            licenseRequiredChanging: false
          }))
          return normalized as unknown as typeof records
        }
      }
    })

  const narrow = useNarrowScreen()

  // 宽屏留「编辑」「版本」两个常用按钮。窄屏只留「编辑」，「版本」收进「更多」。
  const appPrimaryActions = computed(() =>
    narrow.value
      ? [{ key: 'edit', label: '编辑' }]
      : [
          { key: 'edit', label: '编辑' },
          { key: 'versions', label: '版本' }
        ]
  )

  function appMoreActions(row: AppRow): RowActionItem[] {
    return [
      ...(narrow.value ? [{ key: 'versions', label: '版本' }] : []),
      ...(!row.repo && !row.archived ? [{ key: 'bind', label: '绑定仓库' }] : []),
      { key: 'sdk', label: 'SDK 包' },
      { key: 'secret', label: '重置密钥', danger: true },
      row.archived
        ? { key: 'restore', label: '恢复' }
        : { key: 'archive', label: '归档', danger: true }
    ]
  }

  function onAppAction(row: AppRow, action: RowActionItem) {
    switch (action.key) {
      case 'edit':
        handleEdit(row)
        break
      case 'bind':
        router.push('/source-station/repos')
        break
      case 'versions':
        handleVersions(row)
        break
      case 'sdk':
        handleSDKPack(row)
        break
      case 'secret':
        handleResetSecret(row)
        break
      case 'archive':
        handleDelete(row)
        break
      case 'restore':
        handleRestore(row)
        break
    }
  }

  const orderedPurchaseLicenseTypes = (types: string[] = []) => {
    return purchaseLicenseTypeOrder.filter((licenseType) => types.includes(licenseType))
  }

  const storeAccount = ref<StoreAccount | null>(null)
  const multiAppText = commercialCopy.multi_app
  // 官网自己出售商业版，应用数不受限，不显示升级提示。
  const showCommercialHint = computed(
    () => !commercialContext.managed && !isCommercialActive(storeAccount.value)
  )
  const showAppLimitBar = computed(() => showCommercialHint.value && (data.value?.length || 0) >= 1)

  async function loadCommercialAccount() {
    try {
      storeAccount.value = await fetchStoreAccount()
    } catch {
      storeAccount.value = null
    }
    rememberCommercialAccount(storeAccount.value)
  }

  async function loadCommercialContext() {
    try {
      const ctx = await fetchAppCommercialContext()
      commercialContext.managed = !!ctx?.managed
      commercialContext.super = !!ctx?.super
    } catch {
      commercialContext.managed = false
    }
    toggleColumn?.('sale', commercialContext.managed)
  }

  onMounted(() => {
    loadCommercialContext()
    loadCommercialAccount()
    window.addEventListener('store-account-refresh', loadCommercialAccount)
  })
  onBeforeUnmount(() => window.removeEventListener('store-account-refresh', loadCommercialAccount))

  const handleAdd = () => {
    if (showAppLimitBar.value) {
      openCommercialPrompt(multiAppText, 'multi_app')
      return
    }
    isEdit.value = false
    formData.id = 0
    formData.name = ''
    formData.callbackUrl = ''
    formData.enabled = true
    formData.remark = ''
    formData.purchaseLicenseTypes = [...purchaseLicenseTypeOrder]
    editingRow.value = null
    commercialForm.value = defaultCommercialForm()
    step.value = 0
    createResult.value = null
    repoTouched.value = false
    repoName.value = ''
    repoHint.value = ''
    requestId.value = crypto.randomUUID()
    void loadTokenState()
    dialogVisible.value = true
  }

  const fillCommercialForm = (row: AppRow) => {
    const view = row.commercial
    commercialForm.value = {
      mode: view?.mode || 'off',
      legacyDefault: !!view?.legacyDefault,
      graceDays: view?.graceDays || 7,
      revokeOnPasswordChange: view?.revokeOnPasswordChange !== false,
      features: (view?.features?.length ? view.features : ['multi_app']).join(',')
    }
  }

  const handleEdit = (row: AppRow) => {
    isEdit.value = true
    formData.id = row.id
    formData.name = row.name
    formData.callbackUrl = ''
    formData.enabled = row.enabled
    formData.remark = row.remark || ''
    formData.purchaseLicenseTypes = Array.isArray(row.purchaseLicenseTypes)
      ? [...row.purchaseLicenseTypes]
      : [...purchaseLicenseTypeOrder]
    editingRow.value = row
    fillCommercialForm(row)
    step.value = 0
    createResult.value = null
    openedSnapshot.value = formSnapshot()
    dialogVisible.value = true
  }

  async function loadTokenState() {
    try {
      const token = await fetchAppRepoToken()
      tokenReady.value = !!token?.ready
      if (token?.message) tokenMessage.value = token.message
    } catch {
      tokenReady.value = false
    }
    repoAction.value = tokenReady.value ? 'create' : 'skip'
  }

  const editDirty = computed(
    () => dialogVisible.value && isEdit.value && formSnapshot() !== openedSnapshot.value
  )

  function createDirty() {
    if (createResult.value) return false
    if (isEdit.value) return editDirty.value
    return formData.name.trim() !== '' || repoTouched.value || step.value > 0
  }

  function beforeCloseCreate(done: () => void) {
    if (!createDirty()) {
      done()
      return
    }
    appConfirm('有未保存的修改，确定放弃吗？', '确认', {
      confirmButtonText: '继续编辑',
      cancelButtonText: '放弃'
    })
      .then(() => undefined)
      .catch(() => done())
  }

  function requestCloseCreate() {
    beforeCloseCreate(() => {
      dialogVisible.value = false
    })
  }

  async function goRepoStep() {
    const valid = await formRef.value?.validate().catch(() => false)
    if (!valid) return
    if (formData.name.trim().length > 100) {
      ElMessage.error('应用名称不能超过 100 个字符')
      return
    }
    step.value = 1
    if (!repoTouched.value && tokenReady.value) {
      try {
        const suggested = await suggestAppRepo(formData.name)
        repoName.value = suggested?.repo || ''
        repoHint.value = /[\u4e00-\u9fff]/.test(formData.name)
          ? '应用名不能直接作为仓库名，已填入短码，可以改成自己的名字。'
          : ''
      } catch {
        repoHint.value = ''
      }
    }
  }

  function finishCreate() {
    dialogVisible.value = false
    refreshData()
  }

  function retryBind() {
    createResult.value = null
    step.value = 1
    if (repoAction.value === 'create') repoAction.value = 'bind'
  }

  // 当前接收老客户端的其他应用。
  const legacyOwnerName = computed(() => {
    const owner = ((data.value || []) as AppRow[]).find(
      (row) => row.commercial?.legacyDefault && row.id !== formData.id
    )
    return owner?.name || ''
  })

  // 只提交和打开时不一样的商业版字段。已停售的应用不提交状态，恢复出售要明确打开开关。
  const commercialPayload = (): AppCommercialInput | undefined => {
    if (!commercialContext.managed) return undefined
    const form = commercialForm.value
    const saved = editingRow.value?.commercial
    const savedMode = saved?.mode || 'off'
    const input: AppCommercialInput = {}
    if (form.mode !== savedMode && (form.mode === 'selling' || form.mode === 'off')) {
      input.mode = form.mode
    }
    if (form.mode !== 'off') {
      if (form.legacyDefault !== !!saved?.legacyDefault) input.legacyDefault = form.legacyDefault
      input.graceDays = form.graceDays
      input.revokeOnPasswordChange = form.revokeOnPasswordChange
      input.features = form.features
        .split(',')
        .map((item) => item.trim())
        .filter(Boolean)
    }
    return Object.keys(input).length ? input : undefined
  }

  const closingRow = ref<AppRow | null>(null)
  const closeVisible = ref(false)
  const legacyTargets = computed(() =>
    ((data.value || []) as AppRow[])
      .filter((row) => row.id !== closingRow.value?.id && row.commercial?.mode === 'selling')
      .map((row) => ({ id: row.id, name: row.name }))
  )

  function openCloseDialog() {
    closingRow.value = editingRow.value
    closeVisible.value = true
  }

  async function afterClose() {
    dialogVisible.value = false
    await refreshData()
  }

  function goPlans() {
    if (!formData.id) return
    router.push({ path: '/license/plans', query: { appId: String(formData.id) } })
  }

  const handleSaleGap = async (gap: AppSaleGap) => {
    if (gap.path) {
      router.push(gap.path)
      return
    }
    if (gap.code !== 'signing_key' || gap.label !== '签名密钥未生成') return
    try {
      await appConfirm('尚未生成商店签名密钥。现在在本机生成？', '签名密钥', {
        type: 'warning'
      })
      await fetchEnsureStoreSnapshotKey()
      refreshData()
    } catch {
      // 用户取消时保留当前状态。
    }
  }

  const handleLicenseRequiredChange = async (row: AppRow) => {
    const licenseRequired = !row.licenseRequired
    if (!licenseRequired) {
      try {
        await appConfirm(
          `关闭应用「${row.name}」的授权校验后，客户端无需许可证即可通过验证和版本检查。应用签名与启用状态校验仍然有效，是否继续？`,
          '关闭授权校验',
          {
            type: 'warning',
            confirmButtonText: '确认关闭',
            cancelButtonText: '取消'
          }
        )
      } catch {
        return false
      }
    }

    row.licenseRequiredChanging = true
    try {
      await fetchUpdateAppLicenseRequired(row.id, licenseRequired)
      ElMessage.success(licenseRequired ? '已要求授权校验' : '已关闭授权校验')
      return true
    } catch (e) {
      console.error('[AppManage] 更新授权校验失败:', e)
      return false
    } finally {
      row.licenseRequiredChanging = false
    }
  }

  const handleResetSecret = async (row: AppRow) => {
    try {
      await appConfirm(`确定重置应用「${row.name}」的AppSecret？旧密钥将立即失效`, '警告', {
        type: 'warning'
      })
      const data = await fetchResetAppSecret(row.id)
      row.appSecret = data.appSecret
      ElMessage.success('密钥已重置')
    } catch {
      // 用户取消操作时保留当前数据。
    }
  }

  const handleVersions = (row: AppRow) => {
    router.push(`/license/apps/${row.id}/versions`)
  }

  const handleSDKPack = (row: AppRow) => {
    router.push({ name: 'SdkIndex', query: { appId: String(row.id) } })
  }

  const migrateVisible = ref(false)
  const migrateSaving = ref(false)
  const archiveSaving = ref(false)
  const migrateCount = ref(0)
  const migrateAppId = ref<number>()
  const redirectSource = ref(false)
  const redirectAppId = ref<number>()
  const migrateSource = ref<AppRow | null>(null)
  const migrateTargets = computed(() =>
    ((data.value || []) as AppRow[]).filter((item) => item.id !== migrateSource.value?.id)
  )

  const confirmMigrateAndDelete = async () => {
    const row = migrateSource.value
    if (!row || !migrateAppId.value) {
      ElMessage.info('请选择要迁移到的应用')
      return
    }
    migrateSaving.value = true
    try {
      if (redirectSource.value && !redirectAppId.value) {
        ElMessage.info('请选择要接收软件源地址的应用')
        return
      }
      await fetchDeleteLicenseApp(row.id, {
        migrateAppId: migrateAppId.value,
        redirectAppId: redirectSource.value ? redirectAppId.value : undefined
      })
      ElMessage.success(
        redirectSource.value
          ? '目录条目已迁移，软件源地址已转到目标应用'
          : '目录条目已迁移，应用已归档'
      )
      migrateVisible.value = false
      refreshRemove()
    } finally {
      migrateSaving.value = false
    }
  }

  const confirmArchiveInPlace = async () => {
    const row = migrateSource.value
    if (!row) return
    archiveSaving.value = true
    try {
      if (redirectSource.value && !redirectAppId.value) {
        ElMessage.info('请选择要接收软件源地址的应用')
        return
      }
      await fetchDeleteLicenseApp(row.id, {
        archive: true,
        redirectAppId: redirectSource.value ? redirectAppId.value : undefined
      })
      ElMessage.success(
        redirectSource.value
          ? '应用已归档，软件源地址已转到目标应用'
          : migrateCount.value > 0
            ? '应用已归档，目录条目仍挂在该应用下'
            : '应用已归档'
      )
      migrateVisible.value = false
      refreshRemove()
    } finally {
      archiveSaving.value = false
    }
  }

  const handleRestore = async (row: AppRow) => {
    try {
      await appConfirm(
        `恢复应用「${row.name}」后，可以继续往上面登记目录条目。原有授权和版本都还在。`,
        '恢复应用',
        { type: 'warning' }
      )
      await fetchRestoreLicenseApp(row.id)
      ElMessage.success('应用已恢复')
      refreshData()
    } catch {
      // 用户取消时保留当前数据。
    }
  }

  const handleDelete = async (row: AppRow) => {
    try {
      const usage = await fetchCatalogAppUsage(row.id)
      migrateSource.value = row
      migrateCount.value = usage.count
      const firstTarget = ((data.value || []) as AppRow[]).find(
        (item) => item.id !== row.id && !item.archived
      )
      migrateAppId.value = firstTarget?.id
      redirectAppId.value = firstTarget?.id
      redirectSource.value = false
      migrateVisible.value = true
    } catch {
      // 用户取消操作时保留当前数据。
    }
  }

  const handleSubmit = async () => {
    if (isEdit.value || step.value === 0) {
      const valid = await formRef.value?.validate().catch(() => false)
      if (!valid) return
    }
    if (!formData.name.trim()) return
    if (!isEdit.value && repoAction.value !== 'skip' && !repoName.value.includes('/')) {
      ElMessage.error('请填写仓库，格式为 所有者/仓库')
      return
    }

    saving.value = true
    try {
      const payload = {
        name: formData.name,
        enabled: formData.enabled,
        remark: formData.remark,
        purchaseLicenseTypes: formData.purchaseLicenseTypes,
        commercial: commercialPayload()
      }
      if (isEdit.value) {
        const saved = await fetchUpdateLicenseApp(formData.id, payload)
        ElMessage.success(
          saved?.commercial?.notice ? `已保存。${saved.commercial.notice}` : '已保存'
        )
        dialogVisible.value = false
        refreshData()
        return
      }
      const saved = await fetchCreateLicenseApp({
        ...payload,
        repoAction: repoAction.value,
        repo: repoAction.value === 'skip' ? '' : repoName.value,
        requestId: requestId.value
      })
      if (saved?.repoError) {
        createResult.value = {
          ok: false,
          title: '应用已创建，仓库没有建好',
          sub: saved.repoError,
          appId: saved.id
        }
        return
      }
      const msg = saved?.bound ? '应用已创建，仓库已绑定' : '应用已创建'
      if (saved?.commercialError) {
        ElMessage.error(`${msg}，商业版设置没有保存：${saved.commercialError}`)
      } else {
        ElMessage.success(saved?.commercial?.notice ? `${msg}。${saved.commercial.notice}` : msg)
      }
      dialogVisible.value = false
      refreshData()
    } catch (e) {
      console.error('[AppManage] 提交失败:', e)
    } finally {
      saving.value = false
    }
  }

  // keep-alive 从版本管理返回时刷新列表（首次挂载由 useTable immediate 加载，跳过避免重复请求）
  let activatedOnce = false
  onActivated(() => {
    if (activatedOnce) {
      refreshData()
    }
    activatedOnce = true
  })
</script>

<style scoped lang="scss">
  .license-apps-page {
    .no-search-card {
      margin-top: 0;
    }

    .create-steps {
      margin-bottom: 20px;
    }

    .create-steps :deep(.el-step__title) {
      font-size: 14px;
      line-height: 24px;
      white-space: nowrap;
    }

    .app-repo-cell {
      display: block;
      overflow: hidden;
      color: #6b7686;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .version-count {
      margin-left: 6px;
      font-size: 12px;
      color: var(--art-gray-600);
    }

    .license-type-tags,
    .license-type-options {
      display: flex;
      gap: 6px;
    }

    .license-type-tags {
      flex-wrap: nowrap;
      overflow: hidden;
    }

    .license-type-options {
      flex-wrap: wrap;
    }

    .license-type-options :deep(.el-checkbox) {
      margin-right: 12px;
    }

    .app-name-cell__title {
      display: block;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    :deep(.el-table .cell) {
      white-space: nowrap;
    }

    .sale-status {
      display: flex;
      flex-wrap: nowrap;
      align-items: center;
      gap: 6px;
      overflow: hidden;
    }

    .unsaved-tip {
      margin-right: auto;
      font-size: 12px;
      color: var(--el-color-danger);
    }

    .form-tip {
      width: 100%;
      margin-top: 4px;
      font-size: 12px;
      line-height: 1.5;
      color: var(--el-text-color-secondary);
    }
  }
</style>
