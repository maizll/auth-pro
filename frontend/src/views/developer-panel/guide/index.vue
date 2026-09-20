<template>
  <div class="developer-guide">
    <el-card shadow="never" class="mb-5">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">接入说明</span>
            <p class="card-hint">
              给插件 / 首页模板作者使用。管理端「按应用下载的 AuthPro 客户端 SDK ZIP」是授权接入包，不是本页的开发者入门包。
            </p>
          </div>
          <div class="header-actions">
            <el-button :loading="downloadingStarter" @click="handleDownloadStarter">
              下载入门包 ZIP
            </el-button>
            <el-button type="primary" :loading="downloadingSkill" @click="handleDownloadSkill">
              下载 AI Skill（SKILL.md）
            </el-button>
          </div>
        </div>
      </template>
      <el-alert
        type="warning"
        :closable="false"
        show-icon
        class="mb-4"
        title="源站不存储源码。你把 ZIP 放到自己的 HTTPS 空间，面板里只登记元数据、downloadUrl/templateUrl 和 sha256。"
      />
    </el-card>

    <el-card shadow="never" class="mb-5">
      <template #header>
        <span class="card-title">打包规则</span>
      </template>
      <el-descriptions :column="1" border>
        <el-descriptions-item label="插件">
          ZIP 根目录或一层子目录必须有 plugin.json。必填：id、name、version、description、author。
        </el-descriptions-item>
        <el-descriptions-item label="首页模板">
          必须有 template.json。必填：id 或 templateKey、name、version、description、author、schemaVersion=1、hero.title。禁止
          scripts。
        </el-descriptions-item>
        <el-descriptions-item label="标识格式">
          id 为 2-59 位小写字母、数字或连字符；version 允许数字、字母和 . + _ -
        </el-descriptions-item>
        <el-descriptions-item label="硬校验拒绝">
          非 ZIP、超过 20 MiB、路径穿越、符号链接、缺清单、JSON 非法。失败不写库、不留临时文件。
        </el-descriptions-item>
      </el-descriptions>
    </el-card>

    <el-card shadow="never" class="mb-5">
      <template #header>
        <span class="card-title">应用隔离与公开目录</span>
      </template>
      <p class="body-text">
        每个插件/模板必须绑定一个应用。客户端应请求：
      </p>
      <pre class="code-block">/software-source/{app_key}/index.json</pre>
      <p class="body-text">
        未带应用的 <code>/software-source/index.json</code> 是空目录，不要当默认源。开发者「我的插件 / 我的模板」里选择的应用，其
        indexUrl 可直接拼站点 origin。
      </p>
    </el-card>

    <el-card shadow="never" class="mb-5">
      <template #header>
        <span class="card-title">审核流</span>
      </template>
      <p class="body-text">目录项：草稿 → 待审核 → 已通过 → 已上架。驳回后可改再提交。已发布版本不可改包地址，请新增版本。</p>
      <p class="body-text">广告申请单独走 pending / approved / rejected，通过后才会生成真实投放。</p>
    </el-card>

    <el-card shadow="never">
      <template #header>
        <span class="card-title">给 AI 编码工具</span>
      </template>
      <p class="body-text">
        将下载的 <code>SKILL.md</code> 放到项目
        <code>.cursor/skills/auth-pro-plugin-template/SKILL.md</code>（入门包 ZIP 内已含此路径），或复制仓库
        <code>developer-skills/auth-pro-plugin-template/SKILL.md</code>。完整中文规范也在仓库
        <code>docs/developer/</code>。
      </p>
      <p class="body-text muted">
        机器可读 schema：<code>GET /software-source/package-schema.json</code>
      </p>
    </el-card>
  </div>
</template>

<script setup lang="ts">
  import { ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import {
    downloadSourceDeveloperSkill,
    downloadSourceDeveloperStarter
  } from '@/api/source-developer'

  const downloadingStarter = ref(false)
  const downloadingSkill = ref(false)

  async function handleDownloadStarter() {
    downloadingStarter.value = true
    try {
      await downloadSourceDeveloperStarter()
      ElMessage.success('已开始下载入门包')
    } catch (error: unknown) {
      ElMessage.error(error instanceof Error ? error.message : '下载入门包失败')
    } finally {
      downloadingStarter.value = false
    }
  }

  async function handleDownloadSkill() {
    downloadingSkill.value = true
    try {
      await downloadSourceDeveloperSkill()
      ElMessage.success('已开始下载 SKILL.md')
    } catch (error: unknown) {
      ElMessage.error(error instanceof Error ? error.message : '下载 Skill 失败')
    } finally {
      downloadingSkill.value = false
    }
  }
</script>

<style scoped lang="scss">
  .table-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
  }

  .header-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .card-title {
    font-weight: 600;
  }

  .card-hint,
  .body-text {
    margin: 6px 0 0;
    font-size: 13px;
    line-height: 1.7;
    color: var(--el-text-color-regular);
  }

  .muted {
    color: var(--el-text-color-secondary);
  }

  .code-block {
    margin: 10px 0;
    padding: 10px 12px;
    overflow: auto;
    font-size: 13px;
    background: var(--el-fill-color-light);
    border-radius: 8px;
  }

  .mb-4 {
    margin-bottom: 16px;
  }

  .mb-5 {
    margin-bottom: 20px;
  }
</style>
