# 业务小件：应用选择、状态标签、可复制密钥

基线：`origin/master`。只改前端。不升版本，不打 Release，不改后端路由和响应契约。

## 目标

把三处重复的业务 UI 收成可复用小件，替换真实页面后接口、权限和校验保持原样。

- `BizAppSelect`：筛选项和弹窗里的应用下拉，内部复用现有应用列表请求。
- `BizStatusTag`：按业务域显示状态文案和标签颜色。
- `BizCopySecret`：密钥默认遮罩，可查看/隐藏，可复制。

## 放置

`frontend/src/components/core/biz/`。不使用 `art-*` 命名。`unplugin-vue-components` 扫描 `src/components`，模板可直接写组件名。

## 组件

### BizAppSelect

职责：`v-model` 绑定应用 id（或 `valueKey="appKey"`），`filterable` 的 `ElSelect`，带 loading 和「暂无应用」。

Props：`multiple`、`clearable`、`disabled`、`placeholder`、`valueKey`（`id` | `appKey`，默认 `id`）、`api`、`loader`。

`loader` 优先。未传时按 `api` 调用仓库里已经在用的请求：

| `api`                     | 现有函数 / 请求                                          | 失败提示                                |
| ------------------------- | -------------------------------------------------------- | --------------------------------------- |
| `license-options`（默认） | `fetchLicenseAppOptions` → `GET /api/license/apps`       | 沿用 `request` 自带提示，组件不再加一条 |
| `license-apps`            | `fetchLicenseAppList` → `GET /api/app/list`              | 同上                                    |
| `promotion`               | `fetchPromotionApps` → `GET /api/license/apps`           | 同上                                    |
| `user-panel`              | `axios` `GET /api/user-panel/apps`，`user_panel_token`   | `加载应用列表失败`                      |
| `agent-panel`             | `axios` `GET /api/agent-panel/apps`，`agent_panel_token` | 静默，保持空列表                        |

面板端继续用页面同款 `axios` + 面板 token，不走管理端 `request`（那个客户端会带后台 token，401 会登出后台）。

`valueKey="appKey"` 只在选项带 `appKey` 时可用（`license-apps` 或自定义 `loader`）。没有 `appKey` 的项不进入下拉。

非目标：创建应用、应用详情弹窗、新后端路由。

### BizStatusTag

职责：`status` + `domain` 映射成中文 `ElTag`。未知状态用灰色 `info` 标签显示原文，不抛错。空状态显示 `-`。传入 `label` 时只覆盖文案，颜色仍走字典（用于授权列表继续显示接口返回的 `statusLabel`）。

| domain     | 依据                                         | 字典                                                                                                               |
| ---------- | -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `review`   | `SOURCE_APPLICATION_STATUS`                  | pending 待审核 warning；approved 已通过 success；rejected 已拒绝 danger；cancelled 已取消 info；frozen 已冻结 info |
| `ad`       | 广告申请页同一份 `SOURCE_APPLICATION_STATUS` | 与 `review` 相同。分开是为了广告状态以后单独变，不改审核域                                                         |
| `license`  | 管理端授权列表颜色 + 用户/代理端 `expiring`  | active 正常 success；expiring 即将到期 warning；expired 已过期 info；disabled 已禁用 danger                        |
| `campaign` | 活动页 `statusMeta`                          | active 进行中 success；upcoming 未开始 primary；ended 已结束 info；disabled 已禁用 warning                         |

同一英文状态在不同域可以不同：`active` 在授权是「正常」、在活动是「进行中」；`disabled` 在授权是 danger、在活动是 warning。目录商品的 `SOURCE_ITEM_STATUS`（已通过是 primary、已驳回等）不并进 `review`，避免和入驻审核冲突。

非目标：审核时间线、通过/拒绝操作条。

### BizCopySecret

职责：默认 16 位圆点遮罩（与应用列表现有遮罩一致）；「查看 / 隐藏」；「复制」。空值只显示 `emptyText`（默认 `—`），不提供复制。成功/失败用 `ElMessage`。`defaultVisible` 默认 false；兑换结果页传 true，让刚发出的密钥仍然直接可见。

非目标：二维码、下载。

## 替换页面

应用选择：

- `frontend/src/views/user-panel/licenses/index.vue` 筛选（`user-panel`）
- `frontend/src/views/promotion-campaigns/index.vue` 筛选和新建/编辑弹窗（`promotion`）
- `frontend/src/views/license/list/index.vue` 新增/编辑弹窗（`license-options`）

状态标签：

- `frontend/src/views/developer-panel/ads/index.vue`（`ad`）
- `frontend/src/views/source-station/applications/index.vue`（`review`）
- `frontend/src/views/license/list/index.vue`（`license`，文案仍用 `statusLabel`）
- `frontend/src/views/user-panel/licenses/index.vue` 与 `frontend/src/views/agent-panel/licenses/index.vue`（`license`，含即将到期，文案仍用 `statusLabel`）
- `frontend/src/views/promotion-campaigns/index.vue`（`campaign`）

可复制密钥：

- `frontend/src/views/license/apps/index.vue` 的 AppSecret 列
- `frontend/src/views/agent-panel/licenses/index.vue` 兑换结果里的授权密钥
- `frontend/src/views/user-panel/licenses/index.vue` 同一处兑换结果（与代理端同一套复制文案）

## 验收

- 三个组件位于 `biz` 目录，业务页模板可直接使用。
- 每类至少两处真实页面已替换，请求路径与替换前相同。
- `pnpm exec tsx` 跑字典/选项/复制的纯函数测试。
- `pnpm exec vue-tsc --noEmit` 做前端类型检查。
- 不改 P0 列表骨架、示例页、`ArtStatsCard`、零引用模板清理。
