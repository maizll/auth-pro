package handler

import (
	"database/sql"
	"strings"

	"auto_pro/config"
)

const (
	catalogPartyOfficial = "official"
	catalogPartyThird    = "third"
)

// normalizeCatalogListing 把来源和「商业版免费」收成可保存的值。
// specified 为 false 时按旧数据兜底：内置官方插件默认官方且付费即包含，开发者编号大于 0 视为第三方。
// specified 为 true 时以表单为准，第三方不能勾选商业版免费。
func normalizeCatalogListing(party string, included, specified bool, kind, id string, developerID, priceCents int64) (string, bool) {
	party = strings.TrimSpace(party)
	known := party == catalogPartyOfficial || party == catalogPartyThird
	if !specified || !known {
		if developerID > 0 && !(kind == sourceKindPlugin && officialBuiltinPlugin(id)) {
			return catalogPartyThird, false
		}
		party = catalogPartyOfficial
		included = priceCents > 0
		if kind == sourceKindPlugin && officialBuiltinPlugin(id) && priceCents > 0 {
			included = true
		}
		return party, included
	}
	if party == catalogPartyThird {
		return catalogPartyThird, false
	}
	return catalogPartyOfficial, included
}

func sealPluginListing(item *sourcePlugin, existing sourcePlugin, hasExisting bool) {
	if item == nil {
		return
	}
	specified := item.Party == catalogPartyOfficial || item.Party == catalogPartyThird
	if !specified && hasExisting && (existing.Party == catalogPartyOfficial || existing.Party == catalogPartyThird) {
		item.Party = existing.Party
		item.CommercialIncluded = existing.CommercialIncluded
		return
	}
	item.Party, item.CommercialIncluded = normalizeCatalogListing(item.Party, item.CommercialIncluded, specified, sourceKindPlugin, item.ID, item.DeveloperID, item.PriceCents)
}

func sealTemplateListing(item *sourceTemplate, existing sourceTemplate, hasExisting bool) {
	if item == nil {
		return
	}
	id := strings.TrimSpace(item.TemplateKey)
	if id == "" {
		id = item.ID
	}
	specified := item.Party == catalogPartyOfficial || item.Party == catalogPartyThird
	if !specified && hasExisting && (existing.Party == catalogPartyOfficial || existing.Party == catalogPartyThird) {
		item.Party = existing.Party
		item.CommercialIncluded = existing.CommercialIncluded
		return
	}
	item.Party, item.CommercialIncluded = normalizeCatalogListing(item.Party, item.CommercialIncluded, specified, sourceKindTemplate, id, item.DeveloperID, item.PriceCents)
}

func listingBit(included bool) int {
	if included {
		return 1
	}
	return 0
}

func pluginListingParty(item sourcePlugin) string {
	party, _ := publishedListing(item.Party, item.CommercialIncluded, sourceKindPlugin, item.ID, item.DeveloperID, item.PriceCents)
	return party
}

func pluginListingIncluded(item sourcePlugin) bool {
	_, included := publishedListing(item.Party, item.CommercialIncluded, sourceKindPlugin, item.ID, item.DeveloperID, item.PriceCents)
	return included
}

func templateListingID(item sourceTemplate) string {
	id := strings.TrimSpace(item.TemplateKey)
	if id == "" {
		return item.ID
	}
	return id
}

func templateListingParty(item sourceTemplate) string {
	party, _ := publishedListing(item.Party, item.CommercialIncluded, sourceKindTemplate, templateListingID(item), item.DeveloperID, item.PriceCents)
	return party
}

func templateListingIncluded(item sourceTemplate) bool {
	_, included := publishedListing(item.Party, item.CommercialIncluded, sourceKindTemplate, templateListingID(item), item.DeveloperID, item.PriceCents)
	return included
}

func indexListing(kind, id string, priceCents int64, party string, included, purchaseOnly bool) (string, bool, bool) {
	if party == catalogPartyOfficial || party == catalogPartyThird {
		party, included = publishedListing(party, included, kind, id, 0, priceCents)
		return party, included, priceCents > 0 && !included
	}
	if legacyCommercialExcludes(kind, id, priceCents, 0, purchaseOnly) {
		return catalogPartyThird, false, priceCents > 0
	}
	return catalogPartyOfficial, priceCents > 0, false
}

