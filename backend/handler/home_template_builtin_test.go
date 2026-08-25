package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"auto_pro/softwaresource"

	"github.com/gin-gonic/gin"
)

func TestBuiltinDefaultPreview(t *testing.T) {
	payload, contentType, err := builtinPreviewPayload(builtinDefaultPreviewFile)
	if err != nil || contentType != "image/svg+xml" || !strings.Contains(string(payload), "<svg") {
		t.Fatalf("default preview type=%q err=%v", contentType, err)
	}
	if _, _, err := builtinPreviewPayload("fintech-gold-home.svg"); err == nil {
		t.Fatal("migrated preview must not be embedded in authorization backend")
	}
}

func TestPublicSoftwareSourcePluginsUsesHTTPClient(t *testing.T) {
	useRemoteCatalogTestClient(t)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/software-source/plugins", nil)
	PublicSoftwareSourcePlugins(c)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"sourceType":"remote"`) || !strings.Contains(recorder.Body.String(), `"remote-home"`) {
		t.Fatalf("response=%s", recorder.Body.String())
	}
}

func TestPublicSoftwareSourcePreviewRejectsUnknownFile(t *testing.T) {
	useRemoteCatalogTestClient(t)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/software-source/previews/unknown.svg", nil)
	c.Params = gin.Params{{Key: "file", Value: "unknown.svg"}}
	PublicSoftwareSourcePreview(c)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d", recorder.Code)
	}
}

func TestPublicSoftwareSourcePreviewServesDefaultSVG(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, builtinDefaultPreviewURL(), nil)
	c.Params = gin.Params{{Key: "file", Value: builtinDefaultPreviewFile}}
	PublicSoftwareSourcePreview(c)
	if recorder.Code != http.StatusOK || !strings.HasPrefix(recorder.Header().Get("Content-Type"), "image/svg+xml") {
		t.Fatalf("status=%d type=%q", recorder.Code, recorder.Header().Get("Content-Type"))
	}
}

func useRemoteCatalogTestClient(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var data any
		switch request.URL.Path {
		case "/api/v1/catalog/sources":
			data = gin.H{"list": []gin.H{{"id": "builtin", "name": "内置软件源", "type": "builtin", "state": "ok"}}, "revision": 1}
		case "/api/v1/catalog/apps":
			data = gin.H{"list": []gin.H{{"id": "app-1", "appKey": "epay", "name": "易支付", "version": "1.0.0", "author": gin.H{"name": "官方"}, "source": gin.H{"id": "builtin", "name": "内置软件源", "type": "builtin"}, "available": true, "published": true, "updatedAt": time.Now()}}, "total": 1, "revision": 1}
		case "/api/v1/catalog/templates":
			data = gin.H{"list": []gin.H{{"id": "template-1", "templateKey": "remote-home", "name": "远程首页", "description": "模板", "previewUrl": "/preview", "version": "1.0.0", "author": gin.H{"name": "官方"}, "schemaVersion": 1, "sha256": strings.Repeat("a", 64), "contentUrl": "/content", "source": gin.H{"id": "builtin", "name": "内置软件源", "type": "builtin"}, "available": true, "published": true, "updatedAt": time.Now()}}, "total": 1, "revision": 1}
		default:
			response.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(response).Encode(gin.H{"code": 200, "msg": "", "data": data})
	}))
	client, err := softwaresource.NewClient(softwaresource.ClientConfig{BaseURL: server.URL, CatalogKey: "test-key", Timeout: time.Second})
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	restore := softwaresource.SetDefaultForTest(client)
	t.Cleanup(func() { restore(); server.Close() })
}
