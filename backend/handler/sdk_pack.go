package handler

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

const (
	sdkPackModuleLicense      = "license"
	sdkPackModulePiracy       = "piracy"
	sdkPackModuleUpdate       = "update"
	sdkPackModuleAds          = "ads"
	sdkPackModulePluginSource = "plugin_source"
)

var sdkPackModuleOrder = []string{
	sdkPackModuleLicense,
	sdkPackModulePiracy,
	sdkPackModuleUpdate,
	sdkPackModuleAds,
	sdkPackModulePluginSource,
}

var sdkPackModuleSet = map[string]struct{}{
	sdkPackModuleLicense:      {},
	sdkPackModulePiracy:       {},
	sdkPackModuleUpdate:       {},
	sdkPackModuleAds:          {},
	sdkPackModulePluginSource: {},
}

var errSDKPackAppID = errors.New("请选择应用")

type sdkPackInput struct {
	AppID     int64
	AppName   string
	AppKey    string
	AppSecret string
	BaseURL   string
	Modules   []string
	IncludeJS bool
}

type sdkPackHTTPRequest struct {
	AppID     int64    `json:"appId"`
	Modules   []string `json:"modules"`
	BaseURL   string   `json:"baseUrl"`
	IncludeJS *bool    `json:"includeJs"`
}

func normalizeSDKPackModules(raw []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		name := strings.ToLower(strings.TrimSpace(item))
		if name == "" {
			continue
		}
		if _, ok := sdkPackModuleSet[name]; !ok {
			return nil, fmt.Errorf("不支持的模块：%s", item)
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, errors.New("请至少选择一个接入模块")
	}
	ordered := make([]string, 0, len(out))
	for _, name := range sdkPackModuleOrder {
		if seen[name] {
			ordered = append(ordered, name)
		}
	}
	return ordered, nil
}

func normalizeSDKPackBaseURL(raw, fallback string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = strings.TrimSpace(fallback)
	}
	if value == "" {
		return "", errors.New("请填写授权站地址")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("授权站地址不合法，需为 http(s):// 开头")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("授权站地址仅支持 http 或 https")
	}
	if parsed.User != nil {
		return "", errors.New("授权站地址不能包含用户名和密码")
	}
	host := parsed.Host
	hostname, port, err := net.SplitHostPort(host)
	if err != nil {
		hostname = host
		port = ""
	}
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if hostname == "" {
		return "", errors.New("授权站地址缺少主机名")
	}
	if port != "" {
		hostname = net.JoinHostPort(hostname, port)
	}
	return scheme + "://" + hostname, nil
}

func sdkPackHasModule(modules []string, name string) bool {
	for _, item := range modules {
		if item == name {
			return true
		}
	}
	return false
}

