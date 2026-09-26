package handler

import (
	"database/sql"
	"fmt"
)

const (
	sourceMigrationCatalogAdditive = "source_catalog_additive_v1"
	sourceMigrationPluginFilePath  = "source_catalog_plugin_file_path_v1"
	sourceMigrationPluginPublished = "source_catalog_plugin_published_v1"
	sourceMigrationTemplateLegacy  = "source_catalog_template_legacy_v1"
	sourceMigrationVersionBackfill = "source_catalog_version_backfill_v1"
	sourceMigrationAppIDBackfill   = "source_catalog_app_id_backfill_v1"
	sourceMigrationDeveloperAgent  = "source_developer_agent_backfill_v1"
	sourceMigrationCatalogPrice    = "source_catalog_price_v1"
	sourceMigrationCatalogOrigin   = "source_catalog_origin_v1"
	sourceMigrationPaidExternal    = "source_catalog_paid_external_visible_v1"
)

// ensureSourceStationMigrations 把源站的一次性 ALTER / 回填记入 schema_migrations。
// 成功后同名迁移不再执行，因此不会在每次启动时重复 DROP COLUMN。
func ensureSourceStationMigrations(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		name VARCHAR(100) NOT NULL,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (name)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='运行时结构迁移记录'`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	steps := []struct {
		name string
		run  func(*sql.DB) error
	}{
		{sourceMigrationCatalogAdditive, migrateSourceCatalogAdditive},
		{sourceMigrationPluginFilePath, migrateSourcePluginFilePath},
		{sourceMigrationPluginPublished, migrateSourcePluginPublished},
		{sourceMigrationTemplateLegacy, migrateSourceTemplateLegacy},
		{sourceMigrationVersionBackfill, migrateSourceCatalogVersionBackfill},
		{sourceMigrationAppIDBackfill, migrateSourceCatalogAppIDBackfill},
		{sourceMigrationDeveloperAgent, migrateSourceDeveloperAgentBackfill},
		{sourceMigrationCatalogPrice, migrateSourceCatalogPrice},
		{sourceMigrationCatalogOrigin, migrateSourceCatalogOrigin},
		{sourceMigrationPaidExternal, migratePaidExternalVisible},
		{sourceMigrationVersionStorage, migrateSourceCatalogVersionStorage},
		{storeMigrationBindings, migrateStoreBindings},
		{storeMigrationEditions, migrateStoreEditions},
		{storeMigrationOrders, migrateStorePurchaseOrders},
		{storeMigrationEntitlements, migratePluginEntitlements},
		{storeMigrationRevenue, migrateStoreRevenueLedger},
		{storeMigrationLicenseSource, migrateLicenseSourceStoreBind},
		{storeMigrationLicenseSourcePurchase, migrateLicenseSourceStorePurchase},
		{storeMigrationDomainChanges, migrateLicenseDomainChanges},
	}
	for _, step := range steps {
		if err := runSourceStationMigration(db, step.name, step.run); err != nil {
			return err
		}
	}
	return nil
}

func runSourceStationMigration(db *sql.DB, name string, migrate func(*sql.DB) error) error {
	pending, err := sourceStationMigrationPending(db, name)
	if err != nil {
		return err
	}
	if !pending {
		return nil
	}
	if err := migrate(db); err != nil {
		return fmt.Errorf("migration %s: %w", name, err)
	}
	if err := markSourceStationMigration(db, name); err != nil {
		return fmt.Errorf("migration %s: %w", name, err)
	}
	return nil
}

func sourceStationMigrationPending(db *sql.DB, name string) (bool, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&count); err != nil {
		return false, fmt.Errorf("check migration %s: %w", name, err)
	}
	return count == 0, nil
}

func markSourceStationMigration(db *sql.DB, name string) error {
	if _, err := db.Exec(`INSERT IGNORE INTO schema_migrations (name) VALUES (?)`, name); err != nil {
		return fmt.Errorf("mark migration %s: %w", name, err)
	}
	return nil
}

func sourceStationColumnExists(db *sql.DB, table, column string) (bool, error) {
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?
	`, table, column).Scan(&count); err != nil {
		return false, fmt.Errorf("check column %s.%s: %w", table, column, err)
	}
	return count > 0, nil
}

