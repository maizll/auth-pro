package handler

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"auto_pro/config"
)

func normalizeSourceVersion(raw string) (string, error) {
	version := strings.TrimSpace(raw)
	if version == "" {
		version = "1.0.0"
	}
	if !sourceVersionPattern.MatchString(version) {
		return "", errors.New("版本号不合法")
	}
	return version, nil
}

func sourceVersionTransitionAllowed(from, to string) bool {
	if from == to {
		return true
	}
	switch to {
	case sourceVersionPending:
		return from == sourceVersionDraft
	case sourceVersionDraft:
		return from == sourceVersionPending
	case sourceVersionPublished:
		return from == sourceVersionPending || from == sourceVersionDraft
	case sourceVersionDeprecated:
		return from == sourceVersionPublished
	default:
		return false
	}
}

func (store *memorySourceStore) versionMap(kind string) map[string]map[string]sourceRelease {
	if kind == sourceKindTemplate {
		return store.templateVersions
	}
	return store.pluginVersions
}

func (store *memorySourceStore) ListVersions(kind, itemID string) ([]sourceRelease, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	bucket := store.versionMap(kind)[itemID]
	result := make([]sourceRelease, 0, len(bucket))
	for _, item := range bucket {
		result = append(result, item)
	}
	return result, nil
}

func (store *memorySourceStore) GetVersion(kind, itemID, version string) (sourceRelease, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	item, ok := store.versionMap(kind)[itemID][version]
	if !ok {
		return sourceRelease{}, errSourceNotFound
	}
	return item, nil
}

func (store *memorySourceStore) UpsertVersion(rel sourceRelease, developerID int64, asAdmin bool) (sourceRelease, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.upsertVersionLocked(rel, developerID, asAdmin)
}

func (store *memorySourceStore) upsertVersionLocked(rel sourceRelease, developerID int64, asAdmin bool) (sourceRelease, error) {
	if rel.Kind == "" {
		rel.Kind = sourceKindPlugin
	}
	if err := store.assertVersionOwnerLocked(rel.Kind, rel.ItemID, developerID, asAdmin); err != nil {
		return sourceRelease{}, err
	}
	version, err := normalizeSourceVersion(rel.Version)
	if err != nil {
		return sourceRelease{}, err
	}
	rel.Version = version
	rel.Changelog = truncateText(rel.Changelog, 2000)
	rel.Location = strings.TrimSpace(rel.Location)
	rel.SHA256 = strings.ToLower(strings.TrimSpace(rel.SHA256))
	now := time.Now().UTC()
	bucket := store.versionMap(rel.Kind)
	if bucket[rel.ItemID] == nil {
		bucket[rel.ItemID] = map[string]sourceRelease{}
	}
	existing, exists := bucket[rel.ItemID][rel.Version]
	if exists {
		if existing.Status == sourceVersionPublished || existing.Status == sourceVersionDeprecated {
			if !asAdmin && (existing.Location != rel.Location || existing.SHA256 != rel.SHA256) {
				return sourceRelease{}, errSourceVersionImmutable
			}
			if !asAdmin {
				return existing, nil
			}
			// Admin re-upload: overwrite package fields and restart version at draft
			// so catalog item draft→approve can find a draft/pending release.
			if rel.Location != "" && rel.Location != existing.Location {
				store.auditLocked("url_change", rel.Kind+"-version", rel.ItemID+"@"+rel.Version, "admin", rel.Location)
				existing.Location = rel.Location
			} else if rel.Location != "" {
				existing.Location = rel.Location
			}
			if rel.SHA256 != "" {
				existing.SHA256 = rel.SHA256
			}
			if rel.Location != "" {
				existing.OriginURL = rel.OriginURL
			}
			if rel.Changelog != "" {
				existing.Changelog = rel.Changelog
			}
			stampReleaseStorage(&existing)
			existing.Status = sourceVersionDraft
			existing.ReviewNote = ""
			existing.ReviewedBy = ""
			existing.UpdatedAt = now
			bucket[rel.ItemID][rel.Version] = existing
			store.auditLocked("reupload_reset", rel.Kind+"-version", rel.ItemID+"@"+rel.Version, "admin", "status reset to draft")
			if err := store.denormalizeLatestLocked(rel.Kind, rel.ItemID); err != nil {
				return sourceRelease{}, err
			}
			return existing, nil
		}
		if rel.Location != "" {
			existing.Location = rel.Location
			existing.OriginURL = rel.OriginURL
		}
		if rel.SHA256 != "" {
			existing.SHA256 = rel.SHA256
		}
		if rel.Changelog != "" {
			existing.Changelog = rel.Changelog
		}
		stampReleaseStorage(&existing)
		existing.UpdatedAt = now
		bucket[rel.ItemID][rel.Version] = existing
		if err := store.denormalizeLatestLocked(rel.Kind, rel.ItemID); err != nil {
			return sourceRelease{}, err
		}
		return existing, nil
	}
	if rel.Status == "" {
		rel.Status = sourceVersionDraft
	}
	stampReleaseStorage(&rel)
	rel.CreatedAt = now
	rel.UpdatedAt = now
	bucket[rel.ItemID][rel.Version] = rel
	if err := store.denormalizeLatestLocked(rel.Kind, rel.ItemID); err != nil {
		return sourceRelease{}, err
	}
	return rel, nil
}

