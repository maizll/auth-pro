package handler

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"auto_pro/config"
)

const (
	catalogSwitchGrandfather      = "grandfather"
	catalogSwitchPurchaseOnly     = "purchase_only"
	catalogEntitlementGrandfather = "grandfather"
)

type catalogLocationMove struct {
	Location string
	SHA256   string
	Origin   string
	Health   string
}

// preferSealedLocation 在安装包已经迁入私有位置后，允许保存请求仍带着已删除的公开地址。
func preferSealedLocation(existingLocation, existingSHA, submittedLocation, submittedSHA string) (string, string) {
	if !isStationHostedPackageURL(submittedLocation) {
		return submittedLocation, submittedSHA
	}
	if !isPrivatePackageRef(existingLocation) && !isGitHubPackageRef(existingLocation) {
		return submittedLocation, submittedSHA
	}
	name, ok := stationPackageNameFromURL(submittedLocation)
	if !ok {
		return submittedLocation, submittedSHA
	}
	if _, err := os.Stat(filepath.Join(stationPackageDir(), name)); err == nil {
		return submittedLocation, submittedSHA
	}
	sha := strings.TrimSpace(submittedSHA)
	if sha == "" {
		sha = existingSHA
	}
	return existingLocation, sha
}

func preparePluginPriceChange(existing, next sourcePlugin) (sourcePlugin, error) {
	if next.switchPrepared {
		return next, nil
	}
	if err := rejectPaidPriceOnPublicItem(existing.Status, existing.LatestVersion, existing.PriceCents, next.PriceCents, next.PriceSwitch); err != nil {
		return next, err
	}
	policy, err := normalizeCatalogPriceSwitch(next.PriceSwitch, publicFreeBecomingPaid(existing.Status, existing.LatestVersion, existing.PriceCents, next.PriceCents))
	if err != nil {
		return next, err
	}
	next.PriceSwitch = policy
	crossing := publicFreeBecomingPaid(existing.Status, existing.LatestVersion, existing.PriceCents, next.PriceCents)
	reverting := existing.PriceCents > 0 && next.PriceCents <= 0
	if !crossing && !reverting {
		final, ferr := finalizePluginPackage(next)
		final.switchPrepared = true
		return final, ferr
	}
	if catalogDeliveryLabel(next.Delivery) == sourceDeliveryBuiltin {
		next.switchPrepared = true
		next.touchOrigin = true
		if reverting {
			next.OriginURL = ""
			next.OriginHealth = ""
		}
		return next, nil
	}
	locations := []string{existing.DownloadURL, next.DownloadURL}
	versions, err := currentSourceStationStore().ListVersions(sourceKindPlugin, existing.ID)
	if err != nil {
		return next, err
	}
	for _, rel := range versions {
		locations = append(locations, rel.Location)
	}
	var presetKey string
	var preset catalogLocationMove
	if next.PriceCents > 0 && (isPrivatePackageRef(next.DownloadURL) || isGitHubPackageRef(next.DownloadURL)) &&
		catalogLocationNeedsMove(existing.DownloadURL, next.PriceCents) {
		presetKey = strings.TrimSpace(existing.DownloadURL)
		preset = catalogLocationMove{Location: next.DownloadURL, SHA256: next.SHA256, Origin: next.OriginURL, Health: next.OriginHealth}
		filtered := make([]string, 0, len(locations))
		for _, location := range locations {
			if strings.TrimSpace(location) != presetKey {
				filtered = append(filtered, location)
			}
		}
		locations = filtered
	}
	moves, err := relocateCatalogLocations(sourceKindPlugin, next.Category, existing.ID, next.Version, locations, next.PriceCents)
	if err != nil {
		return next, err
	}
	if presetKey != "" {
		if moves == nil {
			moves = map[string]catalogLocationMove{}
		}
		moves[presetKey] = preset
	}
	next.switchMoves = moves
	next.switchPrepared = true
	next.touchOrigin = true
	if move, ok := moves[strings.TrimSpace(next.DownloadURL)]; ok {
		next.DownloadURL = move.Location
		if move.SHA256 != "" {
			next.SHA256 = move.SHA256
		}
		next.OriginURL = move.Origin
		next.OriginHealth = move.Health
	} else if move, ok := moves[strings.TrimSpace(existing.DownloadURL)]; ok && strings.TrimSpace(next.DownloadURL) == strings.TrimSpace(existing.DownloadURL) {
		next.DownloadURL = move.Location
		if move.SHA256 != "" {
			next.SHA256 = move.SHA256
		}
		next.OriginURL = move.Origin
		next.OriginHealth = move.Health
	}
	if reverting {
		next.OriginURL = ""
		next.OriginHealth = ""
	}
	return next, nil
}

