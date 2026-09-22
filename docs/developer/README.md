# 源站开发者文档

唯一规范是 [开发者章程](./charter.md)。插件与首页模板都按它生成、打包、登记。

| 文件 | 用途 |
| --- | --- |
| [章程](./charter.md) | 交给 AI 的合同：整站模板（含登录入口）、插件清单、ZIP、登记表单、拒绝原因 |
| [插件清单](./plugin-package.md) | `plugin.json` 骨架与打包命令 |
| [整站模板](./template-package.md) | `template.json` 整站骨架（登录动作、能力卡片、页脚） |
| [打包与登记](./packaging.md) | ZIP 命令、sha256、表单中文标签对照 |
| [拒绝与改法](./validation.md) | `error.field` / `error.rule` |
| [新版本](./versions.md) | 「版本」抽屉 |
| [审核之后](./review-and-catalog.md) | 状态与管理员操作 |

示例（必须能过硬校验）：

- [`starter/plugin-example/`](./starter/plugin-example/)
- [`starter/template-example/`](./starter/template-example/)

AI 步骤：[SKILL.md](../../developer-skills/auth-pro-plugin-template/SKILL.md)。

支付渠道在包合同之外还要服务端 Channel，见 [payment-channel-plugin.md](./payment-channel-plugin.md)。
