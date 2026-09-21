"""AuthPro 客户端 SDK（Python）。

公共 API：boot / verify / check_update / ads / plugin_source_url
（同时提供 camelCase 别名以对齐其它语言。）
"""

from __future__ import annotations

import hashlib
import hmac
import json
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Mapping, MutableMapping, Optional, Union

ConfigInput = Union[str, Mapping[str, Any]]

_config: MutableMapping[str, Any] = {}
_booted = False


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
    """仅授权校验，不强制退出进程。返回 {ok, code, message, data}。"""
    _ensure_config()
    ctx = _request_context(overrides)
    timestamp = int(time.time())
    payload = {
        "appKey": _cfg("appKey", ""),
        "domain": ctx["domain"],
        "serverIp": ctx["serverIp"],
        "licenseKey": ctx["licenseKey"],
        "timestamp": timestamp,
        "signVersion": "v2",
        "sign": _v2_sign(
            ["v2", _cfg("appKey", ""), ctx["licenseKey"], ctx["domain"], ctx["serverIp"], str(timestamp)]
        ),
    }
    response = _http_json("POST", "/api/license/verify", payload)
    return _normalize_result(response, require_pass=True)


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


def _http_json(method: str, path: str, payload: Any) -> dict:
    base = str(_cfg("baseUrl", "")).rstrip("/")
    url = base + path
    data = None
    headers = {"Accept": "application/json"}
    if method == "POST":
        data = json.dumps(payload or {}).encode("utf-8")
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=8) as resp:
            raw = resp.read().decode("utf-8")
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8") if exc.fp else "{}"
    except Exception:
        return {}
    try:
        decoded = json.loads(raw or "{}")
    except json.JSONDecodeError:
        return {}
    return decoded if isinstance(decoded, dict) else {}
