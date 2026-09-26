package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

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
	switch driverName {
	case packageStorageGitHub:
		driver, ok := packageStorageByName(packageStorageGitHub)
		if !ok {
			storeFail(c, 400, "付费包不存在")
			return
		}
		tempURL, urlErr := driver.SignedURL(c.Request.Context(), objectKey, 5*time.Minute)
		if urlErr != nil {
			storeFail(c, 400, urlErr.Error())
			return
		}
		storeData(c, gin.H{
			"url":       tempURL,
			"sha256":    sha,
			"expiresIn": 300,
		})
		return
	case packageStorageLocal:
		name := objectKey
		if _, ok := privatePackageName(sourcePaidPackagePrefix + name); !ok {
			storeFail(c, 404, "付费包不存在")
			return
		}
		if req.Version == "" {
			req.Version = version
		}
		token, err := createStoreDownloadToken(storeDownloadClaims{
			LicenseID: row.LicenseID, ItemKind: req.Kind, ItemID: req.ID, Version: req.Version, StorageKey: name, Source: "commercial",
		})
		if err != nil {
			storeFail(c, 500, "签发下载票失败")
			return
		}
		storeData(c, gin.H{
			"token":     token,
			"expiresIn": int(storeDownloadTTL.Seconds()),
			"url":       buildRequestURL(c, "/api/v1/store/packages/"+token),
		})
		return
	default:
		storeFail(c, 404, "付费包不存在")
		return
	}
}

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
	name := filepath.Base(claims.StorageKey)
	if name == "." || name == "/" || strings.Contains(claims.StorageKey, "..") {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "下载路径不合法"})
		return
	}
	path := filepath.Join(stationPaidPackageDir(), name)
	payload, err := os.ReadFile(path)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "付费包不存在"})
		return
	}
	c.Header("Content-Disposition", "attachment; filename=\""+name+"\"")
	c.Data(http.StatusOK, "application/zip", payload)
}

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
	if catalogItemPurchaseOnly(kind, itemID) {
		active = false
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
