<template>
  <div class="template-actions">
    <ElButton
      v-if="!builtin && (template.available || template.installed)"
      size="small"
      :disabled="!!busy"
      :loading="busy === 'download'"
      @click="download"
      >下载</ElButton
    >
    <ElButton
      v-if="
        !builtin &&
        !template.enabled &&
        template.available &&
        (!template.installed || template.updateAvailable)
      "
      size="small"
      :disabled="!!busy"
      :loading="busy === 'install'"
      @click="mutate('install')"
      >{{ template.updateAvailable ? '更新安装' : '安装' }}</ElButton
    >
    <ElButton
      v-if="!template.enabled || template.updateAvailable || builtin"
      type="primary"
      size="small"
      :disabled="
        !!busy ||
        (template.enabled && !template.updateAvailable) ||
        (!template.available && !template.installed)
      "
      :loading="busy === 'enable'"
      @click="mutate('enable')"
      >{{
        template.updateAvailable ? '更新并启用' : template.enabled ? '当前模板' : '启用'
      }}</ElButton
    >
    <ElButton
      v-if="!builtin && template.enabled"
      size="small"
      :disabled="!!busy"
      :loading="busy === 'disable'"
      @click="mutate('disable')"
      >停用</ElButton
    >
    <ElButton
      v-if="!builtin && template.installed"
      type="danger"
      plain
      size="small"
      :disabled="!!busy"
      :loading="busy === 'uninstall'"
      @click="mutate('uninstall')"
      >卸载</ElButton
    >
  </div>
</template>

<script setup lang="ts">
  import { computed, ref } from 'vue'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import {
    fetchDisableHomeTemplate,
    fetchDownloadHomeTemplate,
    fetchEnableHomeTemplate,
    fetchInstallHomeTemplate,
    fetchUninstallHomeTemplate,
    type HomeTemplateInfo
  } from '@/api/system-manage'

  const props = defineProps<{ template: HomeTemplateInfo }>()
  const emit = defineEmits<{ changed: [] }>()
  const busy = ref('')
  const builtin = computed(
    () => props.template.id === 'default' || props.template.sourceType === 'builtin'
  )

  const download = async () => {
    if (busy.value) return
    busy.value = 'download'
    let url = ''
    try {
      const file = await fetchDownloadHomeTemplate(props.template.id)
      url = URL.createObjectURL(file)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `${props.template.templateId}.${file.type.includes('zip') ? 'zip' : 'json'}`
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
    } catch {
      // The shared request layer displays failures; never save an error envelope as a ZIP.
    } finally {
      if (url) setTimeout(() => URL.revokeObjectURL(url), 1000)
      busy.value = ''
    }
  }

  const mutate = async (action: 'install' | 'enable' | 'disable' | 'uninstall') => {
    if (busy.value) return
    const label = { install: '安装', enable: '启用', disable: '停用', uninstall: '卸载' }[action]
    const detail = {
      install: '仅下载并安装，不切换当前首页。',
      enable: '将安装所需文件并切换登录页的首页模板。',
      disable: '恢复默认首页，保留模板文件，可再次启用。',
      uninstall:
        '删除本地安装文件；若正在使用，则先恢复默认首页。软件源中的模板不会被删除，可重新安装。'
    }[action]
    busy.value = action
    try {
      await ElMessageBox.confirm(
        `确认${label}「${props.template.name}」？${detail}`,
        `${label}首页模板`,
        {
          confirmButtonText: label,
          cancelButtonText: '取消',
          type: 'warning'
        }
      )
      const request = {
        install: fetchInstallHomeTemplate,
        enable: fetchEnableHomeTemplate,
        disable: fetchDisableHomeTemplate,
        uninstall: fetchUninstallHomeTemplate
      }[action]
      await request(props.template.id)
      ElMessage.success(`${label}成功`)
      emit('changed')
    } catch {
      // Cancellation is silent; API failures are reported by the shared request layer.
    } finally {
      busy.value = ''
    }
  }
</script>

<style scoped>
  .template-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    justify-content: flex-end;
  }
  .template-actions :deep(.el-button + .el-button) {
    margin-left: 0;
  }
</style>
