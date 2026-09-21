package handler

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

type sdkPackTemplateData struct {
	AppID           int64
	AppName         string
	AppKey          string
	BaseURL         string
	PluginIndexURL  string
	PluginIndexPath string
	License         bool
	Piracy          bool
	Update          bool
	Ads             bool
	PluginSource    bool
	ModuleLabels    []string
	GeneratedAt     string
}

type sdkPackExampleFile struct {
	name string
	body string
}

var sdkPackTemplateFuncs = template.FuncMap{
	"join": func(items []string, sep string) string { return strings.Join(items, sep) },
	"phpComment": func(value string) string {
		return strings.ReplaceAll(value, "*/", "* /")
	},
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

func renderSDKPackExample(lang string, data sdkPackTemplateData) (sdkPackExampleFile, error) {
	raw, ok := sdkPackExampleTemplates[lang]
	if !ok {
		return sdkPackExampleFile{}, fmt.Errorf("未知示例语言：%s", lang)
	}
	body, err := renderSDKPackTemplate("example-"+lang, raw.body, data)
	if err != nil {
		return sdkPackExampleFile{}, err
	}
	return sdkPackExampleFile{name: raw.name, body: body}, nil
}

const sdkPackReadmeTemplate = `# AuthPro 客户端接入包（[[.AppName]]）

> 本包已取代旧版「单文件 PHP/JS 模板汤」接入包。核心库在仓库 ` + "`" + `sdk/*` + "`" + `，本 ZIP 只预填本应用的 ` + "`" + `config.json` + "`" + `，并把五语言库快照到 ` + "`" + `vendor/` + "`" + `，方便离线 require/import。

## 本应用信息

- 授权站：` + "`" + `[[.BaseURL]]` + "`" + `
- 应用 ID：[[.AppID]]
- appKey：` + "`" + `[[.AppKey]]` + "`" + `
- 已选模块：[[join .ModuleLabels "、"]]
[[if .PluginSource]]
- 插件源清单（应用隔离）：` + "`" + `[[.PluginIndexURL]]` + "`" + `
[[end]]

## 目录结构

` + "```" + `text
auth-pro-client-*/
  README.md
  config.json                 # 本应用预填（含服务端 appSecret）
  examples/{php,node,python,go,browser}/
  vendor/{php,node,python,go,browser}/   # sdk/* 快照
` + "```" + `

换应用时**只改** ` + "`" + `config.json` + "`" + `（或重新下载接入包），不要改 ` + "`" + `vendor/` + "`" + ` 源码。

## 五语言公共 API

| API | 说明 |
| --- | --- |
| ` + "`" + `boot(config)` + "`" + ` | 加载配置；开启 license/piracy 时校验失败会拦截/抛错 |
| ` + "`" + `verify()` + "`" + ` | 仅授权校验，返回 ` + "`" + `{ ok, code, message, data }` + "`" + `，不强制退出 |
| ` + "`" + `checkUpdate(currentVersion)` + "`" + ` | 对照 ` + "`" + `/api/app/version/check` + "`" + ` |
| ` + "`" + `ads(slot)` + "`" + ` | 广告位：home-banner / sidebar / popup |
| ` + "`" + `pluginSourceUrl()` + "`" + ` | 本应用 ` + "`" + `/software-source/{appKey}/index.json` + "`" + ` |

## 快速开始

### PHP

` + "```" + `php
require __DIR__ . '/vendor/php/src/AuthPro.php';
AuthPro::boot(__DIR__ . '/config.json');
$result = AuthPro::verify();
` + "```" + `

### Node.js

` + "```" + `js
const AuthPro = require('./vendor/node/src/index.js');
await AuthPro.boot('./config.json');
const result = await AuthPro.verify();
` + "```" + `

### Python

` + "```" + `python
import sys
sys.path.insert(0, "vendor/python")
import authpro
authpro.boot("config.json")
print(authpro.verify())
` + "```" + `

### Go

` + "```" + `go
import authpro "github.com/maizll/auth-pro/sdk/go/authpro"
// 离线包可将 vendor/go 以 replace 引入，或直接复制 authpro 包
_ = authpro.Boot("config.json")
` + "```" + `

### 浏览器

浏览器 vendor **不会**写入 ` + "`" + `appSecret` + "`" + `。请用 ` + "`" + `examples/browser/config.json` + "`" + `（已去密钥）。` + "`" + `ads` + "`" + ` / ` + "`" + `pluginSourceUrl` + "`" + ` 可直连；` + "`" + `verify` + "`" + ` / ` + "`" + `checkUpdate` + "`" + ` 请走服务端 SDK 或配置 ` + "`" + `proxyVerifyUrl` + "`" + ` / ` + "`" + `proxyCheckUpdateUrl` + "`" + ` 同源代理。

生成时间：[[.GeneratedAt]]
`

type sdkPackExampleTemplate struct {
	name string
	body string
}

var sdkPackExampleTemplates = map[string]sdkPackExampleTemplate{
	"php": {
		name: "boot.php",
		body: `<?php
/**
 * AuthPro PHP 示例 — [[phpComment .AppName]]
 * 将本目录上一级的 config.json + vendor/php 拷入业务项目即可。
 */
require dirname(__DIR__, 2) . '/vendor/php/src/AuthPro.php';

AuthPro::boot(dirname(__DIR__, 2) . '/config.json');

// 仅校验，不退出：
$result = AuthPro::verify();
if (empty($result['ok'])) {
    fwrite(STDERR, 'verify failed: ' . ($result['message'] ?? '') . PHP_EOL);
}

[[if .Ads]]print_r(AuthPro::ads('home-banner'));
[[end]][[if .Update]]$update = AuthPro::checkUpdate('1.0.0');
[[end]][[if .PluginSource]]echo AuthPro::pluginSourceUrl(), PHP_EOL;
[[end]]
`,
	},
	"node": {
		name: "boot.js",
		body: `'use strict';
/**
 * AuthPro Node 示例 — [[.AppName]]
 */
const path = require('path');
const AuthPro = require('../../vendor/node/src/index.js');

(async () => {
  await AuthPro.boot(path.join(__dirname, '../../config.json'));
  const result = await AuthPro.verify();
  console.log('verify', result);
[[if .Ads]]  console.log('ads', await AuthPro.ads('home-banner'));
[[end]][[if .Update]]  console.log('update', await AuthPro.checkUpdate('1.0.0'));
[[end]][[if .PluginSource]]  console.log('pluginSource', AuthPro.pluginSourceUrl());
[[end]]})().catch((err) => {
  console.error(err);
  process.exitCode = 1;
});
`,
	},
	"python": {
		name: "boot.py",
		body: `#!/usr/bin/env python3
# AuthPro Python 示例 — [[.AppName]]
import os
import sys

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
sys.path.insert(0, os.path.join(ROOT, "vendor", "python"))

import authpro

authpro.boot(os.path.join(ROOT, "config.json"))
print("verify", authpro.verify())
[[if .Ads]]print("ads", authpro.ads("home-banner"))
[[end]][[if .Update]]print("update", authpro.check_update("1.0.0"))
[[end]][[if .PluginSource]]print("pluginSource", authpro.plugin_source_url())
[[end]]
`,
	},
	"go": {
		name: "boot.go",
		body: `package main

// AuthPro Go 示例 — [[.AppName]]
// 离线使用：将 vendor/go 放到 GOPATH/module，或：
//   go mod edit -replace github.com/maizll/auth-pro/sdk/go=../../vendor/go

import (
	"fmt"
	"path/filepath"
	"runtime"

	authpro "github.com/maizll/auth-pro/sdk/go/authpro"
)

func main() {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if err := authpro.Boot(filepath.Join(root, "config.json")); err != nil {
		panic(err)
	}
	result, err := authpro.Verify()
	if err != nil {
		panic(err)
	}
	fmt.Printf("verify: %+v\n", result)
[[if .Ads]]	ads, _ := authpro.Ads("home-banner")
	fmt.Printf("ads: %+v\n", ads)
[[end]][[if .Update]]	update, _ := authpro.CheckUpdate("1.0.0")
	fmt.Printf("update: %+v\n", update)
[[end]][[if .PluginSource]]	url, _ := authpro.PluginSourceURL()
	fmt.Println("pluginSource", url)
[[end]]}
`,
	},
	"browser": {
		name: "index.html",
		body: `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <title>AuthPro Browser 示例 — [[.AppName]]</title>
</head>
<body>
  <h1>AuthPro Browser SDK</h1>
  <p>本页使用 <code>examples/browser/config.json</code>（不含 appSecret）。授权/更新请走服务端或代理。</p>
  <pre id="out"></pre>
  <script src="../../vendor/browser/src/auth-pro.js"></script>
  <script>
    fetch('./config.json').then(function (r) { return r.json(); }).then(function (cfg) {
      return AuthPro.boot(cfg).then(function () {
        var lines = [];
        lines.push('pluginSourceUrl = ' + AuthPro.pluginSourceUrl());
        return AuthPro.ads('home-banner').then(function (ads) {
          lines.push('ads = ' + JSON.stringify(ads));
          return AuthPro.verify();
        }).then(function (result) {
          lines.push('verify = ' + JSON.stringify(result));
          document.getElementById('out').textContent = lines.join('\n');
        });
      });
    }).catch(function (err) {
      document.getElementById('out').textContent = String(err);
    });
  </script>
</body>
</html>
`,
	},
}
