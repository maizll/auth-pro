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
	Language        string
	LanguageLabel   string
	DropInDir       string
	RequireFence    string
	RequireSnippet  string
	Browser         bool
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

const sdkPackReadmeTemplate = `# AuthPro [[.LanguageLabel]] 接入包（[[.AppName]]）

把本文件夹**整份**复制到你的项目（例如 ` + "`" + `[[.DropInDir]]/` + "`" + `），然后在核心入口引用下面几行即可。换应用时只改同目录的 ` + "`" + `config.json` + "`" + `，不要改库文件。

` + "```" + `[[.RequireFence]]
[[.RequireSnippet]]
` + "```" + `

## 本应用

- 授权站：` + "`" + `[[.BaseURL]]` + "`" + `
- 应用 ID：[[.AppID]]
- appKey：` + "`" + `[[.AppKey]]` + "`" + `
- 已选模块：[[join .ModuleLabels "、"]]
[[if .PluginSource]]
- 插件源清单（应用隔离）：` + "`" + `[[.PluginIndexURL]]` + "`" + `
[[end]]
[[if .Browser]]
本包 **不含 appSecret**。` + "`" + `ads` + "`" + ` / ` + "`" + `pluginSourceUrl` + "`" + ` 可直连；` + "`" + `verify` + "`" + ` / ` + "`" + `checkUpdate` + "`" + ` 请走服务端 SDK，或配置 ` + "`" + `proxyVerifyUrl` + "`" + ` / ` + "`" + `proxyCheckUpdateUrl` + "`" + ` 同源代理。
[[end]]
公共 API：` + "`" + `boot` + "`" + ` / ` + "`" + `verify` + "`" + ` / ` + "`" + `checkUpdate` + "`" + ` / ` + "`" + `ads` + "`" + ` / ` + "`" + `pluginSourceUrl` + "`" + `。

生成时间：[[.GeneratedAt]]
`

type sdkPackExampleTemplate struct {
	name string
	body string
}

var sdkPackExampleTemplates = map[string]sdkPackExampleTemplate{
	"php": {
		name: "example.php",
		body: `<?php
/**
 * AuthPro PHP 示例 — [[phpComment .AppName]]
 * 把本文件夹复制进项目后，在入口 require AuthPro.php 即可。
 */
require __DIR__ . '/AuthPro.php';

AuthPro::boot(__DIR__ . '/config.json');

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
		name: "example.js",
		body: `'use strict';
/**
 * AuthPro Node 示例 — [[.AppName]]
 */
const path = require('path');
const AuthPro = require('./index.js');

(async () => {
  await AuthPro.boot(path.join(__dirname, 'config.json'));
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
		name: "example.py",
		body: `#!/usr/bin/env python3
# AuthPro Python 示例 — [[.AppName]]
import os
import sys

ROOT = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, ROOT)

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
		name: "example.go",
		body: `package main

// AuthPro Go 示例 — [[.AppName]]
// 离线使用：go mod edit -replace github.com/maizll/auth-pro/sdk/go=.

import (
	"fmt"
	"path/filepath"
	"runtime"

	authpro "github.com/maizll/auth-pro/sdk/go/authpro"
)

func main() {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(file)
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
		name: "example.html",
		body: `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <title>AuthPro 浏览器示例 — [[.AppName]]</title>
</head>
<body>
  <h1>AuthPro 浏览器 SDK</h1>
  <p>同目录 <code>config.json</code> 不含 appSecret。授权/更新请走服务端或代理。</p>
  <pre id="out"></pre>
  <script src="./auth-pro.js"></script>
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
