package handler

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"auto_pro/config"
	"github.com/go-sql-driver/mysql"
)

func TestUploadedTemplateRebasesOnlyPackagedHTMLAssets(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	entry := `<!doctype html><link href='/assets/site.css?v=1&amp;theme=gold' rel="stylesheet">
<script type="module" crossorigin src=/assets/app.js></script>
<img src="/assets/cover.svg"><video poster="/assets/cover.svg"></video>
<a href="/user/login">Host login</a><a href="//example.test/file.css">External</a>
<script>const example = '/assets/app.js'</script><img src="/../outside.svg">`
	archive := makeTestZIP(t, testZIPEntry{name: "dist/index.html", data: entry},
		testZIPEntry{name: "dist/assets/app.js", data: "console.log('loaded')"},
		testZIPEntry{name: "dist/assets/site.css", data: "body { color: gold }"},
		testZIPEntry{name: "dist/assets/cover.svg", data: "<svg/>"})
	installed, checksum, err := installUploadedHomeTemplateZIP(archive)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`src="./assets/app.js"`, `href="./assets/site.css?v=1&amp;theme=gold"`,
		`src="./assets/cover.svg"`, `poster="./assets/cover.svg"`,
		`href="/user/login"`, `href="//example.test/file.css"`,
		`const example = '/assets/app.js'`, `src="/../outside.svg"`,
	} {
		if !strings.Contains(string(payload), expected) {
			t.Errorf("missing %q in %s", expected, payload)
		}
	}
	if !installedUploadedTemplateMatches(installed, checksum) {
		t.Fatal("checksum must cover the rewritten entry")
	}
}

// Opt in to a real MySQL check using the current connection settings. All writes
// go to a session-local TEMPORARY table; existing template rows/schema stay untouched.
func TestUploadedTemplateRegistrationLegacyMySQL(t *testing.T) {
	if os.Getenv("AUTO_PRO_TEST_UPLOAD_DB") != "1" {
		t.Skip("set AUTO_PRO_TEST_UPLOAD_DB=1 for the temporary-table MySQL regression")
	}
	if os.Getenv("AUTO_PRO_DATA_DIR") == "" {
		// go test runs this package from backend/handler; db.json lives in backend.
		dataDir, err := filepath.Abs("..")
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("AUTO_PRO_DATA_DIR", dataDir)
	}
	cfg, err := config.LoadDBConfig()
	if err != nil {
		t.Fatal(err)
	}
	dsn, err := mysql.ParseDSN(config.GetDSN(cfg))
	if err != nil {
		t.Fatal("invalid database configuration")
	}
	dsn.Timeout, dsn.ReadTimeout, dsn.WriteTimeout = 3*time.Second, 5*time.Second, 5*time.Second
	connector, err := mysql.NewConnector(dsn)
	if err != nil {
		t.Fatal("invalid database connector")
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "CREATE TEMPORARY TABLE codex_upload_regression LIKE home_templates"); err != nil {
		t.Fatal(err)
	}
	defer conn.ExecContext(context.Background(), "DROP TEMPORARY TABLE IF EXISTS codex_upload_regression")
	if _, err := conn.ExecContext(ctx, "ALTER TABLE codex_upload_regression MODIFY source_url VARCHAR(500) NOT NULL"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "SET SESSION sql_mode='STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION'"); err != nil {
		t.Fatal(err)
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO codex_upload_regression (template_key, source_id, name, version, sha256)
		VALUES ('legacy-probe', 0, 'Legacy probe', '1.0.0', REPEAT('0', 64))`)
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1364 {
		t.Fatalf("expected the original missing-default error, got %v", err)
	}

	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	payload := makeTestZIP(t, testZIPEntry{name: "index.html", data: `<script type="module" src="/assets/app.js"></script>`},
		testZIPEntry{name: "assets/app.js", data: "console.log('test')"})
	if filename := os.Getenv("AUTO_PRO_TEST_TEMPLATE_ZIP"); filename != "" {
		payload, err = os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
	}
	installed, checksum, err := installUploadedHomeTemplateZIP(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !installedUploadedTemplateMatches(installed, checksum) {
		t.Fatal("the supplied ZIP did not install correctly")
	}
	entry, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(entry), `src="/assets/`) || strings.Contains(string(entry), `href="/assets/`) {
		t.Fatal("root asset paths were not rebased")
	}
	statement := strings.Replace(insertUploadedHomeTemplateSQL, "INSERT INTO home_templates", "INSERT INTO codex_upload_regression", 1)
	result, err := conn.ExecContext(ctx, statement, filepath.Base(filepath.Dir(installed)), "Uploaded regression", "", "1.0.0", "", checksum, 0, installed)
	if err != nil {
		t.Fatalf("registration against the legacy schema failed: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil || id <= 0 {
		t.Fatalf("invalid installed ID: %d %v", id, err)
	}
	var sourceURL, sourceType, registeredPath, registeredChecksum string
	if err := conn.QueryRowContext(ctx, "SELECT source_url, source_type, installed_path, sha256 FROM codex_upload_regression WHERE id=?", id).
		Scan(&sourceURL, &sourceType, &registeredPath, &registeredChecksum); err != nil {
		t.Fatal(err)
	}
	if sourceURL != "" || sourceType != "upload" || registeredPath != installed || registeredChecksum != checksum {
		t.Fatal("incorrect installed metadata")
	}
}
