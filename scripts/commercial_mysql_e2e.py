#!/usr/bin/env python3
"""商业版双站 MySQL 端到端，以及 v1.5.8 → 1.5.9 升级迁移。

没有可用的 MySQL 时以退出码 77 结束，供 go test 跳过。
域名校验走真实 HTTPS：文档地址 203.0.113.10、本机 CA、SNI 反代。
不改生产代码里的私网/443 限制。
"""

from __future__ import annotations

import base64
import hashlib
import json
import os
import shutil
import signal
import socket
import ssl
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
RUNTIME = "/tmp/auth-pro-e2e"
BIN_DIR = os.path.join(RUNTIME, "bin")
BIN_159 = os.path.join(BIN_DIR, "auth-pro-159")
BIN_158 = os.path.join(BIN_DIR, "auth-pro-158")
WT_158 = "/tmp/auth-pro-158"
COMMIT_158 = "9db6c4492bb259ef3022c4e25072f1dcd9a983ea"
FRONTEND = os.path.join(ROOT, "frontend", "dist")
ART = "/opt/cursor/artifacts/screenshots"
IP = "203.0.113.10"
SOURCE_HOST = "source.auth-pro.test"
BUYER_HOST = "buyer.auth-pro.test"
DB_USER = "authpro_e2e"
DB_PASS = "authpro_e2e_pass"
ADMIN_USER = "e2eadmin"
ADMIN_PASS = "e2e-admin-pass"
BUYER_EMAIL = "buyer@example.com"
BUYER_PASS = "buyer-pass-1"
EPAY_PID = "10001"
EPAY_KEY = "test-epay-key"
SOURCE_DB = "authpro_e2e_source"
BUYER_DB = "authpro_e2e_buyer"
UPGRADE_DB = "authpro_e2e_upgrade"
PROCS: dict[str, subprocess.Popen] = {}


class Fail(Exception):
    pass


def log(msg: str) -> None:
    print(msg, flush=True)


def run(cmd: list[str], **kwargs) -> subprocess.CompletedProcess:
    log("+ " + " ".join(cmd))
    return subprocess.run(cmd, check=False, text=True, **kwargs)


def mysql_server_up() -> bool:
    if shutil.which("mysqladmin"):
        ping = run(["mysqladmin", "--protocol=socket", "ping"], capture_output=True)
        if ping.returncode == 0:
            return True
    ping = run(["sudo", "-n", "mysqladmin", "--protocol=socket", "ping"], capture_output=True)
    return ping.returncode == 0


def sudo_mysql(sql: str) -> None:
    proc = run(["sudo", "-n", "mysql", "-e", sql], capture_output=True)
    if proc.returncode != 0:
        raise Fail(proc.stderr or proc.stdout or "mysql failed")


def sql_query(database: str, statement: str, required: bool = True) -> str:
    proc = subprocess.run(
        ["mysql", "-u", DB_USER, f"-p{DB_PASS}", "-h", "127.0.0.1", "-N", "-B", database, "-e", statement],
        check=False,
        text=True,
        capture_output=True,
    )
    if proc.returncode != 0:
        if required:
            raise Fail(proc.stderr or "query failed")
        return ""
    return proc.stdout.strip()


def setup_databases() -> None:
    sudo_mysql(
        f"""
        CREATE USER IF NOT EXISTS '{DB_USER}'@'127.0.0.1' IDENTIFIED BY '{DB_PASS}';
        ALTER USER '{DB_USER}'@'127.0.0.1' IDENTIFIED BY '{DB_PASS}';
        DROP DATABASE IF EXISTS {SOURCE_DB};
        DROP DATABASE IF EXISTS {BUYER_DB};
        DROP DATABASE IF EXISTS {UPGRADE_DB};
        CREATE DATABASE {SOURCE_DB} CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
        CREATE DATABASE {BUYER_DB} CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
        CREATE DATABASE {UPGRADE_DB} CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
        GRANT ALL PRIVILEGES ON {SOURCE_DB}.* TO '{DB_USER}'@'127.0.0.1';
        GRANT ALL PRIVILEGES ON {BUYER_DB}.* TO '{DB_USER}'@'127.0.0.1';
        GRANT ALL PRIVILEGES ON {UPGRADE_DB}.* TO '{DB_USER}'@'127.0.0.1';
        FLUSH PRIVILEGES;
        """
    )


