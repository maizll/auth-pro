// Package authpro 是 AuthPro 客户端 SDK（Go）。
//
// 公共 API：Boot / Verify / CheckUpdate / Ads / PluginSourceURL
// 应用差异只写在 config.json，不要改本库源码。
package authpro

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// 授权响应签名的用途标记，签名原文第一行。
	licenseProofKind = "auth-pro-license-v3"
	// 校验通过的结果默认缓存 5 分钟，期间不重复请求授权站。
	defaultCacheTTL = 300
	// 连不上授权站或响应验签不过时，最近一次验签通过的结果最多再用 72 小时（从授权站签名时间算起）。
	defaultOfflineGrace = 72 * 3600
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
	BaseURL    string         `json:"baseUrl"`
	AppID      int64          `json:"appId"`
	AppKey     string         `json:"appKey"`
	AppSecret  string         `json:"appSecret"`
	Modules    map[string]any `json:"modules"`
	LicenseKey string         `json:"licenseKey"`
	Domain     string         `json:"domain"`
	ServerIP   string         `json:"serverIp"`
	AppVersion string         `json:"appVersion"`
	// PublicKey 是授权站的授权响应公钥（base64），接入包里已填好。用它验证响应确实来自授权站。
	PublicKey string `json:"publicKey"`
	// CacheTTL 是校验通过后缓存多少秒，默认 300。
	CacheTTL int `json:"cacheTtl"`
	// OfflineGrace 是连不上授权站时沿用上次通过结果的秒数，默认 259200（72 小时）。
	OfflineGrace int `json:"offlineGrace"`
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

// Verify 仅授权校验，不强制退出进程。overrides 可覆盖 licenseKey / domain / serverIp；
// 浏览器代理转发时再传浏览器给的 nonce，这时不读写本机缓存，结果原样交给浏览器验签。
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
	pub, err := c.publicKey()
	if err != nil {
		return nil, err
	}
	override := firstOverride(overrides)
	ctx := c.requestContext(override)
	nonce := strings.TrimSpace(override["nonce"])
	relay := nonce != ""
	if !relay {
		nonce = newNonce()
	}
	cachePath := c.cachePath(ctx)
	now := time.Now().Unix()
	if !relay {
		if entry, ok := c.readCache(cachePath, ctx, pub); ok && now-entry.SavedAt < int64(positiveOr(c.cfg.CacheTTL, defaultCacheTTL)) {
			return entry.result("cached"), nil
		}
	}
	sign, err := c.v2Sign([]string{
		"v3", c.cfg.AppKey, ctx["licenseKey"], ctx["domain"], ctx["serverIp"], fmt.Sprintf("%d", now), nonce,
	})
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"appKey":      c.cfg.AppKey,
		"domain":      ctx["domain"],
		"serverIp":    ctx["serverIp"],
		"licenseKey":  ctx["licenseKey"],
		"timestamp":   now,
		"signVersion": "v3",
		"nonce":       nonce,
		"sign":        sign,
	}
	body, err := c.httpJSON(http.MethodPost, "/api/license/verify", payload)
	if err != nil {
		return c.offlineResult(cachePath, ctx, pub, relay, nil, err)
	}
	if _, ok := verifyLicenseProof(pub, c.cfg.AppKey, ctx["licenseKey"], nonce, body); !ok {
		return c.offlineResult(cachePath, ctx, pub, relay, body, nil)
	}
	result := normalizeResult(body, true)
	if !relay {
		if result.OK {
			c.writeCache(cachePath, cacheEntry{SavedAt: now, Nonce: nonce, Body: body})
		} else {
			// 授权站明确拒绝（签名有效）：立刻失效，不再用旧缓存放行。
			_ = os.Remove(cachePath)
		}
	}
	return result, nil
}

// offlineResult 处理连不上授权站或响应验签不过：宽限期内沿用上次验签通过的结果，否则拒绝。
// 网络错误且没有可用缓存时返回 error；验签不过时返回 OK=false 的结果，data.unverified=true。
func (c *Client) offlineResult(cachePath string, ctx map[string]string, pub ed25519.PublicKey, relay bool, body map[string]any, netErr error) (*Result, error) {
	if !relay {
		if entry, ok := c.readCache(cachePath, ctx, pub); ok {
			now := time.Now().Unix()
			if now <= entry.ServerTime+int64(positiveOr(c.cfg.OfflineGrace, defaultOfflineGrace)) && (entry.ExpireTs == 0 || now < entry.ExpireTs) {
				return entry.result("offline"), nil
			}
		}
	}
	if netErr != nil {
		return nil, netErr
	}
	result := normalizeResult(body, true)
	result.OK = false
	result.Data = copyData(result.Data)
	result.Data["unverified"] = true
	if result.Message == "" {
		result.Message = "授权响应无法验证"
	}
	return result, nil
}

