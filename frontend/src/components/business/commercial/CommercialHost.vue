<!-- 购买商业版和单独购买插件或模板共用这一个窗口。账号栏固定在内容区顶部，付款区是这一步里的二维码和倒计时。 -->
<template>
  <div>
    <ElDialog v-model="commercialUi.promptOpen" title="需要商业版" width="460px" append-to-body>
      <CommercialMark :text="commercialUi.promptText" icon="ri:rocket-2-line" />
      <template #footer>
        <ElButton @click="commercialUi.promptOpen = false">知道了</ElButton>
        <ElButton type="primary" @click="goUpgrade">{{ promptActionLabel }}</ElButton>
      </template>
    </ElDialog>

    <ElDialog
      v-model="commercialUi.upgradeOpen"
      :title="dialogTitle"
      :width="purchaseDialogWidth"
      :modal-class="purchaseModalClass"
      append-to-body
      @open="loadPurchase"
      @closed="onPurchaseClosed"
    >
      <template #header>
        <div v-if="showBrandBanner" class="edition-banner">
          <ArtSvgIcon icon="ri:rocket-2-fill" class="edition-banner__icon" />
          <div>
            <h3>{{
              action === 'renew' ? '续费商业版，继续使用全部能力' : commercialPitch.title
            }}</h3>
            <p>{{ commercialPitch.subtitle }}</p>
          </div>
        </div>
        <span v-else class="plain-title">{{ dialogTitle }}</span>
      </template>
      <div v-loading="loading" class="upgrade-body">
        <ElAlert v-if="loadError" type="error" :closable="false" show-icon :title="loadError" />
        <ElAlert
          v-else-if="siteProblem"
          type="warning"
          :closable="false"
          show-icon
          :title="siteProblem"
        />
        <template v-else-if="upgraded && buyingItem && !itemContinued">
          <div class="celebrate-copy">
            <ArtSvgIcon icon="ri:shield-check-fill" class="celebrate-copy__icon" />
            <h3>已购买</h3>
            <p>权益已刷新。可以立即启用。</p>
            <ElButton type="primary" :loading="acting" @click="resumeItem"
              >已购买，立即启用</ElButton
            >
          </div>
        </template>
        <template v-else-if="upgraded">
          <div class="celebrate" aria-hidden="true">
            <span v-for="n in 10" :key="n" class="celebrate__spark" :style="{ '--i': n }" />
          </div>
          <div class="celebrate-copy">
            <ArtSvgIcon icon="ri:shield-check-fill" class="celebrate-copy__icon" />
            <h3>{{ successTitle }}</h3>
            <p v-if="buyingItem">购买已生效，并已继续启用。</p>
            <p v-else>授权已经生效。关闭此窗口后，顶栏和页面上的限制提示会马上更新。</p>
            <p v-if="account && !buyingItem">到期时间：{{ commercialExpireText(account) }}</p>
          </div>
        </template>
        <template v-else>
          <section v-if="commercialUi.offer" class="offer-profile">
            <span class="offer-profile__mark" aria-hidden="true">
              <img v-if="offerIconUrl" :src="offerIconUrl" alt="" />
              <ArtSvgIcon v-else-if="offerIcon" :icon="offerIcon" />
              <span v-else>{{ offerInitial }}</span>
            </span>
            <div class="offer-profile__body">
              <div class="offer-profile__title">
                <strong>{{ commercialUi.offer.name }}</strong>
                <span
                  class="offer-profile__badge"
                  :class="commercialUi.offer.purchaseOnly ? 'is-third' : 'is-official'"
                  >{{ commercialUi.offer.purchaseOnly ? '第三方' : '官方' }}</span
                >
              </div>
              <p v-if="offerMeta" class="offer-profile__meta">{{ offerMeta }}</p>
              <p
                v-if="commercialUi.offer.summary"
                class="offer-profile__summary"
                :class="{ 'is-open': summaryOpen }"
                >{{ commercialUi.offer.summary }}</p
              >
              <button
                v-if="commercialUi.offer.summary"
                type="button"
                class="account-bar__link"
                @click="summaryOpen = !summaryOpen"
                >{{ summaryOpen ? '收起简介' : '查看详情' }}</button
              >
            </div>
          </section>
          <div v-if="showBoundAccount" class="account-bar">
            <div class="account-bar__id">
              <span class="account-bar__name">{{ maskedAccount }}</span>
              <span class="account-bar__role">{{ accountRoleLabel }}</span>
            </div>
            <div class="account-bar__links">
              <a
                class="account-bar__link"
                :href="manageAuthHref"
                target="_blank"
                rel="noopener noreferrer"
                >管理授权</a
              >
              <button type="button" class="account-bar__link" @click="switchBinding"
                >切换绑定</button
              >
            </div>
          </div>
          <div v-else-if="needsBind" class="bind-banner">
            <span>{{ bindBannerText }}</span>
            <a
              class="account-bar__link"
              :href="registerHref"
              target="_blank"
              rel="noopener noreferrer"
              >注册账号</a
            >
          </div>
          <p v-else class="upgrade-tip">尚未经源站确认绑定，确认后才显示当前账号。</p>
          <ElAlert
            v-if="account?.domainMismatch"
            type="warning"
            :closable="false"
            show-icon
            title="当前访问域名与授权域名不一致，付费能力暂按免费版处理。"
          />
          <template v-if="action === 'view' && !commercialUi.offer && !needsBind">
            <CommercialLicenseCard :account="account" @refreshed="onAccountRefreshed" />
          </template>
          <template v-else-if="needsBind">
            <p v-if="commercialUi.offer" class="bind-hint">{{ bindPriceHint }}</p>
            <ElForm label-width="72px" class="upgrade-form">
              <ElFormItem label="身份">
                <ElRadioGroup v-model="form.role">
                  <ElRadio value="user">用户</ElRadio>
                  <ElRadio value="agent">代理商</ElRadio>
                </ElRadioGroup>
              </ElFormItem>
              <ElFormItem label="账号">
                <ElInput
                  ref="bindAccountInput"
                  v-model.trim="form.account"
                  placeholder="邮箱或账号"
                />
              </ElFormItem>
              <ElFormItem label="密码">
                <ElInput
                  v-model="form.password"
                  type="password"
                  show-password
                  placeholder="仅用于本次登录，不会保存"
                />
              </ElFormItem>
            </ElForm>
            <div class="bind-actions">
              <ElButton type="primary" :loading="acting" :disabled="!!siteProblem" @click="bind"
                >登录并绑定</ElButton
              >
            </div>
            <p v-if="!siteProblem" class="upgrade-tip"
              >将使用站点域名
              {{ account?.requestDomain || '（未识别）' }} 绑定，域名不可在此修改。</p
            >
          </template>
          <template v-else-if="payUrl">
            <div
              v-if="payOptions.length"
              class="pay-methods"
              role="radiogroup"
              aria-label="支付方式"
            >
              <button
                v-for="option in payOptions"
                :key="option.code"
                type="button"
                class="pay-methods__item"
                :class="{ 'is-selected': payMethod === option.code }"
                @click="choosePayMethod(option.code)"
              >
                <ArtSvgIcon v-if="option.icon" :icon="option.icon" :style="payIconStyle(option)" />
                <span>{{ option.label }}</span>
              </button>
            </div>
            <div class="pay-panel">
              <div class="upgrade-qr">
                <QrcodeVue :value="payUrl" :size="188" />
              </div>
              <div class="pay-panel__meta">
                <p class="pay-panel__title">{{ orderTitle }}</p>
                <p class="pay-panel__amount"
                  >应付金额 <strong>¥{{ yuanText(payAmountCents) }}</strong></p
                >
                <p class="pay-panel__count">剩余 {{ countdownText }}</p>
                <p v-if="payStatus" class="upgrade-status">{{ payStatus }}</p>
                <p class="upgrade-tip"
                  >安全支付：二维码由本站生成。也可以打开付款页，本站不保存支付密码。</p
                >
                <ElButton @click="openPayPage">打开付款页</ElButton>
              </div>
            </div>
            <ElAlert v-if="payFailed" type="error" :closable="false" show-icon :title="payFailed" />
            <ElButton v-if="payFailed" @click="resetPay">{{
              buyingItem ? '返回' : '重新选择套餐'
            }}</ElButton>
          </template>
          <template v-else-if="buyingItem && commercialUi.offer">
            <section class="item-offer">
              <p class="section-title">选择购买方式</p>
              <div
                class="choice-grid"
                :class="{ 'is-single': commercialUi.offer.purchaseOnly }"
                role="radiogroup"
                aria-label="购买方式"
              >
                <button
                  type="button"
                  class="choice-card"
                  :class="{ 'is-selected': itemChoice === 'item' }"
                  @click="itemChoice = 'item'"
                >
                  <span class="choice-card__name">{{ singleBuyTitle }}</span>
                  <span class="choice-card__price">
                    {{ commercialYuanText(commercialUi.offer.priceCents) }}
                    <small>/ {{ offerPeriod }}</small>
                  </span>
                  <span class="choice-card__note">{{
                    commercialUi.offer.purchaseOnly ? purchaseOnlyNote : singleUnlockNote
                  }}</span>
                  <span v-if="itemChoice === 'item'" class="choice-card__check" aria-hidden="true"
                    ><ArtSvgIcon icon="ri:check-line"
                  /></span>
                </button>
                <button
                  v-if="!commercialUi.offer.purchaseOnly && recommendedPlan"
                  type="button"
                  class="choice-card"
                  :class="{ 'is-selected': itemChoice === 'plan' }"
                  @click="itemChoice = 'plan'"
                >
                  <span class="choice-card__badge">推荐</span>
                  <span class="choice-card__name">商业版套餐</span>
                  <span class="choice-card__price">
                    {{ commercialYuanText(recommendedPlan.priceCents) }}
                    <small>/ {{ slashPeriod(recommendedPlan.period) }}</small>
                  </span>
                  <span class="choice-card__note">含全部官方付费插件和模板，应用数不限</span>
                  <span v-if="itemChoice === 'plan'" class="choice-card__check" aria-hidden="true"
                    ><ArtSvgIcon icon="ri:check-line"
                  /></span>
                </button>
              </div>
              <p
                v-if="!commercialUi.offer.purchaseOnly && !loading && !recommendedPlan"
                class="upgrade-tip"
                >源站尚未配置可购买的套餐。</p
              >
              <section v-if="showCompare" class="compare">
                <div class="compare__head">
                  <span>功能</span>
                  <span>免费版</span>
                  <span>商业版</span>
                </div>
                <div v-for="row in commercialCompareRows" :key="row.label" class="compare__row">
                  <span class="compare__label">{{ row.label }}</span>
                  <span class="compare__free">{{ row.free }}</span>
                  <span class="compare__paid">{{ row.commercial }}</span>
                </div>
              </section>
              <div
                v-if="payOptions.length"
                class="pay-methods"
                role="radiogroup"
                aria-label="支付方式"
              >
                <button
                  v-for="option in payOptions"
                  :key="option.code"
                  type="button"
                  class="pay-methods__item"
                  :class="{ 'is-selected': payMethod === option.code }"
                  @click="selectPayMethod(option.code)"
                >
                  <ArtSvgIcon
                    v-if="option.icon"
                    :icon="option.icon"
                    :style="payIconStyle(option)"
                  />
                  <span>{{ option.label }}</span>
                </button>
              </div>
              <ElButton type="primary" class="pay-submit" :loading="acting" @click="paySelected">{{
                payButtonText
              }}</ElButton>
            </section>
          </template>
          <template v-else>
            <section v-if="showCompare" class="compare">
              <div class="compare__head">
                <span>功能</span>
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
                  <small>¥</small>{{ yuanWhole(plan.priceCents)
                  }}<small>.{{ yuanFrac(plan.priceCents) }}</small>
                </span>
                <span class="plan-card__period">{{
                  commercialPeriodText(plan.period) || '按约定时长'
                }}</span>
              </button>
            </div>
            <div
              v-if="plans.length && payOptions.length"
              class="pay-methods"
              role="radiogroup"
              aria-label="支付方式"
            >
              <button
                v-for="option in payOptions"
                :key="option.code"
                type="button"
                class="pay-methods__item"
                :class="{ 'is-selected': payMethod === option.code }"
                @click="selectPayMethod(option.code)"
              >
                <ArtSvgIcon v-if="option.icon" :icon="option.icon" :style="payIconStyle(option)" />
                <span>{{ option.label }}</span>
              </button>
            </div>
            <ElButton
              type="primary"
              class="pay-submit"
              :loading="acting"
              :disabled="!planId"
              @click="pay"
              >{{ payButtonText }}</ElButton
            >
            <p v-if="!plans.length" class="upgrade-tip">源站尚未配置可购买的套餐。</p>
          </template>
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
        <CommercialLicenseCard v-else :account="account" @refreshed="onAccountRefreshed" />
      </div>
      <template #footer>
        <ElButton type="primary" @click="commercialUi.licenseOpen = false">关闭</ElButton>
      </template>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
  import { ElMessage, ElMessageBox } from 'element-plus'
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
    commercialYuanText,
    logoutFailureIsAlreadyGone,
    purchaseNeedsRebind,
    purchaseRebindNotice,
    rememberCommercialAccount,
    requestCatalogResume,
    sourceConfirmedBound
  } from '@/utils/commercial'
  import {
    bindStoreAccount,
    createStoreEditionOrder,
    createStoreItemOrder,
    fetchStoreAccount,
    fetchStoreEditionOrder,
    fetchStorePlans,
    logoutStoreAccount,
    type StoreAccount,
    type StorePayOption,
    type StorePlan
  } from '@/api/store'

  // 管理授权和注册都开源站新窗口。用户和代理商的「我的授权」路径不同。
  const sourceSite = 'https://auth.maizll.com'
  const registerHref = `${sourceSite}/user/login`

  const loading = ref(false)
  const acting = ref(false)
  const loadError = ref('')
  const account = ref<StoreAccount | null>(null)
  const action = computed(() => commercialCta(account.value))
  const upgraded = ref(false)
  const choosingEdition = ref(false)
  // 单品窗口里选商业版套餐时，付的是商业版，成功后不要再去启用这条单品。
  const settleAsEdition = ref(false)
  const itemChoice = ref<'item' | 'plan'>('item')
  const summaryOpen = ref(false)
  const itemContinued = ref(false)
  // 未绑定、源站判定绑定失效，或这次请求被要求重绑，都回到绑定步骤。本地旧记录不能单独算已绑定。
  const needsBind = computed(() => purchaseNeedsRebind(account.value, commercialUi.rebindRequired))
  const rebindNotice = computed(() =>
    purchaseRebindNotice(account.value, commercialUi.rebindRequired)
  )
  const pendingItem = computed(() => !!commercialUi.offer && !choosingEdition.value)
  const buyingItem = computed(() => pendingItem.value && !needsBind.value)
  // 卡片上的期限用短写法：永久、年、N 天。
  function slashPeriod(period?: string) {
    const text = commercialPeriodText(period)
    if (text === '一年') return '年'
    return text || '按约定时长'
  }
  const offerPeriod = computed(() => slashPeriod(commercialUi.offer?.period))
  const singleBuyTitle = computed(() =>
    commercialUi.offer?.kind === 'template' ? '单独购买此模板' : '单独购买此插件'
  )
  const purchaseOnlyNote = '由第三方开发者提供，商业版不包含，需要单独购买'
  const singleUnlockNote = computed(() =>
    commercialUi.offer?.kind === 'template' ? '只解锁这一个模板' : '只解锁这一个插件'
  )
  const bindBannerText = computed(() =>
    rebindNotice.value ? '之前的绑定已失效，请重新绑定' : '未绑定账号，绑定后才能购买'
  )
  const offerIcon = computed(() => {
    const icon = (commercialUi.offer?.icon || '').trim()
    if (!icon || icon === 'ri:puzzle-line' || /^https?:\/\//i.test(icon)) return ''
    return icon
  })
  const offerIconUrl = computed(() => {
    const icon = (commercialUi.offer?.icon || '').trim()
    return /^https?:\/\//i.test(icon) ? icon : ''
  })
  const offerInitial = computed(() => (commercialUi.offer?.name || '插').trim().slice(0, 1))
  const offerMeta = computed(() => {
    const parts: string[] = []
    const version = (commercialUi.offer?.version || '').trim()
    const author = (commercialUi.offer?.author || '').trim()
    if (version)
      parts.push(version.toLowerCase().startsWith('v') ? `版本 ${version}` : `版本 v${version}`)
    if (author) parts.push(`开发者：${author}`)
    return parts.join(' · ')
  })
  const bindPriceHint = computed(() => {
    const offer = commercialUi.offer
    if (!offer) return ''
    const money = `${commercialYuanText(offer.priceCents)} / ${slashPeriod(offer.period)}`
    if (offer.purchaseOnly) return `单独购买 ${money}`
    return `单独购买 ${money}，也可选商业版（绑定后选择）`
  })
  const dialogTitle = computed(() => {
    if (upgraded.value) {
      if (pendingItem.value) return itemContinued.value ? '已购买并启用' : '已购买'
      return '升级完成'
    }
    if (pendingItem.value) return commercialUi.offer?.kind === 'template' ? '购买模板' : '购买插件'
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
  const payOptions = ref<StorePayOption[]>([])
  const payMethod = ref('')
  const payUrl = ref('')
  const orderTitle = ref('')
  const payAmountCents = ref(0)
  const recommendedPlan = computed(() => plans.value[0])
  // 按钮金额跟当前选中的卡片走：单品窗口选套餐时用套餐价，否则用这条插件或模板的价格。
  const payButtonCents = computed(() => {
    if (buyingItem.value && itemChoice.value === 'plan' && recommendedPlan.value) {
      return recommendedPlan.value.priceCents
    }
    if (buyingItem.value && commercialUi.offer) return commercialUi.offer.priceCents
    const plan = plans.value.find((item) => item.id === planId.value)
    return plan?.priceCents || 0
  })
  const payButtonText = computed(() => `立即支付 ${commercialYuanText(payButtonCents.value)}`)
  // 官方单品和套餐窗口都展示对照表。第三方条目商业版不包含，不放这张表。
  const showCompare = computed(() => {
    if (needsBind.value || payUrl.value) return false
    if (buyingItem.value) return !commercialUi.offer?.purchaseOnly
    return action.value !== 'view'
  })
  const payStatus = ref('')
  const payFailed = ref('')
  const showBrandBanner = computed(
    () => !upgraded.value && action.value !== 'view' && !pendingItem.value
  )
  const purchaseModalClass = computed(() =>
    showBrandBanner.value
      ? 'commercial-purchase-modal is-brand'
      : 'commercial-purchase-modal is-plain'
  )
  const successTitle = ref('已升级为商业版')
  const form = reactive({ account: '', password: '', role: 'user' })
  const bindAccountInput = ref<{ focus?: () => void } | null>(null)
  const purchaseDialogWidth = ref('720px')
  // 账号栏的「当前账号」只认源站这次核对。本地快照仍显示已绑定、但 sourceVerified 还没回来时，不显示账号。
  const showBoundAccount = computed(() => sourceConfirmedBound(account.value) && !needsBind.value)
  const accountRoleLabel = computed(() => (account.value?.role === 'agent' ? '代理商' : '用户'))
  const maskedAccount = computed(() => maskAccount(account.value?.account || ''))
  const manageAuthHref = computed(() =>
    account.value?.role === 'agent'
      ? `${sourceSite}/agent-panel/licenses`
      : `${sourceSite}/user/licenses`
  )
  const siteProblem = computed(
    () => account.value?.connectionIssues?.find((item) => item.field === 'site')?.message || ''
  )
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
    payAmountCents.value = 0
  }

  function syncPurchaseWidth() {
    purchaseDialogWidth.value = window.matchMedia('(max-width: 767px)').matches ? '100%' : '720px'
  }

  // 邮箱留首字符和域名，手机号留前三后二，其余只留首字。空名字显示「已绑定」，避免把已绑定画成未绑定。
  function maskAccount(raw: string) {
    const text = raw.trim()
    if (!text) return '已绑定'
    const at = text.indexOf('@')
    if (at > 0) return `${text.slice(0, 1)}***${text.slice(at)}`
    if (/^\d{7,}$/.test(text)) return `${text.slice(0, 3)}****${text.slice(-2)}`
    return `${text.slice(0, 1)}***`
  }

  function applyPayOptions(options?: StorePayOption[]) {
    payOptions.value = options || []
    if (!payOptions.value.some((item) => item.code === payMethod.value)) {
      payMethod.value = payOptions.value[0]?.code || ''
    }
  }

  function selectPayMethod(code: string) {
    payMethod.value = code
  }

  // 付款码已经出来后再换支付方式，用同一套餐或单品重新下单，不另开窗口。
  async function choosePayMethod(code: string) {
    const changed = payMethod.value !== code
    payMethod.value = code
    if (!changed || !payUrl.value || acting.value) return
    if (settleAsEdition.value) await pay()
    else if (buyingItem.value) await buyItem()
    else await pay()
  }

  function payIconStyle(option: StorePayOption) {
    const color = (option.color || '').trim().toLowerCase()
    // 品牌色只涂图标。黄、金不用来强调选中项，选中框跟主题蓝。
    if (!color || /f6bf53|ffd700|gold|e6a23c|f5c518|ffc107|f0b429/.test(color)) return undefined
    return { color }
  }

  function yuanText(cents: number) {
    return `${yuanWhole(cents)}.${yuanFrac(cents)}`
  }

  async function switchBinding() {
    try {
      await ElMessageBox.confirm('解除当前绑定后，需要用另一个账号重新登录才能购买。', '切换绑定', {
        confirmButtonText: '解除并重新绑定',
        cancelButtonText: '取消',
        type: 'warning'
      })
    } catch {
      return
    }
    await releaseBinding()
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
    payFailed.value = buyingItem.value
      ? '支付等待超时。若已经付款，请关闭窗口后刷新应用商店；若尚未付款，请重新生成付款码。'
      : '支付等待超时。若已经付款，请关闭窗口后看顶栏是否已变为商业版；若尚未付款，请重新生成付款码。'
    ElMessage.error(payFailed.value)
  }

  function applyAccount(next: StoreAccount) {
    account.value = next
    rememberCommercialAccount(next)
  }

  function onAccountRefreshed(next: StoreAccount) {
    applyAccount(next)
  }

  async function loadPurchase() {
    stopPoll()
    upgraded.value = false
    itemContinued.value = false
    choosingEdition.value = false
    settleAsEdition.value = false
    itemChoice.value = 'item'
    summaryOpen.value = false
    successTitle.value = commercialUi.offer ? '已购买' : '已升级为商业版'
    resetPay()
    loadError.value = ''
    loading.value = true
    try {
      const next = await fetchStoreAccount(true)
      applyAccount(next)
      // bindingInvalid 自己会带说明。不要再把 rebindRequired 打成 true，否则随后的刷新会丢掉具体原因。
      if (next.bindingInvalid) commercialUi.rebindRequired = false
      else if (next.explicitRevoked) commercialUi.rebindRequired = true
      else if (sourceConfirmedBound(next)) commercialUi.rebindRequired = false
      if (!purchaseNeedsRebind(next, commercialUi.rebindRequired)) {
        // 永久商业版不再卖套餐，但单独购买插件仍要拿到源站开启的收款方式。
        const data = await fetchStorePlans()
        applyPayOptions(data.payOptions)
        if (commercialCta(next) !== 'view') {
          plans.value = data.list || []
          planId.value = plans.value[0]?.id
        } else {
          plans.value = []
          planId.value = undefined
        }
      } else {
        plans.value = []
        planId.value = undefined
      }
    } catch (error: unknown) {
      loadError.value =
        caughtErrorText(error, '读取商业版信息失败', errorAlreadyToasted(error)) ||
        '读取商业版信息失败'
      showCaughtError(error, '读取商业版信息失败')
    } finally {
      loading.value = false
    }
  }

  async function loadLicense() {
    loadError.value = ''
    loading.value = true
    try {
      const next = await fetchStoreAccount(true)
      applyAccount(next)
    } catch (error: unknown) {
      loadError.value =
        caughtErrorText(error, '读取授权信息失败', errorAlreadyToasted(error)) || '读取授权信息失败'
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
      const keptPlan = planId.value
      const next = await bindStoreAccount({ ...form })
      applyAccount(next)
      form.password = ''
      if (!sourceConfirmedBound(next)) {
        commercialUi.rebindRequired =
          !next.bindingInvalid && (!!next.explicitRevoked || !next.bound)
        return
      }
      commercialUi.rebindRequired = false
      ElMessage.success('绑定成功')
      const data = await fetchStorePlans()
      applyPayOptions(data.payOptions)
      // 单品窗口也要套餐列表，绑定成功后才能画出「商业版套餐」那张卡。
      plans.value = data.list || []
      if (!commercialUi.offer || choosingEdition.value) {
        planId.value = plans.value.some((item) => item.id === keptPlan)
          ? keptPlan
          : plans.value[0]?.id
        if (!plans.value.length) {
          ElMessage.warning('源站尚未配置可购买的套餐')
        }
      }
    } catch (error: unknown) {
      showCaughtError(error, '绑定失败，请检查源站是否可访问')
    } finally {
      acting.value = false
    }
  }

  async function paySelected() {
    if (itemChoice.value === 'plan' && recommendedPlan.value && !commercialUi.offer?.purchaseOnly) {
      planId.value = recommendedPlan.value.id
      settleAsEdition.value = true
      await pay()
      return
    }
    settleAsEdition.value = false
    itemChoice.value = 'item'
    await buyItem()
  }

  function startPayWait(payLink: string, orderNo: string, title: string, amountCents: number) {
    payUrl.value = payLink
    orderTitle.value = title
    payAmountCents.value = amountCents
    payDeadline = Date.now() + payWindowMs
    nowTick.value = Date.now()
    stopTick()
    tickTimer = setInterval(() => {
      nowTick.value = Date.now()
      if (payDeadline && nowTick.value >= payDeadline) expirePayWait()
    }, 1000)
    poll(orderNo)
  }

  async function buyItem() {
    const offer = commercialUi.offer
    if (!offer?.id) return
    acting.value = true
    payFailed.value = ''
    try {
      const order = await createStoreItemOrder(offer.kind, offer.id, payMethod.value)
      if (!order?.payUrl || !order.orderNo) {
        payFailed.value = '源站没有返回付款码，无法继续支付。'
        ElMessage.error(payFailed.value)
        return
      }
      successTitle.value = '已购买'
      startPayWait(order.payUrl, order.orderNo, order.title || offer.name, order.amountCents)
    } catch (error: unknown) {
      showCaughtError(error, '创建订单失败')
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
      const order = await createStoreEditionOrder(planId.value, payMethod.value)
      if (!order?.payUrl || !order.orderNo) {
        payFailed.value = '源站没有返回付款码，无法继续支付。'
        ElMessage.error(payFailed.value)
        return
      }
      successTitle.value = buyingUpgrade ? '已升级为商业版' : '商业版已续费'
      startPayWait(order.payUrl, order.orderNo, order.title, order.amountCents)
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
          payFailed.value = buyingItem.value
            ? '这笔订单已退款，购买没有生效。'
            : '这笔订单已退款，商业版没有生效。'
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
        if (commercialUi.rebindRequired) {
          stopPoll()
          resetPay()
          return
        }
        pollFailures += 1
        payStatus.value = '暂时连不上源站，正在重试'
        if (pollFailures >= 5) {
          stopPoll()
          payStatus.value = ''
          payFailed.value =
            caughtErrorText(
              error,
              '无法确认支付结果，请检查网络后重试',
              errorAlreadyToasted(error)
            ) || '无法确认支付结果，请检查网络后重试'
          showCaughtError(error, '无法确认支付结果，请检查网络后重试')
        }
      }
    }, 2000)
  }

  async function finishPaid() {
    payStatus.value = '支付成功，正在同步授权'
    try {
      const next = await fetchStoreAccount(true)
      applyAccount(next)
    } catch (error: unknown) {
      showCaughtError(error, '支付已完成，但读取授权失败。关闭窗口后会再试一次。')
    }
    payUrl.value = ''
    if (commercialUi.offer && !choosingEdition.value && !settleAsEdition.value) {
      const resume = commercialUi.offer.resume
      if (resume) {
        try {
          itemContinued.value = await resume()
        } catch {
          itemContinued.value = false
        }
      } else {
        itemContinued.value = false
      }
      successTitle.value = itemContinued.value ? '已购买并启用' : '已购买'
    }
    upgraded.value = true
    pendingRefresh = true
    payStatus.value = ''
  }

  async function resumeItem() {
    const offer = commercialUi.offer
    if (!offer) return
    acting.value = true
    try {
      if (offer.resume) {
        itemContinued.value = await offer.resume()
      } else {
        requestCatalogResume(offer)
        itemContinued.value = true
      }
      if (itemContinued.value) successTitle.value = '已购买并启用'
    } catch {
      itemContinued.value = false
      ElMessage.error('已购买，但启用失败。请回到应用商店再试一次。')
    } finally {
      acting.value = false
    }
  }

  function onPurchaseClosed() {
    stopPoll()
    if (pendingRefresh) {
      pendingRefresh = false
      window.dispatchEvent(new Event('store-account-refresh'))
    }
    upgraded.value = false
    itemContinued.value = false
    choosingEdition.value = false
    settleAsEdition.value = false
    itemChoice.value = 'item'
    summaryOpen.value = false
    commercialUi.offer = null
    resetPay()
  }

  async function releaseBinding() {
    acting.value = true
    try {
      try {
        await logoutStoreAccount()
      } catch (error: unknown) {
        if (!logoutFailureIsAlreadyGone(error)) throw error
      }
      resetPay()
      const next = await fetchStoreAccount(true)
      applyAccount(next)
      commercialUi.rebindRequired = !!next.bindingInvalid || !!next.explicitRevoked
    } catch (error: unknown) {
      showCaughtError(error, '解除绑定失败')
    } finally {
      acting.value = false
    }
  }

  watch(
    () => commercialUi.rebindRequired,
    async (required) => {
      if (!required) return
      resetPay()
      try {
        const next = await fetchStoreAccount(true)
        applyAccount(next)
      } catch {
        if (account.value) account.value = { ...account.value, bound: false, explicitRevoked: true }
      }
    }
  )

  onMounted(() => {
    syncPurchaseWidth()
    window.addEventListener('resize', syncPurchaseWidth)
  })

  onBeforeUnmount(() => {
    stopPoll()
    stopTick()
    window.removeEventListener('resize', syncPurchaseWidth)
  })
