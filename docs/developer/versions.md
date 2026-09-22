# 新版本

已上架条目的包地址不能在登记抽屉里改。界面提示：`已有正式版本后，包地址请通过「版本」新增，不要改当前草稿字段。`

## 步骤

1. 复制上一版目录，只改清单 `version`（`id` 不变）。模板仍是完整 `template.json`（含 `kind`、`schemaVersion: 1`、`hero.title`、登录动作）。
2. 按 [打包与登记](./packaging.md) 重新 zip，计算新的 sha256。旧校验码不能复用。
3. 列表点 **版本** → **新增版本**。
4. 填写后点 **保存草稿**，再在该行点 **提交审核**。

| 界面标签 | 键 | 规则 |
| --- | --- | --- |
| 版本 | `version` | 新号，如 `1.0.1`。格式同清单 version |
| 包来源 | — | 上传新 ZIP，或填写外部 HTTPS |
| 下载地址 / 模板地址 | `downloadUrl` / `templateUrl` | 上传后自动填写；外链为新 ZIP 的地址 |
| 校验码 (SHA256) | `sha256` | 必填 |
| 更新说明 | `changelog` | 可空，≤2000 |

版本状态：草稿 `draft`、待审核 `pending`、已发布 `published`、已弃用 `deprecated`。标「当前」的是对外版本。

已发布版本再改地址，服务端返回：`已发布版本不可改包地址，请创建新版本`。

管理员通过后把该版本设为对外版本。开发者不能自己切换。弃用当前已发布版本后，条目会从公开目录去掉。

请求体：

```json
{ "version": "1.0.1", "downloadUrl": "https://cdn.example.com/demo-widget-1.0.1.zip", "sha256": "64位", "changelog": "修正说明文案" }
```

模板把 `downloadUrl` 换成 `templateUrl`。
