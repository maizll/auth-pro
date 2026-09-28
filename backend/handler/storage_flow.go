package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// 上传先写主存储，失败再按顺序试备用。勾选双写时，主存储成功后再写一个备用。
// 都没有可用位置时，仍暂存在本站，和原来「没配收费仓库」的退路一样。
// 已经有位置但全部失败时不改存本站，避免同一条收费目录出现两种位置。

func putPaidWithLocations(ctx context.Context, kind, id, version string, payload []byte) (string, error) {
	blob, err := loadStorageBlob()
	if err != nil {
		return "", err
	}
	targets := enabledStorageLocations(blob.Locations)
	if len(targets) == 0 {
		return "", errNoEnabledStorage
	}
	var (
		firstRef string
		firstErr error
		used     = map[string]bool{}
	)
	for _, loc := range targets {
		secret, openErr := locationSecret(loc)
		if openErr != nil {
			if firstErr == nil {
				firstErr = openErr
			}
			continue
		}
		ref, putErr := putZipOnLocation(ctx, loc, secret, kind, id, version, payload)
		if putErr != nil {
			if firstErr == nil {
				firstErr = putErr
			}
			continue
		}
		recordStorageCopy(&blob, kind, id, version, loc.ID, ref, payload)
		used[loc.ID] = true
		firstRef = ref
		break
	}
	if firstRef == "" {
		if firstErr != nil {
			return "", firstErr
		}
		return "", errors.New("没有可用的存储位置")
	}
	if blob.DualWrite {
		for _, loc := range targets {
			if used[loc.ID] {
				continue
			}
			secret, openErr := locationSecret(loc)
			if openErr != nil {
				break
			}
			ref, putErr := putZipOnLocation(ctx, loc, secret, kind, id, version, payload)
			if putErr != nil {
				break
			}
			recordStorageCopy(&blob, kind, id, version, loc.ID, ref, payload)
			break
		}
	}
	if err := saveStorageBlob(blob); err != nil {
		return "", err
	}
	return firstRef, nil
}

var errNoEnabledStorage = errors.New("没有启用的存储位置")

func recordStorageCopy(blob *storageConfigBlob, kind, id, version, locationID, ref string, payload []byte) {
	sum := sha256.Sum256(payload)
	copy := storageCopy{
		Kind: kind, ItemID: id, Version: version, LocationID: locationID, Ref: ref,
		SHA256: hex.EncodeToString(sum[:]), Size: int64(len(payload)), UploadedAt: time.Now().UTC(),
	}
	for i := range blob.Copies {
		if blob.Copies[i].Kind == kind && blob.Copies[i].ItemID == id && blob.Copies[i].Version == version && blob.Copies[i].LocationID == locationID {
			blob.Copies[i] = copy
			return
		}
	}
	blob.Copies = append(blob.Copies, copy)
}

func readStoredPackageWithFallback(ctx context.Context, location string) ([]byte, error) {
	payload, err := readStoredPackageDirect(ctx, location)
	if err == nil {
		return payload, nil
	}
	blob, blobErr := loadStorageBlob()
	if blobErr != nil {
		return nil, err
	}
	group := ""
	for _, copy := range blob.Copies {
		if copy.Ref == location {
			group = copy.Kind + "\n" + copy.ItemID + "\n" + copy.Version
			break
		}
	}
	if group == "" {
		return nil, err
	}
	for _, copy := range blob.Copies {
		if copy.Kind+"\n"+copy.ItemID+"\n"+copy.Version != group || copy.Ref == location {
			continue
		}
		payload, nextErr := readStoredPackageDirect(ctx, copy.Ref)
		if nextErr == nil {
			return payload, nil
		}
	}
	return nil, err
}

func readStoredPackageDirect(ctx context.Context, location string) ([]byte, error) {
	location = strings.TrimSpace(location)
	switch {
	case isGitHubPackageRef(location):
		return fetchGitHubPackageBytes(ctx, location)
	case isGiteePackageRef(location), isS3PackageRef(location), isWebDAVPackageRef(location):
		blob, err := loadStorageBlob()
		if err != nil {
			return nil, err
		}
		loc, key, ok := locationByRef(blob.Locations, location)
		if !ok {
			return nil, errCatalogPackageMissing
		}
		secret, err := locationSecret(loc)
		if err != nil {
			return nil, errCatalogPackageUnavailable
		}
		return readLocationObject(ctx, loc, secret, key)
	case isPrivatePackageRef(location):
		return readPrivatePackageFile(location)
	case isStationHostedPackageURL(location):
		name, ok := stationPackageNameFromURL(location)
		if !ok {
			return nil, errCatalogPackageMissing
		}
		path, ok := publicStationPackagePath(name)
		if !ok {
			return nil, errCatalogPackageMissing
		}
		return readCatalogFile(path)
	case strings.HasPrefix(strings.ToLower(location), "https://"):
		payload, err := safeHTTPGet(ctx, location, safeFetchOptions{
			RequireHTTPS: true,
			MaxBytes:     pluginPackageMaxSize,
			Timeout:      2 * time.Minute,
			MaxRedirects: defaultSafeRedirects,
		})
		if err != nil {
			return nil, errCatalogPackageUnavailable
		}
		return payload, nil
	default:
		return nil, errCatalogPackageMissing
	}
}

func tokenForGitHubRepo(owner, repo string) (string, error) {
	blob, err := loadStorageBlob()
	if err == nil {
		for _, loc := range blob.Locations {
			if loc.Kind == packageStorageGitHub && strings.EqualFold(loc.Owner, owner) && strings.EqualFold(loc.Repo, repo) {
				if secret, openErr := locationSecret(loc); openErr == nil && secret != "" {
					return secret, nil
				}
			}
		}
	}
	return loadGitHubPaidToken()
}

// buyerSafeRedirect 只对对象存储的单个文件返回签名地址。
// GitHub 的临时地址在客户端会被当成代码托管站拒绝，Gitee 地址带仓库路径。
// WebDAV 没有可单独失效的签名地址，不能把账号密码编进跳转。这三类都由本站取回、核对校验码后再下发。
func buyerSafeRedirect(ctx context.Context, location string) (string, bool) {
	if !isS3PackageRef(location) {
		return "", false
	}
	blob, err := loadStorageBlob()
	if err != nil {
		return "", false
	}
	loc, key, ok := locationByRef(blob.Locations, location)
	if !ok {
		return "", false
	}
	secret, err := locationSecret(loc)
	if err != nil {
		return "", false
	}
	client, err := s3ClientFor(loc, secret)
	if err != nil {
		return "", false
	}
	// 清单很小。对象明显大于清单时就不是分片描述，直接签名，避免为了判断而把整包拉回服务器。
	_, size, headErr := client.head(ctx, key)
	if headErr != nil {
		return "", false
	}
	if size > 0 && size <= 8192 {
		raw, readErr := client.get(ctx, key, 8192)
		if readErr == nil {
			if _, sharded := parseShardManifest(raw); sharded {
				return "", false
			}
		}
	}
	signed, safe, err := signedLocationURL(ctx, loc, secret, key, 10*time.Minute)
	if err != nil || !safe || signed == "" {
		return "", false
	}
	return signed, true
}
