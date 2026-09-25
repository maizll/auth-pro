# 源站开发者文档

开发者面板「开发文档」按下面的文件分章，侧栏用文内的 `##` 标题。登记 UI 在「登记插件」「登记模板」。

硬校验以 `backend/handler/source_station_package.go` 的 `fillPluginManifest` / `fillTemplateManifest` 为准。宿主怎么画首页，以 `frontend/src/views/user-panel/login/home-template.ts` 为准。两者不要混成一条。

| 文件 | 内容 |
| --- | --- |
| [章程](charter.md) | 登记表单、两种包、审核前要满足什么 |
| [插件清单](plugin-package.md) | `plugin.json` |
| [整站模板](template-package.md) | 登记用的 `template.json` |
| [打包与登记](packaging.md) | ZIP、SHA256、表单字段 |
| [拒绝与改法](validation.md) | 上传和提交时的错误 |
| [新版本](versions.md) | 已有正式版本之后怎么换包 |
| [审核之后](review-and-catalog.md) | 状态与公开目录 |

能通过硬校验的最小示例：

- [starter/plugin-example/](starter/plugin-example/)
- [starter/template-example/](starter/template-example/)

本站安装到首页时，还接受静态 `index.html`。那不是登记闸门，见 [首页模板](../home-template.md)。

支付渠道除了清单，还要在服务端实现 Channel，见 [payment-channel-plugin.md](payment-channel-plugin.md)。AI 操作清单在 [SKILL.md](../../developer-skills/auth-pro-plugin-template/SKILL.md)。
