package handler

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"auto_pro/config"
)

const (
	sourceStationSourceID   = "local"
	sourceStationSourceName = "本站软件源"
	sourceStationSourceType = "json"
	sourceDeveloperRoleCode = "R_DEVELOPER"
	sourceDeveloperRoleName = "开发者"

	sourceApplicationPending  = "pending"
	sourceApplicationApproved = "approved"
	sourceApplicationRejected = "rejected"
)

var (
	errSourceNotFound      = errors.New("目录项不存在")
	errSourceConflict      = errors.New("标识已被占用")
	errSourceForbidden     = errors.New("只能修改自己发布的目录项")
	errApplicationPending  = errors.New("入驻申请正在审核中")
	errApplicationReviewed = errors.New("入驻申请已处理")
	errDeveloperDisabled   = errors.New("开发者账号已禁用")

	sourceStationOverride   sourceStationStore
	sourceStationOverrideMu sync.RWMutex
)

type sourceAuthor struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Email string `json:"email"`
}

type sourcePlugin struct {
	ID          string
	DeveloperID int64
	Category    string
	Name        string
	Description string
	Icon        string
	Version     string
	Author      sourceAuthor
	SHA256      string
	Published   bool
	UpdatedAt   time.Time
	CreatedAt   time.Time
}

type sourceTemplate struct {
	ID                 string
	DeveloperID        int64
	TemplateKey        string
	Name               string
	Description        string
	Version            string
	Format             string
	SchemaVersion      int
	SHA256             string
	Author             sourceAuthor
	PreviewContentType string
	HasPreview         bool
	Published          bool
	UpdatedAt          time.Time
	CreatedAt          time.Time
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

type sourceStationStore interface {
	Ensure() error
	CatalogKey() (string, error)
	SetCatalogKey(key string) error
	Revision() (int64, error)

	ListPlugins() ([]sourcePlugin, error)
	GetPlugin(id string) (sourcePlugin, []byte, error)
	PutPlugin(plugin sourcePlugin, payload []byte) error

	ListTemplates() ([]sourceTemplate, error)
	GetTemplate(id string) (sourceTemplate, []byte, []byte, error)
	PutTemplate(item sourceTemplate, content, preview []byte) error

	CreateApplication(app sourceApplication) (sourceApplication, error)
	ListApplications(status string) ([]sourceApplication, error)
	GetApplication(id int64) (sourceApplication, error)
	ApproveApplication(id int64, reviewer string) (sourceDeveloper, error)
	RejectApplication(id int64, reviewer, note string) error
	GetDeveloperByUsername(username string) (sourceDeveloper, error)
	GetDeveloperByID(id int64) (sourceDeveloper, error)

	ListAdvertisements(position string) ([]advertisementRecord, error)
	UpsertAdvertisement(record advertisementRecord) error
	DeleteAdvertisement(id string) error
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

type memorySourceStore struct {
	mu            sync.Mutex
	revision      atomic.Int64
	catalogKey    string
	plugins       map[string]sourcePlugin
	pluginBytes   map[string][]byte
	templates     map[string]sourceTemplate
	templateBytes map[string][]byte
	previewBytes  map[string][]byte
	applications  map[int64]sourceApplication
	developers    map[int64]sourceDeveloper
	ads           map[string]advertisementRecord
	nextAppID     int64
	nextDevID     int64
}

func newMemorySourceStore() *memorySourceStore {
	store := &memorySourceStore{
		plugins:       map[string]sourcePlugin{},
		pluginBytes:   map[string][]byte{},
		templates:     map[string]sourceTemplate{},
		templateBytes: map[string][]byte{},
		previewBytes:  map[string][]byte{},
		applications:  map[int64]sourceApplication{},
		developers:    map[int64]sourceDeveloper{},
		ads:           map[string]advertisementRecord{},
		nextAppID:     1,
		nextDevID:     1,
	}
	store.revision.Store(1)
	return store
}

func (store *memorySourceStore) Ensure() error { return nil }

func (store *memorySourceStore) CatalogKey() (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.catalogKey, nil
}

func (store *memorySourceStore) SetCatalogKey(key string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.catalogKey = strings.TrimSpace(key)
	return nil
}

func (store *memorySourceStore) Revision() (int64, error) {
	value := store.revision.Load()
	if value < 1 {
		return 1, nil
	}
	return value, nil
}

func (store *memorySourceStore) bump() {
	if store.revision.Load() < 1 {
		store.revision.Store(1)
	}
	store.revision.Add(1)
}

func (store *memorySourceStore) ListPlugins() ([]sourcePlugin, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]sourcePlugin, 0, len(store.plugins))
	for _, item := range store.plugins {
		if item.Published {
			result = append(result, item)
		}
	}
	return result, nil
}