func (c *Client) publicKey() (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(c.cfg.PublicKey))
	if strings.TrimSpace(c.cfg.PublicKey) == "" || err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("缺少或无效的 publicKey，请在授权站后台重新下载接入包，或从「接入开发」页复制授权响应公钥填入 config.json")
	}
	return ed25519.PublicKey(raw), nil
}

// verifyLicenseProof 核对 data.proof：appKey、nonce、授权码哈希必须是自己这次发出的，
// 再按 docs/api-sdk.md 的规则拼出原文用公钥验签。域名和 IP 取 proof 里授权站规范化后的值，它们已在 v3 请求签名里和 nonce 绑在一起。
// 返回签名里的服务器时间。
func verifyLicenseProof(pub ed25519.PublicKey, appKey, licenseKey, nonce string, body map[string]any) (int64, bool) {
	data, _ := body["data"].(map[string]any)
	proof, _ := data["proof"].(map[string]any)
	if proof == nil {
		return 0, false
	}
	keyHash := ""
	if licenseKey != "" {
		sum := sha256.Sum256([]byte(licenseKey))
		keyHash = hex.EncodeToString(sum[:])
	}
	if text(proof["appKey"]) != appKey || text(proof["nonce"]) != nonce || text(proof["licenseKeyHash"]) != keyHash {
		return 0, false
	}
	serverTime, ok := proof["serverTime"].(float64)
	if !ok {
		return 0, false
	}
	expireTs := ""
	if v, ok := data["expireTs"].(float64); ok {
		expireTs = strconv.FormatInt(int64(v), 10)
	}
	fields := [][2]string{
		{"appKey", appKey},
		{"domain", text(proof["domain"])},
		{"serverIp", text(proof["serverIp"])},
		{"licenseKeyHash", keyHash},
		{"nonce", nonce},
		{"serverTime", strconv.FormatInt(int64(serverTime), 10)},
		{"result", text(data["result"])},
		{"reason", text(data["reason"])},
		{"expireTs", expireTs},
	}
	var b strings.Builder
	b.WriteString(licenseProofKind + "\n")
	clean := strings.NewReplacer("\r", " ", "\n", " ")
	for _, f := range fields {
		b.WriteString(f[0] + "=" + clean.Replace(f[1]) + "\n")
	}
	signature := text(proof["signature"])
	if !strings.HasPrefix(signature, "ed25519:") {
		return 0, false
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(signature, "ed25519:"))
	if err != nil || !ed25519.Verify(pub, []byte(b.String()), sig) {
		return 0, false
	}
	return int64(serverTime), true
}

// cacheEntry 是写在临时目录的校验缓存。读出时重新验签，手改文件没有用。
type cacheEntry struct {
	SavedAt    int64          `json:"savedAt"`
	Nonce      string         `json:"nonce"`
	Body       map[string]any `json:"body"`
	ServerTime int64          `json:"-"`
	ExpireTs   int64          `json:"-"`
}

func (e cacheEntry) result(flag string) *Result {
	result := normalizeResult(e.Body, true)
	result.Data = copyData(result.Data)
	result.Data[flag] = true
	return result
}

func (c *Client) cachePath(ctx map[string]string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{c.cfg.BaseURL, c.cfg.AppKey, ctx["licenseKey"], ctx["domain"], ctx["serverIp"]}, "\n")))
	return filepath.Join(os.TempDir(), "authpro-"+hex.EncodeToString(sum[:12])+".json")
}

func (c *Client) readCache(path string, ctx map[string]string, pub ed25519.PublicKey) (cacheEntry, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return cacheEntry{}, false
	}
	var entry cacheEntry
	if json.Unmarshal(raw, &entry) != nil || !normalizeResult(entry.Body, true).OK {
		return cacheEntry{}, false
	}
	serverTime, ok := verifyLicenseProof(pub, c.cfg.AppKey, ctx["licenseKey"], entry.Nonce, entry.Body)
	if !ok {
		return cacheEntry{}, false
	}
	entry.ServerTime = serverTime
	if data, ok := entry.Body["data"].(map[string]any); ok {
		if v, ok := data["expireTs"].(float64); ok {
			entry.ExpireTs = int64(v)
		}
	}
	return entry, true
}

func (c *Client) writeCache(path string, entry cacheEntry) {
	raw, err := json.Marshal(entry)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, raw, 0o600) == nil {
		_ = os.Rename(tmp, path)
	}
}

func newNonce() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func text(value any) string {
	s, _ := value.(string)
	return s
}

func copyData(data map[string]any) map[string]any {
	out := make(map[string]any, len(data)+1)
	for k, v := range data {
		out[k] = v
	}
	return out
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
