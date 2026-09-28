#!/usr/bin/env python3
# 宝塔一条命令部署原型。只在测试面板里调用内部类，不是正式安装器。
# 官网地址不在这里下载安装包。已有站点或数据库一律拒绝，不覆盖。
import argparse
import json
import os
import socket
import sys
import time

PANEL = "/www/server/panel"
os.chdir(PANEL)
# class 里有 public.py；面板根目录里有 mod 包。两边都要放进路径。
sys.path.insert(0, PANEL)
sys.path.insert(0, os.path.join(PANEL, "class"))

import public  # noqa: E402
import panelSite  # noqa: E402
import database  # noqa: E402

PORT_START = 19127
PORT_END = 19227
MARKER = "# BEGIN AUTH_PRO_ONECLICK"


def info(text):
    print("[信息] " + text, flush=True)


def die(text, code=1):
    print("[错误] " + text, file=sys.stderr, flush=True)
    raise SystemExit(code)


def manual(title, lines):
    print("[手动] " + title, flush=True)
    for line in lines:
        print("       " + line, flush=True)


def port_open(port):
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.settimeout(0.2)
    try:
        return sock.connect_ex(("127.0.0.1", port)) == 0
    except OSError:
        return False
    finally:
        sock.close()


def registered_ports():
    """已有 auth-pro 站点登记过的后端端口。不扫描、不改其它站点的文件。"""
    found = set()
    roots = []
    try:
        rows = public.M("sites").field("name,path").select()
    except Exception:
        rows = []
    for row in rows or []:
        path = row.get("path") or ""
        if os.path.isfile(os.path.join(path, "backend", "baota.env")) or os.path.isfile(
            os.path.join(path, "backend", "baota-nginx.snippet.conf")
        ):
            roots.append(path)
    vhost = "/www/server/panel/vhost/nginx"
    if os.path.isdir(vhost):
        for name in os.listdir(vhost):
            if not name.endswith(".conf"):
                continue
            # 只认文件名等于某个已标记 auth-pro 站点的配置，避免读其它站点。
            site = name[:-5]
            for path in roots:
                if os.path.basename(path) == site:
                    roots.append(os.path.join(vhost, name))
    for path in roots:
        try:
            text = open(path, encoding="utf-8", errors="replace").read()
        except OSError:
            continue
        for token in text.replace(";", " ").split():
            if token.startswith("PORT="):
                token = token.split("=", 1)[1]
            if token.startswith("127.0.0.1:"):
                token = token.split(":", 1)[1]
            if token.isdigit():
                number = int(token)
                if PORT_START <= number <= PORT_END:
                    found.add(number)
    return found


def choose_port(explicit):
    used = registered_ports()
    if explicit is not None:
        if port_open(explicit) or explicit in used:
            die("端口 %s 已被占用或已写在其它 auth-pro 站点配置里，已停止。请换一个端口，本次没有新建站点。" % explicit)
        return explicit
    for port in range(PORT_START, PORT_END + 1):
        if port in used or port_open(port):
            info("端口 %s 不可用，继续查找" % port)
            continue
        return port
    die("从 %s 到 %s 没有空闲端口，已停止。本次没有新建站点。" % (PORT_START, PORT_END))


def obj(**kwargs):
    get = public.dict_obj()
    for key, value in kwargs.items():
        setattr(get, key, value)
        get[key] = value
    return get


def site_count(name):
    return public.M("sites").where("name=?", (name,)).count()


def rollback_site(name, created):
    if not created:
        return
    row = public.M("sites").where("name=?", (name,)).find()
    if not row:
        return
    info("正在撤掉本次新建的站点 %s，避免留下半成品" % name)
    get = obj(id=str(row["id"]), webname=name, ftp="0", database="0", path=row.get("path") or "")
    try:
        result = panelSite.panelSite().DeleteSite(get)
        info("撤站结果：%s" % result)
    except Exception as exc:
        manual("自动撤站失败，请在面板里删除站点 " + name, [str(exc)])


def rollback_database(name, created):
    if not created:
        return
    row = public.M("databases").where("name=? AND LOWER(type)=LOWER('mysql')", (name,)).find()
    if not row:
        return
    info("正在撤掉本次新建的数据库 %s" % name)
    get = obj(id=str(row["id"]), name=name)
    try:
        result = database.database().DeleteDatabase(get)
        info("撤库结果：%s" % result)
    except Exception as exc:
        manual("自动撤库失败，请在面板数据库里删除 " + name, [str(exc)])


