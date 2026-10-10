# Docker 部署

与 [交付说明](交付说明.md) 中的**宝塔路径并列**。适合已熟悉容器、希望 MariaDB 与应用一起编排的环境。

- **不要**把 Compose 的数据卷挂到正在使用的宝塔网站根（例如 `/www/wwwroot/你的域名`）。
- **不要**让容器与宝塔进程监听同一宿主机端口。
- 宝塔用官网 `install.sh`；Docker 用本目录的 Compose。同一域名同一时刻只跑一种方式。

仓库内文件：

| 路径 | 作用 |
| --- | --- |
| `deploy/docker/Dockerfile` | 多阶段构建应用镜像（前端 + Linux amd64 后端） |
| `deploy/docker/docker-compose.yml` | MariaDB + 应用 |
| `deploy/docker/.env.example` | 口令与端口模板 |

## 1. 端口

| 用途 | 容器内 | 默认宿主机映射 | 说明 |
| --- | --- | --- | --- |
| 应用 HTTP | `19127` | `19127`（可用环境变量 `AUTH_PRO_PORT` 改） | 浏览器访问 `http://服务器IP:端口`；生产建议前面再加 HTTPS 反代 |
| MariaDB | `3306` | **不映射到公网** | 仅 compose 网络内供应用访问 |

应用监听地址使用默认 `HOST=0.0.0.0`（容器内需对外网卡开放）。宝塔脚本里才会写成 `127.0.0.1`，两套不要混用同一份 `baota.env`。

## 2. 数据卷

| 卷名（compose） | 挂载点 | 必须保留的内容 |
| --- | --- | --- |
| `auth_pro_data` | `/data` | `db.json`、`install.lock`、`jwt.secret`、插件、模板、更新备份、日志、上传等运行数据（对应环境变量 `AUTO_PRO_DATA_DIR`） |
| `auth_pro_db` | MariaDB 数据目录 | 业务库全部表数据 |

升级或重建**应用容器**时，只要这两个卷还在，管理员账号与业务数据就还在。不要手动删卷，除非确认要清空。

前端静态文件与二进制打在镜像里（`/app`）。换版本 = 换镜像；运行数据仍在 `/data`。

## 3. 你必须填写的配置

```bash
cd deploy/docker
cp .env.example .env
# 编辑 .env：改掉所有「请改成…」的口令，并保证
# AUTO_PRO_DB_PASSWORD 与 MYSQL_PASSWORD 一致
```

必填项：

| 变量 | 含义 |
| --- | --- |
| `MYSQL_ROOT_PASSWORD` | MariaDB root 口令 |
| `MYSQL_DATABASE` / `MYSQL_USER` / `MYSQL_PASSWORD` | 业务库名与账号 |
| `AUTO_PRO_DB_*` | 应用连接库；主机名必须是 compose 服务名 **`db`**，不要写 `127.0.0.1` |
| `AUTH_PRO_PORT` | 宿主机映射端口，默认 `19127` |

可选：`TZ=Asia/Shanghai`。

将 `.env` 权限收紧（例如 `chmod 600 .env`），不要提交到版本库或发给无关人员。

## 4. 启动

在**仓库根目录**执行（构建上下文是整仓，以便编译前端与后端）：

```bash
docker compose -f deploy/docker/docker-compose.yml --env-file deploy/docker/.env up -d --build
```

查看日志：

```bash
docker compose -f deploy/docker/docker-compose.yml --env-file deploy/docker/.env logs -f app
```

首次打开 `http://<主机>:<AUTH_PRO_PORT>/`：

1. 若尚未安装：进入安装向导。
2. 数据库主机填 **`db`**，端口 `3306`，库名/用户/密码与 `.env` 中一致（与 `AUTO_PRO_DB_*` 相同即可）。
3. 创建超级管理员并保存密码。
4. 安装完成后会出现 `install.lock`（位于数据卷 `/data`）。

即使已通过环境变量注入数据库连接，向导仍会走建表与创建管理员步骤；库里已有业务数据时会拒绝重复初始化。

## 5. 升级步骤（Docker）

目标：换新版本程序，**保留** `auth_pro_data` 与 `auth_pro_db`。

1. 备份：对 MariaDB 做一次 `mysqldump`（或停写后拷贝卷）；并备份数据卷中的 `db.json`、`install.lock`、`jwt.secret`。
2. 更新代码或安装包来源到目标版本（本仓库保持 `VERSION` 与要部署的版本一致后重新 build；或按发版说明更换镜像标签）。
3. 重新构建并滚动应用容器：

```bash
docker compose -f deploy/docker/docker-compose.yml --env-file deploy/docker/.env up -d --build app
```

4. 看日志与 `http://127.0.0.1:<端口>/api/system/version`（在宿主机上）确认新版本。
5. **不要**对数据卷执行 `docker compose down -v`（带 `-v` 会删卷）。

### 与「在线更新」的关系

- **宝塔站点**：优先用后台「在线更新」或官网 `install.sh upgrade`。
- **Docker**：以**换镜像 / 重新 build**为准。容器内若执行在线更新，改动落在可写层；一旦重建容器且未把程序目录做成卷，这些改动会丢失。因此 Docker 路径不要依赖在线更新作为唯一升级手段。

## 6. 备份与恢复（Docker）

示例（容器名以 `docker compose ps` 为准）：

```bash
# 导出数据库
docker compose -f deploy/docker/docker-compose.yml --env-file deploy/docker/.env exec -T db \
  mysqldump -u root -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE" > backup-$(date +%Y%m%d).sql

# 备份运行数据卷内容（示例：把卷挂到临时容器打包）
docker run --rm -v auth-pro_auth_pro_data:/data -v "$(pwd)":/out debian:bookworm-slim \
  tar -C /data -czf /out/auth-pro-data-$(date +%Y%m%d).tar.gz .
```

恢复时先恢复 SQL 与 `/data` 文件，再启动 compose；保留 `install.lock`。

## 7. HTTPS

Compose 默认只暴露应用端口。生产环境请在宿主机或另一容器用 Nginx/Caddy 做 HTTPS 反代到 `127.0.0.1:AUTH_PRO_PORT`，并设置 `Host`、`X-Forwarded-Proto: https`。商业版绑定同样要求用正式 https 域名访问后台。

## 8. 构建说明（维护者）

`Dockerfile` 多阶段：Node 构建前端 → Go 交叉编译 `linux/amd64` → 精简运行镜像。构建参数：

| 参数 | 默认 | 含义 |
| --- | --- | --- |
| `AUTH_PRO_VERSION` | 读取构建上下文中的 `VERSION` 文件，否则 `1.9.1` | 写入二进制与前端版本 |

运行用户非 root；数据目录 `/data` 可写。健康检查请求 `GET /api/install/status`。

## 9. 用官网发布包构建镜像（可选）

若本机已有 `auth_pro-full-vX.Y.Z.tar.gz`（例如放在 `release/packages/`），可不编译前端，直接解压进镜像：

```bash
docker build -f deploy/docker/Dockerfile \
  --target from-package \
  --build-arg PACKAGE_PATH=release/packages/auth_pro-full-v1.9.1.tar.gz \
  -t auth-pro:1.9.1 \
  .
```

然后将 compose 里 `app.build.target` 改为 `from-package`，或直接 `image: auth-pro:1.9.1` 并去掉 build 段。数据卷与 `.env` 约定不变。