func prepareTemplatePriceChange(existing, next sourceTemplate) (sourceTemplate, error) {
	if next.switchPrepared {
		return next, nil
	}
	if err := rejectPaidPriceOnPublicItem(existing.Status, existing.LatestVersion, existing.PriceCents, next.PriceCents, next.PriceSwitch); err != nil {
		return next, err
	}
	policy, err := normalizeCatalogPriceSwitch(next.PriceSwitch, publicFreeBecomingPaid(existing.Status, existing.LatestVersion, existing.PriceCents, next.PriceCents))
	if err != nil {
		return next, err
	}
	next.PriceSwitch = policy
	crossing := publicFreeBecomingPaid(existing.Status, existing.LatestVersion, existing.PriceCents, next.PriceCents)
	reverting := existing.PriceCents > 0 && next.PriceCents <= 0
	if !crossing && !reverting {
		final, ferr := finalizeTemplatePackage(next)
		final.switchPrepared = true
		return final, ferr
	}
	if catalogDeliveryLabel(next.Delivery) == sourceDeliveryBuiltin {
		next.switchPrepared = true
		next.touchOrigin = true
		if reverting {
			next.OriginURL = ""
			next.OriginHealth = ""
		}
		return next, nil
	}
	locations := []string{existing.TemplateURL, next.TemplateURL}
	versions, err := currentSourceStationStore().ListVersions(sourceKindTemplate, existing.ID)
	if err != nil {
		return next, err
	}
	for _, rel := range versions {
		locations = append(locations, rel.Location)
	}
	var presetKey string
	var preset catalogLocationMove
	if next.PriceCents > 0 && (isPrivatePackageRef(next.TemplateURL) || isGitHubPackageRef(next.TemplateURL)) &&
		catalogLocationNeedsMove(existing.TemplateURL, next.PriceCents) {
		presetKey = strings.TrimSpace(existing.TemplateURL)
		preset = catalogLocationMove{Location: next.TemplateURL, SHA256: next.SHA256, Origin: next.OriginURL, Health: next.OriginHealth}
		filtered := make([]string, 0, len(locations))
		for _, location := range locations {
			if strings.TrimSpace(location) != presetKey {
				filtered = append(filtered, location)
			}
		}
		locations = filtered
	}
	moves, err := relocateCatalogLocations(sourceKindTemplate, next.Category, existing.ID, next.Version, locations, next.PriceCents)
	if err != nil {
		return next, err
	}
	if presetKey != "" {
		if moves == nil {
			moves = map[string]catalogLocationMove{}
		}
		moves[presetKey] = preset
	}
	next.switchMoves = moves
	next.switchPrepared = true
	next.touchOrigin = true
	if move, ok := moves[strings.TrimSpace(next.TemplateURL)]; ok {
		next.TemplateURL = move.Location
		if move.SHA256 != "" {
			next.SHA256 = move.SHA256
		}
		next.OriginURL = move.Origin
		next.OriginHealth = move.Health
	} else if move, ok := moves[strings.TrimSpace(existing.TemplateURL)]; ok && strings.TrimSpace(next.TemplateURL) == strings.TrimSpace(existing.TemplateURL) {
		next.TemplateURL = move.Location
		if move.SHA256 != "" {
			next.SHA256 = move.SHA256
		}
		next.OriginURL = move.Origin
		next.OriginHealth = move.Health
	}
	if reverting {
		next.OriginURL = ""
		next.OriginHealth = ""
	}
	return next, nil
}

