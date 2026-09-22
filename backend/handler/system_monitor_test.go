package handler

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
)

func useMonitorStateForTest(t *testing.T) {
	t.Helper()
	previous := monitorStatePathOverride
	monitorStatePathOverride = filepath.Join(t.TempDir(), "monitor.json")
	resetMonitorState()
	t.Cleanup(func() {
		monitorStatePathOverride = previous
		resetMonitorState()
	})
}

func monitorRouter(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	api.Use(middleware.JWTAuth(), middleware.RequireAdmin(), middleware.RequireSuperAdmin())
	api.GET("/system/monitor/jobs", AdminMonitorJobs)
	api.PUT("/system/monitor/jobs/:id", AdminMonitorJobUpdate)
	api.POST("/system/monitor/jobs/:id/run", AdminMonitorJobRun)
	return router, sourceAdminToken(t)
}

func TestMonitorJobsExposeFiveRealTasks(t *testing.T) {
	useMonitorStateForTest(t)
	previous := probeOnlineUpdate
	probeOnlineUpdate = func() (string, error) { return "在线更新源可访问，清单版本 1.4.10", nil }
	t.Cleanup(func() { probeOnlineUpdate = previous })

	router, token := monitorRouter(t)
	sourceStationRouter(t)
	rec := sourceJSON(t, router, http.MethodGet, "/api/system/monitor/jobs", token, "")
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("list=%s", rec.Body.String())
	}
	for _, id := range []string{monitorJobExternalURL, monitorJobLicenseVerify, monitorJobPaymentRoutes, monitorJobOnlineUpdate, monitorJobDiskUsage} {
		if !strings.Contains(rec.Body.String(), id) {
			t.Fatalf("missing %s in %s", id, rec.Body.String())
		}
	}

	disable := sourceJSON(t, router, http.MethodPut, "/api/system/monitor/jobs/"+monitorJobDiskUsage, token, `{"enabled":false}`)
	if sourceBodyCode(t, disable) != 200 || !strings.Contains(disable.Body.String(), `"id":"disk-usage"`) {
		t.Fatalf("disable=%s", disable.Body.String())
	}
	var body struct {
		Data struct {
			Jobs []monitorJobView `json:"jobs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(disable.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, job := range body.Data.Jobs {
		if job.ID == monitorJobDiskUsage {
			if job.Enabled || job.NextRunAt != "" {
				t.Fatalf("disabled job still scheduled: %+v", job)
			}
		}
	}

	run := sourceJSON(t, router, http.MethodPost, "/api/system/monitor/jobs/"+monitorJobLicenseVerify+"/run", token, "{}")
	if sourceBodyCode(t, run) != 200 || !strings.Contains(run.Body.String(), `"lastStatus":"ok"`) || !strings.Contains(run.Body.String(), "授权校验") {
		t.Fatalf("license run=%s", run.Body.String())
	}
	pay := sourceJSON(t, router, http.MethodPost, "/api/system/monitor/jobs/"+monitorJobPaymentRoutes+"/run", token, "{}")
	if sourceBodyCode(t, pay) != 200 || !strings.Contains(pay.Body.String(), "easypay/notify") {
		t.Fatalf("payment run=%s", pay.Body.String())
	}
	disk := sourceJSON(t, router, http.MethodPost, "/api/system/monitor/jobs/"+monitorJobDiskUsage+"/run", token, "{}")
	if sourceBodyCode(t, disk) != 200 || !strings.Contains(disk.Body.String(), "上传") || !strings.Contains(disk.Body.String(), "数据盘已用") {
		t.Fatalf("disk run=%s", disk.Body.String())
	}
	update := sourceJSON(t, router, http.MethodPost, "/api/system/monitor/jobs/"+monitorJobOnlineUpdate+"/run", token, "{}")
	if sourceBodyCode(t, update) != 200 || !strings.Contains(update.Body.String(), "1.4.10") {
		t.Fatalf("update run=%s", update.Body.String())
	}

	health := sourceJSON(t, router, http.MethodPost, "/api/system/monitor/jobs/"+monitorJobExternalURL+"/run", token, "{}")
	if sourceBodyCode(t, health) != 200 || !strings.Contains(health.Body.String(), "已检查") {
		t.Fatalf("health run=%s", health.Body.String())
	}

	agent := sourceAgentToken(t, 9, "agent@example.com")
	denied := sourceJSON(t, router, http.MethodGet, "/api/system/monitor/jobs", agent, "")
	if sourceBodyCode(t, denied) != 403 {
		t.Fatalf("agent must be denied: %s", denied.Body.String())
	}
}

func TestMonitorRunNowReportsFailure(t *testing.T) {
	useMonitorStateForTest(t)
	previous := probeOnlineUpdate
	probeOnlineUpdate = func() (string, error) { return "", errMonitorProbe }
	t.Cleanup(func() { probeOnlineUpdate = previous })
	router, token := monitorRouter(t)
	rec := sourceJSON(t, router, http.MethodPost, "/api/system/monitor/jobs/"+monitorJobOnlineUpdate+"/run", token, "{}")
	if sourceBodyCode(t, rec) != 200 || !strings.Contains(rec.Body.String(), `"lastStatus":"fail"`) || !strings.Contains(rec.Body.String(), "更新源拒绝") {
		t.Fatalf("fail run=%s", rec.Body.String())
	}
}

var errMonitorProbe = monitorProbeError("更新源拒绝连接")

type monitorProbeError string

func (e monitorProbeError) Error() string { return string(e) }
