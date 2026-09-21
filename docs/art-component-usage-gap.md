# Art Design Pro 组件与业务页使用差距

分析范围：`frontend/src/components/core/**`、`frontend/src/views/**`，以及 `frontend/vite.config.ts` 的自动导入、`frontend/src/types/import/components.d.ts`、`frontend/src/config/modules/component.ts`。

基线：`origin/master` @ `72e7b07`（`feat: SDK 接入包改为按单一语言下载`）。只读盘点，未改业务代码。

统计口径：

- **业务页**：`frontend/src/views/**/*.vue`（111 个）。`views/index/index.vue` 是后台壳，单独标成「壳」。
- **引用**：模板或脚本里出现 kebab（`art-table`）或 Pascal（`ArtTable`）。不把 `components.d.ts` 的类型声明算成使用。组件自己目录内的自我引用不计入。
- **Element Plus**：`el-*` 与 `El*` 都算。
- **模板残留**：`views/dashboard/console/modules/` 下 7 个文件（`card-list`、`new-user`、`sales-overview`、`active-user`、`dynamic-stats`、`todo-list`、`about-project`）没有任何页面 import，不计入线上业务。

---

## 决策摘要

下一迭代默认做「把还在手写的管理端列表，对齐已经跑通的列表骨架」，不要先做组件示例页，也不要把 `ArtPageContent` 套进每个业务页。

已经跑通的骨架（授权列表、代理、角色、用户、订单等都是这个形状）是：

```text
div.art-full-height
  XxxSearch          ← ArtSearchBar，多数在 modules/*-search.vue
  ElCard.art-table-card
    ArtTableHeader   ← 刷新 / 列设置 / 左侧操作按钮
    ArtTable         ← 列配置 + 插槽 + 内置分页
```

数据层配套 `useTable`（`frontend/src/hooks/core/useTable`）。16 个线上列表页已经这样写。

`ArtPageContent` 不是页面卡片。它是后台布局里的 `RouterView` 容器，只在 `frontend/src/views/index/index.vue` 用了一次。再套一层会嵌套路由出口。

可直接替代、但几乎没人用的模板件，主要是统计卡和一批图表卡。它们覆盖不了业务页真正缺的东西（审核状态、密钥展示、应用选择器）。那些要新做小组件，不要误当成「模板里已经有」。

建议默认路径：

1. 先改同构列表：盗版 4 页、邮件日志、用户/代理授权列表。
2. 顺手改两个工作台组件（源站 `CatalogWorkbench`、开发者 `DeveloperCatalogWorkbench`），一次带上插件/模板/安装包共 5 个路由。
3. 下一轮再抽应用选择器、审核状态、可复制密钥。仪表盘要不要上 `ArtStatsCard`，先看它缺「较昨日」和金额前缀，不要直接替换。
4. 示例页和清理 25 个零引用模板组件放到后面。只改前端，不动后端，不升版本。

---

## 0. 组件是怎么注册的

三套机制并存，业务页不需要 `app.component()`。

| 机制 | 位置 | 作用 |
| --- | --- | --- |
| `unplugin-vue-components` | `frontend/vite.config.ts` 的 `Components({ dts: 'src/types/import/components.d.ts', resolvers: [ElementPlusResolver()] })` | 默认扫描 `src/components`。`art-*` 目录和 `components.d.ts` 里的 `Art*` 都是按需全局组件，模板里可直接写，不必 import。 |
| `unplugin-auto-import` + `ElementPlusResolver` | 同文件 | 自动导入 `vue` / `vue-router` / `pinia` API，以及 Element Plus 的组件解析。`ElMessage`、`ElLoading` 在 auto-import 里额外点名。 |
| `unplugin-element-plus` | 同文件，`useSource: true` | Element Plus 按需样式。 |
| 壳层异步挂载 | `frontend/src/config/modules/component.ts` → `ArtGlobalComponent` | 应用启动时挂 4 个全局件：设置面板、锁屏、礼花、水印。开关是配置里的 `enabled`，不是业务页标签。 |

`components.d.ts` 里还有 `ElAlert`、`ElTable`、`ElForm`、`ElPagination`、`ElCard` 等，说明这些 Element 组件已经在某处模板里被解析过。全局类型表不能当成「业务页采用率」。

