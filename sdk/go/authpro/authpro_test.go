package authpro_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	authpro "github.com/maizll/auth-pro/sdk/go/authpro"
)

func TestBootVerifyAgainstStub(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/license/verify":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 200,
				"msg":  "",
				"data": map[string]any{"result": "pass"},
			})
		case r.URL.Path == "/api/v1/public/advertisements":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 200,
				"data": map[string]any{"records": []any{}, "placeholder": nil},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	raw, _ := json.Marshal(map[string]any{
		"baseUrl":   server.URL,
		"appId":     1,
		"appKey":    "demo",
		"appSecret": "secret",
		"modules":   map[string]bool{"license": true, "ads": true, "plugin_source": true},
	})
	if err := os.WriteFile(cfgPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	client := &authpro.Client{}
	if err := client.Boot(cfgPath); err != nil {
		t.Fatal(err)
	}
	result, err := client.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.OK {
		t.Fatalf("result=%+v", result)
	}
	ads, err := client.Ads("home-banner")
	if err != nil {
		t.Fatal(err)
	}
	if ads == nil {
		t.Fatal("ads nil")
	}
	url, err := client.PluginSourceURL()
	if err != nil {
		t.Fatal(err)
	}
	if url != server.URL+"/software-source/demo/index.json" {
		t.Fatalf("plugin url=%q", url)
	}
}
