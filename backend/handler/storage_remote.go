package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

// 按某一个存储位置上传、列举、删除和签发短时地址。
// GitHub / Gitee 复用原来的 Release 接口；对象存储走 S3 签名。密钥由调用方解开后传入，不写日志。

const storageReadLimit int64 = 512 << 20

func locationSecret(loc storageLocation) (string, error) {
	return openStorageSecret(loc.SecretSealed)
}

func s3ClientFor(loc storageLocation, secret string) (s3Client, error) {
	if strings.TrimSpace(loc.Endpoint) == "" || strings.TrimSpace(loc.Region) == "" || strings.TrimSpace(loc.Bucket) == "" || strings.TrimSpace(loc.AccessKey) == "" || strings.TrimSpace(secret) == "" {
		return s3Client{}, errors.New("对象存储要填写 Endpoint、地域、Bucket、AccessKey 和 Secret")
	}
	return s3Client{
		Endpoint: loc.Endpoint, Region: loc.Region, Bucket: loc.Bucket,
		AccessKey: loc.AccessKey, Secret: secret, PathStyle: loc.PathStyle,
	}, nil
}

func gitSettings(loc storageLocation, secret string) (sourceReleaseSettings, error) {
	if !sourceReleaseRepoPattern.MatchString(loc.Owner) || !sourceReleaseRepoPattern.MatchString(loc.Repo) || strings.TrimSpace(secret) == "" {
		return sourceReleaseSettings{}, errors.New("请填写所有者和仓库，并保存令牌")
	}
	settings := normalizeReleaseSettings(sourceReleaseSettings{
		Provider: loc.Kind, Owner: loc.Owner, Repo: loc.Repo, Token: secret, Branch: loc.Branch,
		TagStrategy: "paid-{kind}-{id}-{version}",
	})
	return settings, nil
}

func paidAssetBase(kind, id, version string) string {
	manifest := sourcePackageManifest{ID: id, Version: version, Kind: kind, Filename: id + "-" + version + ".zip"}
	return sourceReleaseAssetName(manifest)
}

func paidAssetTag(kind, id, version string) string {
	return renderSourceReleaseTag("paid-{kind}-{id}-{version}", id, version, kind)
}

func putZipOnLocation(ctx context.Context, loc storageLocation, secret, kind, id, version string, payload []byte) (string, error) {
	if int64(len(payload)) > storagePartLimit(loc.Kind) {
		return putShardedZip(ctx, loc, secret, kind, id, version, payload)
	}
	return putSingleZip(ctx, loc, secret, kind, id, version, payload)
}

func putSingleZip(ctx context.Context, loc storageLocation, secret, kind, id, version string, payload []byte) (string, error) {
	switch loc.Kind {
	case packageStorageGitHub:
		settings, err := gitSettings(loc, secret)
		if err != nil {
			return "", err
		}
		manifest := sourcePackageManifest{
			ID: id, Version: version, Kind: kind, Name: id, Description: id + " " + version, Filename: id + "-" + version + ".zip",
		}
		if _, err := pushGitHubRelease(ctx, settings, manifest, payload); err != nil {
			if strings.Contains(err.Error(), "401") {
				return "", errGitHubPaidTokenInvalid
			}
			return "", errors.New("上传到收费仓库失败，请检查令牌是否具备 Contents 读写权限")
		}
		ref := gitHubAssetRef{Owner: loc.Owner, Repo: loc.Repo, Tag: paidAssetTag(kind, id, version), Asset: paidAssetBase(kind, id, version)}
		if !validGitHubAssetRef(ref) {
			return "", errors.New("收费仓库里的标签或文件名不合法")
		}
		return formatGitHubPackageRef(ref), nil
	case packageStorageGitee:
		settings, err := gitSettings(loc, secret)
		if err != nil {
			return "", err
		}
		manifest := sourcePackageManifest{
			ID: id, Version: version, Kind: kind, Name: id, Description: id + " " + version, Filename: id + "-" + version + ".zip",
		}
		if _, err := pushGiteeRelease(ctx, settings, manifest, payload); err != nil {
			return "", errors.New("上传到 Gitee 失败，请检查令牌是否具备仓库和发行版权限")
		}
		ref := gitHubAssetRef{Owner: loc.Owner, Repo: loc.Repo, Tag: paidAssetTag(kind, id, version), Asset: paidAssetBase(kind, id, version)}
		if !validGitHubAssetRef(ref) {
			return "", errors.New("Gitee 仓库里的标签或文件名不合法")
		}
		return formatGiteePackageRef(ref), nil
	case packageStorageS3:
		client, err := s3ClientFor(loc, secret)
		if err != nil {
			return "", err
		}
		key := joinStoragePrefix(loc.KeyPrefix, kind+"/"+id+"/"+version+".zip")
		if err := client.put(ctx, key, payload); err != nil {
			return "", err
		}
		return formatS3PackageRef(loc.ID, key), nil
	default:
		return "", errors.New("不支持的存储类型")
	}
}

