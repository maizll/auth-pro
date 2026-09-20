# 首页模板包规范

首页模板与插件同属源站目录，只是分类为模板、清单为 `template.json`。公开 index 里出现在 `homeTemplates`。

## 包布局

```text
my-home.zip
├── template.json
└── …
```

ZIP 硬校验与插件相同。分类 `home-template`（或其它 kind=template 的分类）必须使用 template.json，不能用 plugin.json。

## template.json 必填

| 字段 | 规则 |
| --- | --- |
| id 或 templateKey | 至少一个，格式同插件 id |
| name / version / description / author | 同插件 |
| schemaVersion | **必须为 1** |
| hero.title | 声明式模板必填 |

禁止：`scripts` 字段（包括空数组）。

可选 category 必须是模板类分类，缺省 `home-template`。

## 提交元数据

| 字段 | 规则 |
| --- | --- |
| appId | 必填 |
| templateUrl | HTTPS，或相对路径如 `templates/clean-home.json` |
| sha256 | 64 位十六进制 |
| schemaVersion | 1 |

相对 templateUrl 相对该应用的 `/software-source/{app_key}/index.json` 解析。
