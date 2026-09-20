package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestBuildSDKPackRendersOnlyThisAppAndSelectedModules(t *testing.T) {
	otherSecret := "sk_live_other_app_secret_bbb"
	payload, filename, err := buildSDKPack(testSDKPackInput([]string{
		sdkPackModuleLicense, sdkPackModulePiracy, sdkPackModuleUpdate, sdkPackModuleAds, sdkPackModulePluginSource,
	}, true))
	if err != nil {
		t.Fatal(err)
	}
	if filename != "auth-pro-sdk-app_demo_1.zip" {
		t.Fatalf("filename=%q", filename)
	}
	files := zipFiles(t, payload)
	root := "auth-pro-sdk-app_demo_1/"
	php := files[root+"auth_pro_sdk.php"]
	example := files[root+"config.example.php"]
	readme := files[root+"README.md"]
	js := files[root+"auth-pro-sdk.js"]
	if php == "" || example == "" || readme == "" || js == "" {
		t.Fatalf("missing core files: %v", keysOf(files))
	}
	for _, needle := range []string{"app_demo_1", "sk_live_demo_secret_aaa", "https://auth.example.com", "12"} {
		if !strings.Contains(php, needle) {
			t.Fatalf("php missing %q", needle)
		}
	}
	if strings.Contains(php, otherSecret) || strings.Contains(js, otherSecret) {
		t.Fatal("pack leaked another app secret")
	}
	pluginURL := "https://auth.example.com/software-source/app_demo_1/index.json"
	if !strings.Contains(php, "const PLUGIN_SOURCE_URL = '"+pluginURL+"'") {
		t.Fatalf("php PLUGIN_SOURCE_URL not baked, want %s", pluginURL)
	}
	if !strings.Contains(php, pluginURL) || !strings.Contains(js, pluginURL) {
		t.Fatalf("plugin source must bake app-scoped index: php=%t js=%t", strings.Contains(php, pluginURL), strings.Contains(js, pluginURL))
	}
	unscoped := "https://auth.example.com/software-source/index.json"
	if strings.Contains(php, unscoped) || strings.Contains(js, unscoped) {
		t.Fatal("must not bake unscoped software-source/index.json")
	}
	for _, needle := range []string{"/api/license/verify", "/api/app/version/check", "/api/v1/public/advertisements"} {
		if !strings.Contains(php, needle) {
			t.Fatalf("php missing endpoint %s", needle)
		}
	}
	if !strings.Contains(php, "v2") || !strings.Contains(php, "hash_hmac") {
		t.Fatal("php must implement HMAC v2")
	}
	if !strings.Contains(readme, "require") || !strings.Contains(readme, "AuthPro::boot()") {
		t.Fatal("readme missing 3-step PHP usage")
	}
	if !strings.Contains(example, "licenseKey") {
		t.Fatal("config.example.php should document licenseKey")
	}
}

func TestBuildSDKPackOmitsDisabledModulesAndOptionalJS(t *testing.T) {
	payload, _, err := buildSDKPack(testSDKPackInput([]string{sdkPackModuleLicense}, false))
	if err != nil {
		t.Fatal(err)
	}
	files := zipFiles(t, payload)
	root := "auth-pro-sdk-app_demo_1/"
	if _, ok := files[root+"auth-pro-sdk.js"]; ok {
		t.Fatal("JS should be omitted when includeJs=false")
	}
	php := files[root+"auth_pro_sdk.php"]
	if !strings.Contains(php, "/api/license/verify") {
		t.Fatal("license module missing verify endpoint")
	}
	if strings.Contains(php, "/api/v1/public/advertisements") {
		t.Fatal("ads endpoint should be omitted")
	}
	if strings.Contains(php, "/api/app/version/check") {
		t.Fatal("update endpoint should be omitted")
	}
	if strings.Contains(php, "/software-source/") {
		t.Fatal("plugin source url should be omitted")
	}
	if strings.Contains(php, "function ads") || strings.Contains(php, "function checkUpdate") {
		t.Fatal("disabled module helpers should be omitted")
	}
}

func TestBuildSDKPackOmitsSecretWhenNoSigningModule(t *testing.T) {
	payload, _, err := buildSDKPack(testSDKPackInput([]string{sdkPackModuleAds, sdkPackModulePluginSource}, true))
	if err != nil {
		t.Fatal(err)
	}
	files := zipFiles(t, payload)
	php := files["auth-pro-sdk-app_demo_1/auth_pro_sdk.php"]
	js := files["auth-pro-sdk-app_demo_1/auth-pro-sdk.js"]
	if strings.Contains(php, "sk_live_demo_secret_aaa") || strings.Contains(js, "sk_live_demo_secret_aaa") {
		t.Fatal("unsigned modules should not bake appSecret")
	}
	if !strings.Contains(php, "https://auth.example.com/software-source/app_demo_1/index.json") {
		t.Fatal("plugin source URL missing from ads/plugin pack")
	}
}

func TestBuildSDKPackLicenseV2FieldOrderMatchesBackend(t *testing.T) {
	payload, _, err := buildSDKPack(testSDKPackInput([]string{sdkPackModuleLicense, sdkPackModuleUpdate}, false))
	if err != nil {
		t.Fatal(err)
	}
	php := zipFiles(t, payload)["auth-pro-sdk-app_demo_1/auth_pro_sdk.php"]
	licenseParts := []string{"'v2'", "self::APP_KEY", "$ctx['licenseKey']", "$ctx['domain']", "$ctx['serverIp']", "(string)$timestamp"}
	assertAppearsInOrder(t, php, licenseParts)
	updateParts := []string{"'v2'", "self::APP_KEY", "(string)$version", "$ctx['licenseKey']", "$ctx['domain']", "$ctx['serverIp']", "(string)$timestamp"}
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
	php := zipFiles(t, payload)["auth-pro-sdk-app_demo_1/auth_pro_sdk.php"]
	if strings.Contains(php, "evil */") {
		t.Fatal("app name must not terminate the PHP file comment")
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

func TestBuildSDKPackPiracyImpliesLicenseVerify(t *testing.T) {
	payload, _, err := buildSDKPack(testSDKPackInput([]string{sdkPackModulePiracy}, true))
	if err != nil {
		t.Fatal(err)
	}
	php := zipFiles(t, payload)["auth-pro-sdk-app_demo_1/auth_pro_sdk.php"]
	if !strings.Contains(php, "/api/license/verify") {
		t.Fatal("piracy pack must still call license verify so the server can record hits")
	}
	if !strings.Contains(php, "未授权") && !strings.Contains(php, "授权无效") {
		t.Fatal("piracy pack must include hard-fail UX copy")
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
