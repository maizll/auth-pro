# v1.4.0 版本说明

## 摘要

四端统一站内通知中心：管理后台、用户端、代理端、开发者端顶栏共用真实铃铛，按 JWT 身份隔离，不再使用演示 mock。

## 构建

打 `v1.4.0` 标签后由 `.github/workflows/release.yml` 注入 `auto_pro/config.AppVersion`：

```bash
git tag v1.4.0
git push origin v1.4.0
```

本地核对：

```bash
cd backend
go test ./handler -run TestNotification
```

## 接口

| 项 | 值 |
|---|---|
| 列表 | `GET /api/v1/notifications?tab=notice\|message\|todo&unread=1` |
| 未读 | `GET /api/v1/notifications/unread-count` |
| 已读 | `POST /api/v1/notifications/:id/read`、`POST /api/v1/notifications/read-all` |

详见根目录 `CHANGELOG.md` 与 `docs/release-notes-1.4.0.txt`。
