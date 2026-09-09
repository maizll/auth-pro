package handler

import (
	"os"
	"path/filepath"
	"testing"

	"auto_pro/config"
)

func TestManagedTemplateRemovalPaths(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	base := config.GetHomeTemplateDir()
	for _, name := range []string{"upload-test/index.html", "upload-test/template.json", "9/0123456789abcdef/template.json"} {
		path := filepath.Join(base, filepath.FromSlash(name))
		directory, err := managedHomeTemplateDirectory(path, 9)
		if err != nil || directory != filepath.Dir(path) {
			t.Fatalf("safe missing directory rejected: %s %v", name, err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if _, err := managedHomeTemplateDirectory(path, 9); err != nil {
			t.Fatalf("safe existing directory rejected: %s %v", name, err)
		}
	}
	for _, path := range []string{
		"", filepath.Join(base, "index.html"), filepath.Join(base, "9", "template.json"),
		filepath.Join(base, "10", "0123456789abcdef", "template.json"),
		filepath.Join(base, "9", "not-a-checksum", "template.json"),
		filepath.Join(base, "9", "0123456789abcdef", "index.html"),
		filepath.Join(base, "upload-test", "other.html"),
		filepath.Join(base, "upload-test", "nested", "index.html"),
		filepath.Join(t.TempDir(), "upload-test", "index.html"),
	} {
		if _, err := managedHomeTemplateDirectory(path, 9); err == nil {
			t.Fatalf("unsafe removal path accepted: %q", path)
		}
	}
}
