<template>
  <div class="panel-licenses">
    <!-- 授权列表卡片：筛选工具栏 + 表格 + 分页 -->
    <div class="art-card licenses-card">
      <div class="licenses-toolbar">
        <div class="toolbar-filters">
          <el-input
            v-model="searchForm.keyword"
            placeholder="搜索域名 / IP / 密钥"
            clearable
            class="filter-keyword"
            @keyup.enter="handleSearch"
          >
            <template #prefix>
              <iconify-icon icon="ri:search-line" width="15" />
            </template>
          </el-input>
          <el-select
            v-model="searchForm.appId"
            placeholder="全部应用"
            clearable
            class="filter-select"
          >
            <el-option v-for="app in appList" :key="app.id" :label="app.name" :value="app.id" />
          </el-select>
          <el-select
            v-model="searchForm.status"
            placeholder="全部状态"
            clearable
            class="filter-select status-select"
          >
            <el-option label="正常" value="active" />
            <el-option label="即将到期" value="expiring" />
            <el-option label="已过期" value="expired" />
          </el-select>
          <el-button type="primary" @click="handleSearch">查询</el-button>
          <el-button @click="handleReset">重置</el-button>
        </div>
        <el-button type="primary" class="redeem-btn" @click="openRedeemDialog">
          <iconify-icon icon="ri:ticket-2-line" width="15" />
          兑换卡密
        </el-button>
      </div>

      <el-table :data="tableData" stripe v-loading="loading" class="licenses-table">
        <el-table-column label="域名/IP/密钥" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <div class="target-cell">
              <span class="target-icon" :class="`target-icon-${row.type}`">
                <iconify-icon :icon="typeIconMap[row.type] || 'ri:global-line'" width="15" />
              </span>
              <el-tag v-if="row.bindingPending" type="warning" size="small">未绑定</el-tag>
              <span v-else class="target-value" :class="{ mono: row.type !== 'domain' }">{{
                row.domain || '--'
              }}</span>
              <span v-if="row.type === 'key'" class="bound-count">
                已绑定 {{ row.boundSites ?? 0 }}{{ Number(row.maxSites) ? ` / ${row.maxSites}` : '' }}
              </span>
              <span class="change-quota">{{ freeChangeText(row) }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="appName" label="应用" width="120" show-overflow-tooltip />
        <el-table-column prop="typeLabel" label="类型" width="90">
          <template #default="{ row }">
            <el-tag :type="typeTagMap[row.type]" size="small" effect="light">{{
              row.typeLabel
            }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="statusLabel" label="状态" width="100" align="center">
          <template #default="{ row }">
            <BizStatusTag
              domain="license"
              :status="row.status"
              :label="row.statusLabel"
              size="small"
            />
          </template>
        </el-table-column>
        <el-table-column prop="expireAt" label="到期时间" width="130" />
        <el-table-column prop="createdAt" label="开通时间" width="130" />
        <el-table-column prop="source" label="来源" width="110">
          <template #default="{ row }">
            <span class="source-text">{{ row.source }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" min-width="220" fixed="right" align="center">
          <template #default="{ row }">
            <div class="row-actions">
              <template v-if="isDomainLicense(row.type)">
                <el-button
                  v-if="row.bindingPending"
                  link
                  type="primary"
                  size="small"
                  @click="openEditDialog(row)"
                >
                  绑定
                </el-button>
                <template v-else>
                  <el-button link type="primary" size="small" @click="openEditDialog(row)">
                    更换
                  </el-button>
                  <el-button link type="danger" size="small" @click="unbindDomain(row)">
                    解绑
                  </el-button>
                </template>
              </template>
              <template v-else-if="row.type === 'key'">
                <el-button link type="primary" size="small" @click="openSiteDialog(row, 'bind')">
                  绑定
                </el-button>
                <el-button link type="primary" size="small" @click="openEditDialog(row)">
                  更换
                </el-button>
                <el-button link type="danger" size="small" @click="openSiteDialog(row, 'unbind')">
                  解绑
                </el-button>
              </template>
              <el-button link type="primary" size="small" @click="openVersionsDialog(row)">
                版本下载
              </el-button>
            </div>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="暂无授权记录" :image-size="80" />
        </template>
      </el-table>

      <div class="pagination-wrapper">
        <el-pagination
          v-model:current-page="pagination.page"
          v-model:page-size="pagination.pageSize"
          :page-sizes="[10, 20, 50]"
          :total="pagination.total"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="fetchList"
          @size-change="handleSizeChange"
        />
      </div>
    </div>

    <el-dialog
      v-model="editDialog.visible"
      :title="editDialog.type === 'key' ? '更换密钥' : editDialog.bindingPending ? '绑定域名' : '更换域名'"
      width="min(460px, 92vw)"
      destroy-on-close
    >
      <el-alert
        v-if="editDialog.serverError"
        :title="editDialog.serverError"
        type="error"
        show-icon
        :closable="false"
        class="bind-error"
      />
      <el-form label-width="86px" @submit.prevent="submitLicenseEdit">
        <el-form-item label="授权编号">
          <el-input :model-value="editDialog.licenseNo" disabled />
        </el-form-item>
        <el-form-item label="应用">
          <el-input :model-value="editDialog.appName" disabled />
        </el-form-item>
        <el-form-item label="授权类型">
          <el-input :model-value="editDialog.typeLabel" disabled />
        </el-form-item>
        <el-form-item :label="editTargetLabel" :error="editFieldError">
          <div class="target-editor">
            <el-input
              v-model="editDialog.target"
              :placeholder="editTargetPlaceholder"
              :disabled="editDialog.type === 'key'"
              maxlength="255"
              :show-word-limit="editDialog.type !== 'key'"
              clearable
            />
            <el-button
              v-if="editDialog.type === 'key'"
              type="primary"
              plain
              :loading="editDialog.refreshing"
              title="生成新的16位密钥"
              @click="refreshLicenseKey"
            >
              <iconify-icon icon="ri:refresh-line" width="16" />
              刷新
            </el-button>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editDialog.visible = false">取消</el-button>
        <el-button
          v-if="editDialog.type !== 'key'"
          type="primary"
          :loading="editDialog.submitting"
          :disabled="!!editFieldError"
          @click="submitLicenseEdit"
        >
          {{ editDialog.bindingPending ? '绑定' : '更换' }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="siteDialog.visible" title="密钥绑定站点" width="min(680px, 92vw)" destroy-on-close>
      <el-alert
        v-if="siteDialog.maxSites > 0"
        :title="`当前已绑定 ${siteDialog.list.length} / ${siteDialog.maxSites} 个站点，达到上限后新站点验证会被拒绝，可解绑释放名额。`"
        type="info"
        show-icon
        :closable="false"
        class="mb-3"
      />
      <el-alert
        v-else
        title="该密钥不限制站点数量。"
        type="info"
        show-icon
        :closable="false"
        class="mb-3"
      />
      <el-table
        :data="siteDialog.list"
        size="small"
        v-loading="siteDialog.loading"
        max-height="360"
      >
        <el-table-column label="类型" width="80">
          <template #default="{ row }">
            <el-tag :type="row.targetType === 'ip' ? 'warning' : undefined" size="small">
              {{ row.targetType === 'ip' ? 'IP' : '域名' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="target" label="站点" min-width="160" show-overflow-tooltip />
        <el-table-column prop="serverIp" label="最近服务器IP" width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ row.serverIp || '--' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="firstSeenAt" label="首次绑定" width="160" />
        <el-table-column prop="lastSeenAt" label="最近验证" width="160" />
        <el-table-column label="操作" min-width="220" align="center">
          <template #default="{ row }">
            <div class="site-replace">
              <el-input v-model="siteReplace[row.id]" size="small" placeholder="新域名或 IP" />
              <el-button size="small" type="primary" plain @click="replaceSite(row)">更换</el-button>
              <el-button link type="danger" size="small" @click="handleUnbindSite(row)">解绑</el-button>
            </div>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="暂无绑定站点" :image-size="60" />
        </template>
      </el-table>
    </el-dialog>

    <SiteChangePayDialog
      :visible="changePay.visible"
      :price="changePay.price"
      panel="agent"
      :license-id="changePay.licenseId"
      :action="changePay.action"
      :site-id="changePay.siteId"
      :target="changePay.target"
      @close="changePay.visible = false"
      @paid="onChangePaid"
    />

    <el-dialog v-model="redeemDialog.visible" title="兑换卡密" width="460px" destroy-on-close>
      <el-alert
        title="兑换后授权将归当前代理账号，不能代他人兑换或转赠。"
        type="info"
        show-icon
        :closable="false"
        class="redeem-alert"
      />
      <el-form label-width="72px" @submit.prevent="submitRedeem">
        <el-form-item label="卡密">
          <el-input
            v-model="redeemDialog.cardCode"
            placeholder="请输入卡密"
            maxlength="64"
            clearable
            autocomplete="off"
            @keyup.enter="submitRedeem"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="redeemDialog.visible = false">取消</el-button>
        <el-button type="primary" :loading="redeemDialog.submitting" @click="submitRedeem"
          >确认兑换</el-button
        >
      </template>
    </el-dialog>

    <el-dialog v-model="redeemResult.visible" title="兑换结果" width="520px" destroy-on-close>
      <el-result icon="success" :title="redeemResult.idempotent ? '该卡密已兑换' : '卡密兑换成功'">
        <template #sub-title>
          <div class="redeem-summary">
            <div>{{ redeemResult.appName }} · {{ redeemResult.planName }}</div>
            <div>授权编号：{{ redeemResult.licenseNo }}</div>
            <div>授权类型：{{ redeemResult.typeLabel }} 有效期至：{{ redeemResult.expireAt }}</div>
          </div>
        </template>
        <template #extra>
          <div v-if="redeemResult.type === 'key'" class="license-key-result">
            <BizCopySecret
              :value="redeemResult.licenseKey"
              default-visible
              copy-label="复制密钥"
              success-text="密钥已复制"
              fail-text="复制失败，请手动复制"
            />
          </div>
          <el-alert
            v-else
            title="授权尚未绑定目标，请在列表中点击“绑定目标”后使用。"
            type="warning"
            show-icon
            :closable="false"
          />
        </template>
      </el-result>
      <template #footer>
        <el-button type="primary" @click="closeRedeemResult">完成</el-button>
      </template>
    </el-dialog>

    <LicenseVersionsDialog
      ref="versionsDialogRef"
      api-prefix="/api/agent-panel"
      token-key="agent_panel_token"
    />
  </div>
</template>

<script setup lang="ts">
  import { ref, reactive, onMounted, computed } from 'vue'
  import { useRoute, useRouter } from 'vue-router'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { licenseTargetError } from '@/utils/license-target'
  import SiteChangePayDialog from '@/components/core/pay/SiteChangePayDialog.vue'
  import { Icon as IconifyIcon } from '@iconify/vue'
  import axios from 'axios'
  import LicenseVersionsDialog from '@/components/core/panels/LicenseVersionsDialog.vue'

  const route = useRoute()
  const router = useRouter()

  const loading = ref(false)
  const searchForm = reactive({ keyword: '', appId: '', status: '' })
  const pagination = reactive({ page: 1, pageSize: 10, total: 0 })
  const appList = ref<{ id: number; name: string }[]>([])
  const tableData = ref<any[]>([])
  const editDialog = reactive({
    visible: false,
    submitting: false,
    refreshing: false,
    bindingPending: false,
    id: 0,
    licenseNo: '',
    appName: '',
    type: 'domain',
    typeLabel: '',
    target: '',
    serverError: ''
  })

  const siteDialog = reactive({
    visible: false,
    loading: false,
    licenseId: 0,
    licenseNo: '',
    maxSites: 0,
    list: [] as any[]
  })

  const versionsDialogRef = ref<InstanceType<typeof LicenseVersionsDialog>>()

  function openVersionsDialog(row: any) {
    versionsDialogRef.value?.open({ id: row.id, appName: row.appName })
  }

  const redeemDialog = reactive({
    visible: false,
    submitting: false,
    cardCode: ''
  })
  const redeemResult = reactive({
    visible: false,
    licenseNo: '',
    appName: '',
    planName: '',
    type: '',
    typeLabel: '',
    licenseKey: '',
    expireAt: '',
    idempotent: false
  })

  const typeTagMap: Record<
    string,
    'primary' | 'success' | 'warning' | 'info' | 'danger' | undefined
  > = { domain: undefined, wildcard: 'success', ip: 'warning', key: 'info' }
  const typeIconMap: Record<string, string> = {
    domain: 'ri:global-line',
    wildcard: 'ri:asterisk',
    ip: 'ri:router-line',
    key: 'ri:key-2-line'
  }
  const editTargetLabel = computed(() => {
    if (editDialog.type === 'key') return '授权密钥'
    if (editDialog.type === 'ip') return 'IP地址'
    return editDialog.type === 'wildcard' ? '泛域名' : '单域名'
  })
  const editTargetPlaceholder = computed(() => {
    const placeholders: Record<string, string> = {
      domain: 'example.com',
      wildcard: '*.example.com',
      ip: '192.168.1.1',
      key: '请输入授权密钥'
    }
    return placeholders[editDialog.type] || ''
  })
  const editFieldError = computed(() => {
    if (!editDialog.visible || editDialog.type === 'key') return ''
    const value = editDialog.target.trim()
    if (!value) return ''
    return licenseTargetError(editDialog.type, value)
  })
  const domainLicenseTypes = new Set(['domain', 'wildcard', 'ip'])

  function isDomainLicense(type: string) {
    return domainLicenseTypes.has(type)
  }

  const siteReplace = reactive<Record<number, string>>({})
  const changePay = reactive({
    visible: false,
    price: 0,
    licenseId: 0,
    action: 'replace' as 'replace' | 'unbind',
    siteId: 0,
    target: ''
  })

  function freeChangeText(row: { freeSiteChanges?: number }) {
    const left = Number(row.freeSiteChanges)
    if (!Number.isFinite(left) || left < 0) return '剩余免费更换 不限'
    return `剩余免费更换 ${left} 次`
  }

  function openChangePay(payload: {
    price: number
    licenseId: number
    action: 'replace' | 'unbind'
    siteId?: number
    target?: string
  }) {
    changePay.price = payload.price
    changePay.licenseId = payload.licenseId
    changePay.action = payload.action
    changePay.siteId = payload.siteId || 0
    changePay.target = payload.target || ''
    changePay.visible = true
  }

  function onChangePaid() {
    fetchList()
    if (siteDialog.visible) fetchLicenseSites()
  }

  function getToken() {
    return localStorage.getItem('agent_panel_token') || ''
  }

  function authHeaders() {
    return { Authorization: `Bearer ${getToken()}` }
  }

  async function fetchApps() {
    try {
      const { data } = await axios.get('/api/agent-panel/apps', { headers: authHeaders() })
      if (data.code === 200) appList.value = data.data || []
    } catch {
      // 拉取应用列表失败时保持空列表
    }
  }

  async function fetchList() {
    loading.value = true
    try {
      const { data } = await axios.get('/api/agent-panel/licenses', {
        headers: authHeaders(),
        params: {
          keyword: searchForm.keyword || undefined,
          appId: searchForm.appId || undefined,
          status: searchForm.status || undefined,
          page: pagination.page,
          pageSize: pagination.pageSize
        }
      })
      if (data.code === 200) {
        tableData.value = data.data.list || []
        pagination.total = data.data.total || 0
      }
    } catch {
      // 拉取授权列表失败时保持原数据
    }
    loading.value = false
  }

  function handleSearch() {
    pagination.page = 1
    fetchList()
  }

  function handleReset() {
    searchForm.keyword = ''
    searchForm.appId = ''
    searchForm.status = ''
    handleSearch()
  }

  function handleSizeChange() {
    pagination.page = 1
    fetchList()
  }

  function openEditDialog(row: any) {
    Object.assign(editDialog, {
      visible: true,
      submitting: false,
      bindingPending: !!row.bindingPending,
      id: Number(row.id),
      licenseNo: row.licenseNo || '',
      appName: row.appName || '',
      type: row.type,
      typeLabel: row.typeLabel || '',
      target: row.bindingPending ? '' : row.domain || '',
      serverError: ''
    })
  }

  async function submitLicenseEdit() {
    const target = editDialog.target.trim()
    const localError = licenseTargetError(editDialog.type, target)
    if (editDialog.type !== 'key' && localError) {
      editDialog.serverError = localError
      return
    }

    editDialog.submitting = true
    editDialog.serverError = ''
    try {
      const { data } = await axios.put(
        `/api/agent-panel/licenses/${editDialog.id}`,
        {
          type: editDialog.type,
          target
        },
        { headers: authHeaders() }
      )
      if (data.code === 200) {
        ElMessage.success(data.msg || (editDialog.bindingPending ? '已绑定域名' : '已更换域名'))
        editDialog.visible = false
        await fetchList()
      } else if (data.code === 402 && data.data?.needPay) {
        editDialog.visible = false
        openChangePay({
          price: Number(data.data.price),
          licenseId: editDialog.id,
          action: 'replace',
          target
        })
      } else {
        editDialog.serverError = data.msg || '更新失败'
      }
    } catch {
      editDialog.serverError = '更新失败，请稍后重试'
    } finally {
      editDialog.submitting = false
    }
  }

  async function unbindDomain(row: any) {
    try {
      await ElMessageBox.confirm(
        `确定解绑「${row.domain}」？解绑后需要重新绑定才能使用。`,
        '解绑域名',
        { type: 'warning', confirmButtonText: '解绑', cancelButtonText: '取消' }
      )
      const { data } = await axios.put(
        `/api/agent-panel/licenses/${row.id}`,
        { unbind: true },
        { headers: authHeaders() }
      )
      if (data.code === 200) {
        ElMessage.success(data.msg || '已解绑域名')
        await fetchList()
      } else if (data.code === 402 && data.data?.needPay) {
        openChangePay({
          price: Number(data.data.price),
          licenseId: row.id,
          action: 'unbind'
        })
      } else {
        ElMessage.error(data.msg || '解绑失败')
      }
    } catch (error) {
      if (error !== 'cancel' && error !== 'close') {
        ElMessage.error('解绑失败，请稍后重试')
      }
    }
  }

  async function refreshLicenseKey() {
    editDialog.refreshing = true
    try {
      const { data } = await axios.post(
        `/api/agent-panel/licenses/${editDialog.id}/refresh-key`,
        {},
        { headers: authHeaders() }
      )
      if (data.code === 200) {
        editDialog.target = data.data?.licenseKey || ''
        ElMessage.success(data.msg || '密钥已刷新')
        await fetchList()
      } else {
        ElMessage.error(data.msg || '刷新密钥失败')
      }
    } catch {
      ElMessage.error('刷新密钥失败，请稍后重试')
    } finally {
      editDialog.refreshing = false
    }
  }

  async function openSiteDialog(row: any, intent: 'bind' | 'unbind' = 'bind') {
    const bound = Number(row.boundSites) || 0
    const maxSites = Number(row.maxSites) || 0
    if (intent === 'bind' && maxSites > 0 && bound >= maxSites) {
      ElMessage.warning('授权已达到最大站点数')
    }
    siteDialog.licenseId = Number(row.id)
    siteDialog.licenseNo = row.licenseNo || ''
    siteDialog.maxSites = maxSites
    siteDialog.visible = true
    await fetchLicenseSites()
  }

  async function fetchLicenseSites() {
    siteDialog.loading = true
    try {
      const { data } = await axios.get(`/api/agent-panel/licenses/${siteDialog.licenseId}/sites`, {
        headers: authHeaders()
      })
      if (data.code === 200) {
        siteDialog.list = data.data?.list || []
        if (data.data?.maxSites !== undefined) siteDialog.maxSites = Number(data.data.maxSites)
      } else {
        ElMessage.error(data.msg || '加载绑定站点失败')
      }
    } catch {
      ElMessage.error('加载绑定站点失败')
    } finally {
      siteDialog.loading = false
    }
  }

  async function replaceSite(row: any) {
    const target = (siteReplace[row.id] || '').trim()
    if (!target) {
      ElMessage.warning('请填写新的域名或 IP')
      return
    }
    const { data } = await axios.post(
      `/api/agent-panel/licenses/${siteDialog.licenseId}/sites/${row.id}/replace`,
      { target },
      { headers: authHeaders() }
    )
    if (data.code === 200) {
      ElMessage.success(data.msg || '已更换')
      siteReplace[row.id] = ''
      await fetchLicenseSites()
      await fetchList()
    } else if (data.code === 402 && data.data?.needPay) {
      openChangePay({
        price: Number(data.data.price),
        licenseId: siteDialog.licenseId,
        action: 'replace',
        siteId: row.id,
        target
      })
    } else {
      ElMessage.error(data.msg || '更换失败')
    }
  }

  async function handleUnbindSite(row: any) {
    try {
      await ElMessageBox.confirm(`确定解绑站点「${row.target}」？解绑后名额立即释放。`, '提示', {
        type: 'warning'
      })
      const { data } = await axios.delete(
        `/api/agent-panel/licenses/${siteDialog.licenseId}/sites/${row.id}`,
        { headers: authHeaders() }
      )
      if (data.code === 200) {
        ElMessage.success(data.msg || '解绑成功')
        await fetchLicenseSites()
        await fetchList()
      } else if (data.code === 402 && data.data?.needPay) {
        openChangePay({
          price: Number(data.data.price),
          licenseId: siteDialog.licenseId,
          action: 'unbind',
          siteId: row.id
        })
      } else {
        ElMessage.error(data.msg || '解绑失败')
      }
    } catch (error) {
      if (error !== 'cancel' && error !== 'close') {
        ElMessage.error('解绑失败，请稍后重试')
      }
    }
  }

  function openRedeemDialog() {
    redeemDialog.cardCode = ''
    redeemDialog.submitting = false
    redeemDialog.visible = true
  }

  async function submitRedeem() {
    if (redeemDialog.submitting) return
    const cardCode = redeemDialog.cardCode.trim()
    if (!cardCode) {
      ElMessage.warning('请输入卡密')
      return
    }

    redeemDialog.submitting = true
    try {
      const { data } = await axios.post(
        '/api/agent-panel/cards/redeem',
        { cardCode },
        { headers: authHeaders() }
      )
      if (data.code !== 200) {
        ElMessage.error(data.msg || '兑换失败')
        return
      }
      redeemDialog.visible = false
      Object.assign(redeemResult, {
        ...data.data,
        visible: true,
        licenseKey: data.data?.licenseKey || '',
        idempotent: data.data?.idempotent === true
      })
      await fetchList()
    } catch {
      ElMessage.error('兑换失败，请稍后重试')
    } finally {
      redeemDialog.submitting = false
    }
  }

  function closeRedeemResult() {
    redeemResult.visible = false
  }

  onMounted(async () => {
    fetchApps()
    await fetchList()
    const bind = typeof route.query.bind === 'string' ? route.query.bind : ''
    if (!bind) return
    const row = tableData.value.find((item) => String(item.id) === bind)
    if (row?.type === 'key') openSiteDialog(row, 'bind')
    else if (row && isDomainLicense(row.type)) openEditDialog(row)
    router.replace({ path: '/agent-panel/licenses' })
  })
</script>

<style scoped lang="scss">
  .panel-licenses {
    .art-card {
      overflow: hidden;
      background: var(--el-bg-color);
      border-radius: 12px !important;
    }
  }

  .mb-3 {
    margin-bottom: 12px;
  }

  .text-secondary {
    color: var(--el-text-color-secondary);
  }

  .licenses-card {
    padding: 20px;
  }

  // 工具栏：左侧筛选，右侧兑换入口
  .licenses-toolbar {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 16px;
  }

  .toolbar-filters {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    align-items: center;
  }

  .filter-keyword {
    width: 220px;
  }

  .filter-select {
    width: 130px;
  }

  .status-select {
    width: 120px;
  }

  .redeem-btn {
    display: inline-flex;
    gap: 5px;
    align-items: center;
  }

  // 表格细节
  .licenses-table {
    :deep(.el-table__header th) {
      font-weight: 600;
      color: var(--el-text-color-secondary);
      background: var(--el-fill-color-light);
    }
  }

  .target-cell {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
    min-width: 0;
  }

  .target-icon {
    display: inline-flex;
    flex-shrink: 0;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    color: var(--el-color-primary);
    background: var(--el-color-primary-light-9);
    border-radius: 8px;
  }

  .target-icon-wildcard {
    color: var(--el-color-success);
    background: var(--el-color-success-light-9);
  }

  .target-icon-ip {
    color: var(--el-color-warning);
    background: var(--el-color-warning-light-9);
  }

  .target-icon-key {
    color: var(--el-color-info);
    background: var(--el-color-info-light-9);
  }

  .target-value {
    overflow: hidden;
    color: var(--el-text-color-primary);
    text-overflow: ellipsis;
    white-space: nowrap;

    &.mono {
      font-family: 'Roboto Mono', monospace;
      font-size: 12px;
    }
  }

  .source-text {
    font-size: 12px;
    color: var(--el-text-color-secondary);
  }

  .pagination-wrapper {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
  }

  .target-editor {
    display: flex;
    gap: 8px;
    align-items: center;
    width: 100%;
  }

  .target-editor .el-button {
    flex: 0 0 auto;
  }

  .target-editor .el-button :deep(.el-icon) {
    margin-right: 4px;
  }

  .redeem-alert {
    margin-bottom: 18px;
  }

  .redeem-summary {
    display: grid;
    gap: 6px;
    color: var(--el-text-color-regular);
  }

  .license-key-result {
    width: 100%;
    min-width: 360px;
  }

  .row-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 2px 8px;
    justify-content: center;
  }

  .change-quota {
    color: var(--el-text-color-secondary);
    font-size: 12px;
  }

  .site-replace {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    align-items: center;
    justify-content: flex-end;
  }

  .bound-count {
    flex-shrink: 0;
    font-size: 12px;
    color: var(--el-text-color-secondary);
  }

  .bind-error {
    margin-bottom: 12px;
  }

  @media (width <= 768px) {
    .licenses-card {
      padding: 14px;
    }

    .filter-keyword,
    .filter-select,
    .status-select {
      width: 100%;
    }

    .toolbar-filters {
      width: 100%;
    }

    .redeem-btn {
      justify-content: center;
      width: 100%;
    }

    .pagination-wrapper {
      justify-content: center;
    }

    .license-key-result {
      min-width: 0;
      max-width: 100%;
    }

    .row-actions {
      justify-content: flex-start;
    }
  }
</style>
