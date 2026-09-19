# 更新日志

## [1.2.0] - 2026-09-20

### 新增

- 自托管软件源站（源站）能力：插件/首页模板目录、多版本、上架审核流。
- 代理商面板「开发者入驻」；管理端审核通过/拒绝；取消开发者资格（硬删除，用户名可再申请）。
- 管理端上传包硬校验（`plugin.json` / 模板清单不合规直接拒绝）。
- Release 推送（GitHub/Gitee）与「测试连接」；严格遵循是否勾选推送（不勾选必须填外部下载地址）。

### 变更

- 在线更新源默认改为 GitHub：`https://api.github.com/repos/maizll/auth-pro/releases/latest`（可用 `AUTO_PRO_UPDATE_URL` 覆盖）。
- 广告源默认 `https://auth.maizll.com/api/v1/public/advertisements`（不连官方 91ani）。本地覆盖：`AUTO_PRO_ADVERTISEMENT_URL=/api/v1/public/advertisements`。
- 软件源默认 `https://auth.maizll.com/software-source`（基路径，不是 `index.json`；不连官方 `plug.91ani.cn`）。设 `AUTO_PRO_SOFTWARE_SOURCE_URL=-` 可关闭；本地覆盖：`AUTO_PRO_SOFTWARE_SOURCE_URL=http://127.0.0.1:19127/software-source`。默认集成源不拼接 `/admin/`，旧 `/admin/app-store` 仍落到本站 `/plugin-store`。
- 产品版本号调整为 `1.2.0`。

### 修复/体验

- 目录操作按钮按状态显示（草稿/待审仅通过·驳回；通过后才可上架等）。
- 管理员重传覆盖并回到待审核；版本状态与目录项对齐，避免「目录项不存在」。
- 入驻申请审核后从列表删除，列表默认仅待审核。
- 在线更新正确解析 GitHub `maizll/auth-pro` Releases：优先 `latest.json`，否则用 `tag_name` 与更新包附件合成清单；历史走 `/releases?per_page=30`，不再拼接不存在的 `releases.json`。

## [源站] 2026-09-19 — 自托管软件源与仓库对接

### 新增

- 本实例可作为自托管软件源（源站）：公开 `GET /software-source/index.json`（`plugins` + `homeTemplates`）。
- 管理后台侧栏「源站」：入驻审核、插件/首页模板、目录快照、广告、Release 仓库设置。
- 上传 ZIP 硬校验（必须含合规 `plugin.json` / `template.json`），失败不写库、不推 Release、不留临时文件。
- 校验通过后自动填表；可推送 GitHub/Gitee Release，源站只保存元数据 + https 下载地址 + SHA256（不存插件源码）。
- 多版本发布、latest、回滚、上架/下架；下架不等于远程卸载。
- 未配置远程软件源时回退本站目录/内置模板，不再整页报「未配置远程软件源」。

### 对接 GitHub / Gitee 仓库（必读）

目标：后台上传合规 ZIP → 自动校验填表 → 推到仓库 Release → 源站只保存附件 https 地址与 SHA256。

#### 1. 准备发包仓库

- 在 Gitee 或 GitHub 新建仓库（如 `yourname/auth-pro-packages`）。
- 建议单独用作「附件柜」，与日常开发仓分离亦可。

#### 2. 创建 Token

**Gitee**：设置 → 安全设置 → 私人令牌；勾选仓库与发行版（projects / release）相关权限；复制令牌（只显示一次）。

**GitHub**：Settings → Developer settings → Personal access tokens；建议 Fine-grained 仅授权目标仓；Contents + Releases 读写（或经典令牌勾选 `repo`）。

#### 3. 后台填写

1. 管理员登录后台。
2. 打开 **源站 → Release / 仓库 Token**。
3. 填写：
   - Provider：`gitee` 或 `github`
   - Owner：用户名或组织名
   - 仓库：仓库名（不要带 `.git`）
   - Token：私人令牌（仅服务端保存，再次打开只显示掩码）
   - Tag 策略：默认 `{id}-{version}`（如 `demo-plugin-1.0.0`）
   - Gitee 时填写分支（多为 `master` 或 `main`）
4. 保存，状态变为「已配置」即可。

#### 4. 上传发包

1. ZIP 根目录或一层子目录必须有 `plugin.json`（模板为 `template.json`），缺字段直接拒收。
2. **源站 → 插件管理** 上传 ZIP。
3. 系统：硬校验 → 自动填表 →（已配仓库则）创建/更新 Release 并上传附件 → 只把 `downloadUrl` + `sha256` 写入目录。
4. 审核 → 上架后公开目录：`http://<源站>/software-source/index.json`  
   未配仓库时仍可解析入库，但需自行粘贴外部 https 下载地址。

#### 5. 消费端使用

在「软件源管理」添加：`http://<源站主机:端口>/software-source/index.json`  
本机示例：`http://127.0.0.1:19127/software-source/index.json`

#### 6. 常见问题

- Token 权限不足 → 推 Release 失败。
- Owner/仓库名错误 → 上传推送报错。
- ZIP 无清单或字段非法 → 拦截，不写库、不推 Release。
- 私有仓 Release 无法被消费端匿名下载 → 安装失败（附件链接须可访问）。
- 同一 `{id}-{version}` 重复发版通常更新同一 Release 附件，勿随意打乱版本号规则。

### 说明

- 源站默认不连接官方 `plug.91ani.cn`。
- 包规范：`GET /software-source/package-schema.json`。
- 下次发布在线更新包时，可将 `docs/release-notes-source-station.txt` 写入 `latest.json` / `releases.json` 的 `notes`，供后台「在线更新」页展示。