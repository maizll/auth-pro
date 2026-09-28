#!/usr/bin/env python3
# 宝塔面板内部调用。只给 baota-install.sh 用，不要单独当安装器。
# 输入是子命令和参数。标准输出只给调用方捕获的 AUTH_PRO_* 键值，操作者看不到。
# 中文说明走标准错误。失败时退出码非 0，并且不删除调用方没有点名的站点、目录或数据库。
"""宝塔 13 面板内部建站、建库、反代、进程守护和证书。"""

import argparse
import json
import os
import shutil
import subprocess
import sys

PANEL = "/www/server/panel"
NGINX = "/www/server/nginx/sbin/nginx"
MARKER_BEGIN = "# BEGIN AUTH_PRO_ONECLICK"
MARKER_END = "# END AUTH_PRO_ONECLICK"
OWNER_FILE = ".auth-pro-oneclick"


def emit(key, value):
    """写给 baota-lib.sh 捕获的一行。值里不能有换行。不要把这一行当成给操作者看的说明。"""
    text = "" if value is None else str(value)
    text = text.replace("\r", " ").replace("\n", " ")
    print("%s=%s" % (key, text), flush=True)


def info(text):
    print("[信息] " + text, file=sys.stderr, flush=True)


def die(text, code=1):
    """失败即停。调用方根据退出码决定要不要撤掉本次新建的资源。"""
    print("[错误] " + text, file=sys.stderr, flush=True)
    raise SystemExit(code)


def manual(title, lines):
    """自动步骤失败时留给操作者的中文说明。不代替回滚。"""
    print("[手动] " + title, file=sys.stderr, flush=True)
    for line in lines:
        print("       " + line, file=sys.stderr, flush=True)


def bootstrap():
    """进入面板目录再导入。class 提供 public，面板根目录提供 mod。"""
    if not os.path.isdir(PANEL):
        die("未检测到宝塔面板（没有 %s）。请先安装宝塔面板，并在软件商店安装 Nginx 和 MySQL 后再执行。脚本不会替你安装这些软件。" % PANEL)
    os.chdir(PANEL)
    if PANEL not in sys.path:
        sys.path.insert(0, PANEL)
    classes = os.path.join(PANEL, "class")
    if classes not in sys.path:
        sys.path.insert(0, classes)


def obj(**kwargs):
    """面板方法要 public.dict_obj()，既支持属性也支持下标。"""
    import public

    get = public.dict_obj()
    for key, value in kwargs.items():
        setattr(get, key, "" if value is None else str(value))
        get[key] = "" if value is None else str(value)
    return get


def load_public():
    bootstrap()
    import public

    return public


def preflight(_args):
    """确认面板、Nginx 和面板 MySQL 都已经能用。缺任何一个就退出，不安装 MySQL。"""
    public = load_public()
    if not os.path.isfile(NGINX):
        die("未检测到面板 Nginx（没有 %s）。请先在宝塔软件商店安装 Nginx，然后再执行本命令。脚本不会替你安装。" % NGINX)
    try:
        import db_mysql

        mysql = db_mysql.panelMysql()
        rows = mysql.query("SELECT 1 AS ok")
    except Exception as exc:
        die("面板 MySQL 未安装或无法连接（%s）。请先在宝塔软件商店安装并启动 MySQL，然后再执行本命令。脚本不会替你安装 MySQL。" % exc)
    if rows in (False, None) or (isinstance(rows, str) and rows):
        die("面板 MySQL 查询失败：%s。请先在软件商店确认 MySQL 已安装并启动。脚本不会替你安装 MySQL。" % rows)
    emit("AUTH_PRO_RESULT", "ok")
    try:
        emit("AUTH_PRO_PANEL_VERSION", public.version())
    except Exception:
        emit("AUTH_PRO_PANEL_VERSION", "")


def check_site(args):
    """站点名或非空目录已存在时拒绝。不会改目录里的文件。"""
    public = load_public()
    domain = args.domain.strip().lower()
    path = os.path.abspath(args.path)
    try:
        count = public.M("sites").where("name=?", (domain,)).count()
    except Exception as exc:
        die("无法查询面板站点列表：%s" % exc)
    if count:
        die("站点 %s 已存在，拒绝覆盖。请在面板网站列表里查看它。本次没有修改该站点，也没有删除任何文件。" % domain)
    if os.path.isdir(path):
        names = [name for name in os.listdir(path) if name not in (".", "..")]
        if names:
            die("目录 %s 已有文件，拒绝覆盖。本次没有修改该目录。" % path)
    emit("AUTH_PRO_RESULT", "ok")


