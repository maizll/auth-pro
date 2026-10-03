<!-- 上传更新包：更新源连不上时的备用办法。只上传发布页里的整包原文件，版本和说明从包里认出，用户只点确认。 -->
<template>
  <ElButton
    v-if="!hideTrigger"
    v-roles="'R_SUPER'"
    size="small"
    :icon="Upload"
    :disabled="disabled"
    @click="open"
  >
    上传更新包
  </ElButton>
  <AppDialog
    v-model="visible"
    title="上传更新包"
    size="lg"
    flow="long"
    :show-close="!busy"
    :close-on-click-modal="!busy"
    @closed="reset"
  >
    <div class="upload-update">
      <ElAlert type="info" :closable="false" show-icon class="upload-update__tip">
        <template v-if="official">
          更新源连不上时用。请上传发布页里的 auth_pro-full-v版本号.tar.gz
          原文件，不要解压或改名后重新打包。
        </template>
        <template v-else>
          更新源连不上时用。请在官网「我的授权」→「版本下载」下载对应版本的原文件（auth_pro-full-v版本号.tar.gz），不要解压或重新打包。
        </template>
      </ElAlert>

      <ElUpload
        v-if="!info"
        ref="uploadRef"
        drag
        class="upload-update__drop"
        :auto-upload="false"
        :show-file-list="false"
        accept=".gz,.tgz,application/gzip"
        :disabled="busy"
        :on-change="selectFile"
      >
        <ArtSvgIcon icon="ri:upload-cloud-2-line" class="upload-update__icon" />
        <div class="upload-update__title">点击选择更新包，或拖到这里</div>
        <ElText v-if="file" type="primary" :title="file.name" truncated>
          {{ file.name }} · {{ formatSize(file.size) }}
        </ElText>
        <ElText v-else type="info" size="small">只认官方签名的整包，最大 512 MB</ElText>
      </ElUpload>
      <ElProgress v-if="uploading" :percentage="percent" :stroke-width="8" />
      <ElText v-if="uploading" type="info" size="small">
        {{ percent < 100 ? '正在上传…' : '已上传，正在核对官方签名…' }}
      </ElText>

      <ElDescriptions v-if="info" :column="1" border class="upload-update__info">
        <ElDescriptionsItem label="更新包版本">v{{ info.version }}</ElDescriptionsItem>
        <ElDescriptionsItem label="当前版本">v{{ info.currentVersion }}</ElDescriptionsItem>
        <ElDescriptionsItem label="适用于">{{ info.editionLabel }}</ElDescriptionsItem>
        <ElDescriptionsItem label="官方签名">
          <ElTag type="success" effect="plain">已核对</ElTag>
        </ElDescriptionsItem>
        <ElDescriptionsItem label="文件大小">{{ formatSize(info.size) }}</ElDescriptionsItem>
        <ElDescriptionsItem label="更新说明">
          <div v-if="info.notes?.length" class="upload-update__notes">
            <div v-for="note in info.notes" :key="note">{{ note }}</div>
          </div>
          <span v-else>这个版本没有写更新说明</span>
        </ElDescriptionsItem>
      </ElDescriptions>
      <ElText v-if="info" type="info" size="small">
        确认后和在线更新一样：先备份数据库，再替换程序并重启，新版本起不来会自动换回现在的版本。
      </ElText>

      <ElAlert v-if="error" :title="error" type="error" :closable="false" show-icon />
    </div>
    <template #footer>
      <ElButton :disabled="busy" @click="visible = false">取消</ElButton>
      <ElButton v-if="info" :disabled="busy" @click="chooseAgain">重新选择</ElButton>
      <ElButton v-if="!info" type="primary" :loading="uploading" :disabled="!file" @click="upload">
        上传并核对
      </ElButton>
      <ElButton v-else type="primary" :loading="applying" @click="confirmInstall">
        确认更新到 v{{ info.version }}
      </ElButton>
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
  import { computed, ref } from 'vue'
  import type { UploadFile, UploadInstance } from 'element-plus'
  import { Upload } from '@element-plus/icons-vue'
  import AppDialog from '@/components/core/dialog/AppDialog.vue'
  import {
    applyUploadedOnlineUpdate,
    uploadOnlineUpdatePackage,
    type OnlineUpdateJob,
    type OnlineUpdateUpload
  } from '@/api/update'

  defineOptions({ name: 'UploadUpdatePackage' })

  // hideTrigger：窄屏时按钮收进页面的「更多」，由页面调用 open()
  defineProps<{ official: boolean; disabled?: boolean; hideTrigger?: boolean }>()
  const emit = defineEmits<{ started: [job: OnlineUpdateJob] }>()

  const maxSize = 512 * 1024 * 1024
  const visible = ref(false)
  const uploading = ref(false)
  const applying = ref(false)
  const percent = ref(0)
  const file = ref<File | null>(null)
  const info = ref<OnlineUpdateUpload | null>(null)
  const error = ref('')
  const uploadRef = ref<UploadInstance>()
  const busy = computed(() => uploading.value || applying.value)

  const open = () => {
    visible.value = true
  }
  defineExpose({ open })

  const reset = () => {
    file.value = null
    info.value = null
    error.value = ''
    percent.value = 0
  }

  const chooseAgain = () => {
    reset()
  }

  const selectFile = (selected: UploadFile) => {
    uploadRef.value?.clearFiles()
    error.value = ''
    const raw = selected.raw || null
    file.value = null
    if (!raw) return
    if (!/\.(tar\.gz|tgz)$/i.test(raw.name)) {
      error.value = '这不是更新包文件，请选 auth_pro-full 开头、.tar.gz 结尾的原文件'
      return
    }
    if (!raw.size) {
      error.value = '这是一个空文件，请重新选择'
      return
    }
    if (raw.size > maxSize) {
      error.value = '文件太大了，更新包不会超过 512 MB，请确认选对了文件'
      return
    }
    file.value = raw
  }

  const upload = async () => {
    if (!file.value) return
    uploading.value = true
    percent.value = 0
    error.value = ''
    try {
      info.value = await uploadOnlineUpdatePackage(file.value, (value) => {
        percent.value = value
      })
    } catch (cause: unknown) {
      error.value = cause instanceof Error && cause.message ? cause.message : '上传失败，请重试'
    } finally {
      uploading.value = false
    }
  }

  const confirmInstall = async () => {
    if (!info.value) return
    applying.value = true
    error.value = ''
    try {
      const job = await applyUploadedOnlineUpdate(info.value.uploadId)
      visible.value = false
      emit('started', job)
    } catch (cause: unknown) {
      error.value = cause instanceof Error && cause.message ? cause.message : '更新没有启动，请重试'
    } finally {
      applying.value = false
    }
  }

  const formatSize = (size: number) => {
    if (size >= 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`
    return `${Math.max(1, Math.round(size / 1024))} KB`
  }
</script>

<style scoped>
  .upload-update {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .upload-update__drop {
    width: 100%;
  }

  .upload-update__icon {
    font-size: 40px;
    color: var(--el-color-primary);
  }

  .upload-update__title {
    margin: 6px 0;
    color: var(--el-text-color-primary);
  }

  .upload-update__notes {
    display: flex;
    flex-direction: column;
    gap: 4px;
    line-height: 1.6;
    word-break: break-word;
    white-space: pre-wrap;
  }

  .upload-update__info :deep(.el-descriptions__label) {
    width: 96px;
    white-space: nowrap;
  }
</style>
