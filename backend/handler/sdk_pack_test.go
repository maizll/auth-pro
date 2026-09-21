package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNormalizeSDKPackModulesRejectsUnknownAndEmpty(t *testing.T) {
	if _, err := normalizeSDKPackModules(nil); err == nil {
		t.Fatal("empty modules should be rejected")
	}
	if _, err := normalizeSDKPackModules([]string{"license", "malware"}); err == nil {
		t.Fatal("unknown module should be rejected")
	}
}

func TestNormalizeSDKPackModulesDedupesAndKeepsOrder(t *testing.T) {
	got, err := normalizeSDKPackModules([]string{" ads ", "LICENSE", "ads", "plugin_source"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{sdkPackModuleLicense, sdkPackModuleAds, sdkPackModulePluginSource}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("modules=%v want %v", got, want)
	}
}

func TestNormalizeSDKPackBaseURL(t *testing.T) {
	got, err := normalizeSDKPackBaseURL(" https://Auth.Example.com/admin/ ", "http://fallback.local")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://auth.example.com" {
		t.Fatalf("baseURL=%q", got)
	}
	if _, err := normalizeSDKPackBaseURL("ftp://x", ""); err == nil {
		t.Fatal("ftp should be rejected")
	}
	if _, err := normalizeSDKPackBaseURL("https://user:pass@evil.example/path", ""); err == nil {
		t.Fatal("userinfo should be rejected")
	}
	fallback, err := normalizeSDKPackBaseURL("", "https://this-site.example:19127/")
	if err != nil {
		t.Fatal(err)
	}
	if fallback != "https://this-site.example:19127" {
		t.Fatalf("fallback=%q", fallback)
	}
}

func testSDKPackInput(modules []string, includeJS bool) sdkPackInput {
	return sdkPackInput{
		AppID:     12,
		AppName:   "演示应用",
		AppKey:    "app_demo_1",
		AppSecret: "sk_live_demo_secret_aaa",
		BaseURL:   "https://auth.example.com",
		Modules:   modules,
		IncludeJS: includeJS,
	}
}

func zipFiles(t *testing.T, payload []byte) map[string]string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, entry := range reader.File {
		if strings.HasSuffix(entry.Name, "/") {
			continue
		}
		src, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name] = string(body)
	}
	return files
}

func TestSDKPackPluginSourceURLMatchesIsolationContract(t *testing.T) {
	got := sdkPackPluginSourceURL("https://auth.example.com/", "app_key-1")
	want := "https://auth.example.com/software-source/" + url.PathEscape("app_key-1") + "/index.json"
	if got != want {
		t.Fatalf("plugin source URL=%q want %q", got, want)
	}
	if got == "https://auth.example.com/software-source/index.json" {
		t.Fatal("must not use unscoped catalog URL")
	}
}

