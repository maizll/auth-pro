package handler

import (
	"database/sql"
	"fmt"
)

const (
	storeMigrationBindings              = "store_bindings_v1"
	storeMigrationEditions              = "store_editions_v1"
	storeMigrationOrders                = "store_purchase_orders_v1"
	storeMigrationEntitlements          = "plugin_entitlements_v1"
	storeMigrationRevenue               = "store_revenue_ledger_v1"
	storeMigrationLicenseSource         = "licenses_source_store_bind_v1"
	storeMigrationLicenseSourcePurchase = "licenses_source_store_purchase_v1"
	storeMigrationDomainChanges         = "license_domain_changes_v1"
)

func migrateStoreBindings(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS store_bindings (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			binding_id VARCHAR(64) NOT NULL,
			secret_salt VARBINARY(32) NOT NULL,
			owner_type ENUM('user','agent') NOT NULL,
			owner_id BIGINT UNSIGNED NOT NULL,
			license_id BIGINT UNSIGNED NOT NULL,
			domain_snapshot VARCHAR(255) NOT NULL,
			install_id VARCHAR(64) NOT NULL DEFAULT '',
			app_version VARCHAR(40) NOT NULL DEFAULT '',
			last_ip VARCHAR(64) NOT NULL DEFAULT '',
			status VARCHAR(20) NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_seen_at DATETIME DEFAULT NULL,
			revoked_at DATETIME DEFAULT NULL,
			revoke_reason VARCHAR(200) NOT NULL DEFAULT '',
			PRIMARY KEY (id),
			UNIQUE KEY uk_store_binding_id (binding_id),
			KEY idx_store_binding_license (license_id, status),
			KEY idx_store_binding_owner (owner_type, owner_id, status)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS store_bind_challenges (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			challenge_id VARCHAR(64) NOT NULL,
			nonce VARCHAR(80) NOT NULL,
			nonce_hash CHAR(64) NOT NULL,
			owner_type ENUM('user','agent') NOT NULL,
			owner_id BIGINT UNSIGNED NOT NULL,
			domain VARCHAR(255) NOT NULL,
			install_id VARCHAR(64) NOT NULL DEFAULT '',
			app_version VARCHAR(40) NOT NULL DEFAULT '',
			expires_at DATETIME NOT NULL,
			used_at DATETIME DEFAULT NULL,
			result VARCHAR(40) NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			UNIQUE KEY uk_store_challenge_id (challenge_id),
			KEY idx_store_challenge_expires (expires_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("store bindings: %w", err)
		}
	}
	return nil
}

func migrateStoreEditions(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS store_edition_plans (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			name VARCHAR(100) NOT NULL,
			period VARCHAR(20) NOT NULL,
			price_cents BIGINT NOT NULL,
			enabled TINYINT(1) NOT NULL DEFAULT 0,
			sort INT NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			KEY idx_store_edition_plan_enabled (enabled, sort)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS main_license_editions (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			license_id BIGINT UNSIGNED NOT NULL,
			edition VARCHAR(20) NOT NULL DEFAULT 'commercial',
			period VARCHAR(20) NOT NULL,
			started_at DATETIME NOT NULL,
			expires_at DATETIME DEFAULT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'active',
			order_id BIGINT UNSIGNED DEFAULT NULL,
			granted_by BIGINT UNSIGNED DEFAULT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			KEY idx_main_license_edition (license_id, status)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("store editions: %w", err)
		}
	}
	return nil
}

func migrateStorePurchaseOrders(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS store_purchase_orders (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
		order_no VARCHAR(64) NOT NULL,
		owner_type ENUM('user','agent') NOT NULL,
		owner_id BIGINT UNSIGNED NOT NULL,
		license_id BIGINT UNSIGNED NOT NULL,
		binding_id VARCHAR(64) NOT NULL DEFAULT '',
		item_kind VARCHAR(20) NOT NULL,
		item_id VARCHAR(64) NOT NULL,
		item_version VARCHAR(40) NOT NULL DEFAULT '',
		period VARCHAR(20) NOT NULL,
		developer_id BIGINT UNSIGNED DEFAULT NULL,
		amount_cents BIGINT NOT NULL,
		price_cents_snapshot BIGINT NOT NULL,
		title_snapshot VARCHAR(200) NOT NULL DEFAULT '',
		pay_channel VARCHAR(40) NOT NULL DEFAULT '',
		pay_method VARCHAR(40) NOT NULL DEFAULT '',
		gateway_trade_no VARCHAR(80) NOT NULL DEFAULT '',
		status VARCHAR(20) NOT NULL DEFAULT 'pending',
		return_url VARCHAR(500) NOT NULL DEFAULT '',
		needs_review TINYINT(1) NOT NULL DEFAULT 0,
		expires_at DATETIME NOT NULL,
		paid_at DATETIME DEFAULT NULL,
		result_ref VARCHAR(64) NOT NULL DEFAULT '',
		notify_payload TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		UNIQUE KEY uk_store_purchase_order_no (order_no),
		KEY idx_store_purchase_license (license_id, status),
		KEY idx_store_purchase_owner (owner_type, owner_id, created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		return fmt.Errorf("store orders: %w", err)
	}
	return nil
}

func migratePluginEntitlements(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS plugin_entitlements (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
		order_id BIGINT UNSIGNED DEFAULT NULL,
		license_id BIGINT UNSIGNED NOT NULL,
		owner_type ENUM('user','agent') NOT NULL,
		owner_id BIGINT UNSIGNED NOT NULL,
		item_kind VARCHAR(20) NOT NULL,
		item_id VARCHAR(64) NOT NULL,
		period VARCHAR(20) NOT NULL,
		expires_at DATETIME DEFAULT NULL,
		source VARCHAR(20) NOT NULL DEFAULT 'purchase',
		status VARCHAR(20) NOT NULL DEFAULT 'active',
		granted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		revoked_at DATETIME DEFAULT NULL,
		revoke_reason VARCHAR(200) NOT NULL DEFAULT '',
		PRIMARY KEY (id),
		KEY idx_plugin_entitlement_license (license_id, item_kind, item_id, status)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		return fmt.Errorf("plugin entitlements: %w", err)
	}
	return nil
}

func migrateStoreRevenueLedger(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS store_revenue_ledger (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
		order_id BIGINT UNSIGNED NOT NULL,
		source_type VARCHAR(20) NOT NULL,
		developer_id BIGINT UNSIGNED DEFAULT NULL,
		gross_cents BIGINT NOT NULL,
		fee_bps INT NOT NULL,
		net_cents BIGINT NOT NULL,
		status VARCHAR(20) NOT NULL DEFAULT 'pending',
		settled_at DATETIME DEFAULT NULL,
		payout_note VARCHAR(500) NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		KEY idx_store_revenue_order (order_id),
		KEY idx_store_revenue_status (status, source_type)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		return fmt.Errorf("store revenue: %w", err)
	}
	return nil
}

func migrateLicenseSourceStoreBind(db *sql.DB) error {
	_, err := db.Exec(`ALTER TABLE licenses MODIFY source ENUM('admin','agent','user_purchase','card','store_bind') NOT NULL`)
	if err != nil {
		return fmt.Errorf("licenses.source store_bind: %w", err)
	}
	return nil
}

func migrateLicenseSourceStorePurchase(db *sql.DB) error {
	_, err := db.Exec(`ALTER TABLE licenses MODIFY source ENUM('admin','agent','user_purchase','card','store_bind','store_purchase') NOT NULL`)
	if err != nil {
		return fmt.Errorf("licenses.source store_purchase: %w", err)
	}
	return nil
}

func migrateLicenseDomainChanges(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS license_domain_changes (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
		license_id BIGINT UNSIGNED NOT NULL,
		old_domain VARCHAR(255) NOT NULL DEFAULT '',
		new_domain VARCHAR(255) NOT NULL DEFAULT '',
		actor VARCHAR(50) NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		KEY idx_license_domain_changes (license_id, created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		return fmt.Errorf("license domain changes: %w", err)
	}
	return nil
}

func ensurePaidStoreSchema(db *sql.DB) error {
	return ensureSourceStationMigrations(db)
}
