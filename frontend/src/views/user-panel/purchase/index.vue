<template>
  <div class="user-purchase">
    <header class="page-head">
      <h2>购买授权</h2>
      <p>选择应用和套餐，填写授权信息后支付，授权会即时开通。</p>
    </header>

    <el-card
      v-if="step === 4 && purchaseResult"
      shadow="never"
      class="step-card art-card success-card"
    >
      <div class="success-hero">
        <div class="success-icon-wrap">
          <iconify-icon icon="ri:checkbox-circle-fill" width="34" />
        </div>
        <div class="success-title-block">
          <h3>授权已生成</h3>
          <p>购买完成，授权已经即时生效</p>
        </div>
        <BizStatusTag domain="license" status="active" label="已生效" size="small" />
      </div>

      <div class="license-no-card">
        <span class="license-no-label">授权编号</span>
        <span class="license-no-value">{{ purchaseResult?.licenseNo }}</span>
      </div>

      <div class="success-info-grid">
        <div class="success-info-item">
          <span class="info-label">应用</span>
          <span class="info-value">{{ purchaseResult?.appName }}</span>
        </div>
        <div class="success-info-item">
          <span class="info-label">套餐</span>
          <span class="info-value">{{ purchaseResult?.planName }}</span>
        </div>
        <div class="success-info-item">
          <span class="info-label">授权时长</span>
          <span class="info-value">{{ formatDuration(purchaseResult?.durationDays) }}</span>
        </div>
        <div class="success-info-item">
          <span class="info-label">剩余余额</span>
          <span class="info-value">¥{{ userBalance.toFixed(2) }}</span>
        </div>
      </div>

      <div class="success-amount-card">
        <span>本次扣款</span>
        <strong>¥{{ Number(purchaseResult?.cost || 0).toFixed(2) }}</strong>
      </div>

      <div class="success-actions">
        <el-button size="large" @click="goToLicenses">查看授权</el-button>
        <el-button type="primary" size="large" @click="resetFlow">继续购买</el-button>
      </div>
    </el-card>

    <el-card
      v-else-if="appsLoading && !appList.length"
      v-loading="true"
      shadow="never"
      class="step-card art-card state-card"
    >
      <div class="state-placeholder">正在加载可购买应用</div>
    </el-card>

    <el-card
      v-else-if="appsError && !appList.length"
      shadow="never"
      class="step-card art-card state-card"
    >
      <el-empty description="应用列表加载失败，请稍后重试" :image-size="80">
        <el-button type="primary" @click="retryApps">重新加载</el-button>
      </el-empty>
    </el-card>

    <el-card v-else-if="!appList.length" shadow="never" class="step-card art-card state-card">
      <el-empty description="暂无可购买应用，请联系管理员先启用应用和套餐" :image-size="80" />
    </el-card>

    <template v-else>
      <el-card shadow="never" class="step-card art-card" :class="{ 'is-done': !!formData.appId }">
        <template #header>
          <div class="step-card-head">
            <span class="step-index">1</span>
            <div>
              <h3>选择应用</h3>
              <p>选择要开通授权的应用</p>
            </div>
          </div>
        </template>

        <BizAppSelect
          :key="appSelectKey"
          v-model="formData.appId"
          :loader="loadPurchaseAppOptions"
          placeholder="请选择要开通的应用"
          class="purchase-app-select"
          @change="onAppChange"
        />

        <div v-if="selectedApp" class="app-summary">
          <div class="app-summary-icon">
            <iconify-icon :icon="selectedApp.icon || 'ri:apps-line'" width="22" />
          </div>
          <div class="app-summary-body">
            <div class="app-summary-title">
              <strong>{{ selectedApp.name }}</strong>
              <BizStatusTag
                v-if="hasPromotion(selectedApp)"
                domain="campaign"
                status="active"
                label="限时活动"
                size="small"
              />
            </div>
            <p>{{ selectedApp.desc }}</p>
          </div>
          <div class="app-summary-price">
            <span>¥{{ minPlanPrice(selectedApp).toFixed(2) }}</span>
            <em>起</em>
          </div>
        </div>
      </el-card>

      <el-card shadow="never" class="step-card art-card" :class="{ 'is-done': !!formData.planId }">
        <template #header>
          <div class="step-card-head">
            <span class="step-index">2</span>
            <div>
              <h3>选择套餐</h3>
              <p>确认时长和价格，活动价以下单时为准</p>
            </div>
          </div>
        </template>

        <el-empty v-if="!formData.appId" description="请先选择应用" :image-size="72" />
        <el-empty
          v-else-if="!filteredAppPlans.length"
          description="当前授权类型暂无套餐，可在下一步更换类型"
          :image-size="72"
        />
        <div v-else class="plan-grid">
          <button
            v-for="plan in filteredAppPlans"
            :key="plan.id"
            type="button"
            class="plan-card"
            :class="{ active: formData.planId === plan.id, 'has-promo': plan.promotion }"
            @click="selectPlan(plan.id)"
          >
            <div class="plan-head">
              <span class="plan-name">{{ plan.name }}</span>
              <BizStatusTag
                v-if="plan.promotion"
                domain="campaign"
                status="active"
                :label="plan.promotion.name"
                size="small"
              />
            </div>
            <div class="plan-pricing">
              <span class="plan-currency">¥</span>
              <span class="plan-amount">{{ Number(plan.price).toFixed(2) }}</span>
              <span v-if="plan.promotion" class="plan-original">
                ¥{{ Number(plan.originalPrice).toFixed(2) }}
              </span>
            </div>
            <div class="plan-meta">
              <span class="plan-duration">{{ plan.durationText }}</span>
              <span v-if="plan.promotion" class="plan-time">
                {{ promoRuleText(plan.promotion) }} · 截止 {{ plan.promotion.endsAt }}
              </span>
              <span v-if="plan.purchaseLimit" class="plan-limit">
                {{ purchaseLimitText(plan.purchaseLimit) }}
              </span>
            </div>
          </button>
        </div>
      </el-card>

      <el-card shadow="never" class="step-card art-card" :class="{ 'is-done': infoReady }">
        <template #header>
          <div class="step-card-head">
            <span class="step-index">3</span>
            <div>
              <h3>填写授权信息</h3>
              <p>授权类型决定可用套餐，目标在支付前校验</p>
            </div>
          </div>
        </template>

        <div v-if="!formData.appId" class="section-muted">请先选择应用</div>
        <template v-else>
          <div class="field-block">
            <label class="field-label">授权类型</label>
            <div v-if="availableTypeOptions.length" class="type-options">
              <button
                v-for="item in availableTypeOptions"
                :key="item.value"
                type="button"
                class="type-chip"
                :class="{ active: formData.type === item.value }"
                @click="selectType(item.value)"
              >
                <iconify-icon :icon="item.icon" width="16" />
                <span>{{ item.label }}</span>
              </button>
            </div>
            <p v-else class="field-error">当前应用暂不支持自助开通</p>
          </div>

          <div class="field-block">
            <label class="field-label">{{ domainLabel }}</label>
            <el-input
              v-model="formData.domain"
              :disabled="formData.type === 'key'"
              :placeholder="domainPlaceholder"
              clearable
            >
              <template #prefix>
                <iconify-icon :icon="domainIcon" width="16" />
              </template>
            </el-input>
            <p v-if="targetFieldError" class="field-error">{{ targetFieldError }}</p>
            <p v-else-if="formData.type === 'key'" class="field-hint">密钥由系统在开通后自动生成</p>
          </div>
        </template>
      </el-card>

      <el-card shadow="never" class="step-card art-card">
        <template #header>
          <div class="step-card-head">
            <span class="step-index">4</span>
            <div>
              <h3>支付并提交</h3>
              <p>确认订单后选择余额或在线支付</p>
            </div>
          </div>
        </template>

        <div class="pay-layout">
          <div class="order-panel">
            <div class="order-row">
              <span>应用</span>
              <strong>{{ selectedApp?.name || '—' }}</strong>
            </div>
            <div class="order-row">
              <span>套餐</span>
              <strong>{{ selectedPlan?.name || '—' }}</strong>
            </div>
            <div class="order-row">
              <span>授权时长</span>
              <strong>{{ selectedPlan?.durationText || '—' }}</strong>
            </div>
            <div class="order-row">
              <span>授权类型</span>
              <strong>{{ typeLabels[formData.type] || '—' }}</strong>
            </div>
            <div class="order-row">
              <span>授权目标</span>
              <strong class="mono">{{ displayTarget }}</strong>
            </div>
            <div v-if="hasDiscount" class="order-row">
              <span>优惠</span>
              <strong class="discount">
                <template v-if="selectedPromotion">
                  {{ promoRuleText(selectedPromotion) }}，优惠 ¥{{ discountAmount.toFixed(2) }}
                </template>
                <template v-else>已优惠 ¥{{ discountAmount.toFixed(2) }}</template>
              </strong>
            </div>
            <div class="order-total">
              <span>应付金额</span>
              <div>
                <em v-if="hasDiscount">原价 ¥{{ originalPrice.toFixed(2) }}</em>
                <strong>¥{{ computedCost.toFixed(2) }}</strong>
              </div>
            </div>
          </div>

          <div class="method-panel">
            <label class="field-label">支付方式</label>
            <div class="method-options">
              <button
                v-for="option in payOptions"
                :key="option.code"
                type="button"
                class="method-item"
                :class="{ active: payMethod === option.code }"
                @click="payMethod = option.code"
              >
                <iconify-icon :icon="option.icon" width="22" :color="option.color" />
                <span class="method-label">{{ option.label }}</span>
                <template v-if="option.code === 'balance'">
                  <span class="method-balance">¥{{ userBalance.toFixed(2) }}</span>
                  <el-button size="small" @click.stop="openRechargeDialog">充值余额</el-button>
                </template>
              </button>
            </div>
            <el-alert
              v-if="balanceShort"
              type="warning"
              show-icon
              :closable="false"
              title="当前余额不足，无法使用余额支付"
              class="balance-alert"
            >
              <el-button link type="primary" @click="openRechargeDialog">立即充值</el-button>
            </el-alert>
          </div>
        </div>
      </el-card>

      <div class="purchase-bar">
        <div class="purchase-bar-summary">
          <div class="summary-line">
            <span>{{ selectedApp?.name || '未选择应用' }}</span>
            <span class="summary-dot">·</span>
            <span>{{ selectedPlan?.name || '未选择套餐' }}</span>
          </div>
          <div class="summary-price">
            <span v-if="hasDiscount" class="summary-original">¥{{ originalPrice.toFixed(2) }}</span>
            <strong>¥{{ computedCost.toFixed(2) }}</strong>
          </div>
          <p class="summary-hint">{{ purchaseHint }}</p>
        </div>
        <el-button
          type="primary"
          size="large"
          class="purchase-submit"
          :loading="purchasing"
          :disabled="purchaseDisabled"
          @click="handlePurchase"
        >
          <iconify-icon v-if="!purchasing" icon="ri:secure-payment-line" width="18" />
          {{ purchaseButtonText }}
        </el-button>
      </div>
    </template>

    <el-dialog v-model="rechargeDialog.visible" title="余额充值" width="420px" append-to-body>
      <div class="recharge-dialog-body">
        <label class="field-label">充值金额</label>
        <el-input-number
          v-model="rechargeDialog.amount"
          :min="0.01"
          :max="1000000"
          :precision="2"
          :step="10"
          controls-position="right"
          class="recharge-amount-input"
        />
        <div class="quick-amounts">
          <button
            v-for="amount in quickRechargeAmounts"
            :key="amount"
            type="button"
            @click="rechargeDialog.amount = amount"
          >
            ¥{{ amount }}
          </button>
        </div>

        <label class="field-label recharge-pay-label">支付方式</label>
        <el-radio-group
          v-if="rechargeMethodOptions.length > 0"
          v-model="rechargeDialog.payType"
          class="recharge-pay-types"
        >
          <el-radio-button
            v-for="item in rechargeMethodOptions"
            :key="item.code"
            :label="item.code"
          >
            {{ item.label }}
          </el-radio-button>
        </el-radio-group>
        <p v-else class="recharge-pay-empty">支付通道未开启，请联系管理员</p>
        <p class="recharge-tip">支付成功后余额自动入账，再用余额购买授权。</p>
      </div>
      <template #footer>
        <el-button @click="rechargeDialog.visible = false">取消</el-button>
        <el-button
          type="primary"
          :loading="rechargeDialog.submitting"
          :disabled="rechargeMethodOptions.length === 0"
          @click="submitRecharge"
        >
          去支付
        </el-button>
      </template>
    </el-dialog>

    <PayQrDialog
      :visible="qrCheckout.visible"
      :qr-code="qrCheckout.qrCode"
      :amount="qrCheckout.amount"
      :order-no="qrCheckout.orderNo"
      @close="qrCheckout.visible = false"
    />
  </div>
