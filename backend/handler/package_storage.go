package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	packageStorageLocal    = "local"
	packageStorageGitHub   = "github"
	packageStorageExternal = "external"
)

// errLocalPackageNeedsTicket 本地包的下载地址必须带授权票，不能由驱动直接签名。
var errLocalPackageNeedsTicket = errors.New("本站暂存的安装包需要先换下载票")

// StoredObject 是某个驱动里的一个对象。校验码以版本行的 sha256 为准。
type StoredObject struct {
	Key    string
	SHA256 string
	Size   int64
}

// PackageStorage 是安装包存储驱动。1.6.2 的 Gitee / S3 只注册新驱动，不改发布和下载。
type PackageStorage interface {
	Name() string
	Put(ctx context.Context, key string, payload []byte) (storedKey string, err error)
	SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	List(ctx context.Context, prefix string) ([]StoredObject, error)
}

var packageStorages = map[string]PackageStorage{}

func registerPackageStorage(driver PackageStorage) {
	if driver == nil || strings.TrimSpace(driver.Name()) == "" {
		return
	}
	packageStorages[driver.Name()] = driver
}

func packageStorageByName(name string) (PackageStorage, bool) {
	driver, ok := packageStorages[strings.TrimSpace(name)]
	return driver, ok
}

func init() {
	registerPackageStorage(localPackageStorage{})
	registerPackageStorage(githubPackageStorage{})
	registerPackageStorage(externalPackageStorage{})
}

// classifyPackageRef 把库存地址拆成驱动名和对象键。认不出的地址返回空字符串。
func classifyPackageRef(raw string) (driver, key string) {
	raw = strings.TrimSpace(raw)
	if ref, ok := parseGitHubPackageRef(raw); ok {
		return packageStorageGitHub, gitHubObjectKey(ref)
	}
	if name, ok := privatePackageName(raw); ok {
		return packageStorageLocal, name
	}
	if strings.HasPrefix(strings.ToLower(raw), "https://") {
		return packageStorageExternal, raw
	}
	return "", ""
}

func stampReleaseStorage(rel *sourceRelease) {
	if rel == nil {
		return
	}
	rel.StorageDriver, rel.ObjectKey = classifyPackageRef(rel.Location)
}

func gitHubObjectKey(ref gitHubAssetRef) string {
	return ref.Owner + "/" + ref.Repo + "/" + ref.Tag + "/" + ref.Asset
}

func paidStoragePutKey(kind, id, version string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = sourceKindPlugin
	}
	return kind + "/" + strings.TrimSpace(id) + "/" + strings.TrimSpace(version)
}

func parsePaidStoragePutKey(key string) (kind, id, version string, err error) {
	parts := strings.Split(strings.TrimSpace(key), "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", errors.New("存储对象键不合法")
	}
	return parts[0], parts[1], parts[2], nil
}

type localPackageStorage struct {
	dir string
}

func (s localPackageStorage) Name() string { return packageStorageLocal }

func (s localPackageStorage) root() string {
	if strings.TrimSpace(s.dir) != "" {
		_ = os.MkdirAll(s.dir, 0750)
		return s.dir
	}
	return stationPaidPackageDir()
}

func (s localPackageStorage) Put(_ context.Context, key string, payload []byte) (string, error) {
	if !isZipPayload(payload) {
		return "", errors.New("必须上传 ZIP 压缩包")
	}
	name := strings.TrimSpace(key)
	if name == "" {
		generated, err := newPaidPackageName()
		if err != nil {
			return "", err
		}
		name = generated
	}
	if _, ok := privatePackageName(sourcePaidPackagePrefix + name); !ok {
		return "", errors.New("本站对象键不合法")
	}
	dir := s.root()
	finalPath := filepath.Join(dir, name)
	tmp, err := os.CreateTemp(dir, "paid-import-*.zip")
	if err != nil {
		return "", errors.New("保存付费包失败")
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.Write(payload)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tmpName)
		return "", errors.New("保存付费包失败")
	}
	if err := os.Rename(tmpName, finalPath); err != nil {
		_ = os.Remove(tmpName)
		return "", errors.New("保存付费包失败")
	}
	_ = os.Chmod(finalPath, 0640)
	return name, nil
}

func (localPackageStorage) SignedURL(context.Context, string, time.Duration) (string, error) {
	return "", errLocalPackageNeedsTicket
}

func (s localPackageStorage) Delete(_ context.Context, key string) error {
	name, ok := privatePackageName(sourcePaidPackagePrefix + strings.TrimSpace(key))
	if !ok {
		return errors.New("本站对象键不合法")
	}
	path := filepath.Join(s.root(), name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return errors.New("删除本站安装包失败")
	}
	return nil
}

func (s localPackageStorage) Exists(_ context.Context, key string) (bool, error) {
	name, ok := privatePackageName(sourcePaidPackagePrefix + strings.TrimSpace(key))
	if !ok {
		return false, nil
	}
	info, err := os.Stat(filepath.Join(s.root(), name))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular(), nil
}

func (s localPackageStorage) List(_ context.Context, prefix string) ([]StoredObject, error) {
	entries, err := os.ReadDir(s.root())
	if err != nil {
		return nil, errors.New("读取本站安装包目录失败")
	}
	out := make([]StoredObject, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		if _, ok := privatePackageName(sourcePaidPackagePrefix + entry.Name()); !ok {
			continue
		}
		info, infoErr := entry.Info()
		var size int64
		if infoErr == nil {
			size = info.Size()
		}
		out = append(out, StoredObject{Key: entry.Name(), Size: size})
	}
	return out, nil
}

