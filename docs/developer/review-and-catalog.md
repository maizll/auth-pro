# 审核状态与应用隔离

## 目录项状态

`draft` → `review` → `approved` → `published`（上架）。也可 `rejected` / `hidden`（下架） / `deprecated`。

| 状态 | 含义 |
| --- | --- |
| draft | 草稿，可改元数据 |
| review | 已提交，等待管理员 |
| approved | 审核通过，尚未出现在公开目录 |
| published | 已上架，写入该应用的 index.json |
| hidden | 已下架，公开目录不再列出；不会远程卸载已安装实例 |
| rejected | 已驳回，可改后再提交 |
| deprecated | 已弃用 |

版本另有：`draft` / `pending` / `published` / `deprecated`。已发布版本不可改包地址，请新增版本。

## 应用隔离

- 每个插件/模板必须绑定 `appId`
- 公开清单：`GET /software-source/{app_key}/index.json`
- 未带应用的 `/software-source/index.json` 是空目录，不要当默认源
- 开发者 `GET /api/v1/source/developer/apps` 返回每条应用的 `indexUrl`

## 广告申请

广告对全站客户端投放，不按应用拆目录。开发者提交申请（pending），管理员通过后才会生成真实广告记录；拒绝不会投放。
