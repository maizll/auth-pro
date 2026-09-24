# 打包与登记

## 打包

在含清单的目录内执行，保证清单在 ZIP 根目录：

```bash
rm -f /tmp/package.zip
zip -X -r /tmp/package.zip plugin.json
# 模板改为：zip -X -r /tmp/package.zip template.json
unzip -l /tmp/package.zip
sha256sum /tmp/package.zip
```

限制：ZIP、≤20 MiB、≤2048 个条目、解压后 ≤100 MiB、清单 ≤2 MiB、UTF-8。禁止 `..`、绝对路径、反斜杠、符号链接。不要用访达压缩（会带 `__MACOSX`）。

模板登记包里不要放 `index.html`。

## 登记表单

打开 **登记插件** 或 **登记模板**，包来源二选一：

- **上传 ZIP（本站托管）**：选择 ZIP。校验通过后源站保存文件，并自动填写地址和 SHA256。公开地址是 `/api/v1/public/source-packages/<sha256>.zip`。
- **外部 HTTPS**：自己把 ZIP 放到 `https://`，填写地址和校验码。提交审核时源站会下载并核对可达性、ZIP 和 sha256。

本站托管的包在提交时只核对本地文件。

### 默认显示

| 界面标签 | 键 | 提交审核前 |
| --- | --- | --- |
| 应用 | `appId` | 必选 |
| 分类 | `category` | 必选。插件：支付 `payment` / 实名认证 `realname` / 其他 `other`。模板：首页模板 `home-template` |
| 名称 | `name` | 必填，与清单一致 |
| 标识 | `id` | 手写，与清单 `id` 一致。模板同时作为 `templateKey` |
| 版本 | `version` | 默认 `1.0.0` |
| 包来源 | — | 上传 ZIP，或外部 HTTPS |
| 下载地址 | `downloadUrl` | 插件。上传后自动填本站地址；外链必须 `https://` |
| 模板地址 | `templateUrl` | 模板。上传后自动填本站地址；外链为 `https://` 或相对路径 `templates/demo-home.json` |
| 校验码 (SHA256) | `sha256` | 必填，64 位十六进制，对整个 ZIP |
| 简介 | `description` | 界面可空。ZIP 内必填 |

### 高级选项

| 界面标签 | 键 | 说明 |
| --- | --- | --- |
| 作者 | `author.name` | 只读，登录开发者名称 |
| 图标 | `icon` | 仅插件，缺省 `ri:puzzle-line` |
| 更新说明 | `changelog` | 可空，≤2000 |

草稿可以后补地址和校验码。**提交审核** 会检查这两项，并对外部 HTTPS 做可达、ZIP、sha256 核对。外链会先解析 DNS，拒绝回环、私网、链路本地、运营商级 NAT 和云元数据；每次重定向都重新检查，并拨号到已检查的 IP。本站托管 ZIP 只核对本地文件，不发外网请求。界面原文：`提交审核前请先填写下载地址和校验码`（模板为「模板地址」）。外链失败时原文为 `外链不可达`、`外链不是 ZIP`、`外链内容与 sha256 不一致`、`拒绝访问非公网地址`、`拒绝访问链路本地或云元数据地址`。

「自动计算」只在浏览器能跨域读取 https 文件时成功。失败就粘贴 `sha256sum` 的结果。

已有已发布版本后不要改主表单里的地址，改用 [新版本](./versions.md)。
