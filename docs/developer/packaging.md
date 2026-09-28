# 打包与登记

## ZIP 限制

登记上传与管理员解析使用同一套限制：

- 必须是 ZIP（文件头为 `PK`），表单字段名 `file`
- 不超过 20 MiB
- 解压后不超过 100 MiB，条目不超过 2048
- 清单在根目录或一层子目录
- 拒绝路径穿越、绝对路径、符号链接、重复或大小写冲突的路径、空包、只有 `__MACOSX` 的包

`kind` 可随表单提交。不提交时按包里是 `plugin.json` 还是 `template.json` 识别。两者都在时，未指定 `kind` 会先按插件解析。

## 打包命令

在示例目录执行。`-X` 去掉额外属性，便于 SHA256 稳定。

```bash
cd docs/developer/starter/plugin-example
rm -f /tmp/demo-widget.zip
zip -X -r /tmp/demo-widget.zip plugin.json
sha256sum /tmp/demo-widget.zip
```

模板把目录换成 `docs/developer/starter/template-example`，把 `plugin.json` 换成 `template.json`。

Windows 可用资源管理器压缩该 json，再计算 SHA256。算的是 ZIP 文件，不是 json 文本。

## 登记时怎么填

| 表单标签 | 插件 | 模板 |
| --- | --- | --- |
| 标识 | `demo-widget` | `demo-home` |
| 分类 | 其他（`other`） | 首页模板（`home-template`） |
| 售价（元） | 默认 0。大于 0 为买断 | 同左 |
| 包来源 | 二选一：上传 ZIP，或公开 https | 同左 |
| 校验码 | 免费公开地址自行计算。收费由本站按包内容填写 | 同左 |

售价不写进清单。两种来源：

1. **公开地址**。免费时填写 https 下载地址。本站下载校验清单、路径和大小后不保存压缩包，目录继续使用这条地址。收费时本站拉取一次，校验后放进站长的收费仓库。
2. **上传压缩包**。收费条目校验后由本站保管。免费条目请改用公开地址，或由管理员勾选推送 Release。

开发者不填令牌，也不需要自己的仓库。站长在「源站运营 → 存储管理」添加主存储。可以是 GitHub 私有仓库、Gitee 私有仓库、S3 兼容对象存储，或 Nextcloud、ownCloud、Seafile、Alist、Cloudreve 这类 WebDAV 网盘。自建 MinIO 仍选对象存储。点「测试连接」会用中文说明缺什么权限。密钥加密保存，页面不能查看。上传先写主存储，失败再试备用；可以同时写入备用。对象存储下载使用短时签名地址。GitHub、Gitee 和 WebDAV 的地址或账号不能交给买家，买家仍从官网下载，由官网取包并核对校验码。超过单文件上限时自动分片，下载时拼合后再核对。还没配置主存储时，收费包暂存在本站，只在后台提醒。存储检查出问题只通知管理员，不会自动下架。旧的收费仓库和发布仓库设置会自动迁成列表里的两条。详见 [存储管理](../storage-management.md)。

按钮不可用时会写出中文原因。以前被标成隐藏或弃用的收费公开地址，升级后回到草稿，并标注「需要重新上传 zip 或填写公开地址，才能继续收费出售」。已上架、免费、以及已经由本站保管的条目不会被改写。

## 管理员解析

管理员在「源站 → 软件目录」也可以上传。只解析、不入库：

`POST /api/v1/source/admin/packages/parse`

字段 `file`，可选 `kind=plugin|template`。失败时 HTTP 体里 `code` 为 400，并带 `error.field` 与 `error.rule`，不写库、不推 Release。
