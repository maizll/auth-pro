# 在线更新分发（内部）

这份说明只给维护源站的人。不要放进官网文档。

同一份 1.7.1 发两次，版本号相同。一次在 `maizll/auth-pro-client`，官网的分发接口从这里取客户包。一次同步到 `maizll/auth-pro` 的 Release。官网自己从后者更新。1.7.0 及更早的客户也只认识后者，所以他们从那里一次升到 1.7.1。老客户都升完以后，`maizll/auth-pro` 改为私有。改成私有之后，官网仍用已经保存的令牌继续更新。

## 怎么认出官网

官网和客户站是同一份程序。数据目录 `store/snapshot-ed25519.key` 的前 32 字节当种子重算公钥，必须等于编译进程序的公钥 `pwAizm/sOyWCu+qi8+Dl/xJr0Upuamh5u7vL3wGT14A=`，文件后 32 字节也必须是这个公钥，这台机器才算官网。不能只看后 32 字节：那一段可以照抄公开的公钥。文件不存在、读失败、内容损坏，或公钥对不上，都按客户站处理。只看到文件还不够。

## 客户站

1.7.1 及以后、没有这把私钥的站点只向下面三个地址要更新：

```text
https://auth.maizll.com/api/v1/update/latest.json
https://auth.maizll.com/api/v1/update/releases.json
https://auth.maizll.com/api/v1/update/package/<版本号>
```

后台不能改地址。环境变量 `AUTO_PRO_UPDATE_URL` 不再起作用。数据库里如果还存着旧的更新地址（含 `api.github.com/repos/`、`github.com/*/releases/`、`raw.githubusercontent.com/`），进程启动时会删掉。

## 官网自己更新

不设环境变量时，官网从 `maizll/auth-pro` 的 Release 取 `latest.json`。服务器上可以用 `AUTO_PRO_OFFICIAL_UPDATE_REPOSITORY` 改成别的 `owner/repo`。格式不对时接口只返回「更新仓库未配置」，不回显这段配置。

令牌不另建。顺序和分发客户包相同：软件源设置里的收费仓库令牌，然后是 Release 设置里的令牌，最后才匿名。仓库改为私有后，匿名会失败，前两处已保存的令牌仍然有效。

后台拿到的清单会去掉下载地址。页面上看不到仓库地址。

## 分发给客户

1.7.2 起，上面三个地址只读官网应用「授权系统」（`app_f93896d80066_5811`）的发布版本。不再在对外接口里连接代码托管站。下载地址一律写成 `https://auth.maizll.com/api/v1/update/package/<版本号>`，清单里的 SHA256 和大小按磁盘上的安装包计算，`signature` 仍是 `sha256:` 加上同一个哈希。文件名仍是 `auth_pro-full-v<版本>.tar.gz`。说明里如果带有托管站地址，会在返回前删掉。

`latest.json` 和 `releases.json` 缓存大约 3 分钟，保存发布版本后清掉。同一 IP 每分钟最多 30 次清单请求、6 次安装包下载。

后台「发布版本」里的「从仓库导入」只读该应用已经绑定的私有仓库，不再使用写死的默认仓库。`AUTO_PRO_UPDATE_REPOSITORY` 只在官网升级迁入客户安装包时作为来源，不改变客户站请求的地址，也不拿去改官网自己的更新来源。开发者端不挂这组导入接口。开发者上传的收费包仍由本站用站长令牌写入该应用绑定的仓库。登记外链时只从公开 https 下载，下载请求不带站长令牌。

## 老客户怎么升上来

1.7.0 默认读取 `maizll/auth-pro` 最新 Release 里的 `latest.json`。清单字段没有改：安装包地址在该仓库的 Release 下，`sha256` 是包的哈希，`signature` 是 `sha256:` 加上同一个哈希，文件名是 `auth_pro-full-v<版本>.tar.gz`。在 `maizll/auth-pro` 上打 1.7.1 的标签后，1.7.0 按这套规则安装。安装时它还会向该仓库核对附件摘要，摘要就是这个包的 SHA256。

装上 1.7.1 之后，没有签名私钥的站点自动只向官网要更新，客户不用改配置。
