'use strict';

const crypto = require('crypto');
const fs = require('fs');
const http = require('http');
const https = require('https');
const { URL } = require('url');

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

function verify(overrides) {
  ensureConfig();
  const ctx = requestContext(overrides);
  const timestamp = Math.floor(Date.now() / 1000);
  const payload = {
    appKey: cfg('appKey', ''),
    domain: ctx.domain,
    serverIp: ctx.serverIp,
    licenseKey: ctx.licenseKey,
    timestamp,
    signVersion: 'v2',
    sign: v2Sign(['v2', cfg('appKey', ''), ctx.licenseKey, ctx.domain, ctx.serverIp, String(timestamp)]),
  };
  return httpJson('POST', '/api/license/verify', payload).then((body) => normalizeResult(body, true));
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