func TestBuildSDKPackHybridLayoutAndAPIs(t *testing.T) {
	otherSecret := "sk_live_other_app_secret_bbb"
	payload, filename, err := buildSDKPack(testSDKPackInput([]string{
		sdkPackModuleLicense, sdkPackModulePiracy, sdkPackModuleUpdate, sdkPackModuleAds, sdkPackModulePluginSource,
	}, true))
	if err != nil {
		t.Fatal(err)
	}
	if filename != "auth-pro-client-app_demo_1.zip" {
		t.Fatalf("filename=%q", filename)
	}
	files := zipFiles(t, payload)
	root := "auth-pro-client-app_demo_1/"
	readme := files[root+"README.md"]
	configRaw := files[root+"config.json"]
	if readme == "" || configRaw == "" {
		t.Fatalf("missing README/config: %v", keysOf(files))
	}
	for _, lang := range sdkPackLanguages {
		if !strings.Contains(readme, lang) {
			t.Fatalf("README missing language %s", lang)
		}
		foundVendor := false
		foundExample := false
		for name := range files {
			if strings.HasPrefix(name, root+"vendor/"+lang+"/") {
				foundVendor = true
			}
			if strings.HasPrefix(name, root+"examples/"+lang+"/") {
				foundExample = true
			}
		}
		if !foundVendor || !foundExample {
			t.Fatalf("lang %s vendor=%t example=%t", lang, foundVendor, foundExample)
		}
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(configRaw), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["appKey"] != "app_demo_1" || cfg["baseUrl"] != "https://auth.example.com" {
		t.Fatalf("config=%v", cfg)
	}
	if cfg["appSecret"] != "sk_live_demo_secret_aaa" {
		t.Fatal("server config should include appSecret when signing modules enabled")
	}
	if strings.Contains(configRaw, otherSecret) {
		t.Fatal("pack leaked another app secret")
	}

	browserCfg := files[root+"examples/browser/config.json"]
	if browserCfg == "" {
		t.Fatal("browser example config missing")
	}
	if strings.Contains(browserCfg, "appSecret") || strings.Contains(browserCfg, "sk_live_demo_secret_aaa") {
		t.Fatal("browser config must not embed appSecret")
	}

	phpCore := files[root+"vendor/php/src/AuthPro.php"]
	nodeCore := files[root+"vendor/node/src/index.js"]
	pyCore := files[root+"vendor/python/authpro/__init__.py"]
	goCore := files[root+"vendor/go/authpro/authpro.go"]
	browserCore := files[root+"vendor/browser/src/auth-pro.js"]
	for _, needle := range []string{"/api/license/verify", "/api/app/version/check", "/api/v1/public/advertisements", "pluginSource"} {
		for lang, body := range map[string]string{"php": phpCore, "node": nodeCore, "python": pyCore, "go": goCore} {
			if !strings.Contains(body, needle) && !(needle == "pluginSource" && (strings.Contains(body, "plugin_source_url") || strings.Contains(body, "PluginSourceURL") || strings.Contains(body, "pluginSourceUrl"))) {
				if needle == "pluginSource" {
					continue
				}
				t.Fatalf("%s missing %s", lang, needle)
			}
		}
	}
	if !strings.Contains(phpCore, "function verify") && !strings.Contains(phpCore, "public static function verify") {
		t.Fatal("php verify missing")
	}
	if !strings.Contains(browserCore, "function ads") || !strings.Contains(browserCore, "pluginSourceUrl") {
		t.Fatal("browser must implement ads and pluginSourceUrl")
	}
	if strings.Contains(browserCore, "sk_live_demo_secret_aaa") {
		t.Fatal("browser vendor must not bake app secret")
	}
	if strings.Contains(readme, "auth_pro_sdk.php") {
		t.Fatal("old monolithic pack should be superseded in README")
	}
	pluginURL := "https://auth.example.com/software-source/app_demo_1/index.json"
	if !strings.Contains(readme, pluginURL) {
		t.Fatalf("readme should document plugin URL %s", pluginURL)
	}
}

func TestBuildSDKPackOmitsSecretWhenNoSigningModule(t *testing.T) {
	payload, _, err := buildSDKPack(testSDKPackInput([]string{sdkPackModuleAds, sdkPackModulePluginSource}, true))
	if err != nil {
		t.Fatal(err)
	}
	files := zipFiles(t, payload)
	cfg := files["auth-pro-client-app_demo_1/config.json"]
	if strings.Contains(cfg, "appSecret") || strings.Contains(cfg, "sk_live_demo_secret_aaa") {
		t.Fatal("unsigned modules should not bake appSecret into config.json")
	}
	if !strings.Contains(files["auth-pro-client-app_demo_1/README.md"], "https://auth.example.com/software-source/app_demo_1/index.json") {
		t.Fatal("plugin source URL missing from ads/plugin pack")
	}
}

func TestBuildSDKPackSwitchingAppOnlyChangesConfig(t *testing.T) {
	a := testSDKPackInput([]string{sdkPackModuleLicense, sdkPackModuleAds}, false)
	b := a
	b.AppID = 99
	b.AppName = "另一应用"
	b.AppKey = "app_other"
	b.AppSecret = "sk_live_other_secret_zzz"
	pa, _, err := buildSDKPack(a)
	if err != nil {
		t.Fatal(err)
	}
	pb, _, err := buildSDKPack(b)
	if err != nil {
		t.Fatal(err)
	}
	fa := zipFiles(t, pa)
	fb := zipFiles(t, pb)
	vendorA := fa["auth-pro-client-app_demo_1/vendor/php/src/AuthPro.php"]
	vendorB := fb["auth-pro-client-app_other/vendor/php/src/AuthPro.php"]
	if vendorA == "" || vendorA != vendorB {
		t.Fatal("vendor core must be identical across apps")
	}
	if fa["auth-pro-client-app_demo_1/config.json"] == fb["auth-pro-client-app_other/config.json"] {
		t.Fatal("config.json must differ between apps")
	}
}

func TestBuildSDKPackLicenseV2FieldOrderMatchesBackend(t *testing.T) {
	payload, _, err := buildSDKPack(testSDKPackInput([]string{sdkPackModuleLicense, sdkPackModuleUpdate}, false))
	if err != nil {
		t.Fatal(err)
	}
	php := zipFiles(t, payload)["auth-pro-client-app_demo_1/vendor/php/src/AuthPro.php"]
	licenseParts := []string{"'v2'", "self::cfg('appKey', '')", "$ctx['licenseKey']", "$ctx['domain']", "$ctx['serverIp']", "(string)$timestamp"}
	assertAppearsInOrder(t, php, licenseParts)
	updateParts := []string{"'v2'", "self::cfg('appKey', '')", "(string)$currentVersion", "$ctx['licenseKey']", "$ctx['domain']", "$ctx['serverIp']", "(string)$timestamp"}
	assertAppearsInOrder(t, php, updateParts)
	if !strings.Contains(php, "rtrim($value, '.')") {
		t.Fatal("php domain normalize must strip trailing dots to match v2 canonical")
	}

	req := licenseVerifyRequest{
		AppKey: "app_demo_1", LicenseKey: "LIC", Domain: "Example.COM.", ServerIP: "127.0.0.1", Timestamp: 1700000000,
	}
	req.Domain = normalizeLicenseDomain(req.Domain)
	req.ServerIP = normalizeLicenseServerIP(req.ServerIP)
	if req.Domain != "example.com" {
		t.Fatalf("backend canonical domain=%q", req.Domain)
	}
	want := licenseVerifyV2Sign(req, "sk_live_demo_secret_aaa")
	got := licenseVerifyV2Sign(licenseVerifyRequest{
		AppKey: "app_demo_1", LicenseKey: "LIC", Domain: "example.com", ServerIP: "127.0.0.1", Timestamp: 1700000000,
	}, "sk_live_demo_secret_aaa")
	if want != got {
		t.Fatalf("php-normalized domain must produce the same HMAC as backend")
	}
}

func TestPHPCommentDoesNotBreakOut(t *testing.T) {
	input := testSDKPackInput([]string{sdkPackModuleLicense}, false)
	input.AppName = "evil */ echo secret"
	payload, _, err := buildSDKPack(input)
	if err != nil {
		t.Fatal(err)
	}
	example := zipFiles(t, payload)["auth-pro-client-app_demo_1/examples/php/boot.php"]
	if strings.Contains(example, "evil */") {
		t.Fatal("app name must not terminate the PHP file comment")
	}
}

func TestBuildSDKPackPiracyImpliesLicenseVerifyOnBoot(t *testing.T) {
	payload, _, err := buildSDKPack(testSDKPackInput([]string{sdkPackModulePiracy}, true))
	if err != nil {
		t.Fatal(err)
	}
	php := zipFiles(t, payload)["auth-pro-client-app_demo_1/vendor/php/src/AuthPro.php"]
	if !strings.Contains(php, "/api/license/verify") {
		t.Fatal("piracy pack must still call license verify so the server can record hits")
	}
	if !strings.Contains(php, "未授权") && !strings.Contains(php, "授权无效") {
		t.Fatal("piracy pack must include hard-fail UX copy")
	}
}

func TestClientSDKAssetsEmbedComplete(t *testing.T) {
	for _, lang := range sdkPackLanguages {
		found := false
		_ = fs.WalkDir(clientSDKAssets, path.Join("sdk_assets", lang), func(walkPath string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				found = true
			}
			return nil
		})
		if !found {
			t.Fatalf("embedded sdk_assets/%s is empty", lang)
		}
	}
}

