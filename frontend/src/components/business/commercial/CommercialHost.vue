<template>
  <div>
    <ElDialog v-model="commercialUi.promptOpen" title="需要商业版" width="460px" append-to-body>
      <CommercialMark :text="commercialUi.promptText" icon="ri:vip-crown-fill" />
      <template #footer>
        <ElButton @click="commercialUi.promptOpen = false">知道了</ElButton>
        <ElButton type="primary" @click="goUpgrade">{{ promptActionLabel }}</ElButton>
      </template>
    </ElDialog>

    <ElDialog v-model="commercialUi.upgradeOpen" :title="dialogTitle" width="480px" append-to-body @open="load">
      <div v-loading="loading" class="upgrade-body">
        <CommercialMark
          v-if="account && account.edition !== 'commercial'"
          text="绑定源站账号后，在本窗口完成支付，无需跳转网页。"
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
          <p>已绑定 {{ account?.account || '账号' }}，{{ action === 'renew' ? '请选择续费套餐。' : '请选择套餐。' }}</p>
          <ElSelect v-model="planId" placeholder="选择套餐" style="width: 100%">
            <ElOption
              v-for="plan in plans"
              :key="plan.id"
              :label="`${plan.name} ${(plan.priceCents / 100).toFixed(2)} 元`"
              :value="plan.id"
            />
          </ElSelect>
          <ElButton type="primary" :loading="acting" :disabled="!planId" @click="pay">生成付款码</ElButton>
          <p v-if="!plans.length" class="upgrade-tip">源站尚未配置可购买的套餐。</p>
        </template>
        <template v-else>
          <p>请使用支付宝或微信扫描付款。支付完成后本页会自动刷新，无需新开窗口。</p>
          <div class="upgrade-qr">
            <QrcodeVue :value="payUrl" :size="200" />
          </div>
          <p class="upgrade-tip">{{ orderTitle }}</p>
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
      </div>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { computed, onBeforeUnmount, reactive, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import { showCaughtError } from '@/utils/http/error-toast'
  import QrcodeVue from 'qrcode.vue'
  import CommercialMark from './CommercialMark.vue'
  import { commercialCta, commercialCtaLabel, commercialUi, rememberCommercialAccount } from '@/utils/commercial'
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
  const account = ref<StoreAccount | null>(null)
  const action = computed(() => commercialCta(account.value))
  const dialogTitle = computed(() => {
    if (action.value === 'view') return '查看授权'
    if (action.value === 'renew') return '续费'
    return '升级商业版'
  })
  const promptActionLabel = computed(() => commercialCtaLabel(commercialCta(commercialUi.account)))
  const plans = ref<StorePlan[]>([])
  const planId = ref<number>()
  const payUrl = ref('')
  const orderTitle = ref('')
  const form = reactive({ account: '', password: '', role: 'user' })
  const connection = reactive({ sourceBase: 'https://auth.maizll.com', siteUrl: '', trustProxy: false })
  let pollTimer: ReturnType<typeof setInterval> | null = null

  function goUpgrade() {
    commercialUi.promptOpen = false
    commercialUi.upgradeOpen = true
  }

  async function load() {
    loading.value = true
    try {
      account.value = await fetchStoreAccount()
      rememberCommercialAccount(account.value)
      connection.sourceBase = account.value.sourceBase || connection.sourceBase
      connection.siteUrl = account.value.siteUrl || ''
      connection.trustProxy = !!account.value.trustProxy
      if (account.value.bound) {
        const data = await fetchStorePlans()
        plans.value = data.list || []
        planId.value = plans.value[0]?.id
      }
    } catch (error: any) {
      showCaughtError(error, '读取商店账号失败')
    } finally {
      loading.value = false
    }
  }

  async function bind() {
    acting.value = true
    try {
      account.value = await bindStoreAccount({ ...form })
      rememberCommercialAccount(account.value)
      form.password = ''
      ElMessage.success('已绑定，请继续支付')
      const data = await fetchStorePlans()
      plans.value = data.list || []
      planId.value = plans.value[0]?.id
    } catch (error: any) {
      showCaughtError(error, '绑定失败')
    } finally {
      acting.value = false
    }
  }

  async function pay() {
    if (!planId.value) return
    acting.value = true
    try {
      const order = await createStoreEditionOrder(planId.value)
      payUrl.value = order.payUrl
      orderTitle.value = `${order.title} ${(order.amountCents / 100).toFixed(2)} 元`
      poll(order.orderNo)
    } catch (error: any) {
      showCaughtError(error, '创建订单失败')
    } finally {
      acting.value = false
    }
  }

  function poll(orderNo: string) {
    if (pollTimer) clearInterval(pollTimer)
    pollTimer = setInterval(async () => {
      try {
        const order = await fetchStoreEditionOrder(orderNo)
        if (order.status === 'paid') {
          if (pollTimer) clearInterval(pollTimer)
          ElMessage.success('商业版已生效')
          commercialUi.upgradeOpen = false
          payUrl.value = ''
          window.dispatchEvent(new Event('store-account-refresh'))
        }
      } catch {
        /* 轮询失败时保留二维码，下一轮再试 */
      }
    }, 2000)
  }

  async function saveConnection() {
    acting.value = true
    try {
      await saveStoreConnection({ ...connection })
    } catch (error: any) {
      showCaughtError(error, '保存失败')
    } finally {
      acting.value = false
    }
  }

  onBeforeUnmount(() => {
    if (pollTimer) clearInterval(pollTimer)
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

  .upgrade-tip {
    margin: 0;
    color: var(--el-text-color-secondary);
    font-size: 12px;
  }

  .upgrade-qr {
    display: flex;
    justify-content: center;
  }
</style>
