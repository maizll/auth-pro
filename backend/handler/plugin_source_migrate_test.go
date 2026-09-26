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
	defer control.Close()

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

func TestMigrateRetiredDefaultPluginSource(t *testing.T) {
	db := openPluginSourceMigrateDB(t)
	if _, err := db.Exec(`INSERT INTO plugin_sources (name, url, source_type, created_at) VALUES
		('旧官方', ?, 'git', NOW()),
		('旧官方斜杠', ?, 'git', NOW()),
		('自定义', 'https://github.com/example/custom-plugins.git', 'git', NOW()),
		('镜像', 'https://mirror.example.com/software-source/app_4e85b4724223_2603/index.json', 'json', NOW())`,
		retiredDefaultPluginSourceURL, retiredDefaultPluginSourceURL+"/"); err != nil {
		t.Fatal(err)
	}
	if err := ensurePluginSourceStorage(db); err != nil {
		t.Fatal(err)
	}
	assertDefaultPluginSource(t, db, "旧官方")
	assertPluginSourceKept(t, db, "自定义", "https://github.com/example/custom-plugins.git", "git")
	assertPluginSourceKept(t, db, "镜像", "https://mirror.example.com/software-source/app_4e85b4724223_2603/index.json", "json")
	if err := ensurePluginSourceStorage(db); err != nil {
		t.Fatal(err)
	}
	assertDefaultPluginSource(t, db, "旧官方")

	both := openPluginSourceMigrateDB(t)
	if _, err := both.Exec(`INSERT INTO plugin_sources (name, url, source_type, created_at) VALUES
		('旧官方', ?, 'git', NOW()),
		('已是新地址', ?, 'git', NOW()),
		('自定义', 'https://cdn.example.com/mine.json', 'json', NOW())`,
		retiredDefaultPluginSourceURL, defaultPluginSourceURL+"/"); err != nil {
		t.Fatal(err)
	}
	if err := ensurePluginSourceStorage(both); err != nil {
		t.Fatal(err)
	}
	if err := ensurePluginSourceStorage(both); err != nil {
		t.Fatal(err)
	}
	assertDefaultPluginSource(t, both, "已是新地址")
	assertPluginSourceKept(t, both, "自定义", "https://cdn.example.com/mine.json", "json")
	var retiredLeft int
	if err := both.QueryRow("SELECT COUNT(*) FROM plugin_sources WHERE url LIKE ?", "%app_4e85b4724223_2603%").Scan(&retiredLeft); err != nil {
		t.Fatal(err)
	}
	if retiredLeft != 0 {
		t.Fatalf("retired rows=%d", retiredLeft)
	}

	fresh := openPluginSourceMigrateDB(t)
	if err := ensurePluginSourceStorage(fresh); err != nil {
		t.Fatal(err)
	}
	if err := ensurePluginSourceStorage(fresh); err != nil {
		t.Fatal(err)
	}
	assertDefaultPluginSource(t, fresh, defaultPluginSourceName)
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

func assertDefaultPluginSource(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM plugin_sources WHERE url=?", defaultPluginSourceURL).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("default rows=%d", count)
	}
	var gotName, gotType string
	if err := db.QueryRow("SELECT name, source_type FROM plugin_sources WHERE url=?", defaultPluginSourceURL).Scan(&gotName, &gotType); err != nil {
		t.Fatal(err)
	}
	if gotName != name || gotType != pluginSourceTypeJSON {
		t.Fatalf("default name=%s type=%s", gotName, gotType)
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
