package config

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DBConfig 数据库配置
type DBConfig struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	Database string `json:"database"`
	Username string `json:"username"`
	Password string `json:"password"`
}

var (
	dbConfig *DBConfig
	mu       sync.RWMutex

	jwtSecret []byte
	secretMu  sync.RWMutex
)

// AppVersion 是当前系统整体版本号。前后端共用该版本。
// 发布构建通过 -ldflags 从 git tag / VERSION 注入；仓库默认必须与根目录 VERSION（当前 1.5.x）一致，
// 避免忘记 -ldflags 时静默显示 1.0.0。
var AppVersion = "1.5.0"

// BuildTime 是二进制构建时间，发布时通过 -ldflags 注入。
var BuildTime = ""

// DefaultUpdateManifestURL 是默认的在线更新清单地址，可用 AUTO_PRO_UPDATE_URL 覆盖。
// 发布面冻结在 GitHub maizll/auth-pro Releases（latest.json / 标签附件）。
const DefaultUpdateManifestURL = "https://api.github.com/repos/maizll/auth-pro/releases/latest"

// GetDataDir 获取运行数据目录。
func GetDataDir() string {
	return getDataDir()
}

// GetUpdateManifestURL 获取在线更新清单地址。
func GetUpdateManifestURL() string {
	if value := os.Getenv("AUTO_PRO_UPDATE_URL"); value != "" {
		return value
	}
	return DefaultUpdateManifestURL
}

