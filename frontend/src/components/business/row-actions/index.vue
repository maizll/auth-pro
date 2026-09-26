<!-- 列表操作：常用按钮留在外面，其余收进「更多」。 -->
<template>
  <div class="row-actions">
    <ElButton
      v-for="item in visiblePrimary"
      :key="item.key"
      link
      :type="item.danger ? 'danger' : 'primary'"
      :disabled="item.disabled"
      :title="item.hint"
      @click="emit('click', item)"
    >
      {{ item.label }}
    </ElButton>
    <ElDropdown
      v-if="visibleMore.length"
      trigger="click"
      popper-class="row-actions-menu"
      @command="onCommand"
    >
      <span class="row-actions__more">
        <ElButton link type="primary">更多</ElButton>
      </span>
      <template #dropdown>
        <ElDropdownMenu>
          <ElDropdownItem
            v-for="item in visibleMore"
            :key="item.key"
            :command="item.key"
            :disabled="item.disabled"
            :class="{ 'is-danger': item.danger }"
            :title="item.hint"
          >
            {{ item.label }}
          </ElDropdownItem>
        </ElDropdownMenu>
      </template>
    </ElDropdown>
  </div>
</template>

<script setup lang="ts">
  export interface RowActionItem {
    key: string
    label: string
    danger?: boolean
    disabled?: boolean
    hidden?: boolean
    hint?: string
  }

  const props = defineProps<{
    primary?: RowActionItem[]
    more?: RowActionItem[]
  }>()

  const emit = defineEmits<{
    click: [item: RowActionItem]
  }>()

  const visiblePrimary = computed(() => (props.primary || []).filter((item) => !item.hidden))
  const visibleMore = computed(() => (props.more || []).filter((item) => !item.hidden))

  function onCommand(key: string | number) {
    const item = visibleMore.value.find((entry) => entry.key === String(key))
    if (!item || item.disabled) return
    emit('click', item)
  }
</script>

<style scoped>
  .row-actions {
    display: inline-flex;
    align-items: center;
    flex-wrap: nowrap;
    gap: 2px;
    white-space: nowrap;
  }

  .row-actions :deep(.el-button.is-link) {
    padding: 0 4px;
    margin: 0;
  }
</style>

<style>
  .row-actions-menu .el-dropdown-menu__item.is-danger {
    color: var(--el-color-danger);
  }

  .row-actions-menu .el-dropdown-menu__item.is-danger:not(.is-disabled):focus,
  .row-actions-menu .el-dropdown-menu__item.is-danger:not(.is-disabled):hover {
    color: var(--el-color-danger);
    background-color: var(--el-color-danger-light-9);
  }
</style>
