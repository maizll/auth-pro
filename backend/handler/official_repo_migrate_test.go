package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"auto_pro/config"
)

// 官网升级到 1.8.0 后，结构迁移还没记完就去读令牌，再次进入启动流程，栈溢出。
// 这里按真实启动把存储和收费仓库都放好，确认迁移能结束，并且读仓库时结构迁移已经不在进行。
func TestOfficialStartupMigrationDoesNotRecurse(t *testing.T) {
	state := prepareOfficialStartupState(t, false)
	db := openSourceSchemaMigrateDB(t, state)
	runOfficialStartupMigration(t, db)
	if state.queryDuringMigration {
		t.Fatal("官网仓库迁移发生在结构迁移尚未结束时，会再次进入启动流程")
	}
	if !state.migrationApplied("app_repo_bindings_v1") {
		t.Fatal("建表迁移没有记上")
	}
	if !state.appRepoBound {
		t.Fatal("授权系统没有绑上已有仓库")
	}
}

// 线上已经手动补过 app_repo_bindings_v1。1.8.1 仍要按应用是否已绑定再迁一次，不能看迁移记录。
func TestOfficialStartupMigrationRunsWhenSchemaAlreadyMarked(t *testing.T) {
	state := prepareOfficialStartupState(t, true)
	db := openSourceSchemaMigrateDB(t, state)
	runOfficialStartupMigration(t, db)
	if state.queryDuringMigration {
		t.Fatal("补跑官网迁移时又进了结构迁移")
	}
	if !state.appRepoBound {
		t.Fatal("迁移记录已存在时没有按未绑定的授权系统再迁一次")
	}
}

func TestOfficialMigrationDoesNotBindWhenReleaseListFails(t *testing.T) {
	cases := []struct {
		name   string
		status int
		reason string
	}{
		{name: "forbidden", status: http.StatusForbidden, reason: "HTTP 403"},
		{name: "server", status: http.StatusInternalServerError, reason: "HTTP 500"},
		{name: "unauthorized", status: http.StatusUnauthorized, reason: "HTTP 401"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := prepareOfficialStartupState(t, false)
			db := openSourceSchemaMigrateDB(t, state)
			runOfficialStartupMigrationStatus(t, db, tc.status)
			if state.appRepoBound {
				t.Fatal("列出 Release 失败时不应绑定")
			}
			if !auditHas(state, "migrate_failed", tc.reason) {
				t.Fatalf("审计 = %#v，缺少失败原因 %s", state.audits, tc.reason)
			}
		})
	}
}

func TestOfficialMigrationDoesNotBindWhenGitHubUnreachable(t *testing.T) {
	state := prepareOfficialStartupState(t, false)
	db := openSourceSchemaMigrateDB(t, state)
	runOfficialStartupMigrationAt(t, db, "http://127.0.0.1:1")
	if state.appRepoBound {
		t.Fatal("连不上托管站时不应绑定")
	}
	if !auditHas(state, "migrate_failed", "网络错误") {
		t.Fatalf("审计 = %#v，缺少网络错误", state.audits)
	}
}

func TestOfficialMigrationSkipsMissingRepository(t *testing.T) {
	state := prepareOfficialStartupState(t, false)
	db := openSourceSchemaMigrateDB(t, state)
	runOfficialStartupMigrationStatus(t, db, http.StatusNotFound)
	if !state.appRepoBound {
		t.Fatal("源仓库不存在时应跳过复制并完成绑定")
	}
	if auditHas(state, "migrate_failed", "") {
		t.Fatalf("仓库不存在不应记失败审计：%#v", state.audits)
	}
}

func TestOfficialMigrationRetriesAfterListFailure(t *testing.T) {
	state := prepareOfficialStartupState(t, false)
	db := openSourceSchemaMigrateDB(t, state)
	runOfficialStartupMigrationStatus(t, db, http.StatusForbidden)
	if state.appRepoBound {
		t.Fatal("第一次列出失败不应绑定")
	}
	runOfficialStartupMigration(t, db)
	if !state.appRepoBound {
		t.Fatal("托管站恢复后再次启动应完成绑定")
	}
	if !auditHas(state, "migrate", "已迁入") {
		t.Fatalf("恢复后审计 = %#v", state.audits)
	}
}