// GetUpdateDir 获取更新包与更新日志目录。
func GetUpdateDir() string {
	dir := filepath.Join(getDataDir(), "updates")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// GetAppReleaseDir 获取应用版本更新包目录。
func GetAppReleaseDir() string {
	if dir := os.Getenv("AUTO_PRO_APP_RELEASE_DIR"); dir != "" {
		_ = os.MkdirAll(dir, 0755)
		return dir
	}
	dir := filepath.Join(getDataDir(), "app-releases")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// GetFrontendDir 返回在线更新落地与 HTTP 共用的盘上前端目录。
// 与 ResolveFrontendRoot().ApplyDir 同一套候选：AUTO_PRO_FRONTEND_DIR、
// data/frontend/current、可执行文件旁或宝塔网站根。
func GetFrontendDir() string {
	return intendedFrontendApplyDir()
}

func resolveFrontendDir(dataDir string, executable string) string {
	if dir := firstValidFrontendCandidate(dataDir, executable); dir != "" {
		return dir
	}
	return defaultFrontendApplyDir(dataDir, executable)
}

// GetServiceName 获取 systemd 服务名。
func GetServiceName() string {
	if name := os.Getenv("AUTO_PRO_SERVICE_NAME"); name != "" {
		return name
	}
	return "auth_pro"
}

// 获取数据文件目录
func getDataDir() string {
	if dir := os.Getenv("AUTO_PRO_DATA_DIR"); dir != "" {
		_ = os.MkdirAll(dir, 0755)
		return dir
	}

	if cwd, err := os.Getwd(); err == nil {
		candidates := []string{cwd, filepath.Join(cwd, "backend")}
		for _, dir := range candidates {
			if fileExists(filepath.Join(dir, "install.lock")) || fileExists(filepath.Join(dir, "db.json")) || fileExists(filepath.Join(dir, "go.mod")) {
				return dir
			}
		}
	}

	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}

	return "."
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// 获取配置文件路径
func getConfigPath() string {
	return filepath.Join(getDataDir(), "db.json")
}

// 获取 install.lock 路径
func GetLockPath() string {
	return filepath.Join(getDataDir(), "install.lock")
}

// IsInstalled 检查是否已安装
func IsInstalled() bool {
	_, err := os.Stat(GetLockPath())
	return err == nil
}

// CreateLockFile 创建安装锁文件
func CreateLockFile() error {
	return os.WriteFile(GetLockPath(), []byte("installed"), 0644)
}

// ClearCachedDBConfig 丢掉内存中的数据库配置缓存。安装测试用它避免把临时配置泄漏到后续用例。
func ClearCachedDBConfig() {
	mu.Lock()
	dbConfig = nil
	mu.Unlock()
}

// SaveDBConfig 保存数据库配置
func SaveDBConfig(cfg *DBConfig) error {
	mu.Lock()
	defer mu.Unlock()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	dbConfig = cfg
	return os.WriteFile(getConfigPath(), data, 0644)
}

// LoadDBConfig 加载数据库配置
func LoadDBConfig() (*DBConfig, error) {
	if cfg, ok := loadDBConfigFromEnv(); ok {
		return cfg, nil
	}

	mu.RLock()
	if dbConfig != nil {
		defer mu.RUnlock()
		return dbConfig, nil
	}
	mu.RUnlock()

	mu.Lock()
	defer mu.Unlock()

	data, err := os.ReadFile(getConfigPath())
	if err != nil {
		return nil, err
	}

	var cfg DBConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	dbConfig = &cfg
	return dbConfig, nil
}

// GetDSN 获取数据库连接字符串
func GetDSN(cfg *DBConfig) string {
	network := "tcp"
	address := cfg.Host + ":" + cfg.Port
	if strings.HasPrefix(cfg.Host, "unix:") {
		network = "unix"
		address = strings.TrimPrefix(cfg.Host, "unix:")
	}
	return cfg.Username + ":" + cfg.Password + "@" + network + "(" + address + ")/" + cfg.Database + "?charset=utf8mb4&parseTime=True&loc=Local&multiStatements=true"
}

func loadDBConfigFromEnv() (*DBConfig, bool) {
	host := strings.TrimSpace(os.Getenv("AUTO_PRO_DB_HOST"))
	if host == "" {
		return nil, false
	}
	port := strings.TrimSpace(os.Getenv("AUTO_PRO_DB_PORT"))
	if port == "" && !strings.HasPrefix(host, "unix:") {
		port = "3306"
	}
	return &DBConfig{
		Host:     host,
		Port:     port,
		Database: strings.TrimSpace(os.Getenv("AUTO_PRO_DB_NAME")),
		Username: os.Getenv("AUTO_PRO_DB_USER"),
		Password: os.Getenv("AUTO_PRO_DB_PASSWORD"),
	}, true
}

// GetPort 获取服务端口
// GetPluginDir 获取本地插件存放目录
func GetPluginDir() string {
	dir := filepath.Join(getDataDir(), "plugins")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// GetHomeTemplateDir 获取已安装首页模板的存放目录。
func GetHomeTemplateDir() string {
	dir := filepath.Join(getDataDir(), "home-templates")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// LocalSoftwareSourceAdminPath 是未配置远程软件源管理后台时，旧 /admin/app-store 的本站入口。
const LocalSoftwareSourceAdminPath = "/plugin-store"

// GetSoftwareSourceURL 返回远程软件源地址。本分叉默认空（不连接官方源），
// 可用 AUTO_PRO_SOFTWARE_SOURCE_URL 指向自建或可选的远程源。
func GetSoftwareSourceURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("AUTO_PRO_SOFTWARE_SOURCE_URL")), "/")
}

// GetSoftwareSourceAPIKey 返回远程软件源目录 Key。默认空，不内置任何官方密钥。
func GetSoftwareSourceAPIKey() string {
	return strings.TrimSpace(os.Getenv("AUTO_PRO_SOFTWARE_SOURCE_API_KEY"))
}

func GetSoftwareSourceAdminURL() string {
	if value := strings.TrimSpace(os.Getenv("AUTO_PRO_SOFTWARE_SOURCE_ADMIN_URL")); value != "" {
		return strings.TrimRight(value, "/") + "/"
	}
	if base := GetSoftwareSourceURL(); base != "" {
		return base + "/admin/"
	}
	return ""
}

// GetSoftwareSourceAdminRedirect 是旧 /admin/app-store 的跳转目标。
// 未配置远程管理后台时落到本站应用商店，避免跳转到官方域名。
func GetSoftwareSourceAdminRedirect() string {
	if value := GetSoftwareSourceAdminURL(); value != "" {
		return value
	}
	return LocalSoftwareSourceAdminPath
}

func GetSoftwareSourceTimeout() time.Duration {
	return durationEnv("AUTO_PRO_SOFTWARE_SOURCE_TIMEOUT", 5*time.Second)
}

func GetSoftwareSourceStaleTTL() time.Duration {
	return durationEnv("AUTO_PRO_SOFTWARE_SOURCE_STALE_TTL", 24*time.Hour)
}

func GetSoftwareSourceCacheDir() string {
	dir := filepath.Join(getDataDir(), "software-source-cache")
	_ = os.MkdirAll(dir, 0750)
	return dir
}

// DefaultAdvertisementURL 是本站自托管广告接口（相对路径，不发起外网请求）。
// 需要代理到其他投放服务时，用 AUTO_PRO_ADVERTISEMENT_URL 覆盖为绝对 http(s) 地址。
const DefaultAdvertisementURL = "/api/v1/public/advertisements"

func GetAdvertisementURL() string {
	value := strings.TrimSpace(os.Getenv("AUTO_PRO_ADVERTISEMENT_URL"))
	if value == "" {
		value = DefaultAdvertisementURL
	}
	return strings.TrimRight(value, "/")
}

// AdvertisementURLIsRemote 表示广告代理需要 HTTP 拉取外部投放接口。
// 空值或相对路径视为本进程自托管，不得访问官方域名。
func AdvertisementURLIsRemote() bool {
	return isRemoteHTTPURL(GetAdvertisementURL())
}

func isRemoteHTTPURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func GetAdvertisementTimeout() time.Duration {
	return durationEnv("AUTO_PRO_ADVERTISEMENT_TIMEOUT", 5*time.Second)
}

// GetAdvertisementCacheTTL 是投放内容的新鲜期。广告图片是带签名的临时地址，
// 缓存过久会让前端拿到已过期的链接，所以默认只留 5 分钟。
func GetAdvertisementCacheTTL() time.Duration {
	return durationEnv("AUTO_PRO_ADVERTISEMENT_CACHE_TTL", 5*time.Minute)
}

// GetAdvertisementStaleTTL 是上游不可用时旧内容的容忍期，超出后广告位改回占位。
func GetAdvertisementStaleTTL() time.Duration {
	return durationEnv("AUTO_PRO_ADVERTISEMENT_STALE_TTL", time.Hour)
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func GetPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "19127"
	}
	return port
}