func (store *memorySourceStore) SetVersionStatus(kind, itemID, version, status, actor, note string) (sourceRelease, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	item, ok := store.versionMap(kind)[itemID][version]
	if !ok {
		return sourceRelease{}, errSourceNotFound
	}
	if !sourceVersionTransitionAllowed(item.Status, status) {
		return sourceRelease{}, errSourceInvalidStatus
	}
	if status == sourceVersionPublished && !sourceItemReady(item.SHA256, item.Location) {
		return sourceRelease{}, errSourcePublishIncomplete
	}
	item.Status = status
	item.ReviewNote = truncateText(note, 500)
	item.ReviewedBy = actor
	item.UpdatedAt = time.Now().UTC()
	store.versionMap(kind)[itemID][version] = item
	action := status
	if status == sourceVersionPublished {
		action = "approve"
		if err := store.setLatestLocked(kind, itemID, version, actor, note); err != nil {
			return sourceRelease{}, err
		}
	}
	if status == sourceVersionDeprecated {
		action = "deprecate"
		if err := store.repointOrDeprecateItemAfterVersionLocked(kind, itemID, actor, note); err != nil {
			return sourceRelease{}, err
		}
	}
	store.auditLocked(action, kind+"-version", itemID+"@"+version, actor, note)
	if err := store.denormalizeLatestLocked(kind, itemID); err != nil {
		return sourceRelease{}, err
	}
	return store.versionMap(kind)[itemID][version], nil
}

func (store *memorySourceStore) SetLatestVersion(kind, itemID, version, actor, note string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.setLatestLocked(kind, itemID, version, actor, note); err != nil {
		return err
	}
	store.auditLocked("rollback", kind, itemID+"@"+version, actor, note)
	return store.denormalizeLatestLocked(kind, itemID)
}

func (store *memorySourceStore) setLatestLocked(kind, itemID, version, actor, note string) error {
	item, ok := store.versionMap(kind)[itemID][version]
	if !ok {
		return errSourceNotFound
	}
	if item.Status != sourceVersionPublished {
		return errSourceVersionNotLatest
	}
	_ = actor
	_ = note
	if kind == sourceKindTemplate {
		tpl, ok := store.templates[itemID]
		if !ok {
			return errSourceNotFound
		}
		tpl.LatestVersion = version
		store.templates[itemID] = tpl
		return nil
	}
	plugin, ok := store.plugins[itemID]
	if !ok {
		return errSourceNotFound
	}
	plugin.LatestVersion = version
	store.plugins[itemID] = plugin
	return nil
}

func (store *memorySourceStore) assertVersionOwnerLocked(kind, itemID string, developerID int64, asAdmin bool) error {
	if asAdmin {
		return nil
	}
	if kind == sourceKindTemplate {
		item, ok := store.templates[itemID]
		if !ok {
			return errSourceNotFound
		}
		if item.DeveloperID != developerID {
			return errSourceForbidden
		}
		return nil
	}
	item, ok := store.plugins[itemID]
	if !ok {
		return errSourceNotFound
	}
	if item.DeveloperID != developerID {
		return errSourceForbidden
	}
	return nil
}

func (store *memorySourceStore) pickVersionLocked(kind, itemID, preferred, wantStatus string) (sourceRelease, bool) {
	bucket := store.versionMap(kind)[itemID]
	if preferred != "" {
		if item, ok := bucket[preferred]; ok && (wantStatus == "" || item.Status == wantStatus) {
			return item, true
		}
	}
	for _, item := range bucket {
		if wantStatus == "" || item.Status == wantStatus {
			return item, true
		}
	}
	return sourceRelease{}, false
}