func assertAppearsInOrder(t *testing.T, haystack string, parts []string) {
	t.Helper()
	cursor := 0
	for _, part := range parts {
		idx := strings.Index(haystack[cursor:], part)
		if idx < 0 {
			t.Fatalf("missing %q after offset %d", part, cursor)
		}
		cursor += idx + len(part)
	}
}

func TestParseSDKPackHTTPRequestUsesOriginFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := `{"appId":3,"modules":["ads"]}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/sdk/pack", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Host = "license.local:19127"
	c.Request.Header.Set("X-Forwarded-Proto", "https")

	req, err := parseSDKPackHTTPRequest(c)
	if err != nil {
		t.Fatal(err)
	}
	if req.AppID != 3 || req.BaseURL != "https://license.local:19127" {
		t.Fatalf("parsed=%+v", req)
	}
	if !req.IncludeJS {
		t.Fatal("includeJs should default true")
	}
}

func TestAdminSDKPackDownloadRejectsMissingAppID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/sdk/pack", strings.NewReader(`{"modules":["license"]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	AdminSDKPackDownload(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d", recorder.Code)
	}
	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 400 || body.Msg == "" {
		t.Fatalf("want 400 json, got %+v body=%s", body, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "sk_live_") {
		t.Fatal("error response must not include secrets")
	}
}

func keysOf(files map[string]string) []string {
	out := make([]string, 0, len(files))
	for name := range files {
		out = append(out, name)
	}
	return out
}
