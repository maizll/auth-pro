<!-- 全站表单和确认共用这一层。宽度分四档，长表单在手机上用抽屉。 -->
<template>
  <ElDrawer
    v-if="useDrawer"
    :model-value="modelValue"
    :title="title"
    direction="rtl"
    size="100%"
    append-to-body
    :destroy-on-close="destroyOnClose"
    :close-on-click-modal="closeOnClickModal"
    :before-close="beforeClose"
    @update:model-value="emit('update:modelValue', $event)"
    @open="emit('open')"
    @closed="emit('closed')"
    @close="emit('close')"
  >
    <template v-if="$slots.header" #header>
      <slot name="header" />
    </template>
    <slot />
    <template v-if="$slots.footer" #footer>
      <div class="app-dialog__foot">
        <slot name="footer" />
      </div>
    </template>
  </ElDrawer>
  <ElDialog
    v-else
    :model-value="modelValue"
    :title="title"
    :width="width"
    :class="dialogClass"
    :show-close="showClose"
    :close-on-click-modal="closeOnClickModal"
    :before-close="beforeClose"
    align-center
    append-to-body
    :destroy-on-close="destroyOnClose"
    @update:model-value="emit('update:modelValue', $event)"
    @open="emit('open')"
    @closed="emit('closed')"
    @close="emit('close')"
  >
    <template v-if="$slots.header" #header>
      <slot name="header" />
    </template>
    <slot />
    <template v-if="$slots.footer" #footer>
      <div class="app-dialog__foot">
        <slot name="footer" />
      </div>
    </template>
  </ElDialog>
</template>

<script setup lang="ts">
  import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

  defineOptions({ name: 'AppDialog' })

  const props = withDefaults(
    defineProps<{
      modelValue: boolean
      title?: string
      size?: 'sm' | 'md' | 'lg' | 'xl'
      /** long：手机用全屏抽屉。short：手机仍用对话框。 */
      flow?: 'long' | 'short'
      destroyOnClose?: boolean
      showClose?: boolean
      closeOnClickModal?: boolean
      dialogClass?: string
      beforeClose?: (done: () => void) => void
    }>(),
    {
      size: 'md',
      flow: 'short',
      destroyOnClose: false,
      title: '',
      showClose: true,
      closeOnClickModal: true
    }
  )

  const emit = defineEmits<{
    'update:modelValue': [boolean]
    open: []
    closed: []
    close: []
  }>()

  const narrow = ref(false)
  const widths = { sm: 420, md: 520, lg: 640, xl: 760 }

  function sync() {
    narrow.value = window.innerWidth < 768
  }

  onMounted(() => {
    sync()
    window.addEventListener('resize', sync)
  })
  onBeforeUnmount(() => window.removeEventListener('resize', sync))

  const useDrawer = computed(() => props.flow === 'long' && narrow.value)
  const width = computed(() => `min(${widths[props.size]}px, calc(100vw - 32px))`)
</script>

<style scoped>
  .app-dialog__foot {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }
</style>