同目录但不是 `art-*` 的业务封装已经存在，后文缺口里会用到：

- `components/core/panels/PanelTicketCenter.vue`：用户面板、代理面板工单列表共用
- `components/core/panels/TicketChatPanel.vue`：工单会话
- `components/core/panels/LicenseVersionsDialog.vue`：版本弹窗

---

## 1. 组件清单

下列「用途」来自组件文件头注释；没有注释的，按模板实际行为写。使用次数见第 2 节。

### banners

| 组件 | 用途 |
| --- | --- |
| `art-ad-creative` | 单条广告创意：真实链接包一张投放图；无图时退回「广告位出租」占位。 |
| `art-ad-popup` | 弹出广告，每个会话只打扰一次；没有投放内容时不弹。 |
| `art-ad-slot` | 广告位容器：按位置拉投放，多条轮播，无投放时显示占位。 |
| `art-basic-banner` | 带标题、副标题、装饰/流星的基础横幅。模板展示用。 |
| `art-card-banner` | 图文卡片横幅，可带确认/取消按钮。模板展示用。 |

### base

| 组件 | 用途 |
| --- | --- |
| `art-svg-icon` | Iconify 图标。全库和业务页都在用。 |
| `art-logo` | 系统 Logo。侧栏、登录顶栏使用。 |
| `art-back-to-top` | 返回顶部按钮。 |

### cards

| 组件 | 用途 |
| --- | --- |
| `art-stats-card` | 图标 + 标题 + 数字滚动 + 可选描述/箭头。没有「较昨日」趋势，没有金额前缀。 |
| `art-progress-card` | 大号百分比卡片（数字 + 一条进度条）。不是表格单元格里的进度条。 |
| `art-line-chart-card` | 折线图卡片。 |
| `art-bar-chart-card` | 柱状图卡片。 |
| `art-donut-chart-card` | 环型图卡片。 |
| `art-data-list-card` | 标题 + 图标行列表 + 底部按钮。 |
| `art-timeline-list-card` | 标题 + 时间轴列表。 |
| `art-image-card` | 封面图卡片，点击回调。 |

### charts

| 组件 | 用途 |
| --- | --- |
| `art-line-chart` | 折线图，多序列，阶梯动画。 |
| `art-bar-chart` | 柱状图。 |
| `art-h-bar-chart` | 水平柱状图。 |
| `art-dual-bar-compare-chart` | 双向堆叠柱状图。 |
| `art-ring-chart` | 环形图。 |
| `art-radar-chart` | 雷达图。 |
| `art-scatter-chart` | 散点图。 |
| `art-k-line-chart` | K 线图。 |

### forms

| 组件 | 用途 |
| --- | --- |
| `art-search-bar` | 声明式搜索条：按 item 配置输入、下拉、日期等，带展开/收起。 |
| `art-form` | 同一套声明式表单，给弹窗/整页表单用，不是搜索条。 |
| `art-table-header` 见 tables | — |
| `art-button-table` | 表格行内图标按钮：add / edit / delete / more / view。 |
| `art-button-more` | 「更多」下拉按钮。 |
| `art-excel-export` | 把当前数据导出为 xlsx。 |
| `art-excel-import` | 导入 Excel。 |
| `art-wang-editor` | wangEditor 富文本。 |
| `art-drag-verify` | 拖拽滑块验证。 |

### layouts（后台壳，不是业务页积木）

| 组件 | 用途 |
| --- | --- |
| `art-page-content` | 布局内容区：节日滚动 + `RouterView` + 缓存/过渡。不是每页的内容卡片。 |
| `art-header-bar` | 顶栏：菜单按钮、刷新、快捷入口、通知铃、用户菜单、多页签。 |
| `art-sidebar-menu` | 左侧或双列菜单。 |
| `art-horizontal-menu` | 顶栏水平菜单。 |
| `art-mixed-menu` | 混合菜单。 |
| `art-breadcrumb` | 面包屑，嵌在顶栏里。 |
| `art-work-tab` | 多页签，嵌在顶栏里。 |
| `art-fast-enter` | 顶栏快捷入口气泡。 |
| `art-global-search` | 顶栏全局菜单搜索弹窗。 |
| `art-notification` | 四端共用站内通知面板：通知 / 消息 / 待办。 |
| `art-notification-bell`（`bell.vue`，注册名 `ArtNotificationBell`） | 顶栏铃铛，打开上面的面板。 |
| `art-global-component` | 按配置循环挂载全局壳组件。 |
| `art-settings-panel` | 主题/布局设置抽屉。 |
| `art-screen-lock` | 锁屏。 |
| `art-fireworks-effect` | 礼花。 |
| `art-chat-window` | 模板自带的 Art Bot 抽屉聊天，不是工单。 |
| `art-user-menu` | 顶栏用户菜单。 |
| `art-icon-button`（在 `widget/`） | 顶栏/侧栏用的图标按钮。 |

