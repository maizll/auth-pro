package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGetDSN(t *testing.T) {
	tests := []struct {
		name string
		cfg  DBConfig
		want string
	}{
		{
			name: "tcp",
			cfg:  DBConfig{Host: "127.0.0.1", Port: "3306", Database: "auto_pro", Username: "root", Password: "secret"},
			want: "root:secret@tcp(127.0.0.1:3306)/auto_pro?charset=utf8mb4&parseTime=True&loc=Local&multiStatements=true",
		},
		{
			name: "unix socket",
			cfg:  DBConfig{Host: "unix:/tmp/mysql.sock", Database: "auto_pro", Username: "root"},
			want: "root:@unix(/tmp/mysql.sock)/auto_pro?charset=utf8mb4&parseTime=True&loc=Local&multiStatements=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetDSN(&tt.cfg); got != tt.want {
				t.Fatalf("GetDSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSoftwareSourceConfig(t *testing.T) {
	t.Setenv("AUTO_PRO_SOFTWARE_SOURCE_URL", "http://127.0.0.1:19128/")
	t.Setenv("AUTO_PRO_SOFTWARE_SOURCE_API_KEY", "self-hosted-catalog-key")
	t.Setenv("AUTO_PRO_SOFTWARE_SOURCE_ADMIN_URL", "https://source.example.com/admin")
	t.Setenv("AUTO_PRO_SOFTWARE_SOURCE_TIMEOUT", "3s")
	t.Setenv("AUTO_PRO_SOFTWARE_SOURCE_STALE_TTL", "12h")
	if got := GetSoftwareSourceURL(); got != "http://127.0.0.1:19128" {
		t.Fatalf("software source URL = %q", got)
	}
	if got := GetSoftwareSourceAPIKey(); got != "self-hosted-catalog-key" {
		t.Fatalf("software source API key = %q", got)
	}
	if got := GetSoftwareSourceAdminURL(); got != "https://source.example.com/admin/" {
		t.Fatalf("software source admin URL = %q", got)
	}
	if GetSoftwareSourceTimeout() != 3*time.Second || GetSoftwareSourceStaleTTL() != 12*time.Hour {
		t.Fatal("software source durations were not parsed")
	}
}

func TestSoftwareSourceDefaultsAreSelfHosted(t *testing.T) {
	for _, value := range []string{"", "   "} {
		t.Run("empty-"+value, func(t *testing.T) {
			t.Setenv("AUTO_PRO_SOFTWARE_SOURCE_URL", value)
			t.Setenv("AUTO_PRO_SOFTWARE_SOURCE_API_KEY", value)
			t.Setenv("AUTO_PRO_SOFTWARE_SOURCE_ADMIN_URL", "")
			if got := GetSoftwareSourceURL(); got != "" {
				t.Fatalf("default software source URL must be empty, got %q", got)
			}
			if got := GetSoftwareSourceAPIKey(); got != "" {
				t.Fatalf("default software source key must be empty, got %q", got)
			}
			if got := GetSoftwareSourceAdminURL(); got != "" {
				t.Fatalf("default software source admin URL must be empty, got %q", got)
			}
			if got := GetSoftwareSourceAdminRedirect(); got != LocalSoftwareSourceAdminPath {
				t.Fatalf("unconfigured admin redirect = %q", got)
			}
			joined := GetSoftwareSourceURL() + GetSoftwareSourceAPIKey() + GetSoftwareSourceAdminURL()
			if strings.Contains(joined, "91ani") {
				t.Fatal("defaults must not mention the official host")
			}
		})
	}

	t.Setenv("AUTO_PRO_SOFTWARE_SOURCE_URL", "https://source.example.com")
	t.Setenv("AUTO_PRO_SOFTWARE_SOURCE_API_KEY", "deployment-override")
	t.Setenv("AUTO_PRO_SOFTWARE_SOURCE_ADMIN_URL", "")
	if got := GetSoftwareSourceURL(); got != "https://source.example.com" {
		t.Fatalf("software source URL override = %q", got)
	}
	if got := GetSoftwareSourceAPIKey(); got != "deployment-override" {
		t.Fatalf("software source key override = %q", got)
	}
	if got := GetSoftwareSourceAdminURL(); got != "https://source.example.com/admin/" {
		t.Fatalf("derived admin URL = %q", got)
	}
}

func TestAdvertisementConfig(t *testing.T) {
	t.Setenv("AUTO_PRO_ADVERTISEMENT_URL", "")
	if got := GetAdvertisementURL(); got != DefaultAdvertisementURL {
		t.Fatalf("默认广告接口地址 = %q", got)
	}
	if AdvertisementURLIsRemote() {
		t.Fatal("默认广告地址必须是本进程相对路径，不能走外网")
	}
	if strings.Contains(GetAdvertisementURL(), "91ani") {
		t.Fatal("默认广告地址不得指向官方域名")
	}
	// 投放方给出的地址常带多余的尾斜杠，拼 query 前必须归一化
	t.Setenv("AUTO_PRO_ADVERTISEMENT_URL", " https://ads.example.com/api/v1/public/advertisements// ")
	t.Setenv("AUTO_PRO_ADVERTISEMENT_TIMEOUT", "2s")
	t.Setenv("AUTO_PRO_ADVERTISEMENT_CACHE_TTL", "30s")
	t.Setenv("AUTO_PRO_ADVERTISEMENT_STALE_TTL", "6h")
	if got := GetAdvertisementURL(); got != "https://ads.example.com/api/v1/public/advertisements" {
		t.Fatalf("广告接口地址 = %q", got)
	}
	if !AdvertisementURLIsRemote() {
		t.Fatal("绝对 http(s) 广告地址应走远程代理")
	}
	if GetAdvertisementTimeout() != 2*time.Second || GetAdvertisementCacheTTL() != 30*time.Second ||
		GetAdvertisementStaleTTL() != 6*time.Hour {
		t.Fatal("广告相关时长未按环境变量解析")
	}
}

func TestLoadDBConfigFromEnv(t *testing.T) {
	t.Setenv("AUTO_PRO_DB_HOST", "unix:/tmp/auto-pro-test.sock")
	t.Setenv("AUTO_PRO_DB_PORT", "")
	t.Setenv("AUTO_PRO_DB_NAME", "auto_pro_test")
	t.Setenv("AUTO_PRO_DB_USER", "test_user")
	t.Setenv("AUTO_PRO_DB_PASSWORD", "test_password")

	cfg, err := LoadDBConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "unix:/tmp/auto-pro-test.sock" || cfg.Port != "" || cfg.Database != "auto_pro_test" || cfg.Username != "test_user" || cfg.Password != "test_password" {
		t.Fatalf("unexpected environment database config: %#v", cfg)
	}
}

func TestDefaultUpdateManifestURL(t *testing.T) {
	t.Setenv("AUTO_PRO_UPDATE_URL", "")
	if got := GetUpdateManifestURL(); got != DefaultUpdateManifestURL {
		t.Fatalf("GetUpdateManifestURL() = %q", got)
	}
	if !strings.Contains(DefaultUpdateManifestURL, "github.com") || !strings.Contains(DefaultUpdateManifestURL, "/maizll/auth-pro/") {
		t.Fatalf("default update URL must point at GitHub maizll/auth-pro, got %q", DefaultUpdateManifestURL)
	}
	for _, banned := range []string{"Zcy-sa", "cy70923167", "gitee.com"} {
		if strings.Contains(DefaultUpdateManifestURL, banned) {
			t.Fatalf("default update URL still mentions %q: %s", banned, DefaultUpdateManifestURL)
		}
	}
}

func TestResolveFrontendDirForWebsiteRoot(t *testing.T) {
	root := t.TempDir()
	backendDir := filepath.Join(root, "backend")
	if err := os.MkdirAll(backendDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}

	got := resolveFrontendDir(backendDir, filepath.Join(backendDir, "auth_pro"))
	if got != root {
		t.Fatalf("resolveFrontendDir() = %q, want %q", got, root)
	}
}

func TestGetUpdateManifestURL(t *testing.T) {
	t.Setenv("AUTO_PRO_UPDATE_URL", "")
	if got := GetUpdateManifestURL(); got != DefaultUpdateManifestURL {
		t.Fatalf("GetUpdateManifestURL() = %q", got)
	}

	t.Setenv("AUTO_PRO_UPDATE_URL", "https://mirror.example.com/latest.json")
	if got := GetUpdateManifestURL(); got != "https://mirror.example.com/latest.json" {
		t.Fatalf("GetUpdateManifestURL() override = %q", got)
	}
}

func TestAppVersionMatchesVERSIONFile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "VERSION"))
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	want := strings.TrimSpace(string(data))
	if want == "" || want == "1.0.0" {
		t.Fatalf("VERSION file must track the current product line, got %q", want)
	}
	if AppVersion != want {
		t.Fatalf("AppVersion=%q, VERSION file=%q", AppVersion, want)
	}
	if !strings.HasPrefix(AppVersion, "1.5.") {
		t.Fatalf("repo default AppVersion should be on the 1.5.x line, got %q", AppVersion)
	}
}

