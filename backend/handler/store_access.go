package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

type buyerSnapshotState struct {
	Snapshot        storeSnapshot `json:"snapshot"`
	VerifiedAt      int64         `json:"verifiedAt"`
	GraceUntil      int64         `json:"graceUntil"`
	ExplicitRevoked bool          `json:"explicitRevoked"`
	RevokeReason    string        `json:"revokeReason"`
	LastRefreshOK   bool          `json:"lastRefreshOK"`
	AccountName     string        `json:"accountName"`
	AccountRole     string        `json:"accountRole"`
	LicenseNo       string        `json:"licenseNo"`
	BindingID       string        `json:"bindingId"`
}

func buyerSnapshotPath() string {
	return filepath.Join(config.GetDataDir(), "store", "snapshot.json")
}

func loadBuyerSnapshot() (buyerSnapshotState, bool) {
	payload, err := os.ReadFile(buyerSnapshotPath())
	if err != nil {
		return buyerSnapshotState{}, false
	}
	var state buyerSnapshotState
	if json.Unmarshal(payload, &state) != nil || state.Snapshot.BindingID == "" {
		return buyerSnapshotState{}, false
	}
	state.SnapshotOK()
	return state, verifyStoreSnapshot(state.Snapshot)
}

func (s buyerSnapshotState) SnapshotOK() bool {
	return verifyStoreSnapshot(s.Snapshot)
}

func saveBuyerSnapshot(state buyerSnapshotState) error {
	dir := filepath.Dir(buyerSnapshotPath())
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(buyerSnapshotPath(), payload, 0600)
}

type buyerAccessView struct {
	Bound           bool
	Edition         string
	Reason          string
	Domain          string
	RequestDomain   string
	DomainMismatch  bool
	Permanent       bool
	ExpireAt        *int64
	Features        []string
	OfflineGrace    bool
	GraceWarning    bool
	ExplicitRevoked bool
	VerifiedAt      int64
	GraceUntil      int64
	AccountName     string
	AccountRole     string
	LicenseNo       string
	Snapshot        storeSnapshot
	SnapshotValid   bool
}

func requestHostOnly(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	host := c.Request.Host
	if h, _, err := splitHostPortLoose(host); err == nil && h != "" {
		host = h
	}
	return normalizeLicenseDomain(host)
}

func buyerRequestDomain(c *gin.Context) string {
	host := ""
	if trust, _ := strconv.Atoi(buyerConfig(storeConfigTrustProxy)); trust == 1 {
		host = strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	}
	if host == "" {
		if site := strings.TrimSpace(buyerConfig(storeConfigSiteURL)); site != "" {
			if parsed, err := parseHTTPSBase(site); err == nil {
				host = parsed.Hostname()
			}
		}
	}
	if host == "" {
		host = c.Request.Host
	}
	if i := strings.Index(host, ","); i >= 0 {
		host = host[:i]
	}
	host = strings.TrimSpace(host)
	if h, _, err := splitHostPortLoose(host); err == nil && h != "" {
		host = h
	}
	return normalizeLicenseDomain(host)
}

func splitHostPortLoose(host string) (string, string, error) {
	if strings.HasPrefix(host, "[") {
		return "", "", errors.New("skip")
	}
	if strings.Count(host, ":") == 1 {
		parts := strings.SplitN(host, ":", 2)
		return parts[0], parts[1], nil
	}
	return host, "", nil
}

func buyerConfig(key string) string {
	db, err := config.DB()
	if err != nil {
		return ""
	}
	return configValue(db, storeConfigGroup, key)
}

