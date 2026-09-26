package handler

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

const githubAlipayReleaseURL = "https://github.com/maizll/authproPlus-source/releases/download/alipay-f2f-1.0.0/alipay-f2f-1.0.0.zip"

func TestSourcePackageRemoteURLFollowsPublicRedirect(t *testing.T) {
	payload := sourcePluginTestZIP(t)
	wantSHA := sha256Hex(payload)
	zipHits := 0
	zipServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zipHits++
		_, _ = w.Write(payload)
	}))
	t.Cleanup(zipServer.Close)
	_, zipPort, err := net.SplitHostPort(zipServer.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := "https://objects.githubusercontent.com:" + zipPort + "/pkg.zip"
		http.Redirect(w, r, target, http.StatusFound)
	}))
	t.Cleanup(redirector.Close)
	rawURL := pinTwoPublicHosts(t, "release.example.com", redirector, "objects.githubusercontent.com", zipServer)

	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	rec := sourceMultipart(t, router, "/api/v1/source/admin/packages/parse", admin, "", nil, map[string]string{
		"downloadUrl": rawURL,
		"category":    "other",
	})
	if sourceBodyCode(t, rec) != 200 || !strings.Contains(rec.Body.String(), wantSHA) {
		t.Fatalf("parse via redirect: %s", rec.Body.String())
	}
	if zipHits != 1 {
		t.Fatalf("zip server hits=%d", zipHits)
	}
	plugins, err := store.ListPlugins("")
	if err != nil || len(plugins) != 0 {
		t.Fatalf("parse persisted catalog: %v %#v", err, plugins)
	}
}

func TestSourcePackageRemoteURLRejectsPrivateRedirect(t *testing.T) {
	secretHits := 0
	secret := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secretHits++
		_, _ = w.Write(sourcePluginTestZIP(t))
	}))
	t.Cleanup(secret.Close)
	redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, secret.URL+"/pkg.zip", http.StatusFound)
	}))
	t.Cleanup(redirector.Close)
	rawURL := pinHostToServer(t, "release.example.com", redirector, secret)

	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	rec := sourceMultipart(t, router, "/api/v1/source/admin/packages/parse", admin, "", nil, map[string]string{
		"downloadUrl": rawURL,
	})
	if sourceBodyCode(t, rec) == 200 || !strings.Contains(rec.Body.String(), "拒绝访问非公网地址") {
		t.Fatalf("private redirect accepted: %s", rec.Body.String())
	}
	if secretHits != 0 {
		t.Fatalf("private hop was fetched %d times", secretHits)
	}
}

func TestSourcePackageRemoteURLRequiresFileOrHTTPS(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	missing := sourceMultipart(t, router, "/api/v1/source/admin/packages/parse", admin, "", nil, nil)
	if sourceBodyCode(t, missing) == 200 || !strings.Contains(missing.Body.String(), "二选一") {
		t.Fatalf("missing source: %s", missing.Body.String())
	}
	plain := sourceMultipart(t, router, "/api/v1/source/admin/packages/parse", admin, "", nil, map[string]string{
		"downloadUrl": "http://cdn.example.com/plugin.zip",
	})
	if sourceBodyCode(t, plain) == 200 || !strings.Contains(plain.Body.String(), "https://") {
		t.Fatalf("http url: %s", plain.Body.String())
	}
}

