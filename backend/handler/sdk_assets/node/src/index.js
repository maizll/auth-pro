'use strict';

const crypto = require('crypto');
const fs = require('fs');
const http = require('http');
const https = require('https');
const os = require('os');
const path = require('path');
const { URL } = require('url');

// 授权响应签名的用途标记，签名原文第一行。
const LICENSE_PROOF_KIND = 'auth-pro-license-v3';
// 校验通过的结果默认缓存 5 分钟；连不上授权站时最近一次验签通过的结果最多再用 72 小时（从授权站签名时间算起）。
const DEFAULT_CACHE_TTL = 300;
const DEFAULT_OFFLINE_GRACE = 72 * 3600;
// Ed25519 公钥的 SPKI DER 前缀，后面接 32 字节原始公钥。
const ED25519_SPKI_PREFIX = Buffer.from('302a300506032b6570032100', 'hex');

let config = {};
let booted = false;

function boot(nextConfig) {
  loadConfig(nextConfig);
  booted = true;
  if (moduleEnabled('license') || moduleEnabled('piracy')) {
    return verify().then((result) => {
      if (!result.ok) {
        deny(result);
      }
      return true;
    });
  }
  return Promise.resolve(true);
}

function loadConfig(nextConfig) {
  if (typeof nextConfig === 'string') {
    const raw = fs.readFileSync(nextConfig, 'utf8');
    config = JSON.parse(raw || '{}');
    return;
  }
  config = Object.assign({}, config, nextConfig || {});
}

function ensureConfig() {
  if (!booted && Object.keys(config).length === 0) {
    throw new Error('请先调用 boot(config) 或 loadConfig(config)');
  }
}

function cfg(key, fallback) {
  return Object.prototype.hasOwnProperty.call(config, key) ? config[key] : fallback;
}

function moduleEnabled(name) {
  const modules = cfg('modules', {});
  if (!modules || typeof modules !== 'object') return false;
  if (Object.prototype.hasOwnProperty.call(modules, name)) {
    return Boolean(modules[name]);
  }
  if (Array.isArray(modules)) {
    return modules.map(String).map((s) => s.toLowerCase()).includes(String(name).toLowerCase());
  }
  return false;
}

// verify 校验授权。overrides 可覆盖 licenseKey / domain / serverIp；
// 浏览器代理转发时再传浏览器给的 nonce，这时不读写本机缓存，结果原样交给浏览器验签。
function verify(overrides) {
  ensureConfig();
  let publicKey;
  try {
    publicKey = loadPublicKey();
  } catch (err) {
    return Promise.reject(err);
  }
  overrides = overrides || {};
  const ctx = requestContext(overrides);
  const relay = Boolean(overrides.nonce && String(overrides.nonce).trim());
  const nonce = relay ? String(overrides.nonce).trim() : crypto.randomBytes(16).toString('hex');
  const file = cachePath(ctx);
  const now = Math.floor(Date.now() / 1000);
  if (!relay) {
    const entry = readCache(file, ctx, publicKey);
    if (entry && now - entry.savedAt < positiveOr(cfg('cacheTtl', 0), DEFAULT_CACHE_TTL)) {
      return Promise.resolve(entryResult(entry, 'cached'));
    }
  }
  let sign;
  try {
    sign = v2Sign(['v3', cfg('appKey', ''), ctx.licenseKey, ctx.domain, ctx.serverIp, String(now), nonce]);
  } catch (err) {
    return Promise.reject(err);
  }
  const payload = {
    appKey: cfg('appKey', ''),
    domain: ctx.domain,
    serverIp: ctx.serverIp,
    licenseKey: ctx.licenseKey,
    timestamp: now,
    signVersion: 'v3',
    nonce,
    sign,
  };
  return httpJson('POST', '/api/license/verify', payload).then(
    (body) => {
      if (verifyLicenseProof(publicKey, ctx.licenseKey, nonce, body) === null) {
        return offlineResult(file, ctx, publicKey, relay, body, null);
      }
      const result = normalizeResult(body, true);
      if (!relay) {
        if (result.ok) {
          writeCache(file, { savedAt: now, nonce, body });
        } else {
          // 授权站明确拒绝（签名有效）：立刻失效，不再用旧缓存放行。
          try { fs.unlinkSync(file); } catch (err) { /* 没有缓存 */ }
        }
      }
      return result;
    },
    (err) => offlineResult(file, ctx, publicKey, relay, null, err)
  );
}

