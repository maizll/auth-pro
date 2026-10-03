"""AuthPro 客户端 SDK（Python）。

公共 API：boot / verify / check_update / ads / plugin_source_url
（同时提供 camelCase 别名以对齐其它语言。）
"""

from __future__ import annotations

import base64
import hashlib
import hmac
import json
import os
import secrets
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Mapping, MutableMapping, Optional, Union

ConfigInput = Union[str, Mapping[str, Any]]

_config: MutableMapping[str, Any] = {}
_booted = False

# 授权响应签名的用途标记，签名原文第一行。
_LICENSE_PROOF_KIND = "auth-pro-license-v3"
# 校验通过的结果默认缓存 5 分钟；连不上授权站时最近一次验签通过的结果最多再用 72 小时（从授权站签名时间算起）。
_DEFAULT_CACHE_TTL = 300
_DEFAULT_OFFLINE_GRACE = 72 * 3600


def boot(config: ConfigInput | None = None) -> bool:
    """加载配置；license/piracy 开启时校验失败会抛错（盗版模式结构化错误）。"""
    load_config(config or {})
    global _booted
    _booted = True
    if _module_enabled("license") or _module_enabled("piracy"):
        result = verify()
        if not result.get("ok"):
            _deny(result)
    return True


def load_config(config: ConfigInput) -> None:
    global _config
    if isinstance(config, str):
        with open(config, "r", encoding="utf-8") as fh:
            data = json.load(fh)
        if not isinstance(data, dict):
            raise ValueError("config.json 必须是对象")
        _config = data
        return
    merged = dict(_config)
    merged.update(dict(config or {}))
    _config = merged


def verify(overrides: Optional[Mapping[str, Any]] = None) -> dict:
    """仅授权校验，不强制退出进程。返回 {ok, code, message, data}。

    overrides 可覆盖 licenseKey / domain / serverIp；浏览器代理转发时再传浏览器给的 nonce，
    这时不读写本机缓存，结果原样交给浏览器验签。
    """
    _ensure_config()
    public_key = _public_key()
    overrides = dict(overrides or {})
    nonce = str(overrides.pop("nonce", "") or "").strip()
    relay = nonce != ""
    if not relay:
        nonce = secrets.token_hex(16)
    ctx = _request_context(overrides)
    cache_file = _cache_path(ctx)
    now = int(time.time())
    if not relay:
        entry = _read_cache(cache_file, ctx, public_key)
        if entry and now - entry["savedAt"] < _positive_or(_cfg("cacheTtl"), _DEFAULT_CACHE_TTL):
            return _entry_result(entry, "cached")
    payload = {
        "appKey": _cfg("appKey", ""),
        "domain": ctx["domain"],
        "serverIp": ctx["serverIp"],
        "licenseKey": ctx["licenseKey"],
        "timestamp": now,
        "signVersion": "v3",
        "nonce": nonce,
        "sign": _v2_sign(
            ["v3", _cfg("appKey", ""), ctx["licenseKey"], ctx["domain"], ctx["serverIp"], str(now), nonce]
        ),
    }
    try:
        response = _http_json("POST", "/api/license/verify", payload, raise_errors=True)
    except Exception:
        return _offline_result(cache_file, ctx, public_key, relay, None)
    if _verify_license_proof(public_key, ctx["licenseKey"], nonce, response) is None:
        return _offline_result(cache_file, ctx, public_key, relay, response)
    result = _normalize_result(response, require_pass=True)
    if not relay:
        if result["ok"]:
            _write_cache(cache_file, {"savedAt": now, "nonce": nonce, "body": response})
        else:
            # 授权站明确拒绝（签名有效）：立刻失效，不再用旧缓存放行。
            try:
                os.remove(cache_file)
            except OSError:
                pass
    return result


def _offline_result(cache_file: str, ctx: dict, public_key: bytes, relay: bool, response: Any) -> dict:
    """连不上授权站或响应验签不过：宽限期内沿用上次验签通过的结果，否则拒绝（data.unverified=True）。"""
    if not relay:
        entry = _read_cache(cache_file, ctx, public_key)
        now = int(time.time())
        grace = _positive_or(_cfg("offlineGrace"), _DEFAULT_OFFLINE_GRACE)
        if entry and now <= entry["serverTime"] + grace and (not entry["expireTs"] or now < entry["expireTs"]):
            return _entry_result(entry, "offline")
    result = _normalize_result(response, require_pass=True)
    result["ok"] = False
    data = dict(result["data"]) if isinstance(result["data"], dict) else {}
    data["unverified"] = True
    result["data"] = data
    if not result["message"]:
        result["message"] = "无法连接授权站" if response is None else "授权响应无法验证"
    return result


