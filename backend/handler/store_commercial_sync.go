// 对照本机快照和源站库里的商业版。明确吊销立刻回到免费版；只有连不上源站才进入离线宽限。

package handler

import (
	"database/sql"
	"errors"

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
	var licenseStatus string
	err = db.QueryRow(`SELECT status FROM licenses WHERE id = ?`, licenseID).Scan(&licenseStatus)
	licenseExists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, ""
	}
	var editionCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM main_license_editions
		WHERE license_id = ? AND edition = 'commercial' AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > NOW())`, licenseID).Scan(&editionCount); err != nil {
		return false, ""
	}
	editionActive := editionCount > 0
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

// revokeCommercialRightsForLicense 撤销这条授权上的商业版权益和站点绑定，并清掉本机对应快照。
func revokeCommercialRightsForLicense(db *sql.DB, licenseID, reason string) error {
	if db == nil || licenseID == "" {
		return errors.New("授权不正确")
	}
	var licenseNo string
	_ = db.QueryRow(`SELECT license_no FROM licenses WHERE id = ?`, licenseID).Scan(&licenseNo)
	rows, err := db.Query(`SELECT binding_id FROM store_bindings WHERE license_id = ?`, licenseID)
	if err != nil {
		return err
	}
	bindingIDs := make([]string, 0)
	for rows.Next() {
		var bindingID string
		if err := rows.Scan(&bindingID); err != nil {
			continue
		}
		bindingIDs = append(bindingIDs, bindingID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if _, err := db.Exec(`UPDATE main_license_editions SET status = 'revoked', updated_at = NOW() WHERE license_id = ? AND status = 'active'`, licenseID); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE store_bindings SET status = 'revoked', revoked_at = NOW(), revoke_reason = ? WHERE license_id = ? AND status = 'active'`, trimStoreText(reason, 200), licenseID); err != nil {
		return err
	}
	invalidateLocalBuyerSnapshot(licenseNo, bindingIDs, reason)
	return nil
}

func invalidateLocalBuyerSnapshot(licenseNo string, bindingIDs []string, reason string) {
	state, ok := loadBuyerSnapshot()
	if !ok || state.ExplicitRevoked {
		return
	}
	if !snapshotMatchesCommercialLicense(state, licenseNo, bindingIDs) {
		return
	}
	state.ExplicitRevoked = true
	state.RevokeReason = trimStoreText(reason, 200)
	_ = saveBuyerSnapshot(state)
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