// GetHost 获取服务监听地址，生产环境应设置为 127.0.0.1。
func GetHost() string {
	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	return host
}

// GetJWTSecretPath 获取 JWT 密钥文件路径
func GetJWTSecretPath() string {
	return filepath.Join(getDataDir(), "jwt.secret")
}

// LoadOrCreateJWTSecret 读取 JWT 密钥；文件不存在时生成随机密钥并持久化。
// 安装流程中会调用本函数生成密钥，之后所有进程复用同一密钥。
func LoadOrCreateJWTSecret() ([]byte, error) {
	secretMu.RLock()
	if jwtSecret != nil {
		defer secretMu.RUnlock()
		return jwtSecret, nil
	}
	secretMu.RUnlock()

	secretMu.Lock()
	defer secretMu.Unlock()
	if jwtSecret != nil {
		return jwtSecret, nil
	}

	if data, err := os.ReadFile(GetJWTSecretPath()); err == nil {
		if secret := bytes.TrimSpace(data); len(secret) >= 32 {
			jwtSecret = secret
			return jwtSecret, nil
		}
	}

	raw := make([]byte, 48)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	secret := []byte(hex.EncodeToString(raw))
	if err := os.WriteFile(GetJWTSecretPath(), secret, 0600); err != nil {
		return nil, err
	}
	jwtSecret = secret
	return jwtSecret, nil
}