func sourceStationIndexExists(db *sql.DB, table, index string) (bool, error) {
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?
	`, table, index).Scan(&count); err != nil {
		return false, fmt.Errorf("check index %s.%s: %w", table, index, err)
	}
	return count > 0, nil
}

func ensureSourceStationColumn(db *sql.DB, table, column, statement string) error {
	exists, err := sourceStationColumnExists(db, table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := db.Exec(statement); err != nil {
		return fmt.Errorf("add %s.%s: %w", table, column, err)
	}
	return nil
}

func ensureSourceStationIndex(db *sql.DB, table, index, statement string) error {
	exists, err := sourceStationIndexExists(db, table, index)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := db.Exec(statement); err != nil {
		return fmt.Errorf("add %s.%s: %w", table, index, err)
	}
	return nil
}

func dropSourceStationColumn(db *sql.DB, table, column string) error {
	exists, err := sourceStationColumnExists(db, table, column)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", table, column)); err != nil {
		return fmt.Errorf("drop %s.%s: %w", table, column, err)
	}
	return nil
}

func migrateSourceCatalogAdditive(db *sql.DB) error {
	columns := []struct {
		table, column, statement string
	}{
		{"source_catalog_plugins", "download_url", "ALTER TABLE source_catalog_plugins ADD COLUMN download_url VARCHAR(500) NOT NULL DEFAULT '' AFTER sha256"},
		{"source_catalog_plugins", "status", "ALTER TABLE source_catalog_plugins ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'draft' AFTER download_url"},
		{"source_catalog_plugins", "review_note", "ALTER TABLE source_catalog_plugins ADD COLUMN review_note VARCHAR(500) NOT NULL DEFAULT '' AFTER status"},
		{"source_catalog_plugins", "reviewed_by", "ALTER TABLE source_catalog_plugins ADD COLUMN reviewed_by VARCHAR(50) NOT NULL DEFAULT '' AFTER review_note"},
		{"source_catalog_templates", "template_url", "ALTER TABLE source_catalog_templates ADD COLUMN template_url VARCHAR(500) NOT NULL DEFAULT '' AFTER sha256"},
		{"source_catalog_templates", "status", "ALTER TABLE source_catalog_templates ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'draft' AFTER template_url"},
		{"source_catalog_templates", "review_note", "ALTER TABLE source_catalog_templates ADD COLUMN review_note VARCHAR(500) NOT NULL DEFAULT '' AFTER status"},
		{"source_catalog_templates", "reviewed_by", "ALTER TABLE source_catalog_templates ADD COLUMN reviewed_by VARCHAR(50) NOT NULL DEFAULT '' AFTER review_note"},
		{"source_catalog_plugins", "latest_version", "ALTER TABLE source_catalog_plugins ADD COLUMN latest_version VARCHAR(40) NOT NULL DEFAULT '' AFTER version"},
		{"source_catalog_plugins", "min_version", "ALTER TABLE source_catalog_plugins ADD COLUMN min_version VARCHAR(40) NOT NULL DEFAULT '' AFTER latest_version"},
		{"source_catalog_plugins", "force_update", "ALTER TABLE source_catalog_plugins ADD COLUMN force_update TINYINT(1) NOT NULL DEFAULT 0 AFTER min_version"},
		{"source_catalog_plugins", "changelog", "ALTER TABLE source_catalog_plugins ADD COLUMN changelog VARCHAR(2000) NOT NULL DEFAULT '' AFTER download_url"},
		{"source_catalog_templates", "latest_version", "ALTER TABLE source_catalog_templates ADD COLUMN latest_version VARCHAR(40) NOT NULL DEFAULT '' AFTER version"},
		{"source_catalog_templates", "min_version", "ALTER TABLE source_catalog_templates ADD COLUMN min_version VARCHAR(40) NOT NULL DEFAULT '' AFTER latest_version"},
		{"source_catalog_templates", "force_update", "ALTER TABLE source_catalog_templates ADD COLUMN force_update TINYINT(1) NOT NULL DEFAULT 0 AFTER min_version"},
		{"source_catalog_templates", "changelog", "ALTER TABLE source_catalog_templates ADD COLUMN changelog VARCHAR(2000) NOT NULL DEFAULT '' AFTER template_url"},
		{"source_catalog_templates", "category", "ALTER TABLE source_catalog_templates ADD COLUMN category VARCHAR(30) NOT NULL DEFAULT 'home-template' AFTER developer_id"},
		{"source_catalog_plugins", "app_id", "ALTER TABLE source_catalog_plugins ADD COLUMN app_id BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER developer_id"},
		{"source_catalog_templates", "app_id", "ALTER TABLE source_catalog_templates ADD COLUMN app_id BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER developer_id"},
		{"source_developer_applications", "agent_id", "ALTER TABLE source_developer_applications ADD COLUMN agent_id BIGINT UNSIGNED DEFAULT NULL AFTER id"},
		{"source_developers", "agent_id", "ALTER TABLE source_developers ADD COLUMN agent_id BIGINT UNSIGNED DEFAULT NULL AFTER application_id"},
	}
	for _, column := range columns {
		if err := ensureSourceStationColumn(db, column.table, column.column, column.statement); err != nil {
			return err
		}
	}
	indexes := []struct {
		table, index, statement string
	}{
		{"source_catalog_plugins", "idx_source_catalog_plugin_app", "ALTER TABLE source_catalog_plugins ADD KEY idx_source_catalog_plugin_app (app_id, status)"},
		{"source_catalog_templates", "idx_source_catalog_template_app", "ALTER TABLE source_catalog_templates ADD KEY idx_source_catalog_template_app (app_id, status)"},
		{"source_developer_applications", "uk_source_developer_application_agent", "ALTER TABLE source_developer_applications ADD UNIQUE KEY uk_source_developer_application_agent (agent_id)"},
		{"source_developers", "uk_source_developer_agent", "ALTER TABLE source_developers ADD UNIQUE KEY uk_source_developer_agent (agent_id)"},
	}
	for _, index := range indexes {
		if err := ensureSourceStationIndex(db, index.table, index.index, index.statement); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`ALTER TABLE source_developer_applications MODIFY username VARCHAR(100) NOT NULL`); err != nil {
		return fmt.Errorf("normalize source_developer_applications.username: %w", err)
	}
	if _, err := db.Exec(`ALTER TABLE source_developers MODIFY username VARCHAR(100) NOT NULL`); err != nil {
		return fmt.Errorf("normalize source_developers.username: %w", err)
	}
	return nil
}

func migrateSourcePluginFilePath(db *sql.DB) error {
	exists, err := sourceStationColumnExists(db, "source_catalog_plugins", "file_path")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	hasURL, err := sourceStationColumnExists(db, "source_catalog_plugins", "download_url")
	if err != nil {
		return err
	}
	if !hasURL {
		return fmt.Errorf("refuse to drop source_catalog_plugins.file_path: download_url column is missing")
	}
	if _, err := db.Exec(`UPDATE source_catalog_plugins
		SET download_url = file_path
		WHERE TRIM(download_url) = '' AND TRIM(IFNULL(file_path, '')) <> ''`); err != nil {
		return fmt.Errorf("backfill download_url from file_path: %w", err)
	}
	unsafe, err := countSourceStationRows(db, `SELECT COUNT(*) FROM source_catalog_plugins
		WHERE TRIM(IFNULL(file_path, '')) <> '' AND TRIM(IFNULL(download_url, '')) = ''`)
	if err != nil {
		return fmt.Errorf("check file_path backfill: %w", err)
	}
	if unsafe > 0 {
		return fmt.Errorf("refuse to drop source_catalog_plugins.file_path: %d rows still have file_path but empty download_url", unsafe)
	}
	return dropSourceStationColumn(db, "source_catalog_plugins", "file_path")
}

func migrateSourcePluginPublished(db *sql.DB) error {
	exists, err := sourceStationColumnExists(db, "source_catalog_plugins", "published")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	hasStatus, err := sourceStationColumnExists(db, "source_catalog_plugins", "status")
	if err != nil {
		return err
	}
	if !hasStatus {
		return fmt.Errorf("refuse to drop source_catalog_plugins.published: status column is missing")
	}
	if _, err := db.Exec(`UPDATE source_catalog_plugins
		SET status = 'published'
		WHERE published = 1 AND (status = '' OR status = 'draft')`); err != nil {
		return fmt.Errorf("backfill plugin status from published: %w", err)
	}
	return dropSourceStationColumn(db, "source_catalog_plugins", "published")
}

func migrateSourceTemplateLegacy(db *sql.DB) error {
	hasFilePath, err := sourceStationColumnExists(db, "source_catalog_templates", "file_path")
	if err != nil {
		return err
	}
	hasPreview, err := sourceStationColumnExists(db, "source_catalog_templates", "preview_path")
	if err != nil {
		return err
	}
	if hasFilePath || hasPreview {
		hasURL, err := sourceStationColumnExists(db, "source_catalog_templates", "template_url")
		if err != nil {
			return err
		}
		if !hasURL {
			return fmt.Errorf("refuse to drop source_catalog_templates legacy paths: template_url column is missing")
		}
	}
	if hasFilePath {
		if _, err := db.Exec(`UPDATE source_catalog_templates
			SET template_url = file_path
			WHERE TRIM(template_url) = '' AND TRIM(IFNULL(file_path, '')) <> ''`); err != nil {
			return fmt.Errorf("backfill template_url from file_path: %w", err)
		}
		unsafe, err := countSourceStationRows(db, `SELECT COUNT(*) FROM source_catalog_templates
			WHERE TRIM(IFNULL(file_path, '')) <> '' AND TRIM(IFNULL(template_url, '')) = ''`)
		if err != nil {
			return fmt.Errorf("check template file_path backfill: %w", err)
		}
		if unsafe > 0 {
			return fmt.Errorf("refuse to drop source_catalog_templates.file_path: %d rows still have file_path but empty template_url", unsafe)
		}
	}
	if hasPreview {
		if _, err := db.Exec(`UPDATE source_catalog_templates
			SET template_url = preview_path
			WHERE TRIM(template_url) = '' AND TRIM(IFNULL(preview_path, '')) <> ''`); err != nil {
			return fmt.Errorf("backfill template_url from preview_path: %w", err)
		}
		unsafe, err := countSourceStationRows(db, `SELECT COUNT(*) FROM source_catalog_templates
			WHERE TRIM(IFNULL(preview_path, '')) <> '' AND TRIM(IFNULL(template_url, '')) = ''`)
		if err != nil {
			return fmt.Errorf("check preview_path backfill: %w", err)
		}
		if unsafe > 0 {
			return fmt.Errorf("refuse to drop source_catalog_templates.preview_path: %d rows still have preview_path but empty template_url", unsafe)
		}
	}
	hasPublished, err := sourceStationColumnExists(db, "source_catalog_templates", "published")
	if err != nil {
		return err
	}
	if hasPublished {
		hasStatus, err := sourceStationColumnExists(db, "source_catalog_templates", "status")
		if err != nil {
			return err
		}
		if !hasStatus {
			return fmt.Errorf("refuse to drop source_catalog_templates.published: status column is missing")
		}
		if _, err := db.Exec(`UPDATE source_catalog_templates
			SET status = 'published'
			WHERE published = 1 AND (status = '' OR status = 'draft')`); err != nil {
			return fmt.Errorf("backfill template status from published: %w", err)
		}
	}
	for _, column := range []string{"file_path", "preview_path", "preview_content_type", "format", "published"} {
		if err := dropSourceStationColumn(db, "source_catalog_templates", column); err != nil {
			return err
		}
	}
	return nil
}

func migrateSourceCatalogVersionBackfill(db *sql.DB) error {
	if _, err := db.Exec(`INSERT IGNORE INTO source_catalog_plugin_versions
		(plugin_id, version, changelog, download_url, sha256, status, created_at, updated_at)
		SELECT id, IF(version='', '0.0.0', version), '', download_url, sha256,
			CASE status WHEN 'published' THEN 'published' WHEN 'review' THEN 'pending' WHEN 'deprecated' THEN 'deprecated' ELSE 'draft' END,
			created_at, updated_at FROM source_catalog_plugins`); err != nil {
		return fmt.Errorf("backfill plugin versions: %w", err)
	}
	if _, err := db.Exec(`INSERT IGNORE INTO source_catalog_template_versions
		(template_id, version, changelog, template_url, sha256, status, created_at, updated_at)
		SELECT id, IF(version='', '0.0.0', version), '', template_url, sha256,
			CASE status WHEN 'published' THEN 'published' WHEN 'review' THEN 'pending' WHEN 'deprecated' THEN 'deprecated' ELSE 'draft' END,
			created_at, updated_at FROM source_catalog_templates`); err != nil {
		return fmt.Errorf("backfill template versions: %w", err)
	}
	if _, err := db.Exec(`UPDATE source_catalog_plugins SET latest_version=version WHERE status='published' AND latest_version=''`); err != nil {
		return fmt.Errorf("backfill plugin latest_version: %w", err)
	}
	if _, err := db.Exec(`UPDATE source_catalog_templates SET category='home-template' WHERE category='' OR category IS NULL`); err != nil {
		return fmt.Errorf("backfill template category: %w", err)
	}
	if _, err := db.Exec(`UPDATE source_catalog_templates SET latest_version=version WHERE status='published' AND latest_version=''`); err != nil {
		return fmt.Errorf("backfill template latest_version: %w", err)
	}
	return nil
}

func migrateSourceCatalogAppIDBackfill(db *sql.DB) error {
	if _, err := db.Exec(`UPDATE source_catalog_plugins p JOIN (SELECT id FROM apps ORDER BY id ASC LIMIT 1) a SET p.app_id=a.id WHERE p.app_id=0`); err != nil {
		return fmt.Errorf("backfill plugin app_id: %w", err)
	}
	if _, err := db.Exec(`UPDATE source_catalog_templates t JOIN (SELECT id FROM apps ORDER BY id ASC LIMIT 1) a SET t.app_id=a.id WHERE t.app_id=0`); err != nil {
		return fmt.Errorf("backfill template app_id: %w", err)
	}
	return nil
}

func migrateSourceCatalogPrice(db *sql.DB) error {
	columns := []struct {
		table, column, statement string
	}{
		{"source_catalog_plugins", "price_cents", "ALTER TABLE source_catalog_plugins ADD COLUMN price_cents BIGINT NOT NULL DEFAULT 0"},
		{"source_catalog_plugins", "billing", "ALTER TABLE source_catalog_plugins ADD COLUMN billing VARCHAR(20) NOT NULL DEFAULT 'free'"},
		{"source_catalog_plugins", "delivery", "ALTER TABLE source_catalog_plugins ADD COLUMN delivery VARCHAR(20) NOT NULL DEFAULT 'zip'"},
		{"source_catalog_templates", "price_cents", "ALTER TABLE source_catalog_templates ADD COLUMN price_cents BIGINT NOT NULL DEFAULT 0"},
		{"source_catalog_templates", "billing", "ALTER TABLE source_catalog_templates ADD COLUMN billing VARCHAR(20) NOT NULL DEFAULT 'free'"},
		{"source_catalog_templates", "delivery", "ALTER TABLE source_catalog_templates ADD COLUMN delivery VARCHAR(20) NOT NULL DEFAULT 'zip'"},
	}
	for _, column := range columns {
		if err := ensureSourceStationColumn(db, column.table, column.column, column.statement); err != nil {
			return err
		}
	}
	return nil
}

func migrateSourceCatalogOrigin(db *sql.DB) error {
	columns := []struct {
		table, column, statement string
	}{
		{"source_catalog_plugins", "origin_url", "ALTER TABLE source_catalog_plugins ADD COLUMN origin_url VARCHAR(500) NOT NULL DEFAULT ''"},
		{"source_catalog_plugins", "origin_health", "ALTER TABLE source_catalog_plugins ADD COLUMN origin_health VARCHAR(20) NOT NULL DEFAULT ''"},
		{"source_catalog_templates", "origin_url", "ALTER TABLE source_catalog_templates ADD COLUMN origin_url VARCHAR(500) NOT NULL DEFAULT ''"},
		{"source_catalog_templates", "origin_health", "ALTER TABLE source_catalog_templates ADD COLUMN origin_health VARCHAR(20) NOT NULL DEFAULT ''"},
		{"source_catalog_plugin_versions", "origin_url", "ALTER TABLE source_catalog_plugin_versions ADD COLUMN origin_url VARCHAR(500) NOT NULL DEFAULT ''"},
		{"source_catalog_template_versions", "origin_url", "ALTER TABLE source_catalog_template_versions ADD COLUMN origin_url VARCHAR(500) NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		if err := ensureSourceStationColumn(db, column.table, column.column, column.statement); err != nil {
			return err
		}
	}
	return nil
}

func migrateSourceDeveloperAgentBackfill(db *sql.DB) error {
	if _, err := db.Exec(`UPDATE source_developers d
		INNER JOIN agents a ON a.email = d.email AND d.email <> ''
		LEFT JOIN source_developers taken ON taken.agent_id = a.id AND taken.id <> d.id
		SET d.agent_id = a.id
		WHERE d.agent_id IS NULL AND taken.id IS NULL`); err != nil {
		return fmt.Errorf("backfill developer agent_id: %w", err)
	}
	if _, err := db.Exec(`UPDATE source_developer_applications app
		INNER JOIN agents a ON a.email = app.email AND app.email <> ''
		LEFT JOIN source_developer_applications taken ON taken.agent_id = a.id AND taken.id <> app.id
		SET app.agent_id = a.id
		WHERE app.agent_id IS NULL AND taken.id IS NULL`); err != nil {
		return fmt.Errorf("backfill developer application agent_id: %w", err)
	}
	return nil
}

func countSourceStationRows(db *sql.DB, query string) (int, error) {
	var count int
	if err := db.QueryRow(query).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
