'use strict';
/**
 * AuthPro 浏览器端 SDK
 *
 * - 不内置 / 不要求 config 携带 appSecret
 * - ads / pluginSourceUrl 可直接调用公开接口
 * - verify / checkUpdate 为真实实现，但需要服务端代理签名；浏览器侧缺少签名时返回结构化错误
 * - boot：开启 license/piracy 时若无法完成服务端校验，抛出结构化错误（不渲染拦截页依赖 DOM 密钥）
 * - verify 经代理时带上浏览器生成的 nonce，代理交给服务端 SDK 的 verify({ nonce })；
 *   浏览器支持 WebCrypto Ed25519 时再用 config.publicKey 验一遍授权站签名，并在 localStorage 缓存通过结果，
 *   连不上时按 offlineGrace 沿用。不支持时以代理那边服务端 SDK 的验签结果为准。
 */
(function (root, factory) {
  if (typeof module === 'object' && module.exports) {
    module.exports = factory();
  } else {
    root.AuthPro = factory();
  }
})(typeof self !== 'undefined' ? self : this, function () {
  var config = {};
  var booted = false;
  var LICENSE_PROOF_KIND = 'auth-pro-license-v3';
  var DEFAULT_CACHE_TTL = 300;
  var DEFAULT_OFFLINE_GRACE = 72 * 3600;

  function boot(next) {
    loadConfig(next || {});
    booted = true;
    if (moduleEnabled('license') || moduleEnabled('piracy')) {
      return verify().then(function (result) {
        // 浏览器缺少 appSecret 时不硬拦：授权校验应走服务端 SDK / 代理。
        if (!result.ok && result.data && result.data.reason === 'browser_no_app_secret') {
          return true;
        }
        if (!result.ok) {
          var err = new Error(result.message || '授权无效');
          err.result = result;
          throw err;
        }
        return true;
      });
    }
    return Promise.resolve(true);
  }

  function loadConfig(next) {
    if (typeof next === 'string') {
      throw new Error('浏览器 SDK 请传入配置对象，不要在前端读取含密钥的本地文件路径');
    }
    config = Object.assign({}, config, next || {});
    if (Object.prototype.hasOwnProperty.call(config, 'appSecret')) {
      delete config.appSecret;
    }
  }

  function ensureConfig() {
    if (!booted && Object.keys(config).length === 0) {
      throw new Error('请先调用 AuthPro.boot(config)');
    }
  }

  function cfg(key, fallback) {
    return Object.prototype.hasOwnProperty.call(config, key) ? config[key] : fallback;
  }

  function moduleEnabled(name) {
    var modules = cfg('modules', {});
    if (!modules || typeof modules !== 'object') return false;
    if (Object.prototype.hasOwnProperty.call(modules, name)) return Boolean(modules[name]);
    if (Array.isArray(modules)) {
      return modules.map(String).map(function (s) { return s.toLowerCase(); }).indexOf(String(name).toLowerCase()) >= 0;
    }
    return false;
  }

  function browserSigningBlocked(apiName) {
    return {
      ok: false,
      code: 403,
      message:
        '浏览器 SDK 不携带 appSecret。请用 PHP/Node/Python/Go SDK 在服务端调用 ' +
        apiName +
        '，或由贵站接口代理签名后再返回结果。',
      data: { reason: 'browser_no_app_secret', api: apiName },
    };
  }

  function verify() {
    ensureConfig();
    // 若调用方通过同源代理注入了已签名结果端点，可走 proxyVerifyUrl
    var proxy = cfg('proxyVerifyUrl', '');
    if (!proxy) {
      return Promise.resolve(browserSigningBlocked('verify'));
    }
    var ctx = requestContext();
    var nonce = randomNonce();
    var cacheKey = 'authpro:' + cfg('appKey', '') + ':' + ctx.domain;
    var now = Math.floor(Date.now() / 1000);
    return importPublicKey().then(function (key) {
      return readCache(cacheKey, key).then(function (entry) {
        if (entry && now - entry.savedAt < positiveOr(cfg('cacheTtl', 0), DEFAULT_CACHE_TTL)) {
          return entryResult(entry, 'cached');
        }
        return httpJson('POST', absoluteOrPath(proxy), {
          appKey: cfg('appKey', ''),
          domain: ctx.domain,
          licenseKey: ctx.licenseKey,
          nonce: nonce,
        }).then(function (body) {
          var result = normalizeResult(body, true);
          if (!key) return result;
          return verifyLicenseProof(key, nonce, body).then(function (serverTime) {
            if (serverTime === null) return offlineResult(cacheKey, key, body);
            if (result.ok) {
              writeCache(cacheKey, { savedAt: now, nonce: nonce, body: body });
            } else {
              removeCache(cacheKey);
            }
            return result;
          });
        }, function (err) {
          if (!key) throw err;
          return offlineResult(cacheKey, key, null, err);
        });
      });
    });
  }

  // offlineResult：连不上或验签不过时，宽限期内沿用 localStorage 里上次验签通过的结果，否则拒绝。
  function offlineResult(cacheKey, key, body, err) {
    return readCache(cacheKey, key).then(function (entry) {
      var now = Math.floor(Date.now() / 1000);
      if (entry && now <= entry.serverTime + positiveOr(cfg('offlineGrace', 0), DEFAULT_OFFLINE_GRACE) && (!entry.expireTs || now < entry.expireTs)) {
        return entryResult(entry, 'offline');
      }
      if (err) throw err;
      var result = normalizeResult(body, true);
      result.ok = false;
      result.data = Object.assign({}, result.data || {}, { unverified: true });
      if (!result.message) result.message = '授权响应无法验证';
      return result;
    });
  }

  // importPublicKey 返回 CryptoKey；没配 publicKey 或浏览器不支持 Ed25519 时返回 null。
  function importPublicKey() {
    var encoded = String(cfg('publicKey', '') || '').trim();
    var subtle = typeof crypto !== 'undefined' && crypto.subtle;
    if (!encoded || !subtle) return Promise.resolve(null);
    var raw;
    try {
      raw = base64Bytes(encoded);
    } catch (e) {
      return Promise.resolve(null);
    }
    if (raw.length !== 32) return Promise.resolve(null);
    return subtle.importKey('raw', raw, { name: 'Ed25519' }, false, ['verify']).catch(function () {
      return null;
    });
  }

  // verifyLicenseProof：appKey、nonce 必须是自己这次发出的；域名、IP、授权码哈希由代理那边的服务端 SDK 决定，取 proof 里的值。
  // 通过返回签名里的服务器时间，不通过返回 null。
  function verifyLicenseProof(key, nonce, body) {
    var data = body && body.data;
    var proof = data && data.proof;
    if (!proof || proof.appKey !== String(cfg('appKey', '')) || proof.nonce !== nonce ||
        typeof proof.serverTime !== 'number' || typeof proof.signature !== 'string' || proof.signature.indexOf('ed25519:') !== 0) {
      return Promise.resolve(null);
    }
    var fields = [
      ['appKey', proof.appKey],
      ['domain', text(proof.domain)],
      ['serverIp', text(proof.serverIp)],
      ['licenseKeyHash', text(proof.licenseKeyHash)],
      ['nonce', nonce],
      ['serverTime', String(Math.trunc(proof.serverTime))],
      ['result', text(data.result)],
      ['reason', text(data.reason)],
      ['expireTs', typeof data.expireTs === 'number' ? String(Math.trunc(data.expireTs)) : ''],
    ];
    var message = LICENSE_PROOF_KIND + '\n';
    for (var i = 0; i < fields.length; i++) {
      message += fields[i][0] + '=' + fields[i][1].replace(/[\r\n]/g, ' ') + '\n';
    }
    var sig;
    try {
      sig = base64Bytes(proof.signature.slice(8));
    } catch (e) {
      return Promise.resolve(null);
    }
    return crypto.subtle.verify({ name: 'Ed25519' }, key, sig, new TextEncoder().encode(message)).then(function (ok) {
      return ok ? Math.trunc(proof.serverTime) : null;
    }, function () {
      return null;
    });
  }

  function readCache(cacheKey, key) {
    if (!key || typeof localStorage === 'undefined') return Promise.resolve(null);
    var entry;
    try {
      entry = JSON.parse(localStorage.getItem(cacheKey) || 'null');
    } catch (e) {
      return Promise.resolve(null);
    }
    if (!entry || !normalizeResult(entry.body, true).ok) return Promise.resolve(null);
    return verifyLicenseProof(key, entry.nonce, entry.body).then(function (serverTime) {
      if (serverTime === null) return null;
      var expireTs = typeof entry.body.data.expireTs === 'number' ? entry.body.data.expireTs : 0;
      return { savedAt: Number(entry.savedAt) || 0, body: entry.body, serverTime: serverTime, expireTs: expireTs };
    });
  }

  function writeCache(cacheKey, entry) {
    try {
      localStorage.setItem(cacheKey, JSON.stringify(entry));
    } catch (e) {
      // 存不了缓存不影响这次结果。
    }
  }

  function removeCache(cacheKey) {
    try {
      localStorage.removeItem(cacheKey);
    } catch (e) {
      // 忽略
    }
  }

  function entryResult(entry, flag) {
    var result = normalizeResult(entry.body, true);
    var extra = {};
    extra[flag] = true;
    result.data = Object.assign({}, result.data || {}, extra);
    return result;
  }

  function randomNonce() {
    var bytes = new Uint8Array(16);
    crypto.getRandomValues(bytes);
    return Array.prototype.map.call(bytes, function (b) {
      return ('0' + b.toString(16)).slice(-2);
    }).join('');
  }

  function base64Bytes(value) {
    var binary = atob(value);
    var out = new Uint8Array(binary.length);
    for (var i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
    return out;
  }

  function positiveOr(value, fallback) {
    var n = Number(value);
    return n > 0 ? n : fallback;
  }

  function text(value) {
    return typeof value === 'string' ? value : '';
  }

  function checkUpdate(currentVersion) {
    ensureConfig();
    var proxy = cfg('proxyCheckUpdateUrl', '');
    if (proxy) {
      return httpJson('POST', absoluteOrPath(proxy), {
        appKey: cfg('appKey', ''),
        currentVersion: String(currentVersion || cfg('appVersion', '1.0.0')),
      }).then(function (body) {
        return normalizeResult(body, false);
      });
    }
    return Promise.resolve(browserSigningBlocked('checkUpdate'));
  }

  function ads(slot) {
    ensureConfig();
    var position = (slot && String(slot).trim()) || 'home-banner';
    return httpJson(
      'GET',
      '/api/v1/public/advertisements?position=' + encodeURIComponent(position),
      null
    ).then(function (body) {
      return (body && body.data) || { records: [], placeholder: null };
    });
  }

  function pluginSourceUrl() {
    ensureConfig();
    var base = String(cfg('baseUrl', '')).replace(/\/$/, '');
    var appKey = String(cfg('appKey', '')).trim();
    return base + '/software-source/' + encodeURIComponent(appKey) + '/index.json';
  }

  function requestContext() {
    var domain = normalizeDomain(cfg('domain', '') || '');
    if (!domain && typeof location !== 'undefined') {
      domain = normalizeDomain(location.hostname || '');
    }
    return {
      licenseKey: String(cfg('licenseKey', '') || ''),
      domain: domain,
      serverIp: normalizeServerIP(cfg('serverIp', '') || ''),
    };
  }

  function normalizeDomain(value) {
    value = String(value || '').toLowerCase().replace(/^https?:\/\//, '').replace(/\/.*$/, '');
    if (value.charAt(0) === '[') value = value.replace(/^\[/, '').replace(/\]$/, '');
    value = value.replace(/:\d+$/, '');
    return value.replace(/\.+$/, '');
  }

  function normalizeServerIP(value) {
    value = String(value || '').trim();
    var zone = value.lastIndexOf('%');
    if (zone >= 0) value = value.slice(0, zone);
    return value.replace(/^\[/, '').replace(/\]$/, '');
  }

  function normalizeResult(response, requirePass) {
    response = response || {};
    var code = Number(response.code || 0);
    var message = response.msg || response.message || '';
    var data = response.data != null ? response.data : null;
    var ok = code === 200;
    if (requirePass) ok = ok && data && data.result === 'pass';
    return { ok: ok, code: code, message: message, data: data };
  }

  function absoluteOrPath(path) {
    if (/^https?:\/\//i.test(path)) return path;
    return path;
  }

  function httpJson(method, path, payload) {
    var url = path;
    if (path.charAt(0) === '/') {
      url = String(cfg('baseUrl', '')).replace(/\/$/, '') + path;
    }
    var init = { method: method, headers: { Accept: 'application/json' } };
    if (method === 'POST') {
      init.headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(payload || {});
    }
    return fetch(url, init).then(function (res) {
      return res.json();
    });
  }

  return {
    boot: boot,
    loadConfig: loadConfig,
    verify: verify,
    checkUpdate: checkUpdate,
    ads: ads,
    pluginSourceUrl: pluginSourceUrl,
  };
});
