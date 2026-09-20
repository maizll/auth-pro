# 源站开发者规范

面向**插件 / 首页模板作者**，不是管理端按应用下载的 AuthPro 客户端 SDK。

开发者入驻**复用代理商账号**：在代理商面板「开发者入驻」一键申请，管理员在源站「入驻审核」通过后，使用同一套代理商登录进入开发者端，不再单独设置用户名密码。存量独立开发者账号仍可读；新申请均绑定 `agent_id`。

源站只保存元数据与外部下载地址，**不存储源码或 ZIP**。

| 文档 | 内容 |
| --- | --- |
| [plugin-package.md](./plugin-package.md) | 插件 ZIP 与 plugin.json、硬校验拒绝项 |
| [template-package.md](./template-package.md) | 首页模板 ZIP 与 template.json |
| [review-and-catalog.md](./review-and-catalog.md) | 审核状态、应用隔离、广告申请 |
| [starter/](./starter/) | 最小示例包 |
| [SKILL.md](../../developer-skills/auth-pro-plugin-template/SKILL.md) | 给 AI 编码工具安装的 Skill |

公开机器可读 schema：`GET /software-source/package-schema.json`。

应用级公开目录：`/software-source/{app_key}/index.json`。
