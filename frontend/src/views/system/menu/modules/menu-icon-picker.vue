<!-- 菜单图标：可手填，也可以从完整图标集里选。完整集只在打开选择器时加载。 -->
<template>
  <div class="menu-icon-picker">
    <ElInput
      :model-value="modelValue"
      placeholder="如：ri:user-line"
      @update:model-value="emit('update:modelValue', String($event ?? ''))"
    >
      <template #prefix>
        <ArtSvgIcon v-if="modelValue" :icon="modelValue" />
      </template>
      <template #append>
        <ElButton @click="open = true">选择</ElButton>
      </template>
    </ElInput>

    <ElDialog v-model="open" title="选择图标" width="640px" append-to-body @open="loadIcons">
      <ElInput v-model="keyword" clearable placeholder="搜索图标名称" />
      <p v-if="loading" class="menu-icon-picker__hint">正在加载图标…</p>
      <p v-else-if="!visible.length" class="menu-icon-picker__hint">没有匹配的图标</p>
      <div v-else class="menu-icon-picker__grid">
        <button
          v-for="name in visible"
          :key="name"
          type="button"
          class="menu-icon-picker__item"
          :class="{ 'is-current': modelValue === `ri:${name}` }"
          @click="pick(name)"
        >
          <ArtSvgIcon :icon="`ri:${name}`" />
          <span>{{ name }}</span>
        </button>
      </div>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { computed, ref } from 'vue'
  import ArtSvgIcon from '@/components/core/base/art-svg-icon/index.vue'
  import { listRemixIconNames } from '@/utils/ui/iconify-loader'

  defineProps<{ modelValue?: string }>()
  const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

  const open = ref(false)
  const loading = ref(false)
  const keyword = ref('')
  const names = ref<string[]>([])

  const visible = computed(() => {
    const query = keyword.value.trim().toLowerCase()
    const list = query ? names.value.filter((name) => name.includes(query)) : names.value
    return list.slice(0, 120)
  })

  async function loadIcons() {
    if (names.value.length) return
    loading.value = true
    try {
      names.value = await listRemixIconNames()
    } finally {
      loading.value = false
    }
  }

  function pick(name: string) {
    emit('update:modelValue', `ri:${name}`)
    open.value = false
  }
</script>

<style scoped lang="scss">
  .menu-icon-picker__hint {
    margin: 16px 0 0;
    color: var(--el-text-color-secondary);
  }

  .menu-icon-picker__grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(148px, 1fr));
    gap: 8px;
    max-height: 360px;
    margin-top: 12px;
    overflow: auto;
  }

  .menu-icon-picker__item {
    display: flex;
    gap: 8px;
    align-items: center;
    min-width: 0;
    padding: 8px;
    color: var(--el-text-color-primary);
    cursor: pointer;
    background: var(--el-fill-color-blank);
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 8px;

    span {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    &.is-current {
      border-color: var(--el-color-primary);
    }
  }
</style>