func (store *memorySourceStore) GetPlugin(id string) (sourcePlugin, []byte, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	item, ok := store.plugins[id]
	if !ok || !item.Published {
		return sourcePlugin{}, nil, errSourceNotFound
	}
	return item, append([]byte(nil), store.pluginBytes[id]...), nil
}

func (store *memorySourceStore) PutPlugin(plugin sourcePlugin, payload []byte) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if existing, ok := store.plugins[plugin.ID]; ok && existing.DeveloperID != plugin.DeveloperID {
		return errSourceForbidden
	}
	plugin.SHA256 = sourceContentSHA256(payload)
	plugin.Published = true
	now := time.Now().UTC()
	if plugin.CreatedAt.IsZero() {
		if existing, ok := store.plugins[plugin.ID]; ok {
			plugin.CreatedAt = existing.CreatedAt
		} else {
			plugin.CreatedAt = now
		}
	}
	plugin.UpdatedAt = now
	store.plugins[plugin.ID] = plugin
	store.pluginBytes[plugin.ID] = append([]byte(nil), payload...)
	store.bump()
	return nil
}

func (store *memorySourceStore) ListTemplates() ([]sourceTemplate, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]sourceTemplate, 0, len(store.templates))
	for _, item := range store.templates {
		if item.Published {
			result = append(result, item)
		}
	}
	return result, nil
}

func (store *memorySourceStore) GetTemplate(id string) (sourceTemplate, []byte, []byte, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	item, ok := store.templates[id]
	if !ok || !item.Published {
		return sourceTemplate{}, nil, nil, errSourceNotFound
	}
	return item, append([]byte(nil), store.templateBytes[id]...), append([]byte(nil), store.previewBytes[id]...), nil
}