func (store *memorySourceStore) applyItemStatusToVersionsLocked(kind, itemID, status, actor, note string) error {
	preferred := ""
	if kind == sourceKindTemplate {
		if item, ok := store.templates[itemID]; ok {
			preferred = item.Version
			if item.LatestVersion != "" {
				preferred = item.LatestVersion
			}
		}
	} else if item, ok := store.plugins[itemID]; ok {
		preferred = item.Version
		if item.LatestVersion != "" {
			preferred = item.LatestVersion
		}
	}
	switch status {
	case sourceItemReview:
		rel, ok := store.pickVersionLocked(kind, itemID, preferred, sourceVersionDraft)
		if !ok {
			rel, ok = store.pickVersionLocked(kind, itemID, preferred, "")
		}
		if !ok {
			return errSourceNotFound
		}
		if rel.Status == sourceVersionPublished {
			return nil
		}
		if !sourceVersionTransitionAllowed(rel.Status, sourceVersionPending) && rel.Status != sourceVersionPending {
			return errSourceInvalidStatus
		}
		rel.Status = sourceVersionPending
		rel.ReviewNote = truncateText(note, 500)
		rel.ReviewedBy = actor
		rel.UpdatedAt = time.Now().UTC()
		store.versionMap(kind)[itemID][rel.Version] = rel
	case sourceItemApproved:
		rel, ok := store.pickVersionLocked(kind, itemID, preferred, sourceVersionPending)
		if !ok {
			rel, ok = store.pickVersionLocked(kind, itemID, preferred, sourceVersionDraft)
		}
		if !ok {
			return errSourceNotFound
		}
		if !sourceItemReady(rel.SHA256, rel.Location) {
			return errSourcePublishIncomplete
		}
		rel.Status = sourceVersionPublished
		rel.ReviewNote = truncateText(note, 500)
		rel.ReviewedBy = actor
		rel.UpdatedAt = time.Now().UTC()
		store.versionMap(kind)[itemID][rel.Version] = rel
		if err := store.setLatestLocked(kind, itemID, rel.Version, actor, note); err != nil {
			return err
		}
	case sourceItemRejected:
		rel, ok := store.pickVersionLocked(kind, itemID, preferred, sourceVersionPending)
		if ok {
			rel.Status = sourceVersionDraft
			rel.ReviewNote = truncateText(note, 500)
			rel.ReviewedBy = actor
			rel.UpdatedAt = time.Now().UTC()
			store.versionMap(kind)[itemID][rel.Version] = rel
		}
	case sourceItemPublished:
		rel, ok := store.pickVersionLocked(kind, itemID, preferred, sourceVersionPublished)
		if !ok {
			rel, ok = store.pickVersionLocked(kind, itemID, preferred, sourceVersionPending)
		}
		if !ok {
			rel, ok = store.pickVersionLocked(kind, itemID, preferred, sourceVersionDraft)
		}
		if !ok {
			return errSourceNotFound
		}
		if rel.Status != sourceVersionPublished {
			if !sourceItemReady(rel.SHA256, rel.Location) {
				return errSourcePublishIncomplete
			}
			rel.Status = sourceVersionPublished
			rel.ReviewedBy = actor
			rel.UpdatedAt = time.Now().UTC()
			store.versionMap(kind)[itemID][rel.Version] = rel
		}
		if err := store.setLatestLocked(kind, itemID, rel.Version, actor, note); err != nil {
			return err
		}
	}
	return store.denormalizeLatestLocked(kind, itemID)
}

func (store *memorySourceStore) denormalizeLatestLocked(kind, itemID string) error {
	if kind == sourceKindTemplate {
		item, ok := store.templates[itemID]
		if !ok {
			return errSourceNotFound
		}
		rel, ok := store.pickDisplayVersionLocked(kind, itemID, item.LatestVersion, item.Version)
		if ok {
			item.Version = rel.Version
			item.SHA256 = rel.SHA256
			item.TemplateURL = rel.Location
			item.Changelog = rel.Changelog
			if rel.Status == sourceVersionPublished {
				item.LatestVersion = rel.Version
			}
			store.templates[itemID] = item
		}
		return nil
	}
	item, ok := store.plugins[itemID]
	if !ok {
		return errSourceNotFound
	}
	rel, ok := store.pickDisplayVersionLocked(kind, itemID, item.LatestVersion, item.Version)
	if ok {
		item.Version = rel.Version
		item.SHA256 = rel.SHA256
		item.DownloadURL = rel.Location
		item.Changelog = rel.Changelog
		if rel.Status == sourceVersionPublished {
			item.LatestVersion = rel.Version
		}
		store.plugins[itemID] = item
	}
	return nil
}

