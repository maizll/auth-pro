package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

func TestSystemVersionReportsRollbackAndDisablesCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dataDir := t.TempDir()
	t.Setenv("AUTO_PRO_DATA_DIR", dataDir)
	if err := os.MkdirAll(filepath.Join(dataDir, "updates"), 0755); err != nil {
		t.Fatal(err)
	}
	job := onlineUpdateJob{
		ID:        "Urollback",
		Status:    "restarting",
		Message:   "服务正在切换并重启",
		Version:   "9.9.9",
		CreatedAt: time.Now().Add(-time.Minute),
		UpdatedAt: time.Now().Add(-time.Minute),
	}
	raw, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	jobPath := filepath.Join(dataDir, "updates", "Urollback.json")
	if err := os.WriteFile(jobPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	reason := "新版本没有健康启动，已回滚到更新前的版本。旧版本已重新拉起。"
	if err := os.WriteFile(jobPath+".result", []byte("failed\n"+reason+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/system/version?_=1", nil)
	SystemVersion(ctx)
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, `"version":"`+config.AppVersion+`"`) {
		t.Fatalf("version body=%s", body)
	}
	if !strings.Contains(recorder.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("Cache-Control=%q", recorder.Header().Get("Cache-Control"))
	}
	if !strings.Contains(body, `"rolledBack":true`) || !strings.Contains(body, reason) {
		t.Fatalf("rollback hint missing: %s", body)
	}
}
