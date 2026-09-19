package handler

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"auto_pro/config"
)

const (
	sourceStationSourceName = "本站软件源"
	sourceDeveloperRoleCode = "R_DEVELOPER"
	sourceDeveloperRoleName = "开发者"

	sourceApplicationPending  = "pending"
	sourceApplicationApproved = "approved"
	sourceApplicationRejected = "rejected"
	sourceApplicationFrozen   = "frozen"

	sourceItemDraft      = "draft"
	sourceItemReview     = "review"
	sourceItemApproved   = "approved"
	sourceItemPublished  = "published"
	sourceItemHidden     = "hidden"
	sourceItemRejected   = "rejected"
	sourceItemDeprecated = "deprecated"

	sourceVersionDraft      = "draft"
	sourceVersionPending    = "pending"
	sourceVersionPublished  = "published"
	sourceVersionDeprecated = "deprecated"

	sourceKindPlugin   = "plugin"
	sourceKindTemplate = "template"
)

var (
	errSourceNotFound            = errors.New("目录项不存在")
	errSourceConflict            = errors.New("标识已被占用")
	errSourceForbidden           = errors.New("只能修改自己的草稿或已驳回条目")
	errApplicationPending        = errors.New("入驻申请正在审核中")
	errApplicationReviewed       = errors.New("入驻申请已处理")
	errDeveloperDisabled         = errors.New("开发者资格已取消，无法登录")
	errApplicationNotCancellable = errors.New("只能取消已通过的开发者资格，待审核请使用拒绝")
	errDeveloperAlreadyCancelled = errors.New("开发者资格已取消")
	errSourcePublishIncomplete   = errors.New("上架需要 64 位 sha256 和外部下载/模板地址")
	errSourceInvalidStatus       = errors.New("当前状态不允许该操作")
	errSourceVersionImmutable    = errors.New("已发布版本不可改包地址，请创建新版本")
	errSourceVersionNotLatest    = errors.New("只能把 latest 指到已发布且未弃用的版本")

	sha256HexPattern     = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
	sourceVersionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$`)

	sourceStationOverride   sourceStationStore
	sourceStationOverrideMu sync.RWMutex
)

func sourceCancelNote(note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return "管理员取消开发者资格"
	}
	return truncateText(note, 500)
}

type sourceAuthor struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Email string `json:"email"`
}

type sourcePlugin struct {
	ID            string       `json:"id"`
	DeveloperID   int64        `json:"developerId"`
	Category      string       `json:"category"`
	Name          string       `json:"name"`
	Description   string       `json:"description"`
	Icon          string       `json:"icon"`
	Version       string       `json:"version"`
	Author        sourceAuthor `json:"author"`
	SHA256        string       `json:"sha256"`
	DownloadURL   string       `json:"downloadUrl"`
	Changelog     string       `json:"changelog"`
	LatestVersion string       `json:"latestVersion"`
	MinVersion    string       `json:"minVersion"`
	ForceUpdate   bool         `json:"forceUpdate"`
	Status        string       `json:"status"`
	ReviewNote    string       `json:"reviewNote"`
	ReviewedBy    string       `json:"reviewedBy"`
	UpdatedAt     time.Time    `json:"updatedAt"`
	CreatedAt     time.Time    `json:"createdAt"`
}

type sourceTemplate struct {
	ID            string       `json:"id"`
	DeveloperID   int64        `json:"developerId"`
	TemplateKey   string       `json:"templateKey"`
	Name          string       `json:"name"`
	Description   string       `json:"description"`
	Version       string       `json:"version"`
	SchemaVersion int          `json:"schemaVersion"`
	SHA256        string       `json:"sha256"`
	TemplateURL   string       `json:"templateUrl"`
	Changelog     string       `json:"changelog"`
	LatestVersion string       `json:"latestVersion"`
	MinVersion    string       `json:"minVersion"`
	ForceUpdate   bool         `json:"forceUpdate"`
	Status        string       `json:"status"`
	ReviewNote    string       `json:"reviewNote"`
	ReviewedBy    string       `json:"reviewedBy"`
	Author        sourceAuthor `json:"author"`
	UpdatedAt     time.Time    `json:"updatedAt"`
	CreatedAt     time.Time    `json:"createdAt"`
}

type sourceRelease struct {
	Kind       string    `json:"kind"`
	ItemID     string    `json:"itemId"`
	Version    string    `json:"version"`
	Changelog  string    `json:"changelog"`
	Location   string    `json:"location"`
	SHA256     string    `json:"sha256"`
	Status     string    `json:"status"`
	ReviewNote string    `json:"reviewNote"`
	ReviewedBy string    `json:"reviewedBy"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type sourceApplication struct {
	ID           int64
	Username     string
	PasswordHash string
	Email        string
	DisplayName  string
	Reason       string
	Status       string
	ReviewNote   string
	ReviewedBy   string
	ReviewedAt   *time.Time
	CreatedAt    time.Time
}

type sourceDeveloper struct {
	ID            int64
	ApplicationID int64
	Username      string
	PasswordHash  string
	Email         string
	DisplayName   string
	Enabled       bool
	CreatedAt     time.Time
}

type sourceAuditEntry struct {
	ID         int64     `json:"id"`
	ActorType  string    `json:"actorType"`
	ActorName  string    `json:"actorName"`
	Action     string    `json:"action"`
	TargetType string    `json:"targetType"`
	TargetID   string    `json:"targetId"`
	Detail     string    `json:"detail"`
	CreatedAt  time.Time `json:"createdAt"`
}

type sourceIndexSnapshot struct {
	Payload     string    `json:"payload"`
	GeneratedAt time.Time `json:"generatedAt"`
	GeneratedBy string    `json:"generatedBy"`
}

type sourceReleaseSettings struct {
	Provider    string `json:"provider"`
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	Token       string `json:"token,omitempty"`
	TagStrategy string `json:"tagStrategy"`
	Branch      string `json:"branch"`
}

type sourceStationStore interface {
	Ensure() error

	ListPlugins(status string) ([]sourcePlugin, error)
	GetPlugin(id string) (sourcePlugin, error)
	UpsertPlugin(plugin sourcePlugin, asAdmin bool) (sourcePlugin, error)
	SetPluginStatus(id, status, actor, note string) (sourcePlugin, error)

	ListTemplates(status string) ([]sourceTemplate, error)
	GetTemplate(id string) (sourceTemplate, error)
	UpsertTemplate(item sourceTemplate, asAdmin bool) (sourceTemplate, error)
	SetTemplateStatus(id, status, actor, note string) (sourceTemplate, error)

	ListVersions(kind, itemID string) ([]sourceRelease, error)
	GetVersion(kind, itemID, version string) (sourceRelease, error)
	UpsertVersion(rel sourceRelease, developerID int64, asAdmin bool) (sourceRelease, error)
	SetVersionStatus(kind, itemID, version, status, actor, note string) (sourceRelease, error)
	SetLatestVersion(kind, itemID, version, actor, note string) error

	GetReleaseSettings() (sourceReleaseSettings, error)
	SaveReleaseSettings(settings sourceReleaseSettings) error

	CreateApplication(app sourceApplication) (sourceApplication, error)
	ListApplications(status string) ([]sourceApplication, error)
	GetApplication(id int64) (sourceApplication, error)
	ApproveApplication(id int64, reviewer string) (sourceDeveloper, error)
	RejectApplication(id int64, reviewer, note string) error
	FreezeApplication(id int64, reviewer, note string) error
	GetDeveloperByUsername(username string) (sourceDeveloper, error)
	GetDeveloperByID(id int64) (sourceDeveloper, error)
	FreezeDeveloper(id int64, actor, note string) error
	ListDevelopers() ([]sourceDeveloper, error)

	ListAdvertisements(position string) ([]advertisementRecord, error)
	UpsertAdvertisement(record advertisementRecord) error
	DeleteAdvertisement(id string) error

	AppendAudit(entry sourceAuditEntry) error
	ListAudit(limit int) ([]sourceAuditEntry, error)
	SaveIndexSnapshot(payload, actor string) error
	LatestIndexSnapshot() (sourceIndexSnapshot, error)
}

func SetSourceStationStoreForTest(store sourceStationStore) func() {
	sourceStationOverrideMu.Lock()
	previous := sourceStationOverride
	sourceStationOverride = store
	sourceStationOverrideMu.Unlock()
	return func() {
		sourceStationOverrideMu.Lock()
		sourceStationOverride = previous
		sourceStationOverrideMu.Unlock()
	}
}

func currentSourceStationStore() sourceStationStore {
	sourceStationOverrideMu.RLock()
	override := sourceStationOverride
	sourceStationOverrideMu.RUnlock()
	if override != nil {
		return override
	}
	return mysqlSourceStore{}
}

func sourceContentSHA256(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func sourceItemReady(sha256Value, location string) bool {
	return sha256HexPattern.MatchString(strings.TrimSpace(sha256Value)) && strings.TrimSpace(location) != ""
}

func sourceCatalogAuditAction(status string) string {
	switch status {
	case sourceItemPublished:
		return "shelf"
	case sourceItemHidden:
		return "unshelf"
	case sourceItemApproved:
		return "approve"
	case sourceItemRejected:
		return "reject"
	case sourceItemDeprecated:
		return "deprecate"
	case sourceItemReview:
		return "submit"
	default:
		return status
	}
}

func sourceTransitionAllowed(from, to string) bool {
	if from == to {
		return true
	}
	switch to {
	case sourceItemReview:
		return from == sourceItemDraft || from == sourceItemRejected
	case sourceItemApproved:
		return from == sourceItemReview
	case sourceItemRejected:
		return from == sourceItemReview
	case sourceItemPublished:
		return from == sourceItemApproved || from == sourceItemHidden || from == sourceItemDraft || from == sourceItemReview
	case sourceItemHidden:
		return from == sourceItemPublished
	case sourceItemDeprecated:
		return from == sourceItemPublished || from == sourceItemHidden || from == sourceItemApproved
	default:
		return false
	}
}

type memorySourceStore struct {
	mu               sync.Mutex
	revision         atomic.Int64
	plugins          map[string]sourcePlugin
	templates        map[string]sourceTemplate
	pluginVersions   map[string]map[string]sourceRelease
	templateVersions map[string]map[string]sourceRelease
	applications     map[int64]sourceApplication
	developers       map[int64]sourceDeveloper
	ads              map[string]advertisementRecord
	audits           []sourceAuditEntry
	snapshot         sourceIndexSnapshot
	releaseSettings  sourceReleaseSettings
	nextAppID        int64
	nextDevID        int64
	nextAuditID      int64
}

func newMemorySourceStore() *memorySourceStore {
	store := &memorySourceStore{
		plugins:          map[string]sourcePlugin{},
		templates:        map[string]sourceTemplate{},
		pluginVersions:   map[string]map[string]sourceRelease{},
		templateVersions: map[string]map[string]sourceRelease{},
		applications:     map[int64]sourceApplication{},
		developers:       map[int64]sourceDeveloper{},
		ads:              map[string]advertisementRecord{},
		nextAppID:        1,
		nextDevID:        1,
		nextAuditID:      1,
	}
	store.revision.Store(1)
	return store
}

func (store *memorySourceStore) Ensure() error { return nil }

func (store *memorySourceStore) auditLocked(action, targetType, targetID, actor, detail string) {
	store.audits = append([]sourceAuditEntry{{
		ID: store.nextAuditID, ActorType: "admin", ActorName: actor, Action: action,
		TargetType: targetType, TargetID: targetID, Detail: detail, CreatedAt: time.Now().UTC(),
	}}, store.audits...)
	store.nextAuditID++
	if actor == "" {
		store.audits[0].ActorType = "system"
	}
}

func (store *memorySourceStore) ListPlugins(status string) ([]sourcePlugin, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]sourcePlugin, 0, len(store.plugins))
	for _, item := range store.plugins {
		if status == "" || item.Status == status {
			result = append(result, item)
		}
	}
	return result, nil
}

