# v1.2.0 版本说明

## 摘要

自托管软件源站、开发者入驻审核、上传硬校验与 Release 推送；在线更新源改为 GitHub `maizll/auth-pro`；广告与软件源默认本站/空，不连官方。

## 构建

```bash
cd backend
go build -ldflags="-s -w -X 'main.Version=1.2.0'" -o auth_pro_linux_amd64_v1.2.0 .
```

## 默认地址

| 项 | 默认值 |
|---|---|
| 更新清单 | `https://api.github.com/repos/maizll/auth-pro/releases/latest` |
| 广告 | `/api/v1/public/advertisements` |
| 软件源 | 空（可用 `AUTO_PRO_SOFTWARE_SOURCE_URL`） |

详见根目录 `CHANGELOG.md`。
