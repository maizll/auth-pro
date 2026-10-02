// 买家本机快照和这次访问的域名。用来判断是不是商业版、目录条目归谁，以及付费插件能不能启用。

package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
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
	// EditionSource 只给顶栏展示，不参与快照签名。
	EditionSource string `json:"editionSource"`
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

// SnapshotOK 报告这份快照的签名是否对得上发行包公钥。
// 签名无效时调用方应视为没有商业版，而不是沿用过期字段。
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
	EditionSource   string
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

const (
	buyerSourceDefault    = "https://auth.maizll.com"
	buyerSiteRetryMessage = "请用正式的 https 域名打开后台后再试。本机、内网或 http 地址不能用来绑定。"
)

type buyerConnectionIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type buyerConnectionView struct {
	SourceBase string
	SiteURL    string
	TrustProxy bool
	Issues     []buyerConnectionIssue
}

func buyerRequestDomain(c *gin.Context) string {
	view := buyerConnectionForRequest(c)
	if parsed, err := parseHTTPSBase(view.SiteURL); err == nil {
		if host := normalizeLicenseDomain(parsed.Hostname()); host != "" && !storeDomainShapeRejected(host) {
			return host
		}
	}
	host, _, _ := detectBuyerVisit(c)
	return host
}

func buyerConnectionForRequest(c *gin.Context) buyerConnectionView {
	return resolveBuyerConnection(c)
}

// resolveBuyerConnection 按本次访问自动确定本站域名，以及是否信任本机反代。
// 不读取数据库里保存的源站根、站点地址或信任代理。
func resolveBuyerConnection(c *gin.Context) buyerConnectionView {
	view := buyerConnectionView{}
	host, https, trust := detectBuyerVisit(c)
	view.TrustProxy = trust
	if msg := buyerVisitProblem(host, https); msg != "" {
		view.Issues = append(view.Issues, buyerConnectionIssue{Field: "site", Message: msg})
		return view
	}
	view.SiteURL = "https://" + host
	return view
}

func buyerVisitProblem(host string, https bool) string {
	if host == "" || storeDomainShapeRejected(host) || !https {
		return buyerSiteRetryMessage
	}
	return ""
}

// detectBuyerVisit 从这次请求判断访问域名。
// 只有直接连到本机的反代才采用转发头。公网对端带来的 X-Forwarded-* 一律忽略。
// 宝塔常见配置只设置 Host 和 X-Forwarded-Proto，没有 X-Forwarded-Host，同样按 https 站点识别。
func detectBuyerVisit(c *gin.Context) (host string, https bool, trust bool) {
	if c == nil || c.Request == nil {
		return "", false, false
	}
	requestHost := headerHost(c, "")
	peer := requestPeerIP(c.Request.RemoteAddr)
	if peer == nil || !peer.IsLoopback() {
		return requestHost, c.Request.TLS != nil, false
	}
	forwardedHost := headerHost(c, "X-Forwarded-Host")
	proto := headerProto(c)
	if forwardedHost != "" && proto != "" {
		return forwardedHost, proto == "https", true
	}
	if proto == "https" || proto == "http" {
		return requestHost, proto == "https", true
	}
	return requestHost, c.Request.TLS != nil, false
}

func headerHost(c *gin.Context, name string) string {
	if c == nil || c.Request == nil {
		return ""
	}
	raw := ""
	if name == "" {
		raw = c.Request.Host
	} else {
		raw = c.GetHeader(name)
	}
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, ","); i >= 0 {
		raw = strings.TrimSpace(raw[:i])
	}
	if h, _, err := splitHostPortLoose(raw); err == nil && h != "" {
		raw = h
	}
	return normalizeLicenseDomain(raw)
}

func headerProto(c *gin.Context) string {
	if c == nil {
		return ""
	}
	raw := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if i := strings.Index(raw, ","); i >= 0 {
		raw = strings.TrimSpace(raw[:i])
	}
	return strings.ToLower(raw)
}

