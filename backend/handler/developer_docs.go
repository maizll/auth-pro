package handler

import (
	"os"
	"path/filepath"
)

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