### tables

| 组件 | 用途 |
| --- | --- |
| `art-table` | `ElTable` 的列配置封装：分页、全局序号、空态、loading、斑马纹/边框/尺寸，并透传 el-table 属性。 |
| `art-table-header` | 表格头：尺寸、刷新、全屏、列设置、左侧操作插槽。 |

### text-effect

| 组件 | 用途 |
| --- | --- |
| `art-count-to` | 数字滚动。 |
| `art-text-scroll` | 文字滚动。 |
| `art-festival-text-scroll` | 节日文字滚动，挂在 `art-page-content` 顶部。 |

### media / others / views

| 组件 | 用途 |
| --- | --- |
| `art-video-player` | 视频播放器（xgplayer）。 |
| `art-cutter-img` | 图片裁剪。 |
| `art-watermark` | 全站水印，由全局配置挂载。 |
| `art-menu-right` | 多页签右键菜单。 |
| `art-exception` | 403 / 404 / 500 异常页主体。 |
| `art-result-page` | 成功/失败结果页主体。 |
| `AuthTopBar` | 登录/注册页右上角。 |
| `LoginLeftView` | 注册、忘记密码页左侧插画。 |

同目录补充（不是 `art-*`，避免和清单混淆）：`PanelThemeToggle` 是面板主题切换；`theme-svg` 注释写明「只对特定 SVG 生效，不建议开发者使用」。

---

## 2. 实际使用统计

### 2.1 业务页已经在用的 art 组件

「业务页文件数」= `views/**/*.vue` 里出现该组件的文件数，含壳和未挂载的 dashboard 模块。括号里是应怎么读。

| 组件 | 业务页文件数 | 读法 |
| --- | --- | --- |
| `art-svg-icon` | 16 | 图标基础设施，仪表盘、列表、活动页都在用。另有约 28 个 core 内部文件在用。 |
| `art-table` | 17 | 16 个线上列表 + 1 个未挂载模块 `dashboard/console/modules/new-user.vue`。 |
| `art-table-header` | 16 | 与上面 16 个线上列表一一对应。`new-user.vue` 没有它。 |
| `art-search-bar` | 15 | 全部是线上页。12 个在 `modules/*-search.vue`，`system/menu/index.vue` 与 `agent/upgrade/index.vue` 写在页面里。 |
| `art-count-to` | 2 | 线上只有 `agent-panel/dashboard`。另一个是未挂载的 `card-list.vue`。 |
| `art-line-chart` | 3 | 线上：`dashboard/console/index.vue`、`agent-panel/dashboard`。未挂载：`sales-overview.vue`。 |
| `art-bar-chart` | 2 | 线上：`agent-panel/dashboard`。未挂载：`active-user.vue`。 |
| `art-notification` | 3 | `user-panel/layout`、`agent-panel/layout`、`developer-panel/layout`。管理端顶栏用的是铃铛，不直接写面板。 |
| `art-notification-bell` | 3 个面板布局 + 顶栏 | 顶栏在 `art-header-bar` 里，`shouldShowNotification` 控制。管理端站内通知已经接上，不是「通知组件没人用」。 |
| `art-ad-slot` / `art-ad-popup` | 各 2 | 只在用户面板、代理面板布局。开发者面板布局没有广告位。 |
| `art-button-table` | 2 | `system/menu`、`system/user`。其余列表用 `ElButton link`。 |
| `art-button-more` | 1 | `system/role`。 |
| `art-form` | 1 | `system/menu/modules/menu-dialog.vue`。 |
| `art-exception` | 3 | 403 / 404 / 500。 |
| `art-result-page` | 2 | `result/success`、`result/fail`。结果页文案仍是模板句「已提交申请，等待部门审核」。 |
| `AuthTopBar` | 2 | 注册、忘记密码。登录页没有用。 |
| `LoginLeftView` | 2 | 注册、忘记密码。登录页是居中卡片，没有左侧插画。 |

