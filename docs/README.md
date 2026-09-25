# auth-pro 文档

产品版本以仓库根目录 `VERSION` 为准，当前 **1.5.3**。下面的手册按当前代码编写。

## 给采购、运维和管理员

| 文档 | 内容 |
| --- | --- |
| [部署手册](deployment.md) | 宝塔安装/升级脚本、手工部署、进程守护、Nginx、SSL、在线更新、回滚、端口被占用 |
| [管理手册](admin.md) | 后台菜单各自做什么 |
| [安全说明](security.md) | 已做的加固，以及运维必须自己保管的密钥和备份 |
| [发布包目录](../PACKAGING.md) | `auth_pro-full-*.tar.gz` 解压后的文件布局 |
| [更新日志](../CHANGELOG.md) | 本产品各版本变更 |
| [发布说明](release-notes-1.5.3.txt) | 当前版本写入在线更新 `notes` 的文本；历史版本为同目录 `release-notes-*.txt` |

## 给接入方和模板作者

| 文档 | 内容 |
| --- | --- |
| [API 与 SDK](api-sdk.md) | `POST /api/license/verify`、版本检查、按语言下载接入包 |
| [开发者文档](developer/README.md) | 插件与首页模板的登记章程。开发者面板「开发文档」直接渲染这些 Markdown |
| [首页模板安装](home-template.md) | 本站安装模板时的 `template.json` 与静态 `index.html` |
| [软件源清单地址](software-source-client-url.md) | 按应用拉取 `index.json` |
| [源站仓库 Token](source-station-repo-setup.md) | 后台把包推到 GitHub / Gitee Release |

管理端「接入开发 → 开发文档」是授权 API 说明，「模板文档」是首页模板安装说明。开发者面板里的「开发文档」才是登记章程。

## 内部记录（不是采购手册）

这些文件保留作实现记录，不代替上面的手册：

- [侧栏数据来源](admin-navigation.md)
- [P0/P1 审计原文](audit/p0-p1-2026-09.md)（v1.5.0 当时的发现，修复状态见安全说明）
- [支付渠道设计](design/payment-channel-plugin.md)、[源站设计](design/software-source-enterprise.md)
- [SDK 设计记录](superpowers/specs/2026-09-21-client-sdk-hybrid-design.md)
- [业务组件设计记录](superpowers/specs/2026-09-22-biz-app-status-secret-components-design.md)

仓库根目录 [NOTICE](../NOTICE) 说明许可证与界面来源。