def setup_network() -> str:
    run(["sudo", "-n", "ip", "addr", "add", f"{IP}/32", "dev", "lo"], capture_output=True)
    hosts = "/etc/hosts"
    begin, end = "# BEGIN auth-pro-e2e", "# END auth-pro-e2e"
    block = f"{begin}\n{IP} {SOURCE_HOST} {BUYER_HOST}\n{end}\n"
    current = open(hosts, encoding="utf-8").read()
    if begin in current:
        pre, rest = current.split(begin, 1)
        _, post = rest.split(end, 1)
        current = pre + post.lstrip("\n")
    open("/tmp/auth-pro-e2e-hosts", "w", encoding="utf-8").write(current.rstrip() + "\n" + block)
    run(["sudo", "-n", "cp", "/tmp/auth-pro-e2e-hosts", hosts], capture_output=True)

    cert_dir = os.path.join(RUNTIME, "certs")
    os.makedirs(cert_dir, exist_ok=True)
    ca_key, ca_crt = os.path.join(cert_dir, "ca.key"), os.path.join(cert_dir, "ca.crt")
    tls_key, tls_crt = os.path.join(cert_dir, "tls.key"), os.path.join(cert_dir, "tls.crt")
    csr = os.path.join(cert_dir, "tls.csr")
    ext = os.path.join(cert_dir, "san.cnf")
    open(ext, "w", encoding="utf-8").write(
        "subjectAltName=DNS:source.auth-pro.test,DNS:buyer.auth-pro.test\n"
        "basicConstraints=CA:FALSE\n"
        "keyUsage=digitalSignature,keyEncipherment\n"
        "extendedKeyUsage=serverAuth\n"
    )
    run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-keyout", ca_key, "-out", ca_crt, "-days", "2", "-nodes", "-subj", "/CN=auth-pro-e2e-ca"], capture_output=True)
    run(["openssl", "req", "-newkey", "rsa:2048", "-keyout", tls_key, "-out", csr, "-nodes", "-subj", "/CN=source.auth-pro.test"], capture_output=True)
    signed = run(
        ["openssl", "x509", "-req", "-in", csr, "-CA", ca_crt, "-CAkey", ca_key, "-CAcreateserial", "-out", tls_crt, "-days", "2", "-extfile", ext],
        capture_output=True,
    )
    if signed.returncode != 0:
        raise Fail(signed.stderr or "openssl sign failed")
    run(["sudo", "-n", "cp", ca_crt, "/usr/local/share/ca-certificates/auth-pro-e2e.crt"], capture_output=True)
    updated = run(["sudo", "-n", "update-ca-certificates"], capture_output=True)
    if updated.returncode != 0:
        raise Fail(updated.stderr or "update-ca-certificates failed")

    nginx_conf = os.path.join(RUNTIME, "nginx.conf")
    open(nginx_conf, "w", encoding="utf-8").write(
        f"""
pid {RUNTIME}/nginx.pid;
error_log {RUNTIME}/nginx-error.log;
daemon on;
worker_processes 1;
events {{ worker_connections 128; }}
http {{
  include /etc/nginx/mime.types;
  access_log {RUNTIME}/nginx-access.log;
  server {{
    listen {IP}:443 ssl;
    server_name {SOURCE_HOST};
    ssl_certificate {tls_crt};
    ssl_certificate_key {tls_key};
    location / {{
      proxy_pass http://127.0.0.1:18081;
      proxy_set_header Host $host;
      proxy_set_header X-Forwarded-Proto https;
    }}
  }}
  server {{
    listen {IP}:443 ssl;
    server_name {BUYER_HOST};
    ssl_certificate {tls_crt};
    ssl_certificate_key {tls_key};
    location / {{
      proxy_pass http://127.0.0.1:18082;
      proxy_set_header Host $host;
      proxy_set_header X-Forwarded-Proto https;
    }}
  }}
}}
"""
    )
    if os.path.exists(os.path.join(RUNTIME, "nginx.pid")):
        run(["sudo", "-n", "nginx", "-c", nginx_conf, "-s", "stop"], capture_output=True)
        time.sleep(0.3)
    started = run(["sudo", "-n", "nginx", "-c", nginx_conf], capture_output=True)
    if started.returncode != 0:
        raise Fail((started.stderr or "") + (started.stdout or "") + open(os.path.join(RUNTIME, "nginx-error.log"), encoding="utf-8", errors="replace").read())
    return ca_crt