func relocateCatalogLocations(kind, category, itemID, version string, locations []string, price int64) (map[string]catalogLocationMove, error) {
	moves := map[string]catalogLocationMove{}
	for _, raw := range locations {
		location := strings.TrimSpace(raw)
		if location == "" {
			continue
		}
		if _, ok := moves[location]; ok {
			continue
		}
		if !catalogLocationNeedsMove(location, price) {
			continue
		}
		moved, err := relocateOneCatalogLocation(kind, category, itemID, version, location, price)
		if err != nil {
			return nil, err
		}
		moves[location] = moved
	}
	if price > 0 {
		for _, raw := range locations {
			location := strings.TrimSpace(raw)
			if location == "" || !catalogLocationNeedsMove(location, price) {
				continue
			}
			if _, ok := moves[location]; !ok {
				return nil, errors.New("改为收费前需要有安装包地址")
			}
		}
	}
	return moves, nil
}

func catalogLocationNeedsMove(location string, price int64) bool {
	location = strings.TrimSpace(location)
	if location == "" {
		return false
	}
	if price > 0 {
		return !isPrivatePackageRef(location) && !isGitHubPackageRef(location)
	}
	return isPrivatePackageRef(location) || isGitHubPackageRef(location)
}

func relocateOneCatalogLocation(kind, category, itemID, version, location string, price int64) (catalogLocationMove, error) {
	if price > 0 {
		loc, sha, _, origin, health, err := adoptPaidItemLocation(kind, category, itemID, location, "", version, price)
		if err != nil {
			return catalogLocationMove{}, err
		}
		if strings.TrimSpace(loc) == "" {
			return catalogLocationMove{}, errors.New("改为收费前需要有安装包地址")
		}
		if isStationHostedPackageURL(loc) {
			sealed, fileSHA, sealErr := sealStationPackage(loc, sha, kind, itemID)
			if sealErr != nil {
				return catalogLocationMove{}, sealErr
			}
			loc, sha = sealed, fileSHA
		}
		if !isPrivatePackageRef(loc) && !isGitHubPackageRef(loc) {
			return catalogLocationMove{}, errSourcePaidExternal
		}
		if origin == "" && isHTTPSLocation(location) {
			origin = location
		}
		if health == "" && (isGitHubPackageRef(loc) || origin != "") {
			health = paidOriginHealthOK
		}
		return catalogLocationMove{Location: loc, SHA256: sha, Origin: origin, Health: health}, nil
	}
	publicURL, sha, err := releasePaidLocationToPublic(location, "")
	if err != nil {
		return catalogLocationMove{}, err
	}
	return catalogLocationMove{Location: publicURL, SHA256: sha}, nil
}

func releasePaidLocationToPublic(location, sha string) (string, string, error) {
	if isPrivatePackageRef(location) {
		return unsealPaidPackage(location, sha)
	}
	if !isGitHubPackageRef(location) {
		return location, sha, nil
	}
	driver, ok := packageStorageByName(packageStorageGitHub)
	if !ok {
		return "", "", errors.New("收费仓库未配置，无法把安装包放回公开地址")
	}
	tempURL, err := driver.SignedURL(context.Background(), classifyGitHubKey(location), 5*time.Minute)
	if err != nil || strings.TrimSpace(tempURL) == "" {
		return "", "", errors.New("收费仓库暂时无法读取安装包，不能改回免费")
	}
	payload, err := safeHTTPGet(context.Background(), tempURL, safeFetchOptions{
		AllowPrivate: false,
		RequireHTTPS: true,
		MaxBytes:     pluginPackageMaxSize,
		Timeout:      paidOriginFetchTimeout,
		MaxRedirects: defaultSafeRedirects,
		UserAgent:    "auth-pro-paid-import",
		Accept:       "application/zip,*/*",
		BaseClient:   externalPackageClient,
	})
	if err != nil || !isZipPayload(payload) {
		return "", "", errors.New("收费仓库暂时无法读取安装包，不能改回免费")
	}
	return storeStationPackage(payload)
}

func classifyGitHubKey(location string) string {
	_, key := classifyPackageRef(location)
	return key
}

func applyPluginSwitchState(item, patch sourcePlugin) sourcePlugin {
	if !patch.switchPrepared && strings.TrimSpace(patch.PriceSwitch) == "" {
		return item
	}
	item.PriceSwitch = patch.PriceSwitch
	item.switchPrepared = patch.switchPrepared
	item.switchMoves = patch.switchMoves
	item.touchOrigin = patch.touchOrigin
	if patch.touchOrigin {
		item.OriginURL = patch.OriginURL
		item.OriginHealth = patch.OriginHealth
	}
	return item
}

