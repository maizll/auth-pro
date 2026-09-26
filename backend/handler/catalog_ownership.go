package handler

import (
	"database/sql"
	"fmt"
	"strings"
)

const sourceMigrationVersionStorage = "source_catalog_version_storage_v1"

func ensureAppDeletedAt(db *sql.DB) error {
	return ensureSourceStationColumn(db, "apps", "deleted_at",
		"ALTER TABLE apps ADD COLUMN deleted_at DATETIME NULL DEFAULT NULL")
}

func migrateSourceCatalogVersionStorage(db *sql.DB) error {
	columns := []struct {
		table, column, statement string
	}{
		{"source_catalog_plugin_versions", "storage_driver", "ALTER TABLE source_catalog_plugin_versions ADD COLUMN storage_driver VARCHAR(20) NOT NULL DEFAULT ''"},
		{"source_catalog_plugin_versions", "object_key", "ALTER TABLE source_catalog_plugin_versions ADD COLUMN object_key VARCHAR(500) NOT NULL DEFAULT ''"},
		{"source_catalog_template_versions", "storage_driver", "ALTER TABLE source_catalog_template_versions ADD COLUMN storage_driver VARCHAR(20) NOT NULL DEFAULT ''"},
		{"source_catalog_template_versions", "object_key", "ALTER TABLE source_catalog_template_versions ADD COLUMN object_key VARCHAR(500) NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		if err := ensureSourceStationColumn(db, column.table, column.column, column.statement); err != nil {
			return err
		}
	}
	backfills := []string{
		`UPDATE source_catalog_plugin_versions
			SET storage_driver = CASE
				WHEN download_url LIKE 'github:%' THEN 'github'
				WHEN download_url LIKE 'paid:%' THEN 'local'
				WHEN LOWER(download_url) LIKE 'https://%' THEN 'external'
				ELSE ''
			END,
			object_key = CASE
				WHEN download_url LIKE 'github:%' THEN SUBSTRING(download_url, 8)
				WHEN download_url LIKE 'paid:%' THEN SUBSTRING(download_url, 6)
				WHEN LOWER(download_url) LIKE 'https://%' THEN download_url
				ELSE ''
			END
			WHERE storage_driver = '' AND TRIM(download_url) <> ''`,
		`UPDATE source_catalog_template_versions
			SET storage_driver = CASE
				WHEN template_url LIKE 'github:%' THEN 'github'
				WHEN template_url LIKE 'paid:%' THEN 'local'
				WHEN LOWER(template_url) LIKE 'https://%' THEN 'external'
				ELSE ''
			END,
			object_key = CASE
				WHEN template_url LIKE 'github:%' THEN SUBSTRING(template_url, 8)
				WHEN template_url LIKE 'paid:%' THEN SUBSTRING(template_url, 6)
				WHEN LOWER(template_url) LIKE 'https://%' THEN template_url
				ELSE ''
			END
			WHERE storage_driver = '' AND TRIM(template_url) <> ''`,
	}
	for _, statement := range backfills {
		if _, err := db.Exec(statement); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unknown column") || strings.Contains(strings.ToLower(err.Error()), "doesn't exist") {
				continue
			}
			return fmt.Errorf("backfill version storage: %w", err)
		}
	}
	return nil
}

type catalogForeignKey struct {
	table     string
	name      string
	statement string
}

func catalogAppForeignKeys() []catalogForeignKey {
	return []catalogForeignKey{
		{
			table: "source_catalog_plugins", name: "fk_source_catalog_plugin_app",
			statement: "ALTER TABLE source_catalog_plugins ADD CONSTRAINT fk_source_catalog_plugin_app FOREIGN KEY (app_id) REFERENCES apps (id) ON DELETE RESTRICT",
		},
		{
			table: "source_catalog_templates", name: "fk_source_catalog_template_app",
			statement: "ALTER TABLE source_catalog_templates ADD CONSTRAINT fk_source_catalog_template_app FOREIGN KEY (app_id) REFERENCES apps (id) ON DELETE RESTRICT",
		},
		{
			table: "licenses", name: "fk_license_app",
			statement: "ALTER TABLE licenses ADD CONSTRAINT fk_license_app FOREIGN KEY (app_id) REFERENCES apps (id) ON DELETE RESTRICT",
		},
		{
			table: "license_plans", name: "fk_license_plan_app",
			statement: "ALTER TABLE license_plans ADD CONSTRAINT fk_license_plan_app FOREIGN KEY (app_id) REFERENCES apps (id) ON DELETE RESTRICT",
		},
		{
			table: "app_versions", name: "fk_app_version_app",
			statement: "ALTER TABLE app_versions ADD CONSTRAINT fk_app_version_app FOREIGN KEY (app_id) REFERENCES apps (id) ON DELETE RESTRICT",
		},
	}
}

// ensureCatalogForeignKeys 在没有孤儿行时补外键。有历史孤儿就跳过这一条，下次启动再试。
func ensureCatalogForeignKeys(db *sql.DB) error {
	for _, fk := range catalogAppForeignKeys() {
		exists, err := sourceStationTableExists(db, fk.table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		present, err := sourceStationConstraintExists(db, fk.table, fk.name)
		if err != nil {
			return err
		}
		if present {
			continue
		}
		orphans, err := countCatalogAppOrphans(db, fk.table)
		if err != nil {
			return err
		}
		if orphans > 0 {
			continue
		}
		if _, err := db.Exec(fk.statement); err != nil {
			return fmt.Errorf("add %s: %w", fk.name, err)
		}
	}
	return nil
}

func sourceStationTableExists(db *sql.DB, table string) (bool, error) {
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
	`, table).Scan(&count); err != nil {
		return false, fmt.Errorf("check table %s: %w", table, err)
	}
	return count > 0, nil
}

func sourceStationConstraintExists(db *sql.DB, table, name string) (bool, error) {
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.TABLE_CONSTRAINTS
		WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = ? AND CONSTRAINT_NAME = ?
	`, table, name).Scan(&count); err != nil {
		return false, fmt.Errorf("check constraint %s.%s: %w", table, name, err)
	}
	return count > 0, nil
}

func countCatalogAppOrphans(db *sql.DB, table string) (int, error) {
	query := `SELECT COUNT(*) FROM ` + table + ` AS child LEFT JOIN apps AS parent ON parent.id = child.app_id WHERE parent.id IS NULL`
	return countSourceStationRows(db, query)
}
