<template>
  <ElButton :disabled="uploading" @click="visible = true">上传首页模板 ZIP</ElButton>
  <ElDialog
    v-model="visible"
    title="上传首页模板 ZIP"
    width="min(560px, calc(100vw - 32px))"
    :show-close="!uploading"
    :close-on-click-modal="!uploading"
    :close-on-press-escape="!uploading"
    @closed="resetForm"
  >
    <ElAlert type="info" :closable="false" show-icon>
      ZIP 不超过 20 MiB，解压后不超过 100 MiB。支持 template.json 和资源文件，或已构建的 index.html
      静态首页（包内 /assets/ 入口引用会自动适配）。上传安装后需手动启用。
    </ElAlert>
    <ElForm
      label-width="88px"
      :disabled="uploading"
      class="upload-form"
      @submit.prevent="handleUpload"
    >
      <ElFormItem label="模板名称">
        <ElInput v-model="name" maxlength="100" placeholder="默认使用 ZIP 文件名" />
      </ElFormItem>
      <ElFormItem label="版本">
        <ElInput v-model="version" maxlength="40" placeholder="1.0.0" />
      </ElFormItem>
      <ElFormItem label="ZIP 文件" required>
        <ElUpload
          ref="fileUpload"
          drag
          class="template-zip-upload"
          :auto-upload="false"
          :show-file-list="false"
          accept=".zip,application/zip"
          :disabled="uploading"
          :on-change="selectFile"
        >
          <ArtSvgIcon icon="ri:upload-cloud-2-line" class="upload-icon" />
          <div class="upload-title">点击选择 ZIP 文件，或拖拽到此处</div>
          <ElText v-if="file" class="upload-file-name" type="primary" :title="file.name" truncated>
            {{ file.name }} · {{ (file.size / 1024).toFixed(1) }} KiB
          </ElText>
          <ElText v-else class="upload-tip" type="info" size="small"
            >仅支持 ZIP，最大 20 MiB</ElText
          >
        </ElUpload>
      </ElFormItem>
      <ElText type="info" size="small"
        >静态首页在隔离环境中运行，通过系统登录入口登录，不执行包内安装脚本。</ElText
      >
    </ElForm>
    <ElAlert v-if="error" :title="error" type="error" :closable="false" show-icon />
    <template #footer>
      <ElButton :disabled="uploading" @click="visible = false">取消</ElButton>
      <ElButton type="primary" :loading="uploading" :disabled="!file" @click="handleUpload"
        >上传并安装</ElButton
      >
    </template>
  </ElDialog>
</template>

<script setup lang="ts">
  import { ref } from 'vue'
  import { ElMessage, ElUpload, type UploadFile, type UploadInstance } from 'element-plus'
  import { fetchUploadHomeTemplate } from '@/api/system-manage'

  const emit = defineEmits<{ uploaded: [id: number] }>()
  const visible = ref(false)
  const uploading = ref(false)
  const name = ref('')
  const version = ref('1.0.0')
  const file = ref<File | null>(null)
  const fileUpload = ref<UploadInstance>()
  const error = ref('')

  function selectFile(upload: UploadFile) {
    const selected = upload.raw
    // The FormData request owns the selected file; do not retain old files in ElUpload.
    fileUpload.value?.clearFiles()
    error.value = ''
    file.value = null
    if (!selected) return
    if (!/\.zip$/i.test(selected.name) || selected.size > 20 * 1024 * 1024 || !selected.size) {
      error.value = '请选择不超过 20 MiB 的非空 ZIP 文件'
      return
    }
    file.value = selected
    if (!name.value) name.value = selected.name.replace(/\.zip$/i, '')
  }

  function resetForm() {
    name.value = ''
    version.value = '1.0.0'
    file.value = null
    error.value = ''
    fileUpload.value?.clearFiles()
  }

  async function handleUpload() {
    if (!file.value || uploading.value) return
    uploading.value = true
    error.value = ''
    try {
      const data = new FormData()
      data.append('file', file.value)
      data.append('name', name.value.trim())
      data.append('version', version.value.trim())
      const result = await fetchUploadHomeTemplate(data)
      ElMessage.success('首页模板已上传并安装，请在列表中启用')
      visible.value = false
      emit('uploaded', result.id)
    } catch (cause: unknown) {
      error.value = cause instanceof Error ? cause.message : '模板上传失败，请重试'
    } finally {
      uploading.value = false
    }
  }
</script>

<style scoped>
  .upload-form {
    margin: 20px 0;
  }

  .template-zip-upload,
  .template-zip-upload :deep(.el-upload) {
    width: 100%;
  }

  .template-zip-upload :deep(.el-upload-dragger) {
    width: 100%;
    padding: 20px 16px;
  }

  .upload-icon {
    margin-bottom: 8px;
    font-size: 32px;
    color: var(--el-color-primary);
  }

  .upload-title {
    color: var(--el-text-color-primary);
  }

  .upload-tip,
  .upload-file-name {
    display: block;
    max-width: 100%;
    margin-top: 8px;
  }
</style>
