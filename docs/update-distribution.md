# 在线更新分发（内部）

这份说明只给维护源站的人。不要放进官网文档。

## 现在怎么更新

1.7.1 及以后的客户站把更新地址写死为：

```text
https://auth.maizll.com/api/v1/update/latest.json
https://auth.maizll.com/api/v1/update/releases.json
https://auth.maizll.com/api/v1/update/package/<版本号>
```

客户站后台不能改这个地址。环境变量 `AUTO_PRO_UPDATE_URL` 不再起作用。旧数据库里如果存过代码托管站的更新地址，启动迁移会删掉。

源站接口把清单里的下载地址改写成上面的源站地址，SHA256、大小和签名原样保留。安装包按版本缓存在数据目录 `update-cache/<版本>/`。`latest.json` 和 `releases.json` 缓存大约 3 分钟。同一 IP 每分钟最多 30 次清单请求、6 次安装包下载。

## 令牌

不要新建第二套令牌。源站按这个顺序使用已经保存的 GitHub 令牌：

1. 软件源设置里的收费仓库令牌
2. Release / 仓库设置里的 GitHub 令牌
3. 不带令牌再试一次

默认仓库是私有的，没有令牌拉不到包。上面两处至少要有一枚对该仓库有读取权限的令牌（经典令牌的 `repo`，或细粒度令牌的 Contents 读）。只有把环境变量改到一个仍公开的仓库时，最后一步匿名才有用。

## 仓库名

官网分发接口默认从客户交付仓库取 Release：

```text
maizll/auth-pro-client
```

源站服务器可以用环境变量改掉，格式是 `owner/repo`：

```text
AUTO_PRO_UPDATE_REPOSITORY=owner/repo
```

写成别的格式时，接口返回「更新仓库未配置」，响应里不回显这段配置。`maizll/auth-pro` 和官网仓库 `maizll/auth-pro-server` 都不作为客户安装包来源。1.7.1 只进入 `auth-pro-client`，不合并进另外两个仓库。

## 两个仓库怎么更新

客户站（`auth-pro-client`）从 1.7.1 起只向 `https://auth.maizll.com` 要更新。官网（`auth-pro-server`）停留在 1.7.0，继续按自己的仓库地址更新，不带这套分发改动。

1.7.0 及更早、仍指向 `maizll/auth-pro` 的客户站，不会自动从 `auth-pro-client` 的私有 Release 升上来。它们要先装上 1.7.1 的客户包，之后才走官网更新。