</script>

<style scoped>
  .upgrade-body {
    display: flex;
    flex-direction: column;
    gap: 14px;
    max-width: 100%;
    overflow-x: hidden;
  }

  .edition-banner {
    display: flex;
    gap: 12px;
    align-items: flex-start;
    margin: -16px
      calc(-16px - var(--el-dialog-padding-primary, 16px) - var(--el-message-close-size, 16px))
      0 -16px;
    padding: 18px 56px 18px 18px;
    color: #f7faff;
    background: linear-gradient(135deg, #0b1f4d 0%, #1d4ed8 58%, #38bdf8 100%);
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
    color: rgb(247 250 255 / 88%);
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
    color: var(--el-color-primary);
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
    color: var(--el-color-primary-light-3);
  }

  .offer-profile {
    display: flex;
    gap: 12px;
    align-items: flex-start;
    padding: 14px;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 12px;
  }

  .offer-profile__mark {
    display: flex;
    flex: none;
    align-items: center;
    justify-content: center;
    width: 48px;
    height: 48px;
    overflow: hidden;
    color: #fff;
    font-size: 22px;
    font-weight: 600;
    background: var(--el-color-primary);
    border-radius: 12px;
  }

  .offer-profile__mark img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .offer-profile__body {
    min-width: 0;
  }

  .offer-profile__title {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
  }

  .offer-profile__title strong {
    color: var(--el-text-color-primary);
    font-size: 16px;
    line-height: 1.4;
  }

  .offer-profile__badge {
    padding: 0 6px;
    font-size: 12px;
    line-height: 18px;
    border-radius: 4px;
  }

  .offer-profile__badge.is-official {
    color: var(--el-color-primary);
    background: var(--el-color-primary-light-9);
    border: 1px solid var(--el-color-primary-light-7);
  }

  .offer-profile__badge.is-third {
    color: var(--el-text-color-secondary);
    background: var(--el-fill-color);
    border: 1px solid var(--el-border-color);
  }

  .offer-profile__meta,
  .offer-profile__summary {
    margin: 4px 0 0;
    color: var(--el-text-color-secondary);
    font-size: 13px;
    line-height: 1.5;
  }

  .offer-profile__summary {
    display: -webkit-box;
    overflow: hidden;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
  }

  .offer-profile__summary.is-open {
    display: block;
    overflow: visible;
    -webkit-line-clamp: unset;
  }

  .account-bar {
    display: flex;
    flex-wrap: nowrap;
    gap: 8px;
    align-items: center;
    justify-content: space-between;
    min-width: 0;
    padding: 10px 12px;
    background: var(--el-color-primary-light-9);
    border: 1px solid var(--el-color-primary-light-8);
    border-radius: 10px;
  }

  .account-bar__id {
    display: flex;
    flex: 1;
    gap: 6px;
    align-items: center;
    min-width: 0;
  }

  .account-bar__name {
    overflow: hidden;
    color: var(--el-color-primary);
    font-size: 14px;
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .account-bar__role {
    flex: none;
    padding: 0 6px;
    color: var(--el-text-color-regular);
    font-size: 12px;
    line-height: 18px;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 4px;
  }

  .account-bar__links,
  .bind-actions {
    display: flex;
    flex: none;
    flex-wrap: nowrap;
    gap: 12px;
    align-items: center;
  }

  .bind-banner {
    display: flex;
    gap: 8px;
    align-items: center;
    justify-content: space-between;
    padding: 10px 12px;
    color: #9a3412;
    font-size: 13px;
    line-height: 1.5;
    background: #fff7ed;
    border: 1px solid #fdba74;
    border-radius: 10px;
  }

  .bind-hint {
    margin: 0;
    color: var(--el-text-color-regular);
    font-size: 13px;
    line-height: 1.5;
  }

  .bind-actions :deep(.el-button),
  .pay-submit {
    width: 100%;
    height: 44px;
    margin-left: 0;
  }

  .account-bar__link {
    padding: 0;
    color: var(--el-color-primary);
    font-size: 13px;
    line-height: 1.5;
    text-decoration: none;
    cursor: pointer;
    background: none;
    border: 0;
  }

  .pay-methods {
    display: flex;
    gap: 10px;
  }

  .pay-methods__item {
    display: inline-flex;
    flex: 1;
    gap: 6px;
    align-items: center;
    justify-content: center;
    min-width: 0;
    min-height: 40px;
    padding: 6px 12px;
    color: var(--el-text-color-primary);
    cursor: pointer;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color);
    border-radius: 10px;
  }

  .pay-methods__item.is-selected {
    color: var(--el-color-primary);
    border-color: var(--el-color-primary);
    box-shadow: 0 0 0 1px var(--el-color-primary);
  }

  .pay-panel__amount {
    margin: 6px 0 0;
    color: var(--el-text-color-primary);
    font-size: 14px;
  }

  .pay-panel__amount strong {
    color: var(--el-color-primary);
    font-size: 22px;
  }

  .item-offer {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .item-offer__name,
  .item-offer__price,
  .item-offer__note {
    margin: 0;
  }

  .item-offer__name {
    color: var(--el-text-color-primary);
    font-size: 16px;
    font-weight: 600;
  }

  .item-offer__price {
    color: var(--el-color-primary);
    font-size: 22px;
    font-weight: 700;
  }

  .item-offer__note {
    color: var(--el-text-color-secondary);
    font-size: 13px;
  }

  .choice-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
  }

  .choice-grid.is-single {
    grid-template-columns: 1fr;
  }

  .choice-card {
    position: relative;
    display: flex;
    flex-direction: column;
    gap: 6px;
    align-items: flex-start;
    min-width: 0;
    padding: 14px 28px 28px 12px;
    overflow: hidden;
    text-align: left;
    cursor: pointer;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color);
    border-radius: 12px;
  }

  .choice-card.is-selected {
    background: var(--el-color-primary-light-9);
    border-color: var(--el-color-primary);
  }

  .choice-card__badge {
    position: absolute;
    top: 0;
    right: 0;
    padding: 2px 8px;
    color: #fff;
    font-size: 12px;
    line-height: 18px;
    background: var(--el-color-primary);
    border-bottom-left-radius: 8px;
  }

  .choice-card__check {
    position: absolute;
    right: 8px;
    bottom: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    color: #fff;
    font-size: 12px;
    background: var(--el-color-primary);
    border-radius: 50%;
  }

  .choice-card__name,
  .choice-card__plan,
  .choice-card__period,
  .choice-card__note {
    max-width: 100%;
    overflow-wrap: anywhere;
  }

  .choice-card__name {
    color: var(--el-text-color-primary);
    font-size: 14px;
    font-weight: 600;
  }

  .choice-card__plan,
  .choice-card__period,
  .choice-card__note {
    color: var(--el-text-color-secondary);
    font-size: 12px;
  }

  .choice-card__price {
    color: var(--el-color-primary);
    font-size: 28px;
    font-weight: 700;
    line-height: 1;
  }

  .choice-card__price small {
    color: var(--el-text-color-secondary);
    font-size: 13px;
    font-weight: 400;
  }

  .item-offer__actions {
    display: flex;
    flex-direction: column;
    gap: 10px;
    align-items: stretch;
  }

  .item-offer__actions :deep(.el-button) {
    height: auto;
    min-height: 36px;
    margin-left: 0;
    white-space: normal;
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
    border-color: var(--el-color-primary);
    box-shadow: 0 0 0 2px var(--el-color-primary-light-7);
  }

  .plan-card__ribbon {
    position: absolute;
    top: 8px;
    right: -28px;
    padding: 2px 32px;
    color: #fff;
    font-size: 12px;
    background: linear-gradient(180deg, var(--el-color-primary-light-3), var(--el-color-primary));
    transform: rotate(35deg);
  }

  .plan-card__name {
    color: var(--el-text-color-primary);
    font-size: 14px;
  }

  .plan-card__price {
    color: var(--el-color-primary);
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
    max-width: 100%;
    padding: 8px;
    background: #fff;
    border-radius: 12px;
  }

  .upgrade-qr :deep(canvas),
  .upgrade-qr :deep(svg) {
    max-width: 100%;
    height: auto;
  }

  .pay-panel__title {
    color: var(--el-text-color-primary);
    font-weight: 600;
  }

  .pay-panel__count {
    margin-top: 6px;
    color: var(--el-color-primary);
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
    background: var(--el-color-primary-light-3);
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
    color: var(--el-color-primary);
    animation: mark-pop 500ms ease-out both;
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

  @keyframes mark-pop {
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
      gap: 4px;
      padding: 8px;
    }

    .compare__label,
    .compare__free,
    .compare__paid,
    .compare__head {
      font-size: 12px;
    }

    .choice-card {
      padding: 12px 22px 26px 10px;
    }

    .choice-card__name {
      font-size: 13px;
    }

    .choice-card__price {
      font-size: 22px;
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
    color: #f7faff;
  }

  @media (max-width: 767px) {
    .commercial-purchase-modal.el-overlay,
    .commercial-purchase-modal .el-overlay-dialog {
      overflow-x: hidden;
    }

    .commercial-purchase-modal .el-overlay-dialog {
      padding: 0;
    }

    .commercial-purchase-modal .el-dialog {
      width: 100vw !important;
      max-width: 100vw !important;
      margin: 0 !important;
      border-radius: 0;
    }

    .commercial-purchase-modal .el-dialog__body {
      max-width: 100%;
      max-height: calc(100vh - 120px);
      overflow: auto;
      overflow-x: hidden;
    }
  }
</style>
