<template>
  <div>
    <ElDialog v-model="commercialUi.promptOpen" title="需要商业版" width="460px" append-to-body>
      <CommercialMark :text="commercialUi.promptText" icon="ri:vip-crown-fill" />
      <template #footer>
        <ElButton @click="commercialUi.promptOpen = false">知道了</ElButton>
        <ElButton type="primary" @click="goUpgrade">{{ promptActionLabel }}</ElButton>
      </template>
    </ElDialog>

    <ElDialog
      v-model="commercialUi.upgradeOpen"
      :title="dialogTitle"
      :modal-class="purchaseModalClass"
      append-to-body
      @open="loadPurchase"
      @closed="onPurchaseClosed"
    >
      <template #header>
        <div v-if="showBrandBanner" class="edition-banner">
          <ArtSvgIcon icon="ri:vip-diamond-fill" class="edition-banner__icon" />
          <div>
            <h3>{{ action === 'renew' ? '续费商业版，继续使用全部能力' : commercialPitch.title }}</h3>
            <p>{{ commercialPitch.subtitle }}</p>
          </div>
        </div>
        <span v-else class="plain-title">{{ dialogTitle }}</span>
      </template>
      <div v-loading="loading" class="upgrade-body">
        <ElAlert v-if="loadError" type="error" :closable="false" show-icon :title="loadError" />
        <template v-else-if="upgraded">
          <div class="celebrate" aria-hidden="true">
            <span v-for="n in 10" :key="n" class="celebrate__spark" :style="{ '--i': n }" />
          </div>
          <div class="celebrate-copy">
            <ArtSvgIcon icon="ri:vip-crown-fill" class="celebrate-copy__icon" />
            <h3>{{ successTitle }}</h3>
            <p>授权已经生效。关闭此窗口后，顶栏和页面上的限制提示会马上更新。</p>
            <p v-if="account">到期时间：{{ commercialExpireText(account) }}</p>
          </div>
        </template>
        <template v-else>
          <section v-if="action !== 'view'" class="compare">
            <div class="compare__head">
              <span>能力</span>
              <span>免费版</span>
              <span>商业版</span>
            </div>
            <div v-for="row in commercialCompareRows" :key="row.label" class="compare__row">
              <span class="compare__label">{{ row.label }}</span>
              <span class="compare__free">{{ row.free }}</span>
              <span class="compare__paid">{{ row.commercial }}</span>
            </div>
            <p class="upgrade-tip">{{ commercialCompareNote }}</p>
          </section>
          <ElAlert
            v-if="account?.domainMismatch"
            type="warning"
            :closable="false"
            show-icon
            title="当前访问域名与授权域名不一致，付费能力暂按免费版处理。"
          />
          <template v-if="action === 'view'">
            <CommercialLicenseCard :account="account" />
          </template>
          <template v-else-if="!account?.bound">
            <ElForm label-width="72px" class="upgrade-form">
              <ElFormItem label="身份">
                <ElRadioGroup v-model="form.role">
                  <ElRadio value="user">用户</ElRadio>
                  <ElRadio value="agent">代理商</ElRadio>
                </ElRadioGroup>
              </ElFormItem>
              <ElFormItem label="账号">
                <ElInput v-model.trim="form.account" placeholder="邮箱或账号" />
              </ElFormItem>
              <ElFormItem label="密码">
                <ElInput v-model="form.password" type="password" show-password placeholder="仅用于本次登录，不会保存" />
              </ElFormItem>
            </ElForm>
            <ElButton type="primary" :loading="acting" @click="bind">登录并绑定</ElButton>
            <p class="upgrade-tip">将使用站点域名 {{ account?.requestDomain || '（未识别）' }} 绑定，域名不可在此修改。</p>
          </template>
          <template v-else-if="!payUrl">
            <p class="section-title">{{ action === 'renew' ? '选择续费套餐' : '选择套餐' }}</p>
            <div v-if="plans.length" class="plan-grid">
              <button
                v-for="(plan, index) in plans"
                :key="plan.id"
                type="button"
                class="plan-card"
                :class="{ 'is-selected': planId === plan.id, 'is-recommended': index === 0 }"
                @click="planId = plan.id"
              >
                <span v-if="index === 0" class="plan-card__ribbon">推荐</span>
                <span class="plan-card__name">{{ plan.name }}</span>
                <span class="plan-card__price">
                  <small>¥</small>{{ yuanWhole(plan.priceCents) }}<small>.{{ yuanFrac(plan.priceCents) }}</small>
                </span>
                <span class="plan-card__period">{{ commercialPeriodText(plan.period) || '按约定时长' }}</span>
              </button>
            </div>
            <ElButton type="primary" :loading="acting" :disabled="!planId" @click="pay">生成付款码</ElButton>
            <p v-if="!plans.length" class="upgrade-tip">源站尚未配置可购买的套餐。</p>
          </template>
          <template v-else>
            <div class="pay-panel">
              <div class="upgrade-qr">
                <QrcodeVue :value="payUrl" :size="188" />
              </div>
              <div class="pay-panel__meta">
                <p class="pay-panel__title">{{ orderTitle }}</p>
                <p class="pay-panel__count">剩余 {{ countdownText }}</p>
                <p v-if="payStatus" class="upgrade-status">{{ payStatus }}</p>
                <p class="upgrade-tip">安全支付：二维码由本站生成。也可以打开付款页，本站不保存支付密码。</p>
                <ElButton @click="openPayPage">打开付款页</ElButton>
              </div>
            </div>
            <ElAlert v-if="payFailed" type="error" :closable="false" show-icon :title="payFailed" />
            <ElButton v-if="payFailed" @click="resetPay">重新选择套餐</ElButton>
          </template>
          <ElCollapse class="upgrade-settings">
            <ElCollapseItem title="源站连接" name="conn">
              <ElForm label-width="88px">
                <ElFormItem label="源站根">
                  <ElInput v-model.trim="connection.sourceBase" placeholder="https://auth.maizll.com" />
                </ElFormItem>
                <ElFormItem label="站点地址">
                  <ElInput v-model.trim="connection.siteUrl" placeholder="https://你的域名" />
                </ElFormItem>
                <ElFormItem label="信任代理">
                  <ElSwitch v-model="connection.trustProxy" />
                </ElFormItem>
              </ElForm>
              <ElButton :loading="acting" @click="saveConnection">保存连接</ElButton>
            </ElCollapseItem>
          </ElCollapse>
        </template>
      </div>
      <template v-if="upgraded" #footer>
        <ElButton type="primary" @click="commercialUi.upgradeOpen = false">关闭</ElButton>
      </template>
    </ElDialog>

    <ElDialog
      v-model="commercialUi.licenseOpen"
      title="商业版授权"
      modal-class="commercial-purchase-modal"
      append-to-body
      @open="loadLicense"
    >
      <div v-loading="loading" class="upgrade-body">
        <ElAlert v-if="loadError" type="error" :closable="false" show-icon :title="loadError" />
        <CommercialLicenseCard v-else :account="account" />
      </div>
      <template #footer>
        <ElButton type="primary" @click="commercialUi.licenseOpen = false">关闭</ElButton>
      </template>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { computed, onBeforeUnmount, reactive, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import { caughtErrorText, errorAlreadyToasted, showCaughtError } from '@/utils/http/error-toast'
  import QrcodeVue from 'qrcode.vue'
  import CommercialMark from './CommercialMark.vue'
  import CommercialLicenseCard from './CommercialLicenseCard.vue'
  import {
    commercialCompareNote,
    commercialCompareRows,
    commercialCta,
    commercialCtaLabel,
    commercialExpireText,
    commercialPeriodText,
    commercialPitch,
    commercialUi,
    rememberCommercialAccount
  } from '@/utils/commercial'
  import {
    bindStoreAccount,
    createStoreEditionOrder,
    fetchStoreAccount,
    fetchStoreEditionOrder,
    fetchStorePlans,
    saveStoreConnection,
    type StoreAccount,
    type StorePlan
  } from '@/api/store'

  const loading = ref(false)
  const acting = ref(false)
  const loadError = ref('')
  const account = ref<StoreAccount | null>(null)
  const action = computed(() => commercialCta(account.value))
  const upgraded = ref(false)
  const dialogTitle = computed(() => {
    if (upgraded.value) return '升级完成'
    if (action.value === 'view') return '查看授权'
    if (action.value === 'renew') return '续费'
    return '升级商业版'
  })
  const promptActionLabel = computed(() => commercialCtaLabel(commercialCta(commercialUi.account)))
  const nowTick = ref(Date.now())
  let tickTimer: ReturnType<typeof setInterval> | null = null
  let payDeadline = 0
  const payWindowMs = 30 * 60 * 1000
  const countdownText = computed(() => {
    const left = Math.max(0, payDeadline - nowTick.value)
    const total = Math.ceil(left / 1000)
    const minutes = Math.floor(total / 60)
    const seconds = total % 60
    return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`
  })
  const plans = ref<StorePlan[]>([])
  const planId = ref<number>()
  const payUrl = ref('')
  const orderTitle = ref('')
  const payStatus = ref('')
  const payFailed = ref('')
  const showBrandBanner = computed(() => !upgraded.value && action.value !== 'view')
  const purchaseModalClass = computed(() =>
    showBrandBanner.value ? 'commercial-purchase-modal is-brand' : 'commercial-purchase-modal is-plain'
  )
  const successTitle = ref('已升级为商业版')
  const form = reactive({ account: '', password: '', role: 'user' })
  const connection = reactive({ sourceBase: 'https://auth.maizll.com', siteUrl: '', trustProxy: false })
  let pollTimer: ReturnType<typeof setInterval> | null = null
  let pollFailures = 0
  let pendingRefresh = false

  function goUpgrade() {
    commercialUi.promptOpen = false
    commercialUi.upgradeOpen = true
  }

  function stopPoll() {
    if (pollTimer) clearInterval(pollTimer)
    pollTimer = null
  }

  function stopTick() {
    if (tickTimer) clearInterval(tickTimer)
    tickTimer = null
  }

  function resetPay() {
    stopPoll()
    stopTick()
    payDeadline = 0
    payUrl.value = ''
    payStatus.value = ''
    payFailed.value = ''
    orderTitle.value = ''
  }

  function yuanWhole(cents: number) {
    return Math.floor(cents / 100).toString()
  }

  function yuanFrac(cents: number) {
    return String(cents % 100).padStart(2, '0')
  }

  function openPayPage() {
    if (!payUrl.value) return
    window.open(payUrl.value, '_blank', 'noopener')
  }

  function expirePayWait() {
    stopPoll()
    stopTick()
    payStatus.value = ''
    if (payFailed.value) return
    payFailed.value = '支付等待超时。若已经付款，请关闭窗口后看顶栏是否已变为商业版；若尚未付款，请重新生成付款码。'
    ElMessage.error(payFailed.value)
  }

  function applyAccount(next: StoreAccount) {
    account.value = next
    rememberCommercialAccount(next)
    connection.sourceBase = next.sourceBase || connection.sourceBase
    connection.siteUrl = next.siteUrl || ''
    connection.trustProxy = !!next.trustProxy
  }

  async function loadPurchase() {
    stopPoll()
    upgraded.value = false
    successTitle.value = '已升级为商业版'
    resetPay()
    loadError.value = ''
    loading.value = true
    try {
      const next = await fetchStoreAccount()
      applyAccount(next)
      if (next.bound && commercialCta(next) !== 'view') {
        const data = await fetchStorePlans()
        plans.value = data.list || []
        planId.value = plans.value[0]?.id
      } else {
        plans.value = []
        planId.value = undefined
      }
    } catch (error: unknown) {
      loadError.value = caughtErrorText(error, '读取商业版信息失败', errorAlreadyToasted(error)) || '读取商业版信息失败'
      showCaughtError(error, '读取商业版信息失败')
    } finally {
      loading.value = false
    }
  }

  async function loadLicense() {
    loadError.value = ''
    loading.value = true
    try {
      const next = await fetchStoreAccount()
      applyAccount(next)
    } catch (error: unknown) {
      loadError.value = caughtErrorText(error, '读取授权信息失败', errorAlreadyToasted(error)) || '读取授权信息失败'
      showCaughtError(error, '读取授权信息失败')
    } finally {
      loading.value = false
    }
  }

  async function bind() {
    if (!form.account || !form.password) {
      ElMessage.error('请填写账号和密码')
      return
    }
    acting.value = true
    try {
      const next = await bindStoreAccount({ ...form })
      applyAccount(next)
      form.password = ''
      ElMessage.success('已绑定，请继续支付')
      const data = await fetchStorePlans()
      plans.value = data.list || []
      planId.value = plans.value[0]?.id
      if (!plans.value.length) {
        ElMessage.warning('源站尚未配置可购买的套餐')
      }
    } catch (error: unknown) {
      showCaughtError(error, '绑定失败，请检查源站是否可访问')
    } finally {
      acting.value = false
    }
  }

  async function pay() {
    if (!planId.value) {
      ElMessage.warning('请选择套餐')
      return
    }
    acting.value = true
    payFailed.value = ''
    const buyingUpgrade = action.value !== 'renew'
    try {
      const order = await createStoreEditionOrder(planId.value)
      if (!order?.payUrl || !order.orderNo) {
        payFailed.value = '源站没有返回付款码，无法继续支付。'
        ElMessage.error(payFailed.value)
        return
      }
      payUrl.value = order.payUrl
      orderTitle.value = `${order.title} ${(order.amountCents / 100).toFixed(2)} 元`
      successTitle.value = buyingUpgrade ? '已升级为商业版' : '商业版已续费'
      payDeadline = Date.now() + payWindowMs
      nowTick.value = Date.now()
      stopTick()
      tickTimer = setInterval(() => {
        nowTick.value = Date.now()
        if (payDeadline && nowTick.value >= payDeadline) expirePayWait()
      }, 1000)
      poll(order.orderNo)
    } catch (error: unknown) {
      showCaughtError(error, '创建订单失败')
    } finally {
      acting.value = false
    }
  }

  function poll(orderNo: string) {
    stopPoll()
    pollFailures = 0
    payStatus.value = '正在等待支付'
    pollTimer = setInterval(async () => {
      if (payDeadline && Date.now() >= payDeadline) {
        expirePayWait()
        return
      }
      try {
        const order = await fetchStoreEditionOrder(orderNo)
        pollFailures = 0
        if (order.status === 'paid') {
          stopPoll()
          await finishPaid()
          return
        }
        if (order.status === 'refunded') {
          stopPoll()
          payStatus.value = ''
          payFailed.value = '这笔订单已退款，商业版没有生效。'
          ElMessage.error(payFailed.value)
          return
        }
        if (order.status && order.status !== 'pending') {
          stopPoll()
          payStatus.value = ''
          payFailed.value = '支付未完成。请重新生成付款码后再试。'
          ElMessage.error(payFailed.value)
          return
        }
        payStatus.value = '正在等待支付'
      } catch (error: unknown) {
        pollFailures += 1
        payStatus.value = '暂时连不上源站，正在重试'
        if (pollFailures >= 5) {
          stopPoll()
          payStatus.value = ''
          payFailed.value =
            caughtErrorText(error, '无法确认支付结果，请检查网络后重试', errorAlreadyToasted(error)) ||
            '无法确认支付结果，请检查网络后重试'
          showCaughtError(error, '无法确认支付结果，请检查网络后重试')
        }
      }
    }, 2000)
  }

  async function finishPaid() {
    payStatus.value = '支付成功，正在同步授权'
    try {
      const next = await fetchStoreAccount()
      applyAccount(next)
    } catch (error: unknown) {
      showCaughtError(error, '支付已完成，但读取授权失败。关闭窗口后会再试一次。')
    }
    payUrl.value = ''
    upgraded.value = true
    pendingRefresh = true
    payStatus.value = ''
  }

  function onPurchaseClosed() {
    stopPoll()
    if (pendingRefresh) {
      pendingRefresh = false
      window.dispatchEvent(new Event('store-account-refresh'))
    }
    upgraded.value = false
    resetPay()
  }

  async function saveConnection() {
    if (!connection.sourceBase.startsWith('https://')) {
      ElMessage.error('源站地址需要以 https:// 开头')
      return
    }
    acting.value = true
    try {
      await saveStoreConnection({ ...connection })
      await loadPurchase()
    } catch (error: unknown) {
      showCaughtError(error, '保存源站连接失败')
    } finally {
      acting.value = false
    }
  }

  onBeforeUnmount(() => {
    stopPoll()
    stopTick()
  })
</script>

<style scoped>
  .upgrade-body {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .edition-banner {
    display: flex;
    gap: 12px;
    align-items: flex-start;
    margin: -16px calc(-16px - var(--el-dialog-padding-primary, 16px) - var(--el-message-close-size, 16px)) 0 -16px;
    padding: 18px 56px 18px 18px;
    color: #fff8e8;
    background: linear-gradient(135deg, #3a2a12 0%, #8a6232 48%, #e8c56b 100%);
  }

  .edition-banner h3,
  .edition-banner p,
  .celebrate-copy h3,
  .celebrate-copy p,
  .section-title,
  .upgrade-tip,
  .upgrade-status,
  .pay-panel__title,
  .pay-panel__count {
    margin: 0;
  }

  .edition-banner h3 {
    font-size: 18px;
    line-height: 1.35;
  }

  .edition-banner p {
    margin-top: 4px;
    color: rgb(255 248 232 / 88%);
    font-size: 13px;
    line-height: 1.5;
  }

  .edition-banner__icon {
    flex: none;
    font-size: 28px;
  }

  .plain-title {
    font-size: 16px;
    font-weight: 600;
    color: var(--el-text-color-primary);
  }

  .compare {
    overflow: hidden;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 12px;
  }

  .compare__head,
  .compare__row {
    display: grid;
    grid-template-columns: 1.3fr 1fr 1.2fr;
    gap: 8px;
    align-items: center;
    padding: 10px 12px;
  }

  .compare__head {
    color: var(--el-text-color-secondary);
    font-size: 12px;
    background: var(--el-fill-color-light);
  }

  .compare__row + .compare__row {
    border-top: 1px solid var(--el-border-color-extra-light);
  }

  .compare__label {
    color: var(--el-text-color-primary);
    font-size: 13px;
  }

  .compare__free,
  .compare__paid {
    font-size: 13px;
  }

  .compare__free {
    color: var(--el-text-color-secondary);
  }

  .compare__paid {
    color: #8a6232;
    font-weight: 600;
  }

  :global(html.dark) .compare__paid,
  :global(.dark) .compare__paid,
  :global(html.dark) .plan-card__price,
  :global(.dark) .plan-card__price,
  :global(html.dark) .pay-panel__count,
  :global(.dark) .pay-panel__count,
  :global(html.dark) .celebrate-copy__icon,
  :global(.dark) .celebrate-copy__icon {
    color: #f3d48a;
  }

  .section-title {
    color: var(--el-text-color-primary);
    font-weight: 600;
  }

  .plan-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 10px;
  }

  .plan-card {
    position: relative;
    display: flex;
    flex-direction: column;
    gap: 6px;
    align-items: flex-start;
    padding: 16px 14px 14px;
    overflow: hidden;
    text-align: left;
    cursor: pointer;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color);
    border-radius: 12px;
  }

  .plan-card.is-recommended {
    padding-top: 22px;
  }

  .plan-card.is-selected {
    border-color: #c8962e;
    box-shadow: 0 0 0 2px rgb(200 150 46 / 35%);
  }

  .plan-card__ribbon {
    position: absolute;
    top: 8px;
    right: -28px;
    padding: 2px 32px;
    color: #6b4a12;
    font-size: 12px;
    background: linear-gradient(180deg, #fff3cc, #f3d48a);
    transform: rotate(35deg);
  }

  .plan-card__name {
    color: var(--el-text-color-primary);
    font-size: 14px;
  }

  .plan-card__price {
    color: #8a6232;
    font-size: 28px;
    font-weight: 700;
    line-height: 1;
  }

  .plan-card__price small {
    font-size: 14px;
  }

  .plan-card__period {
    color: var(--el-text-color-secondary);
    font-size: 12px;
  }

  .pay-panel {
    display: grid;
    grid-template-columns: 188px 1fr;
    gap: 16px;
    align-items: center;
  }

  .upgrade-qr {
    display: flex;
    justify-content: center;
    padding: 8px;
    background: #fff;
    border-radius: 12px;
  }

  .pay-panel__title {
    color: var(--el-text-color-primary);
    font-weight: 600;
  }

  .pay-panel__count {
    margin-top: 6px;
    color: #8a6232;
    font-size: 20px;
    font-variant-numeric: tabular-nums;
  }

  .upgrade-tip,
  .upgrade-status {
    color: var(--el-text-color-secondary);
    font-size: 12px;
    line-height: 1.5;
  }

  .upgrade-status {
    color: var(--el-color-primary);
  }

  .celebrate {
    position: relative;
    height: 72px;
  }

  .celebrate__spark {
    position: absolute;
    top: 36px;
    left: 50%;
    width: 8px;
    height: 8px;
    background: #e8c56b;
    border-radius: 50%;
    animation: spark-out 900ms ease-out both;
    animation-delay: calc(var(--i) * 40ms);
    transform: rotate(calc(var(--i) * 36deg)) translateY(-8px);
  }

  .celebrate-copy {
    display: flex;
    flex-direction: column;
    gap: 6px;
    align-items: center;
    text-align: center;
  }

  .celebrate-copy__icon {
    font-size: 36px;
    color: #c8962e;
    animation: crown-pop 500ms ease-out both;
  }

  .celebrate-copy h3 {
    color: var(--el-text-color-primary);
    font-size: 22px;
  }

  .celebrate-copy p {
    color: var(--el-text-color-regular);
    font-size: 13px;
  }

  @keyframes spark-out {
    from {
      opacity: 1;
      transform: rotate(calc(var(--i) * 36deg)) translateY(0);
    }

    to {
      opacity: 0;
      transform: rotate(calc(var(--i) * 36deg)) translateY(-46px);
    }
  }

  @keyframes crown-pop {
    from {
      transform: scale(0.6);
    }

    to {
      transform: scale(1);
    }
  }

  @media (max-width: 640px) {
    .edition-banner {
      padding: 16px 48px 16px 14px;
    }

    .compare__head,
    .compare__row {
      grid-template-columns: 1fr;
      gap: 2px;
    }

    .compare__head {
      display: none;
    }

    .compare__free::before {
      content: '免费版：';
    }

    .compare__paid::before {
      content: '商业版：';
    }

    .plan-grid,
    .pay-panel {
      grid-template-columns: 1fr;
    }
  }
</style>

<style>
  .commercial-purchase-modal .el-dialog {
    width: min(720px, calc(100vw - 24px));
    max-width: calc(100vw - 24px);
    overflow: hidden;
  }

  .commercial-purchase-modal.is-brand .el-dialog__header {
    padding-bottom: 0;
  }

  .commercial-purchase-modal.is-brand .el-dialog__headerbtn .el-dialog__close {
    color: #fff8e8;
  }

  @media (max-width: 640px) {
    .commercial-purchase-modal .el-dialog {
      margin-top: 6vh;
    }

    .commercial-purchase-modal .el-dialog__body {
      max-height: calc(100vh - 160px);
      overflow: auto;
    }
  }
</style>