func publishedListing(party string, included bool, kind, id string, developerID, priceCents int64) (string, bool) {
	specified := party == catalogPartyOfficial || party == catalogPartyThird
	party, included = normalizeCatalogListing(party, included, specified, kind, id, developerID, priceCents)
	if priceCents <= 0 || party != catalogPartyOfficial {
		included = false
	}
	return party, included
}

func lookupStoredCatalogListing(kind, id string) (string, bool, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", false, false
	}
	db, err := config.DB()
	if err != nil || db == nil {
		return "", false, false
	}
	var party string
	var included int
	if kind == sourceKindTemplate {
		err = db.QueryRow(`SELECT listing_party, commercial_included FROM source_catalog_templates WHERE template_key = ? OR id = ? ORDER BY CASE WHEN template_key = ? THEN 0 ELSE 1 END LIMIT 1`, id, id, id).Scan(&party, &included)
	} else {
		err = db.QueryRow(`SELECT listing_party, commercial_included FROM source_catalog_plugins WHERE id = ?`, id).Scan(&party, &included)
	}
	if err != nil {
		return "", false, false
	}
	party = strings.TrimSpace(party)
	if party != catalogPartyOfficial && party != catalogPartyThird {
		return "", false, false
	}
	return party, included == 1, true
}

func catalogTableExists(db *sql.DB, table string) (bool, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, table).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// resolveListing 优先读目录上保存的来源和商业版免费，没有这两列时才退回旧规则。
func resolveListing(kind, id string, priceCents int64) (string, bool) {
	if party, included, ok := lookupStoredCatalogListing(kind, id); ok {
		if party == catalogPartyThird || priceCents <= 0 {
			if party == catalogPartyThird {
				return catalogPartyThird, false
			}
			return catalogPartyOfficial, false
		}
		return catalogPartyOfficial, included
	}
	item := findPaidCatalog(kind, id)
	if item.Party == catalogPartyOfficial || item.Party == catalogPartyThird {
		if item.Party == catalogPartyThird || priceCents <= 0 {
			if item.Party == catalogPartyThird {
				return catalogPartyThird, false
			}
			return catalogPartyOfficial, false
		}
		return catalogPartyOfficial, item.CommercialIncluded
	}
	excluded := priceCents > 0 && legacyCommercialExcludes(kind, id, priceCents, 0, item.PurchaseOnly)
	if excluded {
		return catalogPartyThird, false
	}
	return catalogPartyOfficial, priceCents > 0
}

func legacyCommercialExcludes(kind, id string, priceCents, developerID int64, listedPurchaseOnly bool) bool {
	if priceCents <= 0 || strings.TrimSpace(id) == "" {
		return false
	}
	if kind == sourceKindPlugin && officialBuiltinPlugin(id) {
		return false
	}
	if listedPurchaseOnly || developerID > 0 {
		return true
	}
	if catalogItemPurchaseOnly(kind, id) {
		return true
	}
	return catalogDeveloperPaid(kind, id)
}

func backfillCatalogListing(db *sql.DB, table, idColumn, kind string) error {
	if _, err := db.Exec(`UPDATE ` + table + ` SET listing_party='third', commercial_included=0 WHERE developer_id > 0`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE ` + table + ` SET listing_party='official', commercial_included=1 WHERE developer_id = 0 AND price_cents > 0`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE ` + table + ` SET listing_party='official', commercial_included=0 WHERE developer_id = 0 AND price_cents <= 0`); err != nil {
		return err
	}
	var accessTable int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'source_catalog_access'`).Scan(&accessTable); err != nil {
		return err
	}
	if accessTable > 0 {
		if _, err := db.Exec(`UPDATE `+table+` item JOIN source_catalog_access access ON access.item_kind = ? AND access.policy = ? AND access.item_id = item.`+idColumn+` SET item.commercial_included = 0`, kind, catalogSwitchPurchaseOnly); err != nil {
			return err
		}
	}
	if kind != sourceKindPlugin {
		return nil
	}
	rows, err := db.Query(`SELECT id, price_cents FROM ` + table)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var price int64
		if err := rows.Scan(&id, &price); err != nil {
			return err
		}
		if !officialBuiltinPlugin(id) {
			continue
		}
		included := 0
		if price > 0 {
			included = 1
		}
		if _, err := db.Exec(`UPDATE `+table+` SET listing_party='official', commercial_included=? WHERE id=?`, included, id); err != nil {
			return err
		}
	}
	return rows.Err()
}
