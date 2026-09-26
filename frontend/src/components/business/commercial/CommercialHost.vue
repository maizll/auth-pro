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
      modal-class="commercial-purchase-modal"
      append-to-body
      @open="loadPurchase"
      @closed="onPurchaseClosed"
    >
      <div v-loading="loading" class="upgrade-body">
        <ElAlert v-if="loadError" type="error" :closable="false" show-icon :title="loadError" />
        <template v-else-if="upgraded">
          <ElAlert type="success" :closable="false" show-icon :title="successTitle">
            <p>授权已经生效。关闭此窗口后，顶栏和页面上的限制提示会马上更新。</p>
            <p v-if="account">到期时间：{{ commercialExpireText(account) }}</p>
          </ElAlert>
        </template>
        <template v-else>
          <CommercialMark
            v-if="action !== 'view'"
            text="免费版只能创建 1 个授权应用。商业版解除这个限制，可以创建多个授权应用。付款在当前窗口完成，不会跳转到商店。"
          />
          <ElAlert
            v-if="account?.domainMismatch"
            type="warning"
            :closable="false"
            show-icon
            title="当前访问域名与授权域名不一致，付费能力暂按免费版处理。"
          />
          <template v-if="action === 'view'">
            <p>当前是永久商业版，无需再次购买。</p>
            <p v-if="account?.account">绑定账号：{{ account.account }}</p>
            <p v-if="account?.domain">授权域名：{{ account.domain }}</p>
            <p v-if="account?.licenseNo">授权编号：{{ account.licenseNo }}</p>
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
            <p>{{ action === 'renew' ? '已绑定账号，请选择续费套餐。' : '已绑定账号，请选择套餐。' }}</p>
            <ElSelect v-model="planId" placeholder="选择套餐" style="width: 100%">
              <ElOption v-for="plan in plans" :key="plan.id" :label="commercialPlanLabel(plan)" :value="plan.id" />
            </ElSelect>
            <ElButton type="primary" :loading="acting" :disabled="!planId" @click="pay">生成付款码</ElButton>
            <p v-if="!plans.length" class="upgrade-tip">源站尚未配置可购买的套餐。</p>
          </template>
          <template v-else>
            <p>请使用支付宝或微信扫描付款。支付完成后会在此窗口确认，无需新开页面。</p>
            <div class="upgrade-qr">
              <QrcodeVue :value="payUrl" :size="200" />
            </div>
            <p class="upgrade-tip">{{ orderTitle }}</p>
            <p v-if="payStatus" class="upgrade-status">{{ payStatus }}</p>
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
        <template v-else-if="account && isCommercialActive(account)">
          <CommercialMark text="当前已是商业版，无需再次升级。" icon="ri:vip-crown-fill" tone="ok" />
          <p>到期时间：{{ commercialExpireText(account) }}</p>
          <p v-if="account.account">绑定账号：{{ account.account }}</p>
          <p v-if="account.domain">授权域名：{{ account.domain }}</p>
          <p v-if="account.licenseNo">授权编号：{{ account.licenseNo }}</p>
          <p v-if="account.offlineGrace">源站暂时连不上，商业版处于离线宽限。</p>
        </template>
        <template v-else>
          <p>当前还不是商业版。</p>
        </template>
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
  import {
    commercialCta,
    commercialCtaLabel,
    commercialExpireText,
    commercialPlanLabel,
    commercialUi,
    isCommercialActive,
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
  const dialogTitle = computed(() => {
    if (upgraded.value) return '升级完成'
    if (action.value === 'view') return '查看授权'
    if (action.value === 'renew') return '续费'
    return '升级商业版'
  })
  const promptActionLabel = computed(() => commercialCtaLabel(commercialCta(commercialUi.account)))
  const plans = ref<StorePlan[]>([])
  const planId = ref<number>()
  const payUrl = ref('')
  const orderTitle = ref('')
  const payStatus = ref('')
  const payFailed = ref('')
  const upgraded = ref(false)
  const successTitle = ref('已升级为商业版')
  const form = reactive({ account: '', password: '', role: 'user' })
  const connection = reactive({ sourceBase: 'https://auth.maizll.com', siteUrl: '', trustProxy: false })
  let pollTimer: ReturnType<typeof setInterval> | null = null
  let pollStarted = 0
  let pollFailures = 0
  let pendingRefresh = false
  const pollTimeoutMs = 10 * 60 * 1000

  function goUpgrade() {
    commercialUi.promptOpen = false
    commercialUi.upgradeOpen = true
  }

  function stopPoll() {
    if (pollTimer) clearInterval(pollTimer)
    pollTimer = null
  }

  function resetPay() {
    stopPoll()
    payUrl.value = ''
    payStatus.value = ''
    payFailed.value = ''
    orderTitle.value = ''
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
      poll(order.orderNo)
    } catch (error: unknown) {
      showCaughtError(error, '创建订单失败')
    } finally {
      acting.value = false
    }
  }

  function poll(orderNo: string) {
    stopPoll()
    pollStarted = Date.now()
    pollFailures = 0
    payStatus.value = '正在等待支付'
    pollTimer = setInterval(async () => {
      if (Date.now() - pollStarted > pollTimeoutMs) {
        stopPoll()
        payStatus.value = ''
        payFailed.value = '支付等待超时。若已经付款，请关闭窗口后看顶栏是否已变为商业版；若尚未付款，请重新生成付款码。'
        ElMessage.error(payFailed.value)
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
  })
</script>

<style scoped>
  .upgrade-body {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .upgrade-form {
    margin-top: 8px;
  }

  .upgrade-tip,
  .upgrade-status {
    margin: 0;
    color: var(--el-text-color-secondary);
    font-size: 12px;
  }

  .upgrade-status {
    color: var(--el-color-primary);
  }

  .upgrade-qr {
    display: flex;
    justify-content: center;
  }

  .upgrade-body p {
    margin: 0;
  }
</style>

<style>
  .commercial-purchase-modal .el-dialog {
    width: min(480px, calc(100vw - 24px));
    max-width: calc(100vw - 24px);
  }

  @media (max-width: 640px) {
    .commercial-purchase-modal .el-dialog {
      margin-top: 8vh;
    }

    .commercial-purchase-modal .el-dialog__body {
      max-height: calc(100vh - 180px);
      overflow: auto;
    }
  }
</style>