func (store *memorySourceStore) pickDisplayVersionLocked(kind, itemID, latest, fallback string) (sourceRelease, bool) {
	if latest != "" {
		if item, ok := store.versionMap(kind)[itemID][latest]; ok && item.Status == sourceVersionPublished {
			return item, true
		}
	}
	if item, ok := store.pickVersionLocked(kind, itemID, fallback, sourceVersionPublished); ok {
		return item, true
	}
	if item, ok := pickPublishedSourceRelease(store.listedVersionsLocked(kind, itemID)); ok {
		return item, true
	}
	return store.pickVersionLocked(kind, itemID, fallback, "")
}

func (store *memorySourceStore) listedVersionsLocked(kind, itemID string) []sourceRelease {
	bucket := store.versionMap(kind)[itemID]
	result := make([]sourceRelease, 0, len(bucket))
	for _, item := range bucket {
		result = append(result, item)
	}
	return result
}

func (store *memorySourceStore) repointOrDeprecateItemAfterVersionLocked(kind, itemID, actor, note string) error {
	if rel, ok := pickPublishedSourceRelease(store.listedVersionsLocked(kind, itemID)); ok {
		return store.setLatestLocked(kind, itemID, rel.Version, actor, note)
	}
	return store.markItemDeprecatedLocked(kind, itemID, actor, note)
}

func (store *memorySourceStore) markItemDeprecatedLocked(kind, itemID, actor, note string) error {
	now := time.Now().UTC()
	if kind == sourceKindTemplate {
		item, ok := store.templates[itemID]
		if !ok {
			return errSourceNotFound
		}
		if item.Status == sourceItemDeprecated {
			return nil
		}
		if !sourceTransitionAllowed(item.Status, sourceItemDeprecated) {
			return nil
		}
		item.Status = sourceItemDeprecated
		item.ReviewNote = truncateText(note, 500)
		item.ReviewedBy = actor
		item.UpdatedAt = now
		store.templates[itemID] = item
		store.auditLocked(sourceCatalogAuditAction(sourceItemDeprecated), kind, itemID, actor, note)
		return nil
	}
	item, ok := store.plugins[itemID]
	if !ok {
		return errSourceNotFound
	}
	if item.Status == sourceItemDeprecated {
		return nil
	}
	if !sourceTransitionAllowed(item.Status, sourceItemDeprecated) {
		return nil
	}
	item.Status = sourceItemDeprecated
	item.ReviewNote = truncateText(note, 500)
	item.ReviewedBy = actor
	item.UpdatedAt = now
	store.plugins[itemID] = item
	store.auditLocked(sourceCatalogAuditAction(sourceItemDeprecated), kind, itemID, actor, note)
	return nil
}

func pickPublishedSourceRelease(items []sourceRelease) (sourceRelease, bool) {
	var best sourceRelease
	found := false
	for _, item := range items {
		if item.Status != sourceVersionPublished {
			continue
		}
		if !found || item.UpdatedAt.After(best.UpdatedAt) || (item.UpdatedAt.Equal(best.UpdatedAt) && item.Version > best.Version) {
			best = item
			found = true
		}
	}
	return best, found
}

