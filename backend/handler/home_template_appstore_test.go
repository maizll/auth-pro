package handler

import (
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