func requestPeerIP(remoteAddr string) net.IP {
	host := strings.TrimSpace(remoteAddr)
	if split, _, err := net.SplitHostPort(host); err == nil {
		host = split
	}
	return net.ParseIP(strings.Trim(host, "[]"))
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

func currentBuyerAccess(c *gin.Context) buyerAccessView {
	view := buyerAccessView{Edition: storeEditionFree, RequestDomain: requestHostOnly(c), Features: []string{}}
	if domain := buyerRequestDomain(c); domain != "" {
		view.RequestDomain = domain
	}
	if !storeSnapshotPublicKeyConfigured() {
		view.Reason = storeReasonSnapshotKeyUnconfigured
		view.GraceWarning = true
		return view
	}
	state, ok := loadBuyerSnapshot()
	if !ok {
		return view
	}
	if healed, changed := clearEditionOnlyRevoke(state); changed {
		state = healed
		_ = saveBuyerSnapshot(state)
	}
	accessEdition := state.Snapshot.Edition
	if !state.ExplicitRevoked {
		bindingID := state.Snapshot.BindingID
		if bindingID == "" {
			bindingID = state.BindingID
		}
		if stale, why := localCommercialSnapshotStale(bindingID); stale {
			next, edition := classifyLocalCommercialStale(state, why)
			// 禁用只影响这一次读取。重新启用后仍用原来的快照，不必等下一次刷新。
			// 仅仅还没开通商业版不算绑定失效，不写明确吊销。
			if next.ExplicitRevoked && !state.ExplicitRevoked && why != "license_inactive" {
				_ = saveBuyerSnapshot(next)
			}
			state = next
			accessEdition = edition
		}
	}
	view.Bound = true
	view.Snapshot = state.Snapshot
	view.SnapshotValid = true
	view.Domain = state.Snapshot.Domain
	view.AccountName = state.AccountName
	view.AccountRole = state.AccountRole
	view.LicenseNo = state.LicenseNo
	view.EditionSource = state.EditionSource
	view.VerifiedAt = state.VerifiedAt
	view.GraceUntil = state.GraceUntil
	view.ExplicitRevoked = state.ExplicitRevoked
	view.ExpireAt = state.Snapshot.EditionExpireAt
	view.Permanent = accessEdition == storeEditionCommercial && state.Snapshot.EditionExpireAt == nil
	if accessEdition != storeEditionCommercial {
		view.Permanent = false
		view.ExpireAt = nil
	}
	view.Features = state.Snapshot.Features
	if view.Features == nil {
		view.Features = []string{}
	}
	requestDomain := view.RequestDomain
	if requestDomain == "" {
		requestDomain = state.Snapshot.Domain
	}
	tier, reason := evaluatePaidAccess(paidAccessInput{
		SnapshotOK: true, Edition: accessEdition, SnapshotDomain: state.Snapshot.Domain,
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

func buyerOwnsCatalogItem(view buyerAccessView, kind, id string) bool {
	for _, item := range view.Snapshot.Items {
		if item.Kind == kind && item.ID == id {
			return true
		}
	}
	return false
}

// catalogAccess 是一条目录条目的购买判断。列表、按钮和购买弹窗只读这一份。
// party 为 official 或 third。grant 为空、commercial（商业版包含）或 purchase（单独买过）。
type catalogAccess struct {
	Party              string `json:"party"`
	CommercialIncluded bool   `json:"commercialIncluded"`
	Owned              bool   `json:"owned"`
	Grant              string `json:"grant"`
}

// commercialExcludesItem 表示商业版不包含这条。目录里已经写入来源和商业版免费时只看这两个字段。
// 旧目录没有这两列时，才退回内置插件、开发者编号和仅单买策略。
func commercialExcludesItem(kind, id string, priceCents, developerID int64, listedPurchaseOnly bool) bool {
	if priceCents <= 0 || strings.TrimSpace(id) == "" {
		return false
	}
	if _, _, ok := lookupStoredCatalogListing(kind, id); ok {
		_, included := resolveListing(kind, id, priceCents)
		return !included
	}
	item := findPaidCatalog(kind, id)
	if item.Party == catalogPartyOfficial || item.Party == catalogPartyThird {
		_, included := resolveListing(kind, id, priceCents)
		return !included
	}
	return legacyCommercialExcludes(kind, id, priceCents, developerID, listedPurchaseOnly)
}

// resolveCatalogAccess 在排除规则上补上当前买家是否已经拥有。启用和弹窗都走这里。
func resolveCatalogAccess(view buyerAccessView, kind, id string, priceCents int64) catalogAccess {
	if priceCents < 0 {
		priceCents = 0
	}
	party, included := resolveListing(kind, id, priceCents)
	commercial := view.Edition == storeEditionCommercial && !view.DomainMismatch && !view.ExplicitRevoked
	purchased := priceCents > 0 && buyerOwnsCatalogItem(view, kind, id)
	grant := ""
	owned := priceCents <= 0
	if included && commercial {
		grant = "commercial"
		owned = true
	} else if purchased {
		grant = "purchase"
		owned = true
	}
	return catalogAccess{Party: party, CommercialIncluded: included, Owned: owned, Grant: grant}
}

func ownershipLabel(priceCents int64, access catalogAccess) string {
	if priceCents <= 0 {
		return "free"
	}
	if access.Grant == "purchase" {
		return "purchased"
	}
	if access.Owned {
		return "included"
	}
	return "none"
}

func catalogDeveloperPaid(kind, id string) bool {
	db, err := config.DB()
	if err != nil || db == nil || id == "" {
		return false
	}
	var developerID int64
	var queryErr error
	if kind == "template" {
		queryErr = db.QueryRow(`SELECT developer_id FROM source_catalog_templates WHERE template_key = ? OR id = ? ORDER BY CASE WHEN template_key = ? THEN 0 ELSE 1 END LIMIT 1`, id, id, id).Scan(&developerID)
	} else {
		queryErr = db.QueryRow(`SELECT developer_id FROM source_catalog_plugins WHERE id = ?`, id).Scan(&developerID)
	}
	return queryErr == nil && developerID > 0
}

func paidEnableAllowed(priceCents int64, commercial, entitled bool) bool {
	if priceCents <= 0 {
		return true
	}
	return commercial || entitled
}

// buyerMayInstallPaid 判断当前买家能不能直接安装这个标价条目。
// 商业版覆盖官方条目。开发者条目和标成仅单买的条目必须另有购买权益。
func buyerMayInstallPaid(c *gin.Context, kind, id string, priceCents int64) bool {
	return resolveCatalogAccess(currentBuyerAccess(c), kind, id, priceCents).Owned
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
	if resolveCatalogAccess(view, "plugin", id, item.PriceCents).Owned {
		return false
	}
	if item.Kind == "" {
		item.Kind = "plugin"
	}
	if item.ID == "" {
		item.ID = id
	}
	writePaidItemRequired(c, "paid_plugin", item)
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
	if resolveCatalogAccess(view, "template", id, item.PriceCents).Owned {
		return false
	}
	if item.Kind == "" {
		item.Kind = "template"
	}
	if item.ID == "" {
		item.ID = id
	}
	writePaidItemRequired(c, "paid_template", item)
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
		party, included, purchaseOnly := indexListing(sourceKindPlugin, plugin.ID, plugin.PriceCents, plugin.Party, plugin.CommercialIncluded, plugin.PurchaseOnly)
		upsert(paidCatalogItem{
			Kind: "plugin", ID: plugin.ID, Name: plugin.Name, Version: plugin.Version,
			PriceCents: plugin.PriceCents, Billing: plugin.Billing,
			PurchaseOnly: purchaseOnly, Party: party, CommercialIncluded: included,
		})
	}
	for _, raw := range index.HomeTemplates {
		var meta struct {
			ID                 string `json:"id"`
			TemplateKey        string `json:"templateKey"`
			Name               string `json:"name"`
			Version            string `json:"version"`
			PriceCents         int64  `json:"priceCents"`
			Billing            string `json:"billing"`
			PurchaseOnly       bool   `json:"purchaseOnly"`
			Party              string `json:"party"`
			CommercialIncluded bool   `json:"commercialIncluded"`
		}
		if json.Unmarshal(raw, &meta) != nil {
			continue
		}
		id := meta.TemplateKey
		if id == "" {
			id = meta.ID
		}
		party, included, purchaseOnly := indexListing(sourceKindTemplate, id, meta.PriceCents, meta.Party, meta.CommercialIncluded, meta.PurchaseOnly)
		upsert(paidCatalogItem{
			Kind: "template", ID: id, Name: meta.Name, Version: meta.Version,
			PriceCents: meta.PriceCents, Billing: meta.Billing,
			PurchaseOnly: purchaseOnly, Party: party, CommercialIncluded: included,
		})
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
	// 只撤销开了「改密后解绑」的应用下的绑定，各应用互不影响。
	_, _ = db.Exec(`UPDATE store_bindings b
		JOIN app_commercial_settings s ON s.app_id = b.app_id AND s.revoke_on_password_change = 1
		SET b.status = 'revoked', b.revoked_at = NOW(), b.revoke_reason = 'password_changed'
		WHERE b.owner_type = ? AND b.owner_id = ? AND b.status = 'active'`, ownerType, ownerID)
}

func guardProductDomainChange(db *sql.DB, licenseID int64, newDomain string, admin bool) error {
	if db == nil || licenseID <= 0 {
		return nil
	}
	licenseType, oldDomain, ok := commercialDomainLicense(db, licenseID)
	if !ok || licenseType != "domain" {
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

func finishProductDomainChange(db *sql.DB, licenseID int64, oldDomain, newDomain, actor string) {
	if db == nil || licenseID <= 0 {
		return
	}
	licenseType, _, ok := commercialDomainLicense(db, licenseID)
	if !ok || licenseType != "domain" {
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

// commercialDomainLicense 读出商业版应用（出售中或已停售）下授权的类型和当前域名。普通应用的授权返回 ok=false。
func commercialDomainLicense(db *sql.DB, licenseID int64) (licenseType, domain string, ok bool) {
	err := db.QueryRow(`SELECT l.type, COALESCE((SELECT domain FROM license_domains WHERE license_id = l.id ORDER BY id LIMIT 1), '')
		FROM licenses l JOIN app_commercial_settings s ON s.app_id = l.app_id AND s.mode <> 'off'
		WHERE l.id = ?`, licenseID).Scan(&licenseType, &domain)
	return licenseType, domain, err == nil
}