func putShardedZip(ctx context.Context, loc storageLocation, secret, kind, id, version string, payload []byte) (string, error) {
	parts, err := splitPayload(payload, storagePartLimit(loc.Kind))
	if err != nil {
		return "", err
	}
	base := paidAssetBase(kind, id, version)
	names := make([]string, 0, len(parts))
	sum := storageSHA256(payload)
	for i, part := range parts {
		name := shardPartName(base, i)
		if err := putNamedObject(ctx, loc, secret, kind, id, version, name, part); err != nil {
			return "", err
		}
		names = append(names, name)
	}
	manifestBody, err := marshalShardManifest(sum, int64(len(payload)), names)
	if err != nil {
		return "", err
	}
	manifestName := shardManifestName(base)
	if err := putNamedObject(ctx, loc, secret, kind, id, version, manifestName, manifestBody); err != nil {
		return "", err
	}
	return objectRef(loc, kind, id, version, manifestName), nil
}

func storageSHA256(payload []byte) string {
	return hexEncodeSHA(payload)
}

func objectRef(loc storageLocation, kind, id, version, name string) string {
	switch loc.Kind {
	case packageStorageGitHub:
		return formatGitHubPackageRef(gitHubAssetRef{Owner: loc.Owner, Repo: loc.Repo, Tag: paidAssetTag(kind, id, version), Asset: name})
	case packageStorageGitee:
		return formatGiteePackageRef(gitHubAssetRef{Owner: loc.Owner, Repo: loc.Repo, Tag: paidAssetTag(kind, id, version), Asset: name})
	default:
		return formatS3PackageRef(loc.ID, joinStoragePrefix(loc.KeyPrefix, name))
	}
}

func putNamedObject(ctx context.Context, loc storageLocation, secret, kind, id, version, name string, payload []byte) error {
	switch loc.Kind {
	case packageStorageGitHub:
		settings, err := gitSettings(loc, secret)
		if err != nil {
			return err
		}
		return uploadGitHubNamedAsset(ctx, settings, paidAssetTag(kind, id, version), name, payload)
	case packageStorageGitee:
		settings, err := gitSettings(loc, secret)
		if err != nil {
			return err
		}
		return uploadGiteeNamedAsset(ctx, settings, paidAssetTag(kind, id, version), name, payload)
	case packageStorageS3:
		client, err := s3ClientFor(loc, secret)
		if err != nil {
			return err
		}
		return client.put(ctx, joinStoragePrefix(loc.KeyPrefix, name), payload)
	default:
		return errors.New("不支持的存储类型")
	}
}

func joinStoragePrefix(prefix, key string) string {
	prefix = strings.Trim(prefix, "/")
	key = strings.TrimLeft(key, "/")
	if prefix == "" {
		return key
	}
	return prefix + "/" + key
}

func uploadGitHubNamedAsset(ctx context.Context, settings sourceReleaseSettings, tag, filename string, payload []byte) error {
	manifest := sourcePackageManifest{ID: "part", Version: "1", Kind: "plugin", Name: filename, Description: filename, Filename: filename}
	settings.TagStrategy = tag
	// pushGitHubRelease 用清单拼文件名。这里改走按指定文件名上传，避免分片名被改掉。
	_, err := uploadGitHubAssetByName(ctx, settings, tag, filename, manifest, payload)
	return err
}