def build_159(public_key: str = "") -> None:
    os.makedirs(BIN_DIR, exist_ok=True)
    ldflags = ""
    if public_key:
        ldflags = f"-X auto_pro/handler.embeddedStoreSnapshotPublicKey={public_key}"
    cmd = ["go", "build", "-o", BIN_159]
    if ldflags:
        cmd.extend(["-ldflags", ldflags])
    cmd.append(".")
    proc = run(cmd, cwd=os.path.join(ROOT, "backend"), capture_output=True)
    if proc.returncode != 0:
        raise Fail(proc.stderr or "go build 1.5.9 failed")


def build_158() -> None:
    if not os.path.isdir(os.path.join(WT_158, "backend")):
        proc = run(["git", "worktree", "add", "--detach", WT_158, COMMIT_158], cwd=ROOT, capture_output=True)
        if proc.returncode != 0:
            raise Fail(proc.stderr or "worktree v1.5.8 failed")
    version = open(os.path.join(WT_158, "VERSION"), encoding="utf-8").read().strip()
    if version != "1.5.8":
        raise Fail(f"expected v1.5.8, got {version}")
    proc = run(["go", "build", "-o", BIN_158, "."], cwd=os.path.join(WT_158, "backend"), capture_output=True)
    if proc.returncode != 0:
        raise Fail(proc.stderr or "go build 1.5.8 failed")


def stop_named(*names: str) -> None:
    targets = list(PROCS) if not names else list(names)
    for name in targets:
        proc = PROCS.get(name)
        if proc is not None and proc.poll() is None:
            proc.send_signal(signal.SIGTERM)
    deadline = time.time() + 8
    for name in targets:
        proc = PROCS.get(name)
        if proc is None:
            continue
        while proc.poll() is None and time.time() < deadline:
            time.sleep(0.1)
        if proc.poll() is None:
            proc.kill()
        PROCS.pop(name, None)


def start_server(name: str, binary: str, port: int, database: str, data_dir: str) -> None:
    os.makedirs(data_dir, exist_ok=True)
    env = os.environ.copy()
    # 不预置数据库环境变量。空库上的启动迁移会直接退出，安装页也就打不开。
    # 安装接口把连接写进数据目录，重启后仍从 db.json 读取。
    env.update(
        {
            "AUTO_PRO_DATA_DIR": data_dir,
            "AUTO_PRO_FRONTEND_DIR": FRONTEND,
            "PORT": str(port),
            "HOST": "127.0.0.1",
            "CGO_ENABLED": "1",
        }
    )
    log_path = os.path.join(RUNTIME, f"{name}.log")
    log_file = open(log_path, "ab")
    proc = subprocess.Popen([binary], cwd=data_dir, env=env, stdout=log_file, stderr=subprocess.STDOUT)
    PROCS[name] = proc
    wait_local(port, proc, log_path)


def wait_local(port: int, proc: subprocess.Popen, log_path: str) -> None:
    deadline = time.time() + 20
    while time.time() < deadline:
        if proc.poll() is not None:
            raise Fail(f"server on {port} exited\n" + tail(log_path))
        try:
            with socket.create_connection(("127.0.0.1", port), 0.3):
                return
        except OSError:
            time.sleep(0.2)
    raise Fail(f"server on {port} did not listen\n" + tail(log_path))


