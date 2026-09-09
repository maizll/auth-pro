package softwaresource

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCatalogZIPFormatCompatibility(t *testing.T) {
	source := Source{ID: "auth-pro-plug", Name: "Template Center", Type: "json"}
	for _, test := range []struct {
		format string
		schema int
		valid  bool
	}{
		{"", 1, true}, {"json", 1, true}, {"zip", 0, true}, {"zip", 1, true},
		{"", 0, false}, {"json", 0, false}, {"zip", 2, false}, {"executable", 1, false},
	} {
		item := Template{ID: "9", TemplateKey: "home", Name: "Home", Version: "1.0.0", Source: source,
			Format: test.format, SchemaVersion: test.schema, SHA256: sha256Hex([]byte("template"))}
		err := validateCatalog(Catalog{Sources: []Source{source}, Templates: []Template{item}})
		if (err == nil) != test.valid {
			t.Fatalf("format=%q schema=%d: %v", test.format, test.schema, err)
		}
	}
}

func TestCatalogZIPContentUsesArchiveLimitAndChecksum(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), int(templateMaxBytes)+1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Software-Source-Key") != "catalog-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	client, err := NewClient(ClientConfig{BaseURL: server.URL, CatalogKey: "catalog-key", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	item := Template{ID: "9", Format: "zip", SHA256: sha256Hex(payload)}
	downloaded, err := client.TemplateContent(context.Background(), item)
	if err != nil || !bytes.Equal(downloaded, payload) {
		t.Fatalf("ZIP transport rejected a package larger than the JSON limit: %v", err)
	}
	item.Format = "json"
	if _, err := client.TemplateContent(context.Background(), item); err == nil {
		t.Fatal("plain JSON must retain its 2 MiB limit")
	}
	item.Format = "zip"
	item.SHA256 = sha256Hex([]byte("different package"))
	if _, err := client.TemplateContent(context.Background(), item); err == nil {
		t.Fatal("ZIP checksum mismatch accepted")
	}
}
