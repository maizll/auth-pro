# AI Skill

可执行步骤在 [`developer-skills/auth-pro-plugin-template/SKILL.md`](../../developer-skills/auth-pro-plugin-template/SKILL.md)。登记来源三选一：上传压缩包、公开地址（仅免费）、私有 GitHub 仓库（收费）。开发者的只读令牌按账号加密保存，只用于自己的条目。

规范正文是 [`charter.md`](./charter.md)。按 starter 打出的 ZIP 必须能通过 `fillPluginManifest` / `fillTemplateManifest`。模板清单硬校验要求 `kind: "template"`、数字 `schemaVersion: 1`、`hero.title`，并且不能有 `scripts`。`hero.primaryAction.type = "login"` 不是上传拒绝条件；宿主在该字段为 `login` 时打开登录框，外观跟随 `stylePreset` 与 `theme`。不要另写 `login.html`。
