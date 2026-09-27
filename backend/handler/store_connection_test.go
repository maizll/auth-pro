package handler

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

func newBuyerVisit(remote, host string, tlsOn bool, forwardedHost, forwardedProto string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/store/account", nil)
	c.Request.RemoteAddr = remote
	c.Request.Host = host
	if tlsOn {
		c.Request.TLS = &tls.ConnectionState{}
	}
	if forwardedHost != "" {
		c.Request.Header.Set("X-Forwarded-Host", forwardedHost)
	}
	if forwardedProto != "" {
		c.Request.Header.Set("X-Forwarded-Proto", forwardedProto)
	}
	return c
}

func TestBuyerConnectionAutoDetectsLocalProxy(t *testing.T) {
	c := newBuyerVisit("127.0.0.1:41000", "127.0.0.1:19127", false, "shop.example.com", "https")
	view := resolveBuyerConnection(c)
	if !view.TrustProxy || view.SiteURL != "https://shop.example.com" || len(view.Issues) != 0 {
		t.Fatalf("%+v", view)
	}
	if domain := buyerRequestDomain(withEmptyBuyerConfig(t, c)); domain != "shop.example.com" {
		t.Fatalf("domain=%s", domain)
	}
}

func TestBuyerConnectionDetectsBaotaHostAndProto(t *testing.T) {
	c := newBuyerVisit("127.0.0.1:18082", "buyer.auth-pro.test", false, "", "https")
	view := resolveBuyerConnection(c)
	if !view.TrustProxy || view.SiteURL != "https://buyer.auth-pro.test" || len(view.Issues) != 0 {
		t.Fatalf("%+v", view)
	}
}

func TestBuyerConnectionRejectsPrivateAndHTTP(t *testing.T) {
	cases := []struct {
		name     string
		remote   string
		host     string
		tlsOn    bool
		fwdHost  string
		fwdProto string
		want     string
		trust    bool
	}{
		{name: "loopback http", remote: "127.0.0.1:1", host: "shop.example.com", fwdHost: "shop.example.com", fwdProto: "http", want: buyerSiteRetryMessage, trust: true},
		{name: "forwarded private", remote: "[::1]:1", host: "127.0.0.1", fwdHost: "192.168.0.8", fwdProto: "https", want: buyerSiteRetryMessage, trust: true},
		{name: "baota http", remote: "127.0.0.1:80", host: "shop.example.com", fwdProto: "http", want: buyerSiteRetryMessage, trust: true},
		{name: "direct loopback", remote: "127.0.0.1:80", host: "127.0.0.1:19127", want: buyerSiteRetryMessage},
		{name: "direct private", remote: "203.0.113.10:443", host: "10.1.2.3", tlsOn: true, want: buyerSiteRetryMessage},
		{name: "direct http", remote: "203.0.113.10:80", host: "shop.example.com", want: buyerSiteRetryMessage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newBuyerVisit(tc.remote, tc.host, tc.tlsOn, tc.fwdHost, tc.fwdProto)
			view := resolveBuyerConnection(c)
			if view.TrustProxy != tc.trust || view.SiteURL != "" || len(view.Issues) != 1 || view.Issues[0].Field != "site" || view.Issues[0].Message != tc.want {
				t.Fatalf("%+v", view)
			}
		})
	}
}

func TestBuyerConnectionIgnoresSpoofedForwarding(t *testing.T) {
	c := newBuyerVisit("203.0.113.10:443", "shop.example.com", true, "evil.example.com", "http")
	view := resolveBuyerConnection(c)
	if view.TrustProxy || view.SiteURL != "https://shop.example.com" || len(view.Issues) != 0 {
		t.Fatalf("%+v", view)
	}
}

func TestBuyerSourceBaseUsesPackageVariableOnly(t *testing.T) {
	prev := buyerSourceBaseForTest
	buyerSourceBaseForTest = ""
	t.Cleanup(func() { buyerSourceBaseForTest = prev })
	t.Setenv("AUTH_PRO_STORE_SOURCE_BASE", "https://env.example.test")
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	if err := os.WriteFile(filepath.Join(config.GetDataDir(), "source-base"), []byte("https://file.example.test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	base, err := buyerSourceBase()
	if err != nil || base != buyerSourceDefault {
		t.Fatalf("环境变量和文件不应生效 base=%s err=%v", base, err)
	}
	buyerSourceBaseForTest = "https://hook.example.test"
	base, err = buyerSourceBase()
	if err != nil || base != "https://hook.example.test" {
		t.Fatalf("hook base=%s err=%v", base, err)
	}
	buyerSourceBaseForTest = "http://hook.example.test"
	if _, err := buyerSourceBase(); err == nil {
		t.Fatal("非 https 测试地址应失败")
	}
}

func TestBuyerAccountPayloadOmitsSourceBase(t *testing.T) {
	payload := buyerAccountPayload(buyerAccessView{Edition: storeEditionFree, Features: []string{}}, buyerConnectionView{
		SourceBase: "https://secret.example",
		SiteURL:    "https://shop.example.com",
		Issues:     []buyerConnectionIssue{{Field: "site", Message: buyerSiteRetryMessage}},
	}, "install")
	if _, ok := payload["sourceBase"]; ok {
		t.Fatal("账号接口回显了源站地址")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "secret.example") || strings.Contains(text, "auth.maizll.com") {
		t.Fatalf("payload leaked source: %s", text)
	}
	if !strings.Contains(text, buyerSiteRetryMessage) {
		t.Fatalf("缺少识别失败提示: %s", text)
	}
}

func TestBuyerStoreSettingsIgnoresSourceBase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := buyerSourceBaseForTest
	buyerSourceBaseForTest = ""
	t.Cleanup(func() { buyerSourceBaseForTest = prev })
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	t.Setenv("AUTH_PRO_STORE_SOURCE_BASE", "https://evil.example")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/store/settings", bytes.NewBufferString(`{"sourceBase":"https://evil.example","siteUrl":"https://shop.example.com","trustProxy":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	BuyerStoreSettingsSave(c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	base, err := buyerSourceBase()
	if err != nil || base != buyerSourceDefault {
		t.Fatalf("保存接口改了源站 base=%s err=%v", base, err)
	}
	if _, err := os.Stat(filepath.Join(config.GetDataDir(), "store", "source-base")); !os.IsNotExist(err) {
		t.Fatalf("保存接口写出了源站文件: %v", err)
	}
}

func withEmptyBuyerConfig(t *testing.T, c *gin.Context) *gin.Context {
	t.Helper()
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	config.SetDBOverrideForTest(nil)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })
	return c
}
