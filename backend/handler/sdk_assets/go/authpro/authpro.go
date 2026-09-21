// Package authpro 是 AuthPro 客户端 SDK（Go）。
//
// 公共 API：Boot / Verify / CheckUpdate / Ads / PluginSourceURL
// 应用差异只写在 config.json，不要改本库源码。
package authpro

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Result 是 verify / checkUpdate 的统一返回结构。
type Result struct {
	OK      bool           `json:"ok"`
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
}

// Config 对应接入包中的 config.json。
type Config struct {
	BaseURL     string         `json:"baseUrl"`
	AppID       int64          `json:"appId"`
	AppKey      string         `json:"appKey"`
	AppSecret   string         `json:"appSecret"`
	Modules     map[string]any `json:"modules"`
	LicenseKey  string         `json:"licenseKey"`
	Domain      string         `json:"domain"`
	ServerIP    string         `json:"serverIp"`
	AppVersion  string         `json:"appVersion"`
}

// Client 持有运行时配置。
type Client struct {
	cfg Config
}

var defaultClient = &Client{}

// Boot 加载配置；license/piracy 开启时校验失败会返回错误（盗版模式带明确文案）。
func Boot(config any) error {
	return defaultClient.Boot(config)
}

// Verify 仅授权校验，不强制退出进程。
func Verify(overrides ...map[string]string) (*Result, error) {
	return defaultClient.Verify(overrides...)
}

// CheckUpdate 在线更新检查。
func CheckUpdate(currentVersion string, overrides ...map[string]string) (*Result, error) {
	return defaultClient.CheckUpdate(currentVersion, overrides...)
}

// Ads 拉取广告位。
func Ads(slot string) (map[string]any, error) {
	return defaultClient.Ads(slot)
}

// PluginSourceURL 返回本应用隔离的插件源清单 URL。
func PluginSourceURL() (string, error) {
	return defaultClient.PluginSourceURL()
}

// LoadConfig 仅加载配置，不触发 boot 校验。
func LoadConfig(config any) error {
	return defaultClient.LoadConfig(config)
}

func (c *Client) Boot(config any) error {
	if err := c.LoadConfig(config); err != nil {
		return err
	}
	if c.moduleEnabled("license") || c.moduleEnabled("piracy") {
		result, err := c.Verify()
		if err != nil {
			return err
		}
		if result == nil || !result.OK {
			return c.denyError(result)
		}
	}
	return nil
}

func (c *Client) LoadConfig(config any) error {
	switch v := config.(type) {
	case string:
		raw, err := os.ReadFile(v)
		if err != nil {
			return fmt.Errorf("读取配置失败: %w", err)
		}
		var next Config
		if err := json.Unmarshal(raw, &next); err != nil {
			return fmt.Errorf("解析 config.json 失败: %w", err)
		}
		c.cfg = next
	case Config:
		c.cfg = v
	case *Config:
		if v == nil {
			return errors.New("config 不能为空")
		}
		c.cfg = *v
	case map[string]any:
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		var next Config
		if err := json.Unmarshal(raw, &next); err != nil {
			return err
		}
		c.cfg = next
	default:
		return fmt.Errorf("不支持的配置类型 %T", config)
	}
	return nil
}

func (c *Client) Verify(overrides ...map[string]string) (*Result, error) {
	ctx := c.requestContext(firstOverride(overrides))
	timestamp := time.Now().Unix()
	sign, err := c.v2Sign([]string{
		"v2", c.cfg.AppKey, ctx["licenseKey"], ctx["domain"], ctx["serverIp"], fmt.Sprintf("%d", timestamp),
	})
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"appKey":      c.cfg.AppKey,
		"domain":      ctx["domain"],
		"serverIp":    ctx["serverIp"],
		"licenseKey":  ctx["licenseKey"],
		"timestamp":   timestamp,
		"signVersion": "v2",
		"sign":        sign,
	}
	body, err := c.httpJSON(http.MethodPost, "/api/license/verify", payload)
	if err != nil {
		return nil, err
	}
	return normalizeResult(body, true), nil
}

func (c *Client) CheckUpdate(currentVersion string, overrides ...map[string]string) (*Result, error) {
	ctx := c.requestContext(firstOverride(overrides))
	if strings.TrimSpace(currentVersion) == "" {
		currentVersion = c.cfg.AppVersion
	}
	if strings.TrimSpace(currentVersion) == "" {
		currentVersion = "1.0.0"
	}
	timestamp := time.Now().Unix()
	sign, err := c.v2Sign([]string{
		"v2", c.cfg.AppKey, currentVersion, ctx["licenseKey"], ctx["domain"], ctx["serverIp"], fmt.Sprintf("%d", timestamp),
	})
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"appKey":         c.cfg.AppKey,
		"currentVersion": currentVersion,
		"domain":         ctx["domain"],
		"serverIp":       ctx["serverIp"],
		"licenseKey":     ctx["licenseKey"],
		"timestamp":      timestamp,
		"signVersion":    "v2",
		"sign":           sign,
	}
	body, err := c.httpJSON(http.MethodPost, "/api/app/version/check", payload)
	if err != nil {
		return nil, err
	}
	if data, ok := body["data"].(map[string]any); ok {
		if download, ok := data["downloadUrl"].(string); ok && download != "" && !strings.HasPrefix(download, "http") {
			data["downloadUrl"] = strings.TrimRight(c.cfg.BaseURL, "/") + download
		}
	}
	return normalizeResult(body, false), nil
}

