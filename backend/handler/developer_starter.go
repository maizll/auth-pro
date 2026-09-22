package handler

import (
	"archive/zip"
	"bytes"
	"errors"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const developerStarterZipName = "auth-pro-developer-starter.zip"

func SourceDeveloperStarterZIP(c *gin.Context) {
	if _, err := currentSourceDeveloper(c); err != nil {
		writeCurrentSourceDeveloperError(c, err)
		return
	}
	payload, err := buildDeveloperStarterZIP()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "生成开发者入门包失败"})
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+developerStarterZipName+`"`)
	c.Data(http.StatusOK, "application/zip", payload)
}

func SourceDeveloperSkillMarkdown(c *gin.Context) {
	if _, err := currentSourceDeveloper(c); err != nil {
		writeCurrentSourceDeveloperError(c, err)
		return
	}
	body := developerSkillMarkdown()
	if body == "" {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "生成 AI Skill 失败"})
		return
	}
	c.Header("Content-Disposition", `attachment; filename="SKILL.md"`)
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(body))
}

func buildDeveloperStarterZIP() ([]byte, error) {
	files, err := developerStarterFiles()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	buffer := &bytes.Buffer{}
	writer := zip.NewWriter(buffer)
	now := time.Now()
	for _, name := range names {
		header := &zip.FileHeader{Name: path.Join("auth-pro-developer-starter", name), Method: zip.Deflate}
		header.SetModTime(now)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		if _, err := entry.Write([]byte(files[name])); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func developerStarterFiles() (map[string]string, error) {
	skill := developerSkillMarkdown()
	if skill == "" {
		return nil, errMissingEmbeddedDeveloperDoc
	}
	files := map[string]string{
		"SKILL.md": skill,
		".cursor/skills/auth-pro-plugin-template/SKILL.md": skill,
	}
	embedded := map[string]string{
		"README.md":                                   "starter/README.md",
		"docs/README.md":                              "README.md",
		"docs/charter.md":                             "charter.md",
		"docs/plugin-package.md":                      "plugin-package.md",
		"docs/template-package.md":                    "template-package.md",
		"docs/packaging.md":                           "packaging.md",
		"docs/validation.md":                          "validation.md",
		"docs/versions.md":                            "versions.md",
		"docs/review-and-catalog.md":                  "review-and-catalog.md",
		"plugin-example/plugin.json":                  "starter/plugin-example/plugin.json",
		"plugin-example/README.md":                    "starter/plugin-example/README.md",
		"template-example/template.json":              "starter/template-example/template.json",
		"template-example/template.fintech-gold.json": "starter/template-example/template.fintech-gold.json",
		"template-example/README.md":                  "starter/template-example/README.md",
	}
	for name, rel := range embedded {
		payload, err := readEmbeddedDeveloperDoc(rel)
		if err != nil || len(bytes.TrimSpace(payload)) == 0 {
			return nil, errMissingEmbeddedDeveloperDoc
		}
		files[name] = string(payload)
	}
	return files, nil
}

// developerSkillMarkdown 只返回编译进二进制的正式 SKILL。
// 磁盘上的 docs/developer 或三行 stub 不能覆盖下载结果。
func developerSkillMarkdown() string {
	payload, err := developerEmbedFS.ReadFile(embeddedDeveloperSkillPath)
	if err != nil || !developerSkillIsActionable(payload) {
		return ""
	}
	if strings.TrimSpace(string(payload)) == developerSkillStubSentence {
		return ""
	}
	return string(payload)
}

func developerSkillIsActionable(payload []byte) bool {
	if len(payload) < 800 {
		return false
	}
	text := string(payload)
	if strings.Contains(text, developerSkillStubSentence) {
		return false
	}
	for _, needle := range []string{
		"name: auth-pro-plugin-template",
		"sha256sum",
		"zip -X",
		"stylePreset",
		"cartoon-blue",
		"fintech-gold",
		"primaryColor",
		"backgroundColor",
		"textColor",
		"schemaVersion",
		"template.json",
		"plugin.json",
		"primaryAction",
		"downloadUrl",
		"templateUrl",
		"登记（中文标签",
	} {
		if !strings.Contains(text, needle) {
			return false
		}
	}
	return true
}

const developerSkillStubSentence = "template.json 必须包含 kind: template。插件自定义分类会出现在应用商店筛选页签。"

var errMissingEmbeddedDeveloperDoc = errors.New("embedded developer docs missing")
