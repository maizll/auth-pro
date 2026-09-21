<template>
  <ElSelect
    :model-value="modelValue"
    filterable
    :multiple="multiple"
    :clearable="clearable"
    :disabled="disabled"
    :placeholder="placeholder"
    :loading="loading"
    no-data-text="暂无应用"
    @update:model-value="onUpdate"
  >
    <ElOption
      v-for="app in selectable"
      :key="`${app.id}-${app.appKey || ''}`"
      :label="app.name"
      :value="app.value"
    />
  </ElSelect>
</template>

<script
  setup
  lang="ts"
  generic="T extends string | number | Array<string | number> | null | undefined"
>
  import { computed, onMounted, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import { loadBizApps } from './load-apps'
  import {
    appLoadErrorText,
    appOptionValue,
    normalizeAppOptions,
    type BizAppApi,
    type BizAppLoader,
    type BizAppOption,
    type BizAppValueKey
  } from './apps'

  defineOptions({ name: 'BizAppSelect' })

  const props = withDefaults(
    defineProps<{
      modelValue?: T
      multiple?: boolean
      clearable?: boolean
      disabled?: boolean
      placeholder?: string
      valueKey?: BizAppValueKey
      api?: BizAppApi
      loader?: BizAppLoader
    }>(),
    {
      multiple: false,
      clearable: false,
      disabled: false,
      placeholder: '请选择应用',
      valueKey: 'id',
      api: 'license-options'
    }
  )

  const emit = defineEmits<{
    (e: 'update:modelValue', value: T): void
    (e: 'change', value: T): void
  }>()

  const loading = ref(false)
  const options = ref<BizAppOption[]>([])

  const selectable = computed(() =>
    options.value.flatMap((app) => {
      const value = appOptionValue(app, props.valueKey)
      if (value === undefined) return []
      return [{ ...app, value }]
    })
  )

  function onUpdate(value: string | number | Array<string | number> | null | undefined) {
    const next = value as T
    emit('update:modelValue', next)
    emit('change', next)
  }

  async function loadOptions() {
    loading.value = true
    try {
      const raw = props.loader ? await props.loader() : await loadBizApps(props.api)
      options.value = props.loader ? normalizeAppOptions(raw) : (raw as BizAppOption[])
    } catch {
      options.value = []
      const message = props.loader ? undefined : appLoadErrorText(props.api)
      if (message) ElMessage.error(message)
    } finally {
      loading.value = false
    }
  }

  onMounted(loadOptions)
</script>
