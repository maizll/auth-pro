package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"auto_pro/config"
)

// 存储位置保存在一张单独的状态表里，内存库则放在 memorySourceStore。
// 令牌只以密文出现在这份状态里，接口响应走 storageLocationView，不带密文。

const (
	storageRolePrimary   = "primary"
	storageRoleBackup    = "backup"
	storageRoleDisabled  = "disabled"
	storageLegacyPaid    = "github_paid"
	storageLegacyRelease = "release"
)

// storageLocation 是站长配置的一个存储位置。SecretSealed 只在服务端状态里。
type storageLocation struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	Role         string    `json:"role"`
	Legacy       string    `json:"legacy,omitempty"`
	Owner        string    `json:"owner,omitempty"`
	Repo         string    `json:"repo,omitempty"`
	Branch       string    `json:"branch,omitempty"`
	Endpoint     string    `json:"endpoint,omitempty"`
	Region       string    `json:"region,omitempty"`
	Bucket       string    `json:"bucket,omitempty"`
	KeyPrefix    string    `json:"prefix,omitempty"`
	PathStyle    bool      `json:"pathStyle,omitempty"`
	AccessKey    string    `json:"accessKey,omitempty"`
	SecretSealed string    `json:"secretSealed,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// storageCopy 记录某个版本的安装包落在哪个存储、校验码和上传时间。
type storageCopy struct {
	Kind       string    `json:"kind"`
	ItemID     string    `json:"itemId"`
	Version    string    `json:"version"`
	LocationID string    `json:"locationId"`
	Ref        string    `json:"ref"`
	SHA256     string    `json:"sha256"`
	Size       int64     `json:"size"`
	UploadedAt time.Time `json:"uploadedAt"`
}

// storageHealthRow 是一轮检查里的一条结果。出问题只提醒，不改目录状态。
type storageHealthRow struct {
	Level        string `json:"level"`
	LocationID   string `json:"locationId"`
	LocationName string `json:"locationName"`
	Target       string `json:"target"`
	Message      string `json:"message"`
}

// storageConfigBlob 是存储管理的全部服务端状态。
type storageConfigBlob struct {
	Migrated  bool               `json:"migrated"`
	DualWrite bool               `json:"dualWrite"`
	Locations []storageLocation  `json:"locations"`
	Copies    []storageCopy      `json:"copies"`
	Health    []storageHealthRow `json:"health"`
	CheckedAt time.Time          `json:"checkedAt"`
}

var storageOpMu sync.Mutex

func sealStorageSecret(plain string) (string, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", errors.New("请填写密钥")
	}
	key, err := loadOrCreateStoreFileKey("storage-locations.key")
	if err != nil {
		return "", errors.New("保存密钥失败")
	}
	blob, err := sealStoreSecret(key, []byte(plain))
	if err != nil {
		return "", errors.New("保存密钥失败")
	}
	return base64.StdEncoding.EncodeToString(blob), nil
}

func openStorageSecret(sealed string) (string, error) {
	sealed = strings.TrimSpace(sealed)
	if sealed == "" {
		return "", errors.New("尚未保存密钥")
	}
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return "", errors.New("密钥无法读取")
	}
	key, err := loadOrCreateStoreFileKey("storage-locations.key")
	if err != nil {
		return "", errors.New("密钥无法读取")
	}
	plain, err := openStoreSecret(key, raw)
	if err != nil {
		return "", errors.New("密钥无法读取")
	}
	token := strings.TrimSpace(string(plain))
	if token == "" {
		return "", errors.New("尚未保存密钥")
	}
	return token, nil
}

func newStorageID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405.000")))
	}
	return hex.EncodeToString(buf)
}

func loadStorageBlob() (storageConfigBlob, error) {
	storageOpMu.Lock()
	defer storageOpMu.Unlock()
	blob, err := loadStorageBlobLocked()
	if err != nil {
		// 安装前或单测没有 db.json 时，当作还没有存储位置，让收费包继续暂存本站。
		// 数据库已配置但查询失败时仍返回错误，避免把已托管的包悄悄改存本站。
		if errors.Is(err, os.ErrNotExist) {
			return storageConfigBlob{Migrated: true}, nil
		}
		return storageConfigBlob{}, err
	}
	if blob.Migrated {
		return blob, nil
	}
	// 第一次读取时把旧的收费仓库和发布仓库收成列表，避免站长重填令牌。
	if err := migrateLegacyStorageLocked(&blob); err != nil {
		return storageConfigBlob{}, err
	}
	blob.Migrated = true
	if err := saveStorageBlobLocked(blob); err != nil {
		return storageConfigBlob{}, err
	}
	return blob, nil
}

func saveStorageBlob(blob storageConfigBlob) error {
	storageOpMu.Lock()
	defer storageOpMu.Unlock()
	blob.Migrated = true
	return saveStorageBlobLocked(blob)
}

func loadStorageBlobLocked() (storageConfigBlob, error) {
	switch store := currentSourceStationStore().(type) {
	case *memorySourceStore:
		store.mu.Lock()
		defer store.mu.Unlock()
		if store.storageBlob == nil {
			return storageConfigBlob{}, nil
		}
		return *store.storageBlob, nil
	case mysqlSourceStore:
		db, err := config.DB()
		if err != nil {
			return storageConfigBlob{}, err
		}
		if err := ensurePackageStorageState(db); err != nil {
			return storageConfigBlob{}, err
		}
		var raw string
		err = db.QueryRow(`SELECT payload FROM package_storage_state WHERE id=1`).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) || strings.TrimSpace(raw) == "" {
			return storageConfigBlob{}, nil
		}
		if err != nil {
			return storageConfigBlob{}, err
		}
		var blob storageConfigBlob
		if err := json.Unmarshal([]byte(raw), &blob); err != nil {
			return storageConfigBlob{}, errors.New("存储配置无法读取")
		}
		return blob, nil
	default:
		return storageConfigBlob{}, errors.New("读取存储配置失败")
	}
}

func saveStorageBlobLocked(blob storageConfigBlob) error {
	switch store := currentSourceStationStore().(type) {
	case *memorySourceStore:
		store.mu.Lock()
		defer store.mu.Unlock()
		copied := blob
		store.storageBlob = &copied
		return nil
	case mysqlSourceStore:
		db, err := config.DB()
		if err != nil {
			return err
		}
		if err := ensurePackageStorageState(db); err != nil {
			return err
		}
		payload, err := json.Marshal(blob)
		if err != nil {
			return err
		}
		_, err = db.Exec(`INSERT INTO package_storage_state (id, payload) VALUES (1, ?)
			ON DUPLICATE KEY UPDATE payload=VALUES(payload)`, string(payload))
		return err
	default:
		return errors.New("保存存储配置失败")
	}
}

func ensurePackageStorageState(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS package_storage_state (
		id TINYINT NOT NULL PRIMARY KEY,
		payload MEDIUMTEXT NOT NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='存储管理：位置、副本、最近一次检查'`)
	return err
}

