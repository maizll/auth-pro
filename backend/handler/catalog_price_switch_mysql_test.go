package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

func TestCatalogPriceSwitchMariaDB(t *testing.T) {
	control := openAppUpdateControlDB(t)
	defer control.Close()
	databaseName := "authpro_pricesw_" + strconv.Itoa(os.Getpid())
	if _, err := control.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.Exec("DROP DATABASE `" + databaseName + "`") })
	db := openAppUpdateDatabase(t, databaseName)
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE apps (
			id BIGINT UNSIGNED PRIMARY KEY,
			app_name VARCHAR(100) NOT NULL,
			app_key VARCHAR(64) NOT NULL,
			enabled TINYINT NOT NULL DEFAULT 1,
			deleted_at DATETIME NULL
		);
		CREATE TABLE roles (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			role_name VARCHAR(100) NOT NULL,
			role_code VARCHAR(100) NOT NULL,
			description VARCHAR(255) NOT NULL DEFAULT '',
			discount DECIMAL(10,2) NOT NULL DEFAULT 0,
			enabled TINYINT NOT NULL DEFAULT 1
		);
		CREATE TABLE agents (
			id BIGINT PRIMARY KEY,
			email VARCHAR(100) NOT NULL DEFAULT ''
		);
		CREATE TABLE licenses (
			id BIGINT PRIMARY KEY,
			license_no VARCHAR(64) NOT NULL,
			app_id BIGINT UNSIGNED NOT NULL,
			status VARCHAR(20) NOT NULL,
			owner_type VARCHAR(20) NOT NULL,
			owner_id BIGINT NOT NULL,
			source ENUM('admin','agent','user_purchase','card') NOT NULL DEFAULT 'admin'
		);
		INSERT INTO apps (id, app_name, app_key) VALUES (1, '演示应用', 'app-a');
		INSERT INTO licenses (id, license_no, app_id, status, owner_type, owner_id) VALUES
			(11, 'LIC-11', 1, 'active', 'user', 7),
			(12, 'LIC-12', 1, 'active', 'agent', 3),
			(13, 'LIC-13', 1, 'active', 'user', 8),
			(21, 'LIC-21', 1, 'active', 'user', 9);
	`); err != nil {
		t.Fatal(err)
	}
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })
	dataDir := t.TempDir()
	t.Setenv("AUTO_PRO_DATA_DIR", dataDir)
	if err := ensureSourceStationStorage(db); err != nil {
		t.Fatal(err)
	}

	payload := sourcePluginTestZIP(t)
	publicURL, sha, err := storeStationPackage(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO source_catalog_plugins
		(id, app_id, name, version, sha256, download_url, status, latest_version, price_cents, billing)
		VALUES ('keep-free', 1, '老用户免费', '1.0.0', ?, ?, 'published', '1.0.0', 0, 'free')`, sha, publicURL); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO source_catalog_plugin_versions
		(plugin_id, version, download_url, sha256, status) VALUES ('keep-free', '1.0.0', ?, ?, 'published')`, publicURL, sha); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO source_catalog_downloads
		(item_kind, item_id, license_id, owner_type, owner_id) VALUES
		('plugin', 'keep-free', 11, 'user', 7),
		('plugin', 'keep-free', 0, 'agent', 3),
		('plugin', 'keep-free', 11, 'user', 7)`); err != nil {
		t.Fatal(err)
	}

	everyoneURL, everyoneSHA, err := storeStationPackage(sourcePluginTestZIP(t))
	if err != nil {
		t.Fatal(err)
	}
	if everyoneURL == publicURL {
		everyonePayload := makeTestZIP(t, testZIPEntry{name: "everyone/plugin.json", data: `{"id":"everyone","name":"全员购买","version":"1.0.0","category":"other","author":{"name":"站长"}}`})
		everyoneURL, everyoneSHA, err = storeStationPackage(everyonePayload)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO source_catalog_plugins
		(id, app_id, name, version, sha256, download_url, status, latest_version, price_cents, billing)
		VALUES ('everyone', 1, '全员购买', '1.0.0', ?, ?, 'published', '1.0.0', 0, 'free')`, everyoneSHA, everyoneURL); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO source_catalog_downloads (item_kind, item_id, license_id, owner_type, owner_id)
		VALUES ('plugin', 'everyone', 21, 'user', 9)`); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	first := callCatalogPriceUpdate(t, "keep-free", `{"name":"老用户免费","category":"other","version":"1.0.0","downloadUrl":"`+publicURL+`","sha256":"`+sha+`","priceCents":1990,"priceSwitch":"grandfather"}`)
	if first["code"] != float64(200) {
		t.Fatalf("grandfather switch: %#v", first)
	}
	again := callCatalogPriceUpdate(t, "keep-free", `{"name":"老用户免费","category":"other","version":"1.0.0","downloadUrl":"`+publicURL+`","sha256":"`+sha+`","priceCents":1990,"priceSwitch":"grandfather"}`)
	if again["code"] != float64(200) {
		t.Fatalf("repeat switch: %#v", again)
	}
	var entitled int
	if err := db.QueryRow(`SELECT COUNT(*) FROM plugin_entitlements WHERE item_kind='plugin' AND item_id='keep-free' AND source='grandfather' AND status='active'`).Scan(&entitled); err != nil {
		t.Fatal(err)
	}
	if entitled != 2 {
		t.Fatalf("grandfather entitlements=%d, want 2", entitled)
	}
	var outsider int
	if err := db.QueryRow(`SELECT COUNT(*) FROM plugin_entitlements WHERE license_id=13 AND item_id='keep-free'`).Scan(&outsider); err != nil {
		t.Fatal(err)
	}
	if outsider != 0 {
		t.Fatalf("license without a download was granted")
	}
	item, err := (mysqlSourceStore{}).GetPlugin("keep-free")
	if err != nil {
		t.Fatal(err)
	}
	if !isPrivatePackageRef(item.DownloadURL) && !isGitHubPackageRef(item.DownloadURL) {
		t.Fatalf("package was not moved: %#v", item)
	}
	if _, statErr := os.Stat(stationPackagePath(publicURL)); !os.IsNotExist(statErr) {
		t.Fatalf("old public file remains: %v", statErr)
	}
	var versionURL string
	if err := db.QueryRow(`SELECT download_url FROM source_catalog_plugin_versions WHERE plugin_id='keep-free' AND version='1.0.0'`).Scan(&versionURL); err != nil {
		t.Fatal(err)
	}
	if versionURL != item.DownloadURL {
		t.Fatalf("version url=%s item=%s", versionURL, item.DownloadURL)
	}
	if licenseCanDownloadPaid(db, 11, "plugin", "keep-free") != true {
		t.Fatal("grandfather license cannot download")
	}
	if licenseCanDownloadPaid(db, 13, "plugin", "keep-free") {
		t.Fatal("license without a download can download")
	}

	second := callCatalogPriceUpdate(t, "everyone", `{"name":"全员购买","category":"other","version":"1.0.0","downloadUrl":"`+everyoneURL+`","sha256":"`+everyoneSHA+`","priceCents":500,"priceSwitch":"purchase_only"}`)
	if second["code"] != float64(200) {
		t.Fatalf("purchase-only switch: %#v", second)
	}
	var everyoneCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM plugin_entitlements WHERE item_id='everyone' AND status='active'`).Scan(&everyoneCount); err != nil {
		t.Fatal(err)
	}
	if everyoneCount != 0 {
		t.Fatalf("purchase-only granted %d", everyoneCount)
	}
	if _, err := db.Exec(`INSERT INTO main_license_editions (license_id, edition, period, status, started_at) VALUES (21, 'commercial', 'permanent', 'active', NOW())`); err != nil {
		t.Fatal(err)
	}
	if licenseCanDownloadPaid(db, 21, "plugin", "everyone") {
		t.Fatal("commercial license bypassed purchase-only")
	}
	if !licenseCanDownloadPaid(db, 11, "plugin", "keep-free") {
		t.Fatal("grandfather download broke after the other switch")
	}
	if _, statErr := os.Stat(stationPackagePath(everyoneURL)); !os.IsNotExist(statErr) {
		t.Fatalf("everyone public file remains: %v", statErr)
	}

	back := callCatalogPriceUpdate(t, "keep-free", `{"name":"老用户免费","category":"other","version":"1.0.0","downloadUrl":"`+item.DownloadURL+`","sha256":"`+item.SHA256+`","priceCents":0}`)
	if back["code"] != float64(200) {
		t.Fatalf("revert: %#v", back)
	}
	restored, err := (mysqlSourceStore{}).GetPlugin("keep-free")
	if err != nil {
		t.Fatal(err)
	}
	if restored.PriceCents != 0 || !isStationHostedPackageURL(restored.DownloadURL) {
		t.Fatalf("restored=%#v old=%s", restored, publicURL)
	}
	if _, statErr := os.Stat(stationPackagePath(restored.DownloadURL)); statErr != nil {
		t.Fatalf("new public file missing: %v", statErr)
	}
	var audits int
	if err := db.QueryRow(`SELECT COUNT(*) FROM source_audit_logs WHERE target_id='keep-free' AND action IN ('price_to_paid','price_to_free')`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits < 2 {
		t.Fatalf("audit rows=%d", audits)
	}
}

func callCatalogPriceUpdate(t *testing.T, id, body string) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/plugins/"+id, bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: id}}
	c.Set("username", "root")
	AdminSourceUpdatePlugin(c)
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response %s: %v", w.Body.String(), err)
	}
	if payload["code"] != float64(200) && !strings.Contains(w.Body.String(), "老用户") {
		t.Log(w.Body.String())
	}
	return payload
}
