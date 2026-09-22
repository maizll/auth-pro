package handler

import (
	"embed"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// developerEmbedFS 是 docs/developer 与 AI Skill 的编译期副本。
// 宝塔在线更新只替换 frontend 文件和 backend/auth_pro，工作目录通常是数据目录，
// findDeveloperDocsDir 走不到仓库。发布前由 scripts/sync-developer-embed.sh 同步。
//
//go:embed all:developer_embed
var developerEmbedFS embed.FS

const embeddedDeveloperSkillPath = "developer_embed/skill/SKILL.md"

func findDeveloperDocsDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "docs", "developer")
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func readDeveloperDocFile(rel string) ([]byte, error) {
	dir := findDeveloperDocsDir()
	if dir == "" {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
}

func readEmbeddedDeveloperDoc(rel string) ([]byte, error) {
	clean := path.Clean(rel)
	if clean == "." || clean == "/" || strings.HasPrefix(clean, "..") || strings.Contains(clean, `\`) {
		return nil, os.ErrNotExist
	}
	return developerEmbedFS.ReadFile("developer_embed/docs/" + clean)
}