func TestSourceStationMigrationReentryReturns(t *testing.T) {
	state := newLegacySourceSchemaState()
	db := openSourceSchemaMigrateDB(t, state)
	sourceMigrationMu.Lock()
	sourceMigrationRunning = true
	sourceMigrationMu.Unlock()
	t.Cleanup(func() {
		sourceMigrationMu.Lock()
		sourceMigrationRunning = false
		sourceMigrationMu.Unlock()
	})
	if err := ensureSourceStationMigrations(db); err != nil {
		t.Fatal(err)
	}
	if state.migrationApplied("app_repo_bindings_v1") || len(state.allExecs()) != 0 {
		t.Fatal("正在迁移时再次进入不应执行建表")
	}
}

func prepareOfficialStartupState(t *testing.T, schemaAlreadyMarked bool) *sourceSchemaMigrateState {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	previous := embeddedStoreSnapshotPublicKey
	embeddedStoreSnapshotPublicKey = base64.StdEncoding.EncodeToString(publicKey)
	t.Cleanup(func() { embeddedStoreSnapshotPublicKey = previous })
	writeSnapshotKey(t, privateKey)
	if !officialSite() {
		t.Fatal("测试没有进入官网")
	}
	sealed, err := sealGitHubPaidToken("ghp_official_startup")
	if err != nil {
		t.Fatal(err)
	}
	state := newLegacySourceSchemaState()
	state.officialAppID = 7
	state.officialAppName = "授权系统"
	state.stationSettings = map[string]string{
		githubPaidOwnerSettingKey: "acme",
		githubPaidRepoSettingKey:  "paid-packages",
		githubPaidTokenSettingKey: sealed,
	}
	if schemaAlreadyMarked {
		state.migrations = map[string]bool{"app_repo_bindings_v1": true}
	}
	return state
}

func auditHas(state *sourceSchemaMigrateState, action, reason string) bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, line := range state.audits {
		if strings.Contains(line, action) && strings.Contains(line, reason) {
			return true
		}
	}
	return false
}

func runOfficialStartupMigration(t *testing.T, db *sql.DB) {
	t.Helper()
	runOfficialStartupMigrationStatus(t, db, http.StatusOK)
}

func runOfficialStartupMigrationStatus(t *testing.T, db *sql.DB, status int) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			http.Error(w, "nope", status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	t.Cleanup(upstream.Close)
	runOfficialStartupMigrationAt(t, db, upstream.URL)
}

func runOfficialStartupMigrationAt(t *testing.T, db *sql.DB, apiRoot string) {
	t.Helper()
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })
	restoreStore := SetSourceStationStoreForTest(mysqlSourceStore{})
	t.Cleanup(restoreStore)
	previousAPI := productUpdateGitHubAPI
	productUpdateGitHubAPI = apiRoot
	t.Cleanup(func() { productUpdateGitHubAPI = previousAPI })
	officialAppRepoMigrateOnce = sync.Once{}
	officialAppRepoMigrationWait = nil
	t.Cleanup(func() {
		officialAppRepoMigrateOnce = sync.Once{}
		officialAppRepoMigrationWait = nil
	})
	if err := ensureSourceStationStorage(db); err != nil {
		t.Fatalf("启动迁移: %v", err)
	}
	// 和 main 一样：结构迁移返回之后才安排官网仓库迁移。
	ScheduleOfficialAppRepoMigration()
	wait := officialAppRepoMigrationWait
	if wait == nil {
		t.Fatal("官网没有在结构迁移之后安排仓库迁移")
	}
	select {
	case <-wait:
	case <-time.After(10 * time.Second):
		t.Fatal("官网仓库迁移没有结束")
	}
}
