<!--
  强制更新提醒（仅超级管理员）：后台顶部固定一条不能关闭的红色横幅，写明新版本号并带「立即更新」；
  每次登录后弹一次说明，关掉后照常使用。数据来自服务端每小时一次的更新检查。
-->
<template>
  <div v-if="show" class="force-update-banner" role="alert">
    <ArtSvgIcon icon="ri:error-warning-fill" class="banner-icon" />
    <span class="banner-text">
      <b>必须更新到 v{{ notice?.version }}</b>
      <span class="banner-sub">{{ subText }}</span>
    </span>
    <ElButton type="danger" size="small" class="banner-action" @click="goUpdate">立即更新</ElButton>
  </div>

  <AppDialog v-model="dialogVisible" title="有必须安装的更新" size="sm">
    <div class="force-update-dialog">
      <p class="dialog-lead">
        官方发布了 <b>v{{ notice?.version }}</b
        >，并标记为强制更新，通常包含安全修复，请尽快安装。
      </p>
      <ul v-if="notes.length" class="dialog-notes">
        <li v-for="note in notes" :key="note">{{ note }}</li>
      </ul>
      <p class="dialog-muted">{{ subText }}</p>
    </div>
    <template #footer>
      <ElButton @click="dialogVisible = false">稍后</ElButton>
      <ElButton type="danger" @click="goUpdate">立即更新</ElButton>
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
  import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
  import { useRoute, useRouter } from 'vue-router'
  import AppDialog from '@/components/core/dialog/AppDialog.vue'
  import { FORCE_NOTICE_DIALOG_KEY, useUserStore } from '@/store/modules/user'
  import { forceNotice, refreshForceNotice } from './force-notice'

  defineOptions({ name: 'ForceUpdateNotice' })

  const router = useRouter()
  const route = useRoute()
  const userStore = useUserStore()
  const isSuper = computed(() => (userStore.getUserInfo.roles || []).includes('R_SUPER'))
  const notice = computed(() => forceNotice.data)
  const show = computed(() => isSuper.value && notice.value?.force === true)
  const notes = computed(() =>
    (notice.value?.notes || [])
      .map((line) => line.replace(/^[-*]\s*/, '').trim())
      .filter((line, index) => line && !(index === 0 && /^auth-pro\s/i.test(line)))
      .slice(0, 4)
  )
  const subText = computed(() => {
    const data = notice.value
    if (!data) return ''
    if (data.autoFailed)
      return '自动更新这个版本时没有成功，网站已回到原版本，请到在线更新页查看原因后手动更新。'
    if (data.autoUpdate)
      return `已开启自动更新，将在服务器时间${data.autoWindow}自动安装，也可以现在更新。`
    return '更新时会先核对官方签名、备份数据库，新版本没起来会自动回退。'
  })

  const dialogVisible = ref(false)
  watch(show, (visible) => {
    if (!visible || route.path === '/online-update') return
    const key = `v${notice.value?.version}`
    if (sessionStorage.getItem(FORCE_NOTICE_DIALOG_KEY) === key) return
    sessionStorage.setItem(FORCE_NOTICE_DIALOG_KEY, key)
    dialogVisible.value = true
  })

  const goUpdate = () => {
    dialogVisible.value = false
    if (route.path !== '/online-update') void router.push('/online-update')
  }

  // 每 30 分钟再读一次；服务端本身每小时检查一次更新
  let timer = 0
  onMounted(() => {
    if (!isSuper.value) return
    void refreshForceNotice()
    timer = window.setInterval(() => void refreshForceNotice(), 30 * 60 * 1000)
  })
  onBeforeUnmount(() => window.clearInterval(timer))
</script>

<style scoped lang="scss">
  .force-update-banner {
    display: flex;
    gap: 10px;
    align-items: center;
    padding: 8px 14px;
    margin-bottom: 12px;
    color: var(--el-color-danger);
    background: var(--el-color-danger-light-9);
    border: 1px solid var(--el-color-danger-light-7);
    border-radius: 8px;
  }

  .banner-icon {
    flex-shrink: 0;
    font-size: 18px;
  }

  .banner-text {
    display: flex;
    flex: 1;
    flex-wrap: wrap;
    gap: 2px 10px;
    min-width: 0;
    font-size: 13px;
    line-height: 20px;
  }

  .banner-sub {
    color: var(--art-gray-700);
  }

  .banner-action {
    flex-shrink: 0;
  }

  .force-update-dialog {
    font-size: 14px;
    line-height: 22px;
    color: var(--art-gray-800);
  }

  .dialog-lead {
    margin: 0 0 10px;
  }

  .dialog-notes {
    padding-left: 18px;
    margin: 0 0 10px;
    color: var(--art-gray-700);
    list-style: disc;
  }

  .dialog-muted {
    margin: 0;
    font-size: 13px;
    color: var(--art-gray-600);
  }
</style>