func (c *Client) Ads(slot string) (map[string]any, error) {
	slot = strings.TrimSpace(slot)
	if slot == "" {
		slot = "home-banner"
	}
	path := "/api/v1/public/advertisements?position=" + url.QueryEscape(slot)
	body, err := c.httpJSON(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if data, ok := body["data"].(map[string]any); ok {
		return data, nil
	}
	return map[string]any{"records": []any{}, "placeholder": nil}, nil
}

func (c *Client) PluginSourceURL() (string, error) {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	if base == "" || strings.TrimSpace(c.cfg.AppKey) == "" {
		return "", errors.New("缺少 baseUrl 或 appKey")
	}
	return base + "/software-source/" + url.PathEscape(strings.TrimSpace(c.cfg.AppKey)) + "/index.json", nil
}

func (c *Client) moduleEnabled(name string) bool {
	if c.cfg.Modules == nil {
		return false
	}
	if raw, ok := c.cfg.Modules[name]; ok {
		switch v := raw.(type) {
		case bool:
			return v
		case string:
			return strings.EqualFold(v, "true") || v == "1"
		}
	}
	return false
}

func (c *Client) requestContext(overrides map[string]string) map[string]string {
	licenseKey := strings.TrimSpace(c.cfg.LicenseKey)
	domain := normalizeDomain(c.cfg.Domain)
	serverIP := normalizeServerIP(c.cfg.ServerIP)
	if overrides != nil {
		if v, ok := overrides["licenseKey"]; ok {
			licenseKey = strings.TrimSpace(v)
		}
		if v, ok := overrides["domain"]; ok {
			domain = normalizeDomain(v)
		}
		if v, ok := overrides["serverIp"]; ok {
			serverIP = normalizeServerIP(v)
		}
	}
	return map[string]string{
		"licenseKey": licenseKey,
		"domain":     domain,
		"serverIp":   serverIP,
	}
}

func (c *Client) v2Sign(parts []string) (string, error) {
	secret := c.cfg.AppSecret
	if strings.TrimSpace(secret) == "" {
		return "", errors.New("缺少 appSecret，无法签名。请在服务端 config.json 中配置。")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (c *Client) denyError(result *Result) error {
	title := "授权无效"
	if c.moduleEnabled("piracy") {
		title = "未授权访问"
	}
	reason := ""
	if result != nil {
		reason = result.Message
		if reason == "" && result.Data != nil {
			if v, ok := result.Data["reason"].(string); ok {
				reason = v
			}
		}
	}
	if reason == "" {
		return errors.New(title)
	}
	return fmt.Errorf("%s: %s", title, reason)
}

func (c *Client) httpJSON(method, path string, payload any) (map[string]any, error) {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	if base == "" {
		return nil, errors.New("缺少 baseUrl")
	}
	var reader io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, base+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func normalizeResult(body map[string]any, requirePass bool) *Result {
	code := 0
	switch v := body["code"].(type) {
	case float64:
		code = int(v)
	case int:
		code = v
	}
	message := ""
	if v, ok := body["msg"].(string); ok {
		message = v
	} else if v, ok := body["message"].(string); ok {
		message = v
	}
	var data map[string]any
	if v, ok := body["data"].(map[string]any); ok {
		data = v
	}
	ok := code == 200
	if requirePass {
		ok = ok && data != nil && data["result"] == "pass"
	}
	return &Result{OK: ok, Code: code, Message: message, Data: data}
}

func firstOverride(overrides []map[string]string) map[string]string {
	if len(overrides) == 0 {
		return nil
	}
	return overrides[0]
}

func normalizeDomain(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	if idx := strings.Index(value, "/"); idx >= 0 {
		value = value[:idx]
	}
	value = strings.Trim(value, "[]")
	if host, _, err := splitHostPortLoose(value); err == nil {
		value = host
	}
	return strings.TrimRight(value, ".")
}

func normalizeServerIP(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.LastIndex(value, "%"); idx >= 0 {
		value = value[:idx]
	}
	return strings.Trim(value, "[]")
}

func splitHostPortLoose(hostport string) (host, port string, err error) {
	if !strings.Contains(hostport, ":") {
		return hostport, "", fmt.Errorf("no port")
	}
	// IPv6 with multiple colons — leave as-is
	if strings.Count(hostport, ":") > 1 {
		return hostport, "", fmt.Errorf("ipv6")
	}
	parts := strings.SplitN(hostport, ":", 2)
	if parts[1] == "" || !isAllDigits(parts[1]) {
		return hostport, "", fmt.Errorf("bad port")
	}
	return parts[0], parts[1], nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
