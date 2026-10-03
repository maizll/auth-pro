package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"auto_pro/config"
	"auto_pro/updatesign"

	"github.com/gin-gonic/gin"
)

// 本地上传更新包：更新源连不上时的备用路径。
// 上传的只是 Release 里那一个原包，签名清单和发布信息都在包里。
// 识别出版本后交给 executeOnlineUpdate，和在线下载走同一套验签、备份、重启和回退流程，只是不用下载。

var onlineUpdateUploadIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

const onlineUpdateUploadPrefix = "upload-"

// onlineUpdateUploadPath 返回上传包的保存位置。id 只能是 32 位小写十六进制，路径不受上传文件名影响。
func onlineUpdateUploadPath(id string) (string, bool) {
	if !onlineUpdateUploadIDPattern.MatchString(id) {
		return "", false
	}
	return filepath.Join(config.GetUpdateDir(), onlineUpdateUploadPrefix+id+".tar.gz"), true
}

// removeOnlineUpdateUploads 删掉之前留下的上传包，同一时间只保留一个待安装的上传。
func removeOnlineUpdateUploads() {
	matches, _ := filepath.Glob(filepath.Join(config.GetUpdateDir(), onlineUpdateUploadPrefix+"*.tar.gz"))
	for _, item := range matches {
		_ = os.Remove(item)
	}
}

func onlineUpdateEditionLabel(edition string) string {
	switch edition {
	case updatesign.EditionOfficial:
		return "官网"
	case updatesign.EditionClient:
		return "客户站"
	}
	return "未知"
}

func currentOnlineUpdateEdition() string {
	if officialSite() {
		return updatesign.EditionOfficial
	}
	return updatesign.EditionClient
}

// checkOnlineUpdatePackageInfo 核对包内发布信息：版本和签名清单一致，适用端是本站这一端。
// 在线下载和上传都走这里。
func checkOnlineUpdatePackageInfo(info *updatesign.ReleaseInfo, signedVersion string) error {
	if info == nil {
		return errors.New("这个更新包里没有版本说明，是旧格式的包，网站没有任何改动。请下载最新版本的原文件再试")
	}
	if !sameProductVersion(info.Version, signedVersion) {
		return errors.New("这个更新包里的版本信息前后对不上，可能被人改过，网站没有任何改动")
	}
	current := currentOnlineUpdateEdition()
	if info.Edition == current {
		return nil
	}
	if current == updatesign.EditionOfficial {
		return errors.New("这是客户站的更新包，官网不能用，网站没有任何改动。请换成官网的更新包")
	}
	if info.Edition == updatesign.EditionOfficial {
		return errors.New("这是官网的更新包，客户站不能用，网站没有任何改动。请在官网「我的授权」→「版本下载」里下载客户站的更新包")
	}
	return errors.New("认不出这个更新包是给哪一端用的，网站没有任何改动")
}

// inspectUploadedOnlineUpdatePackage 验签并识别上传的包，返回交给安装流程的清单。
// 签名、适用端、版本先后都在这里核对，上传和确认安装时各跑一次。
func inspectUploadedOnlineUpdatePackage(packagePath string) (*onlineUpdateManifest, error) {
	stat, err := os.Stat(packagePath)
	if err != nil {
		return nil, errors.New("找不到刚才上传的更新包，请重新上传")
	}
	if stat.Size() > maxOnlineUpdatePackageSize {
		return nil, errors.New("文件太大了，更新包不会超过 512MB，请确认选的是 auth_pro-full 开头的原文件")
	}
	if !looksLikeGzip(packagePath) {
		return nil, errors.New("这不是更新包文件，请选 auth_pro-full 开头的 .tar.gz 原文件")
	}
	signed, info, err := updatesign.InspectPackage(packagePath, onlineUpdatePublicKey())
	if err != nil {
		return nil, uploadedOnlineUpdateFailure(err)
	}
	if err := checkOnlineUpdatePackageInfo(info, signed.Version); err != nil {
		return nil, err
	}
	sum, err := hashOnlineUpdateFile(packagePath)
	if err != nil {
		return nil, err
	}
	fileName, err := onlineUpdatePackageFileName(signed.Version)
	if err != nil {
		return nil, errors.New("这个更新包的版本号格式不对，网站没有任何改动")
	}
	manifest := &onlineUpdateManifest{
		Version:    strings.TrimPrefix(strings.TrimSpace(signed.Version), "v"),
		Channel:    info.Channel,
		MinVersion: info.MinVersion,
		ReleasedAt: info.ReleasedAt,
		Notes:      info.Notes,
		Package: onlineUpdatePackage{
			OS:        "linux",
			Arch:      "amd64",
			FileName:  fileName,
			SHA256:    sum,
			Size:      stat.Size(),
			Signature: onlineUpdateSignatureForSHA256(sum),
		},
		Actions: onlineUpdateActions{
			UpdateFrontend: true,
			UpdateBackend:  true,
			RestartBackend: true,
			BackupDatabase: true,
		},
		uploadedPath: packagePath,
	}
	available, versionErr := onlineUpdateAvailable(config.AppVersion, manifest)
	if versionErr != "" && manifest.MinVersion == "" {
		return nil, errors.New("认不出当前版本或更新包的版本号，网站没有任何改动")
	}
	if versionErr != "" {
		return nil, fmt.Errorf("当前版本 %s 太旧，不能直接装 %s，请先升级到 %s 或更新的版本", config.AppVersion, manifest.Version, manifest.MinVersion)
	}
	if !available {
		if sameProductVersion(config.AppVersion, manifest.Version) {
			return nil, fmt.Errorf("这个更新包是 %s，和当前版本一样，不用再装", manifest.Version)
		}
		return nil, fmt.Errorf("这个更新包是 %s，比当前版本 %s 旧，不能用来降级", manifest.Version, config.AppVersion)
	}
	return manifest, nil
}

