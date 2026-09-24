package handler

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"auto_pro/config"
)

func installSafeFetchHooks(t *testing.T, resolve func(context.Context, string) ([]net.IP, error), dial func(context.Context, string, string) (net.Conn, error)) {
	t.Helper()
	previous := currentSafeFetchHooks()
	next := previous
	if resolve != nil {
		next.resolve = resolve
	}
	if dial != nil {
		next.dial = dial
	}
	setSafeFetchHooks(next)
	t.Cleanup(func() { setSafeFetchHooks(previous) })
}

func TestFetchIPAllowed(t *testing.T) {
	cases := []struct {
		ip    string
		allow bool
		want  error
	}{
		{"8.8.8.8", false, nil},
		{"127.0.0.1", false, errSafePrivateAddress},
		{"127.0.0.1", true, nil},
		{"10.1.2.3", false, errSafePrivateAddress},
		{"10.1.2.3", true, nil},
		{"192.168.0.8", true, nil},
		{"172.16.5.4", false, errSafePrivateAddress},
		{"::ffff:127.0.0.1", false, errSafePrivateAddress},
		{"::ffff:10.2.3.4", true, nil},
		{"169.254.169.254", true, errSafeMetadataAddress},
		{"169.254.1.1", false, errSafeMetadataAddress},
		{"169.254.170.2", true, errSafeMetadataAddress},
		{"100.100.100.200", true, errSafeMetadataAddress},
		{"100.64.0.1", false, errSafeMetadataAddress},
		{"fd00:ec2::254", true, errSafeMetadataAddress},
		{"fd00::1", false, errSafePrivateAddress},
		{"fd00::1", true, nil},
		{"::1", true, nil},
		{"::1", false, errSafePrivateAddress},
		{"fe80::1", true, errSafeMetadataAddress},
		{"0.0.0.0", true, errSafePrivateAddress},
		{"255.255.255.255", false, errSafePrivateAddress},
	}
	for _, tc := range cases {
		err := fetchIPAllowed(net.ParseIP(tc.ip), tc.allow)
		if !errors.Is(err, tc.want) {
			t.Errorf("ip=%s allow=%v err=%v want=%v", tc.ip, tc.allow, err, tc.want)
		}
	}
}

func TestPluginSourceAllowsPrivateOnlyForLiteralLAN(t *testing.T) {
	if !pluginSourceAllowsPrivate("http://192.168.1.10/index.json") || !pluginSourceAllowsPrivate("http://127.0.0.1:19127/software-source/demo/index.json") {
		t.Fatal("literal intranet source must be allowed")
	}
	if pluginSourceAllowsPrivate("https://example.com/index.json") || pluginSourceAllowsPrivate("http://169.254.169.254/index.json") || pluginSourceAllowsPrivate("http://metadata.google.internal/") {
		t.Fatal("hostname and metadata source must stay on the public policy")
	}
}

func TestVerifyExternalPackageRejectsPrivateTargetsWithoutReadingBody(t *testing.T) {
	sha := strings.Repeat("ab", 32)
	t.Run("literal loopback", func(t *testing.T) {
		hits := 0
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			_, _ = io.Copy(io.Discard, r.Body)
			_, _ = w.Write([]byte("PK\x03\x04"))
		}))
		t.Cleanup(server.Close)
		err := verifyExternalPackageHTTP(context.Background(), server.URL+"/pkg.zip", sha)
		if err == nil || !errors.Is(err, errSafePrivateAddress) || hits != 0 {
			t.Fatalf("err=%v hits=%d", err, hits)
		}
	})

	t.Run("literal private and metadata do not dial", func(t *testing.T) {
		dialed := 0
		installSafeFetchHooks(t, nil, func(context.Context, string, string) (net.Conn, error) {
			dialed++
			return nil, errors.New("should not dial")
		})
		for _, raw := range []string{
			"https://10.0.0.5/pkg.zip",
			"https://169.254.169.254/latest/meta-data",
			"https://metadata.google.internal/computeMetadata/v1/",
		} {
			err := verifyExternalPackageHTTP(context.Background(), raw, sha)
			if err == nil || !isSafeFetchPolicyError(err) {
				t.Fatalf("url=%s err=%v", raw, err)
			}
		}
		if dialed != 0 {
			t.Fatalf("dialed %d times", dialed)
		}
	})

	t.Run("public url redirecting to loopback", func(t *testing.T) {
		secretHits := 0
		secret := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			secretHits++
			_, _ = io.Copy(io.Discard, r.Body)
			_, _ = w.Write([]byte("PK\x03\x04secret"))
		}))
		t.Cleanup(secret.Close)
		redirectHits := 0
		redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			redirectHits++
			http.Redirect(w, r, secret.URL+"/pkg.zip", http.StatusFound)
		}))
		t.Cleanup(redirector.Close)
		useExternalPackageClientForTest(t, redirector.Client())
		rawURL := pinHostToServer(t, "pkg.example.com", redirector, secret)
		err := verifyExternalPackageHTTP(context.Background(), rawURL, sha)
		if err == nil || !errors.Is(err, errSafePrivateAddress) {
			t.Fatalf("err=%v", err)
		}
		if secretHits != 0 {
			t.Fatalf("loopback body was requested %d times", secretHits)
		}
		if redirectHits == 0 {
			t.Fatal("redirect hop was not contacted")
		}
	})
}

