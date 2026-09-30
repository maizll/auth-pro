<!-- 授权列表页面 -->
<!-- art-full-height 自动计算出页面剩余高度 -->
<!-- art-table-card 一个符合系统样式的 class，同时自动撑满剩余高度 -->
<template>
  <div class="license-list-page art-full-height">
    <!-- 搜索栏 -->
    <LicenseSearch v-model="searchForm" @search="handleSearch" @reset="resetSearchParams" />

    <ElCard class="art-table-card" shadow="never">
      <CommercialReissueBanner />
      <!-- 表格头部 -->
      <ArtTableHeader v-model:columns="columnChecks" :loading="loading" @refresh="refreshData">
        <template #left>
          <ElSpace wrap>
            <ElButton @click="handleAdd" v-ripple>新增授权</ElButton>
          </ElSpace>
        </template>
      </ArtTableHeader>

      <!-- 表格 -->
      <ArtTable
        :loading="loading"
        :data="data"
        :columns="columns"
        :pagination="pagination"
        @pagination:size-change="handleSizeChange"
        @pagination:current-change="handleCurrentChange"
      >
        <template #domain="{ row }">
          <div class="domain-cell" :title="row.domain">
            <span class="domain-cell__value">{{ row.domain || '--' }}</span>
          </div>
        </template>

        <template #expireAt="{ row }">
          <EditionExpire
            :commercial="row.commercialActive"
            :title="formatLicenseExpire(row.expireAt)"
          >
            {{ formatLicenseExpire(row.expireAt, narrow) }}
          </EditionExpire>
        </template>

        <!-- 归属账号 -->
        <template #owner="{ row }">
          <div class="owner-cell">
            <ElTag :type="row.ownerType === 'agent' ? 'primary' : 'info'" size="small">
              {{ row.ownerType === 'agent' ? '代理' : '用户' }}
            </ElTag>
            <span>{{ row.ownerName || `ID ${row.ownerId}` }}</span>
          </div>
        </template>

        <!-- 类型 -->
        <template #typeLabel="{ row }">
          <ElTag :type="typeTagMap[row.type]" size="small">{{ row.typeLabel }}</ElTag>
        </template>

        <!-- 状态 -->
        <template #statusLabel="{ row }">
          <BizStatusTag
            domain="license"
            :status="row.status"
            :label="row.statusLabel"
            size="small"
          />
        </template>

        <!-- 站点 -->
        <template #sites="{ row }">
          <span v-if="row.type === 'key'">
            {{ row.boundSites ?? 0 }} / {{ Number(row.maxSites) ? row.maxSites : '不限' }}
          </span>
          <span v-else class="text-secondary">--</span>
        </template>
        <template #freeSiteChanges="{ row }">
          {{ Number(row.freeSiteChanges) < 0 ? '不限' : `${row.freeSiteChanges} 次` }}
        </template>

        <!-- 操作 -->
        <template #source="{ row }">
          <ElTag v-if="row.sourceLabel" size="small" effect="plain">{{ row.sourceLabel }}</ElTag>
          <span v-else class="text-secondary">--</span>
        </template>

        <template #operation="{ row }">
          <RowActions
            :primary="[
              { key: 'edit', label: '编辑' },
              { key: 'detail', label: '详情' }
            ]"
            :more="licenseMoreActions(row)"
            @click="(action) => onLicenseAction(row, action)"
          />
        </template>
      </ArtTable>
    </ElCard>

    <!-- 新增/编辑弹窗 -->
    <ElDialog v-model="dialogVisible" :title="dialogTitle" width="560px" destroy-on-close>
      <ElForm :model="formData" :rules="formRules" ref="formRef" label-width="100px">
        <template v-if="!isEdit">
          <ElFormItem label="开通到" prop="ownerType">
            <ElSegmented v-model="formData.ownerType" :options="ownerTypeOptions" block />
          </ElFormItem>
          <ElFormItem label="归属账号" prop="ownerId">
            <ElSelect
              v-model="formData.ownerId"
              filterable
              remote
              reserve-keyword
              clearable
              :remote-method="fetchOwnerOptions"
              :loading="ownerLoading"
              :placeholder="ownerSelectPlaceholder"
              style="width: 100%"
              @visible-change="handleOwnerSelectVisible"
            >
              <ElOption
                v-for="owner in ownerOptions"
                :key="`${owner.type}-${owner.id}`"
                :label="formatOwnerOption(owner)"
                :value="owner.id"
              />
            </ElSelect>
            <div class="form-tip">可按名称或登录账号搜索，授权会直接显示在该账号下</div>
          </ElFormItem>
        </template>
        <ElFormItem label="应用" prop="appId">
          <BizAppSelect
            v-model="formData.appId"
            api="license-options"
            placeholder="请选择应用"
            style="width: 100%"
            :disabled="isEdit"
            @change="handleAppChange"
          />
        </ElFormItem>
        <ElFormItem v-if="!isEdit" label="套餐" prop="planId">
          <ElSelect
            v-model="formData.planId"
            :loading="planLoading"
            :disabled="!formData.appId"
            :placeholder="formData.appId ? '请选择套餐' : '请先选择应用'"
            no-data-text="该应用暂无可用套餐"
            style="width: 100%"
          >
            <ElOption
              v-for="plan in planList"
              :key="plan.id"
              :label="`${plan.name}（${plan.durationText}，¥${Number(plan.price).toFixed(2)}）`"
              :value="plan.id"
            />
          </ElSelect>
          <div v-if="formData.appId && !planLoading && planList.length === 0" class="form-tip">
            该应用暂无启用套餐，请先在套餐管理中配置
          </div>
        </ElFormItem>
        <ElFormItem label="授权类型" prop="type">
          <ElRadioGroup v-model="formData.type">
            <ElRadio value="domain">单域名</ElRadio>
            <ElRadio value="wildcard">泛域名</ElRadio>
            <ElRadio value="ip">IP地址</ElRadio>
            <ElRadio value="key">密钥</ElRadio>
          </ElRadioGroup>
        </ElFormItem>
        <ElFormItem :label="domainLabel" prop="domain">
          <ElInput v-model="formData.domain" :placeholder="domainPlaceholder" />
          <div class="form-tip">{{ domainTip }}</div>
        </ElFormItem>
        <ElFormItem v-if="!isEdit" label="到期时间">
          <ElInput
            :model-value="planExpireText"
            disabled
            :placeholder="formData.planId ? '' : '选择套餐后自动计算'"
          />
          <div class="form-tip">到期时间由所选套餐自动计算，实际时间以后端创建结果为准</div>
        </ElFormItem>
        <ElFormItem v-else label="到期时间" prop="expireAt">
          <ElDatePicker
            v-model="formData.expireAt"
            type="datetime"
            placeholder="选择到期时间，留空为永久"
            style="width: 100%"
          />
        </ElFormItem>
        <ElFormItem label="备注">
          <ElInput v-model="formData.remark" type="textarea" :rows="2" placeholder="可选备注" />
        </ElFormItem>
      </ElForm>
      <template #footer>
        <ElButton @click="dialogVisible = false">取消</ElButton>
        <ElButton type="primary" :loading="submitting" @click="handleSubmit">确定</ElButton>
      </template>
    </ElDialog>

    <!-- 密钥站点管理弹窗 -->
    <ElDialog v-model="siteDialog.visible" title="密钥绑定站点" width="680px" destroy-on-close>
      <ElAlert
        v-if="siteDialog.maxSites > 0"
        :title="`当前已绑定 ${siteDialog.list.length} / ${siteDialog.maxSites} 个站点，达到上限后新站点验证会被拒绝，可解绑释放名额。`"
        type="info"
        show-icon
        :closable="false"
        class="mb-3"
      />
      <ElAlert
        v-else
        title="该密钥不限制站点数量。"
        type="info"
        show-icon
        :closable="false"
        class="mb-3"
      />
      <ElTable :data="siteDialog.list" size="small" v-loading="siteDialog.loading" max-height="360">
        <ElTableColumn label="类型" width="80">
          <template #default="{ row }">
            <ElTag :type="row.targetType === 'ip' ? 'info' : undefined" size="small">
              {{ row.targetType === 'ip' ? 'IP' : '域名' }}
            </ElTag>
          </template>
        </ElTableColumn>
        <ElTableColumn prop="target" label="站点" min-width="160" show-overflow-tooltip />
        <ElTableColumn prop="serverIp" label="最近服务器IP" width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ row.serverIp || '--' }}</span>
          </template>
        </ElTableColumn>
        <ElTableColumn prop="firstSeenAt" label="首次绑定" width="160" />
        <ElTableColumn prop="lastSeenAt" label="最近验证" width="160" />
        <ElTableColumn label="操作" width="80" align="center">
          <template #default="{ row }">
            <ElButton link type="danger" size="small" @click="handleUnbindSite(row)">解绑</ElButton>
          </template>
        </ElTableColumn>
        <template #empty>
          <ElEmpty description="暂无绑定站点" :image-size="60" />
        </template>
      </ElTable>
    </ElDialog>

    <ElDialog
      v-model="quotaDialog.visible"
      title="调整剩余更换次数"
      width="min(460px, 92vw)"
      destroy-on-close
    >
      <p class="quota-current">当前剩余：{{ quotaDialog.currentText }}</p>
      <ElRadioGroup v-model="quotaDialog.mode">
        <ElRadio value="unlimited">设为不限</ElRadio>
        <ElRadio value="value">设为具体次数</ElRadio>
        <ElRadio value="delta">加减次数</ElRadio>
      </ElRadioGroup>
      <ElInputNumber
        v-if="quotaDialog.mode !== 'unlimited'"
        v-model="quotaDialog.amount"
        :precision="0"
        controls-position="right"
        class="quota-input"
      />
      <p class="form-tip">不限次数时不能直接加减，需要先设为具体次数。</p>
      <template #footer>
        <ElButton @click="quotaDialog.visible = false">取消</ElButton>
        <ElButton type="primary" :loading="quotaDialog.submitting" @click="submitChangeQuota"
          >确定</ElButton
        >
      </template>
    </ElDialog>

    <ElDialog
      v-model="grantDialog.visible"
      title="开通商业版"
      :width="narrow ? '92%' : '420px'"
      destroy-on-close
    >
      <p class="quota-current">为「{{ grantDialog.label }}」开通商业版</p>
      <ElRadioGroup v-model="grantDialog.period">
        <ElRadio value="permanent">永久</ElRadio>
        <ElRadio value="yearly">按年</ElRadio>
        <ElRadio value="monthly">按月</ElRadio>
      </ElRadioGroup>
      <p class="form-tip"
        >按年、按月到期后自动按免费版处理。客户站刷新页面或点「立即刷新」即可生效。</p
      >
      <template #footer>
        <ElButton @click="grantDialog.visible = false">取消</ElButton>
        <ElButton type="primary" :loading="grantDialog.submitting" @click="submitGrantCommercial"
          >开通</ElButton
        >
      </template>
    </ElDialog>

    <ElDrawer
      v-model="detail.visible"
      :title="`授权详情 ${detail.licenseNo}`"
      :size="narrow ? '100%' : '760px'"
      destroy-on-close
    >
      <div v-loading="detail.loading" class="entitlement-drawer">
        <h4>商业版记录</h4>
        <div class="entitlement-scroll">
          <ElTable :data="detail.editions" size="small" empty-text="还没有商业版记录">
            <ElTableColumn label="期限" width="80">
              <template #default="{ row }">
                <button type="button" class="cell-copy" @click="copyCell(row.periodLabel)">
                  {{ row.periodLabel }}
                </button>
              </template>
            </ElTableColumn>
            <ElTableColumn label="来源" min-width="100">
              <template #default="{ row }">
                <button type="button" class="cell-copy" @click="copyCell(row.source)">
                  {{ row.source || '—' }}
                </button>
              </template>
            </ElTableColumn>
            <ElTableColumn label="状态" width="90">
              <template #default="{ row }">
                <span class="cell-copy">{{ row.statusLabel }}</span>
              </template>
            </ElTableColumn>
            <ElTableColumn label="开始" width="140">
              <template #default="{ row }">
                <span class="cell-copy">{{ row.startedAt }}</span>
              </template>
            </ElTableColumn>
            <ElTableColumn label="到期" width="140">
              <template #default="{ row }">
                <span class="cell-copy">{{ row.expireAt || '永久' }}</span>
              </template>
            </ElTableColumn>
          </ElTable>
        </div>

        <h4>已购插件</h4>
        <div class="gift-row">
          <ElSelect v-model="detail.giftKey" placeholder="选择要赠送的插件或模板" filterable>
            <ElOption
              v-for="item in detail.catalog"
              :key="`${item.itemKind}:${item.itemId}`"
              :label="item.label"
              :value="`${item.itemKind}:${item.itemId}`"
            />
          </ElSelect>
          <ElRadioGroup v-model="detail.giftPeriod">
            <ElRadio value="permanent">永久</ElRadio>
            <ElRadio value="yearly">按年</ElRadio>
            <ElRadio value="monthly">按月</ElRadio>
          </ElRadioGroup>
          <ElButton type="primary" :loading="detail.gifting" @click="submitGiftPlugin"
            >赠送</ElButton
          >
        </div>
        <div class="entitlement-scroll">
          <ElTable :data="detail.plugins" size="small" empty-text="还没有已购插件">
            <ElTableColumn label="名称" min-width="140">
              <template #default="{ row }">
                <button type="button" class="cell-copy" @click="copyCell(row.itemId)">
                  {{ row.itemKindLabel }} {{ row.itemId }}
                </button>
              </template>
            </ElTableColumn>
            <ElTableColumn label="期限" width="80">
              <template #default="{ row }">
                <span class="cell-copy">{{ row.periodLabel }}</span>
              </template>
            </ElTableColumn>
            <ElTableColumn label="来源" width="90">
              <template #default="{ row }">
                <span class="cell-copy">{{ row.sourceLabel }}</span>
              </template>
            </ElTableColumn>
            <ElTableColumn label="到期" width="140">
              <template #default="{ row }">
                <span class="cell-copy">{{ row.expireAt || '永久' }}</span>
              </template>
            </ElTableColumn>
            <ElTableColumn label="操作" width="72" fixed="right">
              <template #default="{ row }">
                <ElButton
                  v-if="row.active"
                  link
                  type="danger"
                  @click="revokePlugin(row.id, row.itemId)"
                  >撤销</ElButton
                >
                <span v-else class="text-secondary">已失效</span>
              </template>
            </ElTableColumn>
          </ElTable>
        </div>

        <h4>操作记录</h4>
        <div class="entitlement-scroll">
          <ElTable :data="detail.logs" size="small" empty-text="还没有操作记录">
            <ElTableColumn label="时间" width="140">
              <template #default="{ row }">
                <span class="cell-copy">{{ row.createdAt }}</span>
              </template>
            </ElTableColumn>
            <ElTableColumn label="操作" width="110">
              <template #default="{ row }">
                <span class="cell-copy">{{ row.actionLabel }}</span>
              </template>
            </ElTableColumn>
            <ElTableColumn label="说明" min-width="160">
              <template #default="{ row }">
                <button type="button" class="cell-copy" @click="copyCell(row.detail || row.target)">
                  {{ row.detail || row.target || '—' }}
                </button>
              </template>
            </ElTableColumn>
            <ElTableColumn label="操作人" width="100" fixed="right">
              <template #default="{ row }">
                <span class="cell-copy">{{ row.actor || '—' }}</span>
              </template>
            </ElTableColumn>
          </ElTable>
        </div>
      </div>
    </ElDrawer>
  </div>
