package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

const stationPackagePublicPath = "/api/v1/public/source-packages/"

var (
	stationPackageNamePattern = regexp.MustCompile(`^[a-f0-9]{64}\.zip$`)
	// verifyExternalPackage 校验外链 ZIP。测试可替换；生产走 HTTPS 下载。
	verifyExternalPackage = verifyExternalPackageHTTP
	// externalPackageClient 为空时使用带超时的默认客户端。测试 TLS 服务可注入信任该证书的客户端。
	externalPackageClient *http.Client
)

func stationPackageDir() string {
	dir := filepath.Join(config.GetDataDir(), "source-packages")
	_ = os.MkdirAll(dir, 0750)
	return dir
}

func isStationHostedPackageURL(raw string) bool {
	_, ok := stationPackageNameFromURL(raw)
	return ok
}

func stationPackageNameFromURL(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", false
	}
	path := value
	if strings.Contains(value, "://") {
		parsed, err := http.NewRequest(http.MethodGet, value, nil)
		if err != nil || parsed.URL == nil {
			return "", false
		}
		path = parsed.URL.Path
	}
	if !strings.HasPrefix(path, stationPackagePublicPath) {
		return "", false
	}
	name := strings.TrimPrefix(path, stationPackagePublicPath)
	if strings.Contains(name, "/") || strings.Contains(name, "..") || !stationPackageNamePattern.MatchString(name) {
		return "", false
	}
	return name, true
}

func stationPackagePublicURL(name string) string {
	return stationPackagePublicPath + name
}

func stationPackageIdentity(raw string) (publicURL, fileSHA string, err error) {
	name, ok := stationPackageNameFromURL(raw)
	if !ok {
		return "", "", errors.New("本站托管地址不合法")
	}
	path := filepath.Join(stationPackageDir(), name)
	payload, readErr := os.ReadFile(path)
	if readErr != nil || len(payload) == 0 {
		return "", "", errors.New("本站托管的 ZIP 不存在")
	}
	if !isZipPayload(payload) {
		return "", "", errors.New("本站托管文件不是 ZIP")
	}
	sum := sha256.Sum256(payload)
	fileSHA = hex.EncodeToString(sum[:])
	if strings.TrimSuffix(name, ".zip") != fileSHA {
		return "", "", errors.New("本站托管文件名与内容校验码不一致")
	}
	return stationPackagePublicURL(name), fileSHA, nil
}

func normalizePackageLocation(location, sha string) (string, string, error) {
	location = strings.TrimSpace(location)
	sha = strings.ToLower(strings.TrimSpace(sha))
	if !isStationHostedPackageURL(location) {
		return location, sha, nil
	}
	publicURL, fileSHA, err := stationPackageIdentity(location)
	if err != nil {
		return "", "", err
	}
	if sha == "" {
		sha = fileSHA
	} else if sha != fileSHA {
		return "", "", errors.New("sha256 与本站托管 ZIP 不一致")
	}
	return publicURL, sha, nil
}

func storeStationPackage(payload []byte) (string, string, error) {
	if !isZipPayload(payload) {
		return "", "", errors.New("必须上传 ZIP 压缩包")
	}
	sum := sha256.Sum256(payload)
	fileSHA := hex.EncodeToString(sum[:])
	name := fileSHA + ".zip"
	dir := stationPackageDir()
	finalPath := filepath.Join(dir, name)
	tmp, err := os.CreateTemp(dir, "upload-*.zip")
	if err != nil {
		return "", "", errors.New("保存 ZIP 失败")
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.Write(payload)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tmpName)
		return "", "", errors.New("保存 ZIP 失败")
	}
	if err := os.Rename(tmpName, finalPath); err != nil {
		_ = os.Remove(tmpName)
		return "", "", errors.New("保存 ZIP 失败")
	}
	_ = os.Chmod(finalPath, 0640)
	return stationPackagePublicURL(name), fileSHA, nil
}