// looksLikeGzip 看文件头是不是 gzip，用来给选错文件的人一句明白话。
func looksLikeGzip(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	head := make([]byte, 2)
	if _, err := io.ReadFull(file, head); err != nil {
		return false
	}
	return head[0] == 0x1f && head[1] == 0x8b
}

// uploadedOnlineUpdateFailure 把验签失败换成大白话。上传的包不是下载来的，不说「下载途中损坏」。
func uploadedOnlineUpdateFailure(err error) error {
	switch {
	case errors.Is(err, updatesign.ErrUnsigned):
		return errors.New("这个文件没有官方签名，不是正式发布的更新包，网站没有任何改动")
	case errors.Is(err, updatesign.ErrBadSignature), errors.Is(err, updatesign.ErrTampered):
		return errors.New("这个更新包没有通过官方签名校验，可能文件不完整或被人改过，网站没有任何改动。请重新下载原文件再试")
	}
	return errors.New("读不出这个更新包，请确认选的是 auth_pro-full 开头的 .tar.gz 原文件")
}

// writeOnlineUpdateAuditLog 把更新操作写进操作日志：谁在什么时候装了哪个版本，或者哪个上传被拒。
func writeOnlineUpdateAuditLog(c *gin.Context, action string, detail map[string]any) {
	db, err := config.DB()
	if err != nil {
		return
	}
	detail["fromVersion"] = config.AppVersion
	raw, err := json.Marshal(detail)
	if err != nil {
		return
	}
	_, _ = db.Exec(`INSERT INTO operation_logs
		(operator_type, operator_id, action, target_type, target_id, detail, ip)
		VALUES ('admin', ?, ?, 'system_update', NULL, ?, ?)`, c.GetUint("user_id"), action, string(raw), c.ClientIP())
}

func onlineUpdateUploadView(id string, manifest *onlineUpdateManifest) gin.H {
	edition := currentOnlineUpdateEdition()
	return gin.H{
		"uploadId":       id,
		"currentVersion": config.AppVersion,
		"version":        manifest.Version,
		"edition":        edition,
		"editionLabel":   onlineUpdateEditionLabel(edition),
		"releasedAt":     manifest.ReleasedAt,
		"notes":          manifest.Notes,
		"size":           manifest.Package.Size,
		"sha256":         manifest.Package.SHA256,
	}
}

// AdminOnlineUpdateUpload 接收上传的更新包，验签后只返回识别出的信息，不安装。
func AdminOnlineUpdateUpload(c *gin.Context) {
	if runningOnlineUpdateJob() != nil {
		c.JSON(http.StatusOK, gin.H{"code": 409, "msg": "已经有更新正在进行，请等它结束后再上传"})
		return
	}
	if err := os.MkdirAll(config.GetUpdateDir(), 0755); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "服务器上没法保存更新包，请检查数据目录权限"})
		return
	}
	reject := func(message string) {
		writeOnlineUpdateAuditLog(c, "online_update_upload_rejected", map[string]any{"reason": message})
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": message})
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxOnlineUpdatePackageSize+(1<<20))
	reader, err := c.Request.MultipartReader()
	if err != nil {
		reject("没有收到文件，请重新选择更新包")
		return
	}
	var part io.Reader
	for {
		item, err := reader.NextPart()
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				reject("文件太大了，更新包不会超过 512MB，请确认选的是 auth_pro-full 开头的原文件")
				return
			}
			reject("没有收到文件，请重新选择更新包")
			return
		}
		if item.FormName() == "file" {
			part = item
			break
		}
	}

	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存更新包失败，请重试"})
		return
	}
	id := hex.EncodeToString(idBytes)
	target, _ := onlineUpdateUploadPath(id)
	removeOnlineUpdateUploads()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "服务器上没法保存更新包，请检查数据目录权限"})
		return
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(out, hash), io.LimitReader(part, maxOnlineUpdatePackageSize+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || written > maxOnlineUpdatePackageSize {
		_ = os.Remove(target)
		var tooLarge *http.MaxBytesError
		if written > maxOnlineUpdatePackageSize || errors.As(copyErr, &tooLarge) {
			reject("文件太大了，更新包不会超过 512MB，请确认选的是 auth_pro-full 开头的原文件")
			return
		}
		reject("上传中断了，请检查网络后重新上传")
		return
	}
	if written == 0 {
		_ = os.Remove(target)
		reject("这是一个空文件，请重新选择更新包")
		return
	}

	manifest, err := inspectUploadedOnlineUpdatePackage(target)
	if err != nil {
		_ = os.Remove(target)
		reject(err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": onlineUpdateUploadView(id, manifest)})
}

// AdminOnlineUpdateUploadApply 用户确认后安装上传的包。安装前重新验一遍，避免两次请求之间文件被换掉。
func AdminOnlineUpdateUploadApply(c *gin.Context) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "一键整包更新仅支持 Linux amd64 宝塔部署环境"})
		return
	}
	var req struct {
		UploadID string `json:"uploadId"`
	}
	_ = c.ShouldBindJSON(&req)
	packagePath, ok := onlineUpdateUploadPath(strings.TrimSpace(req.UploadID))
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "找不到刚才上传的更新包，请重新上传"})
		return
	}
	manifest, err := inspectUploadedOnlineUpdatePackage(packagePath)
	if err != nil {
		_ = os.Remove(packagePath)
		writeOnlineUpdateAuditLog(c, "online_update_upload_rejected", map[string]any{"reason": err.Error()})
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	startOnlineUpdateJob(c, manifest, "online_update_upload")
}
