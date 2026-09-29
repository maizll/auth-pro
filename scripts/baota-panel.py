#!/usr/bin/env python3
# 宝塔面板内部调用。只给 backend/handler/install.sh 用，不要单独当安装器。
# 输入是子命令和参数。标准输出只给调用方捕获的 AUTH_PRO_* 键值，操作者看不到。
# 中文说明走标准错误。失败时退出码非 0，并且不删除调用方没有点名的站点、目录或数据库。
"""宝塔 13 面板内部建站、建库、反代、进程守护和证书。"""

import argparse
import json
import os
import shutil
import subprocess
import sys
import time

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


def panel_msg(result):
    """只取面板返回里的一句说明。成功时不要把整份字典打给操作者。"""
    if isinstance(result, dict):
        for key in ("msg", "message"):
            value = result.get(key)
            if value is None:
                continue
            text = str(value).replace("\r", " ").replace("\n", " ").strip()
            if text:
                return text
        return ""
    if result is None:
        return ""
    text = str(result).replace("\r", " ").replace("\n", " ").strip()
    if text.startswith("{") or text.startswith("["):
        return ""
    return text


def panel_ok(result):
    if not isinstance(result, dict):
        return False
    return bool(result.get("siteStatus") or result.get("status"))


def die_panel(sentence, result, code=1):
    """失败时才把面板的 msg 接在中文结果后面。"""
    msg = panel_msg(result)
    if msg:
        die("%s。%s" % (sentence.rstrip("。"), msg), code)
    die(sentence, code)


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
    if not panel_ok(result):
        manual("建站失败。请在面板网站里手动添加纯静态站点，不要勾选 FTP 和数据库。", [
            "域名 %s" % domain,
            "根目录 %s" % path,
        ])
        die_panel("建站失败，没有继续建库和反代", result)
    info("已创建面板站点 %s" % domain)
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
    if not panel_ok(result):
        manual("建库失败。请在面板数据库里手动建库，字符集 utf8mb4，访问权限 127.0.0.1。", [
            "库名 %s 用户 %s" % (name, user),
        ])
        die_panel("建库失败", result)
    info("已创建数据库 %s" % name)
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
            try:
                import database

                result = database.database().DeleteDatabase(obj(id=str(row["id"]), name=args.db_name))
                if panel_ok(result):
                    info("已撤掉本次新建的数据库 %s" % args.db_name)
                else:
                    manual("自动撤库失败，请在面板数据库里删除本次新建的库 %s（不要删其它库）。" % args.db_name, [
                        panel_msg(result) or "面板没有说明原因",
                    ])
            except Exception as exc:
                manual("自动撤库失败，请在面板数据库里删除本次新建的库 %s（不要删其它库）。" % args.db_name, [str(exc)])
    if args.remove_site:
        row = public.M("sites").where("name=?", (domain,)).find()
        if isinstance(row, dict) and row.get("id"):
            try:
                import panelSite

                result = panelSite.panelSite().DeleteSite(obj(
                    id=str(row["id"]),
                    webname=domain,
                    ftp="0",
                    database="0",
                    path=row.get("path") or args.path,
                ))
                if panel_ok(result):
                    info("已撤掉本次新建的站点 %s" % domain)
                else:
                    manual("自动撤站失败，请在面板网站里删除本次新建的站点 %s（不要删其它站点）。" % domain, [
                        panel_msg(result) or "面板没有说明原因",
                    ])
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
    if not panel_ok(result):
        manual("反向代理失败。请在面板里把站点 %s 反代到 http://127.0.0.1:%s 。" % (domain, args.port), [])
        die_panel("反向代理失败", result)
    info("已添加反向代理")
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


PLUGIN_DIR = os.path.join(PANEL, "plugin/supervisor")
PLUGIN_MAIN = os.path.join(PLUGIN_DIR, "supervisor_main.py")
PLUGIN_SAMPLE = os.path.join(PLUGIN_DIR, "sample.conf")
PLUGIN_PROFILE = os.path.join(PLUGIN_DIR, "profile")
PLUGIN_CONFIG = os.path.join(PLUGIN_DIR, "config.json")
SUP_CONF = "/etc/supervisor/supervisord.conf"
STANDALONE_DIR = "/etc/supervisor/auth-pro.d"
# 3.x 插件把进程名写成 program_00。列表用最后一个下划线切出程序名，所以名称里不能再带空格。


