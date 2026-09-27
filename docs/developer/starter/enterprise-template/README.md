# 企业蓝官网模板

这是官方示例模板，风格是浅色留白和蓝色主色。把它打成 ZIP 后，可以走源站「登记」上传校验。

包里只有 `template.json`。硬校验要求 `kind` 为 `template`、`schemaVersion` 为数字 `1`、`hero.title`，并且没有 `scripts`。`stylePreset` 写 `enterprise` 时，宿主用企业风格画首页、登录注册框和四个公开页面。页面正文不在这个包里。

宿主固定提供这些入口，模板不能换成自己的页面：

- 首页
- 授权购买 `/buy`
- 系统对比 `/compare`
- 系统文档 `/docs`
- 更新日志 `/changelog`

可以改 `theme` 里的颜色，以及首页的标题、能力卡片和场景卡片。不要写登录接口，不要放 `login.html`，不要在模板里编套餐价格。

```bash
cd docs/developer/starter/enterprise-template
zip -X -r /tmp/enterprise-blue.zip template.json
```

登记时标识填 `enterprise-blue`，分类选「首页模板」。
