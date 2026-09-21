<template>
  <ElTag :type="view.type" :effect="view.effect" v-bind="sizeBinding">{{ view.label }}</ElTag>
</template>

<script setup lang="ts">
  import { computed } from 'vue'
  import { resolveBizStatus, type BizStatusDomain } from './status'

  defineOptions({ name: 'BizStatusTag' })

  const props = withDefaults(
    defineProps<{
      status?: string | null
      domain: BizStatusDomain
      /** 覆盖字典文案，颜色仍按 domain 字典 */
      label?: string
      emptyText?: string
      size?: 'large' | 'default' | 'small'
    }>(),
    {
      status: '',
      label: '',
      emptyText: '-'
    }
  )

  const view = computed(() =>
    resolveBizStatus({
      domain: props.domain,
      status: props.status,
      label: props.label,
      emptyText: props.emptyText
    })
  )

  const sizeBinding = computed(() => (props.size ? { size: props.size } : {}))
</script>