func currentBuyerAccess(c *gin.Context) buyerAccessView {
	view := buyerAccessView{Edition: storeEditionFree, RequestDomain: requestHostOnly(c), Features: []string{}}
	if !storeSnapshotPublicKeyConfigured() {
		view.Reason = storeReasonSnapshotKeyUnconfigured
		view.GraceWarning = true
		return view
	}
	state, ok := loadBuyerSnapshot()
	if !ok {
		return view
	}
	if domain := buyerRequestDomain(c); domain != "" {
		view.RequestDomain = domain
	}
	view.Bound = true
	view.Snapshot = state.Snapshot
	view.SnapshotValid = true
	view.Domain = state.Snapshot.Domain
	view.AccountName = state.AccountName
	view.AccountRole = state.AccountRole
	view.LicenseNo = state.LicenseNo
	view.VerifiedAt = state.VerifiedAt
	view.GraceUntil = state.GraceUntil
	view.ExplicitRevoked = state.ExplicitRevoked
	view.ExpireAt = state.Snapshot.EditionExpireAt
	view.Permanent = state.Snapshot.Edition == storeEditionCommercial && state.Snapshot.EditionExpireAt == nil
	view.Features = state.Snapshot.Features
	if view.Features == nil {
		view.Features = []string{}
	}
	requestDomain := view.RequestDomain
	if requestDomain == "" {
		requestDomain = state.Snapshot.Domain
	}
	tier, reason := evaluatePaidAccess(paidAccessInput{
		SnapshotOK: true, Edition: state.Snapshot.Edition, SnapshotDomain: state.Snapshot.Domain,
		RequestDomain: requestDomain, Now: time.Now(), GraceUntil: time.Unix(state.GraceUntil, 0),
		ExplicitRevoked: state.ExplicitRevoked, Offline: !state.LastRefreshOK,
	})
	view.Edition = tier
	view.Reason = reason
	view.DomainMismatch = reason == "domain_mismatch"
	view.OfflineGrace = tier == storeEditionCommercial && !state.LastRefreshOK && !state.ExplicitRevoked
	view.GraceWarning = view.OfflineGrace || view.DomainMismatch
	if tier != storeEditionCommercial {
		view.Features = []string{}
	}
	return view
}

func buyerFeatureEnabled(c *gin.Context, feature string) bool {
	view := currentBuyerAccess(c)
	if view.Edition != storeEditionCommercial || view.DomainMismatch {
		return false
	}
	for _, item := range view.Features {
		if item == feature {
			return true
		}
	}
	return view.Snapshot.AllPaidItems && feature != ""
}

func buyerItemEntitled(view buyerAccessView, kind, id string) bool {
	if view.Edition == storeEditionCommercial && view.Snapshot.AllPaidItems {
		return true
	}
	for _, item := range view.Snapshot.Items {
		if item.Kind == kind && item.ID == id {
			return true
		}
	}
	return false
}

func paidEnableAllowed(priceCents int64, commercial, entitled bool) bool {
	if priceCents <= 0 {
		return true
	}
	return commercial || entitled
}

func findPaidCatalog(kind, id string) paidCatalogItem {
	for _, item := range loadPaidCatalog() {
		if item.Kind == kind && item.ID == id {
			return item
		}
	}
	return paidCatalogItem{}
}

func rejectPaidPluginEnable(c *gin.Context, id string) bool {
	if len(loadPaidCatalog()) == 0 {
		return false
	}
	item := findPaidCatalog("plugin", id)
	view := currentBuyerAccess(c)
	if paidEnableAllowed(item.PriceCents, view.Edition == storeEditionCommercial, buyerItemEntitled(view, "plugin", id)) {
		return false
	}
	writeEditionRequired(c, "paid_plugin")
	return true
}

func rejectPaidTemplateEnable(c *gin.Context, rawID string) bool {
	if rawID == "default" || len(loadPaidCatalog()) == 0 {
		return false
	}
	item := findPaidCatalog("template", rawID)
	if item.PriceCents <= 0 {
		if db, err := config.DB(); err == nil {
			var key string
			if err := db.QueryRow(`SELECT template_key FROM home_templates WHERE id = ?`, rawID).Scan(&key); err == nil {
				item = findPaidCatalog("template", key)
			}
		}
	}
	view := currentBuyerAccess(c)
	id := item.ID
	if id == "" {
		id = rawID
	}
	if paidEnableAllowed(item.PriceCents, view.Edition == storeEditionCommercial, buyerItemEntitled(view, "template", id)) {
		return false
	}
	writeEditionRequired(c, "paid_template")
	return true
}