def check_database(args):
    """同名库或用户已在面板登记时拒绝，避免 AddDatabase 碰到已有对象。"""
    public = load_public()
    name = args.name.strip().lower()
    user = args.user.strip()
    try:
        by_name = public.M("databases").where("name=?", (name,)).count()
        by_user = public.M("databases").where("username=?", (user,)).count()
    except Exception as exc:
        die("无法查询面板数据库列表：%s" % exc)
    if by_name:
        die("数据库 %s 已存在，拒绝覆盖。本次没有删除或重建它。" % name)
    if by_user:
        die("数据库用户 %s 已存在，拒绝覆盖。本次没有删除或重建它。" % user)
    emit("AUTH_PRO_RESULT", "ok")


def add_site(args):
    """建纯静态站点，不建 FTP、不建库。成功后写标记，回滚时只认这个标记。"""
    public = load_public()
    import panelSite
    domain = args.domain.strip().lower()
    path = os.path.abspath(args.path)
    webname = json.dumps({"domain": domain, "domainlist": [], "count": 0}, ensure_ascii=False)
    try:
        result = panelSite.panelSite().AddSite(obj(
            webname=webname,
            path=path,
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
    except Exception as exc:
        manual("建站失败。请在面板网站里手动添加纯静态站点，不要勾选 FTP 和数据库。", [
            "域名 %s" % domain,
            "根目录 %s" % path,
            "原因：%s" % exc,
        ])
        die("建站失败，没有继续建库和反代")
    info("AddSite 返回：%s" % result)
    if not isinstance(result, dict) or not (result.get("siteStatus") or result.get("status")):
        manual("建站失败。请在面板网站里手动添加纯静态站点，不要勾选 FTP 和数据库。", [
            "域名 %s" % domain,
            "根目录 %s" % path,
            "返回值：%s" % result,
        ])
        die("建站失败，没有继续建库和反代")
    os.makedirs(path, exist_ok=True)
    marker = os.path.join(path, OWNER_FILE)
    with open(marker, "w", encoding="utf-8") as handle:
        handle.write(domain + "\n")
    row = public.M("sites").where("name=?", (domain,)).find()
    site_id = ""
    if isinstance(row, dict):
        site_id = row.get("id") or ""
    emit("AUTH_PRO_RESULT", "ok")
    emit("AUTH_PRO_SITE_ID", site_id)


def add_database(args):
    """建 utf8mb4 库，账号只允许 127.0.0.1。用户名须不超过 16 字节，以兼容 MariaDB。"""
    load_public()
    import database
    name = args.name.strip().lower()
    user = args.user.strip()
    if len(user.encode("utf-8")) > 16 or len(name.encode("utf-8")) > 64:
        die("数据库名或用户名过长，已停止。本次没有创建数据库。")
    try:
        result = database.database().AddDatabase(obj(
            name=name,
            db_user=user,
            password=args.password,
            address="127.0.0.1",
            codeing="utf8mb4",
            ps="auth-pro",
            sid="0",
        ))
    except Exception as exc:
        manual("建库失败。请在面板数据库里手动建库，字符集 utf8mb4，访问权限 127.0.0.1。", [
            "库名 %s 用户 %s" % (name, user),
            "原因：%s" % exc,
        ])
        die("建库失败")
    info("AddDatabase 返回：%s" % result)
    if not isinstance(result, dict) or not result.get("status"):
        manual("建库失败。请在面板数据库里手动建库，字符集 utf8mb4，访问权限 127.0.0.1。", [
            "库名 %s 用户 %s" % (name, user),
            "返回值：%s" % result,
        ])
        die("建库失败")
    emit("AUTH_PRO_RESULT", "ok")


def created_dir_ok(path, domain):
    """只有本次写入的标记，并且目录就在 /www/wwwroot 下，才允许删除。"""
    real = os.path.realpath(path)
    root = os.path.realpath("/www/wwwroot")
    if real == root or not real.startswith(root + os.sep):
        return False
    marker = os.path.join(real, OWNER_FILE)
    if not os.path.isfile(marker):
        return False
    try:
        text = open(marker, encoding="utf-8").read().strip()
    except OSError:
        return False
    return text == domain.strip().lower()


def rollback(args):
    """只撤本次新建的站点和库。移动或删除失败时打印手工步骤，不碰其它站点。"""
    public = load_public()
    domain = args.domain.strip().lower()
    if args.remove_db and args.db_name:
        row = public.M("databases").where("name=?", (args.db_name,)).find()
        if isinstance(row, dict) and row.get("id"):
            info("正在撤掉本次新建的数据库 %s" % args.db_name)
            try:
                import database

                result = database.database().DeleteDatabase(obj(id=str(row["id"]), name=args.db_name))
                info("撤库结果：%s" % result)
            except Exception as exc:
                manual("自动撤库失败，请在面板数据库里删除本次新建的库 %s（不要删其它库）。" % args.db_name, [str(exc)])
    if args.remove_site:
        row = public.M("sites").where("name=?", (domain,)).find()
        if isinstance(row, dict) and row.get("id"):
            info("正在撤掉本次新建的站点 %s" % domain)
            try:
                import panelSite

                result = panelSite.panelSite().DeleteSite(obj(
                    id=str(row["id"]),
                    webname=domain,
                    ftp="0",
                    database="0",
                    path=row.get("path") or args.path,
                ))
                info("撤站结果：%s" % result)
            except Exception as exc:
                manual("自动撤站失败，请在面板网站里删除本次新建的站点 %s（不要删其它站点）。" % domain, [str(exc)])
        if args.path and os.path.isdir(args.path):
            if created_dir_ok(args.path, domain):
                real = os.path.realpath(args.path)
                # 宝塔会给 .user.ini 加不可变属性，直接 rmtree 会 EPERM。先去掉再删本次目录。
                subprocess.run(["chattr", "-R", "-i", real], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                try:
                    shutil.rmtree(real)
                    info("已删除本次新建的网站目录 %s" % real)
                except OSError as exc:
                    manual("自动删除网站目录失败。请先执行 chattr -R -i %s ，再手工删除这个目录。不要删其它站点。" % real, [str(exc)])
            else:
                manual("未删除网站目录 %s 。没有本次安装标记，或目录不在 /www/wwwroot 下。" % args.path, [
                    "请确认里面没有其它站点的文件后，再在面板里手工删除。",
                ])
    emit("AUTH_PRO_RESULT", "ok")


def nginx_test():
    proc = subprocess.run([NGINX, "-t"], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    return proc.returncode == 0, proc.stdout


def nginx_reload():
    proc = subprocess.run([NGINX, "-s", "reload"], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    if proc.returncode == 0:
        return True
    init_script = "/etc/init.d/nginx"
    if os.path.isfile(init_script):
        proc = subprocess.run([init_script, "reload"], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        return proc.returncode == 0
    return False


def insert_snippet(domain, snippet_path):
    """把片段插到站点主配置最后一个 } 之前。nginx -t 失败就还原这一份配置。"""
    conf = "/www/server/panel/vhost/nginx/%s.conf" % domain
    if not os.path.isfile(conf):
        return False, "找不到站点配置 " + conf
    if not os.path.isfile(snippet_path):
        return False, "找不到片段文件 " + snippet_path
    original = open(conf, encoding="utf-8", errors="replace").read()
    if MARKER_BEGIN in original:
        return True, conf
    body = open(snippet_path, encoding="utf-8", errors="replace").read().strip()
    block = "\n    %s\n%s\n    %s\n" % (MARKER_BEGIN, body, MARKER_END)
    idx = original.rfind("}")
    if idx < 0:
        return False, "站点配置没有结束括号"
    updated = original[:idx] + block + original[idx:]
    open(conf, "w", encoding="utf-8").write(updated)
    ok, detail = nginx_test()
    if not ok:
        open(conf, "w", encoding="utf-8").write(original)
        return False, "nginx -t 未通过，已还原该站点配置。\n" + detail[-1200:]
    if not nginx_reload():
        info("nginx -t 已通过，但 reload 失败。请手工执行 nginx -s reload。")
    return True, conf


def create_proxy(args):
    """反代到本机端口，再写入拦截片段。片段写失败只还原配置，不在这里删站点。"""
    load_public()
    import panelSite
    domain = args.domain.strip().lower()
    try:
        result = panelSite.panelSite().CreateProxy(obj(
            sitename=domain,
            proxyname="auth-pro",
            proxydir="/",
            proxysite="http://127.0.0.1:%s" % args.port,
            todomain="$host",
            type="1",
            cache="0",
            cachetime="1",
            subfilter="[]",
            advanced="0",
        ))
    except Exception as exc:
        manual("反向代理失败。请在面板里把站点 %s 反代到 http://127.0.0.1:%s 。" % (domain, args.port), [str(exc)])
        die("反向代理失败")
    info("CreateProxy 返回：%s" % result)
    if not isinstance(result, dict) or not result.get("status"):
        manual("反向代理失败。请在面板里把站点 %s 反代到 http://127.0.0.1:%s 。" % (domain, args.port), [
            "返回值：%s" % result,
        ])
        die("反向代理失败")
    ok, detail = insert_snippet(domain, args.snippet)
    if not ok:
        manual("反代已添加，但自定义 Nginx 片段没有留在配置里（失败时已还原该站点配置）。请把下面这个文件合并进站点 server。", [
            args.snippet,
            detail[-800:],
        ])
        emit("AUTH_PRO_RESULT", "snippet-failed")
        return
    emit("AUTH_PRO_RESULT", "ok")
    emit("AUTH_PRO_NGINX", detail)


def btpip():
    for candidate in (
        os.path.join(PANEL, "pyenv/bin/btpip"),
        os.path.join(PANEL, "pyenv/bin/pip3"),
        os.path.join(PANEL, "pyenv/bin/pip"),
    ):
        if os.path.isfile(candidate):
            return candidate
    return ""


def ensure_supervisord():
    """用面板自带的 pip 从 PyPI 装 supervisor。不用豆瓣源，也不走 panelPlugin。"""
    binary = os.path.join(PANEL, "pyenv/bin/supervisord")
    if os.path.isfile(binary):
        return binary
    pip = btpip()
    if not pip:
        return ""
    info("正在用面板 Python 从 PyPI 安装 supervisor")
    proc = subprocess.run([pip, "install", "supervisor"], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    if proc.returncode != 0 or not os.path.isfile(binary):
        info(proc.stdout[-800:])
        return ""
    return binary


def systemd_text(domain, workdir, command):
    unit = "auth-pro-%s" % domain.replace(".", "-")
    return """面板进程守护没有加上。下面这份 systemd 单元只包含本站点，不会改其它服务。
确认系统有 systemd 后，可保存为 /etc/systemd/system/%s.service ，再执行 systemctl daemon-reload && systemctl enable --now %s

[Unit]
Description=auth-pro %s
After=network.target

[Service]
Type=simple
User=www
WorkingDirectory=%s
ExecStart=%s
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
""" % (unit, unit, domain, workdir, command)


def start_supervisord(binary):
    conf = "/etc/supervisor/supervisord.conf"
    os.makedirs("/etc/supervisor", exist_ok=True)
    if not os.path.isfile(conf):
        echo = os.path.join(PANEL, "pyenv/bin/echo_supervisord_conf")
        if os.path.isfile(echo):
            with open(conf, "w", encoding="utf-8") as handle:
                subprocess.run([echo], stdout=handle, check=False)
    plugin_conf = os.path.join(PANEL, "plugin/supervisor/config.py")
    if os.path.isfile(plugin_conf):
        subprocess.run([sys.executable, plugin_conf], cwd=os.path.dirname(plugin_conf), stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    if subprocess.run(["pgrep", "-f", "supervisord"], stdout=subprocess.DEVNULL).returncode != 0:
        subprocess.run([binary, "-c", conf], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def add_process(args):
    """挂上本站 start.sh。插件或 pip 不可用时打印本站点的 systemd 单元，退出码仍为 0。"""
    load_public()
    domain = args.domain.strip().lower()
    command = args.command
    workdir = args.workdir
    plugin = os.path.join(PANEL, "plugin/supervisor/supervisor_main.py")
    binary = ensure_supervisord()
    if not binary or not os.path.isfile(plugin):
        manual(systemd_text(domain, workdir, command), [])
        emit("AUTH_PRO_RESULT", "systemd")
        return
    start_supervisord(binary)
    plugin_dir = os.path.dirname(plugin)
    if plugin_dir not in sys.path:
        sys.path.insert(0, plugin_dir)
    try:
        import supervisor_main

        program = "auth_pro_" + domain.split(".")[0].replace("-", "_")
        if len(program) > 40:
            program = program[:40]
        result = supervisor_main.supervisor_main().AddProcess(obj(
            pjname=program,
            user="www",
            path=workdir,
            command=command,
            numprocs="1",
        ))
    except Exception as exc:
        manual(systemd_text(domain, workdir, command), [str(exc)])
        emit("AUTH_PRO_RESULT", "systemd")
        return
    info("AddProcess 返回：%s" % result)
    if isinstance(result, dict) and result.get("status"):
        emit("AUTH_PRO_RESULT", "ok")
        emit("AUTH_PRO_PROGRAM", program)
        return
    text = str(result)
    if "已存在" in text or "exist" in text.lower():
        emit("AUTH_PRO_RESULT", "ok")
        emit("AUTH_PRO_PROGRAM", program)
        return
    manual(systemd_text(domain, workdir, command), ["返回值：%s" % result])
    emit("AUTH_PRO_RESULT", "systemd")


def append_install_log(path, text):
    """把证书接口的完整返回追加到安装日志。写不进去时返回 False，调用方仍只给操作者一句中文。"""
    if not path:
        return False
    try:
        parent = os.path.dirname(path)
        if parent:
            os.makedirs(parent, exist_ok=True)
        with open(path, "a", encoding="utf-8") as handle:
            handle.write(text)
            if not text.endswith("\n"):
                handle.write("\n")
        return True
    except OSError:
        return False


def keep_http(args, detail):
    """证书失败只在终端打一行原因。完整返回值进安装日志，避免同一段内容打两遍。"""
    wrote = append_install_log(args.log, "apply_cert_api: %s" % detail)
    if wrote:
        print("[注意] 证书没有签发，站点保持 HTTP。请在面板里对已经解析到本机的域名申请证书。详情见 %s" % args.log, file=sys.stderr, flush=True)
    else:
        print("[注意] 证书没有签发，站点保持 HTTP。请在面板里对已经解析到本机的域名申请证书。", file=sys.stderr, flush=True)
    emit("AUTH_PRO_RESULT", "http")


def apply_cert(args):
    """HTTP 验证申请证书。失败就保持 HTTP，不撤站点。"""
    public = load_public()
    domain = args.domain.strip().lower()
    try:
        import acme_v2

        row = public.M("sites").where("name=?", (domain,)).find()
        if not isinstance(row, dict) or not row.get("id"):
            keep_http(args, "站点 %s 不在面板网站列表里" % domain)
            return
        result = acme_v2.acme_v2().apply_cert_api(obj(
            id=str(row["id"]),
            auth_type="http",
            auth_to=args.webroot,
            domains=json.dumps([domain]),
        ))
    except Exception as exc:
        keep_http(args, exc)
        return
    if isinstance(result, dict) and (result.get("status") is True or result.get("cert")):
        emit("AUTH_PRO_RESULT", "https")
        return
    keep_http(args, result)


def build_parser():
    parser = argparse.ArgumentParser(description="宝塔面板内部操作")
    sub = parser.add_subparsers(dest="command", required=True)

    sub.add_parser("preflight").set_defaults(func=preflight)

    site = sub.add_parser("check-site")
    site.add_argument("--domain", required=True)
    site.add_argument("--path", required=True)
    site.set_defaults(func=check_site)

    db = sub.add_parser("check-database")
    db.add_argument("--name", required=True)
    db.add_argument("--user", required=True)
    db.set_defaults(func=check_database)

    add = sub.add_parser("add-site")
    add.add_argument("--domain", required=True)
    add.add_argument("--path", required=True)
    add.set_defaults(func=add_site)

    add_db = sub.add_parser("add-database")
    add_db.add_argument("--name", required=True)
    add_db.add_argument("--user", required=True)
    add_db.add_argument("--password", required=True)
    add_db.set_defaults(func=add_database)

    back = sub.add_parser("rollback")
    back.add_argument("--domain", required=True)
    back.add_argument("--path", default="")
    back.add_argument("--db-name", default="")
    back.add_argument("--remove-site", action="store_true")
    back.add_argument("--remove-db", action="store_true")
    back.set_defaults(func=rollback)

    proxy = sub.add_parser("proxy")
    proxy.add_argument("--domain", required=True)
    proxy.add_argument("--port", required=True)
    proxy.add_argument("--snippet", required=True)
    proxy.set_defaults(func=create_proxy)

    proc = sub.add_parser("supervisor")
    proc.add_argument("--domain", required=True)
    proc.add_argument("--command", required=True)
    proc.add_argument("--workdir", required=True)
    proc.set_defaults(func=add_process)

    cert = sub.add_parser("cert")
    cert.add_argument("--domain", required=True)
    cert.add_argument("--webroot", required=True)
    cert.add_argument("--log", default="", help="证书失败时写入完整返回值的安装日志")
    cert.set_defaults(func=apply_cert)
    return parser


def main():
    args = build_parser().parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