func TestSourcePackagePublishRemoteURLFreeAndPaid(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	payload := sourcePluginTestZIP(t)
	wantSHA := sha256Hex(payload)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	rawURL := pinHostToServer(t, "cdn.example.com", server, server)

	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	free := sourceMultipart(t, router, "/api/v1/source/admin/packages/publish", admin, "", nil, map[string]string{
		"downloadUrl": rawURL,
		"category":    "other",
		"appId":       "1",
		"push":        "0",
		"shelf":       "1",
	})
	if sourceBodyCode(t, free) != 200 || !strings.Contains(free.Body.String(), wantSHA) {
		t.Fatalf("free publish: %s", free.Body.String())
	}
	plugin, err := store.GetPlugin("demo-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if plugin.DownloadURL != rawURL || plugin.SHA256 != wantSHA || plugin.OriginURL != "" || plugin.PriceCents != 0 {
		t.Fatalf("free item=%#v", plugin)
	}
	index := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if !strings.Contains(index.Body.String(), wantSHA) || !strings.Contains(index.Body.String(), rawURL) {
		t.Fatalf("free index: %s", index.Body.String())
	}

	paidZIP := makeTestZIP(t, testZIPEntry{name: "paid-plugin/plugin.json", data: `{
		"id":"paid-plugin","name":"付费插件","version":"1.0.0","description":"外链托管",
		"author":{"name":"源站"},"category":"other"
	}`})
	paidSHA := sha256Hex(paidZIP)
	paidServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(paidZIP)
	}))
	t.Cleanup(paidServer.Close)
	paidURL := pinHostToServer(t, "paid.example.com", paidServer, paidServer)
	paid := sourceMultipart(t, router, "/api/v1/source/admin/packages/publish", admin, "", nil, map[string]string{
		"downloadUrl": paidURL,
		"category":    "other",
		"appId":       "1",
		"priceCents":  "2500",
		"push":        "0",
		"shelf":       "1",
	})
	body := paid.Body.String()
	if sourceBodyCode(t, paid) != 200 || !strings.Contains(body, paidSHA) || !strings.Contains(body, `"storedPackage":true`) {
		t.Fatalf("paid publish: %s", body)
	}
	if strings.Contains(body, paidURL) && !strings.Contains(body, `"originUrl"`) {
		t.Fatalf("paid response leaked origin without labeling it: %s", body)
	}
	item, err := store.GetPlugin("paid-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if item.SHA256 != paidSHA || item.OriginURL != paidURL || !isPrivatePackageRef(item.DownloadURL) || item.PriceCents != 2500 {
		t.Fatalf("paid item=%#v", item)
	}
	if strings.Contains(item.DownloadURL, "paid.example.com") {
		t.Fatalf("buyer url is still the origin: %s", item.DownloadURL)
	}
	buyer := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	buyerBody := buyer.Body.String()
	if strings.Contains(buyerBody, "paid.example.com") || strings.Contains(buyerBody, "originUrl") || strings.Contains(buyerBody, paidSHA) || !strings.Contains(buyerBody, "付费插件") {
		t.Fatalf("buyer index: %s", buyerBody)
	}
}

