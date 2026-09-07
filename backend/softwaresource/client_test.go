package softwaresource

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCatalogFetchAndStaleFallback(t *testing.T) {
	var requests atomic.Int32
	cacheDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Header.Get("X-Software-Source-Key") != "catalog-key" {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/catalog/sources":
			writeEnvelope(response, map[string]any{"list": []Source{{ID: "builtin", Name: "内置软件源", Type: "builtin", State: "ok"}}, "revision": 3})
		case "/api/v1/catalog/templates":
			writeEnvelope(response, map[string]any{"list": []Template{{ID: "tpl-1", TemplateKey: "home", Name: "首页", Version: "1.0.0", SchemaVersion: 1, SHA256: strings.Repeat("a", 64), Source: Source{ID: "builtin"}}}, "total": 1, "revision": 3})
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	client, err := NewClient(ClientConfig{BaseURL: server.URL, CatalogKey: "catalog-key", Timeout: time.Second, StaleTTL: time.Hour, CacheDir: cacheDir})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := client.Catalog(context.Background())
	if err != nil || len(catalog.Templates) != 1 || catalog.Revision != 3 {
		t.Fatalf("catalog=%+v err=%v", catalog, err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests=%d", requests.Load())
	}
	server.Close()
	client.mutex.Lock()
	client.cached.FetchedAt = time.Now().Add(-catalogCacheTTL - time.Second)
	client.mutex.Unlock()
	stale, err := client.Catalog(context.Background())
	if err != nil || len(stale.Templates) != 1 {
		t.Fatalf("stale catalog=%+v err=%v", stale, err)
	}
	restarted, err := NewClient(ClientConfig{BaseURL: server.URL, CatalogKey: "catalog-key", Timeout: 20 * time.Millisecond, StaleTTL: time.Hour, CacheDir: cacheDir})
	if err != nil {
		t.Fatal(err)
	}
	diskStale, err := restarted.Catalog(context.Background())
	if err != nil || len(diskStale.Templates) != 1 {
		t.Fatalf("disk stale catalog=%+v err=%v", diskStale, err)
	}
}

func TestClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		time.Sleep(100 * time.Millisecond)
		writeEnvelope(response, map[string]any{"list": []any{}, "revision": 1})
	}))
	defer server.Close()
	client, err := NewClient(ClientConfig{BaseURL: server.URL, CatalogKey: "catalog-key", Timeout: 20 * time.Millisecond, StaleTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Catalog(context.Background()); err == nil || !strings.Contains(err.Error(), ErrUnavailable.Error()) {
		t.Fatalf("timeout error=%v", err)
	}
}

func TestRefreshBypassesFreshCache(t *testing.T) {
	var revision atomic.Int32
	revision.Store(1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		current := revision.Load()
		switch request.URL.Path {
		case "/api/v1/catalog/sources":
			writeEnvelope(response, map[string]any{"list": []Source{{ID: "builtin", Name: "内置软件源", Type: "builtin", State: "ok"}}, "revision": current})
		case "/api/v1/catalog/templates":
			items := []Template{}
			if current == 1 {
				items = append(items, Template{ID: "tpl-1", TemplateKey: "home", Name: "首页", Version: "1.0.0", SchemaVersion: 1, SHA256: strings.Repeat("a", 64), Source: Source{ID: "builtin"}})
			}
			writeEnvelope(response, map[string]any{"list": items, "total": len(items), "revision": current})
		}
	}))
	defer server.Close()
	client, err := NewClient(ClientConfig{BaseURL: server.URL, CatalogKey: "key", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := client.Catalog(context.Background())
	if err != nil || len(initial.Templates) != 1 {
		t.Fatalf("initial catalog=%+v err=%v", initial, err)
	}
	revision.Store(2)
	cached, err := client.Catalog(context.Background())
	if err != nil || cached.Revision != 1 {
		t.Fatalf("cached catalog=%+v err=%v", cached, err)
	}
	refreshed, err := client.Refresh(context.Background())
	if err != nil || refreshed.Revision != 2 || len(refreshed.Templates) != 0 {
		t.Fatalf("refreshed catalog=%+v err=%v", refreshed, err)
	}
}

func TestCatalogFormatChangeUsesLastSuccessfulSnapshot(t *testing.T) {
	var invalid atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if invalid.Load() {
			writeEnvelope(response, map[string]any{"list": []map[string]any{{"id": "broken"}}, "total": 1, "revision": 2})
			return
		}
		switch request.URL.Path {
		case "/api/v1/catalog/sources":
			writeEnvelope(response, map[string]any{"list": []Source{{ID: "builtin", Name: "内置软件源", Type: "builtin", State: "ok"}}, "revision": 1})
		case "/api/v1/catalog/templates":
			writeEnvelope(response, map[string]any{"list": []Template{}, "total": 0, "revision": 1})
		}
	}))
	defer server.Close()
	client, err := NewClient(ClientConfig{BaseURL: server.URL, CatalogKey: "key", Timeout: time.Second, StaleTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Catalog(context.Background()); err != nil {
		t.Fatal(err)
	}
	invalid.Store(true)
	client.mutex.Lock()
	client.cached.FetchedAt = time.Now().Add(-catalogCacheTTL - time.Second)
	client.mutex.Unlock()
	stale, err := client.Catalog(context.Background())
	if err != nil || stale.Revision != 1 {
		t.Fatalf("format-change fallback=%+v err=%v", stale, err)
	}
	emptyClient, err := NewClient(ClientConfig{BaseURL: server.URL, CatalogKey: "key", Timeout: time.Second, StaleTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := emptyClient.Catalog(context.Background()); err == nil || !strings.Contains(err.Error(), ErrUnavailable.Error()) {
		t.Fatalf("invalid initial catalog error=%v", err)
	}
}

func writeEnvelope(response http.ResponseWriter, data any) {
	_ = json.NewEncoder(response).Encode(map[string]any{"code": 200, "msg": "", "data": data})
}

func TestAuthProPlugCatalogRefreshContentAndPreviewContract(t *testing.T) {
	const key = "test-auth-pro-plug-read-only-key-32"
	payload := []byte(`{"schemaVersion":1,"stylePreset":"fintech-gold","hero":{"title":"黑金金融科技"}}`)
	source := Source{ID: "auth-pro-plug", Name: "Auth Pro 模板中心", Type: "json", State: "ok"}
	var revision atomic.Int64
	var requests atomic.Int32
	revision.Store(1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Header.Get("X-Software-Source-Key") != key {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/api/v1/catalog/sources":
			writeEnvelope(response, map[string]any{"list": []Source{source}, "revision": revision.Load()})
		case "/api/v1/catalog/templates":
			item := Template{ID: "9", TemplateKey: "fintech-gold", Name: "黑金金融科技", Version: "1.0.0", SchemaVersion: 1, SHA256: sha256Hex(payload), Source: source, Available: true, Published: true, ContentURL: "/api/v1/catalog/templates/9/content", PreviewURL: "/api/v1/catalog/templates/9/preview"}
			writeEnvelope(response, map[string]any{"list": []Template{item}, "total": 1, "revision": revision.Load()})
		case "/api/v1/catalog/templates/9/content":
			response.Header().Set("Content-Type", "application/json")
			response.Header().Set("X-Checksum-SHA256", sha256Hex(payload))
			_, _ = response.Write(payload)
		case "/api/v1/catalog/templates/9/preview":
			response.Header().Set("Content-Type", "image/jpeg")
			_, _ = response.Write([]byte("bounded-preview"))
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := NewClient(ClientConfig{BaseURL: server.URL, CatalogKey: key, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.Catalog(context.Background())
	if err != nil || first.Revision != 1 || len(first.Templates) != 1 {
		t.Fatalf("first catalog: %+v, %v", first, err)
	}
	revision.Store(2)
	cached, err := client.Catalog(context.Background())
	if err != nil || cached.Revision != 1 || requests.Load() != 2 {
		t.Fatalf("cache not retained: %+v, %v", cached, err)
	}
	fresh, err := client.Refresh(context.Background())
	if err != nil || fresh.Revision != 2 || requests.Load() != 4 {
		t.Fatalf("manual refresh must bypass TTL: %+v, %v", fresh, err)
	}
	content, err := client.TemplateContent(context.Background(), fresh.Templates[0])
	if err != nil || string(content) != string(payload) {
		t.Fatalf("download/checksum: %v", err)
	}
	preview, mime, err := client.TemplatePreview(context.Background(), "9")
	if err != nil || mime != "image/jpeg" || string(preview) != "bounded-preview" {
		t.Fatalf("preview proxy: %v", err)
	}
}