func (store *memorySourceStore) GetPlugin(id string) (sourcePlugin, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	item, ok := store.plugins[id]
	if !ok {
		return sourcePlugin{}, errSourceNotFound
	}
	return item, nil
}

func (store *memorySourceStore) UpsertPlugin(plugin sourcePlugin, asAdmin bool) (sourcePlugin, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	existing, exists := store.plugins[plugin.ID]
	if exists && !asAdmin && existing.DeveloperID != plugin.DeveloperID {
		return sourcePlugin{}, errSourceForbidden
	}
	now := time.Now().UTC()
	incomingVersion := strings.TrimSpace(plugin.Version)
	incomingURL := strings.TrimSpace(plugin.DownloadURL)
	incomingSHA := strings.TrimSpace(plugin.SHA256)
	incomingLog := strings.TrimSpace(plugin.Changelog)
	if exists {
		plugin.CreatedAt = existing.CreatedAt
		plugin.Status = existing.Status
		plugin.ReviewNote = existing.ReviewNote
		plugin.ReviewedBy = existing.ReviewedBy
		plugin.LatestVersion = existing.LatestVersion
		plugin.DeveloperID = existing.DeveloperID
		if existing.LatestVersion != "" {
			plugin.Version = existing.Version
			plugin.SHA256 = existing.SHA256
			plugin.DownloadURL = existing.DownloadURL
			plugin.Changelog = existing.Changelog
		}
		if incomingURL != "" && incomingURL != existing.DownloadURL && existing.LatestVersion == "" {
			actor := "developer"
			if asAdmin {
				actor = "admin"
			}
			store.auditLocked("url_change", "plugin", plugin.ID, actor, incomingURL)
		}
	} else {
		plugin.CreatedAt = now
		if plugin.Status == "" {
			plugin.Status = sourceItemDraft
		}
	}
	plugin.UpdatedAt = now
	store.plugins[plugin.ID] = plugin
	if incomingVersion != "" || incomingURL != "" || incomingSHA != "" {
		rel := sourceRelease{
			Kind: sourceKindPlugin, ItemID: plugin.ID, Version: incomingVersion,
			Changelog: incomingLog, Location: incomingURL, SHA256: incomingSHA, Status: sourceVersionDraft,
		}
		if _, err := store.upsertVersionLocked(rel, plugin.DeveloperID, asAdmin); err != nil {
			return sourcePlugin{}, err
		}
	}
	return store.plugins[plugin.ID], nil
}

func (store *memorySourceStore) SetPluginStatus(id, status, actor, note string) (sourcePlugin, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	item, ok := store.plugins[id]
	if !ok {
		return sourcePlugin{}, errSourceNotFound
	}
	if !sourceTransitionAllowed(item.Status, status) {
		return sourcePlugin{}, errSourceInvalidStatus
	}
	if err := store.applyItemStatusToVersionsLocked(sourceKindPlugin, id, status, actor, note); err != nil {
		return sourcePlugin{}, err
	}
	item = store.plugins[id]
	if status == sourceItemPublished && !sourceItemReady(item.SHA256, item.DownloadURL) {
		return sourcePlugin{}, errSourcePublishIncomplete
	}
	item.Status = status
	item.ReviewNote = truncateText(note, 500)
	item.ReviewedBy = actor
	item.UpdatedAt = time.Now().UTC()
	store.plugins[id] = item
	store.auditLocked(sourceCatalogAuditAction(status), "plugin", id, actor, note)
	return item, nil
}

func (store *memorySourceStore) ListTemplates(status string) ([]sourceTemplate, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]sourceTemplate, 0, len(store.templates))
	for _, item := range store.templates {
		if status == "" || item.Status == status {
			result = append(result, item)
		}
	}
	return result, nil
}

func (store *memorySourceStore) GetTemplate(id string) (sourceTemplate, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	item, ok := store.templates[id]
	if !ok {
		return sourceTemplate{}, errSourceNotFound
	}
	return item, nil
}

func (store *memorySourceStore) UpsertTemplate(item sourceTemplate, asAdmin bool) (sourceTemplate, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	existing, exists := store.templates[item.ID]
	if exists && !asAdmin && existing.DeveloperID != item.DeveloperID {
		return sourceTemplate{}, errSourceForbidden
	}
	now := time.Now().UTC()
	incomingVersion := strings.TrimSpace(item.Version)
	incomingURL := strings.TrimSpace(item.TemplateURL)
	incomingSHA := strings.TrimSpace(item.SHA256)
	incomingLog := strings.TrimSpace(item.Changelog)
	if exists {
		item.CreatedAt = existing.CreatedAt
		item.Status = existing.Status
		item.ReviewNote = existing.ReviewNote
		item.ReviewedBy = existing.ReviewedBy
		item.LatestVersion = existing.LatestVersion
		item.DeveloperID = existing.DeveloperID
		if existing.LatestVersion != "" {
			item.Version = existing.Version
			item.SHA256 = existing.SHA256
			item.TemplateURL = existing.TemplateURL
			item.Changelog = existing.Changelog
		}
		if existing.TemplateURL != incomingURL && incomingURL != "" && existing.LatestVersion == "" {
			actor := "developer"
			if asAdmin {
				actor = "admin"
			}
			store.auditLocked("url_change", "template", item.ID, actor, incomingURL)
		}
	} else {
		item.CreatedAt = now
		if item.Status == "" {
			item.Status = sourceItemDraft
		}
	}
	if item.SchemaVersion == 0 {
		item.SchemaVersion = homeTemplateSchemaVersion
	}
	item.UpdatedAt = now
	store.templates[item.ID] = item
	if incomingVersion != "" || incomingURL != "" || incomingSHA != "" {
		rel := sourceRelease{
			Kind: sourceKindTemplate, ItemID: item.ID, Version: incomingVersion,
			Changelog: incomingLog, Location: incomingURL, SHA256: incomingSHA, Status: sourceVersionDraft,
		}
		if _, err := store.upsertVersionLocked(rel, item.DeveloperID, asAdmin); err != nil {
			return sourceTemplate{}, err
		}
	}
	return store.templates[item.ID], nil
}