// offlineResult 处理连不上授权站或响应验签不过：宽限期内沿用上次验签通过的结果，否则拒绝。
// 网络错误且没有可用缓存时抛错；验签不过时返回 ok=false，data.unverified=true。
function offlineResult(file, ctx, publicKey, relay, body, netErr) {
  if (!relay) {
    const entry = readCache(file, ctx, publicKey);
    const now = Math.floor(Date.now() / 1000);
    if (entry && now <= entry.serverTime + positiveOr(cfg('offlineGrace', 0), DEFAULT_OFFLINE_GRACE) && (!entry.expireTs || now < entry.expireTs)) {
      return entryResult(entry, 'offline');
    }
  }
  if (netErr) throw netErr;
  const result = normalizeResult(body, true);
  result.ok = false;
  result.data = Object.assign({}, result.data || {}, { unverified: true });
  if (!result.message) result.message = '授权响应无法验证';
  return result;
}

function loadPublicKey() {
  const raw = Buffer.from(String(cfg('publicKey', '') || '').trim(), 'base64');
  if (raw.length !== 32) {
    throw new Error('缺少或无效的 publicKey，请在授权站后台重新下载接入包，或从「接入开发」页复制授权响应公钥填入 config.json');
  }
  return crypto.createPublicKey({ key: Buffer.concat([ED25519_SPKI_PREFIX, raw]), format: 'der', type: 'spki' });
}

// verifyLicenseProof 核对 data.proof：appKey、nonce、授权码哈希必须是自己这次发出的，
// 再按接入文档的规则拼出原文用公钥验签。域名和 IP 取 proof 里授权站规范化后的值，它们已在 v3 请求签名里和 nonce 绑在一起。
// 通过返回签名里的服务器时间，不通过返回 null。
function verifyLicenseProof(publicKey, licenseKey, nonce, body) {
  const data = body && body.data;
  const proof = data && data.proof;
  if (!proof || typeof proof !== 'object') return null;
  const appKey = String(cfg('appKey', ''));
  const keyHash = licenseKey ? crypto.createHash('sha256').update(licenseKey).digest('hex') : '';
  if (proof.appKey !== appKey || proof.nonce !== nonce || proof.licenseKeyHash !== keyHash) return null;
  if (typeof proof.serverTime !== 'number' || typeof proof.signature !== 'string' || proof.signature.indexOf('ed25519:') !== 0) return null;
  const fields = [
    ['appKey', appKey],
    ['domain', text(proof.domain)],
    ['serverIp', text(proof.serverIp)],
    ['licenseKeyHash', keyHash],
    ['nonce', nonce],
    ['serverTime', String(Math.trunc(proof.serverTime))],
    ['result', text(data.result)],
    ['reason', text(data.reason)],
    ['expireTs', typeof data.expireTs === 'number' ? String(Math.trunc(data.expireTs)) : ''],
  ];
  let message = LICENSE_PROOF_KIND + '\n';
  for (const [key, value] of fields) {
    message += key + '=' + value.replace(/[\r\n]/g, ' ') + '\n';
  }
  const signature = Buffer.from(proof.signature.slice('ed25519:'.length), 'base64');
  try {
    return crypto.verify(null, Buffer.from(message, 'utf8'), publicKey, signature) ? Math.trunc(proof.serverTime) : null;
  } catch (err) {
    return null;
  }
}

// 校验缓存写在临时目录，读出时重新验签，手改文件没有用。
function cachePath(ctx) {
  const key = [cfg('baseUrl', ''), cfg('appKey', ''), ctx.licenseKey, ctx.domain, ctx.serverIp].join('\n');
  return path.join(os.tmpdir(), 'authpro-' + crypto.createHash('sha256').update(key).digest('hex').slice(0, 24) + '.json');
}

function readCache(file, ctx, publicKey) {
  let entry;
  try {
    entry = JSON.parse(fs.readFileSync(file, 'utf8'));
  } catch (err) {
    return null;
  }
  if (!entry || !normalizeResult(entry.body, true).ok) return null;
  const serverTime = verifyLicenseProof(publicKey, ctx.licenseKey, entry.nonce, entry.body);
  if (serverTime === null) return null;
  const expireTs = entry.body.data && typeof entry.body.data.expireTs === 'number' ? entry.body.data.expireTs : 0;
  return { savedAt: Number(entry.savedAt) || 0, nonce: entry.nonce, body: entry.body, serverTime, expireTs };
}

function writeCache(file, entry) {
  try {
    fs.writeFileSync(file + '.tmp', JSON.stringify(entry), { mode: 0o600 });
    fs.renameSync(file + '.tmp', file);
  } catch (err) {
    // 写不了缓存不影响这次结果。
  }
}

function entryResult(entry, flag) {
  const result = normalizeResult(entry.body, true);
  result.data = Object.assign({}, result.data || {}, { [flag]: true });
  return result;
}

function positiveOr(value, fallback) {
  const n = Number(value);
  return n > 0 ? n : fallback;
}

function text(value) {
  return typeof value === 'string' ? value : '';
}