</template>

<script setup lang="ts">
  import { appConfirm, appPrompt } from '@/utils/app-confirm'
  import { ElMessage } from 'element-plus'
  import CommercialReissueBanner from '@/components/business/commercial/CommercialReissueBanner.vue'
  import EditionExpire from '@/components/business/commercial/EditionExpire.vue'
  import { showCaughtError } from '@/utils/http/error-toast'
  import RowActions, { type RowActionItem } from '@/components/business/row-actions/index.vue'
  import { useNarrowScreen } from '@/hooks/core/useNarrowScreen'
  import { formatLicenseExpire } from '@/utils/license-expire'
  import { useTable } from '@/hooks/core/useTable'
  import {
    fetchLicenseList,
    fetchLicenseOwners,
    fetchCreateLicense,
    fetchUpdateLicense,
    fetchToggleLicense,
    fetchDeleteLicense,
    fetchGrantCommercialEdition,
    fetchRevokeCommercialEdition,
    fetchLicenseCommercialDetail,
    fetchGiftLicensePlugin,
    fetchRevokeLicensePlugin,
    type CommercialPeriod,
    type LicenseCatalogOption,
    type LicenseEditionRecord,
    type LicenseOperationRecord,
    type LicensePluginRecord,
    fetchLicenseSites,
    fetchUnbindLicenseSite,
    fetchAdjustLicenseSiteChanges,
    fetchPlanList,
    type LicenseItem,
    type LicenseOwnerOption,
    type LicensePlanOption,
    type LicenseSearchParams,
    type LicenseSiteItem
  } from '@/api/license-manage'
  import LicenseSearch from './modules/license-search.vue'

  defineOptions({ name: 'LicenseList' })

  type OwnerType = 'user' | 'agent'
  type TagType = 'primary' | 'success' | 'warning' | 'info' | 'danger' | undefined

  interface LicenseSearchForm {
    keyword?: string
    type?: string
    status?: string
    source?: string
    appId?: number | string
  }

  // 弹窗相关
  const dialogVisible = ref(false)
  const isEdit = ref(false)
  const submitting = ref(false)
  const ownerLoading = ref(false)
  const planLoading = ref(false)
  const dialogTitle = computed(() => (isEdit.value ? '编辑授权' : '新增授权'))

  // 搜索表单
  const searchForm = ref<LicenseSearchForm>({
    keyword: undefined,
    type: undefined,
    status: undefined,
    source: undefined,
    appId: undefined
  })

  const planList = ref<LicensePlanOption[]>([])

  const ownerTypeOptions = [
    { label: '用户账号', value: 'user' },
    { label: '代理账号', value: 'agent' }
  ]
  const ownerOptions = ref<LicenseOwnerOption[]>([])
  let ownerSearchTimer: ReturnType<typeof setTimeout> | undefined
  let ownerSearchSequence = 0
  let planRequestSequence = 0

  const typeTagMap: Record<string, TagType> = {
    domain: undefined,
    wildcard: 'success',
    ip: 'info',
    key: 'info'
  }

  const formRef = ref()
  const formData = reactive({
    id: 0,
    appId: '' as string | number | null,
    planId: null as number | null,
    ownerType: 'user' as OwnerType,
    ownerId: null as number | null,
    type: 'domain',
    domain: '',
    expireAt: '',
    remark: ''
  })

  const siteDialog = reactive({
    visible: false,
    loading: false,
    licenseId: 0,
    licenseNo: '',
    maxSites: 0,
    list: [] as LicenseSiteItem[]
  })
  const quotaDialog = reactive({
    visible: false,
    submitting: false,
    licenseId: 0,
    current: -1,
    currentText: '不限',
    mode: 'delta' as 'unlimited' | 'value' | 'delta',
    amount: 1
  })

  function openChangeQuota(row: LicenseItem) {
    const current = Number(row.freeSiteChanges)
    quotaDialog.licenseId = row.id
    quotaDialog.current = Number.isFinite(current) ? current : -1
    quotaDialog.currentText = quotaDialog.current < 0 ? '不限' : `${quotaDialog.current} 次`
    quotaDialog.mode = quotaDialog.current < 0 ? 'value' : 'delta'
    quotaDialog.amount = 1
    quotaDialog.visible = true
  }

  async function submitChangeQuota() {
    const payload: { unlimited?: boolean; delta?: number; value?: number } = {}
    if (quotaDialog.mode === 'unlimited') payload.unlimited = true
    else if (quotaDialog.mode === 'value') payload.value = quotaDialog.amount
    else payload.delta = quotaDialog.amount
    quotaDialog.submitting = true
    try {
      await fetchAdjustLicenseSiteChanges(quotaDialog.licenseId, payload)
      ElMessage.success('已调整剩余更换次数')
      quotaDialog.visible = false
      refreshData()
    } catch (error) {
      showCaughtError(error, '调整失败')
    } finally {
      quotaDialog.submitting = false
    }
  }

  const {
    columns,
    columnChecks,
    data,
    loading,
    pagination,
    getData,
    replaceSearchParams,
    resetSearchParams,
    handleSizeChange,
    handleCurrentChange,
    refreshData,
    refreshCreate,
    refreshUpdate,
    refreshRemove
  } = useTable({
    // 核心配置
    core: {
      apiFn: fetchLicenseList,
      apiParams: {
        page: 1,
        pageSize: 20,
        ...searchForm.value
      },
      // 后端授权接口使用 page / pageSize 分页字段
      paginationKey: {
        current: 'page',
        size: 'pageSize'
      },
      columnsFactory: () => [
        { type: 'index', width: 60, label: '序号', mobileHidden: true },
        {
          prop: 'domain',
          label: '域名/IP/密钥',
          mobileLabel: '域名',
          minWidth: 150,
          useSlot: true,
          mobilePriority: 1
        },
        {
          prop: 'owner',
          label: '归属账号',
          minWidth: 120,
          showOverflowTooltip: true,
          useSlot: true,
          mobileHidden: true
        },
        { prop: 'appName', label: '应用', width: 120, mobileHidden: true },
        {
          prop: 'source',
          label: '来源',
          width: 110,
          align: 'center',
          useSlot: true,
          mobileHidden: true
        },
        {
          prop: 'typeLabel',
          label: '类型',
          width: 90,
          align: 'center',
          useSlot: true,
          mobileHidden: true
        },
        {
          prop: 'statusLabel',
          label: '状态',
          width: 90,
          align: 'center',
          useSlot: true,
          mobilePriority: 2,
          mobileWidth: 72
        },
        {
          prop: 'expireAt',
          label: '到期时间',
          mobileLabel: '到期',
          width: 228,
          useSlot: true,
          mobilePriority: 3,
          mobileWidth: 168
        },
        { prop: 'verifyCount', label: '验证次数', width: 100, align: 'center', mobileHidden: true },
        {
          prop: 'sites',
          label: '站点',
          width: 120,
          align: 'center',
          useSlot: true,
          mobileHidden: true
        },
        {
          prop: 'freeSiteChanges',
          label: '剩余更换',
          width: 110,
          align: 'center',
          useSlot: true,
          mobileHidden: true
        },
        { prop: 'createdAt', label: '创建时间', width: 160, mobileHidden: true },
        {
          prop: 'operation',
          label: '操作',
          width: 156,
          useSlot: true,
          mobilePriority: 4,
          mobileWidth: 120
        }
      ]
    },
    // 数据处理
    transform: {
      dataTransformer: (records) => {
        if (!Array.isArray(records)) {
          return []
        }
        return records
      }
    }
  })

  const narrow = useNarrowScreen()

  function licenseMoreActions(row: LicenseItem): RowActionItem[] {
    return [
      ...(row.type === 'key' ? [{ key: 'sites', label: '站点' }] : []),
      row.commercialActive
        ? { key: 'revoke', label: '吊销商业版', danger: true }
        : { key: 'grant', label: '授予商业版' },
      { key: 'quota', label: '调整次数' },
      { key: 'toggle', label: row.status === 'active' ? '禁用' : '启用' },
      { key: 'delete', label: '删除', danger: true }
    ]
  }

  function onLicenseAction(row: LicenseItem, action: RowActionItem) {
    switch (action.key) {
      case 'edit':
        handleEdit(row)
        break
      case 'detail':
        openLicenseDetail(row)
        break
      case 'sites':
        openSiteDialog(row)
        break
      case 'grant':
        handleGrantCommercial(row)
        break
      case 'revoke':
        handleRevokeCommercial(row)
        break
      case 'quota':
        openChangeQuota(row)
        break
      case 'toggle':
        handleToggle(row)
        break
      case 'delete':
        handleDelete(row)
        break
    }
  }

  // ==================== 表单校验 ====================

  const validateLicenseTarget = (type: string, value: string) => {
    const target = (value || '').trim().toLowerCase()
    if (type === 'key') return ''
    if (type === 'domain' && !isValidSingleDomain(target)) return '单域名格式不正确'
    if (type === 'wildcard' && (!target.startsWith('*.') || !isValidSingleDomain(target.slice(2))))
      return '泛域名格式不正确'
    if (type === 'ip' && !isValidIP(target)) return 'IP 格式不正确'
    return ''
  }

  const isValidSingleDomain = (value: string) => {
    if (
      !value ||
      value.startsWith('*.') ||
      value.endsWith('.') ||
      /[/:@\s]/.test(value) ||
      isValidIP(value)
    )
      return false
    const labels = value.split('.')
    if (labels.length < 2) return false
    if (!/^[a-z]{2,}$/.test(labels[labels.length - 1])) return false
    return labels.every((label) => /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(label))
  }

  const isValidIP = (value: string) => {
    const ipv4 = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/
    const ipv6 = /^(([0-9a-f]{1,4}:){7}[0-9a-f]{1,4}|::1|::)$/i
    return ipv4.test(value) || ipv6.test(value)
  }

  const validateDomainRule = (_rule: any, value: string, callback: (error?: Error) => void) => {
    const target = (value || '').trim()
    if (formData.type !== 'key' && !target) {
      callback(new Error('请输入授权值'))
      return
    }
    const message = validateLicenseTarget(formData.type, target)
    if (message) {
      callback(new Error(message))
      return
    }
    callback()
  }

  const formRules = {
    appId: [{ required: true, message: '请选择应用', trigger: 'change' }],
    planId: [{ required: true, message: '请选择套餐', trigger: 'change' }],
    ownerType: [{ required: true, message: '请选择开通对象', trigger: 'change' }],
    ownerId: [{ required: true, message: '请选择归属账号', trigger: 'change' }],
    type: [{ required: true, message: '请选择授权类型', trigger: 'change' }],
    domain: [{ validator: validateDomainRule, trigger: ['blur', 'change'] }]
  }

  const domainLabel = computed(() => {
    const map: Record<string, string> = {
      domain: '域名',
      wildcard: '泛域名',
      ip: 'IP地址',
      key: '密钥'
    }
    return map[formData.type] || '域名'
  })

  const domainPlaceholder = computed(() => {
    const map: Record<string, string> = {
      domain: '例: example.com',
      wildcard: '例: *.example.com',
      ip: '例: 192.168.1.1',
      key: '留空自动生成'
    }
    return map[formData.type] || ''
  })

  const domainTip = computed(() => {
    const map: Record<string, string> = {
      domain: '只填写根域名或具体单域名，不带 http://、端口和路径',
      wildcard: '必须以 *. 开头，例如 *.example.com',
      ip: '只支持标准 IPv4 或 IPv6 地址，不支持网段或范围',
      key: '不填写时系统自动生成密钥'
    }
    return map[formData.type] || ''
  })

  const planExpireText = computed(() => {
    const plan = planList.value.find((item) => item.id === formData.planId)
    if (!plan) return ''
    if (plan.durationDays <= 0) return '永久'

    const expireAt = new Date()
    expireAt.setDate(expireAt.getDate() + plan.durationDays)
    const pad = (value: number) => String(value).padStart(2, '0')
    return `${expireAt.getFullYear()}-${pad(expireAt.getMonth() + 1)}-${pad(expireAt.getDate())} ${pad(expireAt.getHours())}:${pad(expireAt.getMinutes())}:${pad(expireAt.getSeconds())}`
  })

  const ownerSelectPlaceholder = computed(() =>
    formData.ownerType === 'agent' ? '搜索并选择代理账号' : '搜索并选择用户账号'
  )

  watch(
    () => formData.type,
    () => {
      formRef.value?.clearValidate?.('domain')
      if (formData.domain) formRef.value?.validateField?.('domain')
    }
  )

  watch(
    () => formData.ownerType,
    () => {
      formData.ownerId = null
      ownerOptions.value = []
      formRef.value?.clearValidate?.('ownerId')
      if (!isEdit.value && dialogVisible.value) fetchOwnerOptions('')
    }
  )

  // ==================== 数据加载 ====================

  /**
   * 搜索处理
   */
  const handleSearch = (params: LicenseSearchForm) => {
    replaceSearchParams(params as Partial<LicenseSearchParams>)
    getData()
  }

  const formatOwnerOption = (owner: LicenseOwnerOption) => {
    return owner.name === owner.account ? owner.name : `${owner.name} (${owner.account})`
  }

  const fetchOwnerOptions = (keyword = '') => {
    if (ownerSearchTimer) clearTimeout(ownerSearchTimer)
    const sequence = ++ownerSearchSequence
    ownerSearchTimer = setTimeout(async () => {
      ownerLoading.value = true
      try {
        const data = await fetchLicenseOwners({
          ownerType: formData.ownerType,
          keyword: keyword.trim(),
          limit: 30
        })
        if (sequence === ownerSearchSequence) ownerOptions.value = data || []
      } catch {
        if (sequence === ownerSearchSequence) ownerOptions.value = []
      } finally {
        if (sequence === ownerSearchSequence) ownerLoading.value = false
      }
    }, 250)
  }

  const handleOwnerSelectVisible = (visible: boolean) => {
    if (visible && ownerOptions.value.length === 0) fetchOwnerOptions('')
  }

  const fetchPlans = async (appId: string | number) => {
    const sequence = ++planRequestSequence
    planList.value = []
    if (!appId) return

    planLoading.value = true
    try {
      const data = await fetchPlanList({ appId: Number(appId), status: 'enabled' })
      if (sequence === planRequestSequence) planList.value = data || []
    } catch {
      if (sequence === planRequestSequence) planList.value = []
    } finally {
      if (sequence === planRequestSequence) planLoading.value = false
    }
  }

  const handleAppChange = (appId: string | number | Array<string | number> | null | undefined) => {
    if (isEdit.value || Array.isArray(appId)) return
    formData.planId = null
    formRef.value?.clearValidate?.('planId')
    fetchPlans(appId || '')
  }

  // ==================== 增删改操作 ====================

  const handleAdd = () => {
    isEdit.value = false
    formData.id = 0
    formData.appId = ''
    formData.planId = null
    formData.ownerType = 'user'
    formData.ownerId = null
    formData.type = 'domain'
    formData.domain = ''
    formData.expireAt = ''
    formData.remark = ''
    planRequestSequence++
    planList.value = []
    planLoading.value = false
    ownerOptions.value = []
    dialogVisible.value = true
    fetchOwnerOptions('')
  }

  const handleEdit = (row: LicenseItem) => {
    isEdit.value = true
    formData.id = row.id
    formData.appId = String(row.appId)
    formData.planId = null
    formData.type = row.type
    formData.domain = row.domain
    formData.expireAt = row.expireAt
    formData.remark = row.remark
    planRequestSequence++
    planList.value = []
    planLoading.value = false
    dialogVisible.value = true
  }

  const grantDialog = reactive({
    visible: false,
    id: 0,
    label: '',
    period: 'permanent' as CommercialPeriod,
    submitting: false
  })

  const detail = reactive({
    visible: false,
    loading: false,
    licenseId: 0,
    licenseNo: '',
    editions: [] as LicenseEditionRecord[],
    plugins: [] as LicensePluginRecord[],
    logs: [] as LicenseOperationRecord[],
    catalog: [] as LicenseCatalogOption[],
    giftKey: '',
    giftPeriod: 'permanent' as CommercialPeriod,
    gifting: false
  })

  const handleGrantCommercial = (row: LicenseItem) => {
    grantDialog.id = row.id
    grantDialog.label = row.domain || row.licenseNo || String(row.id)
    grantDialog.period = 'permanent'
    grantDialog.visible = true
  }

  const submitGrantCommercial = async () => {
    grantDialog.submitting = true
    try {
      await fetchGrantCommercialEdition(grantDialog.id, grantDialog.period)
      ElMessage.success('已开通商业版')
      grantDialog.visible = false
      refreshData()
      window.dispatchEvent(new Event('store-account-refresh'))
    } catch (error) {
      showCaughtError(error, '开通失败')
    } finally {
      grantDialog.submitting = false
    }
  }

  async function copyCell(text: string) {
    const value = (text || '').trim()
    if (!value) return
    try {
      await navigator.clipboard.writeText(value)
      ElMessage.success('已复制')
    } catch {
      ElMessage.info(value)
    }
  }

  const loadLicenseDetail = async () => {
    detail.loading = true
    try {
      const data = await fetchLicenseCommercialDetail(detail.licenseId)
      detail.licenseNo = data?.licenseNo || detail.licenseNo
      detail.editions = data?.editions || []
      detail.plugins = data?.plugins || []
      detail.logs = data?.logs || []
      detail.catalog = data?.catalog || []
    } catch (error) {
      showCaughtError(error, '读取授权详情失败')
    } finally {
      detail.loading = false
    }
  }

  const openLicenseDetail = (row: LicenseItem) => {
    detail.licenseId = row.id
    detail.licenseNo = row.licenseNo || ''
    detail.giftKey = ''
    detail.giftPeriod = 'permanent'
    detail.visible = true
    void loadLicenseDetail()
  }

  const submitGiftPlugin = async () => {
    const [itemKind, itemId] = detail.giftKey.split(':')
    if (!itemKind || !itemId) {
      ElMessage.info('请选择要赠送的插件或模板')
      return
    }
    detail.gifting = true
    try {
      await fetchGiftLicensePlugin(detail.licenseId, {
        itemKind,
        itemId,
        period: detail.giftPeriod
      })
      ElMessage.success('已赠送')
      await loadLicenseDetail()
    } catch (error) {
      showCaughtError(error, '赠送失败')
    } finally {
      detail.gifting = false
    }
  }

  const revokePlugin = async (entitlementId: number, itemId: string) => {
    try {
      const { value } = await appPrompt(`撤销「${itemId}」后，这条插件权益立即失效`, '撤销插件', {
        inputPlaceholder: '填写原因，可留空',
        confirmButtonText: '撤销',
        cancelButtonText: '取消'
      })
      await fetchRevokeLicensePlugin(detail.licenseId, entitlementId, value || '')
      ElMessage.success('已撤销')
      await loadLicenseDetail()
    } catch (error) {
      if (error !== 'cancel' && error !== 'close') showCaughtError(error, '撤销失败')
    }
  }

  const handleRevokeCommercial = async (row: LicenseItem) => {
    try {
      const { value } = await appPrompt('请填写吊销原因', '吊销商业版', {
        inputPlaceholder: '例如：退款',
        confirmButtonText: '吊销',
        cancelButtonText: '取消'
      })
      await fetchRevokeCommercialEdition(row.id, value || '')
      ElMessage.success('已吊销商业版')
      refreshData()
      window.dispatchEvent(new Event('store-account-refresh'))
    } catch (error) {
      if (error !== 'cancel' && error !== 'close') showCaughtError(error, '吊销失败')
    }
  }

  const handleToggle = async (row: LicenseItem) => {
    const newStatus = row.status === 'active' ? 'disabled' : 'active'
    const action = row.status === 'active' ? '禁用' : '启用'
    try {
      await appConfirm(`确定${action}该授权？`, '提示', { type: 'warning' })
      await fetchToggleLicense(row.id, newStatus)
      ElMessage.success(`${action}成功`)
      refreshUpdate()
      window.dispatchEvent(new Event('store-account-refresh'))
    } catch (error) {
      if (error !== 'cancel' && error !== 'close') {
        console.error(`[LicenseList] ${action}失败:`, error)
      }
    }
  }

  const handleDelete = async (row: LicenseItem) => {
    try {
      const warning = row.commercialActive
        ? '该授权带有商业版，删除后对应站点将失去商业版'
        : '确定删除该授权？删除后不可恢复'
      await appConfirm(warning, '警告', { type: 'error' })
      await fetchDeleteLicense(row.id)
      ElMessage.success('删除成功')
      refreshRemove()
      window.dispatchEvent(new Event('store-account-refresh'))
    } catch (error) {
      if (error !== 'cancel' && error !== 'close') {
        console.error('[LicenseList] 删除失败:', error)
      }
    }
  }

  // ==================== 密钥站点管理 ====================

  const openSiteDialog = async (row: LicenseItem) => {
    siteDialog.licenseId = Number(row.id)
    siteDialog.licenseNo = row.licenseNo || ''
    siteDialog.maxSites = Number(row.maxSites) || 0
    siteDialog.visible = true
    await loadLicenseSites()
  }

  const loadLicenseSites = async () => {
    siteDialog.loading = true
    try {
      const data = await fetchLicenseSites(siteDialog.licenseId)
      siteDialog.list = data?.list || []
      if (data?.maxSites !== undefined) siteDialog.maxSites = Number(data.maxSites)
    } catch (error) {
      console.error('[LicenseList] 加载绑定站点失败:', error)
      showCaughtError(error, '加载绑定站点失败')
    } finally {
      siteDialog.loading = false
    }
  }

  const handleUnbindSite = async (row: LicenseSiteItem) => {
    try {
      await appConfirm(`确定解绑站点「${row.target}」？解绑后名额立即释放。`, '提示', {
        type: 'warning'
      })
      await fetchUnbindLicenseSite(siteDialog.licenseId, row.id)
      ElMessage.success('解绑成功')
      await loadLicenseSites()
      refreshUpdate()
    } catch (error) {
      if (error !== 'cancel' && error !== 'close') {
        console.error('[LicenseList] 解绑失败:', error)
      }
    }
  }

  const handleSubmit = async () => {
    if (submitting.value) return
    const valid = await formRef.value?.validate().catch(() => false)
    if (!valid) return

    submitting.value = true
    try {
      if (isEdit.value) {
        await fetchUpdateLicense(formData.id, {
          appId: Number(formData.appId),
          type: formData.type,
          domain: formData.domain,
          expireAt: formData.expireAt,
          remark: formData.remark
        })
        ElMessage.success('编辑成功')
      } else {
        await fetchCreateLicense({
          appId: Number(formData.appId),
          planId: Number(formData.planId),
          ownerType: formData.ownerType,
          ownerId: formData.ownerId,
          type: formData.type,
          domain: formData.domain,
          remark: formData.remark
        })
        ElMessage.success('新增成功')
      }
      dialogVisible.value = false
      if (isEdit.value) {
        refreshUpdate()
      } else {
        refreshCreate()
      }
    } catch (e) {
      console.error('[LicenseList] 提交失败:', e)
    } finally {
      submitting.value = false
    }
  }

  onBeforeUnmount(() => {
    if (ownerSearchTimer) clearTimeout(ownerSearchTimer)
    ownerSearchSequence++
    planRequestSequence++
  })