只出现在壳或壳的内部拼装里、业务页标签为 0 的组件（这是预期，不是漏用）：

`art-page-content`、`art-header-bar`、`art-sidebar-menu`、`art-horizontal-menu`、`art-mixed-menu`、`art-breadcrumb`、`art-work-tab`、`art-fast-enter`、`art-user-menu`、`art-icon-button`、`art-logo`、`art-menu-right`、`art-festival-text-scroll`、`art-text-scroll`、`art-settings-panel`、`art-screen-lock`、`art-fireworks-effect`、`art-watermark`、`art-global-component`、`art-ad-creative`（被 slot/popup 内部使用）。

`art-global-search` 连顶栏都没有引用，属于壳组件里的死代码。顶栏现在没有全局搜索入口。

### 2.2 从未被业务页引用的组件

下面 25 个在 `views`、壳拼装、其他 core 组件里都没有引用（只活在 `components.d.ts`）：

- 卡片 8 个：`art-stats-card`、`art-progress-card`、`art-line-chart-card`、`art-bar-chart-card`、`art-donut-chart-card`、`art-data-list-card`、`art-timeline-list-card`、`art-image-card`
- 图表 6 个：`art-h-bar-chart`、`art-dual-bar-compare-chart`、`art-ring-chart`、`art-radar-chart`、`art-scatter-chart`、`art-k-line-chart`
- 横幅 2 个：`art-basic-banner`、`art-card-banner`
- 表单 5 个：`art-excel-export`、`art-excel-import`、`art-wang-editor`、`art-drag-verify`，以及 `art-form` 仅 1 处，上面已单列
- 其他：`art-back-to-top`、`art-chat-window`、`art-cutter-img`、`art-video-player`、`art-global-search`

零引用是 25 个（`art-form` 有 1 处，不算零引用）。卡片类是 8/8 全部零引用。

### 2.3 Element Plus 对照：哪些本可以被 art 组件吃掉

111 个 view `.vue` 里，出现次数（文件数）：

| Element | 文件数 | 和 art 组件的关系 |
| --- | --- | --- |
| `ElCard` / `el-card` | 42 | 没有 `art-card`。列表页用 `ElCard.art-table-card` 包 `ArtTable` 是现成写法，不应替换。 |
| `ElForm` / `el-form` | 39 | 含登录、设置、弹窗。只有「筛选项同构」的才适合 `ArtSearchBar`。整页复杂表单不适合一刀切成 `ArtForm`。 |
| `ElTable` / `el-table` | 24 | 其中 22 个文件没有 `ArtTable`。要拆开看：主列表、仪表盘短表、弹层子表，价值差很多。 |
| `ElPagination` / `el-pagination` | 10 | `ArtTable` 主列表自带分页。这 10 处多半是手写列表或抽屉里的第二张表。 |
| `ElDialog` 等 | 弹窗很多 | 现有列表页的新增/编辑仍是手写 `ElForm`，只有菜单弹窗用了 `ArtForm`。 |

**主列表页对照**（页面的主要工作是筛选 + 表格 + 分页）：

已经是 `ArtTable` + `ArtTableHeader`（16）：

- 代理：`agent/level`、`agent/list`、`agent/quota`、`agent/recharge`、`agent/upgrade`
- 授权：`license/apps`、`license/app-versions`、`license/cards`、`license/list`、`license/logs`、`license/plans`、`license/versions`
- 系统：`system/menu`、`system/user`、`system/role`、`system/payment-orders`

其中带 `ArtSearchBar` 的是代理 5 页、授权里的 list/cards/logs/plans、系统 menu/user/role/payment-orders。`license/apps` 注释写明故意不要搜索条。`license/versions`、`license/app-versions` 没有搜索。

仍是手写 `el-table` / `ElTable` 的主列表（14 个页面文件，外加 2 个工作台组件覆盖 5 条路由）：

