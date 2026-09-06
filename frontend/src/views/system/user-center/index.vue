<!-- 个人中心页面 -->
<template>
  <div class="user-center-page w-full min-h-full pb-8">
    <!-- 顶部个人背景与核心信息横幅 -->
    <div
      class="art-card-sm user-hero-card mb-5 overflow-hidden border border-g-300/60 dark:border-g-800"
    >
      <div class="user-hero-cover relative h-40 w-full overflow-hidden">
        <img class="w-full h-full object-cover select-none" src="@imgs/user/bg.webp" alt="cover" />
        <div class="hero-cover-mask absolute inset-0"></div>
        <div class="absolute right-5 top-5 flex items-center gap-2">
          <span
            class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-medium text-white/90 bg-black/35 backdrop-blur-md border border-white/15"
          >
            <span class="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
            账号状态正常
          </span>
        </div>
      </div>

      <div
        class="user-hero-body relative px-6 pb-5 pt-0 flex flex-wrap items-end justify-between gap-4"
      >
        <div class="flex items-end gap-5">
          <div
            class="user-avatar-wrap relative -mt-11 rounded-full p-1 bg-[var(--default-box-color)] shadow-md"
          >
            <img
              class="w-22 h-22 rounded-full object-cover border-2 border-white/80 dark:border-g-700 shadow-sm"
              src="@imgs/user/avatar.webp"
              alt="avatar"
            />
          </div>
          <div class="pb-1">
            <div class="flex items-center gap-3">
              <h1 class="text-xl font-semibold text-g-900 tracking-tight">{{ displayName }}</h1>
              <span
                class="px-2.5 py-0.5 rounded-full text-xs font-medium bg-primary-100 text-primary-700 dark:bg-primary-950/60 dark:text-primary-300 border border-primary-300/40 dark:border-primary-800/40"
              >
                {{ userRoleName }}
              </span>
            </div>
            <p class="mt-1 text-xs text-g-500 flex items-center gap-2">
              <span>账号：{{ userInfo.userName || '-' }}</span>
              <span class="text-g-300 dark:text-g-700">|</span>
              <span class="flex items-center gap-1">
                <ArtSvgIcon icon="ri:mail-line" class="text-xs text-g-400" />
                {{ userInfo.email || '未绑定邮箱' }}
              </span>
            </p>
          </div>
        </div>

        <div class="flex items-center gap-2 max-sm:w-full max-sm:justify-end">
          <ElButton
            v-if="!isEdit"
            type="primary"
            plain
            class="!h-9 !px-4"
            v-ripple
            @click="isEdit = true"
          >
            <ArtSvgIcon icon="ri:edit-box-line" class="mr-1.5" />
            编辑基本资料
          </ElButton>
          <ElButton v-if="!isEditPwd" class="!h-9 !px-4" v-ripple @click="isEditPwd = true">
            <ArtSvgIcon icon="ri:lock-password-line" class="mr-1.5" />
            修改密码
          </ElButton>
        </div>
      </div>
    </div>

    <!-- 下半部分两列并排网格：左侧基本信息，右侧安全设置 -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-5 items-start">
      <!-- 基本资料模块 -->
      <div class="art-card-sm border border-g-300/60 dark:border-g-800 overflow-hidden">
        <div
          class="p-4 px-5 border-b border-g-200/80 dark:border-g-800/80 flex items-center justify-between"
        >
          <div class="flex items-center gap-2">
            <div
              class="w-8 h-8 rounded-lg bg-primary-100 text-primary-700 dark:bg-primary-950/80 dark:text-primary-300 flex items-center justify-center text-base"
            >
              <ArtSvgIcon icon="ri:user-settings-line" />
            </div>
            <div>
              <h2 class="text-base font-semibold text-g-900">基本信息</h2>
              <p class="text-xs text-g-500">管理管理员账号昵称与通知邮箱</p>
            </div>
          </div>
          <ElTag v-if="isEdit" type="warning" size="small" effect="plain">编辑中</ElTag>
          <ElTag v-else type="info" size="small" effect="plain"
            >UID: #{{ userInfo.userId || '-' }}</ElTag
          >
        </div>

        <div class="p-5">
          <ElForm
            :model="form"
            ref="ruleFormRef"
            :rules="rules"
            label-width="90px"
            label-position="top"
          >
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-x-4">
              <ElFormItem label="登录账号">
                <ElInput :model-value="userInfo.userName" disabled placeholder="账号不可更改">
                  <template #prefix>
                    <ArtSvgIcon icon="ri:user-line" class="text-g-400" />
                  </template>
                </ElInput>
              </ElFormItem>

              <ElFormItem label="用户角色">
                <ElInput :model-value="userRoleName" disabled>
                  <template #prefix>
                    <ArtSvgIcon icon="ri:shield-star-line" class="text-g-400" />
                  </template>
                </ElInput>
              </ElFormItem>
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-x-4">
              <ElFormItem label="个人昵称" prop="nickname">
                <ElInput
                  v-model="form.nickname"
                  :disabled="!isEdit"
                  maxlength="50"
                  show-word-limit
                  placeholder="请输入您的昵称"
                >
                  <template #prefix>
                    <ArtSvgIcon icon="ri:account-circle-line" class="text-g-400" />
                  </template>
                </ElInput>
              </ElFormItem>

              <ElFormItem label="联系邮箱" prop="email">
                <ElInput
                  v-model="form.email"
                  :disabled="!isEdit"
                  placeholder="请输入用于系统接收通知的邮箱"
                >
                  <template #prefix>
                    <ArtSvgIcon icon="ri:mail-line" class="text-g-400" />
                  </template>
                </ElInput>
              </ElFormItem>
            </div>

            <div
              class="flex items-center justify-end gap-3 pt-4 border-t border-g-200/60 dark:border-g-800/60 mt-1"
            >
              <ElButton v-if="isEdit" class="!px-5" v-ripple @click="cancelEdit"> 取消 </ElButton>
              <ElButton
                type="primary"
                class="!px-6"
                v-ripple
                :loading="profileLoading"
                @click="handleProfile"
              >
                {{ isEdit ? '保存更改' : '编辑资料' }}
              </ElButton>
            </div>
          </ElForm>
        </div>
      </div>

      <!-- 安全设置模块 -->
      <div class="art-card-sm border border-g-300/60 dark:border-g-800 overflow-hidden">
        <div
          class="p-4 px-5 border-b border-g-200/80 dark:border-g-800/80 flex items-center justify-between"
        >
          <div class="flex items-center gap-2">
            <div
              class="w-8 h-8 rounded-lg bg-amber-100 text-amber-700 dark:bg-amber-950/80 dark:text-amber-300 flex items-center justify-center text-base"
            >
              <ArtSvgIcon icon="ri:lock-password-line" />
            </div>
            <div>
              <h2 class="text-base font-semibold text-g-900">安全设置</h2>
              <p class="text-xs text-g-500">定期更改密码提升管理员账号安全性</p>
            </div>
          </div>
          <ElTag v-if="isEditPwd" type="warning" size="small" effect="plain">修改中</ElTag>
          <ElTag v-else type="success" size="small" effect="light">密码受保护</ElTag>
        </div>

        <div class="p-5">
          <ElForm
            :model="pwdForm"
            ref="pwdFormRef"
            :rules="pwdRules"
            label-width="90px"
            label-position="top"
          >
            <ElFormItem label="当前密码" prop="oldPassword">
              <ElInput
                v-model="pwdForm.oldPassword"
                type="password"
                :disabled="!isEditPwd"
                show-password
                placeholder="请输入当前正在使用的密码"
              >
                <template #prefix>
                  <ArtSvgIcon icon="ri:lock-2-line" class="text-g-400" />
                </template>
              </ElInput>
            </ElFormItem>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-x-4">
              <ElFormItem label="新密码" prop="newPassword">
                <ElInput
                  v-model="pwdForm.newPassword"
                  type="password"
                  :disabled="!isEditPwd"
                  show-password
                  placeholder="请输入新密码（至少6位）"
                >
                  <template #prefix>
                    <ArtSvgIcon icon="ri:key-line" class="text-g-400" />
                  </template>
                </ElInput>
              </ElFormItem>

              <ElFormItem label="确认新密码" prop="confirmPassword">
                <ElInput
                  v-model="pwdForm.confirmPassword"
                  type="password"
                  :disabled="!isEditPwd"
                  show-password
                  placeholder="请再次输入新密码"
                >
                  <template #prefix>
                    <ArtSvgIcon icon="ri:check-double-line" class="text-g-400" />
                  </template>
                </ElInput>
              </ElFormItem>
            </div>

            <div
              class="flex items-center justify-end gap-3 pt-4 border-t border-g-200/60 dark:border-g-800/60 mt-1"
            >
              <ElButton v-if="isEditPwd" class="!px-5" v-ripple @click="cancelEditPwd">
                取消
              </ElButton>
              <ElButton
                type="primary"
                class="!px-6"
                v-ripple
                :loading="pwdLoading"
                @click="handlePwd"
              >
                {{ isEditPwd ? '确认修改' : '修改密码' }}
              </ElButton>
            </div>
          </ElForm>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
  import { useUserStore } from '@/store/modules/user'
  import { fetchGetUserInfo, fetchUpdateUserInfo, fetchChangePassword } from '@/api/auth'
  import { ElMessage } from 'element-plus'
  import type { FormInstance, FormRules } from 'element-plus'

  defineOptions({ name: 'UserCenter' })

  const userStore = useUserStore()
  const userInfo = computed(() => userStore.getUserInfo)
  const displayName = computed(() => userInfo.value.nickname || userInfo.value.userName || '管理员')
  const userRoles = computed(() => userInfo.value.roles || [])
  const userRoleName = computed(() => {
    const roles = userRoles.value
    if (!roles || roles.length === 0) return '超级管理员'
    if (roles.includes('R_SUPER')) return '超级管理员'
    if (roles.includes('R_ADMIN')) return '管理员'
    return roles.join('、')
  })

  const isEdit = ref(false)
  const isEditPwd = ref(false)
  const profileLoading = ref(false)
  const pwdLoading = ref(false)
  const ruleFormRef = ref<FormInstance>()
  const pwdFormRef = ref<FormInstance>()

  /**
   * 基本设置表单，字段与 admins 表可编辑列一一对应
   */
  const form = reactive({
    nickname: '',
    email: ''
  })

  const pwdForm = reactive({
    oldPassword: '',
    newPassword: '',
    confirmPassword: ''
  })

  const rules = reactive<FormRules>({
    nickname: [
      { required: true, message: '请输入昵称', trigger: 'blur' },
      { min: 2, max: 50, message: '长度在 2 到 50 个字符', trigger: 'blur' }
    ],
    email: [
      { required: true, message: '请输入邮箱', trigger: 'blur' },
      { type: 'email', message: '邮箱格式不正确', trigger: 'blur' }
    ]
  })

  const pwdRules = reactive<FormRules>({
    oldPassword: [{ required: true, message: '请输入当前密码', trigger: 'blur' }],
    newPassword: [
      { required: true, message: '请输入新密码', trigger: 'blur' },
      { min: 6, message: '新密码至少 6 位', trigger: 'blur' }
    ],
    confirmPassword: [
      { required: true, message: '请再次输入新密码', trigger: 'blur' },
      {
        validator: (_rule, value, callback) => {
          if (value !== pwdForm.newPassword) {
            callback(new Error('两次输入的新密码不一致'))
          } else {
            callback()
          }
        },
        trigger: 'blur'
      }
    ]
  })

  /**
   * 用 store 中的用户信息回填表单
   */
  const syncFormFromStore = () => {
    form.nickname = userInfo.value.nickname || ''
    form.email = userInfo.value.email || ''
  }

  onMounted(syncFormFromStore)

  const cancelEdit = () => {
    syncFormFromStore()
    ruleFormRef.value?.clearValidate()
    isEdit.value = false
  }

  const handleProfile = async () => {
    if (!isEdit.value) {
      syncFormFromStore()
      isEdit.value = true
      return
    }

    const valid = await ruleFormRef.value?.validate().catch(() => false)
    if (!valid) return

    const nickname = form.nickname.trim()
    const email = form.email.trim()
    if (nickname === (userInfo.value.nickname || '') && email === (userInfo.value.email || '')) {
      isEdit.value = false
      return
    }

    profileLoading.value = true
    try {
      await fetchUpdateUserInfo({ nickname, email })
      const latest = await fetchGetUserInfo()
      userStore.setUserInfo(latest)
      syncFormFromStore()
      isEdit.value = false
      ElMessage.success('资料更新成功')
    } catch {
      // 错误提示由 http 拦截器统一处理
    } finally {
      profileLoading.value = false
    }
  }

  const resetPwdForm = () => {
    pwdForm.oldPassword = ''
    pwdForm.newPassword = ''
    pwdForm.confirmPassword = ''
    pwdFormRef.value?.clearValidate()
  }

  const cancelEditPwd = () => {
    resetPwdForm()
    isEditPwd.value = false
  }

  const handlePwd = async () => {
    if (!isEditPwd.value) {
      resetPwdForm()
      isEditPwd.value = true
      return
    }

    const valid = await pwdFormRef.value?.validate().catch(() => false)
    if (!valid) return

    pwdLoading.value = true
    try {
      await fetchChangePassword({
        oldPassword: pwdForm.oldPassword,
        newPassword: pwdForm.newPassword,
        confirmPassword: pwdForm.confirmPassword
      })
      resetPwdForm()
      isEditPwd.value = false
      ElMessage.success('密码已修改，请重新登录')
      setTimeout(() => userStore.logOut(), 800)
    } catch {
      // 错误提示由 http 拦截器统一处理
    } finally {
      pwdLoading.value = false
    }
  }
</script>

<style scoped lang="scss">
  .user-center-page {
    .user-hero-cover {
      .hero-cover-mask {
        background: linear-gradient(180deg, rgba(0, 0, 0, 0.1) 0%, rgba(0, 0, 0, 0.45) 100%);
      }
    }
  }
</style>
