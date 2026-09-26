package handler

import (
	"database/sql"
	"os"
	"strconv"
	"testing"
)

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
		{"site", "https://auth.maizll.com/software-source/app_4e85b4724223_2603/index.json", "git"},
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
