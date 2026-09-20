package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"auto_pro/appstore"
	"auto_pro/softwaresource"
)

func TestValidateHomeTemplateDocument(t *testing.T) {
	if err := validateHomeTemplateDocument([]byte(`{"schemaVersion":1,"hero":{"title":"首页"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := validateHomeTemplateDocument([]byte(`{"schemaVersion":1,"hero":{"title":"首页"},"scripts":[]}`)); err == nil {
		t.Fatal("scripts should be rejected")
	}
	if err := validateHomeTemplateDocument([]byte(`{"schemaVersion":2,"hero":{"title":"首页"}}`)); err == nil {
		t.Fatal("unsupported schema should be rejected")
	}
}

func TestAdminHomeTemplateListRefreshWithoutRemoteSourceOmitsWarning(t *testing.T) {
	t.Setenv("AUTO_PRO_SOFTWARE_SOURCE_URL", "")
	t.Setenv("AUTO_PRO_SOFTWARE_SOURCE_API_KEY", "")
	restoreDefault := softwaresource.SetDefaultForTest(nil)
	t.Cleanup(restoreDefault)
	stubHomeTemplateList(t)

	recorder := invokeHandler(t, http.MethodGet, "/api/system/home-templates?refresh=1", nil, nil, AdminHomeTemplateList)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	envelope := decodeHomeTemplateList(t, recorder.Body.Bytes())
	if envelope.Code != 200 {
		t.Fatalf("code=%d body=%s", envelope.Code, recorder.Body.String())
	}
	if len(envelope.Data.List) == 0 {
		t.Fatalf("expected a template list, got %s", recorder.Body.String())
	}
	if warning := strings.TrimSpace(envelope.Data.Warning); warning != "" {
		t.Fatalf("unconfigured software source must not warn on refresh, got %q", warning)
	}
}

func TestAdminHomeTemplateListRefreshKeepsSoftWarningWhenRemoteFails(t *testing.T) {
	t.Setenv("AUTO_PRO_SOFTWARE_SOURCE_URL", "https://source.example.test")
	t.Setenv("AUTO_PRO_SOFTWARE_SOURCE_API_KEY", "catalog-key")
	remote := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusBadGateway)
	}))
	defer remote.Close()
	client, err := softwaresource.NewClient(softwaresource.ClientConfig{
		BaseURL: remote.URL, CatalogKey: "catalog-key", Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	restoreDefault := softwaresource.SetDefaultForTest(client)
	t.Cleanup(restoreDefault)
	stubHomeTemplateList(t)

	recorder := invokeHandler(t, http.MethodGet, "/api/system/home-templates?refresh=1", nil, nil, AdminHomeTemplateList)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	envelope := decodeHomeTemplateList(t, recorder.Body.Bytes())
	if envelope.Code != 200 || len(envelope.Data.List) == 0 {
		t.Fatalf("refresh failure must still return the list: %s", recorder.Body.String())
	}
	warning := envelope.Data.Warning
	if !strings.Contains(warning, "远程") || !strings.Contains(warning, "本站上传与源站模板仍可使用") {
		t.Fatalf("expected a soft remote refresh warning, got %q", warning)
	}
}

func stubHomeTemplateList(t *testing.T) {
	t.Helper()
	previous := listHomeTemplates
	listHomeTemplates = func(context.Context) ([]appstore.Template, error) {
		return []appstore.Template{{
			ID: "default", TemplateID: "default", Name: "默认首页模板",
			Source: "授权系统本地", SourceType: "builtin", Available: true, Installed: true, Enabled: true,
		}}, nil
	}
	t.Cleanup(func() { listHomeTemplates = previous })
}

func decodeHomeTemplateList(t *testing.T, payload []byte) struct {
	Code int `json:"code"`
	Data struct {
		List    []map[string]any `json:"list"`
		Warning string           `json:"warning"`
	} `json:"data"`
} {
	t.Helper()
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			List    []map[string]any `json:"list"`
			Warning string           `json:"warning"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope
}
