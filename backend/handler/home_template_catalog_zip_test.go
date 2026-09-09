package handler

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"auto_pro/softwaresource"
)

func TestCatalogZIPInstallationAndUpdateIntegrity(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	for _, test := range []struct {
		entry, content string
		schema         int
	}{
		{"index.html", `<html><script src="/assets/app.js"></script></html>`, 0},
		{"template.json", `{"schemaVersion":1,"hero":{"title":"home","imageUrl":"assets/cover.png"}}`, 1},
	} {
		t.Run(test.entry, func(t *testing.T) {
			payload := makeTestZIP(t,
				testZIPEntry{name: "dist/" + test.entry, data: test.content},
				testZIPEntry{name: "dist/assets/app.js", data: "console.log('home')"},
				testZIPEntry{name: "dist/assets/cover.png", data: "image"},
			)
			remote := softwaresource.Template{Format: "zip", SchemaVersion: test.schema, SHA256: actualChecksumString(sha256.Sum256(payload))}
			installed, err := installCatalogHomeTemplateZIP(payload, remote)
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Base(installed) != test.entry || !installedTemplateMatches(installed, remote.SHA256) {
				marker, markerErr := readCatalogTemplateInstallation(installed)
				t.Fatalf("catalog ZIP not registered: path=%q wantEntry=%q marker=%+v err=%v entryOK=%v", installed, test.entry, marker, markerErr, installedUploadedTemplateMatches(installed, marker.EntrySHA256))
			}
			marker, err := readCatalogTemplateInstallation(installed)
			if err != nil || marker.SHA256 != remote.SHA256 || !installedUploadedTemplateMatches(installed, marker.EntrySHA256) {
				t.Fatalf("entry checksum not recorded: %+v %v", marker, err)
			}
			if test.schema == 0 {
				content, err := os.ReadFile(installed)
				if err != nil || !strings.Contains(string(content), `src="./assets/app.js"`) {
					t.Fatalf("static assets were not rebased: %q %v", content, err)
				}
			}
			next := remote
			next.SHA256 = strings.Repeat("0", 64)
			if installedTemplateMatches(installed, next.SHA256) {
				t.Fatal("a catalog update must be reported as pending")
			}
			if _, err := installCatalogHomeTemplateZIP(payload, next); err == nil {
				t.Fatal("ZIP checksum mismatch accepted")
			}
			next = remote
			next.SchemaVersion = 2
			if _, err := installCatalogHomeTemplateZIP(payload, next); err == nil {
				t.Fatal("ZIP entry incompatible with catalog schema accepted")
			}
			if !installedUploadedTemplateMatches(installed, marker.EntrySHA256) {
				t.Fatal("failed update damaged the last successful installation")
			}
			if err := os.WriteFile(installed, []byte("tampered"), 0600); err != nil {
				t.Fatal(err)
			}
			if installedTemplateMatches(installed, remote.SHA256) {
				t.Fatal("tampered installed entry accepted")
			}
		})
	}
}