func (store *memorySourceStore) SetTemplateStatus(id, status, actor, note string) (sourceTemplate, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	item, ok := store.templates[id]
	if !ok {
		return sourceTemplate{}, errSourceNotFound
	}
	if !sourceTransitionAllowed(item.Status, status) {
		return sourceTemplate{}, errSourceInvalidStatus
	}
	if err := store.applyItemStatusToVersionsLocked(sourceKindTemplate, id, status, actor, note); err != nil {
		return sourceTemplate{}, err
	}
	item = store.templates[id]
	if status == sourceItemPublished && !sourceItemReady(item.SHA256, item.TemplateURL) {
		return sourceTemplate{}, errSourcePublishIncomplete
	}
	item.Status = status
	item.ReviewNote = truncateText(note, 500)
	item.ReviewedBy = actor
	item.UpdatedAt = time.Now().UTC()
	store.templates[id] = item
	store.auditLocked(sourceCatalogAuditAction(status), "template", id, actor, note)
	return item, nil
}

func (store *memorySourceStore) CreateApplication(app sourceApplication) (sourceApplication, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	username := strings.ToLower(strings.TrimSpace(app.Username))
	for _, existing := range store.applications {
		if strings.ToLower(existing.Username) == username {
			if existing.Status == sourceApplicationPending {
				return sourceApplication{}, errApplicationPending
			}
			return sourceApplication{}, errSourceConflict
		}
	}
	for _, existing := range store.developers {
		if strings.ToLower(existing.Username) == username {
			return sourceApplication{}, errSourceConflict
		}
	}
	app.ID = store.nextAppID
	store.nextAppID++
	app.Status = sourceApplicationPending
	app.CreatedAt = time.Now().UTC()
	store.applications[app.ID] = app
	return app, nil
}

func (store *memorySourceStore) ListApplications(status string) ([]sourceApplication, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]sourceApplication, 0, len(store.applications))
	for _, item := range store.applications {
		if status == "" || item.Status == status {
			copyItem := item
			copyItem.PasswordHash = ""
			result = append(result, copyItem)
		}
	}
	return result, nil
}

func (store *memorySourceStore) GetApplication(id int64) (sourceApplication, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	item, ok := store.applications[id]
	if !ok {
		return sourceApplication{}, errSourceNotFound
	}
	return item, nil
}

func (store *memorySourceStore) ApproveApplication(id int64, reviewer string) (sourceDeveloper, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	app, ok := store.applications[id]
	if !ok {
		return sourceDeveloper{}, errSourceNotFound
	}
	if app.Status != sourceApplicationPending {
		return sourceDeveloper{}, errApplicationReviewed
	}
	now := time.Now().UTC()
	app.Status = sourceApplicationApproved
	app.ReviewedBy = reviewer
	app.ReviewedAt = &now
	store.applications[id] = app
	dev := sourceDeveloper{
		ID: store.nextDevID, ApplicationID: app.ID, Username: app.Username, PasswordHash: app.PasswordHash,
		Email: app.Email, DisplayName: app.DisplayName, Enabled: true, CreatedAt: now,
	}
	store.nextDevID++
	store.developers[dev.ID] = dev
	store.auditLocked("approve", "application", itoaSourceID(id), reviewer, "")
	return dev, nil
}

func (store *memorySourceStore) RejectApplication(id int64, reviewer, note string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	app, ok := store.applications[id]
	if !ok {
		return errSourceNotFound
	}
	if app.Status != sourceApplicationPending {
		return errApplicationReviewed
	}
	now := time.Now().UTC()
	app.Status = sourceApplicationRejected
	app.ReviewedBy = reviewer
	app.ReviewNote = note
	app.ReviewedAt = &now
	store.applications[id] = app
	store.auditLocked("reject", "application", itoaSourceID(id), reviewer, note)
	return nil
}

func (store *memorySourceStore) FreezeApplication(id int64, reviewer, note string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	app, ok := store.applications[id]
	if !ok {
		return errSourceNotFound
	}
	if app.Status == sourceApplicationFrozen {
		return errDeveloperAlreadyCancelled
	}
	if app.Status != sourceApplicationApproved {
		return errApplicationNotCancellable
	}
	now := time.Now().UTC()
	app.Status = sourceApplicationFrozen
	app.ReviewedBy = reviewer
	app.ReviewNote = sourceCancelNote(note)
	app.ReviewedAt = &now
	store.applications[id] = app
	for idKey, dev := range store.developers {
		if dev.ApplicationID == app.ID || strings.EqualFold(dev.Username, app.Username) {
			dev.Enabled = false
			store.developers[idKey] = dev
		}
	}
	store.auditLocked("cancel", "application", itoaSourceID(id), reviewer, app.ReviewNote)
	return nil
}

func (store *memorySourceStore) GetDeveloperByUsername(username string) (sourceDeveloper, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	username = strings.ToLower(strings.TrimSpace(username))
	for _, item := range store.developers {
		if strings.ToLower(item.Username) == username {
			return item, nil
		}
	}
	return sourceDeveloper{}, errSourceNotFound
}

func (store *memorySourceStore) GetDeveloperByID(id int64) (sourceDeveloper, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	item, ok := store.developers[id]
	if !ok {
		return sourceDeveloper{}, errSourceNotFound
	}
	return item, nil
}

func (store *memorySourceStore) FreezeDeveloper(id int64, actor, note string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	item, ok := store.developers[id]
	if !ok {
		return errSourceNotFound
	}
	if !item.Enabled {
		return errDeveloperAlreadyCancelled
	}
	item.Enabled = false
	store.developers[id] = item
	note = sourceCancelNote(note)
	now := time.Now().UTC()
	for appID, app := range store.applications {
		if app.ID == item.ApplicationID || strings.EqualFold(app.Username, item.Username) {
			app.Status = sourceApplicationFrozen
			app.ReviewedBy = actor
			app.ReviewNote = note
			app.ReviewedAt = &now
			store.applications[appID] = app
		}
	}
	store.auditLocked("cancel", "developer", itoaSourceID(id), actor, note)
	return nil
}

func (store *memorySourceStore) ListDevelopers() ([]sourceDeveloper, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]sourceDeveloper, 0, len(store.developers))
	for _, item := range store.developers {
		copyItem := item
		copyItem.PasswordHash = ""
		result = append(result, copyItem)
	}
	return result, nil
}

func (store *memorySourceStore) ListAdvertisements(position string) ([]advertisementRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]advertisementRecord, 0, len(store.ads))
	for _, item := range store.ads {
		if position == "" || item.Position == position {
			result = append(result, item)
		}
	}
	return result, nil
}

func (store *memorySourceStore) UpsertAdvertisement(record advertisementRecord) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ads[record.ID] = record
	return nil
}

func (store *memorySourceStore) DeleteAdvertisement(id string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.ads[id]; !ok {
		return errSourceNotFound
	}
	delete(store.ads, id)
	return nil
}

func (store *memorySourceStore) AppendAudit(entry sourceAuditEntry) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	entry.ID = store.nextAuditID
	store.nextAuditID++
	store.audits = append([]sourceAuditEntry{entry}, store.audits...)
	return nil
}

func (store *memorySourceStore) ListAudit(limit int) ([]sourceAuditEntry, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if limit <= 0 || limit > len(store.audits) {
		limit = len(store.audits)
	}
	return append([]sourceAuditEntry(nil), store.audits[:limit]...), nil
}

func (store *memorySourceStore) SaveIndexSnapshot(payload, actor string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.snapshot = sourceIndexSnapshot{Payload: payload, GeneratedAt: time.Now().UTC(), GeneratedBy: actor}
	store.auditLocked("regenerate", "index", "index.json", actor, "")
	return nil
}

