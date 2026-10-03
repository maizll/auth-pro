package handler

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

const (
	// 官网自己更新用的仓库。和分发给客户的仓库分开，避免改一个默认值把另一边带偏。
	// 这一段只在官网（持有快照私钥的进程）上执行，客户站的更新只认官网接口，不会走到这里。
	officialUpdateRepoEnv           = "AUTO_PRO_OFFICIAL_UPDATE_REPOSITORY"
	officialUpdateDefaultRepository = "maizll/auth-pro"
	officialUpdateUnavailable       = "暂时无法获取更新"

	// 官网专用只读令牌：只授权服务端仓库、只给 Contents 只读。加密后放在数据目录 store/ 下，权限 0600。
	// 保存了它，官网更新只用它，失败就直接报出来，不再退回别的令牌或匿名，免得仓库改私有后才发现令牌早就失效。
	officialUpdateTokenFile    = "official-update-token"
	officialUpdateTokenKeyFile = "official-update.key"

	officialCredentialToken     = "official_token"
	officialCredentialSaved     = "saved_token"
	officialCredentialAnonymous = "anonymous"
)

var officialUpdateTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_]{20,255}$`)

// officialUpdateCheck 记下官网最近一次读取发布仓库用的是哪种凭据、成没成功。只放内存，重启后重新记。
type officialUpdateCheck struct {
	Credential string    `json:"credential"`
	OK         bool      `json:"ok"`
	Status     int       `json:"status,omitempty"`
	At         time.Time `json:"at"`
}

var officialUpdateLastCheck struct {
	mu    sync.Mutex
	value *officialUpdateCheck
}

func officialUpdateRepository() (string, string, error) {
	raw := strings.Trim(strings.TrimSpace(os.Getenv(officialUpdateRepoEnv)), "/")
	if raw == "" {
		raw = officialUpdateDefaultRepository
	}
	parts := strings.Split(raw, "/")
	if len(parts) != 2 || !sourceReleaseRepoPattern.MatchString(parts[0]) || !sourceReleaseRepoPattern.MatchString(parts[1]) {
		return "", "", errProductUpdateRepoMissing
	}
	return parts[0], parts[1], nil
}

func fetchOfficialSiteUpdateManifest() (*onlineUpdateManifest, error) {
	body, err := fetchOfficialReleaseAsset(context.Background(), "latest", "latest.json")
	if err != nil {
		return nil, err
	}
	var manifest onlineUpdateManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, errors.New("更新清单不是有效的 JSON")
	}
	normalizeOnlineUpdateManifest(&manifest)
	if err := validateOnlineUpdateManifest(&manifest); err != nil {
		return nil, err
	}
	redactOfficialUpdateLocations(&manifest)
	return &manifest, nil
}

func fetchOfficialSiteUpdateReleases() ([]onlineUpdateRelease, string, error) {
	body, err := fetchOfficialReleaseAsset(context.Background(), "latest", "releases.json")
	if err != nil {
		return nil, "", err
	}
	var payload onlineUpdateReleases
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "", errors.New("历史版本清单不是有效的 JSON")
	}
	if err := normalizeOnlineUpdateReleases(&payload); err != nil {
		return nil, "", err
	}
	for index := range payload.Releases {
		payload.Releases[index].Notes = filterProductUpdateNotes(payload.Releases[index].Notes)
	}
	return payload.Releases, "", nil
}

func downloadOfficialSiteUpdatePackage(jobID string, manifest *onlineUpdateManifest) (string, error) {
	if manifest == nil {
		return "", errors.New(officialUpdateUnavailable)
	}
	version := strings.TrimPrefix(strings.TrimSpace(manifest.Version), "v")
	fileName := filepath.Base(strings.TrimSpace(manifest.Package.FileName))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	body, err := fetchOfficialReleaseAsset(ctx, "tags/v"+version, fileName)
	if err != nil {
		return "", fmt.Errorf("下载更新包失败：%w", err)
	}
	if int64(len(body)) != manifest.Package.Size {
		return "", fmt.Errorf("更新包大小不一致，期望 %d 字节，实际 %d 字节", manifest.Package.Size, len(body))
	}
	sum := sha256.Sum256(body)
	actual := hex.EncodeToString(sum[:])
	if !strings.EqualFold(actual, manifest.Package.SHA256) {
		return "", fmt.Errorf("更新包 SHA256 不一致，期望 %s，实际 %s", manifest.Package.SHA256, actual)
	}
	target := filepath.Join(config.GetUpdateDir(), jobID+"-"+fileName)
	if err := os.WriteFile(target, body, 0644); err != nil {
		return "", fmt.Errorf("保存更新包失败：%w", err)
	}
	return target, nil
}

func fetchOfficialReleaseAsset(ctx context.Context, releaseRef, assetName string) ([]byte, error) {
	owner, repo, err := officialUpdateRepository()
	if err != nil {
		return nil, err
	}
	tokens, kinds, dedicated := officialUpdateTokens()
	payload, used, err := fetchGitHubReleaseAssetWith(ctx, owner, repo, releaseRef, assetName, tokens)
	record := &officialUpdateCheck{OK: err == nil, At: time.Now()}
	if err == nil && used >= 0 && used < len(kinds) {
		record.Credential = kinds[used]
	} else {
		record.Credential = kinds[len(kinds)-1]
		record.Status = productUpdateHTTPStatus(err)
	}
	officialUpdateLastCheck.mu.Lock()
	officialUpdateLastCheck.value = record
	officialUpdateLastCheck.mu.Unlock()
	if err != nil && dedicated {
		// 只在官网后台显示。客户站拿不到这段说明。
		return nil, errors.New("官网更新令牌读取失败：" + officialUpdateFetchReason(productUpdateHTTPStatus(err)))
	}
	return payload, err
}

// officialUpdateTokens 返回官网读取发布仓库要依次尝试的令牌，以及每个令牌属于哪种凭据。
// 保存了专用只读令牌就只用它；没保存时沿用分发客户包的那组令牌，最后匿名（仓库改私有后匿名会失败）。
func officialUpdateTokens() ([]string, []string, bool) {
	if token, err := loadOfficialUpdateToken(); err == nil && token != "" {
		return []string{token}, []string{officialCredentialToken}, true
	}
	tokens := productUpdateTokenCandidates()
	kinds := make([]string, len(tokens))
	for index, token := range tokens {
		if token == "" {
			kinds[index] = officialCredentialAnonymous
		} else {
			kinds[index] = officialCredentialSaved
		}
	}
	return tokens, kinds, false
}

func officialUpdateFetchReason(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "令牌无效或已过期"
	case http.StatusForbidden:
		return "令牌没有读取权限，或请求太频繁"
	case http.StatusNotFound:
		return "读不到这个仓库的发布：令牌没有选中该仓库，或仓库还没有发布"
	case 0:
		return "连不上代码托管站"
	default:
		return fmt.Sprintf("代码托管站返回 %d", status)
	}
}

func officialUpdateTokenPath() string {
	return filepath.Join(config.GetDataDir(), "store", officialUpdateTokenFile)
}

func loadOfficialUpdateToken() (string, error) {
	payload, err := os.ReadFile(officialUpdateTokenPath())
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(payload)))
	if err != nil {
		return "", err
	}
	key, err := loadOrCreateStoreFileKey(officialUpdateTokenKeyFile)
	if err != nil {
		return "", err
	}
	plain, err := openStoreSecret(key, raw)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(plain)), nil
}

// saveOfficialUpdateToken 加密保存专用只读令牌。传空字符串表示删除，官网回到原来的令牌顺序。
func saveOfficialUpdateToken(token string) error {
	token = strings.TrimSpace(token)
	path := officialUpdateTokenPath()
	if token == "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if !officialUpdateTokenPattern.MatchString(token) {
		return errors.New("令牌格式不对：只能是字母、数字和下划线，长度 20 到 255")
	}
	key, err := loadOrCreateStoreFileKey(officialUpdateTokenKeyFile)
	if err != nil {
		return err
	}
	blob, err := sealStoreSecret(key, []byte(token))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(base64.StdEncoding.EncodeToString(blob)), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// officialUpdateSourceView 是在线更新页「官网更新来源」一栏的数据。只在官网返回。
type officialUpdateSourceView struct {
	Repository string               `json:"repository"`
	TokenSaved bool                 `json:"tokenSaved"`
	TokenHint  string               `json:"tokenHint"`
	LastCheck  *officialUpdateCheck `json:"lastCheck,omitempty"`
}

func currentOfficialUpdateSource() *officialUpdateSourceView {
	if !officialSite() {
		return nil
	}
	view := &officialUpdateSourceView{}
	if owner, repo, err := officialUpdateRepository(); err == nil {
		view.Repository = owner + "/" + repo
	}
	if token, err := loadOfficialUpdateToken(); err == nil && token != "" {
		view.TokenSaved = true
		if len(token) > 4 {
			view.TokenHint = token[len(token)-4:]
		}
	}
	officialUpdateLastCheck.mu.Lock()
	if officialUpdateLastCheck.value != nil {
		copyCheck := *officialUpdateLastCheck.value
		view.LastCheck = &copyCheck
	}
	officialUpdateLastCheck.mu.Unlock()
	return view
}

// AdminOfficialUpdateTokenSave 保存或删除官网专用只读令牌。客户站没有这一项。
func AdminOfficialUpdateTokenSave(c *gin.Context) {
	if !officialSite() {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "只有官网需要设置更新令牌"})
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if err := saveOfficialUpdateToken(req.Token); err != nil {
		if strings.HasPrefix(err.Error(), "令牌格式") {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
			return
		}
		log.Printf("save official update token failed: %v", err)
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存令牌失败"})
		return
	}
	msg := "令牌已保存，官网更新以后只用这个令牌"
	if strings.TrimSpace(req.Token) == "" {
		msg = "令牌已删除"
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": msg, "data": currentOfficialUpdateSource()})
}

// officialUpdateSourceTest 是「测试读取」的结果：令牌能不能读到最新清单，匿名能不能读到（能读到说明仓库还是公开的）。
type officialUpdateSourceTest struct {
	TokenOK           bool   `json:"tokenOk"`
	TokenMessage      string `json:"tokenMessage"`
	LatestVersion     string `json:"latestVersion,omitempty"`
	AnonymousReadable bool   `json:"anonymousReadable"`
}

// AdminOfficialUpdateSourceTest 只读不装：用已保存的令牌读一次 latest.json，再匿名读一次发布信息。
func AdminOfficialUpdateSourceTest(c *gin.Context) {
	if !officialSite() {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "只有官网需要设置更新令牌"})
		return
	}
	owner, repo, err := officialUpdateRepository()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Minute)
	defer cancel()
	result := officialUpdateSourceTest{}
	token, tokenErr := loadOfficialUpdateToken()
	if tokenErr != nil || token == "" {
		result.TokenMessage = "还没有保存官网更新令牌"
	} else {
		body, _, fetchErr := fetchGitHubReleaseAssetWith(ctx, owner, repo, "latest", "latest.json", []string{token})
		if fetchErr != nil {
			result.TokenMessage = officialUpdateFetchReason(productUpdateHTTPStatus(fetchErr))
		} else {
			var manifest onlineUpdateManifest
			if json.Unmarshal(body, &manifest) == nil && strings.TrimSpace(manifest.Version) != "" {
				result.TokenOK = true
				result.LatestVersion = strings.TrimPrefix(strings.TrimSpace(manifest.Version), "v")
				result.TokenMessage = "令牌可以读取最新版本 v" + result.LatestVersion
			} else {
				result.TokenMessage = "读到的更新清单不是有效的 JSON"
			}
		}
	}
	releaseURL := strings.TrimRight(productUpdateGitHubAPI, "/") + "/repos/" + owner + "/" + repo + "/releases/latest"
	if _, anonErr := productUpdateFetch(ctx, releaseURL, "", "application/vnd.github+json"); anonErr == nil {
		result.AnonymousReadable = true
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": result})
}

// redactOfficialUpdateLocations 去掉清单里的下载地址，避免后台接口把仓库地址带出去。
func redactOfficialUpdateLocations(manifest *onlineUpdateManifest) {
	if manifest == nil {
		return
	}
	manifest.Package.URL = ""
	manifest.URL = ""
	manifest.ReleasesURL = ""
	manifest.Notes = filterProductUpdateNotes(manifest.Notes)
}
