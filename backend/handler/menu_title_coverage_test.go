package handler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode"
)

var menuKeyPattern = regexp.MustCompile(`['"]menus(?:\.[A-Za-z0-9_]+)+['"]`)

func TestProductMenuTitlesHaveChinese(t *testing.T) {
	root := repoRoot(t)
	zh := flattenZHMenus(t, filepath.Join(root, "frontend/src/locales/langs/zh.json"))
	keys := collectProductMenuKeys(t, root)
	if len(keys) == 0 {
		t.Fatal("no menus.* titles found in routes, menu_spec.go, or menu_seed.sql")
	}

	var missing []string
	for _, key := range keys {
		zhTitle, ok := zh[key]
		got := resolveMenuTitle(key)
		leaf := key[strings.LastIndex(key, ".")+1:]
		if !ok || !acceptableMenuTitle(zhTitle) || got != zhTitle || got == key || got == leaf {
			missing = append(missing, key+" => map:"+got+" zh:"+zhTitle)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("菜单标题缺中文:\n%s", strings.Join(missing, "\n"))
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func flattenZHMenus(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read zh.json: %v", err)
	}
	var doc struct {
		Menus json.RawMessage `json:"menus"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse zh.json: %v", err)
	}
	var node any
	if err := json.Unmarshal(doc.Menus, &node); err != nil {
		t.Fatalf("parse menus: %v", err)
	}
	out := map[string]string{}
	flattenMenuNode(node, "menus", out)
	return out
}

func flattenMenuNode(node any, prefix string, out map[string]string) {
	switch value := node.(type) {
	case string:
		if prefix != "" {
			out[prefix] = value
		}
	case map[string]any:
		for key, child := range value {
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			flattenMenuNode(child, next, out)
		}
	}
}

func collectProductMenuKeys(t *testing.T, root string) []string {
	t.Helper()
	files := []string{
		filepath.Join(root, "backend/handler/menu_spec.go"),
		filepath.Join(root, "backend/handler/menu_seed.sql"),
	}
	routerDir := filepath.Join(root, "frontend/src/router")
	err := filepath.Walk(routerDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".ts") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk router: %v", err)
	}

	seen := map[string]bool{}
	var keys []string
	for _, file := range files {
		text, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, match := range menuKeyPattern.FindAllString(string(text), -1) {
			key := strings.Trim(match, `'"`)
			if seen[key] {
				continue
			}
			seen[key] = true
			keys = append(keys, key)
		}
	}
	return keys
}

func acceptableMenuTitle(title string) bool {
	if title == "" || title == "未命名菜单" {
		return false
	}
	digits := true
	for _, r := range title {
		if unicode.Is(unicode.Han, r) {
			return true
		}
		if r < '0' || r > '9' {
			digits = false
		}
	}
	return digits
}
