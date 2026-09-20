package handler

import (
	"bytes"
	"encoding/json"
	"strings"
	"text/template"
)

type sdkPackTemplateData struct {
	AppID           int64
	AppName         string
	AppKey          string
	AppSecret       string
	BaseURL         string
	PluginIndexURL  string
	PluginIndexPath string
	License         bool
	Piracy          bool
	Update          bool
	Ads             bool
	PluginSource    bool
	Guard           bool
	NeedHTTP        bool
	NeedSign        bool
	IncludeJS       bool
	ModuleLabels    []string
	GeneratedAt     string
}

func phpSingleQuote(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	return "'" + replacer.Replace(value) + "'"
}

func jsQuote(value string) string {
	payload, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(payload)
}

func phpBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func jsBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

var sdkPackTemplateFuncs = template.FuncMap{
	"php":     phpSingleQuote,
	"js":      jsQuote,
	"phpBool": phpBool,
	"jsBool":  jsBool,
	"join":    func(items []string, sep string) string { return strings.Join(items, sep) },
}

func renderSDKPackTemplate(name, raw string, data sdkPackTemplateData) (string, error) {
	tmpl, err := template.New(name).Delims("[[", "]]").Funcs(sdkPackTemplateFuncs).Parse(raw)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

const sdkPackConfigExampleTemplate = `<?php
/**
 * 复制本文件为 auth_pro_config.php（与 auth_pro_sdk.php 同目录），按需填写。
 * 密钥授权请填写 licenseKey；域名/IP 授权可留空，SDK 会读取当前站点。
 */
return array(
    'licenseKey' => '',
    'domain' => '',
    'serverIp' => '',
    'appVersion' => '1.0.0',
);
`

const sdkPackReadmeTemplate = `# AuthPro 接入包（[[.AppName]]）

本包已绑定当前授权站上的**这一个应用**，不要用于其它应用。

- 授权站：` + "`" + `[[.BaseURL]]` + "`" + `
- 应用 ID：[[.AppID]]
- appKey：` + "`" + `[[.AppKey]]` + "`" + `
- 已选模块：[[join .ModuleLabels "、"]]
[[if .PluginSource]]
- 插件源清单（应用隔离，直接填到消费者「软件源管理」）：` + "`" + `[[.PluginIndexURL]]` + "`" + `
[[end]]

## PHP（推荐，宝塔 / 常规 PHP 产品）

1. 把本目录复制到项目，例如 ` + "`" + `includes/auth-pro-sdk/` + "`" + `。
2. 复制 ` + "`" + `config.example.php` + "`" + ` 为 ` + "`" + `auth_pro_config.php` + "`" + `，填写授权码（密钥授权必填；域名授权可留空）。
3. 在站点入口增加两行：

` + "```" + `php
require __DIR__ . '/includes/auth-pro-sdk/auth_pro_sdk.php';
AuthPro::boot();
` + "```" + `

` + "`" + `AuthPro::boot()` + "`" + ` 会按本包勾选的模块工作：授权失败时直接中断（盗版入口开启时展示拦截页）。可选调用：

[[if .Ads]]- ` + "`" + `AuthPro::ads('home-banner')` + "`" + `（` + "`" + `sidebar` + "`" + ` / ` + "`" + `popup` + "`" + `）
[[end]][[if .Update]]- ` + "`" + `AuthPro::checkUpdate('1.0.0')` + "`" + ` 对照本站 ` + "`" + `/api/app/version/check` + "`" + `
[[end]][[if .PluginSource]]- ` + "`" + `AuthPro::pluginSourceUrl()` + "`" + ` 返回本应用清单 ` + "`" + `[[.PluginIndexPath]]` + "`" + `
[[end]]

## Node / JS（可选）
[[if .IncludeJS]]
` + "```" + `js
const AuthPro = require('./auth-pro-sdk.js');
AuthPro.boot();
` + "```" + `
[[else]]
本包未包含 JS 文件。重新生成时勾选「同时生成 Node/JS」。
[[end]]

生成时间：[[.GeneratedAt]]
`

const sdkPackPHPTemplate = `<?php
/**
 * AuthPro 客户端 SDK
 * 已绑定应用：[[.AppName]]（appId=[[.AppID]] / appKey=[[.AppKey]]）
 * 授权站：[[.BaseURL]]
 *
 * require 本文件后调用 AuthPro::boot();
 */
if (class_exists('AuthPro', false)) {
    return;
}

class AuthPro
{
    const BASE_URL = [[php .BaseURL]];
    const APP_ID = [[.AppID]];
    const APP_KEY = [[php .AppKey]];
    const APP_SECRET = [[php .AppSecret]];
    const MODULE_LICENSE = [[phpBool .License]];
    const MODULE_PIRACY = [[phpBool .Piracy]];
    const MODULE_UPDATE = [[phpBool .Update]];
    const MODULE_ADS = [[phpBool .Ads]];
    const MODULE_PLUGIN_SOURCE = [[phpBool .PluginSource]];
[[if .PluginSource]]
    // 应用隔离公开清单：{origin}/software-source/{app_key}/index.json（app_key 与授权 appKey 相同）
    const PLUGIN_SOURCE_URL = [[php .PluginIndexURL]];
[[end]]

    private static $options = array();

    public static function configure($options = array())
    {
        if (!is_array($options)) {
            $options = array();
        }
        $local = self::loadLocalConfig();
        self::$options = array_merge($local, $options);
        return true;
    }

    public static function boot($options = array())
    {
        self::configure($options);
[[if .Guard]]
        $result = self::verify();
        if (empty($result['ok'])) {
            self::deny($result);
        }
[[end]]
        return true;
    }
[[if .Guard]]

    public static function verify()
    {
        $ctx = self::requestContext();
        $timestamp = time();
        $payload = array(
            'appKey' => self::APP_KEY,
            'domain' => $ctx['domain'],
            'serverIp' => $ctx['serverIp'],
            'licenseKey' => $ctx['licenseKey'],
            'timestamp' => $timestamp,
            'signVersion' => 'v2',
            'sign' => self::v2Sign(array(
                'v2',
                self::APP_KEY,
                $ctx['licenseKey'],
                $ctx['domain'],
                $ctx['serverIp'],
                (string)$timestamp,
            )),
        );
        $response = self::httpJson('POST', '/api/license/verify', $payload);
        $ok = isset($response['code']) && (int)$response['code'] === 200
            && isset($response['data']['result']) && $response['data']['result'] === 'pass';
        $response['ok'] = $ok;
        return $response;
    }

    public static function deny($result)
    {
        $reason = '';
        if (is_array($result)) {
            if (!empty($result['msg'])) {
                $reason = (string)$result['msg'];
            } elseif (!empty($result['data']['reason'])) {
                $reason = (string)$result['data']['reason'];
            }
        }
        $title = '授权无效';
        $hint = '请检查授权码、域名是否与后台登记一致。';
[[if .Piracy]]
        $title = '未授权访问';
        $hint = '当前站点未通过授权校验。授权站会记录未授权命中，请联系软件提供方开通正版。';
[[end]]
        if (PHP_SAPI === 'cli') {
            fwrite(STDERR, $title . ': ' . $reason . PHP_EOL);
            exit(1);
        }
        if (!headers_sent()) {
            http_response_code(403);
            header('Content-Type: text/html; charset=utf-8');
        }
        echo '<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>'
            . htmlspecialchars($title, ENT_QUOTES, 'UTF-8')
            . '</title><style>body{font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;background:#0f172a;color:#e2e8f0;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0}main{max-width:520px;padding:32px;background:#1e293b;border-radius:16px}h1{margin:0 0 12px;font-size:24px}p{line-height:1.7;color:#cbd5e1}</style></head><body><main><h1>'
            . htmlspecialchars($title, ENT_QUOTES, 'UTF-8')
            . '</h1><p>'
            . htmlspecialchars($hint, ENT_QUOTES, 'UTF-8')
            . '</p><p>'
            . htmlspecialchars($reason, ENT_QUOTES, 'UTF-8')
            . '</p></main></body></html>';
        exit;
    }
[[end]]
[[if .Ads]]

    public static function ads($position = 'home-banner')
    {
        $position = trim((string)$position);
        if ($position === '') {
            $position = 'home-banner';
        }
        $response = self::httpJson('GET', '/api/v1/public/advertisements?position=' . rawurlencode($position), null);
        if (isset($response['data']) && is_array($response['data'])) {
            return $response['data'];
        }
        return array('records' => array(), 'placeholder' => null);
    }
[[end]]
[[if .Update]]

    public static function checkUpdate($version = null)
    {
        $ctx = self::requestContext();
        if ($version === null || $version === '') {
            $version = isset(self::$options['appVersion']) ? self::$options['appVersion'] : '1.0.0';
        }
        $timestamp = time();
        $payload = array(
            'appKey' => self::APP_KEY,
            'currentVersion' => (string)$version,
            'domain' => $ctx['domain'],
            'serverIp' => $ctx['serverIp'],
            'licenseKey' => $ctx['licenseKey'],
            'timestamp' => $timestamp,
            'signVersion' => 'v2',
            'sign' => self::v2Sign(array(
                'v2',
                self::APP_KEY,
                (string)$version,
                $ctx['licenseKey'],
                $ctx['domain'],
                $ctx['serverIp'],
                (string)$timestamp,
            )),
        );
        $response = self::httpJson('POST', '/api/app/version/check', $payload);
        if (isset($response['data']['downloadUrl']) && is_string($response['data']['downloadUrl'])
            && strpos($response['data']['downloadUrl'], 'http') !== 0) {
            $response['data']['downloadUrl'] = self::BASE_URL . $response['data']['downloadUrl'];
        }
        return $response;
    }
[[end]]
[[if .PluginSource]]

    public static function pluginSourceUrl()
    {
        return self::PLUGIN_SOURCE_URL;
    }
[[end]]

    private static function loadLocalConfig()
    {
        $path = dirname(__FILE__) . '/auth_pro_config.php';
        if (!is_file($path)) {
            return array();
        }
        $data = include $path;
        return is_array($data) ? $data : array();
    }

    private static function requestContext()
    {
        $licenseKey = '';
        $domain = '';
        $serverIp = '';
        if (isset(self::$options['licenseKey'])) {
            $licenseKey = trim((string)self::$options['licenseKey']);
        }
        if (isset(self::$options['domain'])) {
            $domain = self::normalizeDomain(self::$options['domain']);
        }
        if (isset(self::$options['serverIp'])) {
            $serverIp = trim((string)self::$options['serverIp']);
        }
        if ($domain === '' && !empty($_SERVER['HTTP_HOST'])) {
            $domain = self::normalizeDomain($_SERVER['HTTP_HOST']);
        }
        if ($serverIp === '' && !empty($_SERVER['SERVER_ADDR'])) {
            $serverIp = trim((string)$_SERVER['SERVER_ADDR']);
        }
        return array(
            'licenseKey' => $licenseKey,
            'domain' => $domain,
            'serverIp' => $serverIp,
        );
    }

    private static function normalizeDomain($value)
    {
        $value = strtolower(trim((string)$value));
        $value = preg_replace('#^https?://#', '', $value);
        $value = preg_replace('#/.*$#', '', $value);
        $value = preg_replace('#:\d+$#', '', $value);
        return $value;
    }
[[if .NeedSign]]

    private static function v2Sign($parts)
    {
        return hash_hmac('sha256', implode("\n", $parts), self::APP_SECRET);
    }
[[end]]
[[if .NeedHTTP]]

    private static function httpJson($method, $path, $payload)
    {
        $url = self::BASE_URL . $path;
        $body = $payload === null ? null : json_encode($payload);
        if (function_exists('curl_init')) {
            $ch = curl_init($url);
            $headers = array('Accept: application/json');
            $opts = array(
                CURLOPT_RETURNTRANSFER => true,
                CURLOPT_TIMEOUT => 8,
                CURLOPT_FOLLOWLOCATION => true,
            );
            if ($method === 'POST') {
                $headers[] = 'Content-Type: application/json';
                $opts[CURLOPT_POST] = true;
                $opts[CURLOPT_POSTFIELDS] = $body;
            }
            $opts[CURLOPT_HTTPHEADER] = $headers;
            curl_setopt_array($ch, $opts);
            $raw = curl_exec($ch);
            curl_close($ch);
        } else {
            $header = "Accept: application/json\r\n";
            $http = array('method' => $method, 'timeout' => 8, 'ignore_errors' => true);
            if ($method === 'POST') {
                $header .= "Content-Type: application/json\r\n";
                $http['content'] = $body;
            }
            $http['header'] = $header;
            $raw = @file_get_contents($url, false, stream_context_create(array('http' => $http)));
        }
        $decoded = json_decode($raw ? $raw : '{}', true);
        return is_array($decoded) ? $decoded : array();
    }
[[end]]
}
`

const sdkPackJSTemplate = `'use strict';
(function (root, factory) {
  if (typeof module === 'object' && module.exports) {
    module.exports = factory();
  } else {
    root.AuthPro = factory();
  }
}(typeof self !== 'undefined' ? self : this, function () {
  var cfg = {
    baseUrl: [[js .BaseURL]],
    appId: [[.AppID]],
    appKey: [[js .AppKey]],
    appSecret: [[js .AppSecret]],
    modules: {
      license: [[jsBool .License]],
      piracy: [[jsBool .Piracy]],
      update: [[jsBool .Update]],
      ads: [[jsBool .Ads]],
      pluginSource: [[jsBool .PluginSource]]
    }[[if .PluginSource]],
    pluginSourceUrl: [[js .PluginIndexURL]][[end]]
  };
  var options = {};

  function configure(next) {
    options = Object.assign({}, options, next || {});
    return exports;
  }

  function boot(next) {
    configure(next);
[[if .Guard]]
    return verify().then(function (result) {
      if (!result.ok) {
        deny(result);
      }
      return result;
    });
[[else]]
    return Promise.resolve(true);
[[end]]
  }
[[if .NeedSign]]

  function v2Sign(parts) {
    return hmacSha256Hex(cfg.appSecret, parts.join('\n'));
  }
[[end]]
[[if .Guard]]

  function verify() {
    var ctx = requestContext();
    var timestamp = Math.floor(Date.now() / 1000);
    var payload = {
      appKey: cfg.appKey,
      domain: ctx.domain,
      serverIp: ctx.serverIp,
      licenseKey: ctx.licenseKey,
      timestamp: timestamp,
      signVersion: 'v2',
      sign: v2Sign(['v2', cfg.appKey, ctx.licenseKey, ctx.domain, ctx.serverIp, String(timestamp)])
    };
    return httpJson('POST', '/api/license/verify', payload).then(function (body) {
      body.ok = body && body.code === 200 && body.data && body.data.result === 'pass';
      return body;
    });
  }

  function deny(result) {
    var reason = (result && (result.msg || (result.data && result.data.reason))) || '';
    var message = [[if .Piracy]]'未授权访问'[[else]]'授权无效'[[end]] + (reason ? ': ' + reason : '');
    if (typeof document !== 'undefined') {
      document.body.innerHTML = '<main style="font-family:sans-serif;padding:48px;text-align:center"><h1>' + message + '</h1></main>';
    }
    throw new Error(message);
  }
[[end]]
[[if .Ads]]

  function ads(position) {
    position = position || 'home-banner';
    return httpJson('GET', '/api/v1/public/advertisements?position=' + encodeURIComponent(position), null).then(function (body) {
      return (body && body.data) || { records: [], placeholder: null };
    });
  }
[[end]]
[[if .Update]]

  function checkUpdate(version) {
    var ctx = requestContext();
    version = version || options.appVersion || '1.0.0';
    var timestamp = Math.floor(Date.now() / 1000);
    var payload = {
      appKey: cfg.appKey,
      currentVersion: String(version),
      domain: ctx.domain,
      serverIp: ctx.serverIp,
      licenseKey: ctx.licenseKey,
      timestamp: timestamp,
      signVersion: 'v2',
      sign: v2Sign(['v2', cfg.appKey, String(version), ctx.licenseKey, ctx.domain, ctx.serverIp, String(timestamp)])
    };
    return httpJson('POST', '/api/app/version/check', payload);
  }
[[end]]
[[if .PluginSource]]

  function pluginSourceUrl() {
    return cfg.pluginSourceUrl;
  }
[[end]]

  function requestContext() {
    var domain = String(options.domain || '');
    var serverIp = String(options.serverIp || '');
    if (typeof location !== 'undefined' && !domain) {
      domain = location.hostname || '';
    }
    return {
      licenseKey: String(options.licenseKey || ''),
      domain: domain.toLowerCase(),
      serverIp: serverIp
    };
  }
[[if .NeedHTTP]]

  function httpJson(method, path, payload) {
    var url = cfg.baseUrl + path;
    if (typeof fetch === 'function') {
      var init = { method: method, headers: { Accept: 'application/json' } };
      if (method === 'POST') {
        init.headers['Content-Type'] = 'application/json';
        init.body = JSON.stringify(payload);
      }
      return fetch(url, init).then(function (res) { return res.json(); });
    }
    var http = require(url.indexOf('https://') === 0 ? 'https' : 'http');
    return new Promise(function (resolve, reject) {
      var req = http.request(url, {
        method: method,
        headers: method === 'POST'
          ? { 'Content-Type': 'application/json', Accept: 'application/json' }
          : { Accept: 'application/json' }
      }, function (res) {
        var chunks = [];
        res.on('data', function (c) { chunks.push(c); });
        res.on('end', function () {
          try { resolve(JSON.parse(Buffer.concat(chunks).toString('utf8') || '{}')); }
          catch (err) { reject(err); }
        });
      });
      req.on('error', reject);
      if (method === 'POST') req.write(JSON.stringify(payload));
      req.end();
    });
  }
[[end]]
[[if .NeedSign]]

  function hmacSha256Hex(secret, text) {
    var crypto = require('crypto');
    return crypto.createHmac('sha256', secret).update(text).digest('hex');
  }
[[end]]

  var exports = {
    boot: boot,
    configure: configure,
    config: cfg
  };
[[if .Guard]]
  exports.verify = verify;
[[end]]
[[if .Ads]]
  exports.ads = ads;
[[end]]
[[if .Update]]
  exports.checkUpdate = checkUpdate;
[[end]]
[[if .PluginSource]]
  exports.pluginSourceUrl = pluginSourceUrl;
[[end]]
  return exports;
}));
`