</script>

<style scoped lang="scss">
  .license-list-page {
    .mb-3 {
      margin-bottom: 12px;
    }

    .form-tip {
      margin-top: 4px;
      font-size: 12px;
      line-height: 18px;
      color: var(--el-text-color-secondary);
    }

    .quota-current {
      margin: 0 0 12px;
    }

    .quota-input {
      margin-top: 12px;
    }

    .owner-cell {
      display: flex;
      gap: 8px;
      align-items: center;
      min-width: 0;

      span:last-child {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }
    }

    .domain-cell__value {
      display: block;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .entitlement-drawer h4 {
      margin: 16px 0 8px;
      font-size: 14px;
    }

    .entitlement-scroll {
      overflow-x: auto;
    }

    .gift-row {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      align-items: center;
      margin-bottom: 8px;
    }

    .gift-row .el-select {
      flex: 1 1 220px;
      min-width: 180px;
    }

    .cell-copy {
      display: block;
      max-width: 100%;
      padding: 0;
      overflow: hidden;
      font: inherit;
      color: inherit;
      text-align: left;
      text-overflow: ellipsis;
      white-space: nowrap;
      cursor: pointer;
      background: transparent;
      border: 0;
    }

    .text-secondary {
      color: var(--el-text-color-secondary);
    }
  }
</style>
