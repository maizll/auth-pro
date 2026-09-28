package handler

import (
	"strings"
)

const (
	giteePackagePrefix  = "gitee:"
	s3PackagePrefix     = "s3:"
	webdavPackagePrefix = "webdav:"
)

func formatGiteePackageRef(ref gitHubAssetRef) string {
	return giteePackagePrefix + ref.Owner + "/" + ref.Repo + "/" + ref.Tag + "/" + ref.Asset
}

func parseGiteePackageRef(raw string) (gitHubAssetRef, bool) {
	value := strings.TrimSpace(raw)
	if !strings.HasPrefix(value, giteePackagePrefix) {
		return gitHubAssetRef{}, false
	}
	return parseGitHubPackageRef(githubPackagePrefix + strings.TrimPrefix(value, giteePackagePrefix))
}

func isGiteePackageRef(raw string) bool {
	_, ok := parseGiteePackageRef(raw)
	return ok
}

func formatS3PackageRef(locationID, key string) string {
	return s3PackagePrefix + strings.TrimSpace(locationID) + "/" + strings.TrimLeft(key, "/")
}

func parseS3PackageRef(raw string) (locationID, key string, ok bool) {
	value := strings.TrimSpace(raw)
	if !strings.HasPrefix(value, s3PackagePrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(value, s3PackagePrefix)
	id, objectKey, found := strings.Cut(rest, "/")
	if !found || id == "" || objectKey == "" || strings.Contains(objectKey, "..") {
		return "", "", false
	}
	return id, objectKey, true
}

func isS3PackageRef(raw string) bool {
	_, _, ok := parseS3PackageRef(raw)
	return ok
}

func formatWebDAVPackageRef(locationID, key string) string {
	return webdavPackagePrefix + strings.TrimSpace(locationID) + "/" + strings.TrimLeft(key, "/")
}

func parseWebDAVPackageRef(raw string) (locationID, key string, ok bool) {
	value := strings.TrimSpace(raw)
	if !strings.HasPrefix(value, webdavPackagePrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(value, webdavPackagePrefix)
	id, objectKey, found := strings.Cut(rest, "/")
	if !found || id == "" || objectKey == "" || strings.Contains(objectKey, "..") {
		return "", "", false
	}
	return id, objectKey, true
}

func isWebDAVPackageRef(raw string) bool {
	_, _, ok := parseWebDAVPackageRef(raw)
	return ok
}

// isManagedPackageRef 表示安装包在本站掌管的存储里，不能把内部地址交给客户。
func isManagedPackageRef(raw string) bool {
	return isPrivatePackageRef(raw) || isRemoteManagedRef(raw)
}

func isRemoteManagedRef(raw string) bool {
	return isGitHubPackageRef(raw) || isGiteePackageRef(raw) || isS3PackageRef(raw) || isWebDAVPackageRef(raw)
}

func classifyManagedRef(raw string) (driver, key string) {
	if ref, ok := parseGiteePackageRef(raw); ok {
		return packageStorageGitee, ref.Owner + "/" + ref.Repo + "/" + ref.Tag + "/" + ref.Asset
	}
	if id, objectKey, ok := parseS3PackageRef(raw); ok {
		return packageStorageS3, id + "/" + objectKey
	}
	if id, objectKey, ok := parseWebDAVPackageRef(raw); ok {
		return packageStorageWebDAV, id + "/" + objectKey
	}
	return "", ""
}
