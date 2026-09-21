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

func testSDKPackInput(modules []string, language string) sdkPackInput {
	if language == "" {
		language = "php"
	}
	return sdkPackInput{
		AppID:     12,
		AppName:   "演示应用",
		AppKey:    "app_demo_1",
		AppSecret: "sk_live_demo_secret_aaa",
		BaseURL:   "https://auth.example.com",
		Modules:   modules,
		Language:  language,
	}
}

func testSDKPackModulesAll() []string {
	return []string{
		sdkPackModuleLicense, sdkPackModulePiracy, sdkPackModuleUpdate, sdkPackModuleAds, sdkPackModulePluginSource,
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

func TestNormalizeSDKPackLanguage(t *testing.T) {
	if _, err := normalizeSDKPackLanguage(""); err == nil || !strings.Contains(err.Error(), "请选择接入语言") {
		t.Fatalf("empty language want 请选择接入语言, got %v", err)
	}
	if _, err := normalizeSDKPackLanguage("   "); err == nil || !strings.Contains(err.Error(), "请选择接入语言") {
		t.Fatalf("blank language want 请选择接入语言, got %v", err)
	}
	if _, err := normalizeSDKPackLanguage("java"); err == nil || !strings.Contains(err.Error(), "不支持的接入语言") {
		t.Fatalf("invalid language want 不支持的接入语言, got %v", err)
	}
	got, err := normalizeSDKPackLanguage(" PHP ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "php" {
		t.Fatalf("language=%q", got)
	}
}

func TestBuildSDKPackRequiresLanguage(t *testing.T) {
	input := testSDKPackInput([]string{sdkPackModuleLicense}, "php")
	input.Language = ""
	if _, _, err := buildSDKPack(input); err == nil || !strings.Contains(err.Error(), "请选择接入语言") {
		t.Fatalf("missing language want 请选择接入语言, got %v", err)
	}
	input.Language = "ruby"
	if _, _, err := buildSDKPack(input); err == nil || !strings.Contains(err.Error(), "不支持的接入语言") {
		t.Fatalf("invalid language want 不支持的接入语言, got %v", err)
	}
}

func TestBuildSDKPackSingleLanguageLayout(t *testing.T) {
	otherSecret := "sk_live_other_app_secret_bbb"
	type langSpec struct {
		lang     string
		filename string
		root     string
		must     []string
		mustNot  []string
	}
	specs := []langSpec{
		{
			lang: "php", filename: "auth-pro-php-app_demo_1.zip", root: "auth-pro-php-app_demo_1/",
			must:    []string{"README.md", "config.json", "AuthPro.php", "example.php"},
			mustNot: []string{"index.js", "auth-pro.js", "authpro.go", "__init__.py", "vendor/", "examples/node", "examples/python", "examples/go", "examples/browser"},
		},
		{
			lang: "node", filename: "auth-pro-node-app_demo_1.zip", root: "auth-pro-node-app_demo_1/",
			must:    []string{"README.md", "config.json", "index.js", "example.js"},
			mustNot: []string{"AuthPro.php", "auth-pro.js", "authpro.go", "__init__.py", "vendor/", "examples/php"},
		},
		{
			lang: "python", filename: "auth-pro-python-app_demo_1.zip", root: "auth-pro-python-app_demo_1/",
			must:    []string{"README.md", "config.json", "authpro/__init__.py", "example.py"},
			mustNot: []string{"AuthPro.php", "index.js", "auth-pro.js", "authpro.go", "vendor/", "examples/php"},
		},
		{
			lang: "go", filename: "auth-pro-go-app_demo_1.zip", root: "auth-pro-go-app_demo_1/",
			must:    []string{"README.md", "config.json", "authpro/authpro.go", "go.mod", "example.go"},
			mustNot: []string{"AuthPro.php", "index.js", "auth-pro.js", "__init__.py", "vendor/", "examples/php"},
		},
		{
			lang: "browser", filename: "auth-pro-browser-app_demo_1.zip", root: "auth-pro-browser-app_demo_1/",
			must:    []string{"README.md", "config.json", "auth-pro.js", "example.html"},
			mustNot: []string{"AuthPro.php", "index.js", "authpro.go", "__init__.py", "vendor/", "examples/php", "examples/node"},
		},
	}

	for _, spec := range specs {
		spec := spec
		t.Run(spec.lang, func(t *testing.T) {
			payload, filename, err := buildSDKPack(testSDKPackInput(testSDKPackModulesAll(), spec.lang))
			if err != nil {
				t.Fatal(err)
			}
			if filename != spec.filename {
				t.Fatalf("filename=%q want %q", filename, spec.filename)
			}
			files := zipFiles(t, payload)
			for _, rel := range spec.must {
				if files[spec.root+rel] == "" {
					t.Fatalf("missing %s; have %v", spec.root+rel, keysOf(files))
				}
			}
			for name := range files {
				if !strings.HasPrefix(name, spec.root) {
					t.Fatalf("entry outside single root %s: %s", spec.root, name)
				}
				for _, other := range sdkPackLanguages {
					if other == spec.lang {
						continue
					}
					if strings.Contains(name, "/vendor/"+other+"/") || strings.Contains(name, "/examples/"+other+"/") {
						t.Fatalf("%s pack contains other language path %s", spec.lang, name)
					}
				}
				for _, banned := range spec.mustNot {
					if strings.Contains(name, banned) {
						t.Fatalf("%s pack should not contain %q (saw %s)", spec.lang, banned, name)
					}
				}
			}
			readme := files[spec.root+"README.md"]
			if !strings.Contains(readme, "把") && !strings.Contains(readme, "复制") {
				t.Fatal("README should tell user where to put the folder")
			}
			for _, other := range sdkPackLanguages {
				if other == spec.lang {
					continue
				}
				if strings.Contains(readme, "vendor/"+other) {
					t.Fatalf("README still documents other language vendor/%s", other)
				}
			}
			if strings.Contains(readme, "五语言") {
				t.Fatal("README must not say 五语言")
			}
			cfg := files[spec.root+"config.json"]
			if strings.Contains(cfg, otherSecret) {
				t.Fatal("pack leaked another app secret")
			}
			if spec.lang == "browser" {
				if strings.Contains(cfg, "appSecret") || strings.Contains(cfg, "sk_live_demo_secret_aaa") {
					t.Fatal("browser config must omit appSecret")
				}
				core := files[spec.root+"auth-pro.js"]
				if strings.Contains(core, "sk_live_demo_secret_aaa") {
					t.Fatal("browser entry must not bake app secret")
				}
			} else if !strings.Contains(cfg, "sk_live_demo_secret_aaa") {
				t.Fatal("server language config should include appSecret when signing modules enabled")
			}
		})
	}
}

func TestBuildSDKPackOmitsSecretWhenNoSigningModule(t *testing.T) {
	payload, _, err := buildSDKPack(testSDKPackInput([]string{sdkPackModuleAds, sdkPackModulePluginSource}, "php"))
	if err != nil {
		t.Fatal(err)
	}
	files := zipFiles(t, payload)
	cfg := files["auth-pro-php-app_demo_1/config.json"]
	if strings.Contains(cfg, "appSecret") || strings.Contains(cfg, "sk_live_demo_secret_aaa") {
		t.Fatal("unsigned modules should not bake appSecret into config.json")
	}
	if !strings.Contains(files["auth-pro-php-app_demo_1/README.md"], "https://auth.example.com/software-source/app_demo_1/index.json") {
		t.Fatal("plugin source URL missing from ads/plugin pack")
	}
}

func TestBuildSDKPackSwitchingAppOnlyChangesConfig(t *testing.T) {
	a := testSDKPackInput([]string{sdkPackModuleLicense, sdkPackModuleAds}, "php")
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
	coreA := fa["auth-pro-php-app_demo_1/AuthPro.php"]
	coreB := fb["auth-pro-php-app_other/AuthPro.php"]
	if coreA == "" || coreA != coreB {
		t.Fatal("language entry must be identical across apps")
	}
	if fa["auth-pro-php-app_demo_1/config.json"] == fb["auth-pro-php-app_other/config.json"] {
		t.Fatal("config.json must differ between apps")
	}
}

func TestBuildSDKPackLicenseV2FieldOrderMatchesBackend(t *testing.T) {
	payload, _, err := buildSDKPack(testSDKPackInput([]string{sdkPackModuleLicense, sdkPackModuleUpdate}, "php"))
	if err != nil {
		t.Fatal(err)
	}
	php := zipFiles(t, payload)["auth-pro-php-app_demo_1/AuthPro.php"]
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
	input := testSDKPackInput([]string{sdkPackModuleLicense}, "php")
	input.AppName = "evil */ echo secret"
	payload, _, err := buildSDKPack(input)
	if err != nil {
		t.Fatal(err)
	}
	example := zipFiles(t, payload)["auth-pro-php-app_demo_1/example.php"]
	if strings.Contains(example, "evil */") {
		t.Fatal("app name must not terminate the PHP file comment")
	}
}

func TestBuildSDKPackPiracyImpliesLicenseVerifyOnBoot(t *testing.T) {
	payload, _, err := buildSDKPack(testSDKPackInput([]string{sdkPackModulePiracy}, "php"))
	if err != nil {
		t.Fatal(err)
	}
	php := zipFiles(t, payload)["auth-pro-php-app_demo_1/AuthPro.php"]
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
	body := `{"appId":3,"modules":["ads"],"language":"php"}`
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
	if req.Language != "php" {
		t.Fatalf("language=%q", req.Language)
	}
}

func TestParseSDKPackHTTPRequestRejectsMissingLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/sdk/pack", strings.NewReader(`{"appId":3,"modules":["ads"]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Host = "license.local:19127"

	_, err := parseSDKPackHTTPRequest(c)
	if err == nil || !strings.Contains(err.Error(), "请选择接入语言") {
		t.Fatalf("want 请选择接入语言, got %v", err)
	}
}

func TestAdminSDKPackDownloadRejectsMissingAppID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/sdk/pack", strings.NewReader(`{"modules":["license"],"language":"php"}`))
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

func TestAdminSDKPackDownloadRejectsMissingLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/sdk/pack", strings.NewReader(`{"appId":3,"modules":["license"]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Host = "license.local"
	AdminSDKPackDownload(c)
	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 400 || !strings.Contains(body.Msg, "请选择接入语言") {
		t.Fatalf("want 400 请选择接入语言, got %+v body=%s", body, recorder.Body.String())
	}
}

func keysOf(files map[string]string) []string {
	out := make([]string, 0, len(files))
	for name := range files {
		out = append(out, name)
	}
	return out
}
