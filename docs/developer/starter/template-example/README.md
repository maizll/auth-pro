# 首页模板示例

首页模板与插件一样走源站目录，只是清单文件为 template.json（声明式 schema v1）。

硬性规则：

- schemaVersion 必须为 1
- 必须有 hero.title
- 禁止 scripts 字段
- id 或 templateKey 至少一个，格式同插件 id

提交元数据时填写 templateUrl（HTTPS 或相对路径如 templates/demo-home.json）和 ZIP/文件的 sha256。