func (loc storageLocation) hasSecret() bool {
	return strings.TrimSpace(loc.SecretSealed) != ""
}

func normalizeStorageRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case storageRolePrimary:
		return storageRolePrimary
	case storageRoleDisabled, "off", "stop":
		return storageRoleDisabled
	default:
		return storageRoleBackup
	}
}

func normalizeStorageKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case packageStorageGitHub:
		return packageStorageGitHub
	case packageStorageGitee:
		return packageStorageGitee
	case packageStorageS3, "oss", "cos", "r2", "minio":
		return packageStorageS3
	case packageStorageWebDAV, "nextcloud", "owncloud", "seafile", "alist", "cloudreve":
		return packageStorageWebDAV
	default:
		return ""
	}
}

// enabledStorageLocations 按主存储、再备用的顺序返回未停用且已有密钥的位置。
func enabledStorageLocations(list []storageLocation) []storageLocation {
	primary := make([]storageLocation, 0, 1)
	backup := make([]storageLocation, 0)
	for _, loc := range list {
		if loc.Role == storageRoleDisabled || !loc.hasSecret() {
			continue
		}
		if loc.Role == storageRolePrimary {
			primary = append(primary, loc)
			continue
		}
		backup = append(backup, loc)
	}
	return append(primary, backup...)
}

func demoteOtherPrimaries(list []storageLocation, keepID string) []storageLocation {
	for i := range list {
		if list[i].ID != keepID && list[i].Role == storageRolePrimary {
			list[i].Role = storageRoleBackup
		}
	}
	return list
}
