<!-- 编辑一条软件目录：名称、价格、安装包和上下架。 -->
<template>
  <div class="source-station-page">
    <el-alert
      v-if="paidRepoReminder"
      class="mb-4"
      type="info"
      :closable="false"
      show-icon
      :title="paidRepoReminder"
    />
    <el-card shadow="never" class="art-card mb-4 filter-panel">
      <el-form :model="searchForm" inline>
        <el-form-item label="应用">
          <el-select
            v-model="searchForm.appId"
            placeholder="请选择应用"
            style="width: 240px"
            @change="onAppChange"
          >
            <el-option v-for="app in apps" :key="app.id" :label="appLabel(app)" :value="app.id" />
            <el-option label="未归属（应用已删除）" :value="-1" />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-select
            v-model="searchForm.status"
            placeholder="默认（不含已弃用）"
            clearable
            style="width: 180px"
          >
            <el-option label="草稿" value="draft" />
            <el-option label="待审核" value="review" />
            <el-option label="已通过" value="approved" />
            <el-option label="已上架" value="published" />
            <el-option label="已下架" value="hidden" />
            <el-option label="已驳回" value="rejected" />
            <el-option label="已弃用" value="deprecated" />
          </el-select>
        </el-form-item>
        <el-form-item label="分类">
          <el-select
            v-model="searchForm.category"
            placeholder="全部"
            clearable
            style="width: 180px"
          >
            <el-option
              v-for="item in categories"
              :key="item.key"
              :label="item.label"
              :value="item.key"
            />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="loadItems">查询</el-button>
          <el-button @click="resetSearch">重置</el-button>
          <el-button @click="openCategoryManager">管理分类</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" class="art-card">
      <template #header>
        <div class="table-header">
          <div>
            <span class="card-title">软件目录（共 {{ tableData.length }} 条）</span>
            <p class="card-hint" :class="{ 'is-collapsed': narrow && !hintExpanded }">
              先选择应用，再按分类筛选。源站只保存说明和下载地址，不保存源码。上架后会自动出现在该应用的软件源里。下架后商店不再显示，已经安装的不会被远程卸掉。弃用后会从公开目录移除，默认列表也不再显示；需要查看时，把状态筛成「已弃用」。
            </p>
            <el-button
              v-if="narrow"
              link
              type="primary"
              class="hint-toggle"
              @click="hintExpanded = !hintExpanded"
            >
              {{ hintExpanded ? '收起' : '展开' }}
            </el-button>
          </div>
          <div class="table-actions">
            <el-button :disabled="!selectedRows.length" @click="openRebind(selectedRows)"
              >切换绑定应用</el-button
            >
            <el-button :disabled="Boolean(registerBlockReason)" @click="openRegister"
              >登记外部地址</el-button
            >
            <el-button type="primary" :disabled="Boolean(registerBlockReason)" @click="openUpload"
              >上传压缩包</el-button
            >
          </div>
        </div>
      </template>

      <el-table :data="tableData" stripe v-loading="loading" @selection-change="onSelectionChange">
        <el-table-column type="selection" width="42" />
        <el-table-column prop="id" label="标识" min-width="140" />
        <el-table-column prop="name" label="名称" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">
            <div class="cell-one-line-row">
              <span>{{ row.name }}</span>
              <el-tag size="small" effect="plain">{{ listingLabel(row) }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="托管说明" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">{{ row.fulfillmentHint || '-' }}</template>
        </el-table-column>
        <el-table-column label="分类" width="120">
          <template #default="{ row }">
            {{ row.categoryLabel || categoryLabel(row.category) }}
          </template>
        </el-table-column>
        <el-table-column label="当前版本" width="110">
          <template #default="{ row }">
            {{ row.latestVersion || row.version || '-' }}
          </template>
        </el-table-column>
        <el-table-column label="售价" width="100">
          <template #default="{ row }">{{ formatCatalogPriceLabel(row.priceCents) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="statusMeta(row.status).type" size="small">
              {{ statusMeta(row.status).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="下载地址" min-width="200">
          <template #default="{ row }">
            {{ itemLocation(row) }}
          </template>
        </el-table-column>
        <el-table-column label="来源外链" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">{{ row.originUrl || '-' }}</template>
        </el-table-column>
        <el-table-column label="来源说明" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">{{ row.originHint || '-' }}</template>
        </el-table-column>
        <el-table-column prop="sha256" label="校验码" min-width="160" />
        <el-table-column prop="updatedAt" label="更新时间" width="170" />
        <el-table-column label="操作" width="176" fixed="right">
          <template #default="{ row }">
            <RowActions
              :primary="catalogPrimary(row)"
              :more="catalogMore(row)"
              @click="(action) => onCatalogAction(row, action)"
            />
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog
      v-model="uploadVisible"
      title="上传安装包"
      width="640px"
      append-to-body
      destroy-on-close
    >
      <el-alert
        type="info"
        :closable="false"
        show-icon
        title="校验不通过就会拒绝：压缩包里要有合法的插件或模板清单，不能包含越界路径。失败不会保存，也不会推送到发布页。"
        class="mb-3"
      />
      <p class="card-hint mb-3">
        来源二选一。免费用公开地址，本站不存包。收费可以上传压缩包，或填写公开地址让本站拉一次。配置收费仓库后，安装包放在站长的私有仓库。买家付款后从官网地址下载，由官网取包并核对校验码。
      </p>
      <el-form label-width="120px">
        <el-form-item label="来源">
          <el-radio-group v-model="uploadForm.source">
            <el-radio value="upload">上传压缩包</el-radio>
            <el-radio value="public">公开地址</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="应用" required>
          <el-select v-model="uploadForm.appId" placeholder="请选择应用" style="width: 100%">
            <el-option
              v-for="app in liveApps"
              :key="app.id"
              :label="appLabel(app)"
              :value="app.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item v-if="uploadForm.source === 'upload'" label="压缩包">
          <el-upload
            drag
            :auto-upload="false"
            :show-file-list="false"
            accept=".zip,application/zip"
            :on-change="selectUploadFile"
          >
            <div>{{ uploadFile ? uploadFile.name : '点击或拖拽压缩包（不超过 20 MB）' }}</div>
          </el-upload>
          <p class="card-hint">
            售价大于 0
            时，配置了收费仓库就上传到该仓库并删除临时文件；未配置则暂存在本站。免费请改用公开地址，或勾选推送
            Release。
          </p>
        </el-form-item>
        <el-form-item label="分类">
          <el-select
            v-model="uploadForm.category"
            clearable
            placeholder="可留空，按包内清单自动识别"
            style="width: 100%"
          >
            <el-option
              v-for="item in uploadCategoryOptions"
              :key="item.key"
              :label="`${item.label}（${item.kind === 'template' ? '模板清单' : '插件清单'}）`"
              :value="item.key"
            />
          </el-select>
        </el-form-item>
        <el-form-item v-if="parsedManifest" label="解析结果">
          <el-descriptions :column="1" border size="small">
            <el-descriptions-item label="分类">{{
              categoryLabel(parsedManifest.category || uploadForm.category)
            }}</el-descriptions-item>
            <el-descriptions-item label="ID">{{ parsedManifest.id }}</el-descriptions-item>
            <el-descriptions-item label="名称">{{ parsedManifest.name }}</el-descriptions-item>
            <el-descriptions-item label="版本">{{ parsedManifest.version }}</el-descriptions-item>
            <el-descriptions-item label="作者">{{
              parsedManifest.author?.name
            }}</el-descriptions-item>
            <el-descriptions-item label="校验码">{{ parsedManifest.sha256 }}</el-descriptions-item>
          </el-descriptions>
        </el-form-item>
        <el-form-item label="更新说明">
          <el-input v-model="uploadForm.changelog" type="textarea" :rows="2" placeholder="可选" />
        </el-form-item>
        <el-form-item label="售价（元）">
          <el-input v-model="uploadForm.priceYuan" placeholder="0" />
          <p class="card-hint">填 0 表示免费。大于 0 为买断，上传压缩包或填写公开地址即可。</p>
        </el-form-item>
        <el-form-item v-if="uploadForm.source === 'upload'" label="选项">
          <el-checkbox v-model="uploadForm.push">推送 GitHub/Gitee Release</el-checkbox>
        </el-form-item>
        <el-form-item v-if="uploadForm.source === 'public'" label="公开地址">
          <el-input
            v-model="uploadForm.location"
            placeholder="https://..."
            @input="parsedManifest = null"
          />
          <p class="card-hint">
            免费条目保存这条地址，本站不存包。收费条目会拉取一次：已配置收费仓库则上传后删除临时文件，未配置则暂存在本站。
          </p>
        </el-form-item>
      </el-form>
      <p v-if="uploadBlockReason" class="card-hint">{{ uploadBlockReason }}</p>
      <template #footer>
        <el-button @click="uploadVisible = false">取消</el-button>
        <el-tooltip :disabled="!uploadBlockReason" :content="uploadBlockReason" placement="top">
          <span>
            <el-button :disabled="!!uploadBlockReason" :loading="parsing" @click="handleParse"
              >仅校验解析</el-button
            >
          </span>
        </el-tooltip>
        <el-tooltip :disabled="!uploadBlockReason" :content="uploadBlockReason" placement="top">
          <span>
            <el-button
              type="primary"
              :disabled="!!uploadBlockReason"
              :loading="publishing"
              @click="handlePublish"
            >
              校验并保存元数据
            </el-button>
          </span>
        </el-tooltip>
      </template>
    </el-dialog>

    <el-dialog
      v-model="editVisible"
      title="编辑目录项"
      :width="narrow ? '92%' : '560px'"
      destroy-on-close
    >
      <el-alert
        type="info"
        :closable="false"
        show-icon
        class="mb-3"
        title="已上架条目可直接改名称、地址、校验码等元数据，不会自动退回待审核。标识创建后不可改。更换所属应用请用「切换应用」，版本、价格和安装包会一起过去。"
      />
      <el-form ref="editRef" :model="editForm" :rules="editRules" label-width="110px">
        <el-form-item label="应用">
          <el-input :model-value="editApp ? appLabel(editApp) : '-'" disabled />
        </el-form-item>
        <el-form-item label="标识">
          <el-input v-model="editForm.id" disabled />
        </el-form-item>
        <el-form-item label="分类" prop="category">
          <el-select v-model="editForm.category" style="width: 100%">
            <el-option
              v-for="item in editCategoryOptions"
              :key="item.key"
              :label="item.label"
              :value="item.key"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="名称" prop="name">
          <el-input v-model="editForm.name" maxlength="100" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="editForm.description" type="textarea" :rows="2" maxlength="500" />
        </el-form-item>
        <el-form-item label="售价（元）">
          <el-input v-model="editForm.priceYuan" placeholder="0" />
          <p class="card-hint">
            填 0
            表示免费。已公开的免费条目可以改为收费，保存前会确认老用户是否继续免费。收费仓库已有安装包时，改回
            0 可以不填外链，下载仍由官网提供。
          </p>
        </el-form-item>
        <el-form-item label="来源">
          <el-radio-group v-model="editForm.party">
            <el-radio value="official">官方</el-radio>
            <el-radio value="third">第三方</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="商业版免费">
          <el-checkbox
            v-model="editForm.commercialIncluded"
            :disabled="editForm.party !== 'official'"
            >商业版用户可直接启用</el-checkbox
          >
          <p class="card-hint">仅官方条目可勾选。官方付费条目默认勾选，第三方不包含在商业版里。</p>
        </el-form-item>
        <el-form-item label="下载地址" prop="location">
          <el-input
            v-model="editForm.location"
            :placeholder="editHostedPackage ? '留空则沿用收费仓库里的安装包' : 'https://...'"
          />
          <p v-if="editHostedPackage" class="card-hint">
            收费仓库已有安装包。改回免费可以不填地址，继续用这份托管包，下载仍由官网提供。填写新的
            https 地址则改用外链。
          </p>
        </el-form-item>
        <el-form-item v-if="editingItem?.originUrl" label="来源外链">
          <el-input :model-value="editingItem.originUrl" disabled />
          <p class="card-hint">
            {{ editingItem.originHint || '本站已拉取并私有托管。买家看不到这条外链。' }}
          </p>
        </el-form-item>
        <el-form-item label="校验码" prop="sha256">
          <el-input v-model="editForm.sha256" />
        </el-form-item>
        <el-form-item label="作者">
          <el-input v-model="editForm.authorName" placeholder="作者名称" />
        </el-form-item>
        <el-form-item v-if="!editIsTemplate" label="图标">
          <el-input v-model="editForm.icon" placeholder="ri:puzzle-line" />
        </el-form-item>
        <el-form-item label="更新说明">
          <el-input v-model="editForm.changelog" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item label="审计备注">
          <el-input v-model="editForm.note" placeholder="可选，写入审计日志" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" :loading="editing" @click="handleEdit">保存</el-button>
      </template>
    </el-dialog>

    <CatalogPriceSwitchDialog v-model="priceSwitchVisible" @confirm="confirmPriceSwitch" />

    <el-dialog
      v-model="registerVisible"
      title="登记外部地址"
      :width="narrow ? '92%' : '560px'"
      destroy-on-close
    >
      <el-form ref="registerRef" :model="registerForm" :rules="registerRules" label-width="110px">
        <el-form-item label="应用" prop="appId">
          <el-select v-model="registerForm.appId" placeholder="请选择应用" style="width: 100%">
            <el-option
              v-for="app in liveApps"
              :key="app.id"
              :label="appLabel(app)"
              :value="app.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="分类" prop="category">
          <el-select v-model="registerForm.category" style="width: 100%">
            <el-option
              v-for="item in categories"
              :key="item.key"
              :label="item.label"
              :value="item.key"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="标识" prop="id">
          <el-input v-model="registerForm.id" />
        </el-form-item>
        <el-form-item label="名称" prop="name">
          <el-input v-model="registerForm.name" />
        </el-form-item>
        <el-form-item label="版本" prop="version">
          <el-input v-model="registerForm.version" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="registerForm.description" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item label="售价（元）">
          <el-input v-model="registerForm.priceYuan" placeholder="0" />
          <p class="card-hint">
            填 0 表示免费。大于 0 请到「上传安装包」上传压缩包或填写公开地址。
          </p>
        </el-form-item>
        <el-form-item label="来源">
          <el-radio-group v-model="registerForm.party">
            <el-radio value="official">官方</el-radio>
            <el-radio value="third">第三方</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="商业版免费">
          <el-checkbox
            v-model="registerForm.commercialIncluded"
            :disabled="registerForm.party !== 'official'"
            >商业版用户可直接启用</el-checkbox
          >
          <p class="card-hint">仅官方条目可勾选。官方付费条目默认勾选，第三方不包含在商业版里。</p>
        </el-form-item>
        <el-form-item label="下载地址" prop="location">
          <el-input v-model="registerForm.location" placeholder="https://..." />
        </el-form-item>
        <el-form-item label="校验码" prop="sha256">
          <el-input v-model="registerForm.sha256" />
        </el-form-item>
        <el-form-item label="作者">
          <el-input v-model="registerForm.authorName" placeholder="作者名称" />
        </el-form-item>
        <el-form-item label="更新说明">
          <el-input v-model="registerForm.changelog" />
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="registerForm.shelf"
            >登记后直接上架（须同时有下载地址和校验码）</el-checkbox
          >
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="registerVisible = false">取消</el-button>
        <el-button type="primary" :loading="registering" @click="handleRegister">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="categoryVisible" title="目录分类" width="640px" destroy-on-close>
      <p class="card-hint mb-3">
        内置分类包含插件和首页模板，也可作为应用里的二级筛选。可以自行添加或删除额外分类。应用商店会按这些分类生成筛选页签。
      </p>
      <el-table :data="categories" size="small" class="mb-3">
        <el-table-column prop="label" label="名称" min-width="120" />
        <el-table-column prop="key" label="标识" min-width="140" />
        <el-table-column label="清单类型" width="110">
          <template #default="{ row }">
            {{ row.kind === 'template' ? '首页模板' : '插件' }}
          </template>
        </el-table-column>
        <el-table-column label="来源" width="80">
          <template #default="{ row }">{{ row.builtin ? '内置' : '自定义' }}</template>
        </el-table-column>
        <el-table-column label="操作" width="80" align="center">
          <template #default="{ row }">
            <el-button
              v-if="canDeleteCatalogCategory(row)"
              link
              type="danger"
              size="small"
              :disabled="savingCategories"
              @click="handleDeleteCategory(row)"
              >删除</el-button
            >
          </template>
        </el-table-column>
      </el-table>
      <el-form :model="extraForm" inline>
        <el-form-item label="新分类标识">
          <el-input v-model="extraForm.key" placeholder="theme" />
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="extraForm.label" placeholder="主题" />
        </el-form-item>
        <el-form-item label="清单">
          <el-select v-model="extraForm.kind" style="width: 140px">
            <el-option label="插件" value="plugin" />
            <el-option label="首页模板" value="template" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="savingCategories" @click="handleAddCategory"
            >添加</el-button
          >
        </el-form-item>
      </el-form>
    </el-dialog>

    <el-drawer
      v-model="versionVisible"
      :title="`${currentItem?.name || '插件'} 的版本`"
      :size="narrow ? '100%' : '720px'"
    >
      <div class="table-actions mb-3">
        <el-button type="primary" @click="openVersionUpload">上传新版本</el-button>
        <el-button @click="versionFormVisible = true">登记外部地址</el-button>
      </div>
      <div v-if="narrow" v-loading="versionLoading" class="version-cards">
        <article v-for="row in versions" :key="row.version" class="version-card">
          <header class="version-card-head">
            <strong>{{ row.version }}</strong>
            <el-tag v-if="row.version === currentItem?.latestVersion" type="success" size="small"
              >最新版</el-tag
            >
            <el-tag v-else type="info" size="small">历史版本</el-tag>
          </header>
          <p>大小：{{ formatPackageSize(row.sizeBytes) }}</p>
          <p>上传时间：{{ formatVersionTime(row.createdAt) }}</p>
          <p>说明：{{ row.changelog || '无' }}</p>
          <div class="version-card-actions">
            <el-button
              v-if="canVersionAction(row.status, 'approve')"
              type="success"
              size="small"
              @click="runVersion(row, 'approve')"
              >通过</el-button
            >
            <el-button
              v-if="canVersionAction(row.status, 'reject')"
              type="info"
              size="small"
              @click="runVersion(row, 'reject')"
              >驳回</el-button
            >
            <el-button
              v-if="showSetLatest(row)"
              type="primary"
              size="small"
              @click="runVersion(row, 'latest')"
              >设为最新版</el-button
            >
            <el-button
              v-if="canVersionAction(row.status, 'deprecate')"
              type="danger"
              size="small"
              @click="runVersion(row, 'deprecate')"
              >弃用</el-button
            >
          </div>
        </article>
        <el-empty v-if="!versionLoading && !versions.length" description="还没有版本" />
      </div>
      <el-table v-else :data="versions" v-loading="versionLoading" stripe>
        <el-table-column prop="version" label="版本" width="110" />
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="versionStatusMeta(row.status).type" size="small">
              {{ versionStatusMeta(row.status).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="是否最新" width="90" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.version === currentItem?.latestVersion" type="success" size="small"
              >最新</el-tag
            >
          </template>
        </el-table-column>
        <el-table-column label="地址" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            {{ currentIsTemplate ? row.templateUrl : row.downloadUrl }}
          </template>
        </el-table-column>
        <el-table-column prop="changelog" label="说明" min-width="140" show-overflow-tooltip />
        <el-table-column label="操作" width="260" fixed="right">
          <template #default="{ row }">
            <el-button
              v-if="canVersionAction(row.status, 'approve')"
              link
              type="success"
              size="small"
              @click="runVersion(row, 'approve')"
              >通过</el-button
            >
            <el-button
              v-if="canVersionAction(row.status, 'reject')"
              link
              type="info"
              size="small"
              @click="runVersion(row, 'reject')"
              >驳回</el-button
            >
            <el-button
              v-if="showSetLatest(row)"
              link
              type="primary"
              size="small"
              @click="runVersion(row, 'latest')"
              >设为最新版</el-button
            >
            <el-button
              v-if="canVersionAction(row.status, 'deprecate')"
              link
              type="danger"
              size="small"
              @click="runVersion(row, 'deprecate')"
              >弃用</el-button
            >
          </template>
        </el-table-column>
      </el-table>

      <el-dialog
        v-model="versionFormVisible"
        title="登记新版本"
        :width="narrow ? '92%' : '520px'"
        append-to-body
        destroy-on-close
      >
        <el-form :model="versionForm" label-width="110px">
          <el-form-item label="从仓库导入">
            <ReleaseRepoImport
              api-base="/api/v1/source/admin/release-import"
              :purpose="currentItem && isTemplateItem(currentItem) ? 'template' : 'plugin'"
              @filled="applyVersionImport"
            />
            <p v-if="versionDigest" class="card-hint">{{ versionDigest }}</p>
          </el-form-item>
          <el-form-item label="版本" required>
            <el-input v-model="versionForm.version" placeholder="1.0.1" />
          </el-form-item>
          <el-form-item label="下载地址" required>
            <div class="url-probe">
              <el-input
                :model-value="versionForm.location"
                placeholder="https://..."
                @update:model-value="onCatalogLocationInput"
              />
              <el-button :loading="versionProbing" @click="probeCatalogVersionUrl"
                >计算大小和校验</el-button
              >
            </div>
            <p v-if="versionDigest" class="card-hint">{{ versionDigest }}</p>
          </el-form-item>
          <el-form-item label="校验码">
            <el-input v-model="versionForm.sha256" placeholder="付费外链可留空，由本站拉取后计算" />
            <p class="card-hint">
              免费外链仍须填写 64 位校验码。收费条目请上传压缩包，或填写公开地址让本站拉取。
            </p>
          </el-form-item>
          <el-form-item label="更新说明">
            <el-input v-model="versionForm.changelog" type="textarea" :rows="2" />
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="versionFormVisible = false">取消</el-button>
          <el-button type="primary" :loading="versionSaving" @click="handleAddVersion"
            >保存</el-button
          >
        </template>
      </el-dialog>
    </el-drawer>

    <el-dialog v-model="rebindVisible" title="切换绑定应用" width="480px" destroy-on-close>
      <p class="card-hint mb-3">
        将把选中的
        {{ rebindItems.length }}
        条改到目标应用。版本、价格和安装包跟着走，标识不变。目标应用里已有相同标识时会拒绝。
      </p>
      <el-select v-model="rebindAppId" placeholder="请选择目标应用" style="width: 100%">
        <el-option v-for="app in liveApps" :key="app.id" :label="appLabel(app)" :value="app.id" />
      </el-select>
      <template #footer>
        <el-button @click="rebindVisible = false">取消</el-button>
        <el-button type="primary" :loading="rebindSaving" @click="handleRebind">确定切换</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
  import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
  import { useRoute } from 'vue-router'
  import type { FormInstance, FormRules, UploadFile } from 'element-plus'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import RowActions, { type RowActionItem } from '@/components/business/row-actions/index.vue'
  import ReleaseRepoImport from '@/components/business/release-import/ReleaseRepoImport.vue'
  import {
    materializeRelease,
    probeReleaseUrl,
    type ReleaseImportResult
  } from '@/api/release-import'
  import { useNarrowScreen } from '@/hooks/core/useNarrowScreen'
  import CatalogPriceSwitchDialog from './CatalogPriceSwitchDialog.vue'
  import {
    SOURCE_ITEM_STATUS,
    SOURCE_VERSION_STATUS,
    fetchSourceCatalogCategories,
    fetchGitHubPaidToken,
    fetchSourceCatalogItems,
    fetchSourceCatalogApps,
    rebindSourceCatalogItems,
    parseSourcePackage,
    publishSourcePackage,
    registerSourcePlugin,
    registerSourceTemplate,
    updateSourcePlugin,
    updateSourceTemplate,
    saveSourceCatalogCategories,
    pullSourcePlugin,
    pullSourceTemplate,
    setSourcePluginStatus,
    setSourceTemplateStatus,
    fetchSourcePluginVersions,
    fetchSourceTemplateVersions,
    registerSourcePluginVersion,
    registerSourceTemplateVersion,
    setSourcePluginVersionStatus,
    setSourceTemplateVersionStatus,
    type SourceCatalogCategory,
    type SourceCatalogItem,
    type SourceCatalogApp,
    type SourcePackageManifest,
    type SourceVersion
  } from '@/api/source-station'
  import {
    canDeleteCatalogCategory,
    catalogCategoryUsageCount,
    deleteCatalogCategoryConfirmMessage,
    extrasAfterDeletingCategory
  } from '@/utils/form/catalog-category'
  import {
    catalogPriceSwitchAction,
    catalogUploadBlockReason
  } from '@/utils/form/catalog-package-source'
  import {
    formatCatalogPriceLabel,
    formatCatalogPriceYuan,
    isHttpsLocation,
    isPaidHttpsImportLocation,
    isPrivatePackageLocation,
    isStationPackageLocation,
    parseCatalogPriceYuan,
    resolveCatalogPriceCents
  } from '@/utils/form/catalog-slug'

  const props = withDefaults(
    defineProps<{
      initialCategory?: string
    }>(),
    { initialCategory: '' }
  )

  const route = useRoute()
  const CATEGORY_APP_STORAGE_KEY = 'source-station-catalog-app-id'
  const categories = ref<SourceCatalogCategory[]>([])
  const apps = ref<SourceCatalogApp[]>([])
  const loading = ref(false)
  const pullingId = ref('')
  const tableData = ref<SourceCatalogItem[]>([])
  const selectedRows = ref<SourceCatalogItem[]>([])
  const rebindVisible = ref(false)
  const rebindSaving = ref(false)
  const rebindAppId = ref<number>()
  const rebindItems = ref<SourceCatalogItem[]>([])
  const searchForm = reactive({
    appId: 0,
    status: '',
    category: String(route.query.category || props.initialCategory || '')
  })
  const liveApps = computed(() => apps.value.filter((app) => !app.archived))
  const registerBlockReason = computed(() => {
    if (searchForm.appId <= 0) return '请先选择应用'
    const current = apps.value.find((item) => item.id === searchForm.appId)
    if (current?.archived) return '已归档的应用不能再登记新条目'
    return ''
  })

  const paidRepoReminder = ref('')
  const uploadVisible = ref(false)
  const parsing = ref(false)
  const publishing = ref(false)
  const uploadFile = ref<File | null>(null)
  const parsedManifest = ref<SourcePackageManifest | null>(null)
  const uploadForm = reactive({
    appId: 0,
    category: '',
    changelog: '',
    location: '',
    priceYuan: '0',
    push: false,
    source: 'public' as 'upload' | 'public'
  })
  const uploadBlockReason = computed(() =>
    catalogUploadBlockReason({
      appId: uploadForm.appId,
      source: uploadForm.source,
      push: uploadForm.push,
      hasFile: !!uploadFile.value,
      location: uploadForm.location,
      priceYuan: uploadForm.priceYuan
    })
  )

  const registerVisible = ref(false)
  const registering = ref(false)
  const registerRef = ref<FormInstance>()
  const registerForm = reactive({
    appId: 0,
    id: '',
    name: '',
    version: '1.0.0',
    description: '',
    location: '',
    sha256: '',
    priceYuan: '0',
    authorName: '',
    changelog: '',
    category: 'other',
    shelf: false,
    party: 'official' as 'official' | 'third',
    commercialIncluded: false
  })
  let listingSyncLock = false

  function priceYuanPositive(value: string) {
    const cents = Number(String(value || '').trim())
    return Number.isFinite(cents) && cents > 0
  }

  function listingLabel(row: SourceCatalogItem) {
    const party = row.party === 'third' ? '第三方' : '官方'
    if ((row.priceCents || 0) <= 0) return party
    if (row.party !== 'third' && row.commercialIncluded) return `${party} · 商业版免费`
    return `${party} · 商业版不包含`
  }

  watch(
    () => registerForm.party,
    (party) => {
      if (listingSyncLock) return
      if (party !== 'official') registerForm.commercialIncluded = false
      else if (priceYuanPositive(registerForm.priceYuan)) registerForm.commercialIncluded = true
    }
  )
  watch(
    () => registerForm.priceYuan,
    (value, oldValue) => {
      if (listingSyncLock || registerForm.party !== 'official') return
      if (priceYuanPositive(value) && !priceYuanPositive(oldValue || '')) {
        registerForm.commercialIncluded = true
      }
    }
  )

  const editVisible = ref(false)
  const editing = ref(false)
  const priceSwitchVisible = ref(false)
  const pendingPriceSwitch = ref<'grandfather' | 'purchase_only' | ''>('')
  const editRef = ref<FormInstance>()
  const editingItem = ref<SourceCatalogItem | null>(null)
  function hostedCatalogItem(row: SourceCatalogItem | null) {
    if (!row) return false
    if (row.packageSource === 'github') return true
    const raw = `${row.downloadUrl || ''} ${row.templateUrl || ''}`
    return (
      raw.includes('github:') ||
      isPrivatePackageLocation(row.downloadUrl || '') ||
      isPrivatePackageLocation(row.templateUrl || '')
    )
  }
  const editHostedPackage = computed(() => hostedCatalogItem(editingItem.value))
  const editForm = reactive({
    id: '',
    name: '',
    description: '',
    location: '',
    sha256: '',
    priceYuan: '0',
    authorName: '',
    changelog: '',
    category: '',
    icon: '',
    note: '',
    party: 'official' as 'official' | 'third',
    commercialIncluded: false
  })
  watch(
    () => editForm.party,
    (party) => {
      if (listingSyncLock) return
      if (party !== 'official') editForm.commercialIncluded = false
    }
  )
  watch(
    () => editForm.priceYuan,
    (value, oldValue) => {
      if (listingSyncLock || editForm.party !== 'official') return
      if (priceYuanPositive(value) && !priceYuanPositive(oldValue || '')) {
        editForm.commercialIncluded = true
      }
    }
  )
  const editRules: FormRules = {
    category: [{ required: true, message: '请选择分类', trigger: 'change' }],
    name: [{ required: true, message: '请填写名称', trigger: 'blur' }],
    location: [
      {
        validator: (_rule: unknown, value: string, callback: (error?: Error) => void) => {
          const raw = String(value || '').trim()
          if (!raw || raw.startsWith('私有仓库 ')) {
            if (editHostedPackage.value) {
              callback()
              return
            }
            callback(new Error('请填写外部地址'))
            return
          }
          if (!isHttpsLocation(raw)) {
            callback(new Error('外部地址须以 https:// 开头'))
            return
          }
          callback()
        },
        trigger: 'blur'
      }
    ],
    sha256: [
      optionalPaidShaRule(
        () => editForm.priceYuan,
        () => editForm.location
      )
    ]
  }

  const registerRules: FormRules = {
    appId: [{ required: true, type: 'number', min: 1, message: '请选择应用', trigger: 'change' }],
    category: [{ required: true, message: '请选择分类', trigger: 'change' }],
    id: [{ required: true, message: '请填写标识', trigger: 'blur' }],
    name: [{ required: true, message: '请填写名称', trigger: 'blur' }],
    version: [{ required: true, message: '请填写版本', trigger: 'blur' }],
    location: [{ required: true, message: '请填写外部地址', trigger: 'blur' }],
    sha256: [
      optionalPaidShaRule(
        () => registerForm.priceYuan,
        () => registerForm.location
      )
    ]
  }

  function optionalPaidShaRule(yuan: () => string, location: () => string) {
    return {
      validator: (_: unknown, value: string, callback: (error?: Error) => void) => {
        const raw = String(value || '').trim()
        const cents = parseCatalogPriceYuan(yuan())
        if (!raw && cents !== null && isPaidHttpsImportLocation(location(), cents)) {
          callback()
          return
        }
        if (!/^[a-fA-F0-9]{64}$/.test(raw)) {
          callback(new Error(raw ? '校验码须为 64 位十六进制' : '请填写校验码'))
          return
        }
        callback()
      },
      trigger: 'blur' as const
    }
  }

  const categoryVisible = ref(false)
  const savingCategories = ref(false)
  const extraForm = reactive({ key: '', label: '', kind: 'plugin' as 'plugin' | 'template' })

  const hintExpanded = ref(false)
  const versionVisible = ref(false)
  const versionLoading = ref(false)
  const versionSaving = ref(false)
  const versionFormVisible = ref(false)
  const versions = ref<SourceVersion[]>([])
  const currentItem = ref<SourceCatalogItem | null>(null)
  const versionForm = reactive({ version: '', location: '', sha256: '', changelog: '' })
  const versionStagingId = ref('')
  const versionProbing = ref(false)
  const versionDigest = ref('')

  const registerIsTemplate = computed(() => categoryKind(registerForm.category) === 'template')
  const editIsTemplate = computed(
    () => editingItem.value?.kind === 'template' || categoryKind(editForm.category) === 'template'
  )
  const editCategoryOptions = computed(() => {
    const kind = editingItem.value?.kind || categoryKind(editForm.category)
    return categories.value.filter((item) => item.kind === kind)
  })
  const editApp = computed(
    () => apps.value.find((item) => item.id === editingItem.value?.appId) || apps.value[0]
  )
  const currentIsTemplate = computed(() => currentItem.value?.kind === 'template')
  const uploadCategoryOptions = computed(() => {
    const kind = parsedManifest.value?.kind
    if (!kind) return categories.value
    return categories.value.filter((item) => item.kind === kind)
  })
  function appLabel(app: SourceCatalogApp) {
    const base = `${app.name}（${app.appKey}）`
    if (app.archived) return `${base}· 已归档`
    return app.enabled ? base : `${base}· 已停用`
  }

  function categoryKind(key: string): 'plugin' | 'template' {
    return categories.value.find((item) => item.key === key)?.kind || 'plugin'
  }

  function categoryLabel(key?: string) {
    if (!key) return '-'
    return categories.value.find((item) => item.key === key)?.label || key
  }

  function itemLocation(row: SourceCatalogItem) {
    if (row.packageSource === 'github' && row.githubOwner && row.githubRepo) {
      return `私有仓库 ${row.githubOwner}/${row.githubRepo} @ ${row.githubTag || '-'} / ${row.githubAsset || '-'}`
    }
    return row.location || row.downloadUrl || row.templateUrl || ''
  }

  function isTemplateItem(row: SourceCatalogItem) {
    return row.kind === 'template' || categoryKind(row.category) === 'template'
  }

  function showSetLatest(row: SourceVersion) {
    return (
      canVersionAction(row.status, 'latest') && row.version !== currentItem.value?.latestVersion
    )
  }

  function formatPackageSize(bytes?: number) {
    const size = Number(bytes || 0)
    if (!Number.isFinite(size) || size <= 0) return '未记录'
    if (size < 1024) return `${size} B`
    if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
    return `${(size / 1024 / 1024).toFixed(2)} MB`
  }

  function canVersionAction(
    status: string,
    action: 'approve' | 'reject' | 'latest' | 'deprecate'
  ): boolean {
    switch (action) {
      case 'approve':
        return status === 'pending' || status === 'draft'
      case 'reject':
        return status === 'pending'
      case 'latest':
        return status === 'published'
      case 'deprecate':
        return status === 'published'
      default:
        return false
    }
  }

  const narrow = useNarrowScreen()

  function catalogPrimary(row: SourceCatalogItem): RowActionItem[] {
    const items: RowActionItem[] = []
    if (canEditItem(row.status)) items.push({ key: 'edit', label: '编辑' })
    if (!narrow.value) items.push({ key: 'versions', label: '版本' })
    return items
  }

  function catalogMore(row: SourceCatalogItem): RowActionItem[] {
    const items: RowActionItem[] = [
      ...(narrow.value ? [{ key: 'versions', label: '版本' }] : []),
      { key: 'rebind', label: '切换应用' }
    ]
    if (row.originUrl) {
      items.push({
        key: 'pull',
        label: '重新拉取',
        disabled: pullingId.value === row.id
      })
    }
    if (canAction(row.status, 'approve')) items.push({ key: 'approve', label: '通过' })
    if (canAction(row.status, 'reject')) items.push({ key: 'reject', label: '拒绝' })
    if (canAction(row.status, 'shelf')) items.push({ key: 'shelf', label: '上架' })
    if (canAction(row.status, 'unshelf')) items.push({ key: 'unshelf', label: '下架' })
    if (canAction(row.status, 'deprecate'))
      items.push({ key: 'deprecate', label: '弃用', danger: true })
    if (canAction(row.status, 'restore')) items.push({ key: 'restore', label: '恢复为草稿' })
    return items
  }

  function onCatalogAction(row: SourceCatalogItem, action: RowActionItem) {
    switch (action.key) {
      case 'edit':
        openEdit(row)
        break
      case 'versions':
        openVersions(row)
        break
      case 'rebind':
        openRebind([row])
        break
      case 'pull':
        handlePull(row)
        break
      case 'approve':
      case 'reject':
      case 'shelf':
      case 'unshelf':
      case 'deprecate':
      case 'restore':
        runStatus(row, action.key)
        break
    }
  }

  function canEditItem(status: string) {
    return ['draft', 'review', 'approved', 'published', 'hidden'].includes(status)
  }

  function canAction(
    status: string,
    action: 'approve' | 'reject' | 'shelf' | 'unshelf' | 'deprecate' | 'restore'
  ): boolean {
    if (action === 'restore') return status === 'deprecated'
    const target = {
      approve: 'approved',
      reject: 'rejected',
      shelf: 'published',
      unshelf: 'hidden',
      deprecate: 'deprecated'
    } as const
    const to = target[action]
    if (status === to) return false
    switch (to) {
      case 'approved':
      case 'rejected':
        return status === 'review' || status === 'draft'
      case 'published':
        return status === 'approved' || status === 'hidden'
      case 'hidden':
        return status === 'published'
      case 'deprecated':
        return status === 'published' || status === 'hidden' || status === 'approved'
      default:
        return false
    }
  }

  function statusMeta(status: string) {
    return SOURCE_ITEM_STATUS[status] || { label: status, type: 'info' as const }
  }

  function versionStatusMeta(status: string) {
    return SOURCE_VERSION_STATUS[status] || { label: status, type: 'info' as const }
  }

  async function loadCategories() {
    const data = await fetchSourceCatalogCategories()
    categories.value = data.list || []
  }

  async function loadApps() {
    const data = await fetchSourceCatalogApps()
    apps.value = data.list || []
    const stored = Number(localStorage.getItem(CATEGORY_APP_STORAGE_KEY) || 0)
    if (stored === -1) {
      searchForm.appId = -1
      return
    }
    const exists = apps.value.some((item) => item.id === stored)
    if (exists) {
      searchForm.appId = stored
      return
    }
    searchForm.appId = apps.value[0]?.id || 0
    if (searchForm.appId) {
      localStorage.setItem(CATEGORY_APP_STORAGE_KEY, String(searchForm.appId))
    }
  }

  function onAppChange() {
    if (searchForm.appId) {
      localStorage.setItem(CATEGORY_APP_STORAGE_KEY, String(searchForm.appId))
    }
    loadItems()
  }

  async function handlePull(row: SourceCatalogItem) {
    if (!row.originUrl || pullingId.value) return
    pullingId.value = row.id
    try {
      if (isTemplateItem(row)) await pullSourceTemplate(row.id)
      else await pullSourcePlugin(row.id)
      ElMessage.success('已重新拉取并更新托管包')
      await loadItems()
    } finally {
      pullingId.value = ''
    }
  }

  function onSelectionChange(rows: SourceCatalogItem[]) {
    selectedRows.value = rows
  }

  function openRebind(rows: SourceCatalogItem[]) {
    if (!rows.length) {
      ElMessage.info('请选择要切换的条目')
      return
    }
    rebindItems.value = rows
    rebindAppId.value =
      liveApps.value.find((app) => app.id !== rows[0]?.appId)?.id || liveApps.value[0]?.id
    rebindVisible.value = true
  }

  async function handleRebind() {
    if (!rebindAppId.value) {
      ElMessage.info('请选择目标应用')
      return
    }
    rebindSaving.value = true
    try {
      await rebindSourceCatalogItems(
        rebindAppId.value,
        rebindItems.value.map((row) => ({ kind: row.kind, id: row.id }))
      )
      ElMessage.success('已切换绑定应用')
      rebindVisible.value = false
      await loadItems()
    } finally {
      rebindSaving.value = false
    }
  }

  async function loadItems() {
    if (searchForm.appId === 0) {
      tableData.value = []
      return
    }
    loading.value = true
    try {
      const data = await fetchSourceCatalogItems(
        searchForm.status,
        searchForm.category,
        searchForm.appId,
        searchForm.appId === -1
      )
      tableData.value = data.list || []
    } finally {
      loading.value = false
    }
  }

  function resetSearch() {
    searchForm.status = ''
    searchForm.category = ''
    loadItems()
  }

  function formatVersionTime(value?: string) {
    if (!value) return '未记录'
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return '未记录'
    const pad = (part: number) => String(part).padStart(2, '0')
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
  }

  function openVersionUpload() {
    openUpload()
  }

  function openUpload() {
    if (registerBlockReason.value) {
      ElMessage.info(registerBlockReason.value)
      return
    }
    uploadFile.value = null
    parsedManifest.value = null
    uploadForm.appId = searchForm.appId
    uploadForm.category = searchForm.category
    uploadForm.changelog = ''
    uploadForm.location = ''
    uploadForm.priceYuan = '0'
    uploadForm.push = false
    uploadForm.source = 'public'
    uploadVisible.value = true
  }

  function selectUploadFile(file: UploadFile) {
    uploadFile.value = file.raw || null
    parsedManifest.value = null
  }

  function buildPackageForm() {
    const form = new FormData()
    form.append('packageSource', uploadForm.source)
    if (uploadForm.source === 'upload' && uploadFile.value) form.append('file', uploadFile.value)
    if (uploadForm.category) form.append('category', uploadForm.category)
    if (uploadForm.changelog) form.append('changelog', uploadForm.changelog)
    if (uploadForm.source !== 'upload' && uploadForm.location.trim()) {
      const kind = parsedManifest.value?.kind || categoryKind(uploadForm.category)
      form.append(kind === 'template' ? 'templateUrl' : 'downloadUrl', uploadForm.location.trim())
    }
    const priced = resolveCatalogPriceCents(
      uploadForm.priceYuan,
      uploadForm.source === 'upload' ? '' : uploadForm.location
    )
    if (!priced.error && priced.cents > 0) form.append('priceCents', String(priced.cents))
    if (uploadForm.source === 'upload' && uploadForm.push) form.append('push', 'true')
    if (uploadForm.appId) form.append('appId', String(uploadForm.appId))
    return form
  }

  async function handleParse() {
    if (uploadBlockReason.value) {
      ElMessage.info(uploadBlockReason.value)
      return
    }
    parsing.value = true
    try {
      const form = new FormData()
      form.append('packageSource', uploadForm.source)
      if (uploadForm.source === 'upload' && uploadFile.value) form.append('file', uploadFile.value)
      else if (uploadForm.location.trim()) {
        const kind = categoryKind(uploadForm.category)
        form.append(kind === 'template' ? 'templateUrl' : 'downloadUrl', uploadForm.location.trim())
      }
      const priced = resolveCatalogPriceCents(uploadForm.priceYuan, uploadForm.location)
      if (!priced.error && priced.cents > 0) form.append('priceCents', String(priced.cents))
      if (uploadForm.category) form.append('category', uploadForm.category)
      parsedManifest.value = await parseSourcePackage(form)
      if (parsedManifest.value.category) {
        uploadForm.category = parsedManifest.value.category
      }
      ElMessage.success('已通过校验（包未落盘、未入库）')
    } finally {
      parsing.value = false
    }
  }

  async function handlePublish() {
    if (uploadBlockReason.value) {
      ElMessage.info(uploadBlockReason.value)
      return
    }
    const priced = resolveCatalogPriceCents(
      uploadForm.priceYuan,
      uploadForm.source === 'upload' ? '' : uploadForm.location
    )
    if (priced.error) {
      ElMessage.info(priced.error)
      return
    }
    publishing.value = true
    try {
      const result = await publishSourcePackage(buildPackageForm())
      const origin = (result.item as { originUrl?: string } | undefined)?.originUrl
      ElMessage.success(
        result.pushed
          ? '校验通过，已推送 Release 并保存元数据'
          : origin
            ? '校验通过，已拉取安装包并保存，校验码已自动填写'
            : uploadForm.source === 'upload'
              ? '校验通过，已保存元数据'
              : '校验通过，已保存元数据并自动填写校验码'
      )
      uploadVisible.value = false
      await loadItems()
    } finally {
      publishing.value = false
    }
  }

  function openEdit(row: SourceCatalogItem) {
    editingItem.value = row
    listingSyncLock = true
    editForm.id = row.id
    editForm.name = row.name
    editForm.description = row.description || ''
    editForm.location = hostedCatalogItem(row) ? '' : itemLocation(row)
    editForm.sha256 = row.sha256 || ''
    editForm.priceYuan = formatCatalogPriceYuan(row.priceCents)
    editForm.authorName = row.author?.name || ''
    editForm.changelog = row.changelog || ''
    editForm.category = row.category
    editForm.icon = row.icon || ''
    editForm.note = ''
    editForm.party = row.party === 'third' ? 'third' : 'official'
    editForm.commercialIncluded = editForm.party === 'official' && !!row.commercialIncluded
    editVisible.value = true
    void nextTick(() => {
      listingSyncLock = false
    })
  }

  function confirmPriceSwitch(policy: 'grandfather' | 'purchase_only') {
    pendingPriceSwitch.value = policy
    if (policy === 'purchase_only' || editForm.party !== 'official') {
      editForm.commercialIncluded = false
    } else {
      editForm.commercialIncluded = true
    }
    void saveEdit(policy)
  }

  async function handleEdit() {
    if (!editingItem.value) return
    await editRef.value?.validate()
    const priced = resolveCatalogPriceCents(editForm.priceYuan, editForm.location)
    if (priced.error) {
      ElMessage.info(priced.error)
      return
    }
    const action = catalogPriceSwitchAction(
      editingItem.value.priceCents || 0,
      priced.cents,
      editingItem.value.status,
      editingItem.value.latestVersion
    )
    if (action === 'to-paid') {
      pendingPriceSwitch.value = ''
      priceSwitchVisible.value = true
      return
    }
    if (action === 'to-free') {
      try {
        await ElMessageBox.confirm(
          editHostedPackage.value
            ? '改回免费后继续使用收费仓库里的安装包，下载由官网提供。留空即可；填写新的 https 地址则改用外链。'
            : '改回免费后，安装包会重新提供公开下载地址。已经发给老用户的免费权益会保留。',
          '改回免费',
          { confirmButtonText: '确认改回免费', cancelButtonText: '取消', type: 'warning' }
        )
      } catch {
        return
      }
    }
    await saveEdit('')
  }

  function submittedEditLocation() {
    const raw = editForm.location.trim()
    if (editHostedPackage.value && (raw === '' || raw.startsWith('私有仓库 '))) return ''
    return raw
  }

  async function saveEdit(priceSwitch: 'grandfather' | 'purchase_only' | '') {
    if (!editingItem.value) return
    const priced = resolveCatalogPriceCents(editForm.priceYuan, editForm.location)
    if (priced.error) {
      ElMessage.info(priced.error)
      return
    }
    editing.value = true
    try {
      const note = editForm.note.trim() || '管理员编辑目录元数据（保持原状态）'
      const switchField = priceSwitch ? { priceSwitch } : {}
      if (editIsTemplate.value) {
        await updateSourceTemplate(editingItem.value.id, {
          id: editingItem.value.id,
          appId: editingItem.value.appId,
          templateKey: editingItem.value.templateKey || editingItem.value.id,
          name: editForm.name,
          description: editForm.description,
          version: editingItem.value.version || '1.0.0',
          schemaVersion: editingItem.value.schemaVersion || 1,
          templateUrl: submittedEditLocation(),
          sha256: editForm.sha256,
          priceCents: priced.cents,
          changelog: editForm.changelog,
          category: editForm.category,
          author: { name: editForm.authorName },
          note,
          party: editForm.party,
          commercialIncluded: editForm.party === 'official' && editForm.commercialIncluded,
          ...switchField
        })
      } else {
        await updateSourcePlugin(editingItem.value.id, {
          id: editingItem.value.id,
          appId: editingItem.value.appId,
          name: editForm.name,
          description: editForm.description,
          version: editingItem.value.version || '1.0.0',
          downloadUrl: submittedEditLocation(),
          sha256: editForm.sha256,
          priceCents: priced.cents,
          changelog: editForm.changelog,
          category: editForm.category,
          icon: editForm.icon,
          author: { name: editForm.authorName },
          note,
          party: editForm.party,
          commercialIncluded: editForm.party === 'official' && editForm.commercialIncluded,
          ...switchField
        })
      }
      ElMessage.success(
        priceSwitch === 'purchase_only'
          ? '已改为收费，所有人都需购买'
          : priceSwitch === 'grandfather'
            ? '已改为收费，老用户继续免费'
            : '已更新目录元数据'
      )
      editVisible.value = false
      await loadItems()
    } finally {
      editing.value = false
      pendingPriceSwitch.value = ''
    }
  }

  function openRegister() {
    if (registerBlockReason.value) {
      ElMessage.info(registerBlockReason.value)
      return
    }
    registerForm.appId = searchForm.appId
    registerForm.id = ''
    registerForm.name = ''
    registerForm.version = '1.0.0'
    registerForm.description = ''
    registerForm.location = ''
    registerForm.sha256 = ''
    registerForm.priceYuan = '0'
    registerForm.authorName = ''
    registerForm.changelog = ''
    registerForm.category = searchForm.category || 'other'
    registerForm.shelf = false
    registerForm.party = 'official'
    registerForm.commercialIncluded = false
    registerVisible.value = true
  }

  async function handleRegister() {
    await registerRef.value?.validate()
    const priced = resolveCatalogPriceCents(registerForm.priceYuan, registerForm.location)
    if (priced.error) {
      ElMessage.info(priced.error)
      return
    }
    registering.value = true
    try {
      if (registerIsTemplate.value) {
        await registerSourceTemplate({
          id: registerForm.id,
          appId: registerForm.appId,
          templateKey: registerForm.id,
          name: registerForm.name,
          version: registerForm.version,
          description: registerForm.description,
          templateUrl: registerForm.location,
          sha256: registerForm.sha256,
          priceCents: priced.cents,
          changelog: registerForm.changelog,
          schemaVersion: 1,
          category: registerForm.category,
          author: { name: registerForm.authorName },
          shelf: registerForm.shelf,
          party: registerForm.party,
          commercialIncluded: registerForm.party === 'official' && registerForm.commercialIncluded
        })
      } else {
        await registerSourcePlugin({
          id: registerForm.id,
          appId: registerForm.appId,
          name: registerForm.name,
          version: registerForm.version,
          description: registerForm.description,
          downloadUrl: registerForm.location,
          sha256: registerForm.sha256,
          priceCents: priced.cents,
          changelog: registerForm.changelog,
          category: registerForm.category,
          author: { name: registerForm.authorName },
          shelf: registerForm.shelf,
          party: registerForm.party,
          commercialIncluded: registerForm.party === 'official' && registerForm.commercialIncluded
        })
      }
      ElMessage.success('已登记外部地址（未上传源码）')
      registerVisible.value = false
      await loadItems()
    } finally {
      registering.value = false
    }
  }

  function openCategoryManager() {
    extraForm.key = ''
    extraForm.label = ''
    extraForm.kind = 'plugin'
    categoryVisible.value = true
  }

  async function handleAddCategory() {
    const key = extraForm.key.trim().toLowerCase()
    const label = extraForm.label.trim()
    if (!key || !label) {
      ElMessage.info('请填写分类标识和名称')
      return
    }
    savingCategories.value = true
    try {
      const extras = extrasAfterDeletingCategory(categories.value, '')
      extras.push({ key, label, kind: extraForm.kind })
      const data = await saveSourceCatalogCategories(extras)
      categories.value = data.list || []
      extraForm.key = ''
      extraForm.label = ''
      ElMessage.success('已添加分类')
    } finally {
      savingCategories.value = false
    }
  }

  async function handleDeleteCategory(row: SourceCatalogCategory) {
    if (!canDeleteCatalogCategory(row)) return
    let usedCount = catalogCategoryUsageCount(tableData.value, row.key)
    if (usedCount === 0 && searchForm.appId !== 0) {
      const data = await fetchSourceCatalogItems(
        '',
        '',
        searchForm.appId > 0 ? searchForm.appId : undefined,
        searchForm.appId === -1
      )
      usedCount = catalogCategoryUsageCount(data.list || [], row.key)
    }
    try {
      await ElMessageBox.confirm(
        deleteCatalogCategoryConfirmMessage(row.label, usedCount),
        '删除分类',
        { type: 'warning' }
      )
    } catch {
      return
    }
    savingCategories.value = true
    try {
      const extras = extrasAfterDeletingCategory(categories.value, row.key)
      const data = await saveSourceCatalogCategories(extras)
      categories.value = data.list || []
      if (searchForm.category === row.key) {
        searchForm.category = ''
        await loadItems()
      }
      ElMessage.success('已删除分类')
    } finally {
      savingCategories.value = false
    }
  }

  async function runStatus(
    row: SourceCatalogItem,
    action: 'approve' | 'reject' | 'shelf' | 'unshelf' | 'deprecate' | 'restore'
  ) {
    if (action === 'unshelf') {
      await ElMessageBox.confirm(
        '下架后，该应用的公开软件源里不再显示这一条，已经安装的不会被远程卸掉。确认继续？',
        '下架确认',
        { type: 'warning' }
      )
    }
    if (action === 'restore') {
      await ElMessageBox.confirm(
        '恢复后回到草稿，不会自动上架。需要再审核通过才能出现在公开软件源里。确认继续？',
        '恢复为草稿',
        { type: 'warning' }
      )
    }
    let note = ''
    if (action === 'reject' || action === 'deprecate') {
      const { value } = await ElMessageBox.prompt(
        '备注（可选）',
        action === 'reject' ? '驳回' : '弃用',
        {
          inputPlaceholder: '审核说明',
          confirmButtonText: '确定',
          cancelButtonText: '取消'
        }
      )
      note = value || ''
    }
    if (isTemplateItem(row)) {
      await setSourceTemplateStatus(row.id, action, note)
    } else {
      await setSourcePluginStatus(row.id, action, note)
    }
    ElMessage.success('已更新状态')
    await loadItems()
  }

  async function openVersions(row: SourceCatalogItem) {
    currentItem.value = row
    versionVisible.value = true
    await loadVersions()
  }

  async function loadVersions() {
    if (!currentItem.value) return
    versionLoading.value = true
    try {
      const data = isTemplateItem(currentItem.value)
        ? await fetchSourceTemplateVersions(currentItem.value.id)
        : await fetchSourcePluginVersions(currentItem.value.id)
      versions.value = data.list || []
    } finally {
      versionLoading.value = false
    }
  }

  function onCatalogLocationInput(value: string) {
    versionForm.location = value
    versionStagingId.value = ''
    versionDigest.value = ''
  }

  function describeReleaseFile(result: ReleaseImportResult) {
    const size = result.fileSizeBytes > 0 ? `${result.fileSizeBytes} 字节` : ''
    const md5 = result.fileMd5 ? `MD5 ${result.fileMd5}` : ''
    return [size, md5].filter(Boolean).join('，')
  }

  function applyVersionImport(result: ReleaseImportResult) {
    versionStagingId.value = result.stagingId
    if (result.version) versionForm.version = result.version
    if (result.changelog) versionForm.changelog = result.changelog
    if (result.fileSha256) versionForm.sha256 = result.fileSha256
    versionForm.location = ''
    versionDigest.value = describeReleaseFile(result)
  }

  async function probeCatalogVersionUrl() {
    const location = versionForm.location.trim()
    if (!/^https:\/\//i.test(location)) {
      ElMessage.info('请先填写 https 下载地址')
      return
    }
    versionProbing.value = true
    try {
      const result = await probeReleaseUrl('/api/v1/source/admin/release-import', location)
      versionStagingId.value = result.stagingId
      if (result.fileSha256) versionForm.sha256 = result.fileSha256
      versionDigest.value = describeReleaseFile(result)
      ElMessage.success('已填入大小和校验值，保存前仍可修改')
    } finally {
      versionProbing.value = false
    }
  }

  async function handleAddVersion() {
    if (!currentItem.value) return
    const cents = currentItem.value.priceCents || 0
    const location = versionForm.location.trim()
    if (!versionForm.version.trim() || (!location && !versionStagingId.value)) {
      ElMessage.info('请填写版本和地址')
      return
    }
    if (
      !versionStagingId.value &&
      cents > 0 &&
      !isStationPackageLocation(location) &&
      !isPrivatePackageLocation(location) &&
      !isHttpsLocation(location)
    ) {
      ElMessage.info('付费条目请上传压缩包，或填写 https 网址由本站拉取托管')
      return
    }
    if (
      !versionStagingId.value &&
      !isPaidHttpsImportLocation(location, cents) &&
      !/^[a-fA-F0-9]{64}$/.test(versionForm.sha256.trim())
    ) {
      ElMessage.info('请填写 64 位校验码')
      return
    }
    versionSaving.value = true
    try {
      let location = versionForm.location.trim()
      let sha256 = versionForm.sha256.trim()
      if (versionStagingId.value) {
        const stored = await materializeRelease('/api/v1/source/admin/release-import', {
          stagingId: versionStagingId.value,
          kind: isTemplateItem(currentItem.value) ? 'template' : 'plugin',
          itemId: currentItem.value.id,
          version: versionForm.version.trim(),
          priceCents: cents
        })
        location = stored.location
        if (!sha256) sha256 = stored.sha256
        versionForm.location = location
        versionForm.sha256 = sha256
      }
      const payload = {
        version: versionForm.version,
        sha256,
        changelog: versionForm.changelog,
        ...(isTemplateItem(currentItem.value)
          ? { templateUrl: location }
          : { downloadUrl: location })
      }
      if (isTemplateItem(currentItem.value)) {
        await registerSourceTemplateVersion(currentItem.value.id, payload)
      } else {
        await registerSourcePluginVersion(currentItem.value.id, payload)
      }
      ElMessage.success('已登记外部版本地址')
      versionFormVisible.value = false
      versionForm.version = ''
      versionForm.location = ''
      versionForm.sha256 = ''
      versionForm.changelog = ''
      versionStagingId.value = ''
      versionDigest.value = ''
      await loadVersions()
      await loadItems()
    } finally {
      versionSaving.value = false
    }
  }

  async function runVersion(
    row: SourceVersion,
    action: 'approve' | 'reject' | 'deprecate' | 'latest'
  ) {
    if (!currentItem.value) return
    if (isTemplateItem(currentItem.value)) {
      await setSourceTemplateVersionStatus(currentItem.value.id, row.version, action)
    } else {
      await setSourcePluginVersionStatus(currentItem.value.id, row.version, action)
    }
    ElMessage.success('已更新版本')
    await loadVersions()
    await loadItems()
  }

  async function loadPaidRepoReminder() {
    try {
      const data = await fetchGitHubPaidToken()
      paidRepoReminder.value = data.configured ? '' : data.reminder || ''
    } catch {
      paidRepoReminder.value = ''
    }
  }

  onMounted(async () => {
    await loadCategories()
    await loadApps()
    await loadPaidRepoReminder()
    await loadItems()
  })
</script>

<style scoped lang="scss">
  .source-station-page {
    padding-bottom: 8px;

    :deep(.el-card) {
      --el-card-border-color: var(--art-card-border);
      border-radius: calc(var(--custom-radius) + 4px);
      background: var(--default-box-color);
      box-shadow: none;
    }

    :deep(.el-card__header) {
      padding: 20px 22px 14px;
      border-bottom-color: var(--art-card-border);
    }

    :deep(.el-card__body) {
      padding: 20px 22px;
    }
  }

  .mb-4 {
    margin-bottom: 16px;
  }

  .mb-3 {
    margin-bottom: 12px;
  }

  .table-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 12px;
  }

  .table-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .hint-toggle {
    margin-top: 2px;
    padding: 0;
    height: auto;
  }

  .card-hint.is-collapsed {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 1;
    overflow: hidden;
  }

  .version-cards {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .version-card {
    padding: 12px;
    border: 1px solid var(--art-card-border);
    border-radius: 8px;
    background: var(--default-box-color);
  }

  .version-card p {
    margin: 6px 0 0;
    font-size: 13px;
    line-height: 1.5;
    color: var(--art-gray-700);
    word-break: break-word;
  }

  .version-card-head {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
  }

  .version-card-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 10px;
  }

  @media (max-width: 767px) {
    .table-header {
      flex-direction: column;
    }

    .table-actions {
      width: 100%;
    }

    .table-actions :deep(.el-button) {
      flex: 1 1 140px;
      height: auto;
      margin-left: 0;
      padding: 8px 10px;
      white-space: normal;
      line-height: 1.35;
    }
  }

  .card-title {
    font-size: 16px;
    font-weight: 700;
    color: var(--art-gray-900);
  }

  .card-hint {
    margin: 6px 0 0;
    font-size: 13px;
    color: var(--art-gray-600);
    line-height: 1.5;
  }

  .url-probe {
    display: flex;
    gap: 8px;
    width: 100%;
  }

  .url-probe .el-input {
    flex: 1;
  }

  .filter-panel :deep(.el-form) {
    margin-bottom: -18px;
  }
</style>