func sdkPackSafeName(appKey string) string {
	var b strings.Builder
	for _, r := range appKey {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	name := strings.Trim(b.String(), ".")
	if name == "" {
		return "app"
	}
	return name
}

// sdkPackPluginSourceURL 固化应用隔离清单，等价于 PHP：
// rtrim($origin, '/') . '/software-source/' . rawurlencode($appKey) . '/index.json'
func sdkPackPluginSourceURL(baseURL, appKey string) string {
	return strings.TrimRight(baseURL, "/") + "/software-source/" + url.PathEscape(strings.TrimSpace(appKey)) + "/index.json"
}

func buildSDKPack(input sdkPackInput) ([]byte, string, error) {
	modules, err := normalizeSDKPackModules(input.Modules)
	if err != nil {
		return nil, "", err
	}
	baseURL, err := normalizeSDKPackBaseURL(input.BaseURL, "")
	if err != nil {
		return nil, "", err
	}
	appKey := strings.TrimSpace(input.AppKey)
	appSecret := strings.TrimSpace(input.AppSecret)
	if appKey == "" || appSecret == "" {
		return nil, "", errors.New("应用密钥不完整，无法生成接入包")
	}
	safe := sdkPackSafeName(appKey)
	root := "auth-pro-sdk-" + safe + "/"
	data := sdkPackTemplateData{
		AppID:           input.AppID,
		AppName:         strings.TrimSpace(input.AppName),
		AppKey:          appKey,
		AppSecret:       appSecret,
		BaseURL:         baseURL,
		PluginIndexURL:  sdkPackPluginSourceURL(baseURL, appKey),
		PluginIndexPath: sourceStationPublicIndexPath(appKey),
		License:         sdkPackHasModule(modules, sdkPackModuleLicense),
		Piracy:          sdkPackHasModule(modules, sdkPackModulePiracy),
		Update:          sdkPackHasModule(modules, sdkPackModuleUpdate),
		Ads:             sdkPackHasModule(modules, sdkPackModuleAds),
		PluginSource:    sdkPackHasModule(modules, sdkPackModulePluginSource),
		IncludeJS:       input.IncludeJS,
		GeneratedAt:     time.Now().UTC().Format("2006-01-02 15:04 UTC"),
	}
	data.Guard = data.License || data.Piracy
	data.NeedHTTP = data.Guard || data.Update || data.Ads
	data.NeedSign = data.Guard || data.Update
	data.ModuleLabels = sdkPackModuleLabels(modules)

	php, err := renderSDKPackTemplate("auth_pro_sdk.php", sdkPackPHPTemplate, data)
	if err != nil {
		return nil, "", err
	}
	example, err := renderSDKPackTemplate("config.example.php", sdkPackConfigExampleTemplate, data)
	if err != nil {
		return nil, "", err
	}
	readme, err := renderSDKPackTemplate("README.md", sdkPackReadmeTemplate, data)
	if err != nil {
		return nil, "", err
	}
	files := map[string]string{
		root + "auth_pro_sdk.php":   php,
		root + "config.example.php": example,
		root + "README.md":          readme,
	}
	if input.IncludeJS {
		js, jsErr := renderSDKPackTemplate("auth-pro-sdk.js", sdkPackJSTemplate, data)
		if jsErr != nil {
			return nil, "", jsErr
		}
		files[root+"auth-pro-sdk.js"] = js
	}

	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, body := range files {
		entry, err := archive.CreateHeader(&zip.FileHeader{
			Name:     name,
			Method:   zip.Deflate,
			Modified: time.Now(),
		})
		if err != nil {
			return nil, "", err
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			return nil, "", err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), "auth-pro-sdk-" + safe + ".zip", nil
}

func sdkPackModuleLabels(modules []string) []string {
	labels := map[string]string{
		sdkPackModuleLicense:      "授权验证",
		sdkPackModulePiracy:       "盗版入口",
		sdkPackModuleUpdate:       "在线更新",
		sdkPackModuleAds:          "广告接入",
		sdkPackModulePluginSource: "插件源引用",
	}
	out := make([]string, 0, len(modules))
	for _, name := range modules {
		out = append(out, labels[name])
	}
	return out
}

func sdkPackRequestOrigin(c *gin.Context) string {
	scheme := "http"
	if c.Request != nil && (c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")) {
		scheme = "https"
	}
	host := c.GetHeader("X-Forwarded-Host")
	if host == "" && c.Request != nil {
		host = c.Request.Host
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

func parseSDKPackHTTPRequest(c *gin.Context) (sdkPackInput, error) {
	var raw sdkPackHTTPRequest
	if err := c.ShouldBindJSON(&raw); err != nil {
		return sdkPackInput{}, errors.New("参数错误")
	}
	if raw.AppID <= 0 {
		return sdkPackInput{}, errSDKPackAppID
	}
	modules, err := normalizeSDKPackModules(raw.Modules)
	if err != nil {
		return sdkPackInput{}, err
	}
	baseURL, err := normalizeSDKPackBaseURL(raw.BaseURL, sdkPackRequestOrigin(c))
	if err != nil {
		return sdkPackInput{}, err
	}
	includeJS := true
	if raw.IncludeJS != nil {
		includeJS = *raw.IncludeJS
	}
	return sdkPackInput{
		AppID:     raw.AppID,
		BaseURL:   baseURL,
		Modules:   modules,
		IncludeJS: includeJS,
	}, nil
}

func loadSDKPackApp(db *sql.DB, appID int64) (sdkPackInput, error) {
	var item sdkPackInput
	err := db.QueryRow(`SELECT id, app_name, app_key, app_secret FROM apps WHERE id = ?`, appID).
		Scan(&item.AppID, &item.AppName, &item.AppKey, &item.AppSecret)
	if errors.Is(err, sql.ErrNoRows) {
		return sdkPackInput{}, errors.New("应用不存在")
	}
	return item, err
}

// AdminSDKPackDownload 按单个应用生成可下载的客户端接入 ZIP，密钥只从该应用记录读取。
func AdminSDKPackDownload(c *gin.Context) {
	req, err := parseSDKPackHTTPRequest(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	app, err := loadSDKPackApp(db, req.AppID)
	if err != nil {
		if err.Error() == "应用不存在" {
			c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "应用不存在"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取应用失败"})
		return
	}
	app.BaseURL = req.BaseURL
	app.Modules = req.Modules
	app.IncludeJS = req.IncludeJS
	payload, filename, err := buildSDKPack(app)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "application/zip", payload)
}