def btpip():
    for candidate in (
        os.path.join(PANEL, "pyenv/bin/btpip"),
        os.path.join(PANEL, "pyenv/bin/pip3"),
        os.path.join(PANEL, "pyenv/bin/pip"),
    ):
        if os.path.isfile(candidate):
            return candidate
    return ""


def program_name(domain):
    """与 1.7.5 相同的守护名：auth_pro_ 加域名第一段。修复时要认回已经写过的 ini。"""
    label = domain.split(".")[0].replace("-", "_")
    program = "auth_pro_" + label
    if len(program) > 40:
        program = program[:40]
    return program


def plugin_installed():
    return os.path.isfile(PLUGIN_MAIN)


def ctl_bin():
    candidate = os.path.join(PANEL, "pyenv/bin/supervisorctl")
    if os.path.isfile(candidate):
        return candidate
    return "supervisorctl"


def supervisord_bin():
    candidate = os.path.join(PANEL, "pyenv/bin/supervisord")
    if os.path.isfile(candidate):
        return candidate
    return "supervisord"


def run_ctl(*args):
    """必须带 -c 指向面板这份配置。不带 -c 时会先找到当前目录或 /etc/supervisord.conf，连错守护进程。"""
    return subprocess.run(
        [ctl_bin(), "-c", SUP_CONF, *args],
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
    )


def daemon_responds():
    """0 表示进程都在跑，3 表示已经连上守护但有程序停着。这两种都说明连的是这块配置对应的 supervisord。"""
    if not os.path.isfile(SUP_CONF):
        return False
    proc = run_ctl("status")
    return proc.returncode in (0, 3)


def read_ini_command(path):
    try:
        with open(path, encoding="utf-8", errors="replace") as handle:
            for line in handle:
                if line.startswith("command="):
                    return line.split("=", 1)[1].strip()
    except OSError:
        return ""
    return ""


def command_is_ours(text, command, workdir):
    """只认本站 start.sh 或本站二进制。同机其它站点的 ini 不能删。"""
    got = (text or "").strip()
    if not got:
        return False
    if got == command.strip():
        return True
    return got == os.path.join(workdir, "auth_pro")


def panel_manual(program, workdir, command, detail):
    manual("进程守护没有就绪。请只在宝塔「进程守护管理器」里添加本站点，不要改其它站点，也不要再 nohup 一份。", [
        "名称：%s" % program,
        "启动用户：www",
        "运行目录：%s" % workdir,
        "启动命令：%s" % command,
        "进程数量：1",
        "保存后列表里要有这一项，supervisorctl 状态为 RUNNING，监听进程的父进程是 supervisord。",
        detail,
    ])


def fail_guardian(program, workdir, command, detail):
    panel_manual(program, workdir, command, detail)
    die("进程守护没有就绪，不能当作安装完成")


def plugin_conf_ok():
    """主配置必须把 profile/*.ini 包含进去。列表读的是这块配置拉起的 supervisord，不是 profile 目录本身。"""
    if not os.path.isfile(SUP_CONF):
        return False
    try:
        text = open(SUP_CONF, encoding="utf-8", errors="replace").read()
    except OSError:
        return False
    if "[supervisord]" not in text or "[supervisorctl]" not in text:
        return False
    return "/plugin/supervisor/profile/" in text


def restore_plugin_conf():
    """主配置丢了 include 时，用插件自带的样例补回。

    插件目录里那个整理配置的脚本会把正在使用的 supervisord.conf 换成空文件并删掉原文件。
    空配置下 supervisorctl update 连不上面板正在用的守护进程，ini 写了也不会进列表。
    这里只在主配置不完整时覆盖这一份，不删除 profile 里其它站点的 ini。
    """
    if plugin_conf_ok():
        return
    if not os.path.isfile(PLUGIN_SAMPLE):
        die("进程守护主配置不完整，插件也没有样例配置，无法补回。没有改动其它站点的守护项。")
    os.makedirs("/etc/supervisor", exist_ok=True)
    shutil.copyfile(PLUGIN_SAMPLE, SUP_CONF)
    info("已用插件样例补回进程守护主配置，其它站点的 profile 配置没有删除")


