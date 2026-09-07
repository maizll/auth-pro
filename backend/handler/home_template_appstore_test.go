package handler

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"auto_pro/appstore"
)

func TestNormalizeAppStoreTemplateMetadataDoesNotUseEmbeddedCatalog(t *testing.T) {
	item := appstore.Template{TemplateID: "fintech-gold", Name: "金融金首页", Description: "模板", Version: "1.0.0"}
	normalizeAppStoreTemplateMetadata(&item)
	if item.PreviewImage != "" || item.Author.Name != "未提供" {
		t.Fatalf("embedded catalog metadata leaked into item: %+v", item)
	}
}

func TestNormalizeAppStoreTemplateMetadataCompletesUnknownSource(t *testing.T) {
	item := appstore.Template{TemplateID: "external-template"}
	normalizeAppStoreTemplateMetadata(&item)
	if item.Name == "" || item.Description == "" || item.Version == "" || item.Author.Name == "" {
		t.Fatalf("metadata remains incomplete: %+v", item)
	}
}

func TestInstalledTemplateUpdateDetectionAndLegacyResponse(t *testing.T) {
	payload := []byte(`{"schemaVersion":1,"hero":{"title":"installed"}}`)
	path := filepath.Join(t.TempDir(), "template.json")
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	checksum := actualChecksumString(sha256.Sum256(payload))
	if !installedTemplateMatches(path, checksum) {
		t.Fatal("unchanged installed template should not require a download")
	}
	newChecksum := actualChecksumString(sha256.Sum256([]byte("new version")))
	if installedTemplateMatches(path, newChecksum) || installedTemplateMatches("", checksum) {
		t.Fatal("a new version or missing installation must require a download")
	}
	invalid := []byte(`{"schemaVersion":1,"hero":{"title":"bad"},"scripts":[]}`)
	if err := os.WriteFile(path, invalid, 0600); err != nil {
		t.Fatal(err)
	}
	if installedTemplateMatches(path, actualChecksumString(sha256.Sum256(invalid))) {
		t.Fatal("checksum alone must not bypass document validation")
	}
	items := legacyHomeTemplateItems([]appstore.Template{{ID: "9", TemplateID: "fintech-gold", UpdateAvailable: true}})
	if len(items) != 1 || items[0]["updateAvailable"] != true {
		t.Fatal("both admin template views must receive the update flag")
	}
}
