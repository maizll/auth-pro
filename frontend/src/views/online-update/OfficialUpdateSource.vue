<!-- 官网在线更新页里的「更新来源」：平时只一行，写明用的是哪个存储令牌。令牌统一在存储管理里维护。客户站不显示。 -->
<template>
  <div class="update-section source-section">
    <div class="source-line">
      <strong>更新来源</strong>
      <span class="source-text" :title="text">{{ text }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
  import { computed } from 'vue'
  import type { OfficialUpdateSource } from '@/api/update'

  defineOptions({ name: 'OfficialUpdateSource' })

  const props = defineProps<{ source: OfficialUpdateSource }>()

  // 读不到时也用中性颜色：多半是还没给存储令牌加上发布仓库，不是故障。
  const text = computed(() => {
    const check = props.source.lastCheck
    if (!check) return '本次启动后还没读过，点「检查更新」会读一次。'
    if (check.ok) return `更新来源正常（用的是${check.source || '匿名读取'}）`
    return `暂时读不到：${check.reason || '请稍后再试'}。令牌在「存储管理」里维护。`
  })
</script>

<style scoped lang="scss">
  /* 根节点的分隔线和间距沿用父页面的 .update-section。 */
  .source-line {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 12px;
    align-items: baseline;
    min-width: 0;

    strong {
      font-size: 15px;
      color: var(--art-gray-900);
    }
  }

  .source-text {
    min-width: 0;
    overflow: hidden;
    font-size: 13px;
    color: var(--el-text-color-secondary);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  @media (width <= 640px) {
    .source-text {
      white-space: normal;
    }
  }
</style>