func rememberPaidCatalogFromIndex(index *remotePluginIndex) {
	if index == nil {
		return
	}
	items := loadPaidCatalog()
	seen := map[string]int{}
	for i, item := range items {
		seen[item.Kind+":"+item.ID] = i
	}
	upsert := func(item paidCatalogItem) {
		if item.PriceCents <= 0 || item.ID == "" {
			return
		}
		key := item.Kind + ":" + item.ID
		if idx, ok := seen[key]; ok {
			items[idx] = item
			return
		}
		seen[key] = len(items)
		items = append(items, item)
	}
	for _, plugin := range index.Plugins {
		upsert(paidCatalogItem{Kind: "plugin", ID: plugin.ID, Name: plugin.Name, Version: plugin.Version, PriceCents: plugin.PriceCents})
	}
	for _, raw := range index.HomeTemplates {
		var meta struct {
			ID          string `json:"id"`
			TemplateKey string `json:"templateKey"`
			Name        string `json:"name"`
			Version     string `json:"version"`
			PriceCents  int64  `json:"priceCents"`
		}
		if json.Unmarshal(raw, &meta) != nil {
			continue
		}
		id := meta.TemplateKey
		if id == "" {
			id = meta.ID
		}
		upsert(paidCatalogItem{Kind: "template", ID: id, Name: meta.Name, Version: meta.Version, PriceCents: meta.PriceCents})
	}
	savePaidCatalog(items)
}

func maybeRevokeBindingsAfterPassword(db *sql.DB, ownerType string, ownerID int64) {
	if db == nil || ownerID <= 0 {
		return
	}
	if err := ensurePaidStoreSchema(db); err != nil {
		return
	}
	settings, err := loadEffectiveStoreSettings(db)
	if err != nil || settings.RevokeOnPasswordChange == nil || !*settings.RevokeOnPasswordChange {
		return
	}
	_, _ = db.Exec(`UPDATE store_bindings SET status = 'revoked', revoked_at = NOW(), revoke_reason = 'password_changed'
		WHERE owner_type = ? AND owner_id = ? AND status = 'active'`, ownerType, ownerID)
}

func guardProductDomainChange(db *sql.DB, licenseID int64, newDomain string, admin bool) error {
	if db == nil || licenseID <= 0 {
		return nil
	}
	settings, err := loadEffectiveStoreSettings(db)
	if err != nil || settings.ProductAppKey == "" {
		return nil
	}
	var appKey, licenseType, oldDomain string
	err = db.QueryRow(`SELECT a.app_key, l.type, COALESCE((SELECT domain FROM license_domains WHERE license_id = l.id ORDER BY id LIMIT 1), '')
		FROM licenses l JOIN apps a ON a.id = l.app_id WHERE l.id = ?`, licenseID).Scan(&appKey, &licenseType, &oldDomain)
	if err != nil || appKey != settings.ProductAppKey || licenseType != "domain" {
		return nil
	}
	if normalizeLicenseDomain(oldDomain) == normalizeLicenseDomain(newDomain) {
		return nil
	}
	var last sql.NullTime
	_ = db.QueryRow(`SELECT created_at FROM license_domain_changes WHERE license_id = ? ORDER BY id DESC LIMIT 1`, licenseID).Scan(&last)
	var lastPtr *time.Time
	if last.Valid {
		t := last.Time
		lastPtr = &t
	}
	if !domainChangeAllowed(lastPtr, time.Now(), admin) {
		return errors.New("自助更换域名每 30 天仅一次")
	}
	return nil
}

func finishProductDomainChange(db *sql.DB, licenseID int64, newDomain, actor string) {
	if db == nil || licenseID <= 0 {
		return
	}
	settings, err := loadEffectiveStoreSettings(db)
	if err != nil || settings.ProductAppKey == "" {
		return
	}
	var appKey, licenseType, oldDomain string
	err = db.QueryRow(`SELECT a.app_key, l.type, COALESCE((SELECT domain FROM license_domains WHERE license_id = l.id ORDER BY id LIMIT 1), '')
		FROM licenses l JOIN apps a ON a.id = l.app_id WHERE l.id = ?`, licenseID).Scan(&appKey, &licenseType, &oldDomain)
	if err != nil || appKey != settings.ProductAppKey || licenseType != "domain" {
		return
	}
	if normalizeLicenseDomain(oldDomain) == normalizeLicenseDomain(newDomain) {
		return
	}
	_, _ = db.Exec(`UPDATE store_bindings SET status = 'revoked', revoked_at = NOW(), revoke_reason = 'domain_changed'
		WHERE license_id = ? AND status = 'active'`, licenseID)
	_, _ = db.Exec(`INSERT INTO license_domain_changes (license_id, old_domain, new_domain, actor) VALUES (?, ?, ?, ?)`,
		licenseID, oldDomain, normalizeLicenseDomain(newDomain), trimStoreText(actor, 50))
}
