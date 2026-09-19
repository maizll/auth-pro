package handler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

const (
	advertisementImageMaxBytes  = int64(2 << 20)
	advertisementPublicFilePath = "/api/v1/public/advertisement-files/"
)

var (
	advertisementIDPattern        = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,58}$`)
	advertisementImageNamePattern = regexp.MustCompile(`^[a-f0-9]{12}\.(png|jpe?g|gif|webp)$`)
)

func generateAdvertisementID() (string, error) {
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "ad-" + hex.EncodeToString(raw), nil
}

func advertisementImageDir() string {
	dir := filepath.Join(config.GetDataDir(), "advertisement-images")
	_ = os.MkdirAll(dir, 0750)
	return dir
}

func validateAdvertisementImageURL(raw string) error {
	value := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(value), "https://") {
		return validateExternalHTTPS(value)
	}
	name, ok := advertisementImageNameFromURL(value)
	if !ok {
		return errors.New("广告图片地址不合法")
	}
	if _, err := os.Stat(filepath.Join(advertisementImageDir(), name)); err != nil {
		return errors.New("广告图片不存在")
	}
	return nil
}

func advertisementImageNameFromURL(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if !strings.HasPrefix(value, advertisementPublicFilePath) {
		return "", false
	}
	name := strings.TrimPrefix(value, advertisementPublicFilePath)
	if strings.Contains(name, "/") || strings.Contains(name, "..") || !advertisementImageNamePattern.MatchString(name) {
		return "", false
	}
	return name, true
}

func AdminSourceAdvertisementImageUpload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请选择要上传的图片"})
		return
	}
	if file.Size <= 0 || file.Size > advertisementImageMaxBytes {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "广告图须小于 2MB"})
		return
	}
	opened, err := file.Open()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "读取图片失败"})
		return
	}
	defer opened.Close()

	payload, err := io.ReadAll(io.LimitReader(opened, advertisementImageMaxBytes+1))
	if err != nil || int64(len(payload)) > advertisementImageMaxBytes {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "读取图片失败"})
		return
	}
	ext, ok := detectAdvertisementImageExt(payload)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "仅支持 PNG / JPEG / GIF / WebP"})
		return
	}

	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "生成图片文件名失败"})
		return
	}
	name := hex.EncodeToString(raw) + ext
	if err := os.WriteFile(filepath.Join(advertisementImageDir(), name), payload, 0640); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存图片失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "图片已上传", "data": gin.H{
		"url": advertisementPublicFilePath + name,
	}})
}

func PublicAdvertisementFile(c *gin.Context) {
	name, ok := advertisementImageNameFromURL(advertisementPublicFilePath + strings.TrimSpace(c.Param("name")))
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	path := filepath.Join(advertisementImageDir(), name)
	if _, err := os.Stat(path); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.File(path)
}

func detectAdvertisementImageExt(payload []byte) (string, bool) {
	if len(payload) < 12 {
		return "", false
	}
	switch {
	case payload[0] == 0x89 && payload[1] == 0x50 && payload[2] == 0x4e && payload[3] == 0x47:
		return ".png", true
	case payload[0] == 0xff && payload[1] == 0xd8 && payload[2] == 0xff:
		return ".jpg", true
	case string(payload[:6]) == "GIF87a" || string(payload[:6]) == "GIF89a":
		return ".gif", true
	case string(payload[:4]) == "RIFF" && string(payload[8:12]) == "WEBP":
		return ".webp", true
	default:
		return "", false
	}
}
