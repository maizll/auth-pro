// 付费包下载。先确认授权还能下，再发官网自己的短期票据。
// 仓库里的包由官网取回并核对 sha256，不把临时链接交给客户端。

package handler

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// StoreDownloadTicket 给已签名的买家换一张短期下载凭证。
// kind 和 id 标识插件或模板。授权不能下时返回 403。
// 无论安装包在本站还是在仓库里，返回的地址都是官网票据，过期后不能下载。
func StoreDownloadTicket(c *gin.Context) {
	if !storeTicketRate.allow(c.ClientIP(), 60, time.Minute, time.Now()) {
		storeFail(c, 429, "换票过于频繁")
		return
	}
	body, row, ok := readSignedStoreBody(c)
	if !ok {
		return
	}
	var req struct {
		Kind    string `json:"kind"`
		ID      string `json:"id"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &req); err != nil || (req.Kind != "plugin" && req.Kind != "template") || strings.TrimSpace(req.ID) == "" {
		storeFail(c, 400, "参数错误")
		return
	}
	db := c.MustGet("storeDB").(*sql.DB)
	if !licenseCanDownloadPaid(db, row.LicenseID, req.Kind, req.ID) {
		storeFail(c, 403, "当前授权不能下载该付费包")
		return
	}
	location, version, sha, _ := lookupPaidPackageLocation(db, req.Kind, req.ID)
	driverName, objectKey := classifyPackageRef(location)
	storageKey := ""
	switch driverName {
	case packageStorageGitHub, packageStorageGitee, packageStorageS3:
		// 票据里记下内部引用。客户端只看到本站地址，看不到仓库路径。
		storageKey = strings.TrimSpace(location)
	case packageStorageLocal:
		if _, ok := privatePackageName(sourcePaidPackagePrefix + objectKey); !ok {
			storeFail(c, 404, "付费包不存在")
			return
		}
		storageKey = objectKey
	default:
		storeFail(c, 404, "付费包不存在")
		return
	}
	if req.Version == "" {
		req.Version = version
	}
	token, err := createStoreDownloadToken(storeDownloadClaims{
		LicenseID: row.LicenseID, ItemKind: req.Kind, ItemID: req.ID, Version: req.Version, StorageKey: storageKey, Source: "commercial",
	})
	if err != nil {
		storeFail(c, 500, "签发下载票失败")
		return
	}
	storeData(c, gin.H{
		"token":     token,
		"sha256":    sha,
		"expiresIn": int(storeDownloadTTL.Seconds()),
		"url":       buildRequestURL(c, "/api/v1/store/packages/"+token),
	})
}

// StorePackageDownload 用票据把安装包发给买家。
// 票据过期、路径含 ..、授权已不能下载时拒绝。
// 仓库里的包在这里取回并核对 sha256，响应里不带仓库地址。
func StorePackageDownload(c *gin.Context) {
	claims, err := parseStoreDownloadToken(strings.TrimSpace(c.Param("token")))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	if err := storeDownloadAllowed(time.Unix(claims.ExpiresAt, 0), time.Now(), true); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	if !licenseCanDownloadPaid(db, claims.LicenseID, claims.ItemKind, claims.ItemID) {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": errStoreDownloadDenied.Error()})
		return
	}
	location, _, sha, _ := lookupPaidPackageLocation(db, claims.ItemKind, claims.ItemID)
	if !ticketStorageMatches(claims.StorageKey, location) || strings.Contains(claims.StorageKey, "..") {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "下载路径不合法"})
		return
	}
	resolved := ticketPackageLocation(claims.StorageKey, location)
	if signed, ok := buyerSafeRedirect(c.Request.Context(), resolved); ok {
		c.Redirect(http.StatusFound, signed)
		return
	}
	payload, readErr := readStoredPackageBytes(c.Request.Context(), resolved)
	if readErr != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": catalogPackageMissingText})
		return
	}
	sum := sha256SumHex(payload)
	if sha == "" || !strings.EqualFold(sum, sha) {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": catalogPackageSHAText})
		return
	}
	filename := safeDownloadName(claims.ItemID)
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/zip", payload)
}

func ticketStorageMatches(storageKey, location string) bool {
	storageKey = strings.TrimSpace(storageKey)
	location = strings.TrimSpace(location)
	if storageKey == "" || location == "" {
		return false
	}
	if storageKey == location {
		return true
	}
	if name, ok := privatePackageName(location); ok && name == storageKey {
		return true
	}
	return false
}

func ticketPackageLocation(storageKey, location string) string {
	if isRemoteManagedRef(storageKey) {
		return storageKey
	}
	if isPrivatePackageRef(location) {
		return location
	}
	if name, ok := privatePackageName(sourcePaidPackagePrefix + strings.TrimSpace(storageKey)); ok {
		return sourcePaidPackagePrefix + name
	}
	return location
}

func sha256SumHex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func safeDownloadName(id string) string {
	id = strings.TrimSpace(id)
	if !catalogPackageIDPattern.MatchString(id) {
		return "package.zip"
	}
	return id + ".zip"
}

// licenseDownloadAllowed 判断这条授权现在能不能下这个付费包。
// 授权不是 active 一律拒绝。官方条目在商业版有效期内可以直接下。
// 开发者的条目，以及标成仅单买的条目，必须另有一条未过期的购买权益，商业版不代替购买。
func licenseDownloadAllowed(licenseStatus string, commercialActive bool, developerID int64, entitlementActive bool) bool {
	if licenseStatus != "active" {
		return false
	}
	if commercialActive && developerID == 0 {
		return true
	}
	return entitlementActive
}

func licenseCanDownloadPaid(db *sql.DB, licenseID int64, kind, itemID string) bool {
	var licenseStatus string
	if err := db.QueryRow(`SELECT status FROM licenses WHERE id = ?`, licenseID).Scan(&licenseStatus); err != nil {
		return false
	}
	var developerID int64
	switch kind {
	case "plugin":
		_ = db.QueryRow(`SELECT developer_id FROM source_catalog_plugins WHERE id = ?`, itemID).Scan(&developerID)
	case "template":
		_ = db.QueryRow(`SELECT developer_id FROM source_catalog_templates WHERE id = ? OR template_key = ?`, itemID, itemID).Scan(&developerID)
	default:
		return false
	}
	_, _, _, active := loadCommercialEdition(db, licenseID)
	if commercialExcludesItem(kind, itemID, 1, developerID, false) {
		active = false
	} else {
		developerID = 0
	}
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM plugin_entitlements
		WHERE license_id = ? AND item_kind = ? AND item_id = ? AND status = 'active'
		AND (expires_at IS NULL OR expires_at > NOW())`, licenseID, kind, itemID).Scan(&count)
	return licenseDownloadAllowed(licenseStatus, active, developerID, err == nil && count > 0)
}

func lookupPaidPackageLocation(db *sql.DB, kind, itemID string) (location, version, sha string, developerID int64) {
	switch kind {
	case "plugin":
		_ = db.QueryRow(`SELECT download_url, version, sha256, developer_id FROM source_catalog_plugins WHERE id = ?`, itemID).Scan(&location, &version, &sha, &developerID)
	case "template":
		_ = db.QueryRow(`SELECT template_url, version, sha256, developer_id FROM source_catalog_templates WHERE id = ? OR template_key = ?`, itemID, itemID).Scan(&location, &version, &sha, &developerID)
	}
	return location, version, sha, developerID
}
