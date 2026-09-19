<template>
  <div class="developer-apply">
    <header class="page-hero">
      <div>
        <p class="eyebrow">SOURCE DEVELOPER</p>
        <h1>开发者入驻</h1>
        <p>申请源站开发者账号。审核通过后，可登录开发者端提交插件与首页模板元数据。</p>
      </div>
      <div class="hero-aside">
        <span>开发者账号独立于代理商登录</span>
        <strong>需单独设置用户名和密码</strong>
        <small>与代理商面板账号不互通，请妥善保存</small>
      </div>
    </header>

    <section v-if="view === 'status' && applyStatus" class="result-card">
      <div class="result-icon" :class="`is-${statusMeta.type}`">
        <iconify-icon :icon="statusIcon" width="32" />
      </div>
      <p class="result-kicker">{{ statusMeta.label }}</p>
      <h2>{{ statusTitle }}</h2>
      <p class="result-description">{{ statusMeta.description }}</p>
      <div class="result-grid">
        <div>
          <span>开发者用户名</span>
          <strong>{{ applyStatus.username }}</strong>
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
        <el-button v-if="applyStatus.status === 'approved'" type="primary" @click="goDeveloperLogin">
          前往开发者登录
          <iconify-icon icon="ri:arrow-right-line" />
        </el-button>
        <el-button v-else-if="applyStatus.status === 'rejected'" type="primary" @click="resetToForm">
          更换用户名重新申请
        </el-button>
        <el-button
          v-else-if="applyStatus.status === 'frozen' || applyStatus.status === 'cancelled'"
          type="primary"
          @click="resetToForm"
        >
          更换用户名重新申请
        </el-button>
      </div>
    </section>

    <div v-else class="apply-grid">
      <section class="form-card">
        <div class="section-title">
          <span>01</span>
          <div>
            <h2>填写入驻资料</h2>
            <p>提交后由管理员在源站后台审核</p>
          </div>
        </div>

        <el-form
          ref="formRef"
          :model="form"
          :rules="rules"
          label-position="top"
          @keyup.enter="submitApply"
        >
          <el-form-item label="开发者用户名" prop="username">
            <el-input
              v-model="form.username"
              maxlength="59"
              placeholder="2-59 位小写字母、数字或连字符"
              @blur="form.username = normalizeUsername(form.username)"
            />
          </el-form-item>
          <el-form-item label="开发者登录密码" prop="password">
            <el-input
              v-model="form.password"
              type="password"
              show-password
              maxlength="64"
              placeholder="至少 6 位，用于开发者端登录"
            />
          </el-form-item>
          <el-form-item label="显示名" prop="displayName">
            <el-input v-model="form.displayName" maxlength="80" placeholder="默认使用用户名" />
          </el-form-item>
          <el-form-item label="邮箱" prop="email">
            <el-input v-model="form.email" maxlength="100" placeholder="便于审核联系" />
          </el-form-item>
          <el-form-item label="申请说明" prop="reason">
            <el-input
              v-model="form.reason"
              type="textarea"
              :rows="4"
              maxlength="500"
              show-word-limit
              placeholder="说明计划上架的插件或模板，以及使用场景"
            />
          </el-form-item>
          <el-button
            type="primary"
            size="large"
            class="submit-button"
            :loading="submitting"
            @click="submitApply"
          >
            <iconify-icon icon="ri:send-plane-line" />
            提交入驻申请
          </el-button>
        </el-form>
      </section>

      <aside class="tips-card">
        <div class="section-title">
          <span>02</span>
          <div>
            <h2>审核说明</h2>
            <p>提交后可随时查询进度</p>
          </div>
        </div>
        <ul class="tips-list">
          <li>
            <iconify-icon icon="ri:shield-check-line" />
            管理员在源站「入驻审核」中处理：待审核可选择通过或拒绝；已通过后可取消开发者资格。
          </li>
          <li>
            <iconify-icon icon="ri:lock-line" />
            开发者密码与代理商登录密码相互独立，请勿混用。
          </li>
          <li>
            <iconify-icon icon="ri:time-line" />
            审核期间可在本页刷新状态；通过后即可进入开发者端。
          </li>
        </ul>

        <div class="lookup">
          <p class="lookup-label">已提交过申请？查询审核状态</p>
          <el-input
            v-model="lookupUsername"
            maxlength="59"
            placeholder="输入开发者用户名"
            @keyup.enter="lookupStatus"
          >
            <template #append>
              <el-button :loading="checking" @click="lookupStatus">查询</el-button>
            </template>
          </el-input>
        </div>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, reactive, ref } from 'vue'
  import { useRouter } from 'vue-router'
  import { Icon as IconifyIcon } from '@iconify/vue'
  import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
  import {
    DEVELOPER_APPLY_STATUS,
    DEVELOPER_USERNAME_PATTERN,
    applySourceDeveloper,
    fetchSourceDeveloperApplyStatus,
    loadRememberedDeveloperApplyUsername,
    rememberDeveloperApplyUsername,
    type SourceDeveloperApplyStatus
  } from '@/api/source-developer'

  const router = useRouter()
  const formRef = ref<FormInstance>()
  const submitting = ref(false)
  const checking = ref(false)
  const view = ref<'form' | 'status'>('form')
  const applyStatus = ref<SourceDeveloperApplyStatus | null>(null)
  const lookupUsername = ref('')

  const form = reactive({
    username: '',
    password: '',
    email: '',
    displayName: '',
    reason: ''
  })

  const rules: FormRules = {
    username: [
      { required: true, message: '请输入开发者用户名', trigger: 'blur' },
      {
        validator: (_rule, value: string, callback) => {
          if (!DEVELOPER_USERNAME_PATTERN.test(normalizeUsername(value))) {
            callback(new Error('用户名需为 2-59 位小写字母、数字或连字符'))
            return
          }
          callback()
        },
        trigger: 'blur'
      }
    ],
    password: [
      { required: true, message: '请设置开发者登录密码', trigger: 'blur' },
      { min: 6, message: '密码至少 6 位', trigger: 'blur' }
    ],
    email: [
      {
        validator: (_rule, value: string, callback) => {
          if (!value || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim())) {
            callback()
            return
          }
          callback(new Error('请输入有效邮箱'))
        },
        trigger: 'blur'
      }
    ],
    reason: [{ required: true, message: '请填写申请说明', trigger: 'blur' }]
  }

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
        return '申请已通过，可登录开发者端'
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

  function normalizeUsername(value: string) {
    return value.trim().toLowerCase()
  }

  function readAgentProfile() {
    try {
      return JSON.parse(localStorage.getItem('agent_panel_info') || '{}') as {
        email?: string
        name?: string
      }
    } catch {
      return {}
    }
  }

  function showStatus(data: SourceDeveloperApplyStatus) {
    applyStatus.value = data
    view.value = 'status'
    lookupUsername.value = data.username
    rememberDeveloperApplyUsername(data.username)
  }

  async function loadStatus(username: string, silent = false) {
    const value = normalizeUsername(username)
    if (!value) {
      if (!silent) ElMessage.warning('请输入开发者用户名')
      return false
    }
    checking.value = true
    try {
      const { data } = await fetchSourceDeveloperApplyStatus(value)
      if (data.code === 200 && data.data?.status) {
        showStatus(data.data)
        return true
      }
      if (data.code === 404) {
        if (!silent) ElMessage.warning(data.msg || '未找到入驻申请')
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
    if (!applyStatus.value?.username) return
    const found = await loadStatus(applyStatus.value.username)
    if (found) ElMessage.success('状态已更新')
  }

  async function lookupStatus() {
    await loadStatus(lookupUsername.value)
  }

  function resetToForm() {
    view.value = 'form'
    applyStatus.value = null
    form.username = ''
    form.password = ''
    form.reason = ''
  }

  function goDeveloperLogin() {
    const username = applyStatus.value?.username || ''
    router.push({
      path: '/developer-panel/login',
      query: username ? { username, approved: '1' } : { approved: '1' }
    })
  }

  async function submitApply() {
    if (!formRef.value || submitting.value) return
    const valid = await formRef.value.validate().catch(() => false)
    if (!valid) return

    submitting.value = true
    try {
      const payload = {
        username: normalizeUsername(form.username),
        password: form.password,
        email: form.email.trim(),
        displayName: form.displayName.trim(),
        reason: form.reason.trim()
      }
      const { data } = await applySourceDeveloper(payload)
      if (data.code !== 200) {
        ElMessage.error(data.msg || '提交申请失败')
        if (data.code === 400 && /审核中/.test(data.msg || '')) {
          await loadStatus(payload.username, true)
        }
        return
      }
      rememberDeveloperApplyUsername(payload.username)
      ElMessage.success(data.msg || '入驻申请已提交')
      await loadStatus(payload.username, true)
      if (view.value !== 'status') {
        showStatus({
          id: data.data?.id,
          username: data.data?.username || payload.username,
          status: data.data?.status || 'pending'
        })
      }
    } catch {
      ElMessage.error('提交申请失败，请稍后重试')
    } finally {
      submitting.value = false
    }
  }

  onMounted(async () => {
    const profile = readAgentProfile()
    form.email = profile.email || ''
    form.displayName = profile.name || ''
    lookupUsername.value = loadRememberedDeveloperApplyUsername()
    if (lookupUsername.value) {
      await loadStatus(lookupUsername.value, true)
    }
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
    margin: 0 0 18px;
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

  .lookup-label {
    margin: 0 0 8px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
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