| 页面 | 现状 |
| --- | --- |
| `piracy/alerts`、`blacklist`、`reports`、`tracking` | 手写筛选 + `el-table` + 独立 `el-pagination` |
| `system/mail-logs` | 手写 `ElForm` 筛选 + `ElTable` + `ElPagination` |
| `source-station/applications` | 审核队列，手写 `el-table`，无分页组件 |
| `source-station/ads`、`source-station/catalog` | 手写筛选 + 表 |
| `source-station/plugins`、`packages`、`templates` | 三页都只渲染 `CatalogWorkbench`（内部 `el-table`） |
| `developer-panel/plugins`、`templates` | 两页都只渲染 `DeveloperCatalogWorkbench` |
| `developer-panel/ads` | 手写表，含审核状态列 |
| `promotion-campaigns` | 自定义摘要卡 + 手写筛选 + `el-table` |
| `user-panel/licenses`、`agent-panel/licenses` | 和已完成的 `license/list` 同域，但是另一套手写工具栏 |
| `agent-panel/finance` | 金额统计卡 + 手写流水分页表 |

16 / (16 + 14) ≈ **53%** 的主列表页已经在 art 骨架上。剩下约一半是同一类页面，却没有复用。

不要算进这 53% 的 `ElTable`（换骨架收益低）：

- 仪表盘短表：`license/dashboard`、`user-panel/dashboard`、`developer-panel/dashboard`、`agent-panel/dashboard`
- 主列表已是 `ArtTable`，子弹层另起一张表：`license/list` 的站点弹窗、`license/cards` 的卡密抽屉（抽屉里还有一套 `ElPagination`）
- 设置页里的记录表：`system/config` 实名认证记录
- 弹窗里的小表：`plugin-store` 的软件源管理
- 文档页示例：`sdk/developer-doc.vue`

**搜索条**：15 个文件用了 `ArtSearchBar`。盗版 4 页、邮件日志、活动、两个授权面板、源站广告/目录、两个工作台，都是「有筛选、不用 `ArtSearchBar`」。

**统计卡**：`ArtStatsCard` 引用次数是 0。线上仪表盘全部手写 `div.art-card` 或 `el-card`。`ArtCountTo` 只在代理商仪表盘被直接使用；管理端控制台自己格式化数字，连 `ArtCountTo` 都没用。

**分页**：主列表若迁到 `ArtTable`，独立 `el-pagination` 可以删掉。工单页 `system/tickets` 的分页是左栏列表，不是表格分页，应保留。

---

## 3. 误用 / 半用（10 处）

### 1. 盗版告警：有搜索、有统计、有表，三套都手写

- 路径：`frontend/src/views/piracy/alerts/index.vue`（`blacklist`、`reports`、`tracking` 同一写法）
- 现状：4 张 `el-card` 数字卡 + `el-form` inline 筛选 + `el-table` + 页底 `el-pagination`。状态用本地 `statusTagMap`。
- 建议：筛选换 `ArtSearchBar`，表换 `ArtTable` + `ArtTableHeader`，分页交给 `ArtTable`。4 张「标题 + 数字」卡是全库最接近 `ArtStatsCard` 的用法，可以换，但要接受没有趋势行（这 4 张卡本来也没有趋势）。

### 2. 邮件日志：标准筛选表，却没走上已有骨架

- 路径：`frontend/src/views/system/mail-logs/index.vue`
- 现状：`art-full-height` 已经有了，里面仍是 `ElForm` inline + `ElTable` + `ElPagination`。
- 建议：对齐 `system/payment-orders`：`order-search.vue` 那种 `ArtSearchBar` + `ArtTable`。事件类型、状态都是下拉，`ArtSearchBar` 的 select item 够用。

### 3. 用户授权列表：和后台授权列表同域，骨架分裂

- 路径：`frontend/src/views/user-panel/licenses/index.vue`
- 现状：关键字、应用下拉、状态下拉、查询/重置，然后手写 `el-table` 和分页。应用下拉自己拉 `appList`。
- 建议：筛选与表对齐 `license/list`（已有 `license-search.vue` + `ArtTable`）。应用下拉先继续本地拉数；抽「应用选择器」是下一轮的事，不要和换骨架绑在一起。
- 同类：`frontend/src/views/agent-panel/licenses/index.vue`。

### 4. 授权仪表盘：统计卡和短表都绕开了现成卡片

