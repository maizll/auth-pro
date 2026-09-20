<template>
  <div class="developer-apply">
    <header class="page-hero">
      <div>
        <p class="eyebrow">SOURCE DEVELOPER</p>
        <h1>开发者入驻</h1>
        <p>使用当前代理商账号一键申请。审核通过后，无需另设用户名密码即可进入开发者端。</p>
      </div>
      <div class="hero-aside">
        <span>复用代理商登录</span>
        <strong>一键申请，无需新账号</strong>
        <small>管理员在源站「入驻审核」通过后即可进入</small>
      </div>
    </header>

    <section v-if="applyStatus" class="result-card">
      <div class="result-icon" :class="`is-${statusMeta.type}`">
        <iconify-icon :icon="statusIcon" width="32" />
      </div>
      <p class="result-kicker">{{ statusMeta.label }}</p>
      <h2>{{ statusTitle }}</h2>
      <p class="result-description">{{ statusMeta.description }}</p>
      <div class="result-grid">
        <div>
          <span>代理商账号</span>
          <strong>{{ applyStatus.displayName || applyStatus.username || agentLabel }}</strong>
        </div>
        <div>
          <span>当前状态</span>
          <strong>
            <el-tag :type="statusMeta.type" size="small">{{ statusMeta.label }}</el-tag>
          </strong>
        </div>
      </div>
      <el-alert
        v-if="applyStatus.reviewNote"
        class="review-note"
        :title="applyStatus.reviewNote"
        type="info"
        :closable="false"
        show-icon
      />
      <div class="result-actions">
        <el-button :loading="checking" @click="refreshStatus">刷新状态</el-button>
        <el-button v-if="applyStatus.status === 'approved'" type="primary" @click="enterDeveloper">
          进入开发者端
          <iconify-icon icon="ri:arrow-right-line" />
        </el-button>
        <el-button
          v-else-if="applyStatus.status === 'rejected' || applyStatus.status === 'frozen' || applyStatus.status === 'cancelled'"
          type="primary"
          :loading="submitting"
          @click="submitApply"
        >
          重新申请
        </el-button>
      </div>
    </section>

    <div v-else class="apply-grid">
      <section class="form-card">
        <div class="section-title">
          <span>01</span>
          <div>
            <h2>一键申请</h2>
            <p>提交后由管理员在源站后台审核，不另设开发者密码</p>
          </div>
        </div>
        <p class="apply-copy">
          将以当前登录的代理商账号（{{ agentLabel }}）申请开发者资格。通过后使用同一套登录凭证进入开发者端。
        </p>
        <el-button
          type="primary"
          size="large"
          class="submit-button"
          :loading="submitting"
          @click="submitApply"
        >
          <iconify-icon icon="ri:send-plane-line" />
          申请成为开发者
        </el-button>
      </section>

      <aside class="tips-card">
        <div class="section-title">
          <span>02</span>
          <div>
            <h2>审核说明</h2>
            <p>申请页只保留待审核；通过/拒绝后不再保留申请单</p>
          </div>
        </div>
        <ul class="tips-list">
          <li>
            <iconify-icon icon="ri:shield-check-line" />
            管理员在源站「入驻审核」中处理：待审核可选择通过或拒绝；取消开发者会删除资格记录，可重新申请。
          </li>
          <li>
            <iconify-icon icon="ri:user-shared-line" />
            开发者端与代理商面板共用当前登录，无需第二套用户名密码。
          </li>
          <li>
            <iconify-icon icon="ri:time-line" />
            审核期间可在本页刷新状态；通过后即可进入开发者端。
          </li>
        </ul>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, ref } from 'vue'
  import { useRouter } from 'vue-router'
  import { Icon as IconifyIcon } from '@iconify/vue'
  import { ElMessage } from 'element-plus'
  import {
    AGENT_INFO_KEY,
    DEVELOPER_APPLY_STATUS,
    applySourceDeveloper,
    enterDeveloperSessionFromAgent,
    fetchSourceDeveloperApplyStatus,
    type SourceDeveloperApplyStatus
  } from '@/api/source-developer'

  const router = useRouter()
  const submitting = ref(false)
  const checking = ref(false)
  const applyStatus = ref<SourceDeveloperApplyStatus | null>(null)

  const agentLabel = computed(() => {
    try {
      const info = JSON.parse(localStorage.getItem(AGENT_INFO_KEY) || '{}') as {
        email?: string
        name?: string
      }
      return info.name || info.email || '当前代理商'
    } catch {
      return '当前代理商'
    }
  })

  const statusMeta = computed(() => {
    const status = applyStatus.value?.status || ''
    return (
      DEVELOPER_APPLY_STATUS[status] || {
        label: status || '未知',
        type: 'info' as const,
        description: '无法识别当前申请状态。'
      }
    )
  })

  const statusTitle = computed(() => {
    switch (applyStatus.value?.status) {
      case 'pending':
        return '入驻申请审核中'
      case 'approved':
        return '申请已通过，可进入开发者端'
      case 'rejected':
        return '入驻申请未通过'
      case 'frozen':
      case 'cancelled':
        return '开发者资格已取消'
      default:
        return '申请状态'
    }
  })

  const statusIcon = computed(() => {
    switch (applyStatus.value?.status) {
      case 'approved':
        return 'ri:checkbox-circle-fill'
      case 'pending':
        return 'ri:time-fill'
      default:
        return 'ri:error-warning-fill'
    }
  })

  async function loadStatus(silent = false) {
    checking.value = true
    try {
      const { data } = await fetchSourceDeveloperApplyStatus()
      if (data.code === 200 && data.data?.status) {
        applyStatus.value = data.data
        return true
      }
      if (data.code === 404) {
        applyStatus.value = null
        return false
      }
      if (!silent) ElMessage.error(data.msg || '查询申请失败')
      return false
    } catch {
      if (!silent) ElMessage.error('查询申请失败')
      return false
    } finally {
      checking.value = false
    }
  }

  async function refreshStatus() {
    const found = await loadStatus()
    if (found) ElMessage.success('状态已更新')
  }

  function enterDeveloper() {
    if (!enterDeveloperSessionFromAgent()) {
      ElMessage.warning('请先使用代理商账号登录')
      router.push('/agent-panel/login')
      return
    }
    router.push('/developer-panel/dashboard')
  }

  async function submitApply() {
    if (submitting.value) return
    submitting.value = true
    try {
      const { data } = await applySourceDeveloper()
      if (data.code !== 200) {
        ElMessage.error(data.msg || '提交申请失败')
        if (data.code === 400) await loadStatus(true)
        return
      }
      ElMessage.success(data.msg || '入驻申请已提交')
      await loadStatus(true)
      if (!applyStatus.value) {
        applyStatus.value = {
          id: data.data?.id,
          username: data.data?.username || '',
          displayName: data.data?.displayName,
          status: data.data?.status || 'pending'
        }
      }
    } catch {
      ElMessage.error('提交申请失败，请稍后重试')
    } finally {
      submitting.value = false
    }
  }

  onMounted(async () => {
    await loadStatus(true)
  })