func applyTemplateSwitchState(item, patch sourceTemplate) sourceTemplate {
	if !patch.switchPrepared && strings.TrimSpace(patch.PriceSwitch) == "" {
		return item
	}
	item.PriceSwitch = patch.PriceSwitch
	item.switchPrepared = patch.switchPrepared
	item.switchMoves = patch.switchMoves
	item.touchOrigin = patch.touchOrigin
	if patch.touchOrigin {
		item.OriginURL = patch.OriginURL
		item.OriginHealth = patch.OriginHealth
	}
	return item
}

func (store *memorySourceStore) applyVersionMovesLocked(kind, itemID string, moves map[string]catalogLocationMove) {
	if len(moves) == 0 {
		return
	}
	bucket := store.pluginVersions
	if kind == sourceKindTemplate {
		bucket = store.templateVersions
	}
	for version, rel := range bucket[itemID] {
		move, ok := moves[strings.TrimSpace(rel.Location)]
		if !ok {
			continue
		}
		rel.Location = move.Location
		if move.SHA256 != "" {
			rel.SHA256 = move.SHA256
		}
		rel.OriginURL = move.Origin
		rel.StorageDriver, rel.ObjectKey = classifyPackageRef(move.Location)
		bucket[itemID][version] = rel
	}
}

func applyVersionMovesMySQL(kind, itemID string, moves map[string]catalogLocationMove) error {
	if len(moves) == 0 {
		return nil
	}
	db, err := config.DB()
	if err != nil || db == nil {
		return nil
	}
	if err := ensureCatalogPriceSwitchSchema(db); err != nil {
		return err
	}
	for from, move := range moves {
		driver, key := classifyPackageRef(move.Location)
		var execErr error
		if kind == sourceKindTemplate {
			_, execErr = db.Exec(`UPDATE source_catalog_template_versions
				SET template_url = ?, sha256 = IF(? = '', sha256, ?), origin_url = ?, storage_driver = ?, object_key = ?
				WHERE template_id = ? AND template_url = ?`,
				move.Location, move.SHA256, move.SHA256, move.Origin, driver, key, itemID, from)
		} else {
			_, execErr = db.Exec(`UPDATE source_catalog_plugin_versions
				SET download_url = ?, sha256 = IF(? = '', sha256, ?), origin_url = ?, storage_driver = ?, object_key = ?
				WHERE plugin_id = ? AND download_url = ?`,
				move.Location, move.SHA256, move.SHA256, move.Origin, driver, key, itemID, from)
		}
		if execErr != nil {
			return execErr
		}
	}
	return nil
}

