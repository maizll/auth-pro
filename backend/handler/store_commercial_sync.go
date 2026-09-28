// 对照本机快照和源站库里的商业版。明确吊销立刻回到免费版；只有连不上源站才进入离线宽限。

package handler

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"auto_pro/config"
)

// localCommercialCacheStale 判断本机快照是否还对得上源站库里的商业版。
// 绑定不在本机库里时保持快照：那是买方缓存的远程权益，授权记录在源站。
// 绑定在本机（本站就是源站）时，授权被删、绑定被吊销或商业版权益已失效，就立刻视为免费版。
func localCommercialCacheStale(bindingFound, bindingActive, licenseExists, editionActive bool) bool {
	if !bindingFound {
		return false
	}
	return !bindingActive || !licenseExists || !editionActive
}

func snapshotMatchesCommercialLicense(state buyerSnapshotState, licenseNo string, bindingIDs []string) bool {
	if licenseNo != "" && (state.LicenseNo == licenseNo || state.Snapshot.LicenseNo == licenseNo) {
		return true
	}
	for _, id := range bindingIDs {
		if id == "" {
			continue
		}
		if id == state.BindingID || id == state.Snapshot.BindingID {
			return true
		}
	}
	return false
}

// localCommercialSnapshotStale 只在本机 store_bindings 能找到这条快照时才判定过期。
func localCommercialSnapshotStale(bindingID string) (bool, string) {
	bindingID = trimStoreText(bindingID, 64)
	if bindingID == "" {
		return false, ""
	}
	db, err := config.DB()
	if err != nil || db == nil {
		return false, ""
	}
	var status string
	var licenseID int64
	err = db.QueryRow(`SELECT status, license_id FROM store_bindings WHERE binding_id = ? LIMIT 1`, bindingID).Scan(&status, &licenseID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ""
	}
	if err != nil {
		return false, ""
	}
	var licenseStatus, ownerType string
	var ownerID, appID int64
	err = db.QueryRow(`SELECT status, owner_type, owner_id, app_id FROM licenses WHERE id = ?`, licenseID).Scan(&licenseStatus, &ownerType, &ownerID, &appID)
	licenseExists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, ""
	}
	var domain string
	_ = db.QueryRow(`SELECT domain_snapshot FROM store_bindings WHERE binding_id = ?`, bindingID).Scan(&domain)
	_, _, _, editionActive := loadAccountCommercialEdition(db, ownerType, ownerID, appID, domain)
	if !editionActive && licenseExists {
		_, _, _, editionActive = loadCommercialEdition(db, licenseID)
	}
	if !localCommercialCacheStale(true, status == "active", licenseExists, editionActive) && licenseStatus == "active" {
		return false, ""
	}
	if status != "active" {
		return true, "binding_revoked"
	}
	if !licenseExists {
		return true, "license_deleted"
	}
	if licenseStatus != "active" {
		return true, "license_inactive"
	}
	return true, "edition_revoked"
}

// revokeCommercialRightsForLicense 撤销这条授权上的商业版权益。
// 站点绑定保持有效，客户站刷新后变为免费版，不必重新登录。
// 开通时复制到同域名绑定授权上、且没有购买订单的权益一并撤回。
func revokeCommercialRightsForLicense(db *sql.DB, licenseID, reason string) error {
	if db == nil || strings.TrimSpace(licenseID) == "" {
		return errors.New("授权不正确")
	}
	_ = reason
	var started sql.NullTime
	_ = db.QueryRow(`SELECT started_at FROM main_license_editions
		WHERE license_id = ? AND edition = 'commercial' AND status = 'active'
		ORDER BY id DESC LIMIT 1`, licenseID).Scan(&started)
	if _, err := db.Exec(`UPDATE main_license_editions SET status = 'revoked', updated_at = NOW() WHERE license_id = ? AND status = 'active'`, licenseID); err != nil {
		return err
	}
	if !started.Valid {
		return nil
	}
	return revokeMirroredCommercialEditions(db, licenseID, started.Time)
}

// revokeMirroredCommercialEditions 撤回和这次开通同时写到绑定授权上的商业版。
// 只动没有订单号、开通时间与本次相差不超过 3 秒的权益。客户自己购买的权益留着。
func revokeMirroredCommercialEditions(db *sql.DB, licenseID string, started time.Time) error {
	var ownerType string
	var ownerID, appID, selfID int64
	err := db.QueryRow(`SELECT id, owner_type, owner_id, app_id FROM licenses WHERE id = ?`, licenseID).Scan(&selfID, &ownerType, &ownerID, &appID)
	if err != nil {
		return nil
	}
	rows, err := db.Query(`SELECT DISTINCT l.id FROM licenses l
		JOIN store_bindings b ON b.license_id = l.id AND b.status = 'active'
		WHERE l.owner_type = ? AND l.owner_id = ? AND l.app_id = ? AND l.id <> ? AND l.status = 'active'`,
		ownerType, ownerID, appID, selfID)
	if err != nil {
		return err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		if !licensesShareDomain(db, selfID, id) {
			continue
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	from := started.Add(-3 * time.Second)
	to := started.Add(3 * time.Second)
	for _, id := range ids {
		if _, err := db.Exec(`UPDATE main_license_editions SET status = 'revoked', updated_at = NOW()
			WHERE license_id = ? AND edition = 'commercial' AND status = 'active' AND order_id IS NULL
			  AND started_at >= ? AND started_at <= ?`, id, from, to); err != nil {
			return err
		}
	}
	return nil
}

func licensesShareDomain(db *sql.DB, left, right int64) bool {
	var domain string
	err := db.QueryRow(`SELECT domain FROM license_domains WHERE license_id = ? ORDER BY id LIMIT 1`, left).Scan(&domain)
	if err != nil {
		return false
	}
	var appID int64
	if err := db.QueryRow(`SELECT app_id FROM licenses WHERE id = ?`, right).Scan(&appID); err != nil {
		return false
	}
	return boundLicenseCoversDomain(db, right, appID, domain)
}

// clearEditionOnlyRevoke 去掉「只是还没买商业版」造成的失效标记。
// 这种标记会让购买窗口误以为绑定已被源站删除。绑定本身是否还在，要另问源站。
func clearEditionOnlyRevoke(state buyerSnapshotState) (buyerSnapshotState, bool) {
	if !state.ExplicitRevoked || state.RevokeReason != "edition_revoked" {
		return state, false
	}
	state.ExplicitRevoked = false
	state.RevokeReason = ""
	return state, true
}

// classifyLocalCommercialStale 区分「权益没了」和「绑定没了」。
// 商业版权益未开通或已吊销时，绑定仍可用来购买，不能要求重新登录。
// 绑定或授权记录本身无效时，才标记明确吊销。
func classifyLocalCommercialStale(state buyerSnapshotState, why string) (buyerSnapshotState, string) {
	if why == "edition_revoked" {
		return state, storeEditionFree
	}
	return markLocalBuyerSnapshotRevoked(state, why), state.Snapshot.Edition
}

func markLocalBuyerSnapshotRevoked(state buyerSnapshotState, reason string) buyerSnapshotState {
	state.ExplicitRevoked = true
	if state.RevokeReason == "" {
		state.RevokeReason = reason
	}
	return state
}
