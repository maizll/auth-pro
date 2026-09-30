<!-- 存储管理：维护主存储、备用和停用的位置。密钥只在这里提交，列表不回显。 -->
<template>
  <div class="storage-page art-full-height">
    <ElCard class="art-table-card" shadow="never">
      <ArtTableHeader :loading="loading" @refresh="loadList">
        <template #left>
          <ElSpace wrap>
            <ElButton type="primary" @click="openCreate">新增存储</ElButton>
            <ElButton @click="goFiles">浏览文件</ElButton>
            <span class="dual-write">
              <span>同时写入备用</span>
              <ElSwitch :model-value="dualWrite" :loading="savingOption" @change="onDualWrite" />
            </span>
          </ElSpace>
        </template>
      </ArtTableHeader>
      <ElAlert
        v-if="reminder"
        class="page-alert"
        type="info"
        :closable="false"
        show-icon
        :title="reminder"
      />
      <p class="limit-line">
        单文件上限：GitHub {{ limits.github }}，Gitee {{ limits.gitee }}，对象存储
        {{ limits.s3 }}，WebDAV
        {{ limits.webdav }}。超过上限会自动分片，下载时再拼回去并核对校验码。
      </p>
      <ArtTable :loading="loading" :data="rows" :columns="columns">
        <template #name="{ row }">
          <span class="cell-one-line">{{ row.name }}</span>
        </template>
        <template #kindText="{ row }">
          <span class="cell-one-line">{{ row.kindText }}</span>
        </template>
        <template #roleText="{ row }">
          <ElTag :type="row.role === 'primary' ? 'primary' : 'info'" size="small">{{
            row.roleText
          }}</ElTag>
        </template>
        <template #target="{ row }">
          <span class="cell-one-line">{{ targetText(row) }}</span>
        </template>
        <template #secret="{ row }">
          <span class="cell-one-line">{{ row.hasSecret ? '已加密保存' : '未填写' }}</span>
        </template>
        <template #operation="{ row }">
          <RowActions
            :primary="[{ key: 'edit', label: '编辑' }]"
            :more="[
              { key: 'files', label: '文件' },
              { key: 'disable', label: row.role === 'disabled' ? '改为备用' : '停用' },
              { key: 'delete', label: '删除', danger: true }
            ]"
            @click="(action) => onRow(row, action.key)"
          />
        </template>
      </ArtTable>
    </ElCard>

    <AppDialog
      v-model="dialogVisible"
      :title="editing ? '编辑存储' : '新增存储'"
      size="lg"
      flow="long"
      destroy-on-close
    >
      <ElForm :model="form" label-position="top">
        <ElFormItem label="名称">
          <ElInput v-model.trim="form.name" placeholder="例如主仓库、阿里云 OSS" />
        </ElFormItem>
        <ElFormItem label="类型">
          <ElSelect v-model="form.kind" :disabled="Boolean(editing)" style="width: 100%">
            <ElOption label="GitHub 私有仓库" value="github" />
            <ElOption label="Gitee 私有仓库" value="gitee" />
            <ElOption label="对象存储（OSS / COS / R2 / MinIO）" value="s3" />
            <ElOption label="WebDAV 网盘" value="webdav" />
          </ElSelect>
        </ElFormItem>
        <ElFormItem label="角色">
          <ElRadioGroup v-model="form.role">
            <ElRadio value="primary">主存储</ElRadio>
            <ElRadio value="backup">备用</ElRadio>
            <ElRadio value="disabled">停用</ElRadio>
          </ElRadioGroup>
        </ElFormItem>
        <template v-if="form.kind === 's3'">
          <ElFormItem label="Endpoint">
            <ElInput v-model.trim="form.endpoint" placeholder="https:// 开头的兼容地址" />
          </ElFormItem>
          <ElFormItem label="地域">
            <ElInput v-model.trim="form.region" placeholder="例如 oss-cn-hangzhou" />
          </ElFormItem>
          <ElFormItem label="Bucket">
            <ElInput v-model.trim="form.bucket" />
          </ElFormItem>
          <ElFormItem label="AccessKey">
            <ElInput v-model.trim="form.accessKey" placeholder="留空表示不更换" />
          </ElFormItem>
          <ElFormItem label="路径前缀">
            <ElInput v-model.trim="form.prefix" placeholder="可留空" />
          </ElFormItem>
          <ElFormItem label="路径风格">
            <ElSwitch v-model="form.pathStyle" />
            <span class="field-hint">MinIO 常用路径风格。OSS、COS、R2 一般关闭。</span>
          </ElFormItem>
        </template>
        <template v-else-if="form.kind === 'webdav'">
          <ElFormItem label="网盘地址">
            <ElInput
              v-model.trim="form.endpoint"
              placeholder="https://cloud.example.com/remote.php/dav/files/用户名/"
            />
          </ElFormItem>
          <ElFormItem label="用户名">
            <ElInput
              v-model.trim="form.accessKey"
              :placeholder="editing ? '已保存，留空表示不更换' : '账号。应用令牌仍要填用户名'"
            />
          </ElFormItem>
          <ElFormItem label="路径前缀">
            <ElInput v-model.trim="form.prefix" placeholder="可留空，例如 packages" />
          </ElFormItem>
          <p class="field-hint">
            适用于 Nextcloud、ownCloud、Seafile、Alist、Cloudreve 等提供 WebDAV 的自建网盘。自建
            MinIO 请选对象存储。买家只拿到官网票据，网盘账号不会出现在下载地址里。
          </p>
        </template>
        <template v-else>
          <ElFormItem label="所有者">
            <ElInput v-model.trim="form.owner" placeholder="组织或用户名" />
          </ElFormItem>
          <ElFormItem label="仓库">
            <ElInput v-model.trim="form.repo" placeholder="私有仓库名" />
          </ElFormItem>
          <ElFormItem v-if="form.kind === 'gitee'" label="分支">
            <ElInput v-model.trim="form.branch" placeholder="master" />
          </ElFormItem>
        </template>
        <ElFormItem :label="form.kind === 'webdav' ? '密码或应用令牌' : '密钥'">
          <ElInput
            v-model="form.secret"
            type="password"
            show-password
            :placeholder="
              editing
                ? '已保存，留空表示不更换'
                : form.kind === 'webdav'
                  ? '应用令牌填在这里。只保存到服务器，之后不能查看'
                  : '只保存到服务器，之后不能查看'
            "
          />
        </ElFormItem>
      </ElForm>
      <template #footer>
        <ElButton @click="dialogVisible = false">取消</ElButton>
        <ElButton :loading="testing" @click="handleTest">测试连接</ElButton>
        <ElButton type="primary" :loading="saving" @click="handleSave">保存</ElButton>
      </template>
    </AppDialog>
  </div>
