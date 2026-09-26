<!-- 应用管理页面 -->
<!-- art-full-height 自动计算出页面剩余高度 -->
<!-- art-table-card 一个符合系统样式的 class，同时自动撑满剩余高度 -->
<template>
  <div class="license-apps-page art-full-height">
    <ElCard class="art-table-card no-search-card" shadow="never">
      <!-- 表格头部 -->
      <div v-if="showAppLimitBar" class="app-limit-bar">
        <CommercialMark :text="multiAppText" />
        <ElButton type="primary" @click="openCommercialUpgrade">升级商业版</ElButton>
      </div>
      <ArtTableHeader v-model:columns="columnChecks" :loading="loading" @refresh="refreshData">
        <template #left>
          <ElSpace wrap>
            <ElButton v-if="commercialAction === 'renew'" type="primary" @click="openCommercialUpgrade">续费</ElButton>
            <ElButton v-else-if="commercialAction === 'view'" @click="openCommercialUpgrade">查看授权</ElButton>
            <ElButton @click="handleAdd" v-ripple>新增应用</ElButton>
            <CommercialMark v-if="showCommercialHint" :text="multiAppText" />
          </ElSpace>
        </template>
      </ArtTableHeader>

      <!-- 表格 -->
      <ArtTable :loading="loading" :data="data" :columns="columns">
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
          <div v-if="row.commercialProduct" class="sale-status">
            <ElTag type="warning" size="small">商业版产品</ElTag>
            <ElTag v-if="!row.saleGaps?.length" type="success" size="small">可售</ElTag>
            <ElButton
              v-for="gap in row.saleGaps || []"
              :key="gap.code"
              link
              type="danger"
              @click="handleSaleGap(row, gap)"
            >
              {{ gap.label }}
            </ElButton>
          </div>
          <span v-else class="text-secondary">--</span>
        </template>

        <!-- 状态 -->
        <template #enabled="{ row }">
          <ElTag :type="row.enabled ? 'success' : 'info'" size="small">
            {{ row.enabled ? '启用' : '禁用' }}
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
          <ElButton link type="primary" @click="handleVersions(row)">版本</ElButton>
          <ElButton link type="primary" @click="handleSDKPack(row)">SDK 包</ElButton>
          <ElButton link type="primary" @click="handleEdit(row)">编辑</ElButton>
          <ElButton link type="primary" @click="handleResetSecret(row)">重置密钥</ElButton>
          <ElButton link type="danger" @click="handleDelete(row)">删除</ElButton>
        </template>
      </ArtTable>
    </ElCard>

    <!-- 新增/编辑弹窗 -->
    <ElDialog v-model="dialogVisible" :title="dialogTitle" width="640px" destroy-on-close>
      <ElForm :model="formData" :rules="formRules" ref="formRef" label-width="150px">
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
        <ElFormItem label="作为本站商业版出售">
          <ElSwitch v-model="formData.commercialProduct" />
          <div class="form-tip">全站只能有一个应用打开。买家绑定和扫码购买都用这个应用。</div>
        </ElFormItem>
        <ElCollapse v-if="formData.commercialProduct" class="commercial-advanced">
          <ElCollapseItem title="高级设置" name="advanced">
            <ElFormItem label="离线宽限天数">
              <ElInputNumber v-model="formData.graceDays" :min="1" :max="30" />
              <div class="form-tip">源站暂时连不上时，买方商业版还可以继续使用的天数，默认 7 天。</div>
            </ElFormItem>
            <ElFormItem label="改密撤销绑定">
              <ElSwitch v-model="formData.revokeOnPasswordChange" />
              <div class="form-tip">打开后，买家在源站修改密码，会撤销已经绑定的站点。</div>
            </ElFormItem>
            <ElFormItem label="商业版功能键">
              <ElInput v-model.trim="formData.commercialFeatures" placeholder="一般不用改" />
              <div class="form-tip">
                商业版开放的能力。默认已填好多应用，一般不用改。
              </div>
            </ElFormItem>
          </ElCollapseItem>
        </ElCollapse>
        <ElFormItem label="备注">
          <ElInput v-model="formData.remark" type="textarea" :rows="2" placeholder="可选" />
        </ElFormItem>
      </ElForm>
      <template #footer>
        <ElButton @click="dialogVisible = false">取消</ElButton>
        <ElButton type="primary" @click="handleSubmit">确定</ElButton>
      </template>
    </ElDialog>

    <ElDialog v-model="migrateVisible" title="删除应用前迁移软件目录" width="480px">
      <p>
        应用「{{ migrateSource?.name }}」下还有
        {{ migrateCount }}
        条软件目录条目。删除前要把它们迁到另一个应用，版本、价格和安装包会一起过去。
      </p>
      <ElSelect
        v-if="migrateTargets.length"
        v-model="migrateAppId"
        placeholder="请选择目标应用"
        style="width: 100%; margin-top: 12px"
      >
        <ElOption v-for="app in migrateTargets" :key="app.id" :label="app.name" :value="app.id" />
      </ElSelect>
      <p v-else>没有其他应用可以接收这些目录条目，请先新建应用。</p>
      <template #footer>
        <ElButton @click="migrateVisible = false">取消</ElButton>
        <ElButton
          type="danger"
          :loading="migrateSaving"
          :disabled="!migrateTargets.length"
          @click="confirmMigrateAndDelete"
        >
          迁移并删除
        </ElButton>
      </template>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { useRouter } from 'vue-router'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import CommercialMark from '@/components/business/commercial/CommercialMark.vue'
  import { fetchStoreAccount, type StoreAccount } from '@/api/store'
  import {
    commercialCopy,
    commercialCta,
    isCommercialActive,
    openCommercialPrompt,
    openCommercialUpgrade,
    rememberCommercialAccount
  } from '@/utils/commercial'
  import { useTable } from '@/hooks/core/useTable'
  import {
    fetchLicenseAppList,
    fetchCreateLicenseApp,
    fetchUpdateLicenseApp,
    fetchDeleteLicenseApp,
    fetchResetAppSecret,
    fetchUpdateAppLicenseRequired,
    fetchEnsureStoreSnapshotKey,
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
  const dialogTitle = computed(() => (isEdit.value ? '编辑应用' : '新增应用'))

  const purchaseLicenseTypeOrder = ['domain', 'wildcard', 'ip', 'key'] as const
  const purchaseLicenseTypeMeta: Record<
    string,
    { label: string; tagType: 'primary' | 'success' | 'warning' | 'info' }
  > = {
    domain: { label: '单域名', tagType: 'primary' },
    wildcard: { label: '泛域名', tagType: 'success' },
    ip: { label: 'IP', tagType: 'warning' },
    key: { label: '密钥', tagType: 'info' }
  }

  const formRef = ref()
  const formData = reactive({
    id: 0,
    name: '',
    callbackUrl: '',
    enabled: true,
    remark: '',
    purchaseLicenseTypes: [...purchaseLicenseTypeOrder] as string[],
    commercialProduct: false,
    graceDays: 7,
    revokeOnPasswordChange: true,
    commercialFeatures: 'multi_app'
  })

  const formRules = {
    name: [{ required: true, message: '请输入应用名称', trigger: 'blur' }]
  }

  const { columns, columnChecks, data, loading, refreshData, refreshRemove } = useTable({
    // 核心配置
    core: {
      apiFn: fetchLicenseAppList,
      apiParams: {},
      columnsFactory: () => [
        { type: 'index', width: 60, label: '序号' }, // 序号
        { prop: 'name', label: '应用名称', minWidth: 150, showOverflowTooltip: true },
        { prop: 'sale', label: '商业版', minWidth: 220, useSlot: true },
        { prop: 'appKey', label: 'AppKey', minWidth: 220, showOverflowTooltip: true },
        {
          prop: 'purchaseLicenseTypes',
          label: '授权方式',
          minWidth: 250,
          useSlot: true
        },
        { prop: 'appSecret', label: 'AppSecret', minWidth: 220, useSlot: true },
        { prop: 'licenseCount', label: '授权数', width: 90, align: 'center' },
        { prop: 'version', label: '版本', minWidth: 120, useSlot: true },
        { prop: 'enabled', label: '状态', width: 90, align: 'center', useSlot: true },
        {
          prop: 'licenseRequired',
          label: '授权校验',
          width: 100,
          align: 'center',
          useSlot: true
        },
        { prop: 'createdAt', label: '创建时间', width: 160 },
        {
          prop: 'operation',
          label: '操作',
          width: 230,
          fixed: 'right',
          useSlot: true
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

  const orderedPurchaseLicenseTypes = (types: string[] = []) => {
    return purchaseLicenseTypeOrder.filter((licenseType) => types.includes(licenseType))
  }

  const storeAccount = ref<StoreAccount | null>(null)
  const multiAppText = commercialCopy.multi_app
  const commercialAction = computed(() => commercialCta(storeAccount.value))
  const showCommercialHint = computed(() => !isCommercialActive(storeAccount.value))
  const showAppLimitBar = computed(() => showCommercialHint.value && (data.value?.length || 0) >= 1)

  async function loadCommercialAccount() {
    try {
      storeAccount.value = await fetchStoreAccount()
    } catch {
      storeAccount.value = null
    }
    rememberCommercialAccount(storeAccount.value)
  }

  onMounted(() => {
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
    formData.commercialProduct = false
    formData.graceDays = 7
    formData.revokeOnPasswordChange = true
    formData.commercialFeatures = 'multi_app'
    dialogVisible.value = true
  }

  const fillCommercialForm = (row?: AppRow) => {
    formData.commercialProduct = !!row?.commercialProduct
    formData.graceDays = row?.graceDays || 7
    formData.revokeOnPasswordChange = row?.revokeOnPasswordChange !== false
    formData.commercialFeatures = (row?.commercialFeatures || ['multi_app']).join(',')
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
    fillCommercialForm(row)
    dialogVisible.value = true
  }

  const confirmCommercialSwitch = async () => {
    if (!formData.commercialProduct) return true
    const other = ((data.value || []) as AppRow[]).find(
      (row) => row.commercialProduct && row.id !== formData.id
    )
    if (!other) return true
    try {
      await ElMessageBox.confirm(
        `应用「${other.name}」正在作为本站商业版出售。开启后会改到当前应用，原应用不再出售。`,
        '切换商业版产品',
        { type: 'warning', confirmButtonText: '切换', cancelButtonText: '取消' }
      )
      return true
    } catch {
      return false
    }
  }

  const commercialPayload = () => ({
    commercialProduct: formData.commercialProduct,
    graceDays: formData.graceDays,
    revokeOnPasswordChange: formData.revokeOnPasswordChange,
    commercialFeatures: formData.commercialFeatures
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean)
  })

  const handleSaleGap = async (row: AppRow, gap: AppSaleGap) => {
    if (gap.path) {
      router.push(gap.path)
      return
    }
    if (gap.code !== 'signing_key' || gap.label !== '签名密钥未生成') return
    try {
      await ElMessageBox.confirm('尚未生成商店签名密钥。现在在本机生成？', '签名密钥', {
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
        await ElMessageBox.confirm(
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
      await ElMessageBox.confirm(
        `确定重置应用「${row.name}」的AppSecret？旧密钥将立即失效`,
        '警告',
        { type: 'warning' }
      )
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
  const migrateCount = ref(0)
  const migrateAppId = ref<number>()
  const migrateSource = ref<AppRow | null>(null)
  const migrateTargets = computed(() =>
    ((data.value || []) as AppRow[]).filter((item) => item.id !== migrateSource.value?.id)
  )

  const confirmMigrateAndDelete = async () => {
    const row = migrateSource.value
    if (!row || !migrateAppId.value) {
      ElMessage.warning('请选择要迁移到的应用')
      return
    }
    migrateSaving.value = true
    try {
      await fetchDeleteLicenseApp(row.id, migrateAppId.value)
      ElMessage.success('目录条目已迁移，应用已删除')
      migrateVisible.value = false
      refreshRemove()
    } finally {
      migrateSaving.value = false
    }
  }

  const handleDelete = async (row: AppRow) => {
    try {
      const usage = await fetchCatalogAppUsage(row.id)
      if (usage.count > 0) {
        migrateSource.value = row
        migrateCount.value = usage.count
        migrateAppId.value = ((data.value || []) as AppRow[]).find((item) => item.id !== row.id)?.id
        migrateVisible.value = true
        return
      }
      await ElMessageBox.confirm(
        `删除应用「${row.name}」将同时清除其所有授权记录，确定？`,
        '危险操作',
        { type: 'error' }
      )
      await fetchDeleteLicenseApp(row.id)
      ElMessage.success('删除成功')
      refreshRemove()
    } catch {
      // 用户取消操作时保留当前数据。
    }
  }

  const handleSubmit = async () => {
    const valid = await formRef.value?.validate().catch(() => false)
    if (!valid) return

    try {
      if (!(await confirmCommercialSwitch())) return
      const payload = {
        name: formData.name,
        enabled: formData.enabled,
        remark: formData.remark,
        purchaseLicenseTypes: formData.purchaseLicenseTypes,
        ...commercialPayload()
      }
      if (isEdit.value) {
        const saved = await fetchUpdateLicenseApp(formData.id, payload)
        ElMessage.success(saved?.switched ? '已切换为本站商业版产品，原应用已关闭出售' : '编辑成功')
      } else {
        const saved = await fetchCreateLicenseApp(payload)
        ElMessage.success(saved?.switched ? '已切换为本站商业版产品，原应用已关闭出售' : '新增成功')
      }
      dialogVisible.value = false
      refreshData()
    } catch (e) {
      console.error('[AppManage] 提交失败:', e)
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
    .app-limit-bar {
      display: flex;
      gap: 12px;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 12px;
      padding: 10px 12px;
      background: var(--el-color-warning-light-9);
      border: 1px solid var(--el-color-warning-light-5);
      border-radius: 8px;
    }

    .no-search-card {
      margin-top: 0;
    }

    .version-count {
      margin-left: 6px;
      font-size: 12px;
      color: var(--art-gray-600);
    }

    .license-type-tags,
    .license-type-options {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
    }

    .license-type-options :deep(.el-checkbox) {
      margin-right: 12px;
    }

    .sale-status {
      display: flex;
      flex-wrap: wrap;
      align-items: center;
      gap: 6px;
    }

    .commercial-advanced {
      margin-bottom: 12px;
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
