<!-- 应用管理页面 -->
<!-- art-full-height 自动计算出页面剩余高度 -->
<!-- art-table-card 一个符合系统样式的 class，同时自动撑满剩余高度 -->
<template>
  <div class="license-apps-page art-full-height">
    <ElCard class="art-table-card no-search-card" shadow="never">
      <!-- 表格头部 -->
      <ArtTableHeader v-model:columns="columnChecks" :loading="loading" @refresh="refreshData">
        <template #left>
          <ElSpace wrap>
            <ElButton @click="handleAdd" v-ripple>新增应用</ElButton>
          </ElSpace>
        </template>
      </ArtTableHeader>

      <!-- 表格 -->
      <ArtTable :loading="loading" :data="data" :columns="columns">
        <!-- 授权方式 -->
        <template #purchaseLicenseTypes="{ row }">
          <div v-if="row.purchaseLicenseTypes?.length" class="license-type-tags">
            <ElTag
              v-for="licenseType in orderedPurchaseLicenseTypes(row.purchaseLicenseTypes)"
              :key="licenseType"
              :type="purchaseLicenseTypeMeta[licenseType]?.tagType"
              size="small"
              effect="plain"
            >
              {{ purchaseLicenseTypeMeta[licenseType]?.label || licenseType }}
            </ElTag>
          </div>
          <ElTag v-else type="info" size="small">已关闭购买</ElTag>
        </template>

        <!-- AppSecret -->
        <template #appSecret="{ row }">
          <BizCopySecret
            :value="row.appSecret"
            success-text="已复制"
            fail-text="复制失败，请手动复制"
          />
        </template>

        <!-- 版本 -->
        <template #version="{ row }">
          <ElButton link type="primary" @click="handleVersions(row)">
            {{ row.recentVersion || '未发布' }}
          </ElButton>
          <span v-if="row.versionCount" class="version-count">{{ row.versionCount }}</span>
        </template>

        <!-- 状态 -->
        <template #enabled="{ row }">
          <ElTag :type="row.enabled ? 'success' : 'info'" size="small">
            {{ row.enabled ? '启用' : '禁用' }}
          </ElTag>
        </template>

        <!-- 授权校验 -->
        <template #licenseRequired="{ row }">
          <ElSwitch
            v-model="row.licenseRequired"
            :loading="row.licenseRequiredChanging"
            :before-change="() => handleLicenseRequiredChange(row)"
          />
        </template>

        <!-- 操作 -->
        <template #operation="{ row }">
          <ElButton link type="primary" @click="handleVersions(row)">版本</ElButton>
          <ElButton link type="primary" @click="handleSDKPack(row)">SDK 包</ElButton>
          <ElButton link type="primary" @click="handleEdit(row)">编辑</ElButton>
          <ElButton link type="primary" @click="handleResetSecret(row)">重置密钥</ElButton>
          <ElButton link type="danger" @click="handleDelete(row)">删除</ElButton>
        </template>
      </ArtTable>
    </ElCard>

    <!-- 新增/编辑弹窗 -->
    <ElDialog v-model="dialogVisible" :title="dialogTitle" width="560px" destroy-on-close>
      <ElForm :model="formData" :rules="formRules" ref="formRef" label-width="100px">
        <ElFormItem label="应用名称" prop="name">
          <ElInput v-model="formData.name" placeholder="请输入应用名称" />
        </ElFormItem>
        <ElFormItem label="授权方式">
          <ElCheckboxGroup v-model="formData.purchaseLicenseTypes" class="license-type-options">
            <ElCheckbox
              v-for="licenseType in purchaseLicenseTypeOrder"
              :key="licenseType"
              :value="licenseType"
            >
              {{ purchaseLicenseTypeMeta[licenseType].label }}
            </ElCheckbox>
          </ElCheckboxGroup>
          <div class="form-tip">全部取消后，用户端和代理端将不再显示该应用。</div>
        </ElFormItem>
        <ElFormItem label="回调地址">
          <ElInput v-model="formData.callbackUrl" placeholder="授权验证回调URL（可选）" />
        </ElFormItem>
        <ElFormItem label="状态">
          <ElSwitch v-model="formData.enabled" active-text="启用" inactive-text="禁用" />
        </ElFormItem>
        <ElFormItem label="备注">
          <ElInput v-model="formData.remark" type="textarea" :rows="2" placeholder="可选" />
        </ElFormItem>
      </ElForm>
      <template #footer>
        <ElButton @click="dialogVisible = false">取消</ElButton>
        <ElButton type="primary" @click="handleSubmit">确定</ElButton>
      </template>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { useRouter } from 'vue-router'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { useTable } from '@/hooks/core/useTable'
  import {
    fetchLicenseAppList,
    fetchCreateLicenseApp,
    fetchUpdateLicenseApp,
    fetchDeleteLicenseApp,
    fetchResetAppSecret,
    fetchUpdateAppLicenseRequired,
    type LicenseAppItem
  } from '@/api/license-manage'

  defineOptions({ name: 'LicenseApps' })

  /** 行级本地状态 */
  type AppRow = LicenseAppItem & {
    licenseRequiredChanging: boolean
  }

  const router = useRouter()

  // 弹窗相关
  const dialogVisible = ref(false)
  const isEdit = ref(false)
  const dialogTitle = computed(() => (isEdit.value ? '编辑应用' : '新增应用'))

  const purchaseLicenseTypeOrder = ['domain', 'wildcard', 'ip', 'key'] as const
  const purchaseLicenseTypeMeta: Record<
    string,
    { label: string; tagType: 'primary' | 'success' | 'warning' | 'info' }
  > = {
    domain: { label: '单域名', tagType: 'primary' },
    wildcard: { label: '泛域名', tagType: 'success' },
    ip: { label: 'IP', tagType: 'warning' },
    key: { label: '密钥', tagType: 'info' }
  }

  const formRef = ref()
  const formData = reactive({
    id: 0,
    name: '',
    callbackUrl: '',
    enabled: true,
    remark: '',
    purchaseLicenseTypes: [...purchaseLicenseTypeOrder] as string[]
  })

  const formRules = {
    name: [{ required: true, message: '请输入应用名称', trigger: 'blur' }]
  }

  const { columns, columnChecks, data, loading, refreshData, refreshRemove } = useTable({
    // 核心配置
    core: {
      apiFn: fetchLicenseAppList,
      apiParams: {},
      columnsFactory: () => [
        { type: 'index', width: 60, label: '序号' }, // 序号
        { prop: 'name', label: '应用名称', minWidth: 150, showOverflowTooltip: true },
        { prop: 'appKey', label: 'AppKey', minWidth: 220, showOverflowTooltip: true },
        {
          prop: 'purchaseLicenseTypes',
          label: '授权方式',
          minWidth: 250,
          useSlot: true
        },
        { prop: 'appSecret', label: 'AppSecret', minWidth: 220, useSlot: true },
        { prop: 'licenseCount', label: '授权数', width: 90, align: 'center' },
        { prop: 'version', label: '版本', minWidth: 120, useSlot: true },
        { prop: 'enabled', label: '状态', width: 90, align: 'center', useSlot: true },
        {
          prop: 'licenseRequired',
          label: '授权校验',
          width: 100,
          align: 'center',
          useSlot: true
        },
        { prop: 'createdAt', label: '创建时间', width: 160 },
        {
          prop: 'operation',
          label: '操作',
          width: 230,
          fixed: 'right',
          useSlot: true
        }
      ]
    },
    // 数据处理
    transform: {
      dataTransformer: (records) => {
        if (!Array.isArray(records)) {
          return []
        }
        // 附加行级本地状态：密钥可见性、授权校验切换中
        const normalized = (records as unknown as LicenseAppItem[]).map((item) => ({
          ...item,
          licenseRequired: item.licenseRequired !== false,
          licenseRequiredChanging: false
        }))
        return normalized as unknown as typeof records
      }
    }
  })

  const orderedPurchaseLicenseTypes = (types: string[] = []) => {
    return purchaseLicenseTypeOrder.filter((licenseType) => types.includes(licenseType))
  }

  const handleAdd = () => {
    isEdit.value = false
    formData.id = 0
    formData.name = ''
    formData.callbackUrl = ''
    formData.enabled = true
    formData.remark = ''
    formData.purchaseLicenseTypes = [...purchaseLicenseTypeOrder]
    dialogVisible.value = true
  }

  const handleEdit = (row: AppRow) => {
    isEdit.value = true
    formData.id = row.id
    formData.name = row.name
    formData.callbackUrl = ''
    formData.enabled = row.enabled
    formData.remark = (row as LicenseAppItem & { remark?: string }).remark || ''
    formData.purchaseLicenseTypes = Array.isArray(row.purchaseLicenseTypes)
      ? [...row.purchaseLicenseTypes]
      : [...purchaseLicenseTypeOrder]
    dialogVisible.value = true
  }

  const handleLicenseRequiredChange = async (row: AppRow) => {
    const licenseRequired = !row.licenseRequired
    if (!licenseRequired) {
      try {
        await ElMessageBox.confirm(
          `关闭应用「${row.name}」的授权校验后，客户端无需许可证即可通过验证和版本检查。应用签名与启用状态校验仍然有效，是否继续？`,
          '关闭授权校验',
          {
            type: 'warning',
            confirmButtonText: '确认关闭',
            cancelButtonText: '取消'
          }
        )
      } catch {
        return false
      }
    }

    row.licenseRequiredChanging = true
    try {
      await fetchUpdateAppLicenseRequired(row.id, licenseRequired)
      ElMessage.success(licenseRequired ? '已要求授权校验' : '已关闭授权校验')
      return true
    } catch (e) {
      console.error('[AppManage] 更新授权校验失败:', e)
      return false
    } finally {
      row.licenseRequiredChanging = false
    }
  }

  const handleResetSecret = async (row: AppRow) => {
    try {
      await ElMessageBox.confirm(
        `确定重置应用「${row.name}」的AppSecret？旧密钥将立即失效`,
        '警告',
        { type: 'warning' }
      )
      const data = await fetchResetAppSecret(row.id)
      row.appSecret = data.appSecret
      ElMessage.success('密钥已重置')
    } catch {
      // 用户取消操作时保留当前数据。
    }
  }

  const handleVersions = (row: AppRow) => {
    router.push(`/license/apps/${row.id}/versions`)
  }

  const handleSDKPack = (row: AppRow) => {
    router.push({ name: 'SdkIndex', query: { appId: String(row.id) } })
  }

  const handleDelete = async (row: AppRow) => {
    try {
      await ElMessageBox.confirm(
        `删除应用「${row.name}」将同时清除其所有授权记录，确定？`,
        '危险操作',
        { type: 'error' }
      )
      await fetchDeleteLicenseApp(row.id)
      ElMessage.success('删除成功')
      refreshRemove()
    } catch {
      // 用户取消操作时保留当前数据。
    }
  }

  const handleSubmit = async () => {
    const valid = await formRef.value?.validate().catch(() => false)
    if (!valid) return

    try {
      if (isEdit.value) {
        await fetchUpdateLicenseApp(formData.id, {
          name: formData.name,
          enabled: formData.enabled,
          remark: formData.remark,
          purchaseLicenseTypes: formData.purchaseLicenseTypes
        })
        ElMessage.success('编辑成功')
      } else {
        await fetchCreateLicenseApp({
          name: formData.name,
          enabled: formData.enabled,
          remark: formData.remark,
          purchaseLicenseTypes: formData.purchaseLicenseTypes
        })
        ElMessage.success('新增成功')
      }
      dialogVisible.value = false
      refreshData()
    } catch (e) {
      console.error('[AppManage] 提交失败:', e)
    }
  }

  // keep-alive 从版本管理返回时刷新列表（首次挂载由 useTable immediate 加载，跳过避免重复请求）
  let activatedOnce = false
  onActivated(() => {
    if (activatedOnce) {
      refreshData()
    }
    activatedOnce = true
  })
</script>

<style scoped lang="scss">
  .license-apps-page {
    // 无搜索栏时去掉表格卡片的上间距
    .no-search-card {
      margin-top: 0;
    }

    .version-count {
      margin-left: 6px;
      font-size: 12px;
      color: var(--art-gray-600);
    }

    .license-type-tags,
    .license-type-options {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
    }

    .license-type-options :deep(.el-checkbox) {
      margin-right: 12px;
    }

    .form-tip {
      width: 100%;
      margin-top: 4px;
      font-size: 12px;
      line-height: 1.5;
      color: var(--el-text-color-secondary);
    }
  }
</style>