function checkUpdate(currentVersion, overrides) {
  ensureConfig();
  const ctx = requestContext(overrides);
  const version = currentVersion || cfg('appVersion', '1.0.0');
  const timestamp = Math.floor(Date.now() / 1000);
  const payload = {
    appKey: cfg('appKey', ''),
    currentVersion: String(version),
    domain: ctx.domain,
    serverIp: ctx.serverIp,
    licenseKey: ctx.licenseKey,
    timestamp,
    signVersion: 'v2',
    sign: v2Sign(['v2', cfg('appKey', ''), String(version), ctx.licenseKey, ctx.domain, ctx.serverIp, String(timestamp)]),
  };
  return httpJson('POST', '/api/app/version/check', payload).then((body) => {
    if (body && body.data && typeof body.data.downloadUrl === 'string' && body.data.downloadUrl.indexOf('http') !== 0) {
      body.data.downloadUrl = String(cfg('baseUrl', '')).replace(/\/$/, '') + body.data.downloadUrl;
    }
    return normalizeResult(body, false);
  });
}

function ads(slot) {
  ensureConfig();
  const position = (slot && String(slot).trim()) || 'home-banner';
  return httpJson('GET', '/api/v1/public/advertisements?position=' + encodeURIComponent(position), null).then((body) => {
    return (body && body.data) || { records: [], placeholder: null };
  });
}

function pluginSourceUrl() {
  ensureConfig();
  const base = String(cfg('baseUrl', '')).replace(/\/$/, '');
  const appKey = String(cfg('appKey', '')).trim();
  return base + '/software-source/' + encodeURIComponent(appKey) + '/index.json';
}

function requestContext(overrides) {
  overrides = overrides || {};
  let licenseKey = overrides.licenseKey != null ? String(overrides.licenseKey).trim() : String(cfg('licenseKey', '') || '');
  let domain = overrides.domain != null ? normalizeDomain(overrides.domain) : normalizeDomain(cfg('domain', '') || '');
  let serverIp = overrides.serverIp != null ? normalizeServerIP(overrides.serverIp) : normalizeServerIP(cfg('serverIp', '') || '');
  return { licenseKey, domain, serverIp };
}

function normalizeDomain(value) {
  value = String(value || '').toLowerCase().replace(/^https?:\/\//, '').replace(/\/.*$/, '');
  if (value.charAt(0) === '[') value = value.replace(/^\[/, '').replace(/\]$/, '');
  value = value.replace(/:\d+$/, '');
  return value.replace(/\.+$/, '');
}

function normalizeServerIP(value) {
  value = String(value || '').trim();
  const zone = value.lastIndexOf('%');
  if (zone >= 0) value = value.slice(0, zone);
  return value.replace(/^\[/, '').replace(/\]$/, '');
}

function v2Sign(parts) {
  const secret = String(cfg('appSecret', '') || '');
  if (!secret) {
    throw new Error('缺少 appSecret，无法签名。请在服务端 config.json 中配置。');
  }
  return crypto.createHmac('sha256', secret).update(parts.join('\n')).digest('hex');
}

function normalizeResult(response, requirePass) {
  response = response || {};
  const code = Number(response.code || 0);
  const message = response.msg || response.message || '';
  const data = response.data != null ? response.data : null;
  let ok = code === 200;
  if (requirePass) {
    ok = ok && data && data.result === 'pass';
  }
  return { ok, code, message, data };
}

function deny(result) {
  const reason = (result && (result.message || (result.data && result.data.reason))) || '';
  const title = moduleEnabled('piracy') ? '未授权访问' : '授权无效';
  const err = new Error(title + (reason ? ': ' + reason : ''));
  err.result = result;
  throw err;
}

function httpJson(method, path, payload) {
  const base = String(cfg('baseUrl', '')).replace(/\/$/, '');
  const url = new URL(base + path);
  const lib = url.protocol === 'https:' ? https : http;
  const body = payload == null ? null : JSON.stringify(payload);
  return new Promise((resolve, reject) => {
    const req = lib.request(
      {
        protocol: url.protocol,
        hostname: url.hostname,
        port: url.port || (url.protocol === 'https:' ? 443 : 80),
        path: url.pathname + url.search,
        method,
        headers:
          method === 'POST'
            ? { 'Content-Type': 'application/json', Accept: 'application/json', 'Content-Length': Buffer.byteLength(body || '') }
            : { Accept: 'application/json' },
        timeout: 8000,
      },
      (res) => {
        const chunks = [];
        res.on('data', (c) => chunks.push(c));
        res.on('end', () => {
          try {
            resolve(JSON.parse(Buffer.concat(chunks).toString('utf8') || '{}'));
          } catch (err) {
            reject(err);
          }
        });
      }
    );
    req.on('error', reject);
    req.on('timeout', () => req.destroy(new Error('请求授权站超时')));
    if (method === 'POST' && body) req.write(body);
    req.end();
  });
}

module.exports = {
  boot,
  loadConfig,
  verify,
  checkUpdate,
  ads,
  pluginSourceUrl,
};