func SourceDeveloperPackageUpload(c *gin.Context) {
	if _, err := currentSourceDeveloper(c); err != nil {
		writeCurrentSourceDeveloperError(c, err)
		return
	}
	defer discardSourceMultipart(c)
	filename, payload, err := readSourcePackageUpload(c)
	if err != nil {
		writeSourcePackageReject(c, err)
		return
	}
	manifest, err := parseSourcePackageBytes(filename, payload, c.PostForm("kind"), c.PostForm("category"))
	if err != nil {
		payload = nil
		writeSourcePackageReject(c, err)
		return
	}
	publicURL, fileSHA, err := storeStationPackage(payload)
	payload = nil
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	if !strings.EqualFold(manifest.SHA256, fileSHA) {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存后的校验码与清单不一致"})
		return
	}
	data := manifest.view()
	data["stored"] = true
	data["url"] = publicURL
	data["sha256"] = fileSHA
	if manifest.Kind == sourceKindTemplate {
		data["templateUrl"] = publicURL
	} else {
		data["downloadUrl"] = publicURL
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "ZIP 已保存到本站，地址和校验码已生成", "data": data})
}

func PublicSourcePackageFile(c *gin.Context) {
	name, ok := stationPackageNameFromURL(stationPackagePublicPath + strings.TrimSpace(c.Param("name")))
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	path := filepath.Join(stationPackageDir(), name)
	if _, err := os.Stat(path); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("Content-Disposition", "attachment; filename=\""+name+"\"")
	c.File(path)
}

func validatePackageForSubmit(kind, location, sha string) error {
	location = strings.TrimSpace(location)
	sha = strings.ToLower(strings.TrimSpace(sha))
	label := "下载地址"
	if kind == sourceKindTemplate {
		label = "模板地址"
	}
	if location == "" || sha == "" {
		return errors.New("提交审核前请填写" + label + "和校验码")
	}
	if err := validateSHA256(sha); err != nil {
		return err
	}
	if isStationHostedPackageURL(location) {
		_, fileSHA, err := stationPackageIdentity(location)
		if err != nil {
			return err
		}
		if fileSHA != sha {
			return errors.New("sha256 与本站托管 ZIP 不一致")
		}
		return nil
	}
	if strings.HasPrefix(strings.ToLower(location), "https://") {
		if err := validateExternalHTTPS(location); err != nil {
			return err
		}
		return verifyExternalPackage(context.Background(), location, sha)
	}
	if kind == sourceKindTemplate {
		return validateTemplateLocation(location)
	}
	return errors.New("下载地址须为 https 开头的外链，或上传 ZIP 由本站托管")
}

func verifyExternalPackageHTTP(ctx context.Context, rawURL, sha string) error {
	sha = strings.ToLower(strings.TrimSpace(sha))
	if err := validateExternalHTTPS(rawURL); err != nil {
		return err
	}
	if err := validateSHA256(sha); err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, strings.TrimSpace(rawURL), nil)
	if err != nil {
		return errors.New("外链地址不合法")
	}
	req.Header.Set("User-Agent", "auth-pro-package-check")
	req.Header.Set("Accept", "application/zip,*/*")
	resp, err := externalHTTPClient().Do(req)
	if err != nil {
		return errors.New("外链不可达")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("外链不可达")
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, pluginPackageMaxSize+1))
	if err != nil {
		return errors.New("读取外链失败")
	}
	if int64(len(payload)) > pluginPackageMaxSize {
		return errors.New("外链 ZIP 超过 20 MiB")
	}
	if !isZipPayload(payload) {
		return errors.New("外链不是 ZIP")
	}
	sum := sha256.Sum256(payload)
	if hex.EncodeToString(sum[:]) != sha {
		return errors.New("外链内容与 sha256 不一致")
	}
	return nil
}

func externalHTTPClient() *http.Client {
	if externalPackageClient != nil {
		return externalPackageClient
	}
	return &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("重定向过多")
			}
			if req.URL == nil || !strings.EqualFold(req.URL.Scheme, "https") {
				return errors.New("外链重定向离开 https")
			}
			return nil
		},
	}
}