def snippet(port, site_root):
    return """
    %s
    location ^~ /backend/ { return 404; }
    location = /baota-install.sh { return 404; }
    location = /baota-upgrade.sh { return 404; }
    location = /baota-lib.sh { return 404; }
    location = /guardian-start.sh { return 404; }
    location ~* ^/(db\\.json|install\\.lock|jwt\\.secret)$ { return 404; }
    location ~* \\.(log|pid)$ { return 404; }
    error_page 502 503 504 /backend-unavailable.html;
    location = /backend-unavailable.html {
        root %s;
        default_type text/html;
    }
    location = /index.html {
        proxy_pass http://127.0.0.1:%s;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        add_header Cache-Control "no-cache, no-store, must-revalidate" always;
    }
    # END AUTH_PRO_ONECLICK
""" % (MARKER, site_root, port)


def insert_snippet(site, port, site_root):
    path = "/www/server/panel/vhost/nginx/%s.conf" % site
    if not os.path.isfile(path):
        return False, "找不到站点配置 " + path
    original = open(path, encoding="utf-8", errors="replace").read()
    if MARKER in original:
        return True, path
    idx = original.rfind("}")
    if idx < 0:
        return False, "站点配置没有结束括号"
    updated = original[:idx] + snippet(port, site_root) + "\n" + original[idx:]
    backup = path + ".auth-pro-oneclick.bak"
    open(backup, "w", encoding="utf-8").write(original)
    open(path, "w", encoding="utf-8").write(updated)
    code = os.system("/www/server/nginx/sbin/nginx -t >/tmp/auth-pro-nginx-test.txt 2>&1")
    if code != 0:
        open(path, "w", encoding="utf-8").write(original)
        detail = open("/tmp/auth-pro-nginx-test.txt", encoding="utf-8", errors="replace").read()
        return False, detail
    os.system("/www/server/nginx/sbin/nginx -s reload >/tmp/auth-pro-nginx-reload.txt 2>&1 || /etc/init.d/nginx reload >/tmp/auth-pro-nginx-reload.txt 2>&1")
    return True, path


