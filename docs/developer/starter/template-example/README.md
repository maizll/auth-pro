# 首页模板示例

首页模板清单必须是 `template.json`，且**必须**包含 `"kind": "template"`（2026-09-20 breaking）。缺省分类自动绑定 `home-template`。

硬性规则：

- `kind` 必须为 `template`
- schemaVersion 必须为 1
- 必须有 hero.title
- 禁止 scripts 字段
- id 或 templateKey 至少一个，格式同插件 id
- 不能使用 payment / realname / other，也不能使用插件 extras（即使其标识叫 `template`）

提交元数据时填写 templateUrl（HTTPS 或相对路径）和 ZIP/文件的 sha256。

完整规范：`docs/developer/template-package.md`。
