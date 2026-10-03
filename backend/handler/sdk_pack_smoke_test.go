package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func extractSDKPack(t *testing.T, language, baseURL, publicKey string) string {
	t.Helper()
	input := testSDKPackInput(testSDKPackModulesAll(), language)
	input.BaseURL = baseURL
	input.PublicKey = publicKey
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
	root := filepath.Join(extractDir, "auth-pro-"+language+"-app_demo_1")
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("missing pack root %s: %v", root, err)
	}
	entries, err := os.ReadDir(extractDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("zip should contain a single language folder, got %v", names)
	}
	return root
}

// sdkProofStub 模拟授权站：核对 SDK 的 v3 请求签名，用本站授权响应私钥（和正式代码同一套 licenseVerifyProof）签响应。
// 授权码决定场景：forged-key 用别的私钥签（假服务器），down-key 一律 502，flaky-key 第一次通过、之后 502（授权站中途断开）。
// /proxy 模拟浏览器的同源代理：把浏览器给的 nonce 交给授权站后原样返回。
func sdkProofStub(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	pub := useClientResponseKey(t)
	_, fakePriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secret := testSDKPackInput(nil, "").AppSecret
	var mu sync.Mutex
	flaky := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/license/verify", "/proxy":
			var req licenseVerifyRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			req.Domain = normalizeLicenseDomain(req.Domain)
			req.ServerIP = normalizeLicenseServerIP(req.ServerIP)
			if r.URL.Path == "/api/license/verify" && (req.SignVersion != licenseSignVersionV3 || licenseVerifyV3Sign(req, secret) != req.Sign) {
				_ = json.NewEncoder(w).Encode(licenseVerifyUnsignedFailureBody())
				return
			}
			mu.Lock()
			if req.LicenseKey == "flaky-key" {
				flaky++
			}
			down := req.LicenseKey == "down-key" || (req.LicenseKey == "flaky-key" && flaky > 1)
			mu.Unlock()
			if down {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte("<html>502 Bad Gateway</html>"))
				return
			}
			now := time.Now().Unix()
			data := gin.H{"result": "pass", "appName": "演示应用", "expireAt": "永久", "expireTs": int64(0)}
			proof, err := licenseVerifyProof(req, data, now)
			if err != nil {
				t.Errorf("签名失败: %v", err)
				return
			}
			if req.LicenseKey == "forged-key" {
				fields, _ := licenseVerifyProofFields(req, data, now)
				proof["signature"] = signResponseProofWith(fakePriv, responseProofLicense, fields)
			}
			data["proof"] = proof
			_ = json.NewEncoder(w).Encode(gin.H{"code": 200, "msg": "授权有效", "data": data})
		case "/api/app/version/check":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"hasUpdate": false}})
		case "/api/v1/public/advertisements":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"records": []any{}, "placeholder": nil}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, base64Std(pub)
}