func (mysqlSourceStore) ListVersions(kind, itemID string) ([]sourceRelease, error) {
	db, err := config.DB()
	if err != nil {
		return nil, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return nil, err
	}
	query, args := mysqlVersionListQuery(kind, itemID)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]sourceRelease, 0)
	for rows.Next() {
		item, err := scanSourceRelease(rows, kind)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (mysqlSourceStore) GetVersion(kind, itemID, version string) (sourceRelease, error) {
	db, err := config.DB()
	if err != nil {
		return sourceRelease{}, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return sourceRelease{}, err
	}
	query, args := mysqlVersionGetQuery(kind, itemID, version)
	item, err := scanSourceRelease(db.QueryRow(query, args...), kind)
	if errors.Is(err, sql.ErrNoRows) {
		return sourceRelease{}, errSourceNotFound
	}
	return item, err
}

func (mysqlSourceStore) UpsertVersion(rel sourceRelease, developerID int64, asAdmin bool) (sourceRelease, error) {
	if rel.Kind == "" {
		rel.Kind = sourceKindPlugin
	}
	if err := (mysqlSourceStore{}).assertVersionOwner(rel.Kind, rel.ItemID, developerID, asAdmin); err != nil {
		return sourceRelease{}, err
	}
	version, err := normalizeSourceVersion(rel.Version)
	if err != nil {
		return sourceRelease{}, err
	}
	rel.Version = version
	rel.Changelog = truncateText(rel.Changelog, 2000)
	rel.Location = strings.TrimSpace(rel.Location)
	rel.SHA256 = strings.ToLower(strings.TrimSpace(rel.SHA256))
	existing, err := (mysqlSourceStore{}).GetVersion(rel.Kind, rel.ItemID, rel.Version)
	if err != nil && !errors.Is(err, errSourceNotFound) {
		return sourceRelease{}, err
	}
	db, err := config.DB()
	if err != nil {
		return sourceRelease{}, err
	}
	if err == nil {
		if existing.Status == sourceVersionPublished || existing.Status == sourceVersionDeprecated {
			if !asAdmin && (existing.Location != rel.Location || existing.SHA256 != rel.SHA256) && (rel.Location != "" || rel.SHA256 != "") {
				return sourceRelease{}, errSourceVersionImmutable
			}
			if !asAdmin {
				return (mysqlSourceStore{}).GetVersion(rel.Kind, rel.ItemID, rel.Version)
			}
			merged := coalesceRelease(existing, rel)
			merged.Status = sourceVersionDraft
			merged.ReviewNote = ""
			merged.ReviewedBy = ""
			if err := mysqlWriteVersion(db, rel.Kind, existing.ItemID, existing.Version, merged, sourceVersionDraft); err != nil {
				return sourceRelease{}, err
			}
			if rel.Location != "" && existing.Location != rel.Location {
				mysqlAppendAudit(db, "url_change", rel.Kind+"-version", rel.ItemID+"@"+rel.Version, "admin", rel.Location)
			}
			mysqlAppendAudit(db, "reupload_reset", rel.Kind+"-version", rel.ItemID+"@"+rel.Version, "admin", "status reset to draft")
			_ = (mysqlSourceStore{}).denormalizeLatest(rel.Kind, rel.ItemID)
			return (mysqlSourceStore{}).GetVersion(rel.Kind, rel.ItemID, rel.Version)
		}
		merged := coalesceRelease(existing, rel)
		if err := mysqlWriteVersion(db, rel.Kind, rel.ItemID, rel.Version, merged, existing.Status); err != nil {
			return sourceRelease{}, err
		}
		_ = (mysqlSourceStore{}).denormalizeLatest(rel.Kind, rel.ItemID)
		return (mysqlSourceStore{}).GetVersion(rel.Kind, rel.ItemID, rel.Version)
	}
	if rel.Status == "" {
		rel.Status = sourceVersionDraft
	}
	if err := mysqlWriteVersion(db, rel.Kind, rel.ItemID, rel.Version, rel, rel.Status); err != nil {
		return sourceRelease{}, err
	}
	_ = (mysqlSourceStore{}).denormalizeLatest(rel.Kind, rel.ItemID)
	return (mysqlSourceStore{}).GetVersion(rel.Kind, rel.ItemID, rel.Version)
}

func (mysqlSourceStore) SetVersionStatus(kind, itemID, version, status, actor, note string) (sourceRelease, error) {
	item, err := (mysqlSourceStore{}).GetVersion(kind, itemID, version)
	if err != nil {
		return sourceRelease{}, err
	}
	if !sourceVersionTransitionAllowed(item.Status, status) {
		return sourceRelease{}, errSourceInvalidStatus
	}
	if status == sourceVersionPublished && !sourceItemReady(item.SHA256, item.Location) {
		return sourceRelease{}, errSourcePublishIncomplete
	}
	db, err := config.DB()
	if err != nil {
		return sourceRelease{}, err
	}
	table, idCol := mysqlVersionTable(kind)
	if _, err := db.Exec(`UPDATE `+table+` SET status=?, review_note=?, reviewed_by=? WHERE `+idCol+`=? AND version=?`,
		status, truncateText(note, 500), actor, itemID, version); err != nil {
		return sourceRelease{}, err
	}
	action := status
	if status == sourceVersionPublished {
		action = "approve"
		if err := (mysqlSourceStore{}).pointLatest(kind, itemID, version); err != nil {
			return sourceRelease{}, err
		}
	}
	if status == sourceVersionDeprecated {
		action = "deprecate"
		if err := (mysqlSourceStore{}).repointOrDeprecateItemAfterVersion(kind, itemID, actor, note); err != nil {
			return sourceRelease{}, err
		}
	}
	mysqlAppendAudit(db, action, kind+"-version", itemID+"@"+version, actor, note)
	_ = (mysqlSourceStore{}).denormalizeLatest(kind, itemID)
	return (mysqlSourceStore{}).GetVersion(kind, itemID, version)
}

func (mysqlSourceStore) SetLatestVersion(kind, itemID, version, actor, note string) error {
	item, err := (mysqlSourceStore{}).GetVersion(kind, itemID, version)
	if err != nil {
		return err
	}
	if item.Status != sourceVersionPublished {
		return errSourceVersionNotLatest
	}
	if err := (mysqlSourceStore{}).pointLatest(kind, itemID, version); err != nil {
		return err
	}
	db, err := config.DB()
	if err != nil {
		return err
	}
	mysqlAppendAudit(db, "rollback", kind, itemID+"@"+version, actor, note)
	return (mysqlSourceStore{}).denormalizeLatest(kind, itemID)
}

func (mysqlSourceStore) pointLatest(kind, itemID, version string) error {
	db, err := config.DB()
	if err != nil {
		return err
	}
	if kind == sourceKindTemplate {
		_, err = db.Exec(`UPDATE source_catalog_templates SET latest_version=? WHERE id=?`, version, itemID)
		return err
	}
	_, err = db.Exec(`UPDATE source_catalog_plugins SET latest_version=? WHERE id=?`, version, itemID)
	return err
}

func (mysqlSourceStore) assertVersionOwner(kind, itemID string, developerID int64, asAdmin bool) error {
	if asAdmin {
		return nil
	}
	if kind == sourceKindTemplate {
		item, err := (mysqlSourceStore{}).GetTemplate(itemID)
		if err != nil {
			return err
		}
		if item.DeveloperID != developerID {
			return errSourceForbidden
		}
		return nil
	}
	item, err := (mysqlSourceStore{}).GetPlugin(itemID)
	if err != nil {
		return err
	}
	if item.DeveloperID != developerID {
		return errSourceForbidden
	}
	return nil
}

func (mysqlSourceStore) applyItemStatusToVersions(kind, itemID, status, actor, note string) error {
	preferred := ""
	if kind == sourceKindTemplate {
		if item, err := (mysqlSourceStore{}).GetTemplate(itemID); err == nil {
			preferred = item.Version
			if item.LatestVersion != "" {
				preferred = item.LatestVersion
			}
		}
	} else if item, err := (mysqlSourceStore{}).GetPlugin(itemID); err == nil {
		preferred = item.Version
		if item.LatestVersion != "" {
			preferred = item.LatestVersion
		}
	}
	versions, err := (mysqlSourceStore{}).ListVersions(kind, itemID)
	if err != nil {
		return err
	}
	pick := func(want string) (sourceRelease, bool) {
		if preferred != "" {
			for _, item := range versions {
				if item.Version == preferred && (want == "" || item.Status == want) {
					return item, true
				}
			}
		}
		for _, item := range versions {
			if want == "" || item.Status == want {
				return item, true
			}
		}
		return sourceRelease{}, false
	}
	switch status {
	case sourceItemReview:
		rel, ok := pick(sourceVersionDraft)
		if !ok {
			rel, ok = pick("")
		}
		if !ok {
			return errSourceNotFound
		}
		if rel.Status != sourceVersionPublished && rel.Status != sourceVersionPending {
			if _, err := (mysqlSourceStore{}).SetVersionStatus(kind, itemID, rel.Version, sourceVersionPending, actor, note); err != nil {
				return err
			}
		}
	case sourceItemApproved:
		rel, ok := pick(sourceVersionPending)
		if !ok {
			rel, ok = pick(sourceVersionDraft)
		}
		if !ok {
			return errSourceNotFound
		}
		if _, err := (mysqlSourceStore{}).SetVersionStatus(kind, itemID, rel.Version, sourceVersionPublished, actor, note); err != nil {
			return err
		}
	case sourceItemRejected:
		if rel, ok := pick(sourceVersionPending); ok {
			if _, err := (mysqlSourceStore{}).SetVersionStatus(kind, itemID, rel.Version, sourceVersionDraft, actor, note); err != nil {
				return err
			}
		}
	case sourceItemPublished:
		rel, ok := pick(sourceVersionPublished)
		if !ok {
			rel, ok = pick(sourceVersionPending)
		}
		if !ok {
			rel, ok = pick(sourceVersionDraft)
		}
		if !ok {
			return errSourceNotFound
		}
		if rel.Status != sourceVersionPublished {
			if _, err := (mysqlSourceStore{}).SetVersionStatus(kind, itemID, rel.Version, sourceVersionPublished, actor, note); err != nil {
				return err
			}
		} else if err := (mysqlSourceStore{}).pointLatest(kind, itemID, rel.Version); err != nil {
			return err
		}
	}
	return (mysqlSourceStore{}).denormalizeLatest(kind, itemID)
}

func (mysqlSourceStore) denormalizeLatest(kind, itemID string) error {
	db, err := config.DB()
	if err != nil {
		return err
	}
	if kind == sourceKindTemplate {
		item, err := (mysqlSourceStore{}).GetTemplate(itemID)
		if err != nil {
			return err
		}
		rel, err := (mysqlSourceStore{}).displayVersion(kind, itemID, item.LatestVersion, item.Version)
		if err != nil {
			return nil
		}
		_, err = db.Exec(`UPDATE source_catalog_templates SET version=?, sha256=?, template_url=?, changelog=?, latest_version=? WHERE id=?`,
			rel.Version, rel.SHA256, rel.Location, rel.Changelog, latestIfPublished(item.LatestVersion, rel), itemID)
		return err
	}
	item, err := (mysqlSourceStore{}).GetPlugin(itemID)
	if err != nil {
		return err
	}
	rel, err := (mysqlSourceStore{}).displayVersion(kind, itemID, item.LatestVersion, item.Version)
	if err != nil {
		return nil
	}
	_, err = db.Exec(`UPDATE source_catalog_plugins SET version=?, sha256=?, download_url=?, changelog=?, latest_version=? WHERE id=?`,
		rel.Version, rel.SHA256, rel.Location, rel.Changelog, latestIfPublished(item.LatestVersion, rel), itemID)
	return err
}

func (mysqlSourceStore) displayVersion(kind, itemID, latest, fallback string) (sourceRelease, error) {
	if latest != "" {
		if item, err := (mysqlSourceStore{}).GetVersion(kind, itemID, latest); err == nil && item.Status == sourceVersionPublished {
			return item, nil
		}
	}
	versions, err := (mysqlSourceStore{}).ListVersions(kind, itemID)
	if err != nil {
		return sourceRelease{}, err
	}
	if rel, ok := pickPublishedSourceRelease(versions); ok {
		if fallback != "" {
			for _, item := range versions {
				if item.Version == fallback && item.Status == sourceVersionPublished {
					return item, nil
				}
			}
		}
		return rel, nil
	}
	for _, item := range versions {
		if item.Version == fallback {
			return item, nil
		}
	}
	if len(versions) == 0 {
		return sourceRelease{}, errSourceNotFound
	}
	return versions[0], nil
}

func (mysqlSourceStore) repointOrDeprecateItemAfterVersion(kind, itemID, actor, note string) error {
	versions, err := (mysqlSourceStore{}).ListVersions(kind, itemID)
	if err != nil {
		return err
	}
	if rel, ok := pickPublishedSourceRelease(versions); ok {
		return (mysqlSourceStore{}).pointLatest(kind, itemID, rel.Version)
	}
	if kind == sourceKindTemplate {
		item, err := (mysqlSourceStore{}).GetTemplate(itemID)
		if err != nil {
			return err
		}
		if item.Status == sourceItemDeprecated || !sourceTransitionAllowed(item.Status, sourceItemDeprecated) {
			return nil
		}
		_, err = (mysqlSourceStore{}).SetTemplateStatus(itemID, sourceItemDeprecated, actor, note)
		return err
	}
	item, err := (mysqlSourceStore{}).GetPlugin(itemID)
	if err != nil {
		return err
	}
	if item.Status == sourceItemDeprecated || !sourceTransitionAllowed(item.Status, sourceItemDeprecated) {
		return nil
	}
	_, err = (mysqlSourceStore{}).SetPluginStatus(itemID, sourceItemDeprecated, actor, note)
	return err
}

func latestIfPublished(current string, rel sourceRelease) string {
	if rel.Status == sourceVersionPublished {
		return rel.Version
	}
	if current != "" {
		return current
	}
	return ""
}

func mysqlVersionTable(kind string) (table, idCol string) {
	if kind == sourceKindTemplate {
		return "source_catalog_template_versions", "template_id"
	}
	return "source_catalog_plugin_versions", "plugin_id"
}

func mysqlVersionListQuery(kind, itemID string) (string, []any) {
	table, idCol := mysqlVersionTable(kind)
	loc := "download_url"
	if kind == sourceKindTemplate {
		loc = "template_url"
	}
	return `SELECT ` + idCol + `, version, changelog, ` + loc + `, sha256, origin_url, status, review_note, reviewed_by, created_at, updated_at, storage_driver, object_key FROM ` + table + ` WHERE ` + idCol + `=? ORDER BY created_at DESC, version DESC`, []any{itemID}
}

func mysqlVersionGetQuery(kind, itemID, version string) (string, []any) {
	table, idCol := mysqlVersionTable(kind)
	loc := "download_url"
	if kind == sourceKindTemplate {
		loc = "template_url"
	}
	return `SELECT ` + idCol + `, version, changelog, ` + loc + `, sha256, origin_url, status, review_note, reviewed_by, created_at, updated_at, storage_driver, object_key FROM ` + table + ` WHERE ` + idCol + `=? AND version=?`, []any{itemID, version}
}

func scanSourceRelease(scanner interface{ Scan(dest ...any) error }, kind string) (sourceRelease, error) {
	var item sourceRelease
	var createdAt, updatedAt time.Time
	item.Kind = kind
	if err := scanner.Scan(&item.ItemID, &item.Version, &item.Changelog, &item.Location, &item.SHA256, &item.OriginURL, &item.Status, &item.ReviewNote, &item.ReviewedBy, &createdAt, &updatedAt, &item.StorageDriver, &item.ObjectKey); err != nil {
		return sourceRelease{}, err
	}
	item.CreatedAt, item.UpdatedAt = createdAt.UTC(), updatedAt.UTC()
	if strings.TrimSpace(item.StorageDriver) == "" {
		stampReleaseStorage(&item)
	}
	return item, nil
}

func mysqlWriteVersion(db *sql.DB, kind, itemID, version string, rel sourceRelease, status string) error {
	stampReleaseStorage(&rel)
	if kind == sourceKindTemplate {
		_, err := db.Exec(`INSERT INTO source_catalog_template_versions
			(template_id, version, changelog, template_url, sha256, origin_url, storage_driver, object_key, status, review_note, reviewed_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE changelog=VALUES(changelog), template_url=VALUES(template_url), sha256=VALUES(sha256),
				origin_url=VALUES(origin_url), storage_driver=VALUES(storage_driver), object_key=VALUES(object_key),
				status=VALUES(status), review_note=VALUES(review_note), reviewed_by=VALUES(reviewed_by)`,
			itemID, version, rel.Changelog, rel.Location, rel.SHA256, rel.OriginURL, rel.StorageDriver, rel.ObjectKey, status, rel.ReviewNote, rel.ReviewedBy)
		return err
	}
	_, err := db.Exec(`INSERT INTO source_catalog_plugin_versions
		(plugin_id, version, changelog, download_url, sha256, origin_url, storage_driver, object_key, status, review_note, reviewed_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE changelog=VALUES(changelog), download_url=VALUES(download_url), sha256=VALUES(sha256),
			origin_url=VALUES(origin_url), storage_driver=VALUES(storage_driver), object_key=VALUES(object_key),
			status=VALUES(status), review_note=VALUES(review_note), reviewed_by=VALUES(reviewed_by)`,
		itemID, version, rel.Changelog, rel.Location, rel.SHA256, rel.OriginURL, rel.StorageDriver, rel.ObjectKey, status, rel.ReviewNote, rel.ReviewedBy)
	return err
}

func coalesceRelease(existing, incoming sourceRelease) sourceRelease {
	if incoming.Location != "" {
		existing.Location = incoming.Location
		existing.OriginURL = incoming.OriginURL
	}
	if incoming.SHA256 != "" {
		existing.SHA256 = incoming.SHA256
	}
	if incoming.Changelog != "" {
		existing.Changelog = incoming.Changelog
	}
	stampReleaseStorage(&existing)
	return existing
}
