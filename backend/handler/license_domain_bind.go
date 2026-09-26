package handler

import "database/sql"

const licenseDomainOccupied = "该域名已被占用"

func licenseDomainTaken(db *sql.DB, appID, licenseID int64, domain string) (bool, error) {
	if db == nil || appID <= 0 || domain == "" {
		return false, nil
	}
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM licenses l
		JOIN license_domains ld ON ld.license_id = l.id
		WHERE l.app_id = ? AND l.id <> ? AND l.status = 'active' AND ld.domain = ?
	`, appID, licenseID, domain).Scan(&count)
	return count > 0, err
}
