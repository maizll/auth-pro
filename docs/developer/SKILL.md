# AI Skill

可执行步骤在 [`developer-skills/auth-pro-plugin-template/SKILL.md`](../../developer-skills/auth-pro-plugin-template/SKILL.md)。登记来源二选一：上传压缩包或公开地址。收费包由本站保管，开发者不填令牌。

规范正文是 [`charter.md`](./charter.md)。按 starter 打出的 ZIP 必须能通过 `fillPluginManifest` / `fillTemplateManifest`。模板清单硬校验要求 `kind: "template"`、数字 `schemaVersion: 1`、`hero.title`，并且不能有 `scripts`。`hero.primaryAction.type = "login"` 不是上传拒绝条件；宿主在该字段为 `login` 时打开登录框，外观跟随 `stylePreset` 与 `theme`。不要另写 `login.html`。
