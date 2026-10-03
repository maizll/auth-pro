<!-- 官网在线更新页里的「更新来源」：保存只读令牌，发布仓库改成私有后官网照样能检查和下载更新。客户站不显示。 -->
<template>
  <div class="update-section source-section">
    <div class="section-header">
      <div>
        <strong>更新来源</strong>
        <span class="section-description">发布仓库 {{ source.repository || '未设置' }}</span>
      </div>
      <div class="source-actions">
        <ElButton link type="primary" :loading="testing" @click="handleTest">测试读取</ElButton>
        <ElButton link type="primary" @click="openDialog">
          {{ source.tokenSaved ? '更换令牌' : '设置令牌' }}
        </ElButton>
      </div>
    </div>
    <div class="source-grid">
      <div class="source-item">
        <span>官网更新令牌</span>
        <strong>{{ source.tokenSaved ? `已保存（尾号 ${source.tokenHint}）` : '未保存' }}</strong>
      </div>
      <div class="source-item">
        <span>最近一次读取</span>
        <strong :class="{ failed: source.lastCheck && !source.lastCheck.ok }">{{
          lastCheckText
        }}</strong>
      </div>
    </div>
    <ElAlert
      v-if="!source.tokenSaved"
      title="仓库改成私有前，先在这里保存一个只读令牌。不保存的话，改私有后官网检查更新会失败。"
      type="info"
      show-icon
      :closable="false"
      class="source-alert"
    />
    <ElAlert
      v-if="testResult"
      :title="testSummary"
      :type="testResult.tokenOk ? 'success' : 'error'"
      show-icon
      :closable="false"
      class="source-alert"
    />

    <AppDialog v-model="dialogOpen" title="官网更新令牌" size="sm" flow="short">
      <p class="dialog-tip">
        只给发布仓库「Contents
        只读」权限的细粒度令牌。保存后官网检查和下载更新只用它，加密存在数据目录，不进数据库。
      </p>
      <ElInput
        v-model="tokenInput"
        type="password"
        show-password
        autocomplete="off"
        placeholder="粘贴令牌"
      />
      <template #footer>
        <ElButton v-if="source.tokenSaved" :loading="saving" @click="handleSave('')">
          删除令牌
        </ElButton>
        <ElButton @click="dialogOpen = false">取消</ElButton>
        <ElButton
          type="primary"
          :loading="saving"
          :disabled="!tokenInput.trim()"
          @click="handleSave(tokenInput)"
        >
          保存
        </ElButton>
      </template>
    </AppDialog>
  </div>
</template>

<script setup lang="ts">
  import { computed, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import AppDialog from '@/components/core/dialog/AppDialog.vue'
  import {
    saveOfficialUpdateToken,
    testOfficialUpdateSource,
    type OfficialUpdateSource,
    type OfficialUpdateSourceTest
  } from '@/api/update'

  defineOptions({ name: 'OfficialUpdateSource' })

  const props = defineProps<{ source: OfficialUpdateSource }>()
  const emit = defineEmits<{ changed: [source: OfficialUpdateSource] }>()

  const dialogOpen = ref(false)
  const tokenInput = ref('')
  const saving = ref(false)
  const testing = ref(false)
  const testResult = ref<OfficialUpdateSourceTest | null>(null)

  const credentialLabel: Record<string, string> = {
    official_token: '官网更新令牌',
    saved_token: '分发令牌',
    anonymous: '匿名'
  }

  const lastCheckText = computed(() => {
    const check = props.source.lastCheck
    if (!check) return '本次启动后还没有检查过'
    const who = credentialLabel[check.credential] || check.credential
    const at = new Date(check.at).toLocaleString('zh-CN', { hour12: false })
    return check.ok ? `${at} 用${who}读取成功` : `${at} 用${who}读取失败`
  })

  const testSummary = computed(() => {
    const result = testResult.value
    if (!result) return ''
    const token = result.tokenOk
      ? `令牌可以读取，最新版本 v${result.latestVersion}`
      : result.tokenMessage
    const open = result.anonymousReadable
      ? '仓库目前仍可匿名读取（还是公开的）'
      : '匿名读取不到（仓库已是私有，或网络不通）'
    return `${token}；${open}`
  })

  const openDialog = () => {
    tokenInput.value = ''
    dialogOpen.value = true
  }

  const handleSave = async (token: string) => {
    saving.value = true
    try {
      const next = await saveOfficialUpdateToken(token.trim())
      emit('changed', next)
      testResult.value = null
      dialogOpen.value = false
      ElMessage.success(token.trim() ? '令牌已保存' : '令牌已删除')
    } catch (error: any) {
      ElMessage.error(error?.message || '保存令牌失败')
    } finally {
      tokenInput.value = ''
      saving.value = false
    }
  }

  const handleTest = async () => {
    testing.value = true
    try {
      testResult.value = await testOfficialUpdateSource()
    } catch (error: any) {
      ElMessage.error(error?.message || '测试读取失败')
    } finally {
      testing.value = false
    }
  }
</script>

<style scoped lang="scss">
  /* 根节点的分隔线和间距沿用父页面的 .update-section；里面的标题行父页面的样式管不到，这里单独写。 */
  .section-header {
    display: flex;
    flex-wrap: wrap;
    gap: 8px 12px;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 14px;

    strong {
      font-size: 15px;
      color: var(--art-gray-900);
    }

    .section-description {
      margin-left: 10px;
      font-size: 12px;
      white-space: nowrap;
      color: var(--art-gray-500);
    }
  }

  .source-actions {
    display: flex;
    gap: 12px;
  }

  .source-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: 12px;
  }

  .source-item {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 13px;
    color: var(--el-text-color-secondary);

    strong {
      font-size: 14px;
      font-weight: 500;
      color: var(--el-text-color-primary);
    }

    strong.failed {
      color: var(--el-color-danger);
    }
  }

  .source-alert {
    margin-top: 12px;
  }

  .dialog-tip {
    margin: 0 0 12px;
    font-size: 13px;
    line-height: 1.6;
    color: var(--el-text-color-regular);
  }
</style>
