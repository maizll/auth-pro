package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestSDKPackLanguageVerifyAgainstHTTPStub extracts the hybrid zip and drives
// PHP/Node/Python verify() against a local httptest stub (acceptance smoke).
func TestSDKPackLanguageVerifyAgainstHTTPStub(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/license/verify":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"result": "pass"}})
		case r.URL.Path == "/api/app/version/check":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"hasUpdate": false}})
		case r.URL.Path == "/api/v1/public/advertisements":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"records": []any{}, "placeholder": nil}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	input := testSDKPackInput([]string{
		sdkPackModuleLicense, sdkPackModulePiracy, sdkPackModuleUpdate, sdkPackModuleAds, sdkPackModulePluginSource,
	}, true)
	input.BaseURL = server.URL
	payload, _, err := buildSDKPack(input)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "pack.zip")
	if err := os.WriteFile(zipPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	extractDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("unzip", "-q", zipPath, "-d", extractDir).CombinedOutput(); err != nil {
		t.Fatalf("unzip: %v (%s)", err, out)
	}
	root := filepath.Join(extractDir, "auth-pro-client-app_demo_1")

	t.Run("php", func(t *testing.T) {
		if _, err := exec.LookPath("php"); err != nil {
			t.Skip("php not installed")
		}
		scriptPath := filepath.Join(t.TempDir(), "smoke.php")
		script := `<?php
require $argv[1];
AuthPro::boot($argv[2]);
$r = AuthPro::verify();
if (empty($r['ok'])) { fwrite(STDERR, json_encode($r)); exit(1); }
$ads = AuthPro::ads('home-banner');
if (!is_array($ads)) { fwrite(STDERR, 'ads'); exit(1); }
$u = AuthPro::checkUpdate('1.0.0');
if (!isset($u['code'])) { fwrite(STDERR, json_encode($u)); exit(1); }
echo AuthPro::pluginSourceUrl();
`
		if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("php", scriptPath, filepath.Join(root, "vendor/php/src/AuthPro.php"), filepath.Join(root, "config.json"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("php verify: %v (%s)", err, out)
		}
		if !strings.Contains(string(out), "/software-source/app_demo_1/index.json") {
			t.Fatalf("php plugin url=%s", out)
		}
	})

	t.Run("node", func(t *testing.T) {
		if _, err := exec.LookPath("node"); err != nil {
			t.Skip("node not installed")
		}
		script := `
const AuthPro = require(process.argv[1]);
(async () => {
  await AuthPro.boot(process.argv[2]);
  const r = await AuthPro.verify();
  if (!r.ok) { console.error(r); process.exit(1); }
  await AuthPro.ads('home-banner');
  await AuthPro.checkUpdate('1.0.0');
  console.log(AuthPro.pluginSourceUrl());
})().catch((e) => { console.error(e); process.exit(1); });
`
		cmd := exec.Command("node", "-e", script, filepath.Join(root, "vendor/node/src/index.js"), filepath.Join(root, "config.json"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("node verify: %v (%s)", err, out)
		}
		if !strings.Contains(string(out), "/software-source/app_demo_1/index.json") {
			t.Fatalf("node plugin url=%s", out)
		}
	})

	t.Run("python", func(t *testing.T) {
		if _, err := exec.LookPath("python3"); err != nil {
			t.Skip("python3 not installed")
		}
		script := `
import sys
sys.path.insert(0, sys.argv[1])
import authpro
authpro.boot(sys.argv[2])
r = authpro.verify()
assert r.get("ok"), r
authpro.ads("home-banner")
authpro.check_update("1.0.0")
print(authpro.plugin_source_url())
`
		cmd := exec.Command("python3", "-c", script, filepath.Join(root, "vendor/python"), filepath.Join(root, "config.json"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("python verify: %v (%s)", err, out)
		}
		if !strings.Contains(string(out), "/software-source/app_demo_1/index.json") {
			t.Fatalf("python plugin url=%s", out)
		}
	})

	t.Run("go", func(t *testing.T) {
		modRoot := filepath.Join(root, "vendor/go")
		// Use the repo sdk/go with replace to the extracted vendor for isolation.
		_, thisFile, _, _ := runtime.Caller(0)
		repoSDK := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../sdk/go"))
		if _, err := os.Stat(filepath.Join(modRoot, "authpro/authpro.go")); err != nil {
			t.Fatal(err)
		}
		testMain := filepath.Join(t.TempDir(), "main_test.go")
		content := `package authpro_smoke_test
import (
  "testing"
  authpro "github.com/maizll/auth-pro/sdk/go/authpro"
)
func TestSmoke(t *testing.T) {
  if err := authpro.Boot("` + filepath.ToSlash(filepath.Join(root, "config.json")) + `"); err != nil { t.Fatal(err) }
  r, err := authpro.Verify(); if err != nil || r == nil || !r.OK { t.Fatalf("%v %+v", err, r) }
  if _, err := authpro.Ads("home-banner"); err != nil { t.Fatal(err) }
  if _, err := authpro.CheckUpdate("1.0.0"); err != nil { t.Fatal(err) }
  u, err := authpro.PluginSourceURL(); if err != nil || u == "" { t.Fatal(err, u) }
}
`
		if err := os.WriteFile(testMain, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		// Prefer extracted vendor module.
		cmd := exec.Command("go", "test", "-count=1", ".")
		cmd.Dir = filepath.Dir(testMain)
		cmd.Env = append(os.Environ(), "GO111MODULE=on")
		// Initialize a tiny module that replaces to extracted vendor.
		init := exec.Command("go", "mod", "init", "authpro.smoke")
		init.Dir = filepath.Dir(testMain)
		if out, err := init.CombinedOutput(); err != nil {
			t.Fatalf("go mod init: %v (%s)", err, out)
		}
		edit := exec.Command("go", "mod", "edit", "-require=github.com/maizll/auth-pro/sdk/go@v0.0.0", "-replace=github.com/maizll/auth-pro/sdk/go="+modRoot)
		edit.Dir = filepath.Dir(testMain)
		if out, err := edit.CombinedOutput(); err != nil {
			// fallback to repo sdk if replace path odd on OS
			_ = repoSDK
			t.Fatalf("go mod edit: %v (%s)", err, out)
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go smoke: %v (%s)", err, out)
		}
	})

	t.Run("browser", func(t *testing.T) {
		if _, err := exec.LookPath("node"); err != nil {
			t.Skip("node not installed")
		}
		script := `
const fs = require('fs');
const path = require('path');
const AuthPro = require(process.argv[1]);
const cfg = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
if (cfg.appSecret) { console.error('browser config leaked secret'); process.exit(1); }
(async () => {
  await AuthPro.boot(cfg);
  const ads = await AuthPro.ads('home-banner');
  if (!ads) process.exit(1);
  const v = await AuthPro.verify();
  if (v.ok) { console.error('browser verify should not ok without proxy'); process.exit(1); }
  if (!v.data || v.data.reason !== 'browser_no_app_secret') { console.error(v); process.exit(1); }
  console.log(AuthPro.pluginSourceUrl());
})().catch((e) => { console.error(e); process.exit(1); });
`
		browserCfg := filepath.Join(root, "examples/browser/config.json")
		cmd := exec.Command("node", "-e", script, filepath.Join(root, "vendor/browser/src/auth-pro.js"), browserCfg)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("browser smoke: %v (%s)", err, out)
		}
		if !strings.Contains(string(out), "/software-source/app_demo_1/index.json") {
			t.Fatalf("browser plugin url=%s", out)
		}
	})
}