func uploadGitHubAssetByName(ctx context.Context, settings sourceReleaseSettings, tag, filename string, manifest sourcePackageManifest, payload []byte) (string, error) {
	api := strings.TrimRight(sourceGitHubAPIBase, "/")
	owner, repo := url.PathEscape(settings.Owner), url.PathEscape(settings.Repo)
	createURL := api + "/repos/" + owner + "/" + repo + "/releases"
	body := map[string]any{
		"tag_name": tag, "name": manifest.Name, "body": filename, "draft": false, "prerelease": false,
	}
	status, raw, err := sourceReleaseJSON(ctx, http.MethodPost, createURL, githubHeaders(settings.Token), body)
	if err != nil {
		return "", err
	}
	var release gitHubReleaseDTO
	_ = json.Unmarshal(raw, &release)
	if status == http.StatusUnprocessableEntity || status == http.StatusConflict {
		getURL := api + "/repos/" + owner + "/" + repo + "/releases/tags/" + url.PathEscape(tag)
		status, raw, err = sourceReleaseJSON(ctx, http.MethodGet, getURL, githubHeaders(settings.Token), nil)
		if err != nil {
			return "", err
		}
		release = gitHubReleaseDTO{}
		_ = json.Unmarshal(raw, &release)
	}
	if status < 200 || status >= 300 || release.ID == 0 {
		return "", githubAPIError("创建或读取 Release 失败", status, raw, release.Message)
	}
	for _, asset := range release.Assets {
		if !strings.EqualFold(asset.Name, filename) {
			continue
		}
		delURL := api + "/repos/" + owner + "/" + repo + "/releases/assets/" + fmt.Sprintf("%d", asset.ID)
		_, _, _ = sourceReleaseJSON(ctx, http.MethodDelete, delURL, githubHeaders(settings.Token), nil)
	}
	uploadURL, err := githubAssetUploadURL(release.UploadURL, filename)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	for key, value := range githubHeaders(settings.Token) {
		req.Header.Set(key, value)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(payload))
	status, raw, err = sourceReleaseDo(req)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", githubAPIError("上传 Release 附件失败", status, raw, "")
	}
	return filename, nil
}

func uploadGiteeNamedAsset(ctx context.Context, settings sourceReleaseSettings, tag, filename string, payload []byte) error {
	api := strings.TrimRight(sourceGiteeAPIBase, "/")
	owner, repo := url.PathEscape(settings.Owner), url.PathEscape(settings.Repo)
	body := map[string]any{
		"access_token": settings.Token, "tag_name": tag, "name": filename, "body": filename,
		"target_commitish": settings.Branch,
	}
	status, raw, err := sourceReleaseJSON(ctx, http.MethodPost, api+"/repos/"+owner+"/"+repo+"/releases", map[string]string{"Content-Type": "application/json"}, body)
	if err != nil {
		return err
	}
	var release giteeReleaseDTO
	_ = json.Unmarshal(raw, &release)
	if status >= 400 || release.ID == 0 {
		getURL := api + "/repos/" + owner + "/" + repo + "/releases/tags/" + url.PathEscape(tag) + "?access_token=" + url.QueryEscape(settings.Token)
		_, raw, err = sourceReleaseJSON(ctx, http.MethodGet, getURL, nil, nil)
		if err != nil {
			return err
		}
		release = giteeReleaseDTO{}
		_ = json.Unmarshal(raw, &release)
	}
	if release.ID == 0 {
		return errors.New("创建或读取 Gitee Release 失败")
	}
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("access_token", settings.Token)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return err
	}
	if _, err := part.Write(payload); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	attachURL := api + "/repos/" + owner + "/" + repo + "/releases/" + fmt.Sprintf("%d", release.ID) + "/attach_files"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, attachURL, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	status, raw, err = sourceReleaseDo(req)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return githubAPIError("上传 Gitee Release 附件失败", status, raw, "")
	}
	return nil
}

// hexEncodeSHA 单独放在这里，避免和已有 sha256SumHex 的调用方搅在一起。
func hexEncodeSHA(payload []byte) string {
	return sha256SumHex(payload)
}
