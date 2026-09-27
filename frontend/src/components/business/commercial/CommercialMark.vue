<template>
  <span class="commercial-mark" :class="`is-${tone}`">
    <ArtSvgIcon :icon="icon" class="commercial-mark__icon" />
    <span class="commercial-mark__text">{{ text }}</span>
  </span>
</template>

<script setup lang="ts">
  defineOptions({ name: 'CommercialMark' })

  withDefaults(
    defineProps<{
      text: string
      icon?: string
      tone?: 'primary' | 'warning' | 'ok'
    }>(),
    {
      icon: 'ri:rocket-2-line',
      tone: 'primary'
    }
  )
</script>

<style scoped>
  .commercial-mark {
    display: inline-flex;
    gap: 6px;
    align-items: flex-start;
    max-width: 100%;
    color: var(--el-color-primary);
    font-size: 13px;
    line-height: 1.5;
  }

  .commercial-mark.is-warning {
    color: var(--el-color-warning);
  }

  .commercial-mark.is-ok {
    color: var(--el-color-success);
  }

  .commercial-mark__icon {
    flex: none;
    margin-top: 2px;
    font-size: 16px;
  }

  .commercial-mark__text {
    min-width: 0;
  }
</style>