def tail(path: str) -> str:
    try:
        data = open(path, encoding="utf-8", errors="replace").read()
    except OSError:
        return ""
    return data[-4000:]


def http_json(url: str, method: str = "GET", body: dict | None = None, token: str = "", ca: str = "", form: dict | None = None) -> tuple[int, object, bytes]:
    data = None
    headers = {}
    if form is not None:
        data = urllib.parse.urlencode(form).encode()
        headers["Content-Type"] = "application/x-www-form-urlencoded"
    elif body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    context = ssl.create_default_context(cafile=ca) if ca else None
    try:
        with urllib.request.urlopen(req, context=context, timeout=30) as resp:
            raw = resp.read()
            code = resp.status
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        code = exc.code
    parsed: object
    try:
        parsed = json.loads(raw.decode() or "null")
    except json.JSONDecodeError:
        parsed = raw.decode(errors="replace")
    return code, parsed, raw


def must_api(base: str, method: str, path: str, body: dict | None = None, token: str = "", ca: str = "") -> dict:
    code, parsed, raw = http_json(base + path, method, body, token, ca)
    if not isinstance(parsed, dict) or parsed.get("code") not in (200, "200"):
        raise Fail(f"{method} {path} -> {code} {raw[:800]!r}")
    return parsed


def install_instance(base: str, database: str, ca: str = "") -> str:
    db = {
        "host": "127.0.0.1",
        "port": "3306",
        "database": database,
        "username": DB_USER,
        "password": DB_PASS,
    }
    code, parsed, raw = http_json(base + "/api/install/init-tables", "POST", db, ca=ca)
    if not isinstance(parsed, dict) or parsed.get("code") not in (200, "200"):
        raise Fail(f"init-tables {raw[:800]!r}")
    payload = dict(db)
    payload["adminUsername"] = ADMIN_USER
    payload["adminPassword"] = ADMIN_PASS
    code, parsed, raw = http_json(base + "/api/install/create-admin", "POST", payload, ca=ca)
    if not isinstance(parsed, dict) or parsed.get("code") not in (200, "200"):
        raise Fail(f"create-admin {raw[:800]!r}")
    logged = must_api(base, "POST", "/api/auth/login", {"userName": ADMIN_USER, "password": ADMIN_PASS}, ca=ca)
    token = logged["data"]["token"]
    if not token:
        raise Fail("empty token")
    return token


def public_key_from_private(data_dir: str) -> str:
    raw = open(os.path.join(data_dir, "store", "snapshot-ed25519.key"), encoding="utf-8").read().strip()
    key = base64.b64decode(raw)
    if len(key) != 64:
        raise Fail(f"unexpected private key length {len(key)}")
    return base64.b64encode(key[32:]).decode()


def sign_epay(params: dict[str, str], key: str) -> str:
    items = [f"{name}={params[name]}" for name in sorted(params) if name not in ("sign", "sign_type") and params[name].strip()]
    digest = hashlib.md5(("&".join(items) + key).encode()).hexdigest()
    return digest


def configure_source(base: str, token: str, ca: str) -> None:
    created = must_api(
        base,
        "POST",
        "/api/app/create",
        {
            "name": "商店产品",
            "enabled": True,
            "remark": "e2e",
            "purchaseLicenseTypes": ["domain"],
            "commercialProduct": True,
            "graceDays": 7,
            "revokeOnPasswordChange": True,
            "commercialFeatures": ["multi_app"],
        },
        token,
        ca,
    )
    app_id = created["data"]["id"]
    must_api(
        base,
        "POST",
        "/api/plan/create",
        {
            "appId": app_id,
            "name": "永久商业版",
            "licenseType": "",
            "durationDays": 0,
            "price": 12.34,
            "maxSites": 0,
            "sort": 1,
            "enabled": True,
            "remark": "e2e",
        },
        token,
        ca,
    )
    must_api(
        base,
        "PUT",
        "/api/system/payment-config",
        {
            "easypayEnabled": True,
            "easypayGateway": "https://pay.example.test/submit.php",
            "easypayPid": EPAY_PID,
            "easypayMerchantKey": EPAY_KEY,
            "easypayDefaultType": "alipay",
            "easypayPayTypes": ["alipay", "wxpay"],
        },
        token,
        ca,
    )
    must_api(base, "POST", "/api/user/create", {"email": BUYER_EMAIL, "nickname": "买家", "password": BUYER_PASS}, token, ca)
    generated = must_api(base, "POST", "/api/app/store-snapshot-key", {}, token, ca)
    if "已生成签名密钥" not in str(generated.get("msg", "")):
        raise Fail(f"unexpected keygen response {generated}")


