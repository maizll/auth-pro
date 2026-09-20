package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	FrontendModeDisk  = "disk"
	FrontendModeEmbed = "embed"

	// EnvAllowEmbeddedFrontend 显式允许用 go:embed 的 static 做开发/引导页。
	// 生产部署不得依赖该开关；缺盘上前端时应失败而不是静默回退。
	EnvAllowEmbeddedFrontend = "AUTO_PRO_ALLOW_EMBEDDED_FRONTEND"
)

// FrontendRoot 是 HTTP 服务与在线更新共用的前端根解析结果。
type FrontendRoot struct {
	Mode        string
	Dir         string
	ApplyDir    string
	Fingerprint string
}

// AllowEmbeddedFrontend 仅在显式 opt-in 时为真。
func AllowEmbeddedFrontend() bool {
	value := strings.TrimSpace(os.Getenv(EnvAllowEmbeddedFrontend))
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

// FrontendDirValid 表示目录含有可服务的 index.html。
func FrontendDirValid(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, "index.html"))
	return err == nil && !info.IsDir()
}

// ResolveFrontendRoot 解析唯一前端根，供 HTTP 与更新/落地共用。
// AUTO_PRO_FRONTEND_DIR 已配置但无效时必定失败（即使允许 embed）。
// 未配置且找不到盘上产物时：仅当 AUTO_PRO_ALLOW_EMBEDDED_FRONTEND=1 才走 embed。
func ResolveFrontendRoot() (FrontendRoot, error) {
	applyDir := intendedFrontendApplyDir()
	if configured := strings.TrimSpace(os.Getenv("AUTO_PRO_FRONTEND_DIR")); configured != "" {
		if !FrontendDirValid(configured) {
			return FrontendRoot{ApplyDir: configured}, fmt.Errorf(
				"AUTO_PRO_FRONTEND_DIR=%s 缺少有效 index.html；拒绝静默回退到内嵌前端", configured)
		}
		return diskFrontendRoot(configured), nil
	}

	executable, _ := os.Executable()
	if dir := firstValidFrontendCandidate(getDataDir(), executable); dir != "" {
		return diskFrontendRoot(dir), nil
	}
	if AllowEmbeddedFrontend() {
		return FrontendRoot{Mode: FrontendModeEmbed, ApplyDir: applyDir}, nil
	}
	return FrontendRoot{ApplyDir: applyDir}, fmt.Errorf(
		"未找到盘上前端（期望 %s）。请解压发布包使 index.html 落在该目录，或设置 AUTO_PRO_FRONTEND_DIR；内嵌前端仅开发/引导可用：%s=1",
		applyDir, EnvAllowEmbeddedFrontend)
}

func diskFrontendRoot(dir string) FrontendRoot {
	return FrontendRoot{
		Mode:        FrontendModeDisk,
		Dir:         dir,
		ApplyDir:    dir,
		Fingerprint: FingerprintDir(dir),
	}
}

func intendedFrontendApplyDir() string {
	if dir := strings.TrimSpace(os.Getenv("AUTO_PRO_FRONTEND_DIR")); dir != "" {
		return dir
	}
	executable, _ := os.Executable()
	return resolveFrontendDir(getDataDir(), executable)
}

func firstValidFrontendCandidate(dataDir, executable string) string {
	for _, dir := range frontendCandidates(dataDir, executable) {
		if FrontendDirValid(dir) {
			return dir
		}
	}
	return ""
}

func frontendCandidates(dataDir, executable string) []string {
	candidates := []string{
		filepath.Join(dataDir, "frontend", "current"),
		filepath.Join(filepath.Dir(dataDir), "frontend", "current"),
	}
	if executable != "" {
		executableDir := filepath.Dir(executable)
		candidates = append(candidates, executableDir, filepath.Dir(executableDir))
	}
	return candidates
}

func defaultFrontendApplyDir(dataDir, executable string) string {
	if executable != "" {
		executableDir := filepath.Dir(executable)
		if filepath.Base(executableDir) == "backend" {
			return filepath.Dir(executableDir)
		}
	}
	if dataDir != "" {
		return filepath.Join(dataDir, "frontend", "current")
	}
	return filepath.Join("frontend", "current")
}

// FingerprintDir 用 index.html 内容哈希 + 首个静态资源名标识盘上前端。
func FingerprintDir(dir string) string {
	sum, err := sha256FilePrefix(filepath.Join(dir, "index.html"), 12)
	if err != nil {
		return "missing"
	}
	if asset := firstFrontendAssetName(dir); asset != "" {
		return fmt.Sprintf("index.html:%s asset=%s", sum, asset)
	}
	return fmt.Sprintf("index.html:%s", sum)
}

// FingerprintFS 标识 embed 前端（开发/引导）。
func FingerprintFS(fsys fs.FS) string {
	if fsys == nil {
		return "embed:missing"
	}
	file, err := fsys.Open("index.html")
	if err != nil {
		return "embed:missing"
	}
	defer file.Close()
	sum, err := sha256ReaderPrefix(file, 12)
	if err != nil {
		return "embed:missing"
	}
	if asset := firstFrontendAssetNameFS(fsys); asset != "" {
		return fmt.Sprintf("embed:index.html:%s asset=%s", sum, asset)
	}
	return fmt.Sprintf("embed:index.html:%s", sum)
}

func firstFrontendAssetName(dir string) string {
	entries, err := os.ReadDir(filepath.Join(dir, "assets"))
	if err != nil {
		return ""
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".css") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func firstFrontendAssetNameFS(fsys fs.FS) string {
	entries, err := fs.ReadDir(fsys, "assets")
	if err != nil {
		return ""
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".css") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func sha256FilePrefix(path string, n int) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return sha256ReaderPrefix(file, n)
}

func sha256ReaderPrefix(reader io.Reader, n int) (string, error) {
	sum := sha256.New()
	if _, err := io.Copy(sum, reader); err != nil {
		return "", err
	}
	hexSum := hex.EncodeToString(sum.Sum(nil))
	if n > 0 && n < len(hexSum) {
		return hexSum[:n], nil
	}
	return hexSum, nil
}
