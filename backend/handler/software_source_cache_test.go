package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"auto_pro/config"
	"auto_pro/softwaresource"

	"github.com/gin-gonic/gin"
)

func TestInternalSoftwareSourceCacheInvalidate(t *testing.T) {
	// 目录 Key 已内置于后端，测试直接使用 config 中的固定值。
	handlerKey := config.GetSoftwareSourceAPIKey()
	var requests atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		response.Header().Set("Content-Type", "application/json")
		if request.Header.Get("X-Software-Source-Key") != "catalog-key" {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		data := `{"list":[],"revision":9}`
		if request.URL.Path == "/api/v1/catalog/templates" {
			data = `{"list":[],"total":0,"revision":9}`
		}
		_, _ = response.Write([]byte(`{"code":200,"data":` + data + `}`))
	}))
	defer remote.Close()
	client, err := softwaresource.NewClient(softwaresource.ClientConfig{BaseURL: remote.URL, CatalogKey: "catalog-key", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	restore := softwaresource.SetDefaultForTest(client)
	defer restore()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/internal/software-source/cache/invalidate", InternalSoftwareSourceCacheInvalidate)
	unauthorized := httptest.NewRequest(http.MethodPost, "/api/internal/software-source/cache/invalidate", nil)
	unauthorizedRecorder := httptest.NewRecorder()
	router.ServeHTTP(unauthorizedRecorder, unauthorized)
	if unauthorizedRecorder.Code != http.StatusUnauthorized || requests.Load() != 0 {
		t.Fatalf("unauthorized response=%d requests=%d", unauthorizedRecorder.Code, requests.Load())
	}

	request := httptest.NewRequest(http.MethodPost, "/api/internal/software-source/cache/invalidate", nil)
	request.Header.Set("X-Software-Source-Key", handlerKey)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"revision":9`) || requests.Load() != 2 {
		t.Fatalf("response=%d body=%s requests=%d", recorder.Code, recorder.Body.String(), requests.Load())
	}
}