</script>

<style scoped lang="scss">
  .developer-apply {
    width: 100%;
    max-width: 1040px;
    margin: 0 auto;
    color: var(--el-text-color-primary);
  }

  .page-hero {
    display: flex;
    gap: 20px;
    align-items: center;
    justify-content: space-between;
    padding: 20px 22px;
    overflow: hidden;
    background:
      radial-gradient(circle at 82% 15%, rgb(64 158 255 / 14%), transparent 28%),
      linear-gradient(135deg, var(--el-bg-color) 0%, var(--el-color-primary-light-9) 100%);
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 14px;

    h1 {
      margin: 3px 0 6px;
      font-size: 24px;
      line-height: 1.2;
    }

    p:not(.eyebrow) {
      max-width: 620px;
      margin: 0;
      font-size: 13px;
      line-height: 1.6;
      color: var(--el-text-color-secondary);
    }
  }

  .eyebrow,
  .result-kicker {
    margin: 0;
    font-size: 11px;
    font-weight: 700;
    color: var(--el-color-primary);
    letter-spacing: 1.4px;
  }

  .hero-aside {
    display: flex;
    flex: 0 0 236px;
    flex-direction: column;
    justify-content: center;
    padding: 14px 18px;
    background: rgb(255 255 255 / 58%);
    backdrop-filter: blur(8px);
    border: 1px solid rgb(255 255 255 / 70%);
    border-radius: 12px;
    box-shadow: 0 8px 22px rgb(31 41 55 / 6%);

    span,
    small {
      color: var(--el-text-color-secondary);
    }

    span {
      font-size: 12px;
    }

    strong {
      margin: 4px 0;
      font-size: 15px;
      line-height: 1.4;
    }

    small {
      font-size: 11px;
      line-height: 1.45;
    }
  }

  .apply-grid {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 320px;
    gap: 14px;
    margin-top: 14px;
  }

  .form-card,
  .tips-card,
  .result-card {
    padding: 18px;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 14px;
    box-shadow: 0 5px 18px rgb(15 23 42 / 4%);
  }

  .section-title {
    display: flex;
    gap: 10px;
    align-items: center;
    margin-bottom: 16px;

    > span:first-child {
      display: grid;
      place-items: center;
      width: 30px;
      height: 30px;
      font-size: 11px;
      font-weight: 700;
      color: var(--el-color-primary);
      background: var(--el-color-primary-light-9);
      border-radius: 8px;
    }

    h2 {
      margin: 0;
      font-size: 16px;
    }

    p {
      margin: 2px 0 0;
      font-size: 11px;
      color: var(--el-text-color-secondary);
    }
  }

  .apply-copy {
    margin: 0 0 18px;
    font-size: 13px;
    line-height: 1.7;
    color: var(--el-text-color-regular);
  }

  .submit-button {
    width: 100%;
    margin-top: 4px;
  }

  .tips-card {
    align-self: start;
  }

  .tips-list {
    display: grid;
    gap: 10px;
    padding: 0;
    margin: 0;
    list-style: none;

    li {
      display: flex;
      gap: 8px;
      align-items: flex-start;
      font-size: 12px;
      line-height: 1.55;
      color: var(--el-text-color-regular);
    }

    svg {
      flex: 0 0 auto;
      margin-top: 2px;
      color: var(--el-color-primary);
    }
  }

  .result-card {
    max-width: 620px;
    padding: 32px;
    margin: 18px auto 0;
    text-align: center;

    h2 {
      margin: 6px 0;
      font-size: 22px;
    }
  }

  .result-icon {
    display: grid;
    place-items: center;
    width: 60px;
    height: 60px;
    margin: 0 auto 14px;
    color: #fff;
    border-radius: 18px;

    &.is-success {
      background: linear-gradient(135deg, var(--el-color-success), #20b985);
      box-shadow: 0 10px 22px rgb(32 185 133 / 22%);
    }

    &.is-warning {
      background: linear-gradient(135deg, var(--el-color-warning), #f5a623);
      box-shadow: 0 10px 22px rgb(245 166 35 / 22%);
    }

    &.is-danger,
    &.is-info {
      background: linear-gradient(135deg, var(--el-color-danger), #f56c6c);
      box-shadow: 0 10px 22px rgb(245 108 108 / 22%);
    }
  }

  .result-description {
    margin: 0;
    color: var(--el-text-color-secondary);
  }

  .result-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
    margin: 18px 0;

    div {
      display: flex;
      flex-direction: column;
      padding: 11px 8px;
      background: var(--el-fill-color-light);
      border-radius: 10px;
    }

    span {
      margin-bottom: 6px;
      font-size: 11px;
      color: var(--el-text-color-secondary);
    }
  }

  .review-note {
    margin-bottom: 16px;
    text-align: left;
  }

  .result-actions {
    display: flex;
    gap: 10px;
    justify-content: center;
  }

  @media (width <= 980px) {
    .apply-grid {
      grid-template-columns: 1fr;
    }
  }

  @media (width <= 680px) {
    .page-hero {
      align-items: stretch;
      flex-direction: column;
      gap: 12px;
      padding: 16px;
    }

    .hero-aside {
      flex-basis: auto;
    }

    .result-card,
    .form-card,
    .tips-card {
      padding: 14px;
    }

    .result-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