// TestSDKPackLanguageVerifyAgainstHTTPStub 每种语言解压一个接入包，对着会签名的授权站桩跑：
// 正常通过并命中缓存、假服务器签的「通过」被拒、授权站一直 502 时拒绝、通过一次后授权站断开时按离线宽限放行。
func TestSDKPackLanguageVerifyAgainstHTTPStub(t *testing.T) {
	server, pub := sdkProofStub(t)
	t.Setenv("TMPDIR", t.TempDir())

	t.Run("php", func(t *testing.T) {
		if _, err := exec.LookPath("php"); err != nil {
			t.Skip("php not installed")
		}
		root := extractSDKPack(t, "php", server.URL, pub)
		scriptPath := filepath.Join(t.TempDir(), "smoke.php")
		script := `<?php
require $argv[1];
AuthPro::boot($argv[2]);
$r = AuthPro::verify();
if (empty($r['ok'])) { fwrite(STDERR, json_encode($r)); exit(1); }
$r = AuthPro::verify();
if (empty($r['ok']) || empty($r['data']['cached'])) { fwrite(STDERR, 'cache ' . json_encode($r)); exit(1); }
$r = AuthPro::verify(array('licenseKey' => 'forged-key'));
if (!empty($r['ok']) || empty($r['data']['unverified'])) { fwrite(STDERR, 'forged ' . json_encode($r)); exit(1); }
$r = AuthPro::verify(array('licenseKey' => 'down-key'));
if (!empty($r['ok'])) { fwrite(STDERR, 'down ' . json_encode($r)); exit(1); }
AuthPro::loadConfig(array('cacheTtl' => 1));
$r = AuthPro::verify(array('licenseKey' => 'flaky-key'));
if (empty($r['ok'])) { fwrite(STDERR, 'flaky1 ' . json_encode($r)); exit(1); }
sleep(2);
$r = AuthPro::verify(array('licenseKey' => 'flaky-key'));
if (empty($r['ok']) || empty($r['data']['offline'])) { fwrite(STDERR, 'grace ' . json_encode($r)); exit(1); }
$r = AuthPro::verify(array('nonce' => 'relay_nonce_0123456789'));
if (empty($r['ok']) || $r['data']['proof']['nonce'] !== 'relay_nonce_0123456789') { fwrite(STDERR, 'relay ' . json_encode($r)); exit(1); }
$ads = AuthPro::ads('home-banner');
if (!is_array($ads)) { fwrite(STDERR, 'ads'); exit(1); }
$u = AuthPro::checkUpdate('1.0.0');
if (!isset($u['code'])) { fwrite(STDERR, json_encode($u)); exit(1); }
echo AuthPro::pluginSourceUrl();
`
		if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("php", scriptPath, filepath.Join(root, "AuthPro.php"), filepath.Join(root, "config.json"))
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
		root := extractSDKPack(t, "node", server.URL, pub)
		script := `
const AuthPro = require(process.argv[1]);
(async () => {
  await AuthPro.boot(process.argv[2]);
  let r = await AuthPro.verify();
  if (!r.ok) { console.error(r); process.exit(1); }
  r = await AuthPro.verify();
  if (!r.ok || !r.data.cached) { console.error('cache', r); process.exit(1); }
  r = await AuthPro.verify({ licenseKey: 'forged-key' });
  if (r.ok || !r.data.unverified) { console.error('forged', r); process.exit(1); }
  const down = await AuthPro.verify({ licenseKey: 'down-key' }).catch((e) => ({ ok: false, error: e }));
  if (down.ok) { console.error('down', down); process.exit(1); }
  AuthPro.loadConfig({ cacheTtl: 1 });
  r = await AuthPro.verify({ licenseKey: 'flaky-key' });
  if (!r.ok) { console.error('flaky1', r); process.exit(1); }
  await new Promise((done) => setTimeout(done, 2000));
  r = await AuthPro.verify({ licenseKey: 'flaky-key' });
  if (!r.ok || !r.data.offline) { console.error('grace', r); process.exit(1); }
  await AuthPro.ads('home-banner');
  await AuthPro.checkUpdate('1.0.0');
  console.log(AuthPro.pluginSourceUrl());
})().catch((e) => { console.error(e); process.exit(1); });
`
		cmd := exec.Command("node", "-e", script, filepath.Join(root, "index.js"), filepath.Join(root, "config.json"))
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
		root := extractSDKPack(t, "python", server.URL, pub)
		script := `
import sys
sys.path.insert(0, sys.argv[1])
import authpro
authpro.boot(sys.argv[2])
import time
r = authpro.verify()
assert r.get("ok"), r
r = authpro.verify()
assert r.get("ok") and r["data"].get("cached"), ("cache", r)
r = authpro.verify({"licenseKey": "forged-key"})
assert not r.get("ok") and r["data"].get("unverified"), ("forged", r)
r = authpro.verify({"licenseKey": "down-key"})
assert not r.get("ok"), ("down", r)
authpro.load_config({"cacheTtl": 1})
r = authpro.verify({"licenseKey": "flaky-key"})
assert r.get("ok"), ("flaky1", r)
time.sleep(2)
r = authpro.verify({"licenseKey": "flaky-key"})
assert r.get("ok") and r["data"].get("offline"), ("grace", r)
authpro.ads("home-banner")
authpro.check_update("1.0.0")
print(authpro.plugin_source_url())
`
		cmd := exec.Command("python3", "-c", script, root, filepath.Join(root, "config.json"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("python verify: %v (%s)", err, out)
		}
		if !strings.Contains(string(out), "/software-source/app_demo_1/index.json") {
			t.Fatalf("python plugin url=%s", out)
		}
	})

	t.Run("go", func(t *testing.T) {
		root := extractSDKPack(t, "go", server.URL, pub)
		if _, err := os.Stat(filepath.Join(root, "authpro/authpro.go")); err != nil {
			t.Fatal(err)
		}
		testMain := filepath.Join(t.TempDir(), "main_test.go")
		content := `package authpro_smoke_test
import (
  "testing"
  authpro "auth.maizll.com/sdk/go/authpro"
)
func TestSmoke(t *testing.T) {
  if err := authpro.Boot("` + filepath.ToSlash(filepath.Join(root, "config.json")) + `"); err != nil { t.Fatal(err) }
  r, err := authpro.Verify(); if err != nil || r == nil || !r.OK { t.Fatalf("%v %+v", err, r) }
  r, err = authpro.Verify(); if err != nil || !r.OK || r.Data["cached"] != true { t.Fatalf("cache %v %+v", err, r) }
  r, err = authpro.Verify(map[string]string{"licenseKey": "forged-key"}); if err != nil || r.OK || r.Data["unverified"] != true { t.Fatalf("forged %v %+v", err, r) }
  if r, err := authpro.Verify(map[string]string{"licenseKey": "down-key"}); err == nil && r.OK { t.Fatalf("down %+v", r) }
  if _, err := authpro.Ads("home-banner"); err != nil { t.Fatal(err) }
  if _, err := authpro.CheckUpdate("1.0.0"); err != nil { t.Fatal(err) }
  u, err := authpro.PluginSourceURL(); if err != nil || u == "" { t.Fatal(err, u) }
}
`
		if err := os.WriteFile(testMain, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		init := exec.Command("go", "mod", "init", "authpro.smoke")
		init.Dir = filepath.Dir(testMain)
		if out, err := init.CombinedOutput(); err != nil {
			t.Fatalf("go mod init: %v (%s)", err, out)
		}
		edit := exec.Command("go", "mod", "edit", "-require=auth.maizll.com/sdk/go@v0.0.0", "-replace=auth.maizll.com/sdk/go="+root)
		edit.Dir = filepath.Dir(testMain)
		if out, err := edit.CombinedOutput(); err != nil {
			t.Fatalf("go mod edit: %v (%s)", err, out)
		}
		cmd := exec.Command("go", "test", "-count=1", ".")
		cmd.Dir = filepath.Dir(testMain)
		cmd.Env = append(os.Environ(), "GO111MODULE=on")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go smoke: %v (%s)", err, out)
		}
	})

	t.Run("browser", func(t *testing.T) {
		if _, err := exec.LookPath("node"); err != nil {
			t.Skip("node not installed")
		}
		root := extractSDKPack(t, "browser", server.URL, pub)
		script := `
const fs = require('fs');
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
  if (!cfg.publicKey) { console.error('browser config missing publicKey'); process.exit(1); }
  AuthPro.loadConfig({ proxyVerifyUrl: cfg.baseUrl + '/proxy' });
  let p = await AuthPro.verify();
  if (!p.ok || !p.data.proof) { console.error('proxy', p); process.exit(1); }
  AuthPro.loadConfig({ licenseKey: 'forged-key' });
  p = await AuthPro.verify();
  if (p.ok) { console.error('proxy forged', p); process.exit(1); }
  console.log(AuthPro.pluginSourceUrl());
})().catch((e) => { console.error(e); process.exit(1); });
`
		cmd := exec.Command("node", "-e", script, filepath.Join(root, "auth-pro.js"), filepath.Join(root, "config.json"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("browser smoke: %v (%s)", err, out)
		}
		if !strings.Contains(string(out), "/software-source/app_demo_1/index.json") {
			t.Fatalf("browser plugin url=%s", out)
		}
	})
}

func base64Std(raw []byte) string {
	return base64.StdEncoding.EncodeToString(raw)
}