- 路径：`frontend/src/views/license/dashboard/index.vue`
- 现状：4 张手写统计卡（标题、数字、图标、较昨日百分比）+ 「最近授权」`el-table` + 「即将到期」手写列表。
- 建议：短表可以保持 `el-table`（行数少、无分页列设置）。统计卡不要直接换成现在的 `ArtStatsCard`，因为它没有趋势行。若产品要统一仪表盘，应先给 `ArtStatsCard` 加可选 `trend`，再替换这里、控制台和用户面板。这是「模板不够」，不是「忘了用」。

### 5. 管理端控制台：半用图表，统计卡整段重写

- 路径：`frontend/src/views/dashboard/console/index.vue`
- 现状：真实接口的统计卡（含金额前缀、单位、较昨日）+ `ArtLineChart`。同目录 `modules/*` 是 Art Design Pro 示例，硬编码「+23%」「像素完美」，且没有被 import。
- 建议：删或隔离 `modules/*`，避免后人把示例当业务组件抄进去。统计卡保持自定义，或等 `ArtStatsCard` 补上前缀和趋势后再收。折线图继续用 `ArtLineChart`。

### 6. 代理商仪表盘：把模板卡片抄了一份，还留着假趋势

- 路径：`frontend/src/views/agent-panel/dashboard/index.vue`
- 现状：布局几乎是 `card-list.vue` 的复制，数字用 `ArtCountTo`，但「较上周」和折线图标题「本月新增 +23%」写死在模板里。下面还有 `ArtLineChart`、`ArtBarChart`。
- 建议：先去掉写死的涨跌文案，改成接口字段或先不展示。组件层面不必再包一层 `ArtStatsCard`，除非卡片 API 能表达趋势。图表组件本身用对了。

### 7. 源站入驻审核：审核队列没有沿用列表骨架

- 路径：`frontend/src/views/source-station/applications/index.vue`
- 现状：`el-card` + `el-table`，状态 `el-tag`，操作是通过/拒绝。无搜索、无 `ArtTable`。
- 建议：表可以换成 `ArtTable`（列不多，收益主要是空态、loading、以后加分页时不用再写一套）。通过/拒绝不是 `art-button-table` 能表达的，保留文字按钮。审核状态条见第 4 节，是新组件假设。

### 8. 活动页：摘要卡、筛选、表格全自定义

- 路径：`frontend/src/views/promotion-campaigns/index.vue`
- 现状：自定义 `summary-grid`、应用下拉 + 关键字、`el-table`、本地 `statusMeta`。
- 建议：筛选和表迁到 `ArtSearchBar` + `ArtTable`。摘要卡是运营语义（启用中的活动数等），`ArtStatsCard` 只能覆盖「一个数字」；要统一外观可以包一层，不必为了用而用。

### 9. 配额页：主列表用对了，进度条用的是表格里的 `ElProgress`

- 路径：`frontend/src/views/agent/quota/index.vue`
- 现状：`QuotaSearch` + `ArtTableHeader` + `ArtTable`。使用率列是 `ElProgress`。
- 建议：保持 `ElProgress`。`ArtProgressCard` 是 128px 高的百分比卡片，放进单元格不合适。这是半用里的「正确半用」，不要为了覆盖率去换。

### 10. 工单：通知组件和聊天组件都对不上业务

- 路径：`frontend/src/views/system/tickets/index.vue`；面板侧 `user-panel/tickets`、`agent-panel/tickets` 指向 `PanelTicketCenter`
- 现状：管理端是左列表右会话，自己分页。面板端是另一套 `el-table` 工单表。站内通知走 `ArtNotification`，和工单未读角标（`views/index/index.vue` 里 30 秒轮询菜单 badge）是两条线。
- 建议：不要用 `art-chat-window` 替换工单。那个组件是 Art Bot 演示抽屉。不要用 `ArtNotification` 替换工单中心。若要收口，应在已有 `PanelTicketCenter` / `TicketChatPanel` 上对齐管理端，而不是回到模板聊天窗。

补充一个容易误判的半用：`license/list` 和 `license/cards` 主表已经是 `ArtTable`，弹窗/抽屉里的第二张表仍是 `ElTable`。这不该算「列表页没用 art-table」。

---

## 4. 业务缺口假设

以下都是假设，依据是页面里重复出现的 UI，不是需求文档。分成两类。

### 模板已有，但业务页没用（或只用了一半）