</template>

<script setup lang="ts">
  import { ref, reactive, computed, onMounted } from 'vue'
  import { useRoute, useRouter } from 'vue-router'
  import { ElMessage } from 'element-plus'
  import { Icon as IconifyIcon } from '@iconify/vue'
  import axios from 'axios'
  import PayQrDialog from '@/components/core/pay/PayQrDialog.vue'
  import { isQrCheckout } from '@/utils/checkout'

  const router = useRouter()
  const route = useRoute()
  const step = ref(1)
  const payMethod = ref('balance')
  const payOptions = ref<any[]>([
    { code: 'balance', label: '余额支付', icon: 'ri:wallet-3-line', color: '#2e7d32' }
  ])
  const userBalance = ref(0)
  const purchasing = ref(false)
  const purchaseResult = ref<any>(null)
  const rechargeDialog = reactive({
    visible: false,
    submitting: false,
    amount: 50,
    payType: 'alipay'
  })
  const payTypeLabels: Record<string, string> = {
    alipay: '支付宝',
    wxpay: '微信',
    qqpay: 'QQ支付'
  }
  const rechargeOptions = reactive({
    payTypes: [] as string[],
    defaultType: 'alipay',
    options: [] as Array<{ code: string; label: string; payType?: string }>
  })
  const qrCheckout = reactive({
    visible: false,
    qrCode: '',
    orderNo: '',
    amount: '' as string | number
  })
  const rechargeMethodOptions = computed(() => {
    if (rechargeOptions.options.length > 0) {
      return rechargeOptions.options.map((item) => ({
        code: item.code,
        label: item.label || payTypeLabels[item.payType || ''] || item.code
      }))
    }
    return rechargeOptions.payTypes.map((code) => ({
      code,
      label: payTypeLabels[code] || code
    }))
  })
  const quickRechargeAmounts = [10, 30, 50, 100]
  const rechargeOrderStorageKey = 'user_panel_recharge_order'
  const purchaseOrderStorageKey = 'user_panel_purchase_order'

  function getToken() {
    return localStorage.getItem('user_panel_token') || ''
  }

  const authHeaders = computed(() => ({ Authorization: `Bearer ${getToken()}` }))

  const appList = ref<any[]>([])
  const appsLoading = ref(true)
  const appsError = ref(false)
  const appsLoaded = ref(false)
  const appSelectKey = ref(0)
  let appsRequest: Promise<void> | null = null

  function ensureApps() {
    if (!appsRequest) {
      appsRequest = fetchApps().finally(() => {
        appsRequest = null
      })
    }
    return appsRequest
  }

  async function loadPurchaseAppOptions() {
    if (!appsLoaded.value) await ensureApps()
    return appList.value
  }

  function retryApps() {
    appsLoaded.value = false
    ensureApps()
  }

  async function fetchApps() {
    appsLoading.value = true
    appsError.value = false
    try {
      const { data } = await axios.get('/api/user-panel/apps/purchase', {
        headers: authHeaders.value
      })
      if (data.code !== 200) {
        appsLoaded.value = false
        appsError.value = true
        ElMessage.error(data.msg || '加载可购买应用失败')
        return
      }
      const apps = (data.data || []).map((a: any) => ({
        ...a,
        icon: a.icon || 'ri:apps-line',
        desc: a.desc || a.name,
        purchaseLicenseTypes: Array.isArray(a.purchaseLicenseTypes)
          ? a.purchaseLicenseTypes
          : typeOptionCatalog.map((option) => option.value),
        plans: (a.plans || []).map((p: any) => ({
          ...p,
          price: normalizePrice(p)
        }))
      }))
      appList.value = apps
      appsLoaded.value = true
      if (formData.appId) {
        const selected = apps.find((app: any) => app.id == formData.appId)
        if (!selected) {
          resetPurchaseSelection()
          step.value = 1
        } else if (!selected.purchaseLicenseTypes.includes(formData.type)) {
          formData.type = selected.purchaseLicenseTypes[0] || ''
          formData.domain = ''
        }
      }
    } catch {
      appsLoaded.value = false
      appsError.value = true
      ElMessage.error('加载可购买应用失败')
    } finally {
      appsLoading.value = false
    }
  }

  function normalizePrice(plan: any) {
    return Number(plan.price ?? plan.Price ?? plan.amount ?? plan.Amount ?? 0)
  }

  async function fetchPayOptions() {
    try {
      const { data } = await axios.get('/api/user-panel/purchase/pay-options', {
        headers: authHeaders.value
      })
      if (data.code === 200) {
        const options =
          Array.isArray(data.data?.options) && data.data.options.length > 0
            ? data.data.options
            : [
                {
                  code: 'balance',
                  label: '余额支付',
                  icon: 'ri:wallet-3-line',
                  color: '#2e7d32'
                }
              ]
        payOptions.value = options
        if (!options.some((option: any) => option.code === payMethod.value)) {
          payMethod.value = options[0]?.code || 'balance'
        }
      }
    } catch {
      ElMessage.error('加载支付方式失败')
    }
  }

  async function fetchBalance() {
    try {
      const { data } = await axios.get('/api/user-panel/balance', { headers: authHeaders.value })
      if (data.code === 200) {
        userBalance.value = Number(data.data.balance || 0)
      }
    } catch {
      ElMessage.error('加载余额失败')
    }
  }

  function notifyBalanceRefresh() {
    window.dispatchEvent(new CustomEvent('user-panel-balance-refresh'))
  }

  function showQrCheckout(data: { qrCode?: string; orderNo?: string; amount?: string | number }) {
    qrCheckout.visible = true
    qrCheckout.qrCode = data.qrCode || ''
    qrCheckout.orderNo = data.orderNo || ''
    qrCheckout.amount = data.amount || ''
  }

  async function fetchRechargeOptions() {
    try {
      const { data } = await axios.get('/api/user-panel/recharge/options', {
        headers: authHeaders.value
      })
      if (data.code === 200) {
        rechargeOptions.payTypes = Array.isArray(data.data?.payTypes) ? data.data.payTypes : []
        rechargeOptions.defaultType = data.data?.defaultType || 'alipay'
        rechargeOptions.options = Array.isArray(data.data?.options) ? data.data.options : []
        const codes = rechargeMethodOptions.value.map((item) => item.code)
        if (!codes.includes(rechargeDialog.payType)) {
          rechargeDialog.payType = codes.includes(rechargeOptions.defaultType)
            ? rechargeOptions.defaultType
            : codes[0] || rechargeOptions.defaultType
        }
      }
    } catch {
      ElMessage.error('加载充值方式失败')
    }
  }

  onMounted(() => {
    ensureApps()
    fetchBalance()
    fetchPayOptions()
    fetchRechargeOptions()
    handlePurchaseReturn()
    handleRechargeReturn()
  })

  const typeOptionCatalog = [
    { value: 'domain', label: '单域名', icon: 'ri:global-line' },
    { value: 'wildcard', label: '泛域名', icon: 'ri:asterisk' },
    { value: 'ip', label: 'IP地址', icon: 'ri:router-line' },
    { value: 'key', label: '密钥', icon: 'ri:key-2-line' }
  ]

  const typeLabels: Record<string, string> = {
    domain: '单域名',
    wildcard: '泛域名',
    ip: 'IP地址',
    key: '密钥'
  }

  const formData = reactive({
    appId: '' as string | number,
    planId: '' as string | number,
    type: 'domain',
    domain: ''
  })

  const domainLabel = computed(() => {
    const map: Record<string, string> = {
      domain: '域名',
      wildcard: '泛域名',
      ip: 'IP地址',
      key: '密钥'
    }
    return map[formData.type] || '域名'
  })

  const domainIcon = computed(() => {
    const map: Record<string, string> = {
      domain: 'ri:global-line',
      wildcard: 'ri:asterisk',
      ip: 'ri:router-line',
      key: 'ri:key-2-line'
    }
    return map[formData.type] || 'ri:global-line'
  })

  const domainPlaceholder = computed(() => {
    const map: Record<string, string> = {
      domain: 'example.com',
      wildcard: '*.example.com',
      ip: '192.168.1.1',
      key: '系统自动生成密钥'
    }
    return map[formData.type] || ''
  })

  function validateLicenseTarget(type: string, value: string) {
    const target = (value || '').trim().toLowerCase()
    if (type === 'key') return ''
    if (type === 'domain' && !isValidSingleDomain(target)) return '单域名格式不正确'
    if (type === 'wildcard' && (!target.startsWith('*.') || !isValidSingleDomain(target.slice(2))))
      return '泛域名格式不正确'
    if (type === 'ip' && !isValidIP(target)) return 'IP 格式不正确'
    return ''
  }

  function isValidSingleDomain(value: string) {
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

  function isValidIP(value: string) {
    const ipv4 = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/
    const ipv6 = /^(([0-9a-f]{1,4}:){7}[0-9a-f]{1,4}|::1|::)$/i
    return ipv4.test(value) || ipv6.test(value)
  }

  function getTargetError() {
    if (!availableTypeOptions.value.some((option) => option.value === formData.type)) {
      return '当前应用不支持该授权类型'
    }
    const target = formData.domain.trim()
    if (formData.type !== 'key' && !target) return '请填写授权目标'
    return validateLicenseTarget(formData.type, target)
  }

  const selectedApp = computed(() => appList.value.find((a) => a.id == formData.appId))
  const availableTypeOptions = computed(() => {
    const allowedTypes = Array.isArray(selectedApp.value?.purchaseLicenseTypes)
      ? selectedApp.value.purchaseLicenseTypes
      : []
    return typeOptionCatalog.filter((option) => allowedTypes.includes(option.value))
  })
  const selectedAppPlans = computed(() => selectedApp.value?.plans || [])
  const filteredAppPlans = computed(() =>
    selectedAppPlans.value.filter(
      (plan: any) => !plan.licenseType || plan.licenseType === formData.type
    )
  )
  const selectedPlan = computed(() =>
    selectedAppPlans.value.find((p: any) => p.id == formData.planId)
  )
  const computedCost = computed(() => Number(selectedPlan.value?.price || 0))
  const originalPrice = computed(() =>
    Number(selectedPlan.value?.originalPrice ?? computedCost.value)
  )
  const selectedPromotion = computed(() => selectedPlan.value?.promotion || null)
  const discountAmount = computed(() => Math.max(0, originalPrice.value - computedCost.value))
  const hasDiscount = computed(() => discountAmount.value >= 0.01)
  const isOnlinePay = computed(() => payMethod.value !== 'balance')
  const purchaseButtonText = computed(() => {
    if (purchasing.value) {
      return isOnlinePay.value ? '正在创建支付订单...' : '正在生成授权...'
    }
    return `确认支付 ¥${computedCost.value.toFixed(2)}`
  })
  const displayTarget = computed(() =>
    formData.type === 'key' ? '系统自动生成' : formData.domain || '—'
  )

  const targetFieldError = computed(() => {
    if (!formData.appId) return ''
    const allowed = availableTypeOptions.value.some((option) => option.value === formData.type)
    if (!allowed) return availableTypeOptions.value.length ? '当前应用不支持该授权类型' : ''
    const target = formData.domain.trim()
    if (!target || formData.type === 'key') return ''
    return validateLicenseTarget(formData.type, target)
  })

  const infoReady = computed(() => !!formData.appId && !!formData.planId && !getTargetError())

  const submitBlockReason = computed(() => {
    if (!formData.appId) return '请先选择应用'
    if (!availableTypeOptions.value.length) return '当前应用暂不支持自助开通'
    if (!formData.planId) return '请选择套餐'
    return getTargetError()
  })

  const balanceShort = computed(
    () =>
      !!formData.planId && payMethod.value === 'balance' && userBalance.value < computedCost.value
  )

  const purchaseDisabled = computed(
    () =>
      purchasing.value ||
      !!submitBlockReason.value ||
      balanceShort.value ||
      (isOnlinePay.value && computedCost.value <= 0)
  )

  const purchaseHint = computed(() => {
    if (submitBlockReason.value) return submitBlockReason.value
    if (balanceShort.value) return '当前余额不足，请先充值或更换支付方式'
    if (isOnlinePay.value && computedCost.value <= 0) return '0 元套餐请使用余额支付'
    return payMethod.value === 'balance'
      ? '套餐价格以后端为准，余额扣款后授权即时生效'
      : '支付成功后授权即时生效'
  })

  function minPlanPrice(app: any) {
    const prices = (app.plans || []).map((p: any) => Number(p.price || 0))
    return prices.length ? Math.min(...prices) : 0
  }

  function hasPromotion(app: any) {
    return (app.plans || []).some((p: any) => p.promotion)
  }

  function resetPurchaseSelection() {
    Object.assign(formData, { appId: '', planId: '', type: '', domain: '' })
  }

  function selectApp(id: string | number) {
    const app = appList.value.find((item) => item.id == id)
    formData.appId = id
    formData.planId = ''
    formData.type = app?.purchaseLicenseTypes?.[0] || ''
    formData.domain = ''
  }

  function onAppChange(id: string | number | Array<string | number> | null | undefined) {
    if (id === null || id === undefined || id === '' || Array.isArray(id)) {
      resetPurchaseSelection()
      return
    }
    selectApp(id)
  }

  function selectPlan(id: string | number) {
    formData.planId = id
    fetchPayOptions()
  }

  function selectType(type: string) {
    formData.type = type
    if (type === 'key') {
      formData.domain = ''
    }
    if (formData.planId) {
      const selected = selectedAppPlans.value.find((p: any) => p.id == formData.planId)
      if (selected?.licenseType && selected.licenseType !== type) {
        formData.planId = ''
      }
    }
  }

  function formatDuration(days: number | string | undefined) {
    const value = Number(days || 0)
    return value === 0 ? '永久' : `${value}天`
  }

  function promoRuleText(promotion: any) {
    if (!promotion) return ''
    if (promotion.ruleType === 'discount') return `${promotion.discount} 折`
    if (promotion.ruleType === 'reduction') return '立减'
    return '固定价'
  }

  function purchaseLimitText(limit: any) {
    if (!limit) return ''
    const owner =
      Number(limit.perOwnerLimit || 0) > 0 ? `每人限购 ${limit.perOwnerLimit} 份` : '每人不限购'
    const stock =
      Number(limit.stockLimit || 0) > 0 ? `活动库存上限 ${limit.stockLimit} 份` : '活动库存不限'
    return `${owner} · ${stock}`
  }

  function goToLicenses() {
    router.push('/user/licenses')
  }

  function resetFlow() {
    purchaseResult.value = null
    step.value = 1
    resetPurchaseSelection()
    appsLoaded.value = false
    appSelectKey.value += 1
    ensureApps()
    fetchBalance()
  }

  function openRechargeDialog() {
    rechargeDialog.visible = true
    fetchRechargeOptions()
  }

  async function submitRecharge() {
    if (rechargeDialog.submitting) return
    const amount = Number(rechargeDialog.amount)
    if (!Number.isFinite(amount) || amount < 0.01) {
      ElMessage.warning('充值金额不能低于 ¥0.01')
      return
    }

    rechargeDialog.submitting = true
    try {
      const { data } = await axios.post(
        '/api/user-panel/recharge/orders',
        {
          amount,
          payType: rechargeDialog.payType
        },
        { headers: authHeaders.value }
      )

      if (data.code === 200) {
        const orderNo = data.data?.orderNo || ''
        if (orderNo) sessionStorage.setItem(rechargeOrderStorageKey, orderNo)
        if (isQrCheckout(data.data)) {
          rechargeDialog.visible = false
          showQrCheckout(data.data)
          const paid = await pollRechargeOrder(orderNo)
          if (paid) {
            qrCheckout.visible = false
            sessionStorage.removeItem(rechargeOrderStorageKey)
            await fetchBalance()
            notifyBalanceRefresh()
            ElMessage.success('充值已到账，余额已刷新')
          }
          return
        }
        const payUrl = data.data?.payUrl || ''
        if (!payUrl) {
          ElMessage.error('支付地址生成失败')
          return
        }
        window.location.href = payUrl
        return
      }
      ElMessage.error(data.msg || '创建充值订单失败')
    } catch {
      ElMessage.error('创建充值订单失败，请重试')
    } finally {
      rechargeDialog.submitting = false
    }
  }

  async function handleRechargeReturn() {
    const queryOrderNo =
      typeof route.query.rechargeOrder === 'string' ? route.query.rechargeOrder : ''
    const storedOrderNo = sessionStorage.getItem(rechargeOrderStorageKey) || ''
    const orderNo = queryOrderNo || storedOrderNo
    if (!orderNo || orderNo.startsWith('LP')) return

    const paid = await pollRechargeOrder(orderNo)
    if (paid) {
      sessionStorage.removeItem(rechargeOrderStorageKey)
      clearRechargeReturnQuery()
      await fetchBalance()
      notifyBalanceRefresh()
      ElMessage.success('充值已到账，余额已刷新')
    }
  }

  function clearRechargeReturnQuery() {
    if (!route.query.rechargeOrder && !route.query.rechargeReturn) return
    const nextQuery = { ...route.query }
    delete nextQuery.rechargeOrder
    delete nextQuery.rechargeReturn
    router.replace({ path: route.path, query: nextQuery })
  }

  async function pollRechargeOrder(orderNo: string) {
    for (let index = 0; index < 6; index++) {
      const paid = await fetchRechargeOrderStatus(orderNo)
      if (paid) return true
      await wait(2000)
    }
    ElMessage.info('支付结果处理中，稍后可刷新页面查看余额')
    return false
  }

  async function fetchRechargeOrderStatus(orderNo: string) {
    try {
      const { data } = await axios.get(
        `/api/user-panel/recharge/orders/${encodeURIComponent(orderNo)}`,
        {
          headers: authHeaders.value
        }
      )
      if (data.code !== 200) return false
      if (data.data?.status === 'paid') {
        userBalance.value = Number(data.data.balance || userBalance.value)
        return true
      }
    } catch {
      return false
    }
    return false
  }

  async function handlePurchaseReturn() {
    const queryOrderNo =
      typeof route.query.rechargeOrder === 'string' ? route.query.rechargeOrder : ''
    const storedOrderNo = sessionStorage.getItem(purchaseOrderStorageKey) || ''
    const orderNo = queryOrderNo || storedOrderNo
    if (!orderNo || !orderNo.startsWith('UP')) return

    const result = await pollPurchaseOrder(orderNo)
    if (!result) return

    purchaseResult.value = {
      licenseNo: result.licenseNo,
      licenseId: result.licenseId,
      orderNo,
      payMethod: result.payMethod,
      appName: result.appName,
      planName: result.planName,
      durationDays: result.durationDays,
      cost: Number(result.cost || 0)
    }
    userBalance.value = Number(result.newBalance || userBalance.value)
    sessionStorage.removeItem(purchaseOrderStorageKey)
    clearPurchaseReturnQuery()
    notifyBalanceRefresh()
    step.value = 4
    ElMessage.success('支付成功，授权已生成')
  }

  async function pollPurchaseOrder(orderNo: string) {
    for (let index = 0; index < 20; index++) {
      const result = await fetchPurchaseOrderStatus(orderNo)
      if (result?.status === 'paid') return result
      if (result?.status === 'failed' || result?.status === 'cancelled') {
        sessionStorage.removeItem(purchaseOrderStorageKey)
        clearPurchaseReturnQuery()
        ElMessage.warning('支付未完成')
        return null
      }
      await wait(1500)
    }
    ElMessage.info('支付结果处理中，稍后可刷新页面继续确认')
    return null
  }

  async function fetchPurchaseOrderStatus(orderNo: string) {
    try {
      const { data } = await axios.get(
        `/api/user-panel/purchase/orders/${encodeURIComponent(orderNo)}`,
        {
          headers: authHeaders.value
        }
      )
      return data.code === 200 ? data.data : null
    } catch {
      return null
    }
  }

  function clearPurchaseReturnQuery() {
    if (!route.query.rechargeOrder && !route.query.rechargeReturn) return
    const nextQuery = { ...route.query }
    delete nextQuery.rechargeOrder
    delete nextQuery.rechargeReturn
    router.replace({ path: route.path, query: nextQuery })
  }

  function wait(ms: number) {
    return new Promise((resolve) => window.setTimeout(resolve, ms))
  }

  async function handlePurchase() {
    if (purchasing.value) return
    if (isOnlinePay.value && computedCost.value <= 0) {
      ElMessage.warning('0 元套餐请使用余额支付')
      return
    }
    if (payMethod.value === 'balance' && userBalance.value < computedCost.value) {
      ElMessage.warning('余额不足')
      return
    }
    const targetError = getTargetError()
    if (targetError) {
      ElMessage.warning(targetError)
      return
    }
    purchasing.value = true
    try {
      const { data } = await axios.post(
        '/api/user-panel/purchase',
        {
          appId: Number(formData.appId),
          planId: Number(formData.planId),
          type: formData.type,
          domain: formData.domain,
          payMethod: payMethod.value
        },
        { headers: authHeaders.value }
      )
      if (data.code === 200) {
        if (isQrCheckout(data.data)) {
          const orderNo = data.data?.orderNo || ''
          if (orderNo) sessionStorage.setItem(purchaseOrderStorageKey, orderNo)
          showQrCheckout(data.data)
          const result = await pollPurchaseOrder(orderNo)
          if (result) {
            qrCheckout.visible = false
            purchaseResult.value = {
              licenseNo: result.licenseNo,
              licenseId: result.licenseId,
              orderNo,
              payMethod: result.payMethod,
              appName: result.appName,
              planName: result.planName,
              durationDays: result.durationDays,
              cost: Number(result.cost || 0)
            }
            userBalance.value = Number(result.newBalance || userBalance.value)
            sessionStorage.removeItem(purchaseOrderStorageKey)
            notifyBalanceRefresh()
            step.value = 4
            ElMessage.success('支付成功，授权已生成')
          }
          return
        }
        if (data.data?.payUrl) {
          const orderNo = data.data?.orderNo || ''
          if (orderNo) sessionStorage.setItem(purchaseOrderStorageKey, orderNo)
          ElMessage.success('支付订单已创建，正在跳转收银台')
          window.location.href = data.data.payUrl
          return
        }
        purchaseResult.value = data.data
        userBalance.value = Number(data.data.newBalance || 0)
        notifyBalanceRefresh()
        step.value = 4
        ElMessage.success('购买成功，授权已生成')
      } else {
        ElMessage.error(data.msg || '购买失败')
      }
    } catch {
      ElMessage.error('请求失败，请重试')
    } finally {
      purchasing.value = false
    }
  }
</script>

<style scoped lang="scss">
  .user-purchase {
    max-width: 980px;
    padding-bottom: 12px;
    margin: 0 auto;
  }

  .page-head {
    margin-bottom: 16px;

    h2 {
      margin: 0 0 6px;
      font-size: 20px;
      font-weight: 600;
      color: var(--el-text-color-primary);
    }

    p {
      margin: 0;
      font-size: 13px;
      line-height: 1.6;
      color: var(--el-text-color-secondary);
    }
  }

  .step-card {
    margin-bottom: 16px;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 12px;

    :deep(.el-card__header) {
      padding: 14px 18px;
      border-bottom-color: var(--el-border-color-lighter);
    }

    :deep(.el-card__body) {
      padding: 18px;
    }

    &.is-done .step-index {
      background: var(--el-color-success);
    }
  }

  .step-card-head {
    display: flex;
    gap: 12px;
    align-items: center;

    h3 {
      margin: 0;
      font-size: 15px;
      font-weight: 600;
      color: var(--el-text-color-primary);
    }

    p {
      margin: 2px 0 0;
      font-size: 12px;
      color: var(--el-text-color-secondary);
    }
  }

  .step-index {
    display: inline-flex;
    flex-shrink: 0;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    font-size: 13px;
    font-weight: 700;
    color: #fff;
    background: var(--el-color-primary);
    border-radius: 50%;
  }

  .state-card {
    min-height: 220px;
  }

  .state-placeholder {
    min-height: 160px;
  }

  .purchase-app-select {
    display: block;
    width: 100%;
    max-width: 520px;
  }

  .app-summary {
    display: flex;
    gap: 12px;
    align-items: center;
    padding: 12px 14px;
    margin-top: 14px;
    background: var(--el-fill-color-light);
    border-radius: 10px;
  }

  .app-summary-icon {
    display: flex;
    flex-shrink: 0;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 40px;
    color: var(--el-color-primary);
    background: var(--el-color-primary-light-9);
    border-radius: 10px;
  }

  .app-summary-body {
    flex: 1;
    min-width: 0;

    p {
      margin: 4px 0 0;
      overflow: hidden;
      font-size: 12px;
      color: var(--el-text-color-secondary);
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  .app-summary-title {
    display: flex;
    gap: 8px;
    align-items: center;

    strong {
      font-size: 14px;
      color: var(--el-text-color-primary);
    }
  }

  .app-summary-price {
    display: flex;
    gap: 2px;
    align-items: baseline;
    font-weight: 700;
    color: var(--el-color-primary);

    span {
      font-size: 18px;
    }

    em {
      font-size: 12px;
      font-style: normal;
      font-weight: 500;
    }
  }

  .plan-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(220px, 100%), 1fr));
    gap: 12px;
  }

  .plan-card {
    position: relative;
    padding: 14px 14px 12px;
    text-align: left;
    cursor: pointer;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 10px;
    transition:
      border-color 0.2s,
      box-shadow 0.2s;

    &:hover {
      border-color: var(--el-color-primary-light-5);
    }

    &.active {
      background: var(--el-color-primary-light-9);
      border-color: var(--el-color-primary);
      box-shadow: 0 0 0 1px var(--el-color-primary) inset;
    }
  }

  .plan-head {
    display: flex;
    gap: 8px;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 8px;
  }

  .plan-name {
    font-size: 14px;
    font-weight: 600;
    color: var(--el-text-color-primary);
  }

  .plan-pricing {
    display: flex;
    gap: 2px;
    align-items: baseline;
    margin-bottom: 8px;
  }

  .plan-currency,
  .plan-amount {
    font-weight: 700;
    color: var(--el-color-primary);
  }

  .plan-amount {
    font-size: 22px;
    line-height: 1;
  }

  .plan-original {
    margin-left: 6px;
    font-size: 12px;
    color: var(--el-text-color-placeholder);
    text-decoration: line-through;
  }

  .plan-meta {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
  }

  .plan-duration {
    color: var(--el-text-color-primary);
  }

  .field-block + .field-block {
    margin-top: 16px;
  }

  .field-label {
    display: block;
    margin-bottom: 8px;
    font-size: 13px;
    font-weight: 600;
    color: var(--el-text-color-primary);
  }

  .field-hint,
  .section-muted {
    margin: 8px 0 0;
    font-size: 12px;
    color: var(--el-text-color-secondary);
  }

  .field-error {
    margin: 8px 0 0;
    font-size: 12px;
    color: var(--el-color-danger);
  }

  .type-options {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .type-chip {
    display: inline-flex;
    gap: 6px;
    align-items: center;
    height: 34px;
    padding: 0 12px;
    color: var(--el-text-color-regular);
    cursor: pointer;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color);
    border-radius: 8px;

    &.active {
      color: var(--el-color-primary);
      background: var(--el-color-primary-light-9);
      border-color: var(--el-color-primary);
    }
  }

  .pay-layout {
    display: grid;
    grid-template-columns: minmax(0, 1.1fr) minmax(0, 0.9fr);
    gap: 16px;
  }

  .order-panel,
  .method-panel {
    min-width: 0;
  }

  .order-row,
  .order-total {
    display: flex;
    gap: 12px;
    align-items: center;
    justify-content: space-between;
    padding: 8px 0;
    font-size: 13px;
    color: var(--el-text-color-secondary);

    strong {
      font-weight: 600;
      color: var(--el-text-color-primary);
      text-align: right;
    }
  }

  .order-total {
    padding-top: 12px;
    margin-top: 4px;
    border-top: 1px dashed var(--el-border-color-lighter);

    strong {
      font-size: 20px;
      color: var(--el-color-primary);
    }

    em {
      display: block;
      font-size: 12px;
      font-style: normal;
      color: var(--el-text-color-placeholder);
      text-align: right;
      text-decoration: line-through;
    }
  }

  .discount {
    color: var(--el-color-danger) !important;
  }

  .mono {
    font-family: 'Roboto Mono', ui-monospace, monospace;
  }

  .method-options {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .method-item {
    display: flex;
    gap: 8px;
    align-items: center;
    width: 100%;
    min-height: 48px;
    padding: 8px 12px;
    text-align: left;
    cursor: pointer;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color);
    border-radius: 10px;

    &.active {
      background: var(--el-color-primary-light-9);
      border-color: var(--el-color-primary);
    }
  }

  .method-label {
    flex: 1;
    font-size: 14px;
    color: var(--el-text-color-primary);
  }

  .method-balance {
    font-size: 13px;
    font-weight: 600;
    color: var(--el-color-success);
  }

  .balance-alert {
    margin-top: 12px;
  }

  .purchase-bar {
    position: sticky;
    bottom: 0;
    z-index: 5;
    display: flex;
    gap: 16px;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px;
    margin-top: 4px;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 12px;
    box-shadow: 0 8px 24px rgb(15 23 42 / 8%);
  }

  .purchase-bar-summary {
    min-width: 0;
  }

  .summary-line {
    display: flex;
    gap: 6px;
    align-items: center;
    font-size: 13px;
    color: var(--el-text-color-primary);
  }

  .summary-dot {
    color: var(--el-text-color-placeholder);
  }

  .summary-price {
    display: flex;
    gap: 8px;
    align-items: baseline;
    margin-top: 2px;

    strong {
      font-size: 20px;
      color: var(--el-color-primary);
    }
  }

  .summary-original {
    font-size: 12px;
    color: var(--el-text-color-placeholder);
    text-decoration: line-through;
  }

  .summary-hint {
    margin: 2px 0 0;
    font-size: 12px;
    color: var(--el-text-color-secondary);
  }

  .purchase-submit {
    flex-shrink: 0;
    min-width: 180px;
  }

  .success-hero {
    display: flex;
    gap: 12px;
    align-items: center;
  }

  .success-icon-wrap {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 52px;
    height: 52px;
    color: var(--el-color-success);
    background: var(--el-color-success-light-9);
    border-radius: 50%;
  }

  .success-title-block {
    flex: 1;

    h3 {
      margin: 0;
      font-size: 18px;
    }

    p {
      margin: 4px 0 0;
      font-size: 13px;
      color: var(--el-text-color-secondary);
    }
  }

  .license-no-card {
    display: flex;
    gap: 12px;
    align-items: center;
    justify-content: space-between;
    padding: 12px 14px;
    margin-top: 16px;
    background: var(--el-fill-color-light);
    border-radius: 10px;
  }

  .license-no-label {
    font-size: 13px;
    color: var(--el-text-color-secondary);
  }

  .license-no-value {
    font-family: 'Roboto Mono', ui-monospace, monospace;
    font-weight: 600;
  }

  .success-info-grid {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 12px;
    margin-top: 16px;
  }

  .success-info-item {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 10px 12px;
    background: var(--el-fill-color-lighter);
    border-radius: 10px;
  }

  .info-label {
    font-size: 12px;
    color: var(--el-text-color-secondary);
  }

  .info-value {
    font-size: 14px;
    font-weight: 600;
    color: var(--el-text-color-primary);
  }

  .success-amount-card {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding-top: 14px;
    margin-top: 16px;
    border-top: 1px dashed var(--el-border-color-lighter);

    span {
      color: var(--el-text-color-secondary);
    }

    strong {
      font-size: 22px;
      color: var(--el-color-primary);
    }
  }

  .success-actions {
    display: flex;
    gap: 10px;
    justify-content: flex-end;
    margin-top: 16px;
  }

  .recharge-dialog-body {
    .recharge-amount-input {
      width: 100%;
    }

    .quick-amounts {
      display: flex;
      gap: 8px;
      margin-top: 10px;

      button {
        flex: 1;
        height: 32px;
        cursor: pointer;
        background: var(--el-fill-color-blank);
        border: 1px solid var(--el-border-color);
        border-radius: 6px;

        &:hover {
          color: var(--el-color-primary);
          border-color: var(--el-color-primary-light-5);
        }
      }
    }

    .recharge-pay-label {
      margin-top: 16px;
    }

    .recharge-pay-types {
      display: flex;
      flex-wrap: wrap;
    }

    .recharge-pay-empty,
    .recharge-tip {
      margin: 8px 0 0;
      font-size: 12px;
      color: var(--el-text-color-secondary);
    }
  }

  @media (width <= 768px) {
    .pay-layout,
    .success-info-grid {
      grid-template-columns: 1fr;
    }

    .purchase-bar {
      flex-direction: column;
      align-items: stretch;
    }

    .purchase-submit {
      width: 100%;
      min-width: 0;
    }

    .success-hero,
    .success-actions {
      flex-wrap: wrap;
    }
  }
</style>
