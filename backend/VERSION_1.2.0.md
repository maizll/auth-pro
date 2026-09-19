# v1.2.0 版本说明

产品版本升至 **1.2.0**。在线更新默认改为 GitHub Releases（`maizll/auth-pro`）；广告与软件源默认指向生产域 `auth.maizll.com`，不连接官方 91ani。

## 默认源

| 项 | 默认值 |
|---|---|
| 产品版本 | `1.2.0` |
| 在线更新清单 | `https://api.github.com/repos/maizll/auth-pro/releases/latest`（可用 `AUTO_PRO_UPDATE_URL` 覆盖） |
| 广告 | `https://auth.maizll.com/api/v1/public/advertisements`（本地：`AUTO_PRO_ADVERTISEMENT_URL=/api/v1/public/advertisements`） |
| 远程软件源 | `https://auth.maizll.com/software-source`（设 `-` 关闭；本地：`AUTO_PRO_SOFTWARE_SOURCE_URL=http://127.0.0.1:19127/software-source`） |

更新客户端会读取 GitHub 最新 Release 的 `latest.json` 附件；发布包与历史清单应放在同一 Release。

## 产物

| 文件 | 平台 |
|---|---|
| `auto_pro_linux_amd64_v1.2.0` | Linux x86_64 |
| `auto_pro_windows_amd64_v1.2.0.exe` | Windows x86_64 |
| `auto_pro_darwin_amd64_v1.2.0` | macOS Intel |
| `auto_pro_darwin_arm64_v1.2.0` | macOS Apple Silicon |

## 编译命令

```bash
cd backend
GOCACHE="$PWD/../.cache/go-build" go build -ldflags="-s -w -X 'main.Version=1.2.0'" -o auto_pro_linux_amd64_v1.2.0 .
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X 'main.Version=1.2.0'" -o auto_pro_windows_amd64_v1.2.0.exe .
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w -X 'main.Version=1.2.0'" -o auto_pro_darwin_amd64_v1.2.0 .
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w -X 'main.Version=1.2.0'" -o auto_pro_darwin_arm64_v1.2.0 .
```