</template>

<script setup lang="ts">
  import { appConfirm } from '@/utils/app-confirm'
  import AppDialog from '@/components/core/dialog/AppDialog.vue'

  import { useRouter } from 'vue-router'
  import RowActions from '@/components/business/row-actions/index.vue'
  import { useTableColumns } from '@/hooks/core/useTableColumns'
  import {
    createStorageLocation,
    deleteStorageLocation,
    fetchStorageLocations,
    saveStorageOptions,
    testStorageLocation,
    updateStorageLocation,
    type StorageLocation,
    type StorageLocationInput
  } from '@/api/source-station'

  defineOptions({ name: 'SourceStationStorage' })

  const router = useRouter()
  const loading = ref(false)
  const saving = ref(false)
  const testing = ref(false)
  const savingOption = ref(false)
  const dualWrite = ref(false)
  const reminder = ref('')
  const limits = reactive({ github: '2 GiB', gitee: '100 MB', s3: '5 GiB', webdav: '512 MB' })
  const rows = ref<StorageLocation[]>([])
  const dialogVisible = ref(false)
  const editing = ref<StorageLocation | null>(null)

  const form = reactive<StorageLocationInput>({
    name: '',
    kind: 's3',
    role: 'backup',
    owner: '',
    repo: '',
    branch: 'master',
    endpoint: '',
    region: '',
    bucket: '',
    prefix: '',
    pathStyle: false,
    accessKey: '',
    secret: ''
  })

  const { columns } = useTableColumns<StorageLocation>(() => [
    { prop: 'name', label: '名称', minWidth: 140, useSlot: true },
    { prop: 'kindText', label: '类型', minWidth: 160, useSlot: true },
    { prop: 'roleText', label: '角色', width: 90, useSlot: true },
    { prop: 'target', label: '位置', minWidth: 180, useSlot: true },
    { prop: 'limitText', label: '单文件上限', width: 110 },
    { prop: 'secret', label: '密钥', width: 110, useSlot: true },
    { prop: 'operation', label: '操作', width: 148, fixed: 'right', useSlot: true }
  ])

  function targetText(row: StorageLocation) {
    if (row.kind === 's3') {
      return [row.bucket, row.region].filter(Boolean).join(' · ') || row.endpoint || '-'
    }
    if (row.kind === 'webdav') {
      return row.endpoint || '-'
    }
    return [row.owner, row.repo].filter(Boolean).join(' / ') || '-'
  }

  function resetForm(row?: StorageLocation) {
    editing.value = row || null
    form.name = row?.name || ''
    form.kind = row?.kind || 's3'
    form.role =
      row?.role || (rows.value.some((item) => item.role === 'primary') ? 'backup' : 'primary')
    form.owner = row?.owner || ''
    form.repo = row?.repo || ''
    form.branch = row?.branch || 'master'
    form.endpoint = row?.endpoint || ''
    form.region = row?.region || ''
    form.bucket = row?.bucket || ''
    form.prefix = row?.prefix || ''
    form.pathStyle = Boolean(row?.pathStyle)
    form.accessKey = ''
    form.secret = ''
  }

  async function loadList() {
    loading.value = true
    try {
      const data = await fetchStorageLocations()
      rows.value = data.locations || []
      dualWrite.value = Boolean(data.dualWrite)
      reminder.value = data.reminder || ''
      if (data.limits) {
        limits.github = data.limits.github
        limits.gitee = data.limits.gitee
        limits.s3 = data.limits.s3
        limits.webdav = data.limits.webdav || limits.webdav
      }
    } finally {
      loading.value = false
    }
  }

  function openCreate() {
    resetForm()
    dialogVisible.value = true
  }

  function goFiles() {
    const first = rows.value[0]
    router.push({
      path: '/source-station/storage-files',
      query: first ? { locationId: first.id } : undefined
    })
  }

  async function onDualWrite(value: string | number | boolean) {
    savingOption.value = true
    try {
      const data = await saveStorageOptions(Boolean(value))
      dualWrite.value = Boolean(data.dualWrite)
    } catch {
      dualWrite.value = !value
    } finally {
      savingOption.value = false
    }
  }

  function payload(): StorageLocationInput {
    return {
      id: editing.value?.id,
      name: form.name,
      kind: form.kind,
      role: form.role,
      owner: form.owner,
      repo: form.repo,
      branch: form.branch,
      endpoint: form.endpoint,
      region: form.region,
      bucket: form.bucket,
      prefix: form.prefix,
      pathStyle: form.pathStyle,
      accessKey: form.accessKey,
      secret: form.secret
    }
  }

  async function handleTest() {
    testing.value = true
    try {
      await testStorageLocation(payload())
    } finally {
      testing.value = false
    }
  }

  async function handleSave() {
    saving.value = true
    try {
      if (editing.value) {
        await updateStorageLocation(editing.value.id, payload())
      } else {
        await createStorageLocation(payload())
      }
      dialogVisible.value = false
      await loadList()
    } finally {
      saving.value = false
    }
  }

  async function onRow(row: StorageLocation, key: string) {
    if (key === 'edit') {
      resetForm(row)
      dialogVisible.value = true
      return
    }
    if (key === 'files') {
      router.push({ path: '/source-station/storage-files', query: { locationId: row.id } })
      return
    }
    if (key === 'disable') {
      await updateStorageLocation(row.id, {
        name: row.name,
        kind: row.kind,
        role: row.role === 'disabled' ? 'backup' : 'disabled'
      })
      await loadList()
      return
    }
    if (key === 'delete') {
      try {
        await appConfirm(`删除「${row.name}」后，新的安装包不会再写入这里。`, '删除存储', {
          type: 'info',
          confirmButtonText: '删除',
          cancelButtonText: '取消'
        })
      } catch {
        return
      }
      await deleteStorageLocation(row.id)
      await loadList()
    }
  }

  onMounted(loadList)
</script>

<style scoped lang="scss">
  .storage-page {
    .page-alert,
    .limit-line {
      margin-bottom: 12px;
    }
  }

  .limit-line,
  .field-hint {
    margin: 6px 0 0;
    color: var(--art-gray-600);
    font-size: 13px;
    line-height: 1.5;
  }

  .dual-write {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    color: var(--art-gray-700);
    font-size: 13px;
  }

  .cell-one-line {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
