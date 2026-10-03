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

// 守护方式结束了更新脚本、没写结果文件时，新版本起来后要把停在 restarting 的任务记成完成；
// 目标版本不是当前版本的任务不动，留给更新脚本写回滚结果。
func TestSettleOnlineUpdateJobsAfterRestart(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AUTO_PRO_DATA_DIR", dataDir)
	updates := filepath.Join(dataDir, "updates")
	if err := os.MkdirAll(updates, 0755); err != nil {
		t.Fatal(err)
	}
	write := func(id, status, version string) {
		raw, err := json.Marshal(onlineUpdateJob{ID: id, Status: status, Version: version, Progress: 95, Logs: []string{}, UpdatedAt: time.Now().Add(-time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(updates, id+".json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("Udone", "restarting", "v"+config.AppVersion)
	write("Uother", "restarting", "9.9.9")
	SettleOnlineUpdateJobsAfterRestart()

	done := loadOnlineUpdateJob("Udone")
	if done == nil || done.Status != "success" || done.Progress != 100 || len(done.Logs) != 1 {
		t.Fatalf("目标版本已在运行，任务应记成完成: %+v", done)
	}
	if other := loadOnlineUpdateJob("Uother"); other == nil || other.Status != "restarting" {
		t.Fatalf("目标版本不同的任务不应改动: %+v", other)
	}
	if hint := latestOnlineUpdateHint(); hint == nil || hint.JobID != "Udone" || hint.Status != "success" {
		t.Fatalf("版本接口应报告完成: %+v", hint)
	}
}