func TestDownloadPluginPackageSHA256AndRedirect(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	archive := makeTestZIP(t, testZIPEntry{name: "dist/plugin.json", data: `{"id":"demo-plugin"}`}, testZIPEntry{name: "dist/keep.txt", data: "installed"})
	sum := sha256Hex(archive)
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write(archive)
	}))
	t.Cleanup(server.Close)
	rawURL := server.URL + "/plugin.zip"

	if _, err := downloadPluginPackage(context.Background(), rawURL, "", true); err == nil || !errors.Is(err, errPluginSHAMissing) || hits != 0 {
		t.Fatalf("missing sha err=%v hits=%d", err, hits)
	}
	if _, err := downloadPluginPackage(context.Background(), rawURL, strings.Repeat("cd", 32), true); err == nil || !errors.Is(err, errPluginSHAMismatch) {
		t.Fatalf("mismatch err=%v", err)
	}
	payload, err := downloadPluginPackage(context.Background(), rawURL, sum, true)
	if err != nil || sha256Hex(payload) != sum {
		t.Fatalf("match err=%v", err)
	}

	plugin := pluginInfo{ID: "demo-plugin", Name: "Demo", Category: "other", Version: "1.0.0"}
	root := filepath.Join(config.GetPluginDir(), plugin.ID)
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := downloadAndInstallPluginPackage(context.Background(), rawURL, strings.Repeat("ab", 32), true, plugin); err == nil {
		t.Fatal("wrong sha installed")
	}
	if data, err := os.ReadFile(filepath.Join(root, "keep.txt")); err != nil || string(data) != "old" {
		t.Fatalf("plugin dir changed: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".installed.json")); !os.IsNotExist(err) {
		t.Fatalf("install marker appeared: %v", err)
	}
	if err := downloadAndInstallPluginPackage(context.Background(), rawURL, sum, true, plugin); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "keep.txt")); err != nil || string(data) != "installed" {
		t.Fatalf("installed content=%q err=%v", data, err)
	}

	loopHits := 0
	loop := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loopHits++
		_, _ = w.Write(archive)
	}))
	t.Cleanup(loop.Close)
	if _, err := downloadPluginPackage(context.Background(), loop.URL+"/plugin.zip", sum, false); err == nil || !errors.Is(err, errSafePrivateAddress) || loopHits != 0 {
		t.Fatalf("public policy loopback err=%v hits=%d", err, loopHits)
	}

	secretHits := 0
	secret := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secretHits++
		_, _ = w.Write(archive)
	}))
	t.Cleanup(secret.Close)
	redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, secret.URL+"/plugin.zip", http.StatusFound)
	}))
	t.Cleanup(redirector.Close)
	redirectURL := pinHostToServer(t, "cdn.example.com", redirector, secret)
	before := hits
	if err := downloadAndInstallPluginPackage(context.Background(), redirectURL, sum, false, pluginInfo{ID: "other-plugin", Name: "Other", Category: "other"}); err == nil || !errors.Is(err, errSafePrivateAddress) {
		t.Fatalf("redirect err=%v", err)
	}
	if secretHits != 0 {
		t.Fatalf("redirect target read %d times", secretHits)
	}
	if _, err := os.Stat(filepath.Join(config.GetPluginDir(), "other-plugin")); !os.IsNotExist(err) {
		t.Fatalf("redirect install created plugin dir: %v", err)
	}
	if hits != before {
		t.Fatal("intranet server was used for the public redirect case")
	}
}

func pinHostToServer(t *testing.T, host string, publicServer, loopServer *httptest.Server) string {
	t.Helper()
	_, port, err := net.SplitHostPort(publicServer.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	publicIP := net.ParseIP("8.8.8.8")
	previous := currentSafeFetchHooks()
	setSafeFetchHooks(safeFetchHooks{
		resolve: func(_ context.Context, name string) ([]net.IP, error) {
			if ip := net.ParseIP(name); ip != nil {
				return []net.IP{ip}, nil
			}
			if name == host {
				return []net.IP{publicIP}, nil
			}
			return nil, errSafeBadURL
		},
		dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialHost, _, splitErr := net.SplitHostPort(address)
			if splitErr != nil {
				return nil, splitErr
			}
			if dialHost == publicIP.String() {
				return (&net.Dialer{}).DialContext(ctx, network, publicServer.Listener.Addr().String())
			}
			if ip := net.ParseIP(dialHost); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
				return (&net.Dialer{}).DialContext(ctx, network, loopServer.Listener.Addr().String())
			}
			return nil, errors.New("unexpected dial " + address)
		},
		tls: testServerTLSConfig(publicServer),
	})
	t.Cleanup(func() { setSafeFetchHooks(previous) })
	return "https://" + host + ":" + port + "/pkg.zip"
}

func testServerTLSConfig(server *httptest.Server) *tls.Config {
	transport, ok := server.Client().Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig == nil {
		return nil
	}
	return transport.TLSClientConfig.Clone()
}