def assert_sale_ready(base: str, token: str, ca: str) -> None:
    apps = must_api(base, "GET", "/api/app/list", token=token, ca=ca)["data"]
    product = [item for item in apps if item.get("commercialProduct")]
    if len(product) != 1:
        raise Fail(f"expected one commercial product, got {apps}")
    gaps = product[0].get("saleGaps") or []
    if gaps:
        raise Fail(f"sale gaps remain: {gaps}")
    if product[0].get("name") != "商店产品":
        raise Fail(product[0])


def buyer_prepare(base: str, token: str, ca: str) -> None:
    must_api(
        base,
        "PUT",
        "/api/store/settings",
        {"sourceBase": f"https://{SOURCE_HOST}", "siteUrl": f"https://{BUYER_HOST}", "trustProxy": False},
        token,
        ca,
    )
    must_api(
        base,
        "POST",
        "/api/app/create",
        {"name": "买家主应用", "enabled": True, "remark": "first", "purchaseLicenseTypes": ["domain"]},
        token,
        ca,
    )


def notify_pending(database: str, ca: str) -> dict:
    saw_bind = False
    order_no = ""
    amount = 0
    deadline = time.time() + 90
    while time.time() < deadline:
        sources = sql_query(database, "SELECT GROUP_CONCAT(source) FROM licenses", required=False)
        if sources and "store_bind" in sources.split(","):
            saw_bind = True
        row = sql_query(
            database,
            "SELECT order_no, amount_cents FROM store_purchase_orders WHERE status='pending' ORDER BY id DESC LIMIT 1",
            required=False,
        )
        if row:
            order_no, amount_text = row.split("\t")
            amount = int(amount_text)
            break
        time.sleep(0.4)
    if not saw_bind:
        raise Fail("授权列表在支付前没有出现商店绑定")
    if not order_no:
        raise Fail("没有等到待支付的商业版订单")
    time.sleep(4)
    money = f"{amount // 100}.{amount % 100:02d}"
    params = {
        "pid": EPAY_PID,
        "type": "alipay",
        "out_trade_no": order_no,
        "trade_no": "TEST" + order_no,
        "trade_status": "TRADE_SUCCESS",
        "money": money,
        "name": "永久商业版",
    }
    params["sign"] = sign_epay(params, EPAY_KEY)
    params["sign_type"] = "MD5"
    code, parsed, raw = http_json(f"https://{SOURCE_HOST}/api/payment/easypay/notify", "POST", form=params, ca=ca)
    if raw.strip() != b"success":
        raise Fail(f"notify failed {code} {raw[:400]!r} {parsed!r}")
    return {"orderNo": order_no, "sawBind": True}


def wait_commercial_and_second_app(buyer_base: str, token: str, ca: str) -> None:
    deadline = time.time() + 40
    edition = ""
    while time.time() < deadline:
        account = must_api(buyer_base, "GET", "/api/store/account", token=token, ca=ca)["data"]
        edition = account.get("edition") or ""
        if edition == "commercial" and "multi_app" in (account.get("features") or []):
            break
        time.sleep(1)
    else:
        raise Fail(f"买家站没有自动拿到商业版凭证，当前 {edition}")
    created = must_api(
        buyer_base,
        "POST",
        "/api/app/create",
        {"name": "买家第二个应用", "enabled": True, "remark": "second", "purchaseLicenseTypes": ["domain"]},
        token,
        ca,
    )
    if created.get("code") not in (200, "200"):
        raise Fail(created)


