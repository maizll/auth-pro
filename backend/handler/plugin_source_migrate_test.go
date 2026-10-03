package handler

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"testing"
)

var pluginSourceMigrateDBSeq int

func TestMigrateMisclassifiedJSONPluginSources(t *testing.T) {
	control := openAppUpdateControlDB(t)
	// 关连接要排在删库之后（Cleanup 后进先出），否则删库时连接已关、测试库会留下
	t.Cleanup(func() { control.Close() })

	databaseName := "authpro_plugin_src_" + strconv.Itoa(os.Getpid())
	if _, err := control.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.Exec("DROP DATABASE `" + databaseName + "`") })
	db := openAppUpdateDatabase(t, databaseName)
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE plugin_sources (
		id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(60) NOT NULL DEFAULT '',
		url VARCHAR(500) NOT NULL,
		created_at DATETIME DEFAULT NULL,
		UNIQUE KEY uk_url (url(191))
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE plugin_source_cache (
		source_id BIGINT NOT NULL PRIMARY KEY,
		source_type VARCHAR(20) NOT NULL DEFAULT 'json',
		manifest_json MEDIUMTEXT NOT NULL,
		fetched_at DATETIME NOT NULL,
		expires_at DATETIME NOT NULL,
		last_error VARCHAR(500) NOT NULL DEFAULT ''
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		name string
		url  string
		kind string
	}{
		{"site", "https://auth.example.com/software-source/app_demo/index.json", "git"},
		{"query", "https://auth.example.com/software-source/index.json?app_key=demo", "git"},
		{"slash", "https://auth.example.com/software-source/app/INDEX.JSON/", "git"},
		{"repo", "https://github.com/example/plugins.git", "git"},
		{"catalog", "https://cdn.example.com/catalog.json", "json"},
	}
	ids := map[string]int64{}
	for _, row := range rows {
		result, err := db.Exec("INSERT INTO plugin_sources (name, url, created_at) VALUES (?, ?, NOW())", row.name, row.url)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := result.LastInsertId()
		ids[row.name] = id
		if _, err := db.Exec(`INSERT INTO plugin_source_cache (source_id, source_type, manifest_json, fetched_at, expires_at)
			VALUES (?, ?, '{}', NOW(), NOW())`, id, row.kind); err != nil {
			t.Fatal(err)
		}
	}

	if err := ensurePluginSourceStorage(db); err != nil {
		t.Fatal(err)
	}
	assertPluginSourceTypes(t, db, ids, map[string]string{
		"site": "json", "query": "json", "slash": "json", "repo": "git", "catalog": "json",
	})
	if err := ensurePluginSourceStorage(db); err != nil {
		t.Fatal(err)
	}
	assertPluginSourceTypes(t, db, ids, map[string]string{
		"site": "json", "query": "json", "slash": "json", "repo": "git", "catalog": "json",
	})

	result, err := db.Exec("INSERT INTO plugin_sources (name, url, source_type, created_at) VALUES ('again', 'https://auth.example.com/again.json', 'git', NOW())")
	if err != nil {
		t.Fatal(err)
	}
	againID, _ := result.LastInsertId()
	if _, err := db.Exec(`INSERT INTO plugin_source_cache (source_id, source_type, manifest_json, fetched_at, expires_at)
		VALUES (?, 'git', '{}', NOW(), NOW())`, againID); err != nil {
		t.Fatal(err)
	}
	if err := migrateMisclassifiedJSONPluginSources(db); err != nil {
		t.Fatal(err)
	}
	ids["again"] = againID
	assertPluginSourceTypes(t, db, ids, map[string]string{
		"site": "json", "query": "json", "slash": "json", "repo": "git", "catalog": "json", "again": "json",
	})
	var cacheType string
	if err := db.QueryRow("SELECT source_type FROM plugin_source_cache WHERE source_id=?", ids["site"]).Scan(&cacheType); err != nil {
		t.Fatal(err)
	}
	if cacheType != "json" {
		t.Fatalf("cache type=%s", cacheType)
	}
	if err := db.QueryRow("SELECT source_type FROM plugin_source_cache WHERE source_id=?", ids["repo"]).Scan(&cacheType); err != nil {
		t.Fatal(err)
	}
	if cacheType != "git" {
		t.Fatalf("git cache type=%s", cacheType)
	}
}

