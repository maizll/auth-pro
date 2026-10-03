package handler

import (
	"os"
	"strconv"
	"testing"
	"time"

	"auto_pro/config"
)

// 自动更新只在「开了开关 + 强制更新 + 凌晨 3–5 点」时启动，走同一个更新任务；结果补记操作日志，失败过的版本不再自动重试。
func TestOnlineUpdateAutoApplyRulesMySQL(t *testing.T) {
	control := openAppUpdateControlDB(t)
	t.Cleanup(func() { control.Close() })
	name := "authpro_autoupd_" + strconv.Itoa(os.Getpid())
	if _, err := control.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.Exec("DROP DATABASE `" + name + "`") })
	db := openAppUpdateDatabase(t, name)
	t.Cleanup(func() { db.Close() })
	for _, ddl := range []string{
		"CREATE TABLE system_configs (id BIGINT AUTO_INCREMENT PRIMARY KEY, `group` VARCHAR(50) NOT NULL, `key` VARCHAR(100) NOT NULL, value TEXT, description VARCHAR(255) DEFAULT '', UNIQUE KEY uk_group_key (`group`, `key`))",
		"CREATE TABLE operation_logs (id BIGINT AUTO_INCREMENT PRIMARY KEY, operator_type VARCHAR(20) NOT NULL, operator_id BIGINT UNSIGNED, action VARCHAR(100) NOT NULL, target_type VARCHAR(50), target_id BIGINT UNSIGNED, detail TEXT, ip VARCHAR(64), created_at DATETIME DEFAULT CURRENT_TIMESTAMP)",
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	previousVersion, previousLaunch, previousRuntime := config.AppVersion, launchOnlineUpdateJobAuto, onlineUpdateAutoRuntimeOK
	t.Cleanup(func() {
		config.AppVersion, launchOnlineUpdateJobAuto, onlineUpdateAutoRuntimeOK = previousVersion, previousLaunch, previousRuntime
	})
	config.AppVersion = "1.7.8"
	onlineUpdateAutoRuntimeOK = func() bool { return true }
	launched := 0
	launchOnlineUpdateJobAuto = func(manifest *onlineUpdateManifest, audit func(string)) *onlineUpdateJob {
		launched++
		job := &onlineUpdateJob{ID: "U-auto-" + strconv.Itoa(launched), Status: "running", Version: manifest.Version, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		persistOnlineUpdateJob(job)
		audit(job.ID)
		return job
	}
	manifest := validOnlineUpdateManifestForTest()
	manifest.Version, manifest.Package.FileName, manifest.Force = "1.8.9", "auth_pro-full-v1.8.9.tar.gz", true
	night := time.Date(2026, 10, 4, 3, 20, 0, 0, time.Local)
	noon := time.Date(2026, 10, 4, 12, 0, 0, 0, time.Local)
	countLogs := func(action string) int {
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM operation_logs WHERE action = ? AND operator_type = 'system'", action).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	if maybeAutoApplyOnlineUpdate(db, manifest, night) != nil {
		t.Fatal("开关默认关闭，不能自动更新")
	}
	if err := saveOnlineUpdateAutoValue(db, onlineUpdateAutoEnabledKey, "1"); err != nil {
		t.Fatal(err)
	}
	if maybeAutoApplyOnlineUpdate(db, manifest, noon) != nil {
		t.Fatal("白天不能自动更新")
	}
	optional := *manifest
	optional.Force = false
	if maybeAutoApplyOnlineUpdate(db, &optional, night) != nil {
		t.Fatal("非强制更新不能自动安装")
	}
	job := maybeAutoApplyOnlineUpdate(db, manifest, night)
	if job == nil || launched != 1 || countLogs("online_update_auto_apply") != 1 || onlineUpdateAutoValue(db, onlineUpdateAutoPendingKey) != job.ID {
		t.Fatalf("凌晨强制更新应自动启动并记日志: job=%v launched=%d", job, launched)
	}

	// 任务还在跑：不补记结果
	settleOnlineUpdateAutoResult(db)
	if countLogs("online_update_auto_result") != 0 {
		t.Fatal("任务没结束就记了结果")
	}
	// 健康检查不过、已回退：记失败结果，同一版本不再自动重试
	job.Status, job.Error = "failed", "新版本没有健康启动，已回退"
	persistOnlineUpdateJob(job)
	settleOnlineUpdateAutoResult(db)
	if countLogs("online_update_auto_result") != 1 || onlineUpdateAutoValue(db, onlineUpdateAutoPendingKey) != "" || onlineUpdateAutoValue(db, onlineUpdateAutoFailedKey) != "1.8.9" {
		t.Fatal("失败结果没有记下")
	}
	if maybeAutoApplyOnlineUpdate(db, manifest, night.Add(time.Hour)) != nil || launched != 1 {
		t.Fatal("失败过的版本不能每晚自动重试")
	}
	// 更新的版本仍会自动安装
	newer := *manifest
	newer.Version, newer.Package.FileName = "1.9.0", "auth_pro-full-v1.9.0.tar.gz"
	if maybeAutoApplyOnlineUpdate(db, &newer, night) == nil || launched != 2 {
		t.Fatal("新的强制更新应自动安装")
	}
}