def _public_key() -> bytes:
    try:
        raw = base64.b64decode(str(_cfg("publicKey", "") or "").strip(), validate=True)
    except Exception:
        raw = b""
    if len(raw) != 32:
        raise RuntimeError("缺少或无效的 publicKey，请在授权站后台重新下载接入包，或从「接入开发」页复制授权响应公钥填入 config.json")
    return raw


def _verify_license_proof(public_key: bytes, license_key: str, nonce: str, body: Any) -> Optional[int]:
    """核对 data.proof：appKey、nonce、授权码哈希必须是自己这次发出的，再按接入文档的规则拼原文验签。

    域名和 IP 取 proof 里授权站规范化后的值，它们已在 v3 请求签名里和 nonce 绑在一起。
    通过返回签名里的服务器时间，不通过返回 None。
    """
    data = body.get("data") if isinstance(body, dict) else None
    proof = data.get("proof") if isinstance(data, dict) else None
    if not isinstance(proof, dict):
        return None
    app_key = str(_cfg("appKey", ""))
    key_hash = hashlib.sha256(license_key.encode("utf-8")).hexdigest() if license_key else ""
    if proof.get("appKey") != app_key or proof.get("nonce") != nonce or proof.get("licenseKeyHash") != key_hash:
        return None
    server_time = proof.get("serverTime")
    signature = proof.get("signature")
    if not isinstance(server_time, int) or isinstance(server_time, bool) or not isinstance(signature, str) or not signature.startswith("ed25519:"):
        return None
    expire_ts = data.get("expireTs")
    fields = [
        ("appKey", app_key),
        ("domain", _text(proof.get("domain"))),
        ("serverIp", _text(proof.get("serverIp"))),
        ("licenseKeyHash", key_hash),
        ("nonce", nonce),
        ("serverTime", str(server_time)),
        ("result", _text(data.get("result"))),
        ("reason", _text(data.get("reason"))),
        ("expireTs", str(expire_ts) if isinstance(expire_ts, int) and not isinstance(expire_ts, bool) else ""),
    ]
    message = _LICENSE_PROOF_KIND + "\n" + "".join(
        f"{key}={value.replace(chr(13), ' ').replace(chr(10), ' ')}\n" for key, value in fields
    )
    try:
        sig = base64.b64decode(signature[len("ed25519:"):], validate=True)
    except Exception:
        return None
    return server_time if _ed25519_verify(public_key, message.encode("utf-8"), sig) else None


def _cache_path(ctx: dict) -> str:
    """校验缓存写在临时目录，读出时重新验签，手改文件没有用。"""
    key = "\n".join([str(_cfg("baseUrl", "")), str(_cfg("appKey", "")), ctx["licenseKey"], ctx["domain"], ctx["serverIp"]])
    return os.path.join(tempfile.gettempdir(), "authpro-" + hashlib.sha256(key.encode("utf-8")).hexdigest()[:24] + ".json")


def _read_cache(cache_file: str, ctx: dict, public_key: bytes) -> Optional[dict]:
    try:
        with open(cache_file, "r", encoding="utf-8") as fh:
            entry = json.load(fh)
    except (OSError, ValueError):
        return None
    if not isinstance(entry, dict) or not _normalize_result(entry.get("body"), require_pass=True)["ok"]:
        return None
    server_time = _verify_license_proof(public_key, ctx["licenseKey"], str(entry.get("nonce") or ""), entry.get("body"))
    if server_time is None:
        return None
    expire_ts = entry["body"]["data"].get("expireTs")
    return {
        "savedAt": int(entry.get("savedAt") or 0),
        "body": entry["body"],
        "serverTime": server_time,
        "expireTs": expire_ts if isinstance(expire_ts, int) else 0,
    }


def _write_cache(cache_file: str, entry: dict) -> None:
    try:
        tmp = cache_file + ".tmp"
        fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            json.dump(entry, fh)
        os.replace(tmp, cache_file)
    except OSError:
        pass


def _entry_result(entry: dict, flag: str) -> dict:
    result = _normalize_result(entry["body"], require_pass=True)
    data = dict(result["data"]) if isinstance(result["data"], dict) else {}
    data[flag] = True
    result["data"] = data
    return result