func TestMigrateHideBuiltinPluginSource(t *testing.T) {
	db := openPluginSourceMigrateDB(t)
	result, err := db.Exec(`INSERT INTO plugin_sources (name, url, source_type, created_at) VALUES
		('旧官方', ?, 'git', NOW())`, retiredDefaultPluginSourceURL)
	if err != nil {
		t.Fatal(err)
	}
	retiredID, _ := result.LastInsertId()
	if _, err := db.Exec(`INSERT INTO plugin_source_cache (source_id, source_type, manifest_json, fetched_at, expires_at) VALUES (?, 'git', '{}', NOW(), NOW())`, retiredID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO plugin_sources (name, url, source_type, created_at) VALUES
		('旧官方斜杠', ?, 'git', NOW()),
		('当前官方', ?, 'json', NOW()),
		('http 官方', ?, 'json', NOW()),
		('自定义', 'https://cdn.example.com/mine.json', 'json', NOW()),
		('镜像', 'https://mirror.example.com/software-source/app_4e85b4724223_2603/index.json', 'json', NOW())`,
		retiredDefaultPluginSourceURL+"/", defaultPluginSourceURL, "http://auth.maizll.com/software-source/app_f93896d80066_5811/index.json"); err != nil {
		t.Fatal(err)
	}
	if err := ensurePluginSourceStorage(db); err != nil {
		t.Fatal(err)
	}
	assertNoBuiltinPluginSource(t, db)
	assertPluginSourceKept(t, db, "自定义", "https://cdn.example.com/mine.json", "json")
	assertPluginSourceKept(t, db, "镜像", "https://mirror.example.com/software-source/app_4e85b4724223_2603/index.json", "json")
	var cacheLeft int
	if err := db.QueryRow("SELECT COUNT(*) FROM plugin_source_cache WHERE source_id=?", retiredID).Scan(&cacheLeft); err != nil {
		t.Fatal(err)
	}
	if cacheLeft != 0 {
		t.Fatalf("builtin cache rows=%d", cacheLeft)
	}
	if err := ensurePluginSourceStorage(db); err != nil {
		t.Fatal(err)
	}
	assertNoBuiltinPluginSource(t, db)

	fresh := openPluginSourceMigrateDB(t)
	if err := ensurePluginSourceStorage(fresh); err != nil {
		t.Fatal(err)
	}
	assertNoBuiltinPluginSource(t, fresh)
	var total int
	if err := fresh.QueryRow("SELECT COUNT(*) FROM plugin_sources").Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Fatalf("fresh rows=%d", total)
	}
}

func TestCatalogPluginSourcesOmitBuiltinRow(t *testing.T) {
	stored := []pluginSourceRecord{
		{ID: 3, Name: "官方软件源", URL: defaultPluginSourceURL + "/", SourceType: "json"},
		{ID: 4, Name: "旧官方", URL: retiredDefaultPluginSourceURL, SourceType: "git"},
		{ID: 8, Name: "自定义", URL: "https://cdn.example.com/mine.json", SourceType: "json"},
	}
	managed := managedPluginSources(stored)
	if len(managed) != 1 || managed[0].ID != 8 {
		t.Fatalf("managed=%+v", managed)
	}
	catalog := catalogPluginSources(stored)
	if len(catalog) != 2 || catalog[0].ID != builtinPluginSourceID || catalog[0].URL != defaultPluginSourceURL || catalog[0].Name != defaultPluginSourceName || catalog[1].ID != 8 {
		t.Fatalf("catalog=%+v", catalog)
	}
	if !isBuiltinPluginSourceURL("https://auth.maizll.com/software-source/app_f93896d80066_5811/index.json?x=1") {
		t.Fatal("query variant should still be the builtin source")
	}
	if isBuiltinPluginSourceURL("https://cdn.example.com/mine.json") {
		t.Fatal("customer source must stay editable")
	}
}

func openPluginSourceMigrateDB(t *testing.T) *sql.DB {
	t.Helper()
	control := openAppUpdateControlDB(t)
	t.Cleanup(func() { control.Close() })
	pluginSourceMigrateDBSeq++
	databaseName := fmt.Sprintf("authpro_plugin_def_%d_%d", os.Getpid(), pluginSourceMigrateDBSeq)
	if _, err := control.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.Exec("DROP DATABASE `" + databaseName + "`") })
	db := openAppUpdateDatabase(t, databaseName)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE plugin_sources (
		id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(60) NOT NULL DEFAULT '',
		url VARCHAR(500) NOT NULL,
		source_type VARCHAR(20) NOT NULL DEFAULT 'json',
		created_at DATETIME DEFAULT NULL,
		UNIQUE KEY uk_url (url(191))
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE plugin_source_cache (
		source_id BIGINT NOT NULL PRIMARY KEY,
		source_type VARCHAR(20) NOT NULL DEFAULT 'json',
		manifest_json MEDIUMTEXT NOT NULL,
		fetched_at DATETIME NOT NULL,
		expires_at DATETIME NOT NULL,
		last_error VARCHAR(500) NOT NULL DEFAULT ''
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		t.Fatal(err)
	}
	return db
}

func assertNoBuiltinPluginSource(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query("SELECT url FROM plugin_sources")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var rawURL string
		if err := rows.Scan(&rawURL); err != nil {
			t.Fatal(err)
		}
		if isBuiltinPluginSourceURL(rawURL) {
			t.Fatalf("builtin source still stored: %s", rawURL)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func assertPluginSourceKept(t *testing.T, db *sql.DB, name, url, sourceType string) {
	t.Helper()
	var gotType string
	if err := db.QueryRow("SELECT source_type FROM plugin_sources WHERE name=? AND url=?", name, url).Scan(&gotType); err != nil {
		t.Fatal(name, err)
	}
	if gotType != sourceType {
		t.Fatalf("%s type=%s", name, gotType)
	}
}

func assertPluginSourceTypes(t *testing.T, db *sql.DB, ids map[string]int64, want map[string]string) {
	t.Helper()
	for name, sourceType := range want {
		var got string
		if err := db.QueryRow("SELECT source_type FROM plugin_sources WHERE id=?", ids[name]).Scan(&got); err != nil {
			t.Fatal(name, err)
		}
		if got != sourceType {
			t.Fatalf("%s type=%s want %s", name, got, sourceType)
		}
	}
}
