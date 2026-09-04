<!-- 代理商等级搜索栏 -->
<template>
  <ArtSearchBar
    ref="searchBarRef"
    v-model="formData"
    :items="formItems"
    :showExpand="false"
    @reset="handleReset"
    @search="handleSearch"
  >
  </ArtSearchBar>
</template>

<script setup lang="ts">
  interface LevelSearchForm {
    keyword?: string
    status?: string
  }

  interface Props {
    modelValue: LevelSearchForm
  }

  interface Emits {
    (e: 'update:modelValue', value: LevelSearchForm): void
    (e: 'search', params: LevelSearchForm): void
    (e: 'reset'): void
  }

  const props = defineProps<Props>()
  const emit = defineEmits<Emits>()

  const searchBarRef = ref()

  /**
   * 表单数据双向绑定
   */
  const formData = computed({
    get: () => props.modelValue,
    set: (val) => emit('update:modelValue', val)
  })

  /**
   * 状态选项
   */
  const statusOptions = [
    { label: '启用', value: 'enabled' },
    { label: '禁用', value: 'disabled' }
  ]

  /**
   * 搜索表单配置项
   */
  const formItems = computed(() => [
    {
      label: '等级关键词',
      key: 'keyword',
      type: 'input',
      placeholder: '等级名称',
      clearable: true
    },
    {
      label: '状态',
      key: 'status',
      type: 'select',
      props: {
        placeholder: '全部状态',
        options: statusOptions,
        clearable: true
      }
    }
  ])

  /**
   * 处理重置事件
   */
  const handleReset = () => {
    emit('reset')
  }

  /**
   * 处理搜索事件
   */
  const handleSearch = (params: LevelSearchForm) => {
    emit('search', params)
  }
</script>
