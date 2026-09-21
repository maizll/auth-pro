<template>
  <div class="alipay-f2f-config-page">
    <ElCard v-loading="loading" class="config-card" shadow="never">
      <template #header>
        <div class="card-header">
          <div>
            <h2>支付宝当面付</h2>
            <p>配置开放平台当面付（正扫）。启用后会出现在收银台，用户扫商家收款码完成支付。</p>
          </div>
          <ElButton type="primary" :loading="saving" @click="handleSave">保存配置</ElButton>
        </div>
      </template>

      <ElForm :model="form" label-position="top" class="config-form">
        <section class="section-card">
          <div class="section-title with-switch">
            <div>
              <strong>开放平台凭证</strong>
              <span
                >应用私钥只写不读。沙箱与正式环境请使用对应 APPID /
                密钥，不要把生产私钥提交到仓库。</span
              >
            </div>
            <div class="enable-switch">
              <span>沙箱环境</span>
              <ElSwitch
                v-model="form.sandbox"
                inline-prompt
                active-text="沙箱"
                inactive-text="正式"
              />
            </div>
          </div>
          <ElRow :gutter="16">
            <ElCol :xs="24" :md="12">
              <ElFormItem label="APPID" required>
                <ElInput v-model.trim="form.appId" placeholder="例如 2021000000000000" />
              </ElFormItem>
            </ElCol>
            <ElCol :xs="24" :md="12">
              <ElFormItem label="应用私钥（RSA2）">
                <ElInput
                  v-model="form.privateKey"
                  type="textarea"
                  :rows="4"
                  :placeholder="form.privateKeySet ? '已设置，留空不修改' : 'PKCS#1 / PKCS#8 PEM'"
                />
              </ElFormItem>
            </ElCol>
            <ElCol :xs="24">
              <ElFormItem label="支付宝公钥" required>
                <ElInput
                  v-model="form.alipayPublicKey"
                  type="textarea"
                  :rows="4"
                  placeholder="开放平台「支付宝公钥」，PEM 或裸 base64"
                />
              </ElFormItem>
            </ElCol>
          </ElRow>
        </section>

        <section class="section-card">
          <div class="section-title">
            <div>
              <strong>网关与通知</strong>
              <span
                >均可留空：网关按沙箱开关选择默认地址；通知地址按当前域名生成
                /api/payment/alipay-f2f/notify。</span
              >
            </div>
          </div>
          <ElRow :gutter="16">
            <ElCol :xs="24" :md="12">
              <ElFormItem label="网关地址">
                <ElInput
                  v-model.trim="form.gateway"
                  :placeholder="
                    form.sandbox
                      ? 'https://openapi-sandbox.dl.alipaydev.com/gateway.do'
                      : 'https://openapi.alipay.com/gateway.do'
                  "
                />
              </ElFormItem>
            </ElCol>
            <ElCol :xs="24" :md="12">
              <ElFormItem label="异步通知 URL">
                <ElInput
                  v-model.trim="form.notifyUrl"
                  placeholder="https://your-host/api/payment/alipay-f2f/notify"
                />
              </ElFormItem>
            </ElCol>
          </ElRow>
        </section>

        <section class="section-card">
          <div class="section-title with-switch">
            <div>
              <strong>证书模式（可选）</strong>
              <span
                >公钥模式即可沙箱联调。证书模式请填写开放平台提供的
                SN，仍使用上面的支付宝公钥验签。</span
              >
            </div>
            <div class="enable-switch">
              <span>证书模式</span>
              <ElSwitch v-model="form.certMode" />
            </div>
          </div>
          <ElRow v-if="form.certMode" :gutter="16">
            <ElCol :xs="24" :md="12">
              <ElFormItem label="应用公钥证书 SN">
                <ElInput v-model.trim="form.appCertSn" placeholder="app_cert_sn" />
              </ElFormItem>
            </ElCol>
            <ElCol :xs="24" :md="12">
              <ElFormItem label="支付宝根证书 SN">
                <ElInput v-model.trim="form.alipayRootCertSn" placeholder="alipay_root_cert_sn" />
              </ElFormItem>
            </ElCol>
          </ElRow>
        </section>
      </ElForm>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
  import type { AlipayF2FConfigData } from '@/api/system-manage'
  import { fetchAlipayF2FConfig, fetchUpdateAlipayF2FConfig } from '@/api/system-manage'

  defineOptions({ name: 'AlipayF2FConfigCard' })

  const loading = ref(false)
  const saving = ref(false)
  const activated = ref(false)
  const form = reactive<AlipayF2FConfigData>({
    appId: '',
    privateKey: '',
    privateKeySet: false,
    alipayPublicKey: '',
    gateway: '',
    notifyUrl: '',
    sandbox: false,
    certMode: false,
    appCertSn: '',
    alipayRootCertSn: ''
  })

  const applyForm = (data: AlipayF2FConfigData) => {
    Object.assign(form, {
      appId: data.appId || '',
      privateKey: '',
      privateKeySet: Boolean(data.privateKeySet),
      alipayPublicKey: data.alipayPublicKey || '',
      gateway: data.gateway || '',
      notifyUrl: data.notifyUrl || '',
      sandbox: Boolean(data.sandbox),
      certMode: Boolean(data.certMode),
      appCertSn: data.appCertSn || '',
      alipayRootCertSn: data.alipayRootCertSn || ''
    })
  }

  const loadConfig = async () => {
    loading.value = true
    try {
      applyForm(await fetchAlipayF2FConfig())
    } finally {
      loading.value = false
    }
  }

  const handleSave = async () => {
    if (!form.appId.trim() || !form.alipayPublicKey.trim()) {
      ElMessage.warning('请填写 APPID 和支付宝公钥')
      return
    }
    if (!form.privateKeySet && !form.privateKey?.trim()) {
      ElMessage.warning('请填写应用私钥')
      return
    }
    if (form.certMode && (!form.appCertSn.trim() || !form.alipayRootCertSn.trim())) {
      ElMessage.warning('证书模式请填写两个证书 SN')
      return
    }
    saving.value = true
    try {
      const data = await fetchUpdateAlipayF2FConfig({ ...form })
      applyForm(data)
      ElMessage.success('支付宝当面付配置已保存')
    } catch (error: any) {
      ElMessage.error(error?.message || '保存失败')
    } finally {
      saving.value = false
    }
  }

  onMounted(async () => {
    await loadConfig()
    activated.value = true
  })

  onActivated(() => {
    if (!activated.value) return
    loadConfig()
  })
</script>

<style scoped>
  .alipay-f2f-config-page {
    height: 100%;
  }

  .config-card {
    height: 100%;
    border: none;
  }

  .card-header {
    display: flex;
    gap: 16px;
    align-items: flex-start;
    justify-content: space-between;
  }

  .card-header h2 {
    margin: 0 0 4px;
    font-size: 18px;
  }

  .card-header p {
    margin: 0;
    color: var(--el-text-color-secondary);
  }

  .section-card {
    padding: 16px 16px 4px;
    margin-bottom: 20px;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 12px;
  }

  .section-title {
    display: flex;
    gap: 16px;
    justify-content: space-between;
    margin-bottom: 12px;
  }

  .section-title strong {
    display: block;
    margin-bottom: 4px;
  }

  .section-title span {
    font-size: 13px;
    color: var(--el-text-color-secondary);
  }

  .enable-switch {
    display: flex;
    gap: 8px;
    align-items: center;
    white-space: nowrap;
  }
</style>