func ensureCatalogPriceSwitchSchema(db *sql.DB) error {
	if db == nil {
		return nil
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS source_catalog_downloads (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
		item_kind VARCHAR(20) NOT NULL,
		item_id VARCHAR(64) NOT NULL,
		license_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
		owner_type VARCHAR(20) NOT NULL DEFAULT '',
		owner_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		KEY idx_catalog_download_item (item_kind, item_id, license_id),
		KEY idx_catalog_download_owner (item_kind, item_id, owner_type, owner_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS source_catalog_access (
		item_kind VARCHAR(20) NOT NULL,
		item_id VARCHAR(64) NOT NULL,
		policy VARCHAR(20) NOT NULL DEFAULT '',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (item_kind, item_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return err
	}
	return nil
}

func recordCatalogPriceSwitch(kind, itemID string, appID, developerID int64, beforePrice, afterPrice int64, policy, actorType, actorName string, moves map[string]catalogLocationMove) (int, error) {
	crossing := beforePrice <= 0 && afterPrice > 0
	reverting := beforePrice > 0 && afterPrice <= 0
	if !crossing && !reverting {
		return 0, nil
	}
	if _, memory := currentSourceStationStore().(*memorySourceStore); memory {
		return appendCatalogPriceSwitchAudit(kind, itemID, crossing, policy, actorType, actorName, 0)
	}
	if err := applyVersionMovesMySQL(kind, itemID, moves); err != nil {
		return 0, err
	}
	granted := 0
	detail := "收费改为免费，公开下载地址已恢复"
	action := "price_to_free"
	db, dbErr := config.DB()
	if crossing {
		normalized, err := normalizeCatalogPriceSwitch(policy, true)
		if err != nil {
			return 0, err
		}
		action = "price_to_paid"
		if normalized == catalogSwitchPurchaseOnly {
			detail = "免费改为收费，所有人都需购买"
		} else {
			detail = "免费改为收费，已下载过的老用户继续免费，新发放 0 条权益"
		}
		if dbErr == nil && db != nil {
			if err := ensureCatalogPriceSwitchSchema(db); err != nil {
				return 0, err
			}
			if _, err := db.Exec(`INSERT INTO source_catalog_access (item_kind, item_id, policy) VALUES (?, ?, ?)
				ON DUPLICATE KEY UPDATE policy = VALUES(policy)`, kind, itemID, normalized); err != nil {
				return 0, err
			}
			if normalized == catalogSwitchPurchaseOnly {
				if _, err := db.Exec(`UPDATE plugin_entitlements
					SET status = 'revoked', revoked_at = NOW(), revoke_reason = ?
					WHERE item_kind = ? AND item_id = ? AND source = ? AND status = 'active'`,
					"改为收费后所有人都需购买", kind, itemID, catalogEntitlementGrandfather); err != nil && !isMissingTable(err) {
					return 0, err
				}
			} else {
				granted, err = grantCatalogGrandfather(db, kind, itemID, appID)
				if err != nil {
					return 0, err
				}
				detail = "免费改为收费，已下载过的老用户继续免费，新发放 " + itoaSourceID(int64(granted)) + " 条权益"
			}
		}
	} else if dbErr == nil && db != nil {
		if err := ensureCatalogPriceSwitchSchema(db); err != nil {
			return 0, err
		}
		if _, err := db.Exec(`DELETE FROM source_catalog_access WHERE item_kind = ? AND item_id = ?`, kind, itemID); err != nil {
			return 0, err
		}
	}
	if actorType == "" {
		actorType = "admin"
	}
	_ = developerID
	_ = currentSourceStationStore().AppendAudit(sourceAuditEntry{
		ActorType: actorType, ActorName: actorName, Action: action,
		TargetType: kind, TargetID: itemID, Detail: detail,
	})
	return granted, nil
}

func appendCatalogPriceSwitchAudit(kind, itemID string, crossing bool, policy, actorType, actorName string, granted int) (int, error) {
	detail := "收费改为免费，公开下载地址已恢复"
	action := "price_to_free"
	if crossing {
		normalized, err := normalizeCatalogPriceSwitch(policy, true)
		if err != nil {
			return 0, err
		}
		action = "price_to_paid"
		if normalized == catalogSwitchPurchaseOnly {
			detail = "免费改为收费，所有人都需购买"
		} else {
			detail = "免费改为收费，已下载过的老用户继续免费，新发放 " + itoaSourceID(int64(granted)) + " 条权益"
		}
	}
	if actorType == "" {
		actorType = "admin"
	}
	_ = currentSourceStationStore().AppendAudit(sourceAuditEntry{
		ActorType: actorType, ActorName: actorName, Action: action,
		TargetType: kind, TargetID: itemID, Detail: detail,
	})
	return granted, nil
}

func isMissingTable(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "doesn't exist") || strings.Contains(text, "no such table")
}

func grantCatalogGrandfather(db *sql.DB, kind, itemID string, appID int64) (int, error) {
	if err := ensureCatalogPriceSwitchSchema(db); err != nil {
		return 0, err
	}
	if err := migratePluginEntitlements(db); err != nil {
		return 0, err
	}
	granted := 0
	seen := map[int64]bool{}
	rows, err := db.Query(`SELECT DISTINCT license_id FROM source_catalog_downloads
		WHERE item_kind = ? AND item_id = ? AND license_id > 0`, kind, itemID)
	if err != nil {
		return 0, err
	}
	var licenseIDs []int64
	for rows.Next() {
		var id int64
		if scanErr := rows.Scan(&id); scanErr == nil && id > 0 {
			licenseIDs = append(licenseIDs, id)
		}
	}
	closeErr := rows.Err()
	rows.Close()
	if closeErr != nil {
		return 0, closeErr
	}
	for _, licenseID := range licenseIDs {
		if seen[licenseID] {
			continue
		}
		ok, grantErr := grantGrandfatherLicense(db, kind, itemID, licenseID)
		if grantErr != nil {
			return granted, grantErr
		}
		seen[licenseID] = true
		if ok {
			granted++
		}
	}
	owners, err := db.Query(`SELECT DISTINCT owner_type, owner_id FROM source_catalog_downloads
		WHERE item_kind = ? AND item_id = ? AND license_id = 0 AND owner_id > 0
		AND owner_type IN ('user', 'agent')`, kind, itemID)
	if err != nil {
		return granted, err
	}
	type ownerKey struct {
		Type string
		ID   int64
	}
	var ownerList []ownerKey
	for owners.Next() {
		var item ownerKey
		if scanErr := owners.Scan(&item.Type, &item.ID); scanErr == nil {
			ownerList = append(ownerList, item)
		}
	}
	ownerErr := owners.Err()
	owners.Close()
	if ownerErr != nil {
		return granted, ownerErr
	}
	for _, owner := range ownerList {
		query := `SELECT id FROM licenses WHERE owner_type = ? AND owner_id = ?`
		args := []any{owner.Type, owner.ID}
		if appID > 0 {
			query += ` AND app_id = ?`
			args = append(args, appID)
		}
		licenseRows, queryErr := db.Query(query, args...)
		if queryErr != nil {
			return granted, queryErr
		}
		var ids []int64
		for licenseRows.Next() {
			var id int64
			if scanErr := licenseRows.Scan(&id); scanErr == nil && id > 0 {
				ids = append(ids, id)
			}
		}
		scanErr := licenseRows.Err()
		licenseRows.Close()
		if scanErr != nil {
			return granted, scanErr
		}
		for _, licenseID := range ids {
			if seen[licenseID] {
				continue
			}
			ok, grantErr := grantGrandfatherLicense(db, kind, itemID, licenseID)
			if grantErr != nil {
				return granted, grantErr
			}
			seen[licenseID] = true
			if ok {
				granted++
			}
		}
	}
	return granted, nil
}

func grantGrandfatherLicense(db *sql.DB, kind, itemID string, licenseID int64) (bool, error) {
	var ownerType string
	var ownerID int64
	err := db.QueryRow(`SELECT owner_type, owner_id FROM licenses WHERE id = ?`, licenseID).Scan(&ownerType, &ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if ownerType != "user" && ownerType != "agent" {
		return false, nil
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM plugin_entitlements
		WHERE license_id = ? AND item_kind = ? AND item_id = ? AND status = 'active'
		AND (expires_at IS NULL OR expires_at > NOW())`, licenseID, kind, itemID).Scan(&count); err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	_, err = db.Exec(`INSERT INTO plugin_entitlements
		(order_id, license_id, owner_type, owner_id, item_kind, item_id, period, source, status)
		VALUES (NULL, ?, ?, ?, ?, ?, 'permanent', ?, 'active')`,
		licenseID, ownerType, ownerID, kind, itemID, catalogEntitlementGrandfather)
	if err != nil {
		return false, err
	}
	return true, nil
}

func catalogItemPurchaseOnly(kind, itemID string) bool {
	db, err := config.DB()
	if err != nil || db == nil {
		return false
	}
	var policy string
	err = db.QueryRow(`SELECT policy FROM source_catalog_access WHERE item_kind = ? AND item_id = ?`, kind, itemID).Scan(&policy)
	return err == nil && policy == catalogSwitchPurchaseOnly
}

func withdrawDeveloperPaidListing(kind string, developerID int64, status, itemID, actor string) error {
	if developerID <= 0 || status != sourceItemPublished {
		return nil
	}
	note := "第三方付费条目暂不能上架，已从公开目录下架"
	var err error
	if kind == sourceKindTemplate {
		_, err = currentSourceStationStore().SetTemplateStatus(itemID, sourceItemHidden, actor, note)
	} else {
		_, err = currentSourceStationStore().SetPluginStatus(itemID, sourceItemHidden, actor, note)
	}
	if err != nil {
		return err
	}
	persistIndexSnapshot(actor)
	return nil
}