def try_supervisor(site, start_sh, port):
    """进程守护管理器是插件。商店接口要 Flask 请求上下文，脚本安装不走那个接口。"""
    plugin_py = "/www/server/panel/plugin/supervisor/supervisor_main.py"
    if not os.path.isfile(plugin_py):
        manual(
            "还没有进程守护管理器插件。请执行官方安装脚本，但不要用它里面的豆瓣 pip 源。",
            [
                "curl -fsSL -o /tmp/supervisor-install.sh https://download.bt.cn/install/plugin/supervisor/install.sh",
                "btpip install supervisor",
                "echo_supervisord_conf 写入 /etc/supervisor/supervisord.conf 后执行插件 config.py",
                "没有 systemd 时直接运行 pyenv/bin/supervisord，不要调用 systemctl",
                "装不上时改用 systemd 单元，只写本站点，ExecStart=%s" % start_sh,
            ],
        )
        return {"status": False, "mode": "systemd-fallback", "port": port}
    sys.path.insert(0, "/www/server/panel/plugin/supervisor")
    import supervisor_main
    result = supervisor_main.supervisor_main().AddProcess(obj(
        pjname="auth_pro_" + site.split(".")[0].replace("-", "_"),
        user="www",
        path=os.path.dirname(start_sh),
        command=start_sh,
        numprocs="1",
    ))
    info("AddProcess 返回：%s" % result)
    if not isinstance(result, dict) or not result.get("status"):
        manual("进程守护没有加上。请在面板「进程守护管理器」里手动添加，启动命令填 %s" % start_sh, [
            "返回值：%s" % result,
        ])
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("domain")
    parser.add_argument("--port", type=int)
    args = parser.parse_args()
    domain = args.domain.strip().lower()
    site_root = "/www/wwwroot/" + domain
    db_name = "authpro_" + domain.split(".")[0].replace("-", "_")[:8]
    db_user = db_name[:16]
    db_pass = public.GetRandomString(16) if hasattr(public, "GetRandomString") else "Ap" + str(int(time.time()))

    if site_count(domain):
        die("站点 %s 已存在，拒绝覆盖。请在面板里查看这个站点，本次没有修改它。" % domain)
    if os.path.isdir(site_root) and os.listdir(site_root):
        die("目录 %s 已有文件，拒绝覆盖。本次没有修改该目录。" % site_root)

    port = choose_port(args.port)
    info("使用后端端口 %s" % port)
    info("面板版本 %s" % public.version())

    site_api = panelSite.panelSite()
    webname = json.dumps({"domain": domain, "domainlist": [], "count": 0}, ensure_ascii=False)
    created_site = False
    created_db = False
    try:
        result = site_api.AddSite(obj(
            webname=webname,
            path=site_root,
            type_id="0",
            type="PHP",
            version="00",
            port="80",
            ps="auth-pro",
            ftp="false",
            sql="false",
            ftp_username="",
            ftp_password="",
            datauser="",
            datapassword="",
            codeing="utf8mb4",
        ))
        info("AddSite 返回：%s" % result)
        if not isinstance(result, dict) or not result.get("siteStatus"):
            manual("建站失败。请在面板网站里手动添加纯静态站点。", [
                "域名 %s" % domain,
                "根目录 %s" % site_root,
                "不要勾选 FTP 和数据库",
                "返回值：%s" % result,
            ])
            die("建站失败，没有继续建库和反代")
        created_site = True

        db_api = database.database()
        db_result = db_api.AddDatabase(obj(
            name=db_name,
            db_user=db_user,
            password=db_pass,
            address="127.0.0.1",
            codeing="utf8mb4",
            ps="auth-pro",
            sid="0",
        ))
        info("AddDatabase 返回：%s" % db_result)
        if not isinstance(db_result, dict) or not db_result.get("status"):
            manual("建库失败。站点已撤掉。请在面板数据库里手动建库，字符集 utf8mb4，权限 127.0.0.1。", [
                "库名 %s 用户 %s" % (db_name, db_user),
                "返回值：%s" % db_result,
            ])
            rollback_site(domain, True)
            die("建库失败")
        created_db = True

        proxy = site_api.CreateProxy(obj(
            sitename=domain,
            proxyname="auth-pro",
            proxydir="/",
            proxysite="http://127.0.0.1:%s" % port,
            todomain="$host",
            type="1",
            cache="0",
            cachetime="1",
            subfilter="[]",
            advanced="0",
        ))
        info("CreateProxy 返回：%s" % proxy)
        if not isinstance(proxy, dict) or not proxy.get("status"):
            manual("反向代理失败。站点和数据库已撤掉。请在面板里把站点反代到 127.0.0.1:%s 。" % port, [
                "返回值：%s" % proxy,
            ])
            rollback_database(db_name, True)
            rollback_site(domain, True)
            die("反向代理失败")

        ok, detail = insert_snippet(domain, port, site_root)
        info("自定义 Nginx 片段：%s" % detail)
        if not ok:
            manual("反代已添加，但自定义拦截片段没有写入。请把 backend/baota-nginx.snippet.conf 合并进站点 server。", [
                detail[-800:],
            ])

        start_sh = os.path.join(site_root, "backend", "start.sh")
        os.makedirs(os.path.dirname(start_sh), exist_ok=True)
        if not os.path.exists(start_sh):
            open(start_sh, "w", encoding="utf-8").write("#!/bin/bash\n# 原型占位，正式安装由 baota-install.sh 写入\n")
            os.chmod(start_sh, 0o755)
        guard = try_supervisor(domain, start_sh, port)
        info("进程守护：%s" % guard)

        # 证书只验证调用路径。没有公网域名时期望失败。
        try:
            import acme_v2
            row = public.M("sites").where("name=?", (domain,)).find()
            ssl = acme_v2.acme_v2().apply_cert_api(obj(
                id=str(row["id"]),
                auth_type="http",
                auth_to=site_root,
                domains=json.dumps([domain]),
            ))
            info("apply_cert_api 返回：%s" % ssl)
        except Exception as exc:
            info("apply_cert_api 异常：%s" % exc)
            manual("Let's Encrypt 没有申请成功。站点仍可用 HTTP。请在面板里对真实域名申请证书。", [str(exc)])

        print("----")
        print("网址: http://%s/" % domain)
        print("管理员账号: 安装向导完成后打印（本原型不写 install.lock）")
        print("后端端口: %s" % port)
        print("数据库: %s 用户 %s 密码 %s 主机 127.0.0.1" % (db_name, db_user, db_pass))
    except SystemExit:
        raise
    except Exception as exc:
        manual("执行中断。正在撤掉本次新建的站点和数据库。", [str(exc)])
        rollback_database(db_name, created_db)
        rollback_site(domain, created_site)
        raise


if __name__ == "__main__":
    main()
