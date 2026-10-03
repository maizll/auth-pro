package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// 读取、列举、删除和短时地址。分片清单会在读取时拼回原文件并核对校验码。
// GitHub 的短时地址在 release-assets.githubusercontent.com 上，不含仓库路径，可以交给管理员下载。
// Gitee 的附件地址自带仓库路径，只在服务端使用，不返回给客户。

type storageObjectInfo struct {
	Key     string
	Name    string
	Size    int64
	Updated time.Time
	// Tag 只有 GitHub、Gitee 有：文件所在的发布标签，页面据此分组。
	Tag string
}

// gitReleaseListPages 列仓库发布最多翻几页（每页 100 个）。
const gitReleaseListPages = 10

// gitReleaseAsset 是 GitHub、Gitee 发布附件里分组要用的字段。
type gitReleaseAsset struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updated_at"`
}

type gitReleaseListing struct {
	TagName string            `json:"tag_name"`
	Assets  []gitReleaseAsset `json:"assets"`
}

func gitReleaseObjects(loc storageLocation, releases []gitReleaseListing) []storageObjectInfo {
	out := make([]storageObjectInfo, 0)
	for _, release := range releases {
		for _, asset := range release.Assets {
			key := loc.Owner + "/" + loc.Repo + "/" + release.TagName + "/" + asset.Name
			out = append(out, storageObjectInfo{Key: key, Name: asset.Name, Size: asset.Size, Updated: asset.UpdatedAt, Tag: release.TagName})
		}
	}
	return out
}

func readLocationObject(ctx context.Context, loc storageLocation, secret, key string) ([]byte, error) {
	payload, err := readLocationObjectRaw(ctx, loc, secret, key)
	if err != nil {
		return nil, err
	}
	manifest, ok := parseShardManifest(payload)
	if !ok {
		return payload, nil
	}
	if manifest.Size > storageReadLimit {
		return nil, errors.New("分片文件过大，无法在服务器上拼合")
	}
	parts := make([][]byte, 0, len(manifest.Parts))
	for _, name := range manifest.Parts {
		part, partErr := readLocationObjectRaw(ctx, loc, secret, siblingObjectKey(loc, key, name))
		if partErr != nil {
			return nil, errors.New("有分片读取失败，安装包不完整")
		}
		parts = append(parts, part)
	}
	return joinShards(parts, manifest.SHA256, manifest.Size)
}

func siblingObjectKey(loc storageLocation, currentKey, partName string) string {
	partName = strings.TrimLeft(partName, "/")
	switch loc.Kind {
	case packageStorageS3, packageStorageWebDAV:
		prefix := strings.Trim(loc.KeyPrefix, "/")
		if prefix != "" && !strings.HasPrefix(partName, prefix+"/") {
			return prefix + "/" + partName
		}
		return partName
	case packageStorageGitHub, packageStorageGitee:
		// 清单里只记文件名。分片和清单在同一个标签下，键要保留所有者和标签。
		parts := strings.Split(strings.Trim(currentKey, "/"), "/")
		if len(parts) >= 2 {
			parts[len(parts)-1] = partName
			return strings.Join(parts, "/")
		}
		return partName
	default:
		return partName
	}
}

func readLocationObjectRaw(ctx context.Context, loc storageLocation, secret, key string) ([]byte, error) {
	switch loc.Kind {
	case packageStorageGitHub:
		ref, ok := parseGitHubPackageRef(githubPackagePrefix + strings.TrimPrefix(strings.TrimSpace(key), githubPackagePrefix))
		if !ok {
			ref, ok = parseGitHubPackageRef(key)
		}
		if !ok {
			// key 可能只是 tag/asset 或完整 owner/repo/tag/asset。
			ref = gitHubAssetRef{Owner: loc.Owner, Repo: loc.Repo}
			parts := strings.Split(strings.Trim(key, "/"), "/")
			if len(parts) == 2 {
				ref.Tag, ref.Asset = parts[0], parts[1]
			} else if len(parts) == 4 {
				ref = gitHubAssetRef{Owner: parts[0], Repo: parts[1], Tag: parts[2], Asset: parts[3]}
			} else {
				return nil, errors.New("对象键不合法")
			}
			if !validGitHubAssetRef(ref) {
				return nil, errors.New("对象键不合法")
			}
		}
		return fetchGitHubAssetBytes(ctx, secret, ref)
	case packageStorageGitee:
		ref, ok := parseGiteePackageRef(key)
		if !ok {
			parts := strings.Split(strings.Trim(key, "/"), "/")
			if len(parts) == 2 {
				ref = gitHubAssetRef{Owner: loc.Owner, Repo: loc.Repo, Tag: parts[0], Asset: parts[1]}
				ok = validGitHubAssetRef(ref)
			} else if len(parts) == 4 {
				ref = gitHubAssetRef{Owner: parts[0], Repo: parts[1], Tag: parts[2], Asset: parts[3]}
				ok = validGitHubAssetRef(ref)
			}
		}
		if !ok {
			return nil, errors.New("对象键不合法")
		}
		return fetchGiteeAssetBytes(ctx, secret, ref)
	case packageStorageS3:
		client, err := s3ClientFor(loc, secret)
		if err != nil {
			return nil, err
		}
		return client.get(ctx, key, storageReadLimit)
	case packageStorageWebDAV:
		client, err := webdavClientFor(loc, secret)
		if err != nil {
			return nil, err
		}
		return client.get(ctx, key)
	default:
		return nil, errors.New("不支持的存储类型")
	}
}