| 假设 | 依据 | 判断 |
| --- | --- | --- |
| 管理端列表可以继续用现有骨架，不必新做 `art-page` | 16 个列表已用同一结构；14 个手写列表字段类型类似（关键字、状态下拉、应用下拉、时间） | 成立。优先改页面，不新增组件。 |
| 仪表盘数字卡应该全部换成 `ArtStatsCard` | 控制台、授权仪表盘、用户/开发者/代理仪表盘、活动摘要、财务卡都是手写卡；`ArtStatsCard` 零引用 | 只部分成立。盗版告警那种纯数字卡可以换。带「较昨日」、货币拆分、单位的卡，现组件表达不了。 |
| 站内通知未接入 | 顶栏有 `ArtNotificationBell`，三个面板布局也挂了 `ArtNotification` | 不成立。缺口是「工单未读」和「通知面板」两套提醒，不是缺组件。 |
| 广告位未用 art 广告组件 | 用户/代理侧栏已用 `ArtAdSlot` + `ArtAdPopup` | 基本不成立。开发者面板布局没挂广告位，那是产品要不要投放，不是组件缺失。 |
| 导出应使用 `ArtExcelExport` | 组件零引用；卡密、授权、订单页未见导出按钮 | 弱假设。有导出需求时再接，不要为了用组件先做导出。 |
| 富文本/裁剪/视频/拖拽验证/K 线会在后台用到 | 全部零引用 | 不成立。授权后台没有这些场景。留着只会让清单变脏。 |

### 真正缺的新组件（模板里没有等价物）

| 假设组件 | 重复出现的位置 | 为什么不是现成 art 组件 |
| --- | --- | --- |
| 审核/业务状态标签 | `statusMeta()` 散落在开发者仪表盘、开发者广告、`DeveloperCatalogWorkbench`、源站入驻、活动页、授权列表插槽 | `ElTag` 只有颜色。各页自己映射 pending/approved/rejected。缺的是一份状态字典和统一标签，不是再包一个卡片。 |
| 可复制密钥/授权码 | `license/apps` 的 AppSecret 显示/隐藏；`agent-panel/licenses`、`agent-panel/purchase` 的 `navigator.clipboard` | 没有 `art-secret`。`art-button-table` 只有图标按钮。 |
| 应用选择器 | `user-panel/licenses`、`agent-panel/licenses`、`promotion-campaigns`、`piracy/tracking`、`license/list` 弹窗、`license/plans` 等各自 `fetch` 应用再 `el-option` | `ArtSearchBar` 的 select 只渲染 options，不负责拉应用列表。选择器是数据和控件绑在一起的业务件。 |
| 配额摘要 | 配额表用单元格 `ElProgress`；购买页 `agent-panel/purchase` 只显示「剩余 N」 | `ArtProgressCard` 是整卡百分比，没有「总量 / 已用 / 剩余 / 无限」。表内进度应继续用 `ElProgress`。若要卡片，得新做，不能挪模板卡。 |
| 审核操作条 | 源站入驻的通过/拒绝；目录工作台的「提交审核」+ `reviewNote` | 模板没有审核流组件。`ArtResultPage` 是静态结果页。 |
| 工单会话壳 | 管理端 `system/tickets` 与 `PanelTicketCenter` 两套布局 | `art-chat-window` 是演示机器人。已有面板组件，缺的是管理端与面板端是否合成一个，这是收敛，不是从模板里找。 |

`art-form` 不算缺口。它和 `ArtSearchBar` 同源，适合字段平坦的弹窗。授权开通、购买、源站设置里有分段、联动、上传，硬套声明式表单会把校验和布局弄丢。维持手写 `ElForm` 是合理的。

---

## 5. 优先级

改动量用「动哪些文件、是否改组件 API」描述，不估工期。三档都只改前端。

### P0：对齐列表骨架

收益最高，不改组件库行为。每页把筛选、列、分页搬进已有骨架，接口调用留在原页面。

1. **盗版 4 页**（`piracy/alerts`、`blacklist`、`reports`、`tracking`）  
   改动：4 个 view，可抽一个共用 search 配置。收益：同一模块从 4 套手写表收成 1 种骨架。只改前端。
2. **邮件日志** `system/mail-logs/index.vue`  
   改动：1 个 view，必要时加 `modules/mail-log-search.vue`。收益：和订单、用户列表一致，分页逻辑删掉自写的那份。只改前端。