func (store *memorySourceStore) LatestIndexSnapshot() (sourceIndexSnapshot, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.snapshot, nil
}

func (store *memorySourceStore) GetReleaseSettings() (sourceReleaseSettings, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return normalizeReleaseSettings(store.releaseSettings), nil
}

func (store *memorySourceStore) SaveReleaseSettings(settings sourceReleaseSettings) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.releaseSettings = normalizeReleaseSettings(settings)
	return nil
}

func itoaSourceID(value int64) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	n := value
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[i:])
}

type mysqlSourceStore struct{}

func (mysqlSourceStore) Ensure() error {
	db, err := config.DB()
	if err != nil {
		return err
	}
	return ensureSourceStationStorage(db)
}

func ensureSourceStationStorage(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS source_developer_applications (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			username VARCHAR(50) NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			email VARCHAR(100) NOT NULL DEFAULT '',
			display_name VARCHAR(80) NOT NULL DEFAULT '',
			reason VARCHAR(500) NOT NULL DEFAULT '',
			status VARCHAR(20) NOT NULL DEFAULT 'pending',
			review_note VARCHAR(500) NOT NULL DEFAULT '',
			reviewed_by VARCHAR(50) NOT NULL DEFAULT '',
			reviewed_at DATETIME DEFAULT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_source_developer_application_username (username),
			KEY idx_source_developer_application_status (status)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='软件源开发者入驻申请'`,
		`CREATE TABLE IF NOT EXISTS source_developers (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			application_id BIGINT UNSIGNED DEFAULT NULL,
			username VARCHAR(50) NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			email VARCHAR(100) NOT NULL DEFAULT '',
			display_name VARCHAR(80) NOT NULL DEFAULT '',
			enabled TINYINT(1) NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_source_developer_username (username)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='软件源开发者'`,
		`CREATE TABLE IF NOT EXISTS source_catalog_plugins (
			id VARCHAR(60) NOT NULL PRIMARY KEY,
			developer_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
			category VARCHAR(30) NOT NULL DEFAULT 'other',
			name VARCHAR(100) NOT NULL,
			description VARCHAR(500) NOT NULL DEFAULT '',
			icon VARCHAR(80) NOT NULL DEFAULT 'ri:puzzle-line',
			version VARCHAR(40) NOT NULL,
			author_name VARCHAR(100) NOT NULL DEFAULT '',
			author_url VARCHAR(300) NOT NULL DEFAULT '',
			author_email VARCHAR(200) NOT NULL DEFAULT '',
			sha256 CHAR(64) NOT NULL DEFAULT '',
			download_url VARCHAR(500) NOT NULL DEFAULT '',
			status VARCHAR(20) NOT NULL DEFAULT 'draft',
			review_note VARCHAR(500) NOT NULL DEFAULT '',
			reviewed_by VARCHAR(50) NOT NULL DEFAULT '',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			KEY idx_source_catalog_plugin_status (status),
			KEY idx_source_catalog_plugin_developer (developer_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='软件源插件元数据（不含源码）'`,
		`CREATE TABLE IF NOT EXISTS source_catalog_templates (
			id VARCHAR(60) NOT NULL PRIMARY KEY,
			developer_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
			template_key VARCHAR(60) NOT NULL,
			name VARCHAR(100) NOT NULL,
			description VARCHAR(500) NOT NULL DEFAULT '',
			version VARCHAR(40) NOT NULL,
			schema_version INT NOT NULL DEFAULT 1,
			sha256 CHAR(64) NOT NULL DEFAULT '',
			template_url VARCHAR(500) NOT NULL DEFAULT '',
			status VARCHAR(20) NOT NULL DEFAULT 'draft',
			review_note VARCHAR(500) NOT NULL DEFAULT '',
			reviewed_by VARCHAR(50) NOT NULL DEFAULT '',
			author_name VARCHAR(100) NOT NULL DEFAULT '',
			author_url VARCHAR(300) NOT NULL DEFAULT '',
			author_email VARCHAR(200) NOT NULL DEFAULT '',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_source_catalog_template_key (template_key),
			KEY idx_source_catalog_template_status (status)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='软件源首页模板元数据（不含源码）'`,
		`CREATE TABLE IF NOT EXISTS source_station_settings (
			setting_key VARCHAR(50) NOT NULL PRIMARY KEY,
			setting_value TEXT NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='软件源源站设置（含 Release 推送配置）'`,
		`CREATE TABLE IF NOT EXISTS source_advertisements (
			id VARCHAR(60) NOT NULL PRIMARY KEY,
			title VARCHAR(120) NOT NULL DEFAULT '',
			image_url VARCHAR(500) NOT NULL DEFAULT '',
			destination_url VARCHAR(500) NOT NULL DEFAULT '',
			position VARCHAR(30) NOT NULL,
			weight INT NOT NULL DEFAULT 0,
			start_at VARCHAR(40) NOT NULL DEFAULT '',
			end_at VARCHAR(40) NOT NULL DEFAULT '',
			description VARCHAR(500) NOT NULL DEFAULT '',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			KEY idx_source_advertisement_position (position)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='本站广告投放'`,
		`CREATE TABLE IF NOT EXISTS source_audit_logs (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			actor_type VARCHAR(20) NOT NULL DEFAULT 'admin',
			actor_name VARCHAR(80) NOT NULL DEFAULT '',
			action VARCHAR(40) NOT NULL,
			target_type VARCHAR(40) NOT NULL,
			target_id VARCHAR(80) NOT NULL DEFAULT '',
			detail VARCHAR(500) NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			KEY idx_source_audit_created (created_at),
			KEY idx_source_audit_target (target_type, target_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='软件源控制面审计'`,
		`CREATE TABLE IF NOT EXISTS source_index_snapshots (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			payload MEDIUMTEXT NOT NULL,
			generated_by VARCHAR(80) NOT NULL DEFAULT '',
			generated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='公开 index.json 快照'`,
		`CREATE TABLE IF NOT EXISTS source_catalog_plugin_versions (
			plugin_id VARCHAR(60) NOT NULL,
			version VARCHAR(40) NOT NULL,
			changelog VARCHAR(2000) NOT NULL DEFAULT '',
			download_url VARCHAR(500) NOT NULL DEFAULT '',
			sha256 CHAR(64) NOT NULL DEFAULT '',
			status VARCHAR(20) NOT NULL DEFAULT 'draft',
			review_note VARCHAR(500) NOT NULL DEFAULT '',
			reviewed_by VARCHAR(50) NOT NULL DEFAULT '',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (plugin_id, version),
			KEY idx_source_plugin_version_status (status)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='插件版本元数据（不含源码）'`,
		`CREATE TABLE IF NOT EXISTS source_catalog_template_versions (
			template_id VARCHAR(60) NOT NULL,
			version VARCHAR(40) NOT NULL,
			changelog VARCHAR(2000) NOT NULL DEFAULT '',
			template_url VARCHAR(500) NOT NULL DEFAULT '',
			sha256 CHAR(64) NOT NULL DEFAULT '',
			status VARCHAR(20) NOT NULL DEFAULT 'draft',
			review_note VARCHAR(500) NOT NULL DEFAULT '',
			reviewed_by VARCHAR(50) NOT NULL DEFAULT '',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (template_id, version),
			KEY idx_source_template_version_status (status)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='首页模板版本元数据（不含源码）'`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	alters := []string{
		"ALTER TABLE source_catalog_plugins ADD COLUMN download_url VARCHAR(500) NOT NULL DEFAULT '' AFTER sha256",
		"ALTER TABLE source_catalog_plugins ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'draft' AFTER download_url",
		"ALTER TABLE source_catalog_plugins ADD COLUMN review_note VARCHAR(500) NOT NULL DEFAULT '' AFTER status",
		"ALTER TABLE source_catalog_plugins ADD COLUMN reviewed_by VARCHAR(50) NOT NULL DEFAULT '' AFTER review_note",
		"ALTER TABLE source_catalog_templates ADD COLUMN template_url VARCHAR(500) NOT NULL DEFAULT '' AFTER sha256",
		"ALTER TABLE source_catalog_templates ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'draft' AFTER template_url",
		"ALTER TABLE source_catalog_templates ADD COLUMN review_note VARCHAR(500) NOT NULL DEFAULT '' AFTER status",
		"ALTER TABLE source_catalog_templates ADD COLUMN reviewed_by VARCHAR(50) NOT NULL DEFAULT '' AFTER review_note",
		"ALTER TABLE source_catalog_plugins DROP COLUMN file_path",
		"ALTER TABLE source_catalog_plugins DROP COLUMN published",
		"ALTER TABLE source_catalog_templates DROP COLUMN file_path",
		"ALTER TABLE source_catalog_templates DROP COLUMN preview_path",
		"ALTER TABLE source_catalog_templates DROP COLUMN preview_content_type",
		"ALTER TABLE source_catalog_templates DROP COLUMN format",
		"ALTER TABLE source_catalog_templates DROP COLUMN published",
		"ALTER TABLE source_catalog_plugins ADD COLUMN latest_version VARCHAR(40) NOT NULL DEFAULT '' AFTER version",
		"ALTER TABLE source_catalog_plugins ADD COLUMN min_version VARCHAR(40) NOT NULL DEFAULT '' AFTER latest_version",
		"ALTER TABLE source_catalog_plugins ADD COLUMN force_update TINYINT(1) NOT NULL DEFAULT 0 AFTER min_version",
		"ALTER TABLE source_catalog_plugins ADD COLUMN changelog VARCHAR(2000) NOT NULL DEFAULT '' AFTER download_url",
		"ALTER TABLE source_catalog_templates ADD COLUMN latest_version VARCHAR(40) NOT NULL DEFAULT '' AFTER version",
		"ALTER TABLE source_catalog_templates ADD COLUMN min_version VARCHAR(40) NOT NULL DEFAULT '' AFTER latest_version",
		"ALTER TABLE source_catalog_templates ADD COLUMN force_update TINYINT(1) NOT NULL DEFAULT 0 AFTER min_version",
		"ALTER TABLE source_catalog_templates ADD COLUMN changelog VARCHAR(2000) NOT NULL DEFAULT '' AFTER template_url",
	}
	for _, statement := range alters {
		_, _ = db.Exec(statement)
	}
	_, _ = db.Exec(`INSERT IGNORE INTO source_catalog_plugin_versions
		(plugin_id, version, changelog, download_url, sha256, status, created_at, updated_at)
		SELECT id, IF(version='', '0.0.0', version), '', download_url, sha256,
			CASE status WHEN 'published' THEN 'published' WHEN 'review' THEN 'pending' WHEN 'deprecated' THEN 'deprecated' ELSE 'draft' END,
			created_at, updated_at FROM source_catalog_plugins`)
	_, _ = db.Exec(`INSERT IGNORE INTO source_catalog_template_versions
		(template_id, version, changelog, template_url, sha256, status, created_at, updated_at)
		SELECT id, IF(version='', '0.0.0', version), '', template_url, sha256,
			CASE status WHEN 'published' THEN 'published' WHEN 'review' THEN 'pending' WHEN 'deprecated' THEN 'deprecated' ELSE 'draft' END,
			created_at, updated_at FROM source_catalog_templates`)
	_, _ = db.Exec(`UPDATE source_catalog_plugins SET latest_version=version WHERE status='published' AND latest_version=''`)
	_, _ = db.Exec(`UPDATE source_catalog_templates SET latest_version=version WHERE status='published' AND latest_version=''`)
	_, _ = db.Exec(`INSERT IGNORE INTO roles (role_name, role_code, description, discount, enabled)
		VALUES (?, ?, '软件源开发者，可提交插件与首页模板元数据', 10.0, 1)`,
		sourceDeveloperRoleName, sourceDeveloperRoleCode)
	return nil
}

func mysqlAppendAudit(db *sql.DB, action, targetType, targetID, actor, detail string) {
	actorType := "admin"
	if actor == "" {
		actorType = "system"
	}
	_, _ = db.Exec(`INSERT INTO source_audit_logs (actor_type, actor_name, action, target_type, target_id, detail)
		VALUES (?, ?, ?, ?, ?, ?)`, actorType, actor, action, targetType, targetID, truncateText(detail, 500))
}

func scanSourcePlugin(scanner interface{ Scan(dest ...any) error }) (sourcePlugin, error) {
	var item sourcePlugin
	var updatedAt, createdAt time.Time
	var forceUpdate int
	err := scanner.Scan(&item.ID, &item.DeveloperID, &item.Category, &item.Name, &item.Description, &item.Icon,
		&item.Version, &item.LatestVersion, &item.MinVersion, &forceUpdate, &item.Author.Name, &item.Author.URL, &item.Author.Email,
		&item.SHA256, &item.DownloadURL, &item.Changelog, &item.Status, &item.ReviewNote, &item.ReviewedBy, &updatedAt, &createdAt)
	if err != nil {
		return sourcePlugin{}, err
	}
	item.ForceUpdate = forceUpdate == 1
	item.UpdatedAt, item.CreatedAt = updatedAt.UTC(), createdAt.UTC()
	return item, nil
}

func (mysqlSourceStore) ListPlugins(status string) ([]sourcePlugin, error) {
	db, err := config.DB()
	if err != nil {
		return nil, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return nil, err
	}
	query := `SELECT id, developer_id, category, name, description, icon, version, latest_version, min_version, force_update, author_name, author_url, author_email,
		sha256, download_url, changelog, status, review_note, reviewed_by, updated_at, created_at FROM source_catalog_plugins`
	args := []any{}
	if status != "" {
		query += ` WHERE status=?`
		args = append(args, status)
	}
	query += ` ORDER BY updated_at DESC, id ASC`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]sourcePlugin, 0)
	for rows.Next() {
		item, err := scanSourcePlugin(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (mysqlSourceStore) GetPlugin(id string) (sourcePlugin, error) {
	db, err := config.DB()
	if err != nil {
		return sourcePlugin{}, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return sourcePlugin{}, err
	}
	item, err := scanSourcePlugin(db.QueryRow(`SELECT id, developer_id, category, name, description, icon, version, latest_version, min_version, force_update, author_name, author_url, author_email,
		sha256, download_url, changelog, status, review_note, reviewed_by, updated_at, created_at FROM source_catalog_plugins WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return sourcePlugin{}, errSourceNotFound
	}
	return item, err
}

func (mysqlSourceStore) UpsertPlugin(plugin sourcePlugin, asAdmin bool) (sourcePlugin, error) {
	db, err := config.DB()
	if err != nil {
		return sourcePlugin{}, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return sourcePlugin{}, err
	}
	existing, err := (mysqlSourceStore{}).GetPlugin(plugin.ID)
	if err != nil && !errors.Is(err, errSourceNotFound) {
		return sourcePlugin{}, err
	}
	incomingVersion := strings.TrimSpace(plugin.Version)
	incomingURL := strings.TrimSpace(plugin.DownloadURL)
	incomingSHA := strings.TrimSpace(plugin.SHA256)
	incomingLog := strings.TrimSpace(plugin.Changelog)
	forceUpdate := 0
	if plugin.ForceUpdate {
		forceUpdate = 1
	}
	if err == nil {
		if !asAdmin && existing.DeveloperID != plugin.DeveloperID {
			return sourcePlugin{}, errSourceForbidden
		}
		plugin.Status = existing.Status
		plugin.ReviewNote = existing.ReviewNote
		plugin.ReviewedBy = existing.ReviewedBy
		plugin.LatestVersion = existing.LatestVersion
		plugin.DeveloperID = existing.DeveloperID
		if existing.LatestVersion != "" {
			plugin.Version = existing.Version
			plugin.SHA256 = existing.SHA256
			plugin.DownloadURL = existing.DownloadURL
			plugin.Changelog = existing.Changelog
		}
		if incomingURL != "" && incomingURL != existing.DownloadURL && existing.LatestVersion == "" {
			actor := "developer"
			if asAdmin {
				actor = "admin"
			}
			mysqlAppendAudit(db, "url_change", "plugin", plugin.ID, actor, incomingURL)
		}
	} else if plugin.Status == "" {
		plugin.Status = sourceItemDraft
	}
	_, err = db.Exec(`INSERT INTO source_catalog_plugins
		(id, developer_id, category, name, description, icon, version, latest_version, min_version, force_update, author_name, author_url, author_email, sha256, download_url, changelog, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE category=VALUES(category), name=VALUES(name), description=VALUES(description), icon=VALUES(icon),
			min_version=VALUES(min_version), force_update=VALUES(force_update), author_name=VALUES(author_name), author_url=VALUES(author_url),
			author_email=VALUES(author_email)`,
		plugin.ID, plugin.DeveloperID, plugin.Category, plugin.Name, plugin.Description, plugin.Icon, plugin.Version,
		plugin.LatestVersion, plugin.MinVersion, forceUpdate, plugin.Author.Name, plugin.Author.URL, plugin.Author.Email,
		plugin.SHA256, plugin.DownloadURL, plugin.Changelog, plugin.Status)
	if err != nil {
		return sourcePlugin{}, err
	}
	if incomingVersion != "" || incomingURL != "" || incomingSHA != "" {
		if _, err := (mysqlSourceStore{}).UpsertVersion(sourceRelease{
			Kind: sourceKindPlugin, ItemID: plugin.ID, Version: incomingVersion,
			Changelog: incomingLog, Location: incomingURL, SHA256: incomingSHA, Status: sourceVersionDraft,
		}, plugin.DeveloperID, asAdmin); err != nil {
			return sourcePlugin{}, err
		}
	}
	return (mysqlSourceStore{}).GetPlugin(plugin.ID)
}

func (mysqlSourceStore) SetPluginStatus(id, status, actor, note string) (sourcePlugin, error) {
	item, err := (mysqlSourceStore{}).GetPlugin(id)
	if err != nil {
		return sourcePlugin{}, err
	}
	if !sourceTransitionAllowed(item.Status, status) {
		return sourcePlugin{}, errSourceInvalidStatus
	}
	if err := (mysqlSourceStore{}).applyItemStatusToVersions(sourceKindPlugin, id, status, actor, note); err != nil {
		return sourcePlugin{}, err
	}
	item, err = (mysqlSourceStore{}).GetPlugin(id)
	if err != nil {
		return sourcePlugin{}, err
	}
	if status == sourceItemPublished && !sourceItemReady(item.SHA256, item.DownloadURL) {
		return sourcePlugin{}, errSourcePublishIncomplete
	}
	db, err := config.DB()
	if err != nil {
		return sourcePlugin{}, err
	}
	if _, err := db.Exec(`UPDATE source_catalog_plugins SET status=?, review_note=?, reviewed_by=? WHERE id=?`,
		status, truncateText(note, 500), actor, id); err != nil {
		return sourcePlugin{}, err
	}
	mysqlAppendAudit(db, sourceCatalogAuditAction(status), "plugin", id, actor, note)
	return (mysqlSourceStore{}).GetPlugin(id)
}

func scanSourceTemplateRow(scanner interface{ Scan(dest ...any) error }) (sourceTemplate, error) {
	var item sourceTemplate
	var updatedAt, createdAt time.Time
	var forceUpdate int
	err := scanner.Scan(&item.ID, &item.DeveloperID, &item.TemplateKey, &item.Name, &item.Description, &item.Version,
		&item.LatestVersion, &item.MinVersion, &forceUpdate, &item.SchemaVersion, &item.SHA256, &item.TemplateURL, &item.Changelog,
		&item.Status, &item.ReviewNote, &item.ReviewedBy, &item.Author.Name, &item.Author.URL, &item.Author.Email, &updatedAt, &createdAt)
	if err != nil {
		return sourceTemplate{}, err
	}
	item.ForceUpdate = forceUpdate == 1
	item.UpdatedAt, item.CreatedAt = updatedAt.UTC(), createdAt.UTC()
	return item, nil
}

func (mysqlSourceStore) ListTemplates(status string) ([]sourceTemplate, error) {
	db, err := config.DB()
	if err != nil {
		return nil, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return nil, err
	}
	query := `SELECT id, developer_id, template_key, name, description, version, latest_version, min_version, force_update, schema_version, sha256, template_url, changelog,
		status, review_note, reviewed_by, author_name, author_url, author_email, updated_at, created_at FROM source_catalog_templates`
	args := []any{}
	if status != "" {
		query += ` WHERE status=?`
		args = append(args, status)
	}
	query += ` ORDER BY updated_at DESC, id ASC`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]sourceTemplate, 0)
	for rows.Next() {
		item, err := scanSourceTemplateRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (mysqlSourceStore) GetTemplate(id string) (sourceTemplate, error) {
	db, err := config.DB()
	if err != nil {
		return sourceTemplate{}, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return sourceTemplate{}, err
	}
	item, err := scanSourceTemplateRow(db.QueryRow(`SELECT id, developer_id, template_key, name, description, version, latest_version, min_version, force_update, schema_version, sha256, template_url, changelog,
		status, review_note, reviewed_by, author_name, author_url, author_email, updated_at, created_at FROM source_catalog_templates WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return sourceTemplate{}, errSourceNotFound
	}
	return item, err
}

func (mysqlSourceStore) UpsertTemplate(item sourceTemplate, asAdmin bool) (sourceTemplate, error) {
	db, err := config.DB()
	if err != nil {
		return sourceTemplate{}, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return sourceTemplate{}, err
	}
	if item.SchemaVersion == 0 {
		item.SchemaVersion = homeTemplateSchemaVersion
	}
	existing, err := (mysqlSourceStore{}).GetTemplate(item.ID)
	if err != nil && !errors.Is(err, errSourceNotFound) {
		return sourceTemplate{}, err
	}
	incomingVersion := strings.TrimSpace(item.Version)
	incomingURL := strings.TrimSpace(item.TemplateURL)
	incomingSHA := strings.TrimSpace(item.SHA256)
	incomingLog := strings.TrimSpace(item.Changelog)
	forceUpdate := 0
	if item.ForceUpdate {
		forceUpdate = 1
	}
	if err == nil {
		if !asAdmin && existing.DeveloperID != item.DeveloperID {
			return sourceTemplate{}, errSourceForbidden
		}
		item.Status = existing.Status
		item.ReviewNote = existing.ReviewNote
		item.ReviewedBy = existing.ReviewedBy
		item.LatestVersion = existing.LatestVersion
		item.DeveloperID = existing.DeveloperID
		if existing.LatestVersion != "" {
			item.Version = existing.Version
			item.SHA256 = existing.SHA256
			item.TemplateURL = existing.TemplateURL
			item.Changelog = existing.Changelog
		}
		if incomingURL != "" && existing.TemplateURL != incomingURL && existing.LatestVersion == "" {
			actor := "developer"
			if asAdmin {
				actor = "admin"
			}
			mysqlAppendAudit(db, "url_change", "template", item.ID, actor, incomingURL)
		}
	} else if item.Status == "" {
		item.Status = sourceItemDraft
	}
	_, err = db.Exec(`INSERT INTO source_catalog_templates
		(id, developer_id, template_key, name, description, version, latest_version, min_version, force_update, schema_version, sha256, template_url, changelog, status, author_name, author_url, author_email)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE name=VALUES(name), description=VALUES(description), schema_version=VALUES(schema_version),
			min_version=VALUES(min_version), force_update=VALUES(force_update),
			author_name=VALUES(author_name), author_url=VALUES(author_url), author_email=VALUES(author_email)`,
		item.ID, item.DeveloperID, item.TemplateKey, item.Name, item.Description, item.Version, item.LatestVersion, item.MinVersion, forceUpdate,
		item.SchemaVersion, item.SHA256, item.TemplateURL, item.Changelog, item.Status, item.Author.Name, item.Author.URL, item.Author.Email)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			return sourceTemplate{}, errSourceConflict
		}
		return sourceTemplate{}, err
	}
	if incomingVersion != "" || incomingURL != "" || incomingSHA != "" {
		if _, err := (mysqlSourceStore{}).UpsertVersion(sourceRelease{
			Kind: sourceKindTemplate, ItemID: item.ID, Version: incomingVersion,
			Changelog: incomingLog, Location: incomingURL, SHA256: incomingSHA, Status: sourceVersionDraft,
		}, item.DeveloperID, asAdmin); err != nil {
			return sourceTemplate{}, err
		}
	}
	return (mysqlSourceStore{}).GetTemplate(item.ID)
}

func (mysqlSourceStore) SetTemplateStatus(id, status, actor, note string) (sourceTemplate, error) {
	item, err := (mysqlSourceStore{}).GetTemplate(id)
	if err != nil {
		return sourceTemplate{}, err
	}
	if !sourceTransitionAllowed(item.Status, status) {
		return sourceTemplate{}, errSourceInvalidStatus
	}
	if err := (mysqlSourceStore{}).applyItemStatusToVersions(sourceKindTemplate, id, status, actor, note); err != nil {
		return sourceTemplate{}, err
	}
	item, err = (mysqlSourceStore{}).GetTemplate(id)
	if err != nil {
		return sourceTemplate{}, err
	}
	if status == sourceItemPublished && !sourceItemReady(item.SHA256, item.TemplateURL) {
		return sourceTemplate{}, errSourcePublishIncomplete
	}
	db, err := config.DB()
	if err != nil {
		return sourceTemplate{}, err
	}
	if _, err := db.Exec(`UPDATE source_catalog_templates SET status=?, review_note=?, reviewed_by=? WHERE id=?`,
		status, truncateText(note, 500), actor, id); err != nil {
		return sourceTemplate{}, err
	}
	mysqlAppendAudit(db, sourceCatalogAuditAction(status), "template", id, actor, note)
	return (mysqlSourceStore{}).GetTemplate(id)
}

func (mysqlSourceStore) CreateApplication(app sourceApplication) (sourceApplication, error) {
	db, err := config.DB()
	if err != nil {
		return sourceApplication{}, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return sourceApplication{}, err
	}
	var existingStatus string
	err = db.QueryRow(`SELECT status FROM source_developer_applications WHERE username=?`, app.Username).Scan(&existingStatus)
	if err == nil {
		if existingStatus == sourceApplicationPending {
			return sourceApplication{}, errApplicationPending
		}
		return sourceApplication{}, errSourceConflict
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return sourceApplication{}, err
	}
	var developerID int64
	if err := db.QueryRow(`SELECT id FROM source_developers WHERE username=?`, app.Username).Scan(&developerID); err == nil {
		return sourceApplication{}, errSourceConflict
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return sourceApplication{}, err
	}
	result, err := db.Exec(`INSERT INTO source_developer_applications
		(username, password_hash, email, display_name, reason, status) VALUES (?, ?, ?, ?, ?, 'pending')`,
		app.Username, app.PasswordHash, app.Email, app.DisplayName, app.Reason)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			return sourceApplication{}, errSourceConflict
		}
		return sourceApplication{}, err
	}
	id, _ := result.LastInsertId()
	return (mysqlSourceStore{}).GetApplication(id)
}

func (mysqlSourceStore) ListApplications(status string) ([]sourceApplication, error) {
	db, err := config.DB()
	if err != nil {
		return nil, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return nil, err
	}
	query := `SELECT id, username, email, display_name, reason, status, review_note, reviewed_by, reviewed_at, created_at
		FROM source_developer_applications`
	args := []any{}
	if status != "" {
		query += ` WHERE status=?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC, id DESC`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]sourceApplication, 0)
	for rows.Next() {
		item, err := scanSourceApplication(rows, false)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func scanSourceApplication(scanner interface{ Scan(dest ...any) error }, withPassword bool) (sourceApplication, error) {
	var item sourceApplication
	var reviewedAt sql.NullTime
	var err error
	if withPassword {
		err = scanner.Scan(&item.ID, &item.Username, &item.PasswordHash, &item.Email, &item.DisplayName, &item.Reason,
			&item.Status, &item.ReviewNote, &item.ReviewedBy, &reviewedAt, &item.CreatedAt)
	} else {
		err = scanner.Scan(&item.ID, &item.Username, &item.Email, &item.DisplayName, &item.Reason,
			&item.Status, &item.ReviewNote, &item.ReviewedBy, &reviewedAt, &item.CreatedAt)
	}
	if err != nil {
		return sourceApplication{}, err
	}
	if reviewedAt.Valid {
		t := reviewedAt.Time.UTC()
		item.ReviewedAt = &t
	}
	item.CreatedAt = item.CreatedAt.UTC()
	return item, nil
}

func (mysqlSourceStore) GetApplication(id int64) (sourceApplication, error) {
	db, err := config.DB()
	if err != nil {
		return sourceApplication{}, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return sourceApplication{}, err
	}
	item, err := scanSourceApplication(db.QueryRow(`SELECT id, username, password_hash, email, display_name, reason, status, review_note, reviewed_by, reviewed_at, created_at
		FROM source_developer_applications WHERE id=?`, id), true)
	if errors.Is(err, sql.ErrNoRows) {
		return sourceApplication{}, errSourceNotFound
	}
	return item, err
}

func (mysqlSourceStore) ApproveApplication(id int64, reviewer string) (sourceDeveloper, error) {
	db, err := config.DB()
	if err != nil {
		return sourceDeveloper{}, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return sourceDeveloper{}, err
	}
	tx, err := db.Begin()
	if err != nil {
		return sourceDeveloper{}, err
	}
	defer tx.Rollback()
	app, err := scanSourceApplication(tx.QueryRow(`SELECT id, username, password_hash, email, display_name, reason, status, review_note, reviewed_by, reviewed_at, created_at
		FROM source_developer_applications WHERE id=? FOR UPDATE`, id), true)
	if errors.Is(err, sql.ErrNoRows) {
		return sourceDeveloper{}, errSourceNotFound
	}
	if err != nil {
		return sourceDeveloper{}, err
	}
	if app.Status != sourceApplicationPending {
		return sourceDeveloper{}, errApplicationReviewed
	}
	if _, err := tx.Exec(`UPDATE source_developer_applications SET status='approved', reviewed_by=?, reviewed_at=NOW() WHERE id=?`, reviewer, id); err != nil {
		return sourceDeveloper{}, err
	}
	result, err := tx.Exec(`INSERT INTO source_developers (application_id, username, password_hash, email, display_name, enabled)
		VALUES (?, ?, ?, ?, ?, 1)`, id, app.Username, app.PasswordHash, app.Email, app.DisplayName)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			return sourceDeveloper{}, errSourceConflict
		}
		return sourceDeveloper{}, err
	}
	developerID, _ := result.LastInsertId()
	if _, err := tx.Exec(`INSERT IGNORE INTO roles (role_name, role_code, description, discount, enabled)
		VALUES (?, ?, '软件源开发者，可提交插件与首页模板元数据', 10.0, 1)`, sourceDeveloperRoleName, sourceDeveloperRoleCode); err != nil {
		return sourceDeveloper{}, err
	}
	if err := tx.Commit(); err != nil {
		return sourceDeveloper{}, err
	}
	mysqlAppendAudit(db, "approve", "application", itoaSourceID(id), reviewer, "")
	return (mysqlSourceStore{}).GetDeveloperByID(developerID)
}

func (mysqlSourceStore) RejectApplication(id int64, reviewer, note string) error {
	app, err := (mysqlSourceStore{}).GetApplication(id)
	if err != nil {
		return err
	}
	if app.Status != sourceApplicationPending {
		return errApplicationReviewed
	}
	db, err := config.DB()
	if err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE source_developer_applications SET status='rejected', reviewed_by=?, review_note=?, reviewed_at=NOW() WHERE id=?`,
		reviewer, truncateText(note, 500), id); err != nil {
		return err
	}
	mysqlAppendAudit(db, "reject", "application", itoaSourceID(id), reviewer, note)
	return nil
}

func (mysqlSourceStore) FreezeApplication(id int64, reviewer, note string) error {
	app, err := (mysqlSourceStore{}).GetApplication(id)
	if err != nil {
		return err
	}
	if app.Status == sourceApplicationFrozen {
		return errDeveloperAlreadyCancelled
	}
	if app.Status != sourceApplicationApproved {
		return errApplicationNotCancellable
	}
	note = sourceCancelNote(note)
	db, err := config.DB()
	if err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE source_developer_applications SET status='frozen', reviewed_by=?, review_note=?, reviewed_at=NOW() WHERE id=?`,
		reviewer, truncateText(note, 500), id); err != nil {
		return err
	}
	_, _ = db.Exec(`UPDATE source_developers SET enabled=0 WHERE application_id=? OR username=?`, id, app.Username)
	mysqlAppendAudit(db, "cancel", "application", itoaSourceID(id), reviewer, note)
	return nil
}

func scanSourceDeveloper(scanner interface{ Scan(dest ...any) error }) (sourceDeveloper, error) {
	var item sourceDeveloper
	var enabled int
	if err := scanner.Scan(&item.ID, &item.ApplicationID, &item.Username, &item.PasswordHash, &item.Email, &item.DisplayName, &enabled, &item.CreatedAt); err != nil {
		return sourceDeveloper{}, err
	}
	item.Enabled = enabled == 1
	item.CreatedAt = item.CreatedAt.UTC()
	return item, nil
}

func (mysqlSourceStore) GetDeveloperByUsername(username string) (sourceDeveloper, error) {
	db, err := config.DB()
	if err != nil {
		return sourceDeveloper{}, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return sourceDeveloper{}, err
	}
	item, err := scanSourceDeveloper(db.QueryRow(`SELECT id, COALESCE(application_id, 0), username, password_hash, email, display_name, enabled, created_at
		FROM source_developers WHERE username=?`, username))
	if errors.Is(err, sql.ErrNoRows) {
		return sourceDeveloper{}, errSourceNotFound
	}
	return item, err
}

func (mysqlSourceStore) GetDeveloperByID(id int64) (sourceDeveloper, error) {
	db, err := config.DB()
	if err != nil {
		return sourceDeveloper{}, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return sourceDeveloper{}, err
	}
	item, err := scanSourceDeveloper(db.QueryRow(`SELECT id, COALESCE(application_id, 0), username, password_hash, email, display_name, enabled, created_at
		FROM source_developers WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return sourceDeveloper{}, errSourceNotFound
	}
	return item, err
}

func (mysqlSourceStore) FreezeDeveloper(id int64, actor, note string) error {
	item, err := (mysqlSourceStore{}).GetDeveloperByID(id)
	if err != nil {
		return err
	}
	if !item.Enabled {
		return errDeveloperAlreadyCancelled
	}
	note = sourceCancelNote(note)
	db, err := config.DB()
	if err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE source_developers SET enabled=0 WHERE id=?`, id); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE source_developer_applications SET status='frozen', reviewed_by=?, review_note=?, reviewed_at=NOW() WHERE id=? OR username=?`,
		actor, truncateText(note, 500), item.ApplicationID, item.Username); err != nil {
		return err
	}
	mysqlAppendAudit(db, "cancel", "developer", itoaSourceID(id), actor, note)
	return nil
}

func (mysqlSourceStore) ListDevelopers() ([]sourceDeveloper, error) {
	db, err := config.DB()
	if err != nil {
		return nil, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT id, COALESCE(application_id, 0), username, '' AS password_hash, email, display_name, enabled, created_at FROM source_developers ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]sourceDeveloper, 0)
	for rows.Next() {
		item, err := scanSourceDeveloper(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (mysqlSourceStore) ListAdvertisements(position string) ([]advertisementRecord, error) {
	db, err := config.DB()
	if err != nil {
		return nil, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return nil, err
	}
	query := `SELECT id, title, image_url, destination_url, position, weight, start_at, end_at, description FROM source_advertisements`
	args := []any{}
	if position != "" {
		query += ` WHERE position=?`
		args = append(args, position)
	}
	query += ` ORDER BY weight DESC, id ASC`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]advertisementRecord, 0)
	for rows.Next() {
		var item advertisementRecord
		if err := rows.Scan(&item.ID, &item.Title, &item.ImageURL, &item.DestinationURL, &item.Position, &item.Weight, &item.StartAt, &item.EndAt, &item.Description); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (mysqlSourceStore) UpsertAdvertisement(record advertisementRecord) error {
	db, err := config.DB()
	if err != nil {
		return err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO source_advertisements (id, title, image_url, destination_url, position, weight, start_at, end_at, description)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE title=VALUES(title), image_url=VALUES(image_url), destination_url=VALUES(destination_url),
			position=VALUES(position), weight=VALUES(weight), start_at=VALUES(start_at), end_at=VALUES(end_at), description=VALUES(description)`,
		record.ID, record.Title, record.ImageURL, record.DestinationURL, record.Position, record.Weight, record.StartAt, record.EndAt, record.Description)
	return err
}

func (mysqlSourceStore) DeleteAdvertisement(id string) error {
	db, err := config.DB()
	if err != nil {
		return err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return err
	}
	result, err := db.Exec(`DELETE FROM source_advertisements WHERE id=?`, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return errSourceNotFound
	}
	return nil
}

func (mysqlSourceStore) AppendAudit(entry sourceAuditEntry) error {
	db, err := config.DB()
	if err != nil {
		return err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return err
	}
	if entry.ActorType == "" {
		entry.ActorType = "admin"
	}
	_, err = db.Exec(`INSERT INTO source_audit_logs (actor_type, actor_name, action, target_type, target_id, detail)
		VALUES (?, ?, ?, ?, ?, ?)`, entry.ActorType, entry.ActorName, entry.Action, entry.TargetType, entry.TargetID, truncateText(entry.Detail, 500))
	return err
}

func (mysqlSourceStore) ListAudit(limit int) ([]sourceAuditEntry, error) {
	db, err := config.DB()
	if err != nil {
		return nil, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := db.Query(`SELECT id, actor_type, actor_name, action, target_type, target_id, detail, created_at
		FROM source_audit_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]sourceAuditEntry, 0)
	for rows.Next() {
		var item sourceAuditEntry
		if err := rows.Scan(&item.ID, &item.ActorType, &item.ActorName, &item.Action, &item.TargetType, &item.TargetID, &item.Detail, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.CreatedAt = item.CreatedAt.UTC()
		result = append(result, item)
	}
	return result, rows.Err()
}

func (mysqlSourceStore) SaveIndexSnapshot(payload, actor string) error {
	db, err := config.DB()
	if err != nil {
		return err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT INTO source_index_snapshots (payload, generated_by) VALUES (?, ?)`, payload, actor); err != nil {
		return err
	}
	mysqlAppendAudit(db, "regenerate", "index", "index.json", actor, "")
	return nil
}

func (mysqlSourceStore) LatestIndexSnapshot() (sourceIndexSnapshot, error) {
	db, err := config.DB()
	if err != nil {
		return sourceIndexSnapshot{}, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return sourceIndexSnapshot{}, err
	}
	var item sourceIndexSnapshot
	err = db.QueryRow(`SELECT payload, generated_by, generated_at FROM source_index_snapshots ORDER BY id DESC LIMIT 1`).
		Scan(&item.Payload, &item.GeneratedBy, &item.GeneratedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sourceIndexSnapshot{}, nil
	}
	if err != nil {
		return sourceIndexSnapshot{}, err
	}
	item.GeneratedAt = item.GeneratedAt.UTC()
	return item, nil
}

const sourceReleaseSettingsKey = "release"

func (mysqlSourceStore) GetReleaseSettings() (sourceReleaseSettings, error) {
	db, err := config.DB()
	if err != nil {
		return sourceReleaseSettings{}, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return sourceReleaseSettings{}, err
	}
	var raw string
	err = db.QueryRow(`SELECT setting_value FROM source_station_settings WHERE setting_key=?`, sourceReleaseSettingsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultSourceReleaseSettings(), nil
	}
	if err != nil {
		return sourceReleaseSettings{}, err
	}
	var settings sourceReleaseSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return defaultSourceReleaseSettings(), nil
	}
	return normalizeReleaseSettings(settings), nil
}

func (mysqlSourceStore) SaveReleaseSettings(settings sourceReleaseSettings) error {
	db, err := config.DB()
	if err != nil {
		return err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return err
	}
	settings = normalizeReleaseSettings(settings)
	payload, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO source_station_settings (setting_key, setting_value) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE setting_value=VALUES(setting_value)`, sourceReleaseSettingsKey, string(payload))
	return err
}

func sourceCatalogJSON() ([]byte, ginHCatalog, error) {
	plugins, err := currentSourceStationStore().ListPlugins(sourceItemPublished)
	if err != nil {
		plugins = nil
	}
	templates, err := currentSourceStationStore().ListTemplates(sourceItemPublished)
	if err != nil {
		templates = nil
	}
	pluginItems := make([]map[string]any, 0, len(plugins))
	for _, plugin := range plugins {
		entry := map[string]any{
			"id": plugin.ID, "category": plugin.Category, "name": plugin.Name, "description": plugin.Description,
			"icon": plugin.Icon, "version": plugin.Version, "author": plugin.Author, "downloadUrl": plugin.DownloadURL,
			"sha256": plugin.SHA256, "forceUpdate": plugin.ForceUpdate,
		}
		if plugin.Changelog != "" {
			entry["changelog"] = plugin.Changelog
		}
		if plugin.MinVersion != "" {
			entry["minVersion"] = plugin.MinVersion
		}
		pluginItems = append(pluginItems, entry)
	}
	homeTemplates := make([]map[string]any, 0, len(templates))
	for _, template := range templates {
		schemaVersion := template.SchemaVersion
		if schemaVersion == 0 {
			schemaVersion = homeTemplateSchemaVersion
		}
		entry := map[string]any{
			"id": template.TemplateKey, "name": template.Name, "description": template.Description,
			"version": template.Version, "schemaVersion": schemaVersion, "sha256": template.SHA256,
			"templateUrl": template.TemplateURL, "forceUpdate": template.ForceUpdate,
		}
		if template.Changelog != "" {
			entry["changelog"] = template.Changelog
		}
		if template.MinVersion != "" {
			entry["minVersion"] = template.MinVersion
		}
		homeTemplates = append(homeTemplates, entry)
	}
	catalog := ginHCatalog{Name: sourceStationSourceName, Plugins: pluginItems, HomeTemplates: homeTemplates}
	payload, err := json.Marshal(catalog)
	return payload, catalog, err
}

type ginHCatalog struct {
	Name          string           `json:"name"`
	Plugins       []map[string]any `json:"plugins"`
	HomeTemplates []map[string]any `json:"homeTemplates"`
}
