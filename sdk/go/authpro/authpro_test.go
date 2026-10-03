package authpro_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	authpro "auth.maizll.com/sdk/go/authpro"
)

// stubServer 模拟授权站：按 docs/api-sdk.md 的规则给 v3 响应签名。result 决定返回通过还是拒绝。
type stubServer struct {
	priv   ed25519.PrivateKey
	result atomic.Value
	hits   atomic.Int32
}

func (s *stubServer) handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/api/license/verify":
		s.hits.Add(1)
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		result, _ := s.result.Load().(string)
		reason := ""
		code := 200
		if result != "pass" {
			reason, code = "license_revoked", 403
		}
		lk, _ := req["licenseKey"].(string)
		hash := ""
		if lk != "" {
			sum := sha256.Sum256([]byte(lk))
			hash = hex.EncodeToString(sum[:])
		}
		now := time.Now().Unix()
		text := "auth-pro-license-v3\n" + strings.Join([]string{
			"appKey=" + req["appKey"].(string), "domain=" + req["domain"].(string), "serverIp=" + req["serverIp"].(string),
			"licenseKeyHash=" + hash, "nonce=" + req["nonce"].(string), "serverTime=" + strconv.FormatInt(now, 10),
			"result=" + result, "reason=" + reason, "expireTs=",
		}, "\n") + "\n"
		data := map[string]any{"result": result, "proof": map[string]any{
			"appKey": req["appKey"], "domain": req["domain"], "serverIp": req["serverIp"], "licenseKeyHash": hash,
			"nonce": req["nonce"], "serverTime": now, "signature": "ed25519:" + base64.StdEncoding.EncodeToString(ed25519.Sign(s.priv, []byte(text))),
		}}
		if reason != "" {
			data["reason"] = reason
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "msg": "", "data": data})
	case "/api/v1/public/advertisements":
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"records": []any{}, "placeholder": nil}})
	default:
		http.NotFound(w, r)
	}
}

func newStub(t *testing.T) (*stubServer, *httptest.Server, string) {
	t.Helper()
	t.Setenv("TMPDIR", t.TempDir())
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	stub := &stubServer{priv: priv}
	stub.result.Store("pass")
	server := httptest.NewServer(http.HandlerFunc(stub.handler))
	t.Cleanup(server.Close)
	return stub, server, base64.StdEncoding.EncodeToString(pub)
}

func writeConfig(t *testing.T, cfg map[string]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	raw, _ := json.Marshal(cfg)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBootVerifyAgainstStub(t *testing.T) {
	stub, server, pub := newStub(t)
	cfgPath := writeConfig(t, map[string]any{
		"baseUrl": server.URL, "appId": 1, "appKey": "demo", "appSecret": "secret", "publicKey": pub,
		"domain": "shop.example.com", "modules": map[string]bool{"license": true, "ads": true, "plugin_source": true},
	})
	client := &authpro.Client{}
	if err := client.Boot(cfgPath); err != nil {
		t.Fatal(err)
	}
	result, err := client.Verify()
	if err != nil || result == nil || !result.OK || result.Data["cached"] != true {
		t.Fatalf("第二次校验应命中缓存 result=%+v err=%v", result, err)
	}
	if stub.hits.Load() != 1 {
		t.Fatalf("缓存期内不应重复请求，hits=%d", stub.hits.Load())
	}
	ads, err := client.Ads("home-banner")
	if err != nil || ads == nil {
		t.Fatalf("ads err=%v", err)
	}
	url, err := client.PluginSourceURL()
	if err != nil || url != server.URL+"/software-source/demo/index.json" {
		t.Fatalf("plugin url=%q err=%v", url, err)
	}
}

func TestVerifyRejectsForgedServerAndUsesOfflineGrace(t *testing.T) {
	_, server, pub := newStub(t)
	cfg := map[string]any{"baseUrl": server.URL, "appKey": "demo", "appSecret": "secret", "publicKey": pub, "domain": "shop.example.com", "cacheTtl": 1, "offlineGrace": 2}
	client := &authpro.Client{}
	if err := client.LoadConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if r, err := client.Verify(); err != nil || !r.OK {
		t.Fatalf("首次校验应通过 r=%+v err=%v", r, err)
	}

	// 假服务器：用别的私钥签「通过」。没有缓存时必须拒绝。
	_, fakePriv, _ := ed25519.GenerateKey(rand.Reader)
	fake := &stubServer{priv: fakePriv}
	fake.result.Store("pass")
	fakeServer := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer fakeServer.Close()
	forged := &authpro.Client{}
	_ = forged.LoadConfig(map[string]any{"baseUrl": fakeServer.URL, "appKey": "demo", "appSecret": "secret", "publicKey": pub, "domain": "other.example.com"})
	r, err := forged.Verify()
	if err != nil || r.OK || r.Data["unverified"] != true {
		t.Fatalf("伪造的通过应被拒 r=%+v err=%v", r, err)
	}

	// 授权站连不上：宽限期内沿用上次通过的结果。
	time.Sleep(1100 * time.Millisecond)
	server.Close()
	r, err = client.Verify()
	if err != nil || !r.OK || r.Data["offline"] != true {
		t.Fatalf("宽限期内应沿用缓存 r=%+v err=%v", r, err)
	}
	// 宽限期过了：不再放行。
	time.Sleep(2 * time.Second)
	if r, err := client.Verify(); err == nil && r.OK {
		t.Fatalf("宽限期过后应拒绝 r=%+v", r)
	}
}

func TestVerifySignedDenialClearsCache(t *testing.T) {
	stub, server, pub := newStub(t)
	client := &authpro.Client{}
	_ = client.LoadConfig(map[string]any{"baseUrl": server.URL, "appKey": "demo", "appSecret": "secret", "publicKey": pub, "domain": "shop.example.com", "cacheTtl": 1})
	if r, err := client.Verify(); err != nil || !r.OK {
		t.Fatalf("首次校验应通过 r=%+v err=%v", r, err)
	}
	time.Sleep(1100 * time.Millisecond)
	stub.result.Store("fail")
	if r, err := client.Verify(); err != nil || r.OK {
		t.Fatalf("授权站签名的拒绝应立刻生效 r=%+v err=%v", r, err)
	}
	server.Close()
	if r, err := client.Verify(); err == nil && r.OK {
		t.Fatalf("拒绝后缓存应已清掉，离线时不能再放行 r=%+v", r)
	}
}

func TestVerifyRequiresPublicKey(t *testing.T) {
	_, server, _ := newStub(t)
	client := &authpro.Client{}
	_ = client.LoadConfig(map[string]any{"baseUrl": server.URL, "appKey": "demo", "appSecret": "secret"})
	if _, err := client.Verify(); err == nil || !strings.Contains(err.Error(), "publicKey") {
		t.Fatalf("缺少 publicKey 应明确报错 err=%v", err)
	}
}
