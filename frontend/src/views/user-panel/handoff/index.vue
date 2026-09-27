<!-- 源站用一次性票据换成登录状态，然后只进入「我的授权」。票据放在网址片段里，读完就抹掉。 -->
<template>
  <div class="handoff">正在进入我的授权…</div>
</template>

<script setup lang="ts">
  import { onMounted } from 'vue'
  import { useRouter } from 'vue-router'
  import axios from 'axios'
  import { ElMessage } from 'element-plus'

  const router = useRouter()
  const licensesPaths = ['/user/licenses', '/agent-panel/licenses']

  onMounted(async () => {
    const ticket = window.location.hash.replace(/^#/, '').trim()
    window.history.replaceState(null, '', window.location.pathname)
    if (!/^[0-9a-f]{64}$/.test(ticket)) {
      ElMessage.error('链接无效或已过期')
      return
    }
    try {
      const { data } = await axios.post('/api/v1/store/auth/handoff/consume', { ticket })
      const path = data?.data?.path
      const accessToken = data?.data?.accessToken
      if (data?.code !== 200 || !accessToken || !licensesPaths.includes(path)) {
        ElMessage.error(data?.msg || '链接无效或已过期')
        return
      }
      if (path === '/agent-panel/licenses') {
        localStorage.setItem('agent_panel_token', accessToken)
        localStorage.setItem(
          'agent_panel_info',
          JSON.stringify({
            email: data.data.email || '',
            name: data.data.nickname || ''
          })
        )
      } else {
        localStorage.setItem('user_panel_token', accessToken)
        localStorage.setItem(
          'user_panel_info',
          JSON.stringify({
            email: data.data.email || '',
            nickname: data.data.nickname || ''
          })
        )
      }
      await router.replace(path)
    } catch {
      ElMessage.error('网络不通，请稍后再试')
    }
  })
</script>

<style scoped>
  .handoff {
    padding: 48px 16px;
    color: var(--el-text-color-regular);
    text-align: center;
  }
</style>