def _positive_or(value: Any, fallback: int) -> int:
    try:
        number = int(value)
    except (TypeError, ValueError):
        return fallback
    return number if number > 0 else fallback


def _text(value: Any) -> str:
    return value if isinstance(value, str) else ""


# Ed25519 验签（RFC 8032），纯 Python 实现，不依赖第三方库。只做验签，不涉及私钥。
_P = 2**255 - 19
_Q = 2**252 + 27742317777372353535851937790883648493
_D = -121665 * pow(121666, _P - 2, _P) % _P
_SQRT_M1 = pow(2, (_P - 1) // 4, _P)


def _point_add(a: tuple, b: tuple) -> tuple:
    x1 = (a[1] - a[0]) * (b[1] - b[0]) % _P
    x2 = (a[1] + a[0]) * (b[1] + b[0]) % _P
    c = 2 * a[3] * b[3] * _D % _P
    d = 2 * a[2] * b[2] % _P
    e, f, g, h = x2 - x1, d - c, d + c, x2 + x1
    return (e * f % _P, g * h % _P, f * g % _P, e * h % _P)


def _point_mul(scalar: int, point: tuple) -> tuple:
    result = (0, 1, 1, 0)
    while scalar > 0:
        if scalar & 1:
            result = _point_add(result, point)
        point = _point_add(point, point)
        scalar >>= 1
    return result


def _point_equal(a: tuple, b: tuple) -> bool:
    return (a[0] * b[2] - b[0] * a[2]) % _P == 0 and (a[1] * b[2] - b[1] * a[2]) % _P == 0


def _recover_x(y: int, sign: int) -> Optional[int]:
    if y >= _P:
        return None
    x2 = (y * y - 1) * pow(_D * y * y + 1, _P - 2, _P) % _P
    if x2 == 0:
        return None if sign else 0
    x = pow(x2, (_P + 3) // 8, _P)
    if (x * x - x2) % _P != 0:
        x = x * _SQRT_M1 % _P
    if (x * x - x2) % _P != 0:
        return None
    if (x & 1) != sign:
        x = _P - x
    return x


def _decompress(raw: bytes) -> Optional[tuple]:
    if len(raw) != 32:
        return None
    y = int.from_bytes(raw, "little")
    sign = y >> 255
    y &= (1 << 255) - 1
    x = _recover_x(y, sign)
    if x is None:
        return None
    return (x, y, 1, x * y % _P)


_GY = 4 * pow(5, _P - 2, _P) % _P
_GX = _recover_x(_GY, 0)
_G = (_GX, _GY, 1, _GX * _GY % _P)


def _ed25519_verify(public_key: bytes, message: bytes, signature: bytes) -> bool:
    if len(public_key) != 32 or len(signature) != 64:
        return False
    a = _decompress(public_key)
    r = _decompress(signature[:32])
    if a is None or r is None:
        return False
    s = int.from_bytes(signature[32:], "little")
    if s >= _Q:
        return False
    h = int.from_bytes(hashlib.sha512(signature[:32] + public_key + message).digest(), "little") % _Q
    return _point_equal(_point_mul(s, _G), _point_add(r, _point_mul(h, a)))


def check_update(current_version: Optional[str] = None, overrides: Optional[Mapping[str, Any]] = None) -> dict:
    _ensure_config()
    ctx = _request_context(overrides)
    version = current_version or _cfg("appVersion", "1.0.0")
    timestamp = int(time.time())
    payload = {
        "appKey": _cfg("appKey", ""),
        "currentVersion": str(version),
        "domain": ctx["domain"],
        "serverIp": ctx["serverIp"],
        "licenseKey": ctx["licenseKey"],
        "timestamp": timestamp,
        "signVersion": "v2",
        "sign": _v2_sign(
            [
                "v2",
                _cfg("appKey", ""),
                str(version),
                ctx["licenseKey"],
                ctx["domain"],
                ctx["serverIp"],
                str(timestamp),
            ]
        ),
    }
    response = _http_json("POST", "/api/app/version/check", payload)
    data = response.get("data") if isinstance(response, dict) else None
    if isinstance(data, dict):
        download = data.get("downloadUrl")
        if isinstance(download, str) and not download.startswith("http"):
            data["downloadUrl"] = str(_cfg("baseUrl", "")).rstrip("/") + download
    return _normalize_result(response, require_pass=False)


def ads(slot: str = "home-banner") -> dict:
    _ensure_config()
    position = (slot or "").strip() or "home-banner"
    path = "/api/v1/public/advertisements?position=" + urllib.parse.quote(position)
    response = _http_json("GET", path, None)
    data = response.get("data") if isinstance(response, dict) else None
    if isinstance(data, dict):
        return data
    return {"records": [], "placeholder": None}


def plugin_source_url() -> str:
    _ensure_config()
    base = str(_cfg("baseUrl", "")).rstrip("/")
    app_key = str(_cfg("appKey", "")).strip()
    return f"{base}/software-source/{urllib.parse.quote(app_key)}/index.json"


# camelCase aliases for cross-language parity
checkUpdate = check_update
pluginSourceUrl = plugin_source_url


def _ensure_config() -> None:
    if not _booted and not _config:
        raise RuntimeError("请先调用 boot(config) 或 load_config(config)")


def _cfg(key: str, default: Any = None) -> Any:
    return _config[key] if key in _config else default


def _module_enabled(name: str) -> bool:
    modules = _cfg("modules", {})
    if isinstance(modules, dict):
        if name in modules:
            return bool(modules[name])
        return False
    if isinstance(modules, list):
        return any(str(item).lower() == name.lower() for item in modules)
    return False


def _request_context(overrides: Optional[Mapping[str, Any]] = None) -> dict:
    overrides = overrides or {}
    license_key = str(overrides.get("licenseKey", _cfg("licenseKey", "") or "")).strip()
    domain = _normalize_domain(overrides.get("domain", _cfg("domain", "") or ""))
    server_ip = _normalize_server_ip(overrides.get("serverIp", _cfg("serverIp", "") or ""))
    return {"licenseKey": license_key, "domain": domain, "serverIp": server_ip}


def _normalize_domain(value: Any) -> str:
    text = str(value or "").strip().lower()
    if text.startswith("http://") or text.startswith("https://"):
        text = text.split("://", 1)[1]
    text = text.split("/", 1)[0]
    if text.startswith("["):
        text = text.strip("[]")
    if ":" in text and not text.count(":") > 1:
        host, _, port = text.rpartition(":")
        if port.isdigit():
            text = host
    return text.rstrip(".")


def _normalize_server_ip(value: Any) -> str:
    text = str(value or "").strip()
    if "%" in text:
        text = text.split("%", 1)[0]
    return text.strip("[]")


def _v2_sign(parts: list[str]) -> str:
    secret = str(_cfg("appSecret", "") or "")
    if not secret:
        raise RuntimeError("缺少 appSecret，无法签名。请在服务端 config.json 中配置。")
    canonical = "\n".join(parts)
    return hmac.new(secret.encode("utf-8"), canonical.encode("utf-8"), hashlib.sha256).hexdigest()


def _normalize_result(response: Any, require_pass: bool) -> dict:
    if not isinstance(response, dict):
        response = {}
    code = int(response.get("code") or 0)
    message = str(response.get("msg") or response.get("message") or "")
    data = response.get("data")
    ok = code == 200
    if require_pass:
        ok = ok and isinstance(data, dict) and data.get("result") == "pass"
    return {"ok": ok, "code": code, "message": message, "data": data}


def _deny(result: Mapping[str, Any]) -> None:
    reason = str(result.get("message") or "")
    data = result.get("data")
    if not reason and isinstance(data, dict):
        reason = str(data.get("reason") or "")
    title = "未授权访问" if _module_enabled("piracy") else "授权无效"
    message = f"{title}: {reason}" if reason else title
    raise PermissionError(message)


def _http_json(method: str, path: str, payload: Any, raise_errors: bool = False) -> dict:
    """请求授权站。raise_errors=True 时连不上或响应不是 JSON 会抛错，授权校验据此进入离线宽限。"""
    base = str(_cfg("baseUrl", "")).rstrip("/")
    url = base + path
    data = None
    headers = {"Accept": "application/json"}
    if method == "POST":
        data = json.dumps(payload or {}).encode("utf-8")
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        try:
            with urllib.request.urlopen(request, timeout=8) as resp:
                raw = resp.read().decode("utf-8")
        except urllib.error.HTTPError as exc:
            raw = exc.read().decode("utf-8") if exc.fp else "{}"
        decoded = json.loads(raw or "{}")
    except Exception:
        if raise_errors:
            raise
        return {}
    return decoded if isinstance(decoded, dict) else {}