def assert_source_records(base: str, token: str, ca: str, database: str) -> None:
    bought = must_api(base, "GET", "/api/license/list?source=store_purchase&page=1&pageSize=20", token=token, ca=ca)["data"]["list"]
    if not any(item.get("sourceLabel") == "商店购买" for item in bought):
        raise Fail(f"授权列表没有商店购买: {bought}")
    bound = must_api(base, "GET", "/api/license/list?source=store_bind&page=1&pageSize=20", token=token, ca=ca)["data"]["list"]
    if bound:
        raise Fail(f"支付后原绑定授权应改为商店购买，仍看到 {bound}")
    orders = must_api(base, "GET", "/api/system/payment-orders?subjectType=store_edition&page=1&pageSize=20", token=token, ca=ca)["data"]
    rows = orders.get("list") or []
    if not any(item.get("subjectType") == "store_edition" and item.get("status") == "paid" for item in rows):
        raise Fail(f"订单列表没有已支付商业版: {orders}")
    if float(orders.get("commercialPaidYuan") or 0) <= 0:
        raise Fail(f"商业版合计不正确: {orders}")
    apps = sql_query(database, "SELECT COUNT(*) FROM apps WHERE commercial_product=1")
    if apps != "1":
        raise Fail("源站商业版产品标记不正确")


def titles_of(nodes) -> list[str]:
    found = []
    for node in nodes or []:
        meta = node.get("meta") or {}
        if meta.get("title"):
            found.append(meta["title"])
        found.extend(titles_of(node.get("children")))
    return found


