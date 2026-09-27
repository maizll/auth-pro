package handler

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
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
	view := resolveBuyerConnection(c, "", "", false)
	if !view.TrustProxy || view.SiteURL != "https://shop.example.com" || view.SourceBase != buyerSourceDefault || len(view.Issues) != 0 {
		t.Fatalf("%+v", view)
	}
	if domain := buyerRequestDomain(withEmptyBuyerConfig(t, c)); domain != "shop.example.com" {
		t.Fatalf("domain=%s", domain)
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
		{name: "loopback http", remote: "127.0.0.1:1", host: "shop.example.com", fwdHost: "shop.example.com", fwdProto: "http", want: buyerSiteHTTPMessage, trust: true},
		{name: "forwarded private", remote: "[::1]:1", host: "127.0.0.1", fwdHost: "192.168.0.8", fwdProto: "https", want: buyerSitePrivateMessage, trust: true},
		{name: "direct loopback", remote: "127.0.0.1:80", host: "127.0.0.1:19127", want: buyerSiteUnusableMessage},
		{name: "direct private", remote: "203.0.113.10:443", host: "10.1.2.3", tlsOn: true, want: buyerSitePrivateMessage},
		{name: "direct http", remote: "203.0.113.10:80", host: "shop.example.com", want: buyerSiteHTTPMessage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newBuyerVisit(tc.remote, tc.host, tc.tlsOn, tc.fwdHost, tc.fwdProto)
			view := resolveBuyerConnection(c, buyerSourceDefault, "", false)
			if view.TrustProxy != tc.trust || view.SiteURL != "" || len(view.Issues) != 1 || view.Issues[0].Field != "site" || view.Issues[0].Message != tc.want {
				t.Fatalf("%+v", view)
			}
		})
	}
}

func TestBuyerConnectionIgnoresSpoofedForwarding(t *testing.T) {
	c := newBuyerVisit("203.0.113.10:443", "shop.example.com", true, "evil.example.com", "http")
	view := resolveBuyerConnection(c, "", "", false)
	if view.TrustProxy || view.SiteURL != "https://shop.example.com" || len(view.Issues) != 0 {
		t.Fatalf("%+v", view)
	}
}

func TestBuyerConnectionKeepsSavedSite(t *testing.T) {
	c := newBuyerVisit("127.0.0.1:1", "127.0.0.1", false, "other.example.com", "https")
	view := resolveBuyerConnection(c, "https://buy.example.com/", "https://shop.example.com", false)
	if view.SourceBase != "https://buy.example.com" || view.SiteURL != "https://shop.example.com" || view.TrustProxy || len(view.Issues) != 0 {
		t.Fatalf("%+v", view)
	}
	bad := resolveBuyerConnection(c, "http://buy.example.com", "http://127.0.0.1", true)
	if len(bad.Issues) != 2 || bad.Issues[0].Field != "source" || bad.Issues[1].Field != "site" {
		t.Fatalf("%+v", bad.Issues)
	}
	trusted := resolveBuyerConnection(c, "", "", true)
	if !trusted.TrustProxy || trusted.SiteURL != "https://other.example.com" {
		t.Fatalf("%+v", trusted)
	}
}

func withEmptyBuyerConfig(t *testing.T, c *gin.Context) *gin.Context {
	t.Helper()
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	config.SetDBOverrideForTest(nil)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })
	return c
}