def ensure_daemon():
    """已经在跑就不要再起一个。两个 supervisord 会抢同一个套接字，面板列表仍看原来那一个。"""
    if daemon_responds():
        return True
    subprocess.run(["systemctl", "start", "supervisord"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    if daemon_responds():
        return True
    binary = supervisord_bin()
    if binary == "supervisord" and not shutil.which("supervisord"):
        return False
    subprocess.run([binary, "-c", SUP_CONF], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    for _ in range(20):
        if daemon_responds():
            return True
        time.sleep(0.25)
    return False


def drop_our_registration(program, command, workdir):
    """删掉本站点自己的 ini 和 config.json 条目，好让插件的 AddProcess 重新登记。其它程序保留。"""
    ini = os.path.join(PLUGIN_PROFILE, program + ".ini")
    if os.path.isfile(ini):
        if not command_is_ours(read_ini_command(ini), command, workdir):
            die("进程名 %s 已被占用，且启动命令不是本站点。没有删除该配置，也没有改其它守护项。" % program)
        os.remove(ini)
    if not os.path.isfile(PLUGIN_CONFIG):
        return
    try:
        data = json.loads(open(PLUGIN_CONFIG, encoding="utf-8").read() or "[]")
    except (OSError, ValueError):
        info("进程守护的 config.json 无法解析，没有改写它")
        return
    if not isinstance(data, list):
        return
    kept = [item for item in data if not (isinstance(item, dict) and item.get("program") == program)]
    if len(kept) == len(data):
        return
    tmp = PLUGIN_CONFIG + ".auth-pro-tmp"
    with open(tmp, "w", encoding="utf-8") as handle:
        json.dump(kept, handle, ensure_ascii=False)
    os.replace(tmp, PLUGIN_CONFIG)


def panel_process_rows():
    """和面板同一个 GetProcessList。它向 /var/run/supervisor.sock 要进程表，不扫描 profile 目录。"""
    try:
        bootstrap()
        plugin_dir = os.path.dirname(PLUGIN_MAIN)
        if plugin_dir not in sys.path:
            sys.path.insert(0, plugin_dir)
        import supervisor_main

        result = supervisor_main.supervisor_main().GetProcessList(obj())
    except SystemExit:
        raise
    except Exception as exc:
        return None, str(exc)
    if isinstance(result, list):
        return result, ""
    return None, panel_msg(result)


def wait_running(program):
    """startsecs=3，再加几次重试。刚 update 完立刻看状态会停在 STARTING。"""
    last = ""
    for _ in range(20):
        proc = run_ctl("status", program + ":")
        last = (proc.stdout or "").strip()
        if "RUNNING" in last:
            return last
        time.sleep(1)
    return last


def start_group(program):
    proc = run_ctl("start", program + ":")
    text = proc.stdout or ""
    if "already started" in text or "RUNNING" in text:
        return wait_running(program)
    return wait_running(program)


def log_tail(program):
    path = os.path.join(PLUGIN_DIR, "log", program + ".err.log")
    if not os.path.isfile(path):
        path = os.path.join("/var/log/auth-pro-supervisor", program + ".err.log")
    try:
        with open(path, encoding="utf-8", errors="replace") as handle:
            lines = handle.readlines()
    except OSError:
        return ""
    return "".join(lines[-20:]).strip()


def register_with_plugin(args):
    """插件已装时只走 AddProcess。不另起 supervisord，也不调用会清空主配置的整理脚本。"""
    load_public()
    domain = args.domain.strip().lower()
    command = args.command
    workdir = args.workdir
    program = program_name(domain)
    if not os.path.isdir(workdir):
        fail_guardian(program, workdir, command, "运行目录不存在：%s" % workdir)
    restore_plugin_conf()
    if not ensure_daemon():
        fail_guardian(program, workdir, command, "面板的 supervisord 没有在运行，也没有拉起来。")
    # 已有同名 ini 时 AddProcess 直接返回「已被使用」，不会 update，列表里就不会出现。
    # 只清本站点这一条，再交给插件重新登记。
    drop_our_registration(program, command, workdir)
    run_ctl("update")
    plugin_dir = os.path.dirname(PLUGIN_MAIN)
    if plugin_dir not in sys.path:
        sys.path.insert(0, plugin_dir)
    import supervisor_main

    try:
        # ps 必须带上。3.0.5/3.0.6 的 AddProcess 在写完 ini 之后读取 get.ps，缺了会抛异常，config.json 来不及写入。
        result = supervisor_main.supervisor_main().AddProcess(obj(
            pjname=program,
            user="www",
            path=workdir,
            command=command,
            numprocs="1",
            ps="auth-pro",
        ))
    except Exception as exc:
        fail_guardian(program, workdir, command, "登记进程守护失败：%s" % exc)
    text = str(result)
    if not panel_ok(result) and ("已存在" in text or "已被使用" in text):
        # 并发或漏删时再清一次本站点，不碰其它名称。
        drop_our_registration(program, command, workdir)
        try:
            result = supervisor_main.supervisor_main().AddProcess(obj(
                pjname=program,
                user="www",
                path=workdir,
                command=command,
                numprocs="1",
                ps="auth-pro",
            ))
        except Exception as exc:
            fail_guardian(program, workdir, command, "登记进程守护失败：%s" % exc)
    if not panel_ok(result):
        fail_guardian(program, workdir, command, panel_msg(result) or "面板没有接受进程守护")
    info("已登记进程守护")
    # 插件自己的 update 不带 -c。当前目录在面板根目录时，它会按默认顺序找配置，
    # 连不上正在跑的守护进程，报错分支还会把 supervisord 杀掉。
    # ini 已经写好之后，再用面板这份主配置显式 reread/update，条目才会进面板正在用的进程表。
    if not daemon_responds() and not ensure_daemon():
        fail_guardian(program, workdir, command, "AddProcess 之后面板的 supervisord 没有在运行。")
    run_ctl("reread")
    run_ctl("update")
    status = start_group(program)
    if "RUNNING" not in status:
        fail_guardian(program, workdir, command, "状态不是 RUNNING。错误日志：%s" % (log_tail(program) or "无"))
    info("进程守护已在运行")
    rows, err = panel_process_rows()
    names = []
    if isinstance(rows, list):
        names = [str(item.get("program")) for item in rows if isinstance(item, dict)]
    if program not in names:
        fail_guardian(program, workdir, command, "面板列表里没有 %s。列表接口：%s 当前项：%s" % (program, err or "空", " ".join(names)))
    emit("AUTH_PRO_RESULT", "ok")
    emit("AUTH_PRO_PROGRAM", program)
    emit("AUTH_PRO_SUP_CONF", SUP_CONF)
    emit("AUTH_PRO_PLUGIN", "yes")


def ensure_supervisord_package():
    """插件没装时才用面板的 pip 装 supervisor。插件已装时不能装，否则会盖掉插件钉死的版本。"""
    binary = os.path.join(PANEL, "pyenv/bin/supervisord")
    if os.path.isfile(binary):
        return binary
    pip = btpip()
    if not pip:
        return ""
    info("进程守护插件未安装，改用面板 Python 从 PyPI 安装 supervisor")
    proc = subprocess.run([pip, "install", "supervisor"], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    if proc.returncode != 0 or not os.path.isfile(binary):
        info((proc.stdout or "")[-800:])
        return ""
    return binary


def write_standalone_conf():
    """插件未装时的配置只包含本工具自己的目录，不写进插件 profile，避免以后装插件时混进别人的列表。"""
    os.makedirs(STANDALONE_DIR, exist_ok=True)
    os.makedirs("/var/log", exist_ok=True)
    marker = STANDALONE_DIR + "/*.ini"
    if os.path.isfile(SUP_CONF):
        try:
            text = open(SUP_CONF, encoding="utf-8", errors="replace").read()
        except OSError:
            text = ""
        if marker in text and "[supervisord]" in text:
            return
        if text.strip() and "[supervisord]" in text and marker not in text:
            with open(SUP_CONF, "a", encoding="utf-8") as handle:
                handle.write("\n[include]\nfiles = %s\n" % marker)
            return
    os.makedirs("/etc/supervisor", exist_ok=True)
    with open(SUP_CONF, "w", encoding="utf-8") as handle:
        handle.write("""[unix_http_server]
file=/var/run/supervisor.sock
chmod=0700

[supervisord]
logfile=/var/log/supervisord.log
pidfile=/var/run/supervisord.pid
nodaemon=false

[rpcinterface:supervisor]
supervisor.rpcinterface_factory = supervisor.rpcinterface:make_main_rpcinterface

[supervisorctl]
serverurl=unix:///var/run/supervisor.sock

[include]
files = %s
""" % marker)


def write_standalone_ini(program, user, workdir, command):
    os.makedirs(STANDALONE_DIR, exist_ok=True)
    os.makedirs("/var/log/auth-pro-supervisor", exist_ok=True)
    path = os.path.join(STANDALONE_DIR, program + ".ini")
    if os.path.isfile(path) and not command_is_ours(read_ini_command(path), command, workdir):
        die("独立守护配置 %s 的启动命令不是本站点，已停止。没有覆盖它。" % path)
    body = "\n".join([
        "[program:%s]" % program,
        "command=%s" % command,
        "directory=%s" % workdir,
        "autorestart=true",
        "startsecs=3",
        "startretries=3",
        "stdout_logfile=/var/log/auth-pro-supervisor/%s.out.log" % program,
        "stderr_logfile=/var/log/auth-pro-supervisor/%s.err.log" % program,
        "stdout_logfile_maxbytes=2MB",
        "stderr_logfile_maxbytes=2MB",
        "user=%s" % user,
        "priority=999",
        "numprocs=1",
        "process_name=%(program_name)s_%(process_num)02d",
        "",
    ])
    with open(path, "w", encoding="utf-8") as handle:
        handle.write(body)


def run_user():
    try:
        import pwd
        pwd.getpwnam("www")
        return "www"
    except KeyError:
        return "root"


def register_without_plugin(args):
    """插件未装：用面板 Python 装 supervisor，由这一份 supervisord 拉起。不再 nohup。"""
    domain = args.domain.strip().lower()
    command = args.command
    workdir = args.workdir
    program = program_name(domain)
    if not ensure_supervisord_package():
        manual(systemd_text(domain, workdir, command), ["面板 Python 没能装上 supervisor。"])
        emit("AUTH_PRO_RESULT", "systemd")
        emit("AUTH_PRO_PROGRAM", program)
        return
    write_standalone_conf()
    write_standalone_ini(program, run_user(), workdir, command)
    if not ensure_daemon():
        fail_guardian(program, workdir, command, "supervisord 没有起来。")
    run_ctl("reread")
    run_ctl("update")
    status = start_group(program)
    if "RUNNING" not in status:
        fail_guardian(program, workdir, command, "状态不是 RUNNING。错误日志：%s" % (log_tail(program) or "无"))
    info("进程守护已在运行")
    emit("AUTH_PRO_RESULT", "ok")
    emit("AUTH_PRO_PROGRAM", program)
    emit("AUTH_PRO_SUP_CONF", SUP_CONF)
    emit("AUTH_PRO_PLUGIN", "no")


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


def add_process(args):
    """插件已装就走 AddProcess。未装则由本脚本拉起 supervisord。两种都不 nohup。"""
    if getattr(args, "repair", False):
        info("重新登记本站点的进程守护。只处理本站点的 ini，不改其它站点、数据库和 Nginx。")
    if plugin_installed():
        register_with_plugin(args)
        return
    register_without_plugin(args)


def supervisor_check(args):
    """给安装脚本核验用。标准输出仍只有 AUTH_PRO_*。"""
    domain = args.domain.strip().lower()
    program = program_name(domain)
    emit("AUTH_PRO_PROGRAM", program)
    emit("AUTH_PRO_SUP_CONF", SUP_CONF)
    if plugin_installed():
        emit("AUTH_PRO_PLUGIN", "yes")
        if not daemon_responds():
            emit("AUTH_PRO_RESULT", "stopped")
            emit("AUTH_PRO_LIST", "")
            emit("AUTH_PRO_CTL", "")
            return
        rows, err = panel_process_rows()
        names = []
        hit = ""
        if isinstance(rows, list):
            for item in rows:
                if not isinstance(item, dict):
                    continue
                name = str(item.get("program") or "")
                names.append(name)
                if name == program:
                    hit = str(item.get("runStatus") or "")
        emit("AUTH_PRO_LIST", " ".join(names))
        proc = run_ctl("status", program + ":")
        emit("AUTH_PRO_CTL", (proc.stdout or "").strip())
        if program in names and "RUNNING" in (proc.stdout or "") and hit == "RUNNING":
            emit("AUTH_PRO_RESULT", "running")
            return
        if err:
            info("没有读到进程守护列表。%s" % err)
        emit("AUTH_PRO_RESULT", "missing" if program not in names else "stopped")
        return
    emit("AUTH_PRO_PLUGIN", "no")
    emit("AUTH_PRO_LIST", "")
    if not daemon_responds():
        emit("AUTH_PRO_RESULT", "stopped")
        emit("AUTH_PRO_CTL", "")
        return
    proc = run_ctl("status", program + ":")
    emit("AUTH_PRO_CTL", (proc.stdout or "").strip())
    if "RUNNING" in (proc.stdout or ""):
        emit("AUTH_PRO_RESULT", "running")
        return
    emit("AUTH_PRO_RESULT", "stopped")


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


def replace_proxy_port(path, old_port, new_port):
    """只改未注释的 proxy_pass。注释里的示例端口不动，避免把说明改乱。"""
    if not os.path.isfile(path):
        return False
    original = open(path, encoding="utf-8", errors="replace").read()
    changed = []
    hit = False
    needle = "proxy_pass http://127.0.0.1:%s" % old_port
    replacement = "proxy_pass http://127.0.0.1:%s" % new_port
    for line in original.splitlines(keepends=True):
        stripped = line.lstrip()
        if stripped.startswith("#"):
            changed.append(line)
            continue
        if needle in line:
            hit = True
            changed.append(line.replace(needle, replacement))
            continue
        changed.append(line)
    if not hit:
        return False
    tmp = path + ".auth-pro-port-tmp"
    with open(tmp, "w", encoding="utf-8") as handle:
        handle.write("".join(changed))
    os.replace(tmp, path)
    return True


def set_proxy_port(args):
    """只改本站点的反代端口，nginx -t 失败就还原这些文件。"""
    domain = args.domain.strip().lower()
    new_port = str(int(args.port))
    old_port = str(int(args.old_port))
    if new_port == old_port:
        die("新端口与当前端口相同")
    targets = []
    site_conf = "/www/server/panel/vhost/nginx/%s.conf" % domain
    if os.path.isfile(site_conf):
        targets.append(site_conf)
    proxy_dir = "/www/server/panel/vhost/nginx/proxy/%s" % domain
    if os.path.isdir(proxy_dir):
        for name in sorted(os.listdir(proxy_dir)):
            path = os.path.join(proxy_dir, name)
            if os.path.isfile(path) and name.endswith(".conf"):
                targets.append(path)
    if not targets:
        die("没有找到站点 %s 的 Nginx 配置，没有改端口" % domain)
    originals = {}
    for path in targets:
        originals[path] = open(path, encoding="utf-8", errors="replace").read()
    changed_any = False
    for path in targets:
        if replace_proxy_port(path, old_port, new_port):
            changed_any = True
            info("已更新反代端口：%s" % path)
    if not changed_any:
        die("没有找到指向 127.0.0.1:%s 的反代，没有改配置" % old_port)
    ok, detail = nginx_test()
    if not ok:
        for path, text in originals.items():
            with open(path, "w", encoding="utf-8") as handle:
                handle.write(text)
        manual("nginx -t 未通过，已还原本站点配置。", [detail[-800:]])
        die("修改端口后 nginx -t 未通过，已还原")
    if not nginx_reload():
        info("配置已通过 nginx -t，但 reload 失败。请手工执行 nginx -s reload。")
    emit("AUTH_PRO_RESULT", "ok")
    emit("AUTH_PRO_NGINX", site_conf)


def remove_our_supervisor(domain, workdir):
    """只停并删除本站点的守护项。启动命令不是本站 start.sh 时直接失败，不改其它站点。"""
    program = program_name(domain)
    command = os.path.join(workdir, "start.sh")
    ini_paths = [
        os.path.join(PLUGIN_PROFILE, program + ".ini"),
        os.path.join(STANDALONE_DIR, program + ".ini"),
    ]
    owned = False
    for ini in ini_paths:
        if not os.path.isfile(ini):
            continue
        if not command_is_ours(read_ini_command(ini), command, workdir):
            die("进程名 %s 的启动命令不是本站点。没有删除守护，也没有改其它站点。" % program)
        owned = True
    if not owned:
        info("没有找到本站点的守护配置")
        return program
    if daemon_responds():
        run_ctl("stop", program + ":")
    if plugin_installed():
        drop_our_registration(program, command, workdir)
    for ini in ini_paths:
        if os.path.isfile(ini) and command_is_ours(read_ini_command(ini), command, workdir):
            os.remove(ini)
            info("已删除守护配置 %s" % ini)
    if daemon_responds():
        run_ctl("update")
    return program


def site_dir_is_ours(path, domain):
    """只允许删除 /www/wwwroot/<域名>，并且里面有本程序的 install.lock。"""
    real = os.path.realpath(path)
    root = os.path.realpath("/www/wwwroot")
    if real == root or not real.startswith(root + os.sep):
        return False
    if os.path.basename(real) != domain:
        return False
    return os.path.isfile(os.path.join(real, "backend", "install.lock"))


def uninstall_site(args):
    """删除本站点的守护、面板站点和程序目录。默认不删数据库。"""
    public = load_public()
    domain = args.domain.strip().lower()
    path = args.path
    workdir = os.path.join(path, "backend")
    program = remove_our_supervisor(domain, workdir)
    info("已处理进程守护 %s" % program)
    if args.remove_db and args.db_name:
        row = public.M("databases").where("name=?", (args.db_name,)).find()
        if isinstance(row, dict) and row.get("id"):
            try:
                import database

                result = database.database().DeleteDatabase(obj(id=str(row["id"]), name=args.db_name))
                if not panel_ok(result):
                    manual("删除数据库失败。请在面板里只删除 %s ，不要删其它库。" % args.db_name, [
                        panel_msg(result) or "面板没有说明原因",
                    ])
                    die_panel("删除数据库失败", result)
                info("已删除数据库 %s" % args.db_name)
            except Exception as exc:
                manual("删除数据库失败。请在面板里只删除 %s ，不要删其它库。" % args.db_name, [str(exc)])
                die("删除数据库失败")
        else:
            info("面板里没有名为 %s 的数据库，跳过删库" % args.db_name)
    row = public.M("sites").where("name=?", (domain,)).find()
    if isinstance(row, dict) and row.get("id"):
        try:
            import panelSite

            result = panelSite.panelSite().DeleteSite(obj(
                id=str(row["id"]),
                webname=domain,
                ftp="0",
                database="0",
                path=row.get("path") or path,
            ))
            if not panel_ok(result):
                manual("删除面板站点失败。请在面板里只删除 %s ，不要删其它站点。" % domain, [
                    panel_msg(result) or "面板没有说明原因",
                ])
                die_panel("删除面板站点失败", result)
            info("已删除面板站点 %s" % domain)
        except Exception as exc:
            manual("删除面板站点失败。请在面板里只删除 %s ，不要删其它站点。" % domain, [str(exc)])
            die("删除面板站点失败")
    else:
        info("面板网站列表里没有 %s" % domain)
    if path and os.path.isdir(path):
        if site_dir_is_ours(path, domain):
            real = os.path.realpath(path)
            subprocess.run(["chattr", "-R", "-i", real], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            try:
                shutil.rmtree(real)
                info("已删除网站目录 %s" % real)
            except OSError as exc:
                manual("删除网站目录失败。请执行 chattr -R -i %s 后再手工删除这个目录。" % real, [str(exc)])
                die("删除网站目录失败")
        else:
            die("拒绝删除 %s 。目录不是 /www/wwwroot/%s ，或没有 install.lock。" % (path, domain))
    emit("AUTH_PRO_RESULT", "ok")
    emit("AUTH_PRO_PROGRAM", program)


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
    proc.add_argument("--repair", action="store_true")
    proc.set_defaults(func=add_process)

    check = sub.add_parser("supervisor-check")
    check.add_argument("--domain", required=True)
    check.add_argument("--command", required=True)
    check.set_defaults(func=supervisor_check)

    cert = sub.add_parser("cert")
    cert.add_argument("--domain", required=True)
    cert.add_argument("--webroot", required=True)
    cert.add_argument("--log", default="", help="证书失败时写入完整返回值的安装日志")
    cert.set_defaults(func=apply_cert)

    port_cmd = sub.add_parser("set-port")
    port_cmd.add_argument("--domain", required=True)
    port_cmd.add_argument("--port", required=True)
    port_cmd.add_argument("--old-port", required=True)
    port_cmd.set_defaults(func=set_proxy_port)

    remove = sub.add_parser("uninstall")
    remove.add_argument("--domain", required=True)
    remove.add_argument("--path", required=True)
    remove.add_argument("--db-name", default="")
    remove.add_argument("--remove-db", action="store_true")
    remove.set_defaults(func=uninstall_site)
    return parser


def main():
    args = build_parser().parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