def run_upgrade(ca: str) -> str:
    data = os.path.join(RUNTIME, "upgrade")
    shutil.rmtree(data, ignore_errors=True)
    start_server("upgrade158", BIN_158, 18083, UPGRADE_DB, data)
    base = "http://127.0.0.1:18083"
    token = install_instance(base, UPGRADE_DB)
    code, parsed, raw = http_json(base + "/api/v1/store/captcha", token=token)
    if not isinstance(parsed, dict) or parsed.get("code") not in (200, "200"):
        raise Fail(f"1.5.8 captcha/schema {raw[:500]!r}")
    created = must_api(base, "POST", "/api/app/create", {"name": "旧商店产品", "enabled": True, "remark": "old"}, token)
    app_id = int(created["data"]["id"])
    must_api(base, "POST", "/api/user/create", {"email": "old-buyer@example.com", "nickname": "旧买家", "password": BUYER_PASS}, token)
    user_id = sql_query(UPGRADE_DB, "SELECT id FROM users WHERE email='old-buyer@example.com'")
    sudo_mysql(
        f"""
        USE {UPGRADE_DB};
        UPDATE apps SET app_key='shop' WHERE id={app_id};
        INSERT INTO system_configs (`group`, `key`, value, description)
        VALUES ('store', 'store_product_app_key', ' shop ', '旧产品应用标识')
        ON DUPLICATE KEY UPDATE value=' shop ';
        INSERT INTO store_edition_plans (name, period, price_cents, enabled, sort)
        VALUES ('旧永久商业版', 'permanent', 9900, 1, 1);
        INSERT INTO licenses (license_no, app_id, type, status, source, owner_type, owner_id, duration_days, started_at, license_key, max_domains, remark)
        VALUES ('LIC-OLD-158', {app_id}, 'domain', 'active', 'store_bind', 'user', {user_id}, 0, NOW(), '', 0, '旧商店授权');
        INSERT INTO license_domains (license_id, domain, is_wildcard)
        SELECT id, 'old-shop.example.com', 0 FROM licenses WHERE license_no='LIC-OLD-158';
        INSERT INTO store_purchase_orders
          (order_no, owner_type, owner_id, license_id, binding_id, item_kind, item_id, period, amount_cents, price_cents_snapshot, title_snapshot, status, expires_at, paid_at)
        SELECT 'PPOLD158', 'user', {user_id}, id, 'sb_old', 'edition', '1', 'permanent', 9900, 9900, '旧永久商业版', 'paid', DATE_ADD(NOW(), INTERVAL 1 DAY), NOW()
        FROM licenses WHERE license_no='LIC-OLD-158';
        """
    )
    menus_before = sql_query(UPGRADE_DB, "SELECT COUNT(*) FROM menus WHERE name IN ('SourceStationEdition','SourceStationStoreOrders','SourceStationStoreLicenses','SourceStationStoreRevenue')")
    if menus_before != "4":
        raise Fail(f"1.5.8 应有 4 条旧商店菜单，实际 {menus_before}")
    stop_named("upgrade158")
    start_server("upgrade159", BIN_159, 18083, UPGRADE_DB, data)
    token = must_api(base, "POST", "/api/auth/login", {"userName": ADMIN_USER, "password": ADMIN_PASS})["data"]["token"]
    apps = must_api(base, "GET", "/api/app/list", token=token)["data"]
    product = [item for item in apps if item.get("commercialProduct")]
    if len(product) != 1 or product[0].get("appKey") != "shop":
        raise Fail(f"迁移后商业版产品不正确: {apps}")
    price = sql_query(UPGRADE_DB, "SELECT CAST(price AS CHAR), duration_days, remark FROM license_plans WHERE name='旧永久商业版'")
    if price.split("\t") != ["99.00", "0", "由商业版价格迁移"] and not price.startswith("99"):
        # MariaDB may return 99.00 or 99.0000 depending on scale.
        parts = price.split("\t")
        if len(parts) != 3 or parts[1] != "0" or parts[2] != "由商业版价格迁移" or not parts[0].startswith("99"):
            raise Fail(f"旧价格迁移不正确: {price}")
    menu = must_api(base, "GET", "/api/system/menus", token=token)["data"]
    titles = titles_of(menu)
    menus_after = sql_query(UPGRADE_DB, "SELECT COUNT(*) FROM menus WHERE name IN ('SourceStationEdition','SourceStationStoreOrders','SourceStationStoreLicenses','SourceStationStoreRevenue')")
    if menus_after != "0":
        raise Fail(f"旧菜单未清除: {menus_after} titles={titles}")
    kept = sql_query(UPGRADE_DB, "SELECT source FROM licenses WHERE license_no='LIC-OLD-158'")
    if kept != "store_bind":
        raise Fail(f"旧授权被改写: {kept}")
    old_order = sql_query(UPGRADE_DB, "SELECT status, title_snapshot FROM store_purchase_orders WHERE order_no='PPOLD158'")
    if old_order != "paid\t旧永久商业版":
        raise Fail(f"旧订单丢失: {old_order}")
    for banned in ("商业版设置", "商店订单", "主授权与权益", "商业版收入"):
        if banned in titles:
            raise Fail(f"侧栏仍有旧菜单 {banned}: {titles}")
    for required in ("源站运营", "软件目录", "入驻审核", "公开目录", "广告投放", "源站设置"):
        if required not in titles:
            raise Fail(f"侧栏缺少中文菜单 {required}: {titles}")
    log("upgrade 1.5.8 -> 1.5.9 ok")
    return token


def run_shots(state_path: str) -> None:
    env = os.environ.copy()
    env["AUTH_PRO_E2E_STATE"] = state_path
    proc = run(["node", "scripts/commercial-mysql-shots.mjs"], cwd=os.path.join(ROOT, "frontend"), capture_output=True, env=env)
    sys.stdout.write(proc.stdout or "")
    sys.stderr.write(proc.stderr or "")
    if proc.returncode != 0:
        raise Fail("screenshot script failed")


