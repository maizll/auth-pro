package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"auto_pro/config"
)

const (
	// 官网自己更新用的仓库。和分发给客户的仓库分开，避免改一个默认值把另一边带偏。
	// 不设环境变量时从 maizll/auth-pro 取。仓库改成私有后，仍用已经保存的令牌。
	officialUpdateRepoEnv           = "AUTO_PRO_OFFICIAL_UPDATE_REPOSITORY"
	officialUpdateDefaultRepository = "maizll/auth-pro"
	officialUpdateUnavailable       = "暂时无法获取更新"
)

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
		return nil, errors.New(officialUpdateUnavailable)
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
		return nil, "", errors.New(officialUpdateUnavailable)
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
		return "", errors.New("下载更新包失败")
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
	return fetchGitHubReleaseAsset(ctx, owner, repo, releaseRef, assetName)
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
