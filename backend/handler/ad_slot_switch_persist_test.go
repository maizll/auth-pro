package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

func prepareAdSlotSwitchTestDB(t *testing.T, db *sql.DB) {
	t.Helper()
	// ensureSourceStationStorage 会跑目录迁移，需要空 apps 表；开关本身只写 settings。
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS apps (
			id BIGINT UNSIGNED PRIMARY KEY,
			app_name VARCHAR(100) NOT NULL DEFAULT '',
			app_key VARCHAR(64) NOT NULL DEFAULT '',
			enabled TINYINT NOT NULL DEFAULT 1,
			deleted_at DATETIME NULL
		)`,
		`CREATE TABLE IF NOT EXISTS source_station_settings (
			setting_key VARCHAR(50) NOT NULL PRIMARY KEY,
			setting_value TEXT NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
}

func resetAdSlotEnabledDefaults() {
	adSlotEnabledL.Lock()
	defer adSlotEnabledL.Unlock()
	for _, slot := range adSlotCatalog {
		adSlotEnabled[slot.ID] = true
	}
}

func openAdSlotSwitchTestDB(t *testing.T, suffix string) *sql.DB {
	t.Helper()
	control := openAppUpdateControlDB(t)
	t.Cleanup(func() { control.Close() })
	databaseName := "authpro_adslot_" + suffix + "_" + strconv.Itoa(os.Getpid())
	if _, err := control.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.Exec("DROP DATABASE `" + databaseName + "`") })
	db := openAppUpdateDatabase(t, databaseName)
	t.Cleanup(func() { _ = db.Close() })
	prepareAdSlotSwitchTestDB(t, db)
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })
	t.Cleanup(resetAdSlotEnabledDefaults)
	return db
}

func TestAdSlotSwitchPersistsAcrossReload(t *testing.T) {
	_ = openAdSlotSwitchTestDB(t, "persist")

	if err := switchAdSlotEnabled(adSlotConsoleSidebar, false); err != nil {
		t.Fatalf("switch off: %v", err)
	}
	if isAdSlotEnabled(adSlotConsoleSidebar) {
		t.Fatal("memory should be off after switch")
	}

	resetAdSlotEnabledDefaults()
	loadAdSlotSwitchesFromStore()
	if isAdSlotEnabled(adSlotConsoleSidebar) {
		t.Fatal("reload from DB must keep console-sidebar off")
	}
	if isAdSlotEnabled(adSlotSidebarLegacy) {
		t.Fatal("legacy sidebar alias must stay off with console-sidebar")
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/source/admin/ad-slots", nil)
	AdminAdSlotOverview(c)
	var body struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				ID      string `json:"id"`
				Enabled bool   `json:"enabled"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 200 {
		t.Fatalf("overview code=%d body=%s", body.Code, recorder.Body.String())
	}
	found := false
	for _, row := range body.Data.List {
		if row.ID == adSlotConsoleSidebar {
			found = true
			if row.Enabled {
				t.Fatal("overview must report console-sidebar disabled")
			}
		}
	}
	if !found {
		t.Fatal("console-sidebar missing from overview")
	}
}

func TestAdSlotSwitchSurvivesConcurrentPublicLoad(t *testing.T) {
	_ = openAdSlotSwitchTestDB(t, "race")

	if err := switchAdSlotEnabled(adSlotConsoleSidebar, true); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			loadAdSlotSwitchesFromStore()
			_ = isAdSlotEnabled(adSlotConsoleSidebar)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := switchAdSlotEnabled(adSlotConsoleSidebar, false); err != nil {
			t.Errorf("switch: %v", err)
		}
	}()
	wg.Wait()

	resetAdSlotEnabledDefaults()
	loadAdSlotSwitchesFromStore()
	if isAdSlotEnabled(adSlotConsoleSidebar) {
		t.Fatal("concurrent public loads must not overwrite an in-flight disable")
	}
}

func TestAdminAdSlotSwitchAPIRoundTrip(t *testing.T) {
	_ = openAdSlotSwitchTestDB(t, "api")

	gin.SetMode(gin.TestMode)
	body, _ := json.Marshal(map[string]any{"enabled": false})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: adSlotConsoleSidebar}}
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/source/admin/ad-slots/"+adSlotConsoleSidebar, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	AdminAdSlotSwitch(c)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Code != 200 || resp.Data.Enabled {
		t.Fatalf("switch api resp=%s", recorder.Body.String())
	}

	resetAdSlotEnabledDefaults()
	loadAdSlotSwitchesFromStore()
	if isAdSlotEnabled(adSlotConsoleSidebar) {
		t.Fatal("API disable must persist for subsequent loads")
	}
}