def main() -> int:
    if not mysql_server_up():
        log("SKIP MySQL 不可用")
        return 77
    log("build frontend")
    built = run(["pnpm", "exec", "vite", "build"], cwd=os.path.join(ROOT, "frontend"), capture_output=True)
    if built.returncode != 0:
        raise Fail((built.stderr or built.stdout or "vite build failed")[-2000:])
    os.makedirs(RUNTIME, exist_ok=True)
    os.makedirs(ART, exist_ok=True)
    log("setup databases")
    setup_databases()
    log("setup network")
    ca = setup_network()
    log("build 1.5.9 and 1.5.8")
    build_159()
    build_158()
    source_data = os.path.join(RUNTIME, "source")
    buyer_data = os.path.join(RUNTIME, "buyer")
    shutil.rmtree(source_data, ignore_errors=True)
    shutil.rmtree(buyer_data, ignore_errors=True)
    start_server("source", BIN_159, 18081, SOURCE_DB, source_data)
    start_server("buyer", BIN_159, 18082, BUYER_DB, buyer_data)
    source = f"https://{SOURCE_HOST}"
    buyer = f"https://{BUYER_HOST}"
    # 确认 Go 与本脚本都能校验证书。安装走本机端口，避免安装接口的来源限制。
    code, parsed, raw = http_json(source + "/api/install/status", ca=ca)
    if code != 200:
        raise Fail(f"https status failed {raw[:300]!r}")
    source_token = install_instance("http://127.0.0.1:18081", SOURCE_DB)
    buyer_token = install_instance("http://127.0.0.1:18082", BUYER_DB)
    log("configure source")
    configure_source(source, source_token, ca)
    pub = public_key_from_private(source_data)
    log(f"rebuild with test public key {pub}")
    stop_named()
    build_159(pub)
    start_server("source", BIN_159, 18081, SOURCE_DB, source_data)
    start_server("buyer", BIN_159, 18082, BUYER_DB, buyer_data)
    source_token = must_api(source, "POST", "/api/auth/login", {"userName": ADMIN_USER, "password": ADMIN_PASS}, ca=ca)["data"]["token"]
    buyer_token = must_api(buyer, "POST", "/api/auth/login", {"userName": ADMIN_USER, "password": ADMIN_PASS}, ca=ca)["data"]["token"]
    assert_sale_ready(source, source_token, ca)
    buyer_prepare(buyer, buyer_token, ca)
    log("upgrade migration")
    run_upgrade(ca)
    state = {
        "source": source,
        "buyer": buyer,
        "upgrade": "http://127.0.0.1:18083",
        "adminUser": ADMIN_USER,
        "adminPass": ADMIN_PASS,
        "buyerEmail": BUYER_EMAIL,
        "buyerPass": BUYER_PASS,
        "ca": ca,
        "artifacts": ART,
    }
    state_path = os.path.join(RUNTIME, "state.json")
    open(state_path, "w", encoding="utf-8").write(json.dumps(state))
    import threading

    errors: list[BaseException] = []

    def payer() -> None:
        try:
            notify_pending(SOURCE_DB, ca)
            wait_commercial_and_second_app(buyer, buyer_token, ca)
            assert_source_records(source, source_token, ca, SOURCE_DB)
        except BaseException as exc:  # noqa: BLE001 - surface after the UI run
            errors.append(exc)

    thread = threading.Thread(target=payer, daemon=True)
    thread.start()
    log("browser flow")
    try:
        run_shots(state_path)
    finally:
        thread.join(timeout=120)
    if errors:
        raise errors[0]
    if thread.is_alive():
        raise Fail("支付线程没有结束")
    log("commercial mysql e2e ok")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Fail as exc:
        log("FAIL " + str(exc))
        for name in ("source", "buyer", "upgrade158", "upgrade159"):
            path = os.path.join(RUNTIME, f"{name}.log")
            if os.path.exists(path):
                log(f"----- {name}.log -----")
                log(tail(path))
        nginx_log = os.path.join(RUNTIME, "nginx-error.log")
        if os.path.exists(nginx_log):
            log("----- nginx -----")
            log(tail(nginx_log))
        sys.exit(1)