func TestSourcePackageGitHubReleaseExternalURL(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	parsed := sourceMultipart(t, router, "/api/v1/source/admin/packages/parse", admin, "", nil, map[string]string{
		"downloadUrl": githubAlipayReleaseURL,
		"category":    "payment",
	})
	if sourceBodyCode(t, parsed) != 200 {
		t.Fatalf("github parse: %s", parsed.Body.String())
	}
	var parsedBody struct {
		Data struct {
			ID     string `json:"id"`
			SHA256 string `json:"sha256"`
		} `json:"data"`
	}
	if err := json.Unmarshal(parsed.Body.Bytes(), &parsedBody); err != nil {
		t.Fatal(err)
	}
	if parsedBody.Data.ID != "alipay-f2f" || len(parsedBody.Data.SHA256) != 64 {
		t.Fatalf("parsed manifest: %s", parsed.Body.String())
	}
	freeSHA := parsedBody.Data.SHA256

	free := sourceMultipart(t, router, "/api/v1/source/admin/packages/publish", admin, "", nil, map[string]string{
		"downloadUrl": githubAlipayReleaseURL,
		"category":    "payment",
		"appId":       "1",
		"push":        "0",
		"shelf":       "1",
	})
	if sourceBodyCode(t, free) != 200 || !strings.Contains(free.Body.String(), freeSHA) {
		t.Fatalf("github free publish: %s", free.Body.String())
	}
	plugin, err := store.GetPlugin("alipay-f2f")
	if err != nil {
		t.Fatal(err)
	}
	if plugin.SHA256 != freeSHA || plugin.DownloadURL != githubAlipayReleaseURL || plugin.OriginURL != "" || plugin.PriceCents != 0 {
		t.Fatalf("free github item=%#v", plugin)
	}
	if strings.Contains(plugin.DownloadURL, "release-assets.githubusercontent.com") || strings.Contains(plugin.DownloadURL, "objects.githubusercontent.com") {
		t.Fatalf("stored redirect target: %s", plugin.DownloadURL)
	}
	index := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if !strings.Contains(index.Body.String(), freeSHA) || !strings.Contains(index.Body.String(), githubAlipayReleaseURL) {
		t.Fatalf("free github index: %s", index.Body.String())
	}

	paidRouter, paidStore := sourceStationRouter(t)
	paidParsed := sourceMultipart(t, paidRouter, "/api/v1/source/admin/packages/parse", admin, "", nil, map[string]string{
		"downloadUrl": githubAlipayReleaseURL,
		"category":    "payment",
	})
	if sourceBodyCode(t, paidParsed) != 200 || !strings.Contains(paidParsed.Body.String(), freeSHA) {
		t.Fatalf("github paid parse: %s", paidParsed.Body.String())
	}
	paid := sourceMultipart(t, paidRouter, "/api/v1/source/admin/packages/publish", admin, "", nil, map[string]string{
		"downloadUrl":   githubAlipayReleaseURL,
		"category":      "payment",
		"appId":         "1",
		"priceCents":    "1990",
		"packageSource": "github",
		"push":          "0",
	})
	if sourceBodyCode(t, paid) == 200 || !strings.Contains(paid.Body.String(), "GitHub 只读令牌") {
		t.Fatalf("paid github without token: %s", paid.Body.String())
	}
	if _, err := paidStore.GetPlugin("alipay-f2f"); !errors.Is(err, errSourceNotFound) {
		t.Fatalf("paid github was stored without a token: %v", err)
	}
	matches, globErr := filepath.Glob(filepath.Join(stationPaidPackageDirPath(), "*.zip"))
	if globErr != nil || len(matches) != 0 {
		t.Fatalf("paid github left files: %v %v", matches, globErr)
	}
}

func pinTwoPublicHosts(t *testing.T, fromHost string, fromServer *httptest.Server, toHost string, toServer *httptest.Server) string {
	t.Helper()
	_, fromPort, err := net.SplitHostPort(fromServer.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	fromIP := net.ParseIP("8.8.8.8")
	toIP := net.ParseIP("1.1.1.1")
	cfg := testServerTLSConfig(toServer)
	if cfg == nil {
		cfg = &tls.Config{}
	}
	cfg.InsecureSkipVerify = true
	previous := currentSafeFetchHooks()
	setSafeFetchHooks(safeFetchHooks{
		resolve: func(_ context.Context, name string) ([]net.IP, error) {
			switch name {
			case fromHost:
				return []net.IP{fromIP}, nil
			case toHost:
				return []net.IP{toIP}, nil
			default:
				if ip := net.ParseIP(name); ip != nil {
					return []net.IP{ip}, nil
				}
				return nil, errSafeBadURL
			}
		},
		dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialHost, _, splitErr := net.SplitHostPort(address)
			if splitErr != nil {
				return nil, splitErr
			}
			switch dialHost {
			case fromIP.String():
				return (&net.Dialer{}).DialContext(ctx, network, fromServer.Listener.Addr().String())
			case toIP.String():
				return (&net.Dialer{}).DialContext(ctx, network, toServer.Listener.Addr().String())
			default:
				return nil, errSafeBadURL
			}
		},
		tls: cfg,
	})
	t.Cleanup(func() { setSafeFetchHooks(previous) })
	return "https://" + fromHost + ":" + fromPort + "/pkg.zip"
}
