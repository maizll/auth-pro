<template>
  <span class="biz-copy-secret">
    <span v-if="label" class="biz-copy-secret__label">{{ label }}</span>
    <span class="biz-copy-secret__value">{{ display }}</span>
    <template v-if="hasValue">
      <ElButton link type="primary" size="small" @click="visible = !visible">
        {{ visible ? '隐藏' : '查看' }}
      </ElButton>
      <ElButton link type="primary" size="small" @click="copy">{{ copyLabel }}</ElButton>
    </template>
  </span>
</template>

<script setup lang="ts">
  import { computed, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import { copySecretText, secretDisplay } from './secret'

  defineOptions({ name: 'BizCopySecret' })

  const props = withDefaults(
    defineProps<{
      value?: string | null
      emptyText?: string
      label?: string
      defaultVisible?: boolean
      copyLabel?: string
      successText?: string
      failText?: string
    }>(),
    {
      value: '',
      emptyText: '—',
      label: '',
      defaultVisible: false,
      copyLabel: '复制',
      successText: '已复制',
      failText: '复制失败，请手动复制'
    }
  )

  const visible = ref(props.defaultVisible)
  const hasValue = computed(() => Boolean(String(props.value ?? '').trim()))
  const display = computed(() => secretDisplay(props.value, visible.value, props.emptyText))

  async function copy() {
    const ok = await copySecretText(props.value)
    if (ok) ElMessage.success(props.successText)
    else ElMessage.error(props.failText)
  }
</script>

<style scoped lang="scss">
  .biz-copy-secret {
    display: inline-flex;
    flex-wrap: wrap;
    gap: 4px;
    align-items: center;
    max-width: 100%;
  }

  .biz-copy-secret__label {
    color: var(--el-text-color-secondary);
  }

  .biz-copy-secret__value {
    word-break: break-all;
  }
</style>
