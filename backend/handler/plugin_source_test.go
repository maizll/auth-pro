package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPluginSourceManifestIgnoresHomeTemplates(t *testing.T) {
	index, err := parsePluginSourceManifest([]byte(`{"name":"Plugins","plugins":[{"id":"demo-plugin","name":"Demo","version":"1.0.0"}],"homeTemplates":[{"ignored":true}]}`))
	if err != nil || len(index.Plugins) != 1 || len(index.HomeTemplates) != 1 {
		t.Fatalf("index=%+v err=%v", index, err)
	}
}

func TestFetchPluginSourceManifestJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = response.Write([]byte(`{"name":"Plugins","plugins":[{"id":"demo-plugin","name":"Demo","version":"1.0.0"}]}`))
	}))
	defer server.Close()
	index, _, sourceType, err := fetchPluginSourceManifest(context.Background(), server.URL+"/index.json")
	if err != nil || sourceType != "json" || len(index.Plugins) != 1 {
		t.Fatalf("type=%q index=%+v err=%v", sourceType, index, err)
	}
}
