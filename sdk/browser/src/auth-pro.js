'use strict';
/**
 * AuthPro 浏览器端 SDK
 *
 * - 不内置 / 不要求 config 携带 appSecret
 * - ads / pluginSourceUrl 可直接调用公开接口
 * - verify / checkUpdate 为真实实现，但需要服务端代理签名；浏览器侧缺少签名时返回结构化错误
 * - boot：开启 license/piracy 时若无法完成服务端校验，抛出结构化错误（不渲染拦截页依赖 DOM 密钥）
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
    if (proxy) {
      return httpJson('POST', absoluteOrPath(proxy), {
        appKey: cfg('appKey', ''),
        domain: requestContext().domain,
        licenseKey: requestContext().licenseKey,
      }).then(function (body) {
        return normalizeResult(body, true);
      });
    }
    return Promise.resolve(browserSigningBlocked('verify'));
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