3. **用户授权、代理授权** `user-panel/licenses`、`agent-panel/licenses`  
   改动：2 个 view。收益：和已完成的 `license/list` 同构，后续状态列、密钥列可以复用。只改前端。面板布局不要改成后台 `ArtSidebarMenu`。
4. **两个目录工作台** `CatalogWorkbench.vue`、`DeveloperCatalogWorkbench.vue`  
   改动：2 个组件，带上源站插件/安装包/模板和开发者插件/模板共 5 个路由。收益：一次改完一类审核表。只改前端。
5. **代理财务流水** `agent-panel/finance` 的表格部分  
   改动：1 个 view 的下半张表。金额统计卡先不动。只改前端。

不做：不要在 P0 改 `ArtPageContent`、不要换仪表盘、不要引入 `ArtForm` 重写弹窗。

### P1：补 3 个业务小件，并决定统计卡要不要加能力

1. **应用选择器**（新组件，例如放在 `components/core/panels/` 或业务目录，不必叫 art-）  
   改动：1 个新组件 + 替换 P0 列表和购买/活动里重复的 `el-select`。收益：应用列表请求和空态只维护一处。只改前端。
2. **状态标签字典**  
   改动：1 个小组件或一个映射模块 + 授权/审核/活动列替换。收益：pending、expiring、rejected 的颜色不再各页各写。只改前端。不要做成重型「审核状态条」，除非产品要在列表外展示时间线。
3. **可复制密钥**  
   改动：1 个小组件，替换 AppSecret 显隐和授权码复制。收益：复制失败提示和遮罩一致。只改前端。
4. **统计卡：扩展或放弃**  
   先定产品：要不要「较昨日 / 金额前缀 / 单位」。要，就给 `ArtStatsCard` 加可选字段，再替换控制台、授权仪表盘、用户面板、盗版告警。不要，就让 `ArtStatsCard` 保持零引用，P2 再决定删不删。只改前端。
5. **去掉代理商仪表盘写死的 +23%**  
   改动：`agent-panel/dashboard/index.vue` 文案。收益：避免假数据进生产界面。与组件替换无关，但应在动仪表盘时一起做。只改前端。

### P2：示例页或删模板，二选一

1. **不要先做「组件示例页」当主迭代。** 示例页帮不上授权/审核/订单。若做，只给还可能用到的组件做一页索引（`ArtSearchBar`、`ArtTable`、`ArtStatsCard` 扩展后的样子），并写明 `ArtPageContent` 是布局容器。改动：1 个新 view + 路由。收益是减少误用，不是交付业务。只改前端。
2. **清理零引用模板件**（K 线、雷达、散点、视频、富文本、拖拽验证、Art Bot、两套横幅、全局搜索若确认不要）。改动：删文件并更新 `components.d.ts` 生成结果。收益：清单和打包扫描变短。风险：若有人打算做营销页，删了要找回。只改前端。
3. **`ArtExcelExport` 接到卡密或订单**，仅当产品确认要导出。现在没有按钮，接上是新功能不是还债。
4. **`art-button-table` 推广到各列表操作列。** 收益低。现有页用文字链（编辑/禁用/删除）更清楚。不建议为了统一改成纯图标。
5. **隔离 `dashboard/console/modules/*`。** 7 个文件未被引用，继续留在 `views` 会被当成可用业务模块。移到文档示例或删除。只改前端。

### 默认路径（建议直接采用）

先统一列表页骨架，再补业务专用块。骨架指的是已经在用的这一组，而不是 `ArtPageContent`：

`art-full-height` + `ArtSearchBar` + `ElCard.art-table-card` + `ArtTableHeader` + `ArtTable` + `useTable`

第一批就 P0 的 1–4。P0 完成后再做 P1 的应用选择器、状态标签、密钥展示。示例页放到 P2，除非团队明确有人会按示例页接入新页面。

---

## 6. 本轮明确不做

- 不大范围重构列表或仪表盘。
- 不升级依赖，不改版本号。
- 不改后端接口、权限和菜单数据。
- 不把 `ArtPageContent` 嵌进业务页。
- 不用 `art-chat-window` 替换工单，不用 `ArtProgressCard` 替换表格里的配额进度。
- 不把 39 处 `ElForm` 收成 `ArtForm`。