func (store *memorySourceStore) PutTemplate(template sourceTemplate, content, preview []byte) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if existing, ok := store.templates[template.ID]; ok && existing.DeveloperID != template.DeveloperID {
		return errSourceForbidden
	}
	for id, existing := range store.templates {
		if existing.TemplateKey == template.TemplateKey && id != template.ID && existing.DeveloperID != template.DeveloperID {
			return errSourceConflict
		}
	}
	template.SHA256 = sourceContentSHA256(content)
	template.Published = true
	template.HasPreview = len(preview) > 0
	now := time.Now().UTC()
	if template.CreatedAt.IsZero() {
		if existing, ok := store.templates[template.ID]; ok {
			template.CreatedAt = existing.CreatedAt
		} else {
			template.CreatedAt = now
		}
	}
	template.UpdatedAt = now
	store.templates[template.ID] = template
	store.templateBytes[template.ID] = append([]byte(nil), content...)
	store.previewBytes[template.ID] = append([]byte(nil), preview...)
	store.bump()
	return nil
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
		ID:            store.nextDevID,
		ApplicationID: app.ID,
		Username:      app.Username,
		PasswordHash:  app.PasswordHash,
		Email:         app.Email,
		DisplayName:   app.DisplayName,
		Enabled:       true,
		CreatedAt:     now,
	}
	store.nextDevID++
	store.developers[dev.ID] = dev
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
			developer_id BIGINT UNSIGNED NOT NULL,
			category VARCHAR(30) NOT NULL DEFAULT 'other',
			name VARCHAR(100) NOT NULL,
			description VARCHAR(500) NOT NULL DEFAULT '',
			icon VARCHAR(80) NOT NULL DEFAULT 'ri:puzzle-line',
			version VARCHAR(40) NOT NULL,
			author_name VARCHAR(100) NOT NULL DEFAULT '',
			author_url VARCHAR(300) NOT NULL DEFAULT '',
			author_email VARCHAR(200) NOT NULL DEFAULT '',
			sha256 CHAR(64) NOT NULL,
			file_path VARCHAR(500) NOT NULL DEFAULT '',
			published TINYINT(1) NOT NULL DEFAULT 1,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			KEY idx_source_catalog_plugin_developer (developer_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='软件源已发布插件'`,
		`CREATE TABLE IF NOT EXISTS source_catalog_templates (
			id VARCHAR(60) NOT NULL PRIMARY KEY,
			developer_id BIGINT UNSIGNED NOT NULL,
			template_key VARCHAR(60) NOT NULL,
			name VARCHAR(100) NOT NULL,
			description VARCHAR(500) NOT NULL DEFAULT '',
			version VARCHAR(40) NOT NULL,
			format VARCHAR(10) NOT NULL DEFAULT 'json',
			schema_version INT NOT NULL DEFAULT 1,
			sha256 CHAR(64) NOT NULL,
			author_name VARCHAR(100) NOT NULL DEFAULT '',
			author_url VARCHAR(300) NOT NULL DEFAULT '',
			author_email VARCHAR(200) NOT NULL DEFAULT '',
			file_path VARCHAR(500) NOT NULL DEFAULT '',
			preview_path VARCHAR(500) NOT NULL DEFAULT '',
			preview_content_type VARCHAR(80) NOT NULL DEFAULT '',
			published TINYINT(1) NOT NULL DEFAULT 1,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_source_catalog_template_key (template_key),
			KEY idx_source_catalog_template_developer (developer_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='软件源已发布首页模板'`,
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
		`CREATE TABLE IF NOT EXISTS source_station_settings (
			setting_key VARCHAR(50) NOT NULL PRIMARY KEY,
			setting_value VARCHAR(255) NOT NULL DEFAULT ''
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='软件源源站设置'`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	_, _ = db.Exec(`INSERT IGNORE INTO roles (role_name, role_code, description, discount, enabled)
		VALUES (?, ?, '软件源开发者，可向本站目录发布插件与首页模板', 10.0, 1)`,
		sourceDeveloperRoleName, sourceDeveloperRoleCode)
	return nil
}

func sourceCatalogDir(kind string) string {
	dir := filepath.Join(config.GetDataDir(), "source-catalog", kind)
	_ = os.MkdirAll(dir, 0750)
	return dir
}

func sourceCatalogPath(kind, id string) string {
	return filepath.Join(sourceCatalogDir(kind), id)
}

func writeSourceCatalogFile(kind, id string, payload []byte) (string, error) {
	path := sourceCatalogPath(kind, id)
	if err := os.WriteFile(path, payload, 0640); err != nil {
		return "", err
	}
	return path, nil
}

func readSourceCatalogFile(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errSourceNotFound
	}
	return payload, err
}

func (mysqlSourceStore) CatalogKey() (string, error) {
	db, err := config.DB()
	if err != nil {
		return "", err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return "", err
	}
	var value string
	err = db.QueryRow(`SELECT setting_value FROM source_station_settings WHERE setting_key='catalog_key'`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return strings.TrimSpace(value), err
}

func (mysqlSourceStore) SetCatalogKey(key string) error {
	db, err := config.DB()
	if err != nil {
		return err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO source_station_settings (setting_key, setting_value) VALUES ('catalog_key', ?)
		ON DUPLICATE KEY UPDATE setting_value=VALUES(setting_value)`, strings.TrimSpace(key))
	return err
}

func (mysqlSourceStore) Revision() (int64, error) {
	db, err := config.DB()
	if err != nil {
		return 1, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return 1, err
	}
	var pluginUnix, templateUnix sql.NullInt64
	_ = db.QueryRow(`SELECT UNIX_TIMESTAMP(MAX(updated_at)) FROM source_catalog_plugins WHERE published=1`).Scan(&pluginUnix)
	_ = db.QueryRow(`SELECT UNIX_TIMESTAMP(MAX(updated_at)) FROM source_catalog_templates WHERE published=1`).Scan(&templateUnix)
	revision := int64(1)
	if pluginUnix.Valid && pluginUnix.Int64 > revision {
		revision = pluginUnix.Int64
	}
	if templateUnix.Valid && templateUnix.Int64 > revision {
		revision = templateUnix.Int64
	}
	return revision, nil
}

func scanSourcePlugin(scanner interface {
	Scan(dest ...any) error
}) (sourcePlugin, string, error) {
	var item sourcePlugin
	var filePath string
	var published int
	var updatedAt, createdAt time.Time
	err := scanner.Scan(&item.ID, &item.DeveloperID, &item.Category, &item.Name, &item.Description, &item.Icon,
		&item.Version, &item.Author.Name, &item.Author.URL, &item.Author.Email, &item.SHA256, &filePath,
		&published, &updatedAt, &createdAt)
	if err != nil {
		return sourcePlugin{}, "", err
	}
	item.Published = published == 1
	item.UpdatedAt = updatedAt.UTC()
	item.CreatedAt = createdAt.UTC()
	return item, filePath, nil
}

func (mysqlSourceStore) ListPlugins() ([]sourcePlugin, error) {
	db, err := config.DB()
	if err != nil {
		return nil, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT id, developer_id, category, name, description, icon, version, author_name, author_url,
		author_email, sha256, file_path, published, updated_at, created_at
		FROM source_catalog_plugins WHERE published=1 ORDER BY updated_at DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]sourcePlugin, 0)
	for rows.Next() {
		item, _, err := scanSourcePlugin(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (mysqlSourceStore) GetPlugin(id string) (sourcePlugin, []byte, error) {
	db, err := config.DB()
	if err != nil {
		return sourcePlugin{}, nil, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return sourcePlugin{}, nil, err
	}
	row := db.QueryRow(`SELECT id, developer_id, category, name, description, icon, version, author_name, author_url,
		author_email, sha256, file_path, published, updated_at, created_at
		FROM source_catalog_plugins WHERE id=? AND published=1`, id)
	item, filePath, err := scanSourcePlugin(row)
	if errors.Is(err, sql.ErrNoRows) {
		return sourcePlugin{}, nil, errSourceNotFound
	}
	if err != nil {
		return sourcePlugin{}, nil, err
	}
	payload, err := readSourceCatalogFile(filePath)
	if err != nil {
		return sourcePlugin{}, nil, err
	}
	return item, payload, nil
}

func (mysqlSourceStore) PutPlugin(plugin sourcePlugin, payload []byte) error {
	db, err := config.DB()
	if err != nil {
		return err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return err
	}
	var existingDeveloper sql.NullInt64
	err = db.QueryRow(`SELECT developer_id FROM source_catalog_plugins WHERE id=?`, plugin.ID).Scan(&existingDeveloper)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if existingDeveloper.Valid && existingDeveloper.Int64 != plugin.DeveloperID {
		return errSourceForbidden
	}
	plugin.SHA256 = sourceContentSHA256(payload)
	filePath, err := writeSourceCatalogFile("plugins", plugin.ID, payload)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO source_catalog_plugins
		(id, developer_id, category, name, description, icon, version, author_name, author_url, author_email, sha256, file_path, published)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
		ON DUPLICATE KEY UPDATE category=VALUES(category), name=VALUES(name), description=VALUES(description),
			icon=VALUES(icon), version=VALUES(version), author_name=VALUES(author_name), author_url=VALUES(author_url),
			author_email=VALUES(author_email), sha256=VALUES(sha256), file_path=VALUES(file_path), published=1`,
		plugin.ID, plugin.DeveloperID, plugin.Category, plugin.Name, plugin.Description, plugin.Icon, plugin.Version,
		plugin.Author.Name, plugin.Author.URL, plugin.Author.Email, plugin.SHA256, filePath)
	return err
}

func scanSourceTemplate(scanner interface {
	Scan(dest ...any) error
}) (sourceTemplate, string, string, error) {
	var item sourceTemplate
	var filePath, previewPath string
	var published int
	var updatedAt, createdAt time.Time
	err := scanner.Scan(&item.ID, &item.DeveloperID, &item.TemplateKey, &item.Name, &item.Description, &item.Version,
		&item.Format, &item.SchemaVersion, &item.SHA256, &item.Author.Name, &item.Author.URL, &item.Author.Email,
		&filePath, &previewPath, &item.PreviewContentType, &published, &updatedAt, &createdAt)
	if err != nil {
		return sourceTemplate{}, "", "", err
	}
	item.Published = published == 1
	item.HasPreview = strings.TrimSpace(previewPath) != ""
	item.UpdatedAt = updatedAt.UTC()
	item.CreatedAt = createdAt.UTC()
	return item, filePath, previewPath, nil
}

func (mysqlSourceStore) ListTemplates() ([]sourceTemplate, error) {
	db, err := config.DB()
	if err != nil {
		return nil, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT id, developer_id, template_key, name, description, version, format, schema_version,
		sha256, author_name, author_url, author_email, file_path, preview_path, preview_content_type, published, updated_at, created_at
		FROM source_catalog_templates WHERE published=1 ORDER BY updated_at DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]sourceTemplate, 0)
	for rows.Next() {
		item, _, _, err := scanSourceTemplate(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (mysqlSourceStore) GetTemplate(id string) (sourceTemplate, []byte, []byte, error) {
	db, err := config.DB()
	if err != nil {
		return sourceTemplate{}, nil, nil, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return sourceTemplate{}, nil, nil, err
	}
	row := db.QueryRow(`SELECT id, developer_id, template_key, name, description, version, format, schema_version,
		sha256, author_name, author_url, author_email, file_path, preview_path, preview_content_type, published, updated_at, created_at
		FROM source_catalog_templates WHERE id=? AND published=1`, id)
	item, filePath, previewPath, err := scanSourceTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return sourceTemplate{}, nil, nil, errSourceNotFound
	}
	if err != nil {
		return sourceTemplate{}, nil, nil, err
	}
	content, err := readSourceCatalogFile(filePath)
	if err != nil {
		return sourceTemplate{}, nil, nil, err
	}
	var preview []byte
	if item.HasPreview {
		preview, err = readSourceCatalogFile(previewPath)
		if err != nil && !errors.Is(err, errSourceNotFound) {
			return sourceTemplate{}, nil, nil, err
		}
	}
	return item, content, preview, nil
}

func (mysqlSourceStore) PutTemplate(template sourceTemplate, content, preview []byte) error {
	db, err := config.DB()
	if err != nil {
		return err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return err
	}
	var existingID string
	var existingDeveloper sql.NullInt64
	err = db.QueryRow(`SELECT id, developer_id FROM source_catalog_templates WHERE template_key=?`, template.TemplateKey).
		Scan(&existingID, &existingDeveloper)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if existingDeveloper.Valid && existingDeveloper.Int64 != template.DeveloperID {
		return errSourceConflict
	}
	if existingID != "" && existingID != template.ID && existingDeveloper.Valid && existingDeveloper.Int64 != template.DeveloperID {
		return errSourceConflict
	}
	var owner sql.NullInt64
	err = db.QueryRow(`SELECT developer_id FROM source_catalog_templates WHERE id=?`, template.ID).Scan(&owner)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if owner.Valid && owner.Int64 != template.DeveloperID {
		return errSourceForbidden
	}
	template.SHA256 = sourceContentSHA256(content)
	filePath, err := writeSourceCatalogFile("templates", template.ID, content)
	if err != nil {
		return err
	}
	previewPath := ""
	if len(preview) > 0 {
		previewPath, err = writeSourceCatalogFile("previews", template.ID, preview)
		if err != nil {
			return err
		}
	}
	_, err = db.Exec(`INSERT INTO source_catalog_templates
		(id, developer_id, template_key, name, description, version, format, schema_version, sha256, author_name,
		 author_url, author_email, file_path, preview_path, preview_content_type, published)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
		ON DUPLICATE KEY UPDATE name=VALUES(name), description=VALUES(description), version=VALUES(version),
			format=VALUES(format), schema_version=VALUES(schema_version), sha256=VALUES(sha256),
			author_name=VALUES(author_name), author_url=VALUES(author_url), author_email=VALUES(author_email),
			file_path=VALUES(file_path), preview_path=VALUES(preview_path),
			preview_content_type=VALUES(preview_content_type), published=1`,
		template.ID, template.DeveloperID, template.TemplateKey, template.Name, template.Description, template.Version,
		template.Format, template.SchemaVersion, template.SHA256, template.Author.Name, template.Author.URL,
		template.Author.Email, filePath, previewPath, template.PreviewContentType)
	return err
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

func scanSourceApplication(scanner interface {
	Scan(dest ...any) error
}, withPassword bool) (sourceApplication, error) {
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
	row := db.QueryRow(`SELECT id, username, password_hash, email, display_name, reason, status, review_note, reviewed_by, reviewed_at, created_at
		FROM source_developer_applications WHERE id=?`, id)
	item, err := scanSourceApplication(row, true)
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
	row := tx.QueryRow(`SELECT id, username, password_hash, email, display_name, reason, status, review_note, reviewed_by, reviewed_at, created_at
		FROM source_developer_applications WHERE id=? FOR UPDATE`, id)
	app, err := scanSourceApplication(row, true)
	if errors.Is(err, sql.ErrNoRows) {
		return sourceDeveloper{}, errSourceNotFound
	}
	if err != nil {
		return sourceDeveloper{}, err
	}
	if app.Status != sourceApplicationPending {
		return sourceDeveloper{}, errApplicationReviewed
	}
	if _, err := tx.Exec(`UPDATE source_developer_applications SET status='approved', reviewed_by=?, reviewed_at=NOW() WHERE id=?`,
		reviewer, id); err != nil {
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
		VALUES (?, ?, '软件源开发者，可向本站目录发布插件与首页模板', 10.0, 1)`,
		sourceDeveloperRoleName, sourceDeveloperRoleCode); err != nil {
		return sourceDeveloper{}, err
	}
	if err := tx.Commit(); err != nil {
		return sourceDeveloper{}, err
	}
	return (mysqlSourceStore{}).GetDeveloperByID(developerID)
}

func (mysqlSourceStore) RejectApplication(id int64, reviewer, note string) error {
	db, err := config.DB()
	if err != nil {
		return err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return err
	}
	app, err := (mysqlSourceStore{}).GetApplication(id)
	if err != nil {
		return err
	}
	if app.Status != sourceApplicationPending {
		return errApplicationReviewed
	}
	_, err = db.Exec(`UPDATE source_developer_applications SET status='rejected', reviewed_by=?, review_note=?, reviewed_at=NOW() WHERE id=?`,
		reviewer, truncateText(note, 500), id)
	return err
}

func scanSourceDeveloper(scanner interface {
	Scan(dest ...any) error
}) (sourceDeveloper, error) {
	var item sourceDeveloper
	var enabled int
	if err := scanner.Scan(&item.ID, &item.ApplicationID, &item.Username, &item.PasswordHash, &item.Email,
		&item.DisplayName, &enabled, &item.CreatedAt); err != nil {
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
	row := db.QueryRow(`SELECT id, COALESCE(application_id, 0), username, password_hash, email, display_name, enabled, created_at
		FROM source_developers WHERE username=?`, username)
	item, err := scanSourceDeveloper(row)
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
	row := db.QueryRow(`SELECT id, COALESCE(application_id, 0), username, password_hash, email, display_name, enabled, created_at
		FROM source_developers WHERE id=?`, id)
	item, err := scanSourceDeveloper(row)
	if errors.Is(err, sql.ErrNoRows) {
		return sourceDeveloper{}, errSourceNotFound
	}
	return item, err
}

func (mysqlSourceStore) ListAdvertisements(position string) ([]advertisementRecord, error) {
	db, err := config.DB()
	if err != nil {
		return nil, err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return nil, err
	}
	query := `SELECT id, title, image_url, destination_url, position, weight, start_at, end_at, description
		FROM source_advertisements`
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
		if err := rows.Scan(&item.ID, &item.Title, &item.ImageURL, &item.DestinationURL, &item.Position,
			&item.Weight, &item.StartAt, &item.EndAt, &item.Description); err != nil {
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
	_, err = db.Exec(`INSERT INTO source_advertisements
		(id, title, image_url, destination_url, position, weight, start_at, end_at, description)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE title=VALUES(title), image_url=VALUES(image_url), destination_url=VALUES(destination_url),
			position=VALUES(position), weight=VALUES(weight), start_at=VALUES(start_at), end_at=VALUES(end_at),
			description=VALUES(description)`,
		record.ID, record.Title, record.ImageURL, record.DestinationURL, record.Position, record.Weight,
		record.StartAt, record.EndAt, record.Description)
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
