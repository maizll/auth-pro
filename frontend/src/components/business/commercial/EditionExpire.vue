<!-- 到期时间后面的版本胶囊。商业版用主题蓝，免费版用灰色，和顶栏开通态同一套蓝色。 -->
<template>
  <span class="edition-expire">
    <span class="edition-expire__time"><slot /></span>
    <span
      class="edition-expire__capsule"
      :class="commercial ? 'edition-expire__capsule--commercial' : 'edition-expire__capsule--free'"
    >
      {{ commercial ? '商业版' : '免费版' }}
    </span>
  </span>
</template>

<script setup lang="ts">
  defineOptions({ name: 'EditionExpire' })

  defineProps<{ commercial?: boolean }>()
</script>

<style scoped>
  .edition-expire {
    display: inline-flex;
    flex-wrap: nowrap;
    gap: 6px;
    align-items: center;
    max-width: 100%;
    white-space: nowrap;
  }

  .edition-expire__time {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .edition-expire__capsule {
    display: inline-flex;
    flex: none;
    align-items: center;
    height: 20px;
    padding: 0 8px;
    font-size: 12px;
    font-weight: 600;
    line-height: 1;
    white-space: nowrap;
    border-radius: 999px;
  }

  .edition-expire__capsule--free {
    color: var(--el-text-color-secondary);
    background: var(--el-fill-color);
    border: 1px solid var(--el-border-color);
  }

  .edition-expire__capsule--commercial {
    color: #fff;
    background: var(--el-color-primary);
    border: 1px solid var(--el-color-primary);
  }
</style>