func fetchGitHubAssetBytes(ctx context.Context, token string, ref gitHubAssetRef) ([]byte, error) {
	release, err := githubReleaseByTag(ctx, token, ref)
	if err != nil {
		return nil, err
	}
	asset, err := githubAssetByName(release, ref.Asset)
	if err != nil {
		return nil, err
	}
	rawURL := strings.TrimRight(sourceGitHubAPIBase, "/") + "/repos/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Repo) + "/releases/assets/" + fmt.Sprintf("%d", asset.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errors.New("无法读取安装包")
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	client := &http.Client{
		Timeout: 2 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || req == nil || req.URL == nil {
				return errors.New("无法读取安装包")
			}
			if err := validateGitHubBuyerRedirect(req.URL.String(), false); err != nil {
				return err
			}
			req.Header.Del("Authorization")
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("无法连接 GitHub，请稍后再试")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("GitHub 上找不到该安装包")
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, storageReadLimit+1))
	if err != nil || int64(len(payload)) > storageReadLimit || len(payload) == 0 {
		return nil, errors.New("安装包无法读取")
	}
	return payload, nil
}

func fetchGiteeAssetBytes(ctx context.Context, token string, ref gitHubAssetRef) ([]byte, error) {
	api := strings.TrimRight(sourceGiteeAPIBase, "/")
	rawURL := api + "/repos/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Repo) + "/releases/tags/" + url.PathEscape(ref.Tag) + "?access_token=" + url.QueryEscape(token)
	status, payload, err := sourceReleaseJSON(ctx, http.MethodGet, rawURL, nil, nil)
	if err != nil {
		return nil, errors.New("无法连接 Gitee，请稍后再试")
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil, errors.New("Gitee 令牌无效，或没有该仓库的读取权限")
	}
	var release struct {
		Assets []struct {
			Name        string `json:"name"`
			DownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	_ = json.Unmarshal(payload, &release)
	download := ""
	for _, asset := range release.Assets {
		if asset.Name == ref.Asset {
			download = asset.DownloadURL
			break
		}
	}
	if download == "" {
		return nil, errors.New("Gitee 上找不到该安装包")
	}
	if !strings.Contains(download, "access_token=") {
		if strings.Contains(download, "?") {
			download += "&access_token=" + url.QueryEscape(token)
		} else {
			download += "?access_token=" + url.QueryEscape(token)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, download, nil)
	if err != nil {
		return nil, errors.New("无法读取安装包")
	}
	resp, err := sourceReleaseHTTPClient.Do(req)
	if err != nil {
		return nil, errors.New("无法连接 Gitee，请稍后再试")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, storageReadLimit+1))
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 || int64(len(body)) > storageReadLimit || len(body) == 0 {
		return nil, errors.New("Gitee 上的安装包无法读取")
	}
	return body, nil
}

func listLocationObjects(ctx context.Context, loc storageLocation, secret string) ([]storageObjectInfo, error) {
	switch loc.Kind {
	case packageStorageGitHub:
		return listGitHubObjects(ctx, loc, secret)
	case packageStorageGitee:
		return listGiteeObjects(ctx, loc, secret)
	case packageStorageWebDAV:
		client, err := webdavClientFor(loc, secret)
		if err != nil {
			return nil, err
		}
		listed, err := client.listAll(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]storageObjectInfo, 0, len(listed))
		for _, item := range listed {
			if item.IsDir || item.Key == "" {
				continue
			}
			name := item.Key
			if slash := strings.LastIndex(name, "/"); slash >= 0 {
				name = name[slash+1:]
			}
			out = append(out, storageObjectInfo{Key: item.Key, Name: name, Size: item.Size, Updated: item.Updated})
		}
		return out, nil
	case packageStorageS3:
		client, err := s3ClientFor(loc, secret)
		if err != nil {
			return nil, err
		}
		listed, err := client.list(ctx, strings.Trim(loc.KeyPrefix, "/"), 100)
		if err != nil {
			return nil, err
		}
		out := make([]storageObjectInfo, 0, len(listed))
		for _, item := range listed {
			name := item.Key
			if slash := strings.LastIndex(name, "/"); slash >= 0 {
				name = name[slash+1:]
			}
			out = append(out, storageObjectInfo{Key: item.Key, Name: name, Size: item.Size, Updated: item.LastModified})
		}
		return out, nil
	default:
		return nil, errors.New("不支持的存储类型")
	}
}

func listGitHubObjects(ctx context.Context, loc storageLocation, secret string) ([]storageObjectInfo, error) {
	all := make([]gitReleaseListing, 0)
	for page := 1; page <= gitReleaseListPages; page++ {
		rawURL := strings.TrimRight(sourceGitHubAPIBase, "/") + "/repos/" + url.PathEscape(loc.Owner) + "/" + url.PathEscape(loc.Repo) + "/releases?per_page=100&page=" + strconv.Itoa(page)
		status, payload, err := githubPaidJSON(ctx, http.MethodGet, rawURL, secret, nil)
		if err != nil {
			return nil, errors.New("无法连接 GitHub，请稍后再试")
		}
		if status == http.StatusUnauthorized {
			return nil, errors.New("GitHub 令牌无效或已过期")
		}
		if status == http.StatusNotFound {
			return nil, errors.New("找不到该 GitHub 仓库，请核对所有者和仓库名")
		}
		if status == http.StatusForbidden {
			return nil, errors.New("GitHub 令牌没有读取该仓库的权限")
		}
		if status < 200 || status >= 300 {
			return nil, errors.New("读取 GitHub 仓库文件失败")
		}
		var releases []gitReleaseListing
		if err := json.Unmarshal(payload, &releases); err != nil {
			return nil, errors.New("读取 GitHub 仓库文件失败")
		}
		all = append(all, releases...)
		if len(releases) < 100 {
			break
		}
	}
	return gitReleaseObjects(loc, all), nil
}

func listGiteeObjects(ctx context.Context, loc storageLocation, secret string) ([]storageObjectInfo, error) {
	all := make([]gitReleaseListing, 0)
	for page := 1; page <= gitReleaseListPages; page++ {
		rawURL := strings.TrimRight(sourceGiteeAPIBase, "/") + "/repos/" + url.PathEscape(loc.Owner) + "/" + url.PathEscape(loc.Repo) + "/releases?page=" + strconv.Itoa(page) + "&per_page=100&access_token=" + url.QueryEscape(secret)
		status, payload, err := sourceReleaseJSON(ctx, http.MethodGet, rawURL, nil, nil)
		if err != nil {
			return nil, errors.New("无法连接 Gitee，请稍后再试")
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return nil, errors.New("Gitee 令牌无效，或没有该仓库的读取权限")
		}
		if status == http.StatusNotFound {
			return nil, errors.New("找不到该 Gitee 仓库，请核对所有者和仓库名")
		}
		if status < 200 || status >= 300 {
			return nil, errors.New("读取 Gitee 仓库文件失败")
		}
		var releases []gitReleaseListing
		if err := json.Unmarshal(payload, &releases); err != nil {
			return nil, errors.New("读取 Gitee 仓库文件失败")
		}
		all = append(all, releases...)
		if len(releases) < 100 {
			break
		}
	}
	return gitReleaseObjects(loc, all), nil
}

func deleteLocationObject(ctx context.Context, loc storageLocation, secret, key string) error {
	switch loc.Kind {
	case packageStorageGitHub:
		ref, err := gitRefFromLocationKey(loc, key)
		if err != nil {
			return err
		}
		return deleteGitHubAsset(ctx, secret, ref)
	case packageStorageGitee:
		ref, err := gitRefFromLocationKey(loc, key)
		if err != nil {
			return err
		}
		return deleteGiteeAsset(ctx, secret, ref)
	case packageStorageS3:
		client, err := s3ClientFor(loc, secret)
		if err != nil {
			return err
		}
		return client.delete(ctx, key)
	case packageStorageWebDAV:
		client, err := webdavClientFor(loc, secret)
		if err != nil {
			return err
		}
		return client.delete(ctx, key)
	default:
		return errors.New("不支持的存储类型")
	}
}

func gitRefFromLocationKey(loc storageLocation, key string) (gitHubAssetRef, error) {
	key = strings.TrimSpace(key)
	if ref, ok := parseGitHubPackageRef(key); ok {
		return ref, nil
	}
	if ref, ok := parseGiteePackageRef(key); ok {
		return ref, nil
	}
	if ref, ok := parseGitHubPackageRef(githubPackagePrefix + key); ok {
		return ref, nil
	}
	parts := strings.Split(strings.Trim(key, "/"), "/")
	ref := gitHubAssetRef{Owner: loc.Owner, Repo: loc.Repo}
	if len(parts) == 2 {
		ref.Tag, ref.Asset = parts[0], parts[1]
	} else if len(parts) == 4 {
		ref = gitHubAssetRef{Owner: parts[0], Repo: parts[1], Tag: parts[2], Asset: parts[3]}
	} else {
		return gitHubAssetRef{}, errors.New("对象键不合法")
	}
	if !validGitHubAssetRef(ref) {
		return gitHubAssetRef{}, errors.New("对象键不合法")
	}
	return ref, nil
}

func deleteGitHubAsset(ctx context.Context, token string, ref gitHubAssetRef) error {
	release, err := githubReleaseByTag(ctx, token, ref)
	if err != nil {
		if errors.Is(err, errGitHubPaidAssetMissing) {
			return nil
		}
		return err
	}
	asset, err := githubAssetByName(release, ref.Asset)
	if err != nil {
		if errors.Is(err, errGitHubPaidAssetMissing) {
			return nil
		}
		return err
	}
	rawURL := strings.TrimRight(sourceGitHubAPIBase, "/") + "/repos/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Repo) + "/releases/assets/" + fmt.Sprintf("%d", asset.ID)
	status, _, err := githubPaidJSON(ctx, http.MethodDelete, rawURL, token, nil)
	if err != nil {
		return errors.New("删除 GitHub 附件失败")
	}
	if status == http.StatusNotFound || status == http.StatusNoContent || (status >= 200 && status < 300) {
		return nil
	}
	return errors.New("删除 GitHub 附件失败")
}

func deleteGiteeAsset(ctx context.Context, token string, ref gitHubAssetRef) error {
	api := strings.TrimRight(sourceGiteeAPIBase, "/")
	rawURL := api + "/repos/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Repo) + "/releases/tags/" + url.PathEscape(ref.Tag) + "?access_token=" + url.QueryEscape(token)
	status, payload, err := sourceReleaseJSON(ctx, http.MethodGet, rawURL, nil, nil)
	if err != nil {
		return errors.New("无法连接 Gitee，请稍后再试")
	}
	if status == http.StatusNotFound {
		return nil
	}
	var release struct {
		ID     int64 `json:"id"`
		Assets []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"assets"`
	}
	_ = json.Unmarshal(payload, &release)
	var fileID int64
	for _, asset := range release.Assets {
		if asset.Name == ref.Asset {
			fileID = asset.ID
			break
		}
	}
	if fileID == 0 {
		return nil
	}
	delURL := fmt.Sprintf("%s/repos/%s/%s/releases/%d/attach_files/%d?access_token=%s", api, url.PathEscape(ref.Owner), url.PathEscape(ref.Repo), release.ID, fileID, url.QueryEscape(token))
	status, _, err = sourceReleaseJSON(ctx, http.MethodDelete, delURL, nil, nil)
	if err != nil {
		return errors.New("删除 Gitee 附件失败")
	}
	if status == http.StatusNotFound || status == http.StatusNoContent || (status >= 200 && status < 300) {
		return nil
	}
	return errors.New("删除 Gitee 附件失败")
}

// signedLocationURL 返回短时地址。Gitee 地址含仓库路径，ok 为 false，调用方改走本站转发。
func signedLocationURL(ctx context.Context, loc storageLocation, secret, key string, ttl time.Duration) (string, bool, error) {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	switch loc.Kind {
	case packageStorageWebDAV:
		// 网盘没有可交给买家的签名地址。返回不安全，调用方改用官网票据，由本站带口令去取。
		return "", false, nil
	case packageStorageS3:
		client, err := s3ClientFor(loc, secret)
		if err != nil {
			return "", false, err
		}
		raw, err := client.presign(key, ttl)
		if err != nil {
			return "", false, err
		}
		return raw, true, nil
	case packageStorageGitHub:
		ref, err := gitRefFromLocationKey(loc, key)
		if err != nil {
			return "", false, err
		}
		release, err := githubReleaseByTag(ctx, secret, ref)
		if err != nil {
			return "", false, err
		}
		asset, err := githubAssetByName(release, ref.Asset)
		if err != nil {
			return "", false, err
		}
		raw, err := githubAssetTemporaryURL(ctx, secret, ref, asset.ID, true)
		if err != nil {
			return "", false, err
		}
		if strings.Contains(raw, "/"+ref.Owner+"/") || strings.Contains(raw, secret) {
			return "", false, nil
		}
		return raw, true, nil
	default:
		return "", false, nil
	}
}

func objectExists(ctx context.Context, loc storageLocation, secret, key string) (bool, error) {
	switch loc.Kind {
	case packageStorageWebDAV:
		client, err := webdavClientFor(loc, secret)
		if err != nil {
			return false, err
		}
		entries, err := client.propfind(ctx, key, 0)
		if err != nil {
			if strings.Contains(err.Error(), "找不到") {
				return false, nil
			}
			return false, err
		}
		for _, entry := range entries {
			if entry.Key == strings.Trim(key, "/") && !entry.IsDir {
				return true, nil
			}
		}
		if len(entries) == 1 && !entries[0].IsDir {
			return true, nil
		}
		return false, nil
	case packageStorageS3:
		client, err := s3ClientFor(loc, secret)
		if err != nil {
			return false, err
		}
		ok, _, err := client.head(ctx, key)
		return ok, err
	default:
		_, err := readLocationObjectRaw(ctx, loc, secret, key)
		if err == nil {
			return true, nil
		}
		if errors.Is(err, errGitHubPaidAssetMissing) || strings.Contains(err.Error(), "找不到") {
			return false, nil
		}
		return false, err
	}
}

func probeStorageLocation(ctx context.Context, loc storageLocation, secret string) error {
	switch loc.Kind {
	case packageStorageGitHub:
		return probeGitRepo(ctx, loc, secret, true)
	case packageStorageGitee:
		return probeGitRepo(ctx, loc, secret, false)
	case packageStorageWebDAV:
		return probeWebDAV(ctx, loc, secret)
	case packageStorageS3:
		client, err := s3ClientFor(loc, secret)
		if err != nil {
			return err
		}
		if _, err := client.list(ctx, strings.Trim(loc.KeyPrefix, "/"), 1); err != nil {
			return err
		}
		probeKey := joinStoragePrefix(loc.KeyPrefix, ".auth-pro-connection-test")
		if err := client.put(ctx, probeKey, []byte("ok")); err != nil {
			return errors.New("可以读取，但不能写入。请给密钥加上该桶的写权限。")
		}
		if err := client.delete(ctx, probeKey); err != nil {
			return errors.New("写入测试文件后无法删除，请检查删除权限。")
		}
		return nil
	default:
		return errors.New("不支持的存储类型")
	}
}

func probeGitRepo(ctx context.Context, loc storageLocation, secret string, github bool) error {
	if !sourceReleaseRepoPattern.MatchString(loc.Owner) || !sourceReleaseRepoPattern.MatchString(loc.Repo) {
		return errors.New("所有者或仓库名不合法")
	}
	if github {
		status, _, err := githubPaidJSON(ctx, http.MethodGet, githubPaidAPI("/user"), secret, nil)
		var payload []byte
		if err != nil {
			return errors.New("无法连接 GitHub，请稍后再试")
		}
		if status == http.StatusUnauthorized {
			return errors.New("GitHub 令牌无效或已过期")
		}
		if status < 200 || status >= 300 {
			return errors.New("GitHub 令牌无法使用，请确认 Contents 为读写")
		}
		rawURL := githubPaidAPI("/repos/" + url.PathEscape(loc.Owner) + "/" + url.PathEscape(loc.Repo))
		status, payload, err = githubPaidJSON(ctx, http.MethodGet, rawURL, secret, nil)
		if err != nil {
			return errors.New("无法连接 GitHub，请稍后再试")
		}
		if status == http.StatusNotFound {
			return errors.New("找不到该 GitHub 仓库。可在 GitHub 上先建成私有仓库，再回来测试。")
		}
		if status == http.StatusForbidden || status == http.StatusUnauthorized {
			return errors.New("GitHub 令牌没有该仓库的权限。细粒度令牌需要 Contents 读写，经典令牌需要 repo 权限。")
		}
		var repo struct {
			Private     bool `json:"private"`
			Permissions struct {
				Push bool `json:"push"`
			} `json:"permissions"`
		}
		_ = json.Unmarshal(payload, &repo)
		if !repo.Private {
			return &storageRepoPublicError{kind: packageStorageGitHub, owner: loc.Owner, repo: loc.Repo, text: githubPaidPublicRepoText}
		}
		if !repo.Permissions.Push {
			return errors.New("令牌可以读取该仓库，但没有写入权限。上传安装包需要 Contents 读写。")
		}
		return nil
	}
	rawURL := strings.TrimRight(sourceGiteeAPIBase, "/") + "/repos/" + url.PathEscape(loc.Owner) + "/" + url.PathEscape(loc.Repo) + "?access_token=" + url.QueryEscape(secret)
	status, payload, err := sourceReleaseJSON(ctx, http.MethodGet, rawURL, nil, nil)
	if err != nil {
		return errors.New("无法连接 Gitee，请稍后再试")
	}
	if status == http.StatusUnauthorized {
		return errors.New("Gitee 令牌无效或已过期")
	}
	if status == http.StatusNotFound {
		return errors.New("找不到该 Gitee 仓库，请核对所有者和仓库名")
	}
	if status == http.StatusForbidden {
		return errors.New("Gitee 令牌没有该仓库的权限，需要仓库和发行版的读写。")
	}
	if status < 200 || status >= 300 {
		return errors.New("Gitee 仓库暂时无法访问")
	}
	var repo struct {
		Private    bool `json:"private"`
		Permission struct {
			Push bool `json:"push"`
		} `json:"permission"`
	}
	_ = json.Unmarshal(payload, &repo)
	if !repo.Private {
		return &storageRepoPublicError{kind: packageStorageGitee, owner: loc.Owner, repo: loc.Repo, text: "该 Gitee 仓库是公开的，不能存放收费安装包。请改为私有仓库。"}
	}
	if !repo.Permission.Push {
		return errors.New("令牌可以读取该 Gitee 仓库，但没有写入权限。")
	}
	return nil
}

func findStorageLocation(list []storageLocation, id string) (storageLocation, bool) {
	id = strings.TrimSpace(id)
	for _, loc := range list {
		if loc.ID == id {
			return loc, true
		}
	}
	return storageLocation{}, false
}

func locationByRef(list []storageLocation, ref string) (storageLocation, string, bool) {
	if parsed, ok := parseGitHubPackageRef(ref); ok {
		for _, loc := range list {
			if loc.Kind == packageStorageGitHub && strings.EqualFold(loc.Owner, parsed.Owner) && strings.EqualFold(loc.Repo, parsed.Repo) {
				return loc, parsed.Owner + "/" + parsed.Repo + "/" + parsed.Tag + "/" + parsed.Asset, true
			}
		}
	}
	if parsed, ok := parseGiteePackageRef(ref); ok {
		for _, loc := range list {
			if loc.Kind == packageStorageGitee && strings.EqualFold(loc.Owner, parsed.Owner) && strings.EqualFold(loc.Repo, parsed.Repo) {
				return loc, parsed.Owner + "/" + parsed.Repo + "/" + parsed.Tag + "/" + parsed.Asset, true
			}
		}
	}
	if id, key, ok := parseS3PackageRef(ref); ok {
		if loc, found := findStorageLocation(list, id); found {
			return loc, key, true
		}
	}
	if id, key, ok := parseWebDAVPackageRef(ref); ok {
		if loc, found := findStorageLocation(list, id); found {
			return loc, key, true
		}
	}
	return storageLocation{}, "", false
}
