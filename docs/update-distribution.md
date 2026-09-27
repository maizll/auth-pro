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

仓库还是公开的时候，没有令牌也能拉。改成私有之后，上面两处至少要有一枚对该仓库有读取权限的令牌（经典令牌的 `repo`，或细粒度令牌的 Contents 读）。

## 仓库名

拉取用的仓库不写进程序，只在源站服务器上配置：

```text
AUTO_PRO_UPDATE_REPOSITORY=owner/repo
```

当前产品仓库应写成 `maizll/auth-pro`。不设这个变量时，分发接口返回「更新仓库未配置」，响应里没有仓库地址。不要把这个默认值写进程序，否则发行二进制里又能搜到。

## 什么时候才能把仓库改私有

1.7.0 和更早的客户站仍然直接读取 GitHub Release。`release.yml` 继续把安装包发到 GitHub Release，这样旧站还能升到 1.7.1。

所有客户都升到 1.7.1 之后，才能把仓库改成私有。改私有后，源站必须配好上面的仓库名，以及一枚有读取权限的令牌，否则新客户站会检查更新失败。