type githubPackageStorage struct{}

func (githubPackageStorage) Name() string { return packageStorageGitHub }

func (githubPackageStorage) Put(ctx context.Context, key string, payload []byte) (string, error) {
	kind, id, version, err := parsePaidStoragePutKey(key)
	if err != nil {
		return "", err
	}
	ref, err := uploadPaidZipToStationRepo(ctx, kind, id, version, payload)
	if err != nil {
		return "", err
	}
	parsed, ok := parseGitHubPackageRef(ref)
	if !ok {
		return "", errors.New("收费仓库里的标签或文件名不合法")
	}
	return gitHubObjectKey(parsed), nil
}

func (githubPackageStorage) SignedURL(ctx context.Context, key string, _ time.Duration) (string, error) {
	parsed, ok := parseGitHubPackageRef(githubPackagePrefix + strings.TrimSpace(key))
	if !ok {
		return "", errors.New("付费包不存在")
	}
	return authorizeGitHubBuyerURL(ctx, true, formatGitHubPackageRef(parsed))
}

func (githubPackageStorage) Delete(ctx context.Context, key string) error {
	parsed, ok := parseGitHubPackageRef(githubPackagePrefix + strings.TrimSpace(key))
	if !ok {
		return errors.New("付费包不存在")
	}
	token, err := loadGitHubPaidToken()
	if err != nil {
		return err
	}
	release, err := githubReleaseByTag(ctx, token, parsed)
	if err != nil {
		if errors.Is(err, errGitHubPaidAssetMissing) {
			return nil
		}
		return err
	}
	asset, err := githubAssetByName(release, parsed.Asset)
	if err != nil {
		if errors.Is(err, errGitHubPaidAssetMissing) {
			return nil
		}
		return err
	}
	rawURL := strings.TrimRight(sourceGitHubAPIBase, "/") + "/repos/" + url.PathEscape(parsed.Owner) + "/" + url.PathEscape(parsed.Repo) + "/releases/assets/" + fmt.Sprintf("%d", asset.ID)
	status, _, err := githubPaidJSON(ctx, http.MethodDelete, rawURL, token, nil)
	if err != nil {
		return errors.New("删除收费仓库附件失败")
	}
	if status == http.StatusNotFound || status == http.StatusNoContent || (status >= 200 && status < 300) {
		return nil
	}
	return errors.New("删除收费仓库附件失败")
}

func (githubPackageStorage) Exists(ctx context.Context, key string) (bool, error) {
	location := githubPackagePrefix + strings.TrimSpace(key)
	if _, ok := parseGitHubPackageRef(location); !ok {
		return false, nil
	}
	err := probeGitHubPaidAsset(ctx, location)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, errGitHubPaidAssetMissing) {
		return false, nil
	}
	return false, err
}

func (githubPackageStorage) List(ctx context.Context, prefix string) ([]StoredObject, error) {
	owner, repo, token, err := loadGitHubPaidRepo()
	if err != nil {
		return nil, err
	}
	if owner == "" || repo == "" || token == "" {
		return nil, errGitHubPaidTokenMissing
	}
	rawURL := strings.TrimRight(sourceGitHubAPIBase, "/") + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/releases?per_page=30"
	status, payload, err := githubPaidJSON(ctx, http.MethodGet, rawURL, token, nil)
	if err != nil {
		return nil, errors.New("无法连接 GitHub，请稍后再试")
	}
	if status == http.StatusUnauthorized {
		return nil, errGitHubPaidTokenInvalid
	}
	if status < 200 || status >= 300 {
		return nil, errors.New("读取收费仓库列表失败")
	}
	var releases []gitHubReleaseDTO
	if err := json.Unmarshal(payload, &releases); err != nil {
		return nil, errors.New("读取收费仓库列表失败")
	}
	prefix = strings.TrimSpace(prefix)
	out := make([]StoredObject, 0)
	for _, release := range releases {
		for _, asset := range release.Assets {
			ref := gitHubAssetRef{Owner: owner, Repo: repo, Tag: release.TagName, Asset: asset.Name}
			if !validGitHubAssetRef(ref) {
				continue
			}
			key := gitHubObjectKey(ref)
			if prefix != "" && !strings.HasPrefix(release.TagName, prefix) && !strings.HasPrefix(asset.Name, prefix) && !strings.HasPrefix(key, prefix) {
				continue
			}
			out = append(out, StoredObject{Key: key})
		}
	}
	return out, nil
}

type externalPackageStorage struct{}

func (externalPackageStorage) Name() string { return packageStorageExternal }

func (externalPackageStorage) Put(context.Context, string, []byte) (string, error) {
	return "", errors.New("公开地址不支持上传")
}

func (externalPackageStorage) SignedURL(_ context.Context, key string, _ time.Duration) (string, error) {
	key = strings.TrimSpace(key)
	if !strings.HasPrefix(strings.ToLower(key), "https://") {
		return "", errors.New("公开地址不合法")
	}
	return key, nil
}

func (externalPackageStorage) Delete(context.Context, string) error {
	return errors.New("公开地址不支持删除")
}

func (externalPackageStorage) Exists(_ context.Context, key string) (bool, error) {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(key)), "https://"), nil
}

func (externalPackageStorage) List(context.Context, string) ([]StoredObject, error) {
	return []StoredObject{}, nil
}
