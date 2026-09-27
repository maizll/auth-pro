<!-- 官网页面：导航、文档、更新日志、两边都有的能力。差别行和套餐价格不在这里改。 -->
<template>
  <div class="site-pages">
    <p class="price-hint">
      授权价格在 <RouterLink to="/license/plans">套餐设置</RouterLink> 里修改。
    </p>
    <ElCard shadow="never">
      <ElTabs v-model="tab">
        <ElTabPane label="导航" name="nav">
          <div class="toolbar">
            <ElButton type="primary" @click="openNav()">添加外链</ElButton>
          </div>
          <ElTable :data="navRows" class="fit-table">
            <ElTableColumn prop="label" label="名称" min-width="96" show-overflow-tooltip />
            <ElTableColumn
              label="类型"
              width="90"
              class-name="mobile-col-hidden"
              label-class-name="mobile-col-hidden"
            >
              <template #default="{ row }">{{ row.kind === 'builtin' ? '内置' : '外链' }}</template>
            </ElTableColumn>
            <ElTableColumn
              prop="href"
              label="地址"
              min-width="140"
              show-overflow-tooltip
              class-name="mobile-col-hidden"
              label-class-name="mobile-col-hidden"
            />
            <ElTableColumn label="开关" width="72">
              <template #default="{ row }">
                <ElSwitch :model-value="row.enabled" @change="onNavSwitch(row, $event)" />
              </template>
            </ElTableColumn>
            <ElTableColumn label="操作" width="108">
              <template #default="{ row }">
                <RowActions
                  :primary="[{ key: 'edit', label: '编辑' }]"
                  :more="navMore(row)"
                  @click="onNavAction($event, row)"
                />
              </template>
            </ElTableColumn>
          </ElTable>
        </ElTabPane>

        <ElTabPane label="系统文档" name="docs">
          <div class="toolbar">
            <ElButton type="primary" @click="openCategory()">添加分类</ElButton>
            <ElButton @click="openDoc()">添加文章</ElButton>
          </div>
          <h3>分类</h3>
          <ElTable :data="categories" class="fit-table">
            <ElTableColumn prop="name" label="名称" min-width="96" show-overflow-tooltip />
            <ElTableColumn
              prop="slug"
              label="标识"
              min-width="100"
              show-overflow-tooltip
              class-name="mobile-col-hidden"
              label-class-name="mobile-col-hidden"
            />
            <ElTableColumn label="开关" width="72">
              <template #default="{ row }">
                <ElSwitch :model-value="!row.hidden" @change="onCategorySwitch(row, $event)" />
              </template>
            </ElTableColumn>
            <ElTableColumn label="操作" width="108">
              <template #default="{ row }">
                <RowActions
                  :primary="[{ key: 'edit', label: '编辑' }]"
                  :more="[
                    { key: 'up', label: '上移' },
                    { key: 'down', label: '下移' },
                    { key: 'toggle', label: row.hidden ? '显示' : '隐藏' },
                    { key: 'delete', label: '删除', danger: true }
                  ]"
                  @click="onCategoryAction($event, row)"
                />
              </template>
            </ElTableColumn>
          </ElTable>
          <h3>文章</h3>
          <ElTable :data="docs" class="fit-table">
            <ElTableColumn prop="title" label="名称" min-width="96" show-overflow-tooltip />
            <ElTableColumn
              prop="category"
              label="分类"
              width="110"
              show-overflow-tooltip
              class-name="mobile-col-hidden"
              label-class-name="mobile-col-hidden"
            />
            <ElTableColumn label="开关" width="72">
              <template #default="{ row }">
                <ElSwitch :model-value="!row.hidden" @change="onDocSwitch(row, $event)" />
              </template>
            </ElTableColumn>
            <ElTableColumn label="操作" width="108">
              <template #default="{ row }">
                <RowActions
                  :primary="[{ key: 'edit', label: '编辑' }]"
                  :more="[
                    { key: 'up', label: '上移' },
                    { key: 'down', label: '下移' },
                    { key: 'toggle', label: row.hidden ? '显示' : '隐藏' },
                    { key: 'delete', label: '删除', danger: true }
                  ]"
                  @click="onDocAction($event, row)"
                />
              </template>
            </ElTableColumn>
          </ElTable>
        </ElTabPane>

        <ElTabPane label="更新日志" name="changelog">
          <div class="toolbar">
            <ElButton type="primary" @click="openLog()">添加条目</ElButton>
          </div>
          <ElTable :data="logs" class="fit-table">
            <ElTableColumn prop="version" label="名称" width="90" show-overflow-tooltip />
            <ElTableColumn
              prop="releasedOn"
              label="日期"
              width="110"
              show-overflow-tooltip
              class-name="mobile-col-hidden"
              label-class-name="mobile-col-hidden"
            />
            <ElTableColumn
              label="类型"
              width="80"
              class-name="mobile-col-hidden"
              label-class-name="mobile-col-hidden"
            >
              <template #default="{ row }">{{ tagLabel(row.tag) }}</template>
            </ElTableColumn>
            <ElTableColumn
              prop="body"
              label="内容"
              min-width="160"
              show-overflow-tooltip
              class-name="mobile-col-hidden"
              label-class-name="mobile-col-hidden"
            />
            <ElTableColumn label="开关" width="72">
              <template #default="{ row }">
                <ElSwitch :model-value="!row.hidden" @change="onLogSwitch(row, $event)" />
              </template>
            </ElTableColumn>
            <ElTableColumn label="操作" width="108">
              <template #default="{ row }">
                <RowActions
                  :primary="[{ key: 'edit', label: '编辑' }]"
                  :more="[
                    { key: 'up', label: '上移' },
                    { key: 'down', label: '下移' },
                    { key: 'toggle', label: row.hidden ? '显示' : '隐藏' },
                    { key: 'delete', label: '删除', danger: true }
                  ]"
                  @click="onLogAction($event, row)"
                />
              </template>
            </ElTableColumn>
          </ElTable>
        </ElTabPane>

        <ElTabPane label="系统对比" name="compare">
          <p class="hint">免费版和商业版的差别行跟随系统规则，不能在这里修改。</p>
          <div class="toolbar">
            <ElButton type="primary" @click="openShared()">添加能力</ElButton>
          </div>
          <ElTable :data="shared" class="fit-table">
            <ElTableColumn prop="body" label="名称" min-width="96" show-overflow-tooltip />
            <ElTableColumn label="开关" width="72">
              <template #default="{ row }">
                <ElSwitch :model-value="!row.hidden" @change="onSharedSwitch(row, $event)" />
              </template>
            </ElTableColumn>
            <ElTableColumn label="操作" width="108">
              <template #default="{ row }">
                <RowActions
                  :primary="[{ key: 'edit', label: '编辑' }]"
                  :more="[
                    { key: 'up', label: '上移' },
                    { key: 'down', label: '下移' },
                    { key: 'toggle', label: row.hidden ? '显示' : '隐藏' },
                    { key: 'delete', label: '删除', danger: true }
                  ]"
                  @click="onSharedAction($event, row)"
                />
              </template>
            </ElTableColumn>
          </ElTable>
        </ElTabPane>
      </ElTabs>
    </ElCard>

    <ElDialog v-model="navVisible" title="导航" width="min(480px, calc(100vw - 24px))">
      <ElForm label-position="top">
        <ElFormItem label="名称"><ElInput v-model="navForm.label" maxlength="40" /></ElFormItem>
        <ElFormItem v-if="navForm.kind !== 'builtin'" label="链接">
          <ElInput v-model="navForm.href" placeholder="https://" />
        </ElFormItem>
        <ElFormItem label="显示"><ElSwitch v-model="navForm.enabled" /></ElFormItem>
      </ElForm>
      <template #footer>
        <ElButton @click="navVisible = false">取消</ElButton>
        <ElButton type="primary" @click="saveNav">保存</ElButton>
      </template>
    </ElDialog>

    <ElDialog v-model="categoryVisible" title="文档分类" width="min(480px, calc(100vw - 24px))">
      <ElForm label-position="top">
        <ElFormItem label="名称"><ElInput v-model="categoryForm.name" maxlength="40" /></ElFormItem>
        <ElFormItem v-if="!categoryForm.id" label="英文标识">
          <ElInput v-model="categoryForm.slug" maxlength="80" />
        </ElFormItem>
      </ElForm>
      <template #footer>
        <ElButton @click="categoryVisible = false">取消</ElButton>
        <ElButton type="primary" @click="saveCategory">保存</ElButton>
      </template>
    </ElDialog>

    <ElDialog v-model="docVisible" title="文档" width="min(720px, calc(100vw - 24px))">
      <ElForm label-position="top">
        <ElFormItem label="分类">
          <ElSelect v-model="docForm.categoryId" style="width: 100%">
            <ElOption
              v-for="item in categories"
              :key="item.id"
              :label="item.name"
              :value="item.id"
            />
          </ElSelect>
        </ElFormItem>
        <ElFormItem label="标题"><ElInput v-model="docForm.title" maxlength="120" /></ElFormItem>
        <ElFormItem label="英文标识"><ElInput v-model="docForm.slug" maxlength="80" /></ElFormItem>
        <ElFormItem label="摘要"><ElInput v-model="docForm.summary" maxlength="300" /></ElFormItem>
        <ElFormItem label="Markdown">
          <ElInput v-model="docForm.body" type="textarea" :rows="12" />
        </ElFormItem>
        <ElFormItem v-if="docForm.body.trim()" label="预览">
          <SiteMarkdown :source="docForm.body" :page-title="docForm.title" />
        </ElFormItem>
        <ElFormItem label="隐藏"><ElSwitch v-model="docForm.hidden" /></ElFormItem>
      </ElForm>
      <template #footer>
        <ElButton @click="docVisible = false">取消</ElButton>
        <ElButton type="primary" @click="saveDoc">保存</ElButton>
      </template>
    </ElDialog>

    <ElDialog v-model="logVisible" title="更新日志" width="min(640px, calc(100vw - 24px))">
      <ElForm label-position="top">
        <ElFormItem label="版本号"
          ><ElInput v-model="logForm.version" placeholder="1.7.0"
        /></ElFormItem>
        <ElFormItem label="发布日期（北京时间）">
          <ElInput v-model="logForm.releasedOn" placeholder="YYYY-MM-DD" />
        </ElFormItem>
        <ElFormItem label="类型">
          <ElSelect v-model="logForm.tag" style="width: 100%">
            <ElOption label="新增" value="added" />
            <ElOption label="优化" value="improved" />
            <ElOption label="修复" value="fixed" />
          </ElSelect>
        </ElFormItem>
        <ElFormItem label="内容"
          ><ElInput v-model="logForm.body" type="textarea" :rows="5"
        /></ElFormItem>
        <ElFormItem label="隐藏"><ElSwitch v-model="logForm.hidden" /></ElFormItem>
      </ElForm>
      <template #footer>
        <ElButton @click="logVisible = false">取消</ElButton>
        <ElButton type="primary" @click="saveLog">保存</ElButton>
      </template>
    </ElDialog>

    <ElDialog v-model="sharedVisible" title="两个版本都具备" width="min(520px, calc(100vw - 24px))">
      <ElInput v-model="sharedForm.body" maxlength="200" />
      <template #footer>
        <ElButton @click="sharedVisible = false">取消</ElButton>
        <ElButton type="primary" @click="saveShared">保存</ElButton>
      </template>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, reactive, ref } from 'vue'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import RowActions, { type RowActionItem } from '@/components/business/row-actions/index.vue'
  import SiteMarkdown from '@/components/site/SiteMarkdown.vue'
  import {
    createAdminNav,
    deleteAdminChangelog,
    deleteAdminCompare,
    deleteAdminDoc,
    deleteAdminDocCategory,
    deleteAdminNav,
    fetchAdminChangelog,
    fetchAdminCompare,
    fetchAdminDocCategories,
    fetchAdminDocs,
    fetchAdminNav,
    saveAdminChangelog,
    saveAdminCompare,
    saveAdminDoc,
    saveAdminDocCategory,
    updateAdminNav,
    type SiteChangelogRow,
    type SiteCompareRow,
    type SiteDocCategory,
    type SiteDocRow,
    type SiteNavRow
  } from '@/api/site-pages'

  defineOptions({ name: 'SitePages' })

  const tab = ref('nav')
  const navRows = ref<SiteNavRow[]>([])
  const categories = ref<SiteDocCategory[]>([])
  const docs = ref<SiteDocRow[]>([])
  const logs = ref<SiteChangelogRow[]>([])
  const shared = ref<SiteCompareRow[]>([])

  const navVisible = ref(false)
  const navForm = reactive({ id: 0, label: '', href: '', enabled: true, kind: 'external', sort: 0 })
  const categoryVisible = ref(false)
  const categoryForm = reactive({ id: 0, name: '', slug: '', sort: 0, hidden: false })
  const docVisible = ref(false)
  const docForm = reactive({
    id: 0,
    categoryId: 0,
    title: '',
    slug: '',
    summary: '',
    body: '',
    sort: 0,
    hidden: false
  })
  const logVisible = ref(false)
  const logForm = reactive({
    id: 0,
    version: '',
    releasedOn: '',
    tag: 'added',
    body: '',
    sort: 0,
    hidden: false
  })
  const sharedVisible = ref(false)
  const sharedForm = reactive({ id: 0, body: '', sort: 0, hidden: false })

  function tagLabel(tag: string) {
    if (tag === 'improved') return '优化'
    if (tag === 'fixed') return '修复'
    return '新增'
  }

  function navMore(row: SiteNavRow): RowActionItem[] {
    const items: RowActionItem[] = [
      { key: 'up', label: '上移' },
      { key: 'down', label: '下移' },
      { key: 'toggle', label: row.enabled ? '关闭' : '开启' }
    ]
    if (row.kind === 'external') items.push({ key: 'delete', label: '删除', danger: true })
    return items
  }

  async function loadAll() {
    const [nav, category, doc, log, compare] = await Promise.all([
      fetchAdminNav(),
      fetchAdminDocCategories(),
      fetchAdminDocs(),
      fetchAdminChangelog(),
      fetchAdminCompare()
    ])
    navRows.value = nav.list || []
    categories.value = category.list || []
    docs.value = doc.list || []
    logs.value = log.list || []
    shared.value = compare.list || []
  }

  function openNav(row?: SiteNavRow) {
    navForm.id = row?.id || 0
    navForm.label = row?.label || ''
    navForm.href = row?.href || ''
    navForm.enabled = row?.enabled ?? true
    navForm.kind = row?.kind || 'external'
    navForm.sort = row?.sort || 0
    navVisible.value = true
  }

  async function saveNav() {
    if (navForm.id) {
      await updateAdminNav(navForm.id, {
        label: navForm.label,
        href: navForm.href,
        enabled: navForm.enabled,
        sort: navForm.sort
      })
    } else {
      await createAdminNav({ label: navForm.label, href: navForm.href })
    }
    navVisible.value = false
    await loadAll()
  }

  async function confirmDelete(name: string) {
    await ElMessageBox.confirm(`确定删除「${name}」？`, '删除确认', { type: 'warning' })
  }

  async function move(
    list: { id: number; sort: number }[],
    row: { id: number },
    dir: -1 | 1,
    save: (id: number, sort: number) => Promise<unknown>
  ) {
    const index = list.findIndex((item) => item.id === row.id)
    const other = list[index + dir]
    if (!other) return
    await save(row.id, other.sort)
    await save(other.id, list[index].sort)
    await loadAll()
  }

  function switchedOn(value: string | number | boolean) {
    return value === true || value === 1 || value === 'true'
  }

  async function onNavSwitch(row: SiteNavRow, value: string | number | boolean) {
    await updateAdminNav(row.id, {
      label: row.label,
      href: row.href,
      enabled: switchedOn(value),
      sort: row.sort
    })
    await loadAll()
  }

  async function onCategorySwitch(row: SiteDocCategory, value: string | number | boolean) {
    await saveAdminDocCategory(row.id, {
      name: row.name,
      sort: row.sort,
      hidden: !switchedOn(value)
    })
    await loadAll()
  }

  async function onDocSwitch(row: SiteDocRow, value: string | number | boolean) {
    await saveAdminDoc(row.id, { ...row, hidden: !switchedOn(value) })
    await loadAll()
  }

  async function onLogSwitch(row: SiteChangelogRow, value: string | number | boolean) {
    await saveAdminChangelog(row.id, { ...row, hidden: !switchedOn(value) })
    await loadAll()
  }

  async function onSharedSwitch(row: SiteCompareRow, value: string | number | boolean) {
    await saveAdminCompare(row.id, {
      body: row.body,
      sort: row.sort,
      hidden: !switchedOn(value)
    })
    await loadAll()
  }

  async function onNavAction(action: RowActionItem, row: SiteNavRow) {
    if (action.key === 'edit') return openNav(row)
    if (action.key === 'toggle') {
      await updateAdminNav(row.id, {
        label: row.label,
        href: row.href,
        enabled: !row.enabled,
        sort: row.sort
      })
      await loadAll()
      return
    }
    if (action.key === 'up' || action.key === 'down') {
      await move(navRows.value, row, action.key === 'up' ? -1 : 1, async (id, sort) => {
        const current = navRows.value.find((item) => item.id === id)
        if (!current) return
        await updateAdminNav(id, {
          label: current.label,
          href: current.href,
          enabled: current.enabled,
          sort
        })
      })
      return
    }
    if (action.key === 'delete') {
      await confirmDelete(row.label)
      await deleteAdminNav(row.id)
      await loadAll()
    }
  }

  function openCategory(row?: SiteDocCategory) {
    categoryForm.id = row?.id || 0
    categoryForm.name = row?.name || ''
    categoryForm.slug = row?.slug || ''
    categoryForm.sort = row?.sort || 0
    categoryForm.hidden = row?.hidden || false
    categoryVisible.value = true
  }

  async function saveCategory() {
    await saveAdminDocCategory(categoryForm.id || null, {
      name: categoryForm.name,
      slug: categoryForm.slug,
      sort: categoryForm.sort,
      hidden: categoryForm.hidden
    })
    categoryVisible.value = false
    await loadAll()
  }

  async function onCategoryAction(action: RowActionItem, row: SiteDocCategory) {
    if (action.key === 'edit') return openCategory(row)
    if (action.key === 'toggle') {
      await saveAdminDocCategory(row.id, { name: row.name, sort: row.sort, hidden: !row.hidden })
      await loadAll()
      return
    }
    if (action.key === 'up' || action.key === 'down') {
      await move(categories.value, row, action.key === 'up' ? -1 : 1, async (id, sort) => {
        const current = categories.value.find((item) => item.id === id)
        if (!current) return
        await saveAdminDocCategory(id, { name: current.name, sort, hidden: current.hidden })
      })
      return
    }
    if (action.key === 'delete') {
      await confirmDelete(row.name)
      await deleteAdminDocCategory(row.id)
      await loadAll()
    }
  }

  function openDoc(row?: SiteDocRow) {
    docForm.id = row?.id || 0
    docForm.categoryId = row?.categoryId || categories.value[0]?.id || 0
    docForm.title = row?.title || ''
    docForm.slug = row?.slug || ''
    docForm.summary = row?.summary || ''
    docForm.body = row?.body || ''
    docForm.sort = row?.sort || 0
    docForm.hidden = row?.hidden || false
    docVisible.value = true
  }

  async function saveDoc() {
    await saveAdminDoc(docForm.id || null, { ...docForm, id: undefined })
    docVisible.value = false
    await loadAll()
  }

  async function onDocAction(action: RowActionItem, row: SiteDocRow) {
    if (action.key === 'edit') return openDoc(row)
    if (action.key === 'toggle') {
      await saveAdminDoc(row.id, { ...row, hidden: !row.hidden })
      await loadAll()
      return
    }
    if (action.key === 'up' || action.key === 'down') {
      const same = docs.value.filter((item) => item.categoryId === row.categoryId)
      await move(same, row, action.key === 'up' ? -1 : 1, async (id, sort) => {
        const current = docs.value.find((item) => item.id === id)
        if (!current) return
        await saveAdminDoc(id, { ...current, sort })
      })
      return
    }
    if (action.key === 'delete') {
      await confirmDelete(row.title)
      await deleteAdminDoc(row.id)
      await loadAll()
    }
  }

  function openLog(row?: SiteChangelogRow) {
    logForm.id = row?.id || 0
    logForm.version = row?.version || ''
    logForm.releasedOn = row?.releasedOn || ''
    logForm.tag = row?.tag || 'added'
    logForm.body = row?.body || ''
    logForm.sort = row?.sort || 0
    logForm.hidden = row?.hidden || false
    logVisible.value = true
  }

  async function saveLog() {
    await saveAdminChangelog(logForm.id || null, { ...logForm })
    logVisible.value = false
    await loadAll()
  }

  async function onLogAction(action: RowActionItem, row: SiteChangelogRow) {
    if (action.key === 'edit') return openLog(row)
    if (action.key === 'toggle') {
      await saveAdminChangelog(row.id, { ...row, hidden: !row.hidden })
      await loadAll()
      return
    }
    if (action.key === 'up' || action.key === 'down') {
      await move(logs.value, row, action.key === 'up' ? -1 : 1, async (id, sort) => {
        const current = logs.value.find((item) => item.id === id)
        if (!current) return
        await saveAdminChangelog(id, { ...current, sort })
      })
      return
    }
    if (action.key === 'delete') {
      await confirmDelete(row.version)
      await deleteAdminChangelog(row.id)
      await loadAll()
    }
  }

  function openShared(row?: SiteCompareRow) {
    sharedForm.id = row?.id || 0
    sharedForm.body = row?.body || ''
    sharedForm.sort = row?.sort || 0
    sharedForm.hidden = row?.hidden || false
    sharedVisible.value = true
  }

  async function saveShared() {
    await saveAdminCompare(sharedForm.id || null, { ...sharedForm })
    sharedVisible.value = false
    await loadAll()
  }

  async function onSharedAction(action: RowActionItem, row: SiteCompareRow) {
    if (action.key === 'edit') return openShared(row)
    if (action.key === 'toggle') {
      await saveAdminCompare(row.id, { body: row.body, sort: row.sort, hidden: !row.hidden })
      await loadAll()
      return
    }
    if (action.key === 'up' || action.key === 'down') {
      await move(shared.value, row, action.key === 'up' ? -1 : 1, async (id, sort) => {
        const current = shared.value.find((item) => item.id === id)
        if (!current) return
        await saveAdminCompare(id, { body: current.body, sort, hidden: current.hidden })
      })
      return
    }
    if (action.key === 'delete') {
      await confirmDelete(row.body)
      await deleteAdminCompare(row.id)
      await loadAll()
    }
  }

  onMounted(async () => {
    try {
      await loadAll()
    } catch {
      ElMessage.error('读取官网页面失败')
    }
  })
</script>

<style scoped>
  .site-pages {
    display: flex;
    flex-direction: column;
    gap: 12px;
    min-width: 0;
    overflow-x: clip;
  }

  .toolbar {
    display: flex;
    gap: 8px;
    margin-bottom: 12px;
  }

  h3 {
    margin: 16px 0 8px;
    font-size: 14px;
  }

  .price-hint,
  .hint {
    margin: 0;
    color: var(--el-text-color-secondary);
    font-size: 13px;
    line-height: 1.6;
  }

  .price-hint a,
  .hint {
    overflow-wrap: anywhere;
  }

  .hint {
    margin: 0 0 12px;
  }

  .fit-table {
    width: 100%;
  }

  .fit-table :deep(.cell) {
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
</style>
