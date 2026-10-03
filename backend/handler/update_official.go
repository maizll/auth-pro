package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"auto_pro/config"
)

const (
	// 官网自己更新用的仓库。和分发给客户的仓库分开，避免改一个默认值把另一边带偏。
	// 这一段只在官网（持有快照私钥的进程）上执行，客户站的更新只认官网接口，不会走到这里。
	officialUpdateRepoEnv           = "AUTO_PRO_OFFICIAL_UPDATE_REPOSITORY"
	officialUpdateDefaultRepository = "maizll/auth-pro"
	officialUpdateUnavailable       = "暂时无法获取更新"

	// 1.8.5–1.8.7 单独保存的「官网更新令牌」文件。1.8.8 起不再使用，启动时删除，见 RemoveLegacyOfficialUpdateToken。
	legacyOfficialUpdateTokenFile    = "official-update-token"
	legacyOfficialUpdateTokenKeyFile = "official-update.key"
)

// officialUpdateCheck 记下官网最近一次读取发布仓库用的是哪个令牌、成没成功。只放内存，重启后重新记。
type officialUpdateCheck struct {
	OK     bool      `json:"ok"`
	Source string    `json:"source"`
	Reason string    `json:"reason,omitempty"`
	At     time.Time `json:"at"`
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
	sources := productUpdateTokenSources()
	tokens := make([]string, len(sources))
	for index, source := range sources {
		tokens[index] = source.Token
	}
	payload, used, err := fetchGitHubReleaseAssetWith(ctx, owner, repo, releaseRef, assetName, tokens)
	record := &officialUpdateCheck{OK: err == nil, At: time.Now()}
	if err == nil && used >= 0 && used < len(sources) {
		record.Source = sources[used].Label
	} else {
		record.Reason = officialUpdateFetchReason(productUpdateHTTPStatus(err))
	}
	officialUpdateLastCheck.mu.Lock()
	officialUpdateLastCheck.value = record
	officialUpdateLastCheck.mu.Unlock()
	return payload, err
}

// officialUpdateFetchReason 把读取失败的状态码说成大白话。依次试过所有令牌，这里是最后一个的结果。
func officialUpdateFetchReason(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "令牌无效或已过期"
	case http.StatusForbidden:
		return "令牌没有读取权限，或请求太频繁"
	case http.StatusNotFound:
		return "存储管理里的令牌都读不到这个仓库的发布（令牌没有选中该仓库，或仓库还没有发布）"
	case 0:
		return "连不上代码托管站"
	default:
		return fmt.Sprintf("代码托管站返回 %d", status)
	}
}

// officialUpdateSourceView 是在线更新页「更新来源」那一行的数据。只在官网返回。
type officialUpdateSourceView struct {
	Repository string               `json:"repository"`
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
	officialUpdateLastCheck.mu.Lock()
	if officialUpdateLastCheck.value != nil {
		copyCheck := *officialUpdateLastCheck.value
		view.LastCheck = &copyCheck
	}
	officialUpdateLastCheck.mu.Unlock()
	return view
}

// RemoveLegacyOfficialUpdateToken 删除 1.8.5–1.8.7 单独保存的官网更新令牌（密文和它的密钥一起删）。
// 为什么直接删而不是迁进存储管理：
//   - 站长决定只保留一套令牌顺序（存储管理），旧入口已删，留着用不到的密钥文件只会增加泄露面；
//   - 它是只读令牌，放进存储管理会因为不能写而一直报错；
//   - 服务端仓库目前仍是公开的（1.7.0 及更早的客户站要直读），删掉后官网照样能匿名或用存储令牌更新；
//     改私有前，存储检查里的「改为私有」说明会自动检查现有令牌能不能读它。
//
// 删了文件才通知一次超级管理员，提醒到代码托管站撤销那个令牌。返回是否删过。
func RemoveLegacyOfficialUpdateToken() bool {
	dir := filepath.Join(config.GetDataDir(), "store")
	removed := false
	for _, name := range []string{legacyOfficialUpdateTokenFile, legacyOfficialUpdateTokenFile + ".tmp", legacyOfficialUpdateTokenKeyFile} {
		path := filepath.Join(dir, name)
		if err := os.Remove(path); err == nil {
			removed = true
		} else if !errors.Is(err, os.ErrNotExist) {
			log.Printf("remove legacy official update token %s failed: %v", name, err)
		}
	}
	if removed {
		notifyAllAdmins(notificationTabNotice, "旧的官网更新令牌已删除",
			"1.8.8 起官网更新统一使用存储管理里的令牌，之前在在线更新页单独保存的令牌已从服务器删除。如果它在别处没用，可以到代码托管站撤销。",
			"/source-station/storage-monitor", "official_update_token_removed", "system", "official-update-token")
	}
	return removed
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
