<!--
  「正在更新」卡片：在线更新开始到新版本启动完成之间盖在所有页面上，完成后显示「更新成功」并自动刷新。
  样式和文案与 public/backend-unavailable.html 的更新状态一致（那一页给重启中整页刷新时用）。
-->
<template>
  <Transition name="update-busy">
    <div v-if="updateSession.watching" class="online-update-busy" role="status" aria-live="polite">
      <div class="busy-card">
        <div v-if="!updateSession.finished" class="busy-spinner" aria-hidden="true" />
        <ArtSvgIcon v-else icon="ri:checkbox-circle-fill" class="busy-done" />
        <h2>{{ title }}</h2>
        <p class="busy-lead">{{ description }}</p>
        <ElProgress
          class="busy-progress"
          :percentage="progress"
          :status="updateSession.finished ? 'success' : undefined"
          :stroke-width="6"
          :show-text="false"
        />
        <span v-if="!updateSession.finished" class="busy-waited">
          已等待 <b>{{ waitedSeconds }}</b> 秒
        </span>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
  import { computed, onBeforeUnmount, ref } from 'vue'
  import { updateSession } from './update-session'

  defineOptions({ name: 'OnlineUpdateBusyCard' })

  const now = ref(Date.now())
  const clock = window.setInterval(() => (now.value = Date.now()), 1000)
  onBeforeUnmount(() => window.clearInterval(clock))

  const version = computed(() =>
    updateSession.job?.version ? ` v${updateSession.job.version}` : ''
  )
  const title = computed(() => (updateSession.finished ? '更新成功' : '正在更新'))
  const description = computed(() => {
    if (updateSession.finished) return `新版本${version.value} 已经启动，正在刷新页面。`
    if (updateSession.awaitingRestart || updateSession.job?.status === 'restarting') {
      return `站点正在安装新版本${version.value}，通常一分钟内完成，好了会自动刷新。不用关闭页面，也不用手动操作。`
    }
    const step = (updateSession.job?.message || '正在下载并校验更新包').replace(/[。.，,]+$/, '')
    return `${step}，完成后会自动重启并刷新。`
  })
  const progress = computed(() => {
    if (updateSession.finished) return 100
    const value = Number(updateSession.job?.progress)
    const normalized = Number.isFinite(value) ? Math.min(99, Math.max(0, value)) : 0
    return updateSession.awaitingRestart ? Math.max(95, normalized) : normalized
  })
  const waitedSeconds = computed(() =>
    updateSession.since > 0 ? Math.max(0, Math.floor((now.value - updateSession.since) / 1000)) : 0
  )
</script>

<style lang="scss" scoped>
  .online-update-busy {
    position: fixed;
    inset: 0;
    z-index: 3100;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 16px;
    background: rgb(15 23 42 / 35%);
    backdrop-filter: blur(2px);
  }

  .busy-card {
    width: min(440px, 100%);
    padding: 36px 32px 28px;
    text-align: center;
    background: var(--default-box-color, #fff);
    border: 1px solid var(--art-border-color);
    border-radius: 16px;
    box-shadow: 0 18px 50px rgb(15 23 42 / 18%);

    h2 {
      margin: 0 0 10px;
      font-size: 22px;
      color: var(--art-gray-900);
    }
  }

  .busy-spinner {
    width: 52px;
    height: 52px;
    margin: 0 auto 20px;
    border: 4px solid var(--el-color-primary-light-8);
    border-top-color: var(--el-color-primary);
    border-radius: 50%;
    animation: update-busy-spin 0.9s linear infinite;
  }

  .busy-done {
    display: block;
    margin: 0 auto 16px;
    font-size: 56px;
    color: var(--el-color-success);
  }

  .busy-lead {
    max-width: 360px;
    margin: 0 auto;
    font-size: 15px;
    line-height: 1.75;
    color: var(--art-gray-600);
  }

  .busy-progress {
    margin: 20px 0 14px;
  }

  .busy-waited {
    display: inline-block;
    padding: 4px 12px;
    font-size: 13px;
    color: var(--el-color-primary);
    background: var(--el-color-primary-light-9);
    border-radius: 999px;
  }

  .update-busy-enter-active,
  .update-busy-leave-active {
    transition: opacity 0.2s ease;
  }

  .update-busy-enter-from,
  .update-busy-leave-to {
    opacity: 0;
  }

  @keyframes update-busy-spin {
    to {
      transform: rotate(360deg);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .busy-spinner {
      animation-duration: 2.4s;
    }
  }
</style>
