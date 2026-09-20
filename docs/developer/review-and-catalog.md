# 审核、目录与广告申请

## 1. 概述

| 项 | 说明 |
| --- | --- |
| 适用对象 | 开发者（提交）与源站管理员（审核、上架、广告） |
| 前置条件 | 开发者已入驻；广告申请走独立状态机 |
| 与源站关系 | 目录项按 `appId` 隔离；广告对全站客户端投放，不按应用拆目录 |

## 2. 目录结构（公开 index）

```json
{
  "name": "…",
  "appKey": "app-a",
  "plugins": [{ "id": "demo-widget", "category": "other" }],
  "homeTemplates": [{ "id": "demo-home", "category": "home-template" }],
  "categories": [
    { "key": "payment", "label": "支付", "kind": "plugin", "builtin": true },
    { "key": "template", "label": "模板", "kind": "plugin", "builtin": false }
  ]
}
```

`plugins` / `homeTemplates` 拆分数组是为了兼容旧消费者。`categories` 含内置 + extras，供应用商店生成二级筛选页签。

## 3. 清单字段表（状态）

| 字段名 | 类型 | 必填 | 默认 / 自动填充 | 中文说明 | 校验规则 | 示例值 |
| --- | --- | --- | --- | --- | --- | --- |
| status | string | 系统 | draft | 目录项状态 | 见下表流转 | `"published"` |
| reviewNote | string | 否 | 空 | 审核说明 | ≤500 字 | `"请补 sha256"` |

| 状态 | 含义 |
| --- | --- |
| draft | 草稿，可改元数据 |
| review | 已提交，等待管理员 |
| approved | 审核通过，尚未出现在公开目录 |
| published | 已上架，写入该应用 index.json |
| hidden | 已下架；不会远程卸载已安装实例 |
| rejected | 已驳回，可改后再提交 |
| deprecated | 已弃用 |

广告申请：`pending` → `approved` / `rejected`。通过后才生成真实投放记录。

## 4. kind / 分类绑定

- 内置：payment、realname、other、home-template。
- extras 示例（产品复现）：名称「模板」、标识 `template`、清单 plugin.json。该分类出现在商店筛选中；其插件待在 `plugins`。
- `template.json` + `kind: "template"` 只进入「首页模板」/`homeTemplates`。

## 5. 完整示例

开发者入驻复用代理商账号。目录登记必须选应用。广告申请填写标题、图片、链接与广告位（首页横幅 / 侧栏 / 弹窗）。

## 6. 开发者提交流程

1. **登记**：开发者面板创建草稿（插件 / 模板 / 广告申请）。
2. **草稿**：可改元数据；上架所需的 URL 与 sha256 可稍后补。
3. **提交审核**：目录项进入 review；广告进入 pending。
4. **通过 / 驳回**：管理员操作；驳回后可修改再提交。
5. **上架 / 更新版本**：仅目录项。上架写入 index；之后用「版本」发新包，或由管理员编辑已上架元数据（保持 published）。

## 7. 常见错误与排查

| 现象 | 原因 | 改法 |
| --- | --- | --- |
| 公开目录看不到条目 | 未 published 或 app_key 不匹配 | 确认上架与 URL |
| 商店没有「模板」页签 | 旧前端硬编码页签，或 index 未带 extras | 使用读取 categories 的商店；重新上架/刷新软件源 |
| 自定义分类被丢进「其他」 | 旧 normalize 把未知分类映射为 other | 已改为保留 extras |
| 广告通过了但看不见 | 未生成投放或时段/权重不符 | 在源站广告投放中核对记录 |

## 8. 变更记录

| 日期 | 变更 | 兼容性 |
| --- | --- | --- |
| 2026-09-20 | extras 写入 index.categories，应用商店按此做二级筛选；公开 index 仍拆 plugins / homeTemplates | 旧消费者兼容 |
| 2026-09-20 | template.json 强制 `kind: "template"`，自动绑定 home-template | **Breaking**（仅模板清单） |
| 2026-09-20 | 管理员可编辑已上架目录项并保持 published | 增强 |