func TestGetFrontendDirOverride(t *testing.T) {
	frontendDir := t.TempDir()
	t.Setenv("AUTO_PRO_FRONTEND_DIR", frontendDir)
	if got := GetFrontendDir(); got != frontendDir {
		t.Fatalf("GetFrontendDir() = %q, want %q", got, frontendDir)
	}
}

func TestResolveFrontendRootRequiresConfiguredDisk(t *testing.T) {
	t.Setenv("AUTO_PRO_ALLOW_EMBEDDED_FRONTEND", "1")
	t.Setenv("AUTO_PRO_FRONTEND_DIR", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	if _, err := ResolveFrontendRoot(); err == nil {
		t.Fatal("configured disk frontend must fail even when embed is opted in")
	}
}

func TestResolveFrontendRootFailsLoudWithoutDiskOrEmbedOptIn(t *testing.T) {
	t.Setenv("AUTO_PRO_ALLOW_EMBEDDED_FRONTEND", "")
	t.Setenv("AUTO_PRO_FRONTEND_DIR", "")
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	if _, err := ResolveFrontendRoot(); err == nil {
		t.Fatal("production path must not silently fall back to embed")
	}
}

func TestResolveFrontendRootUsesDiskAndFingerprint(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>ok</html>"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "index-abc123.js"), []byte("js"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTO_PRO_FRONTEND_DIR", dir)
	t.Setenv("AUTO_PRO_ALLOW_EMBEDDED_FRONTEND", "")
	root, err := ResolveFrontendRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root.Mode != FrontendModeDisk || root.Dir != dir || root.ApplyDir != dir {
		t.Fatalf("root = %+v", root)
	}
	if !strings.Contains(root.Fingerprint, "index.html:") || !strings.Contains(root.Fingerprint, "index-abc123.js") {
		t.Fatalf("fingerprint = %q", root.Fingerprint)
	}
}

func TestResolveFrontendRootEmbedOnlyWhenOptedIn(t *testing.T) {
	t.Setenv("AUTO_PRO_FRONTEND_DIR", "")
	t.Setenv("AUTO_PRO_ALLOW_EMBEDDED_FRONTEND", "1")
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	root, err := ResolveFrontendRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root.Mode != FrontendModeEmbed {
		t.Fatalf("mode=%s dir=%s", root.Mode, root.Dir)
	}
	if root.ApplyDir == "" {
		t.Fatal("embed mode still needs a disk apply target for updates")
	}
}
