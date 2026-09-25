package handler

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"auto_pro/config"
)

// 发行包内置的快照验签公钥。对应私钥只放在源站
// AUTH_PRO_STORE_SNAPSHOT_PRIVATE_KEY 或数据目录 store/snapshot-ed25519.key，不进仓库。
const embeddedStoreSnapshotPublicKey = "SZiAY+pFSw/7WprsEn0sEoD8+UCpq94u49BmSSuYJDA="

var (
	storeSnapshotKeyMu   sync.RWMutex
	storeSnapshotPublic  ed25519.PublicKey
	storeSnapshotPrivate ed25519.PrivateKey
)

func init() {
	raw, err := base64.StdEncoding.DecodeString(embeddedStoreSnapshotPublicKey)
	if err == nil && len(raw) == ed25519.PublicKeySize {
		storeSnapshotPublic = ed25519.PublicKey(raw)
	}
}

func useStoreSnapshotKeysForTest(pub ed25519.PublicKey, priv ed25519.PrivateKey) func() {
	storeSnapshotKeyMu.Lock()
	prevPub, prevPriv := storeSnapshotPublic, storeSnapshotPrivate
	storeSnapshotPublic = pub
	storeSnapshotPrivate = priv
	storeSnapshotKeyMu.Unlock()
	return func() {
		storeSnapshotKeyMu.Lock()
		storeSnapshotPublic = prevPub
		storeSnapshotPrivate = prevPriv
		storeSnapshotKeyMu.Unlock()
	}
}

func loadStoreSnapshotPrivateKey() (ed25519.PrivateKey, error) {
	storeSnapshotKeyMu.RLock()
	if storeSnapshotPrivate != nil {
		key := storeSnapshotPrivate
		storeSnapshotKeyMu.RUnlock()
		return key, nil
	}
	storeSnapshotKeyMu.RUnlock()

	encoded := strings.TrimSpace(os.Getenv("AUTH_PRO_STORE_SNAPSHOT_PRIVATE_KEY"))
	if encoded == "" {
		path := filepath.Join(config.GetDataDir(), "store", "snapshot-ed25519.key")
		payload, err := os.ReadFile(path)
		if err != nil {
			return nil, errors.New("未配置商店签名私钥")
		}
		encoded = strings.TrimSpace(string(payload))
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return nil, errors.New("商店签名私钥格式不正确")
	}
	key := ed25519.PrivateKey(raw)
	storeSnapshotKeyMu.RLock()
	pub := storeSnapshotPublic
	storeSnapshotKeyMu.RUnlock()
	if len(pub) == ed25519.PublicKeySize && !bytes.Equal(key.Public().(ed25519.PublicKey), pub) {
		return nil, errors.New("商店签名私钥与发行包公钥不匹配")
	}
	return key, nil
}

type storeSnapshotItem struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Source   string `json:"source"`
	Period   string `json:"period"`
	ExpireAt *int64 `json:"expireAt"`
}

type storeSnapshot struct {
	BindingID       string              `json:"bindingId"`
	LicenseNo       string              `json:"licenseNo"`
	Domain          string              `json:"domain"`
	LicenseStatus   string              `json:"licenseStatus"`
	LicenseExpireAt *int64              `json:"licenseExpireAt"`
	Edition         string              `json:"edition"`
	EditionPeriod   string              `json:"editionPeriod"`
	EditionExpireAt *int64              `json:"editionExpireAt"`
	Features        []string            `json:"features"`
	Items           []storeSnapshotItem `json:"items"`
	AllPaidItems    bool                `json:"allPaidItems"`
	ServerTime      int64               `json:"serverTime"`
	GraceDays       int                 `json:"graceDays"`
	Signature       string              `json:"signature,omitempty"`
}

func signStoreSnapshot(snapshot storeSnapshot) (storeSnapshot, error) {
	key, err := loadStoreSnapshotPrivateKey()
	if err != nil {
		return storeSnapshot{}, err
	}
	snapshot.Signature = ""
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return storeSnapshot{}, err
	}
	sig := ed25519.Sign(key, payload)
	snapshot.Signature = "ed25519:" + base64.StdEncoding.EncodeToString(sig)
	return snapshot, nil
}

func verifyStoreSnapshot(snapshot storeSnapshot) bool {
	sig := strings.TrimPrefix(snapshot.Signature, "ed25519:")
	if sig == snapshot.Signature || sig == "" {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return false
	}
	snapshot.Signature = ""
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return false
	}
	storeSnapshotKeyMu.RLock()
	pub := storeSnapshotPublic
	storeSnapshotKeyMu.RUnlock()
	if len(pub) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(pub, payload, raw)
}

func deriveBindingSecret(master []byte, bindingID string, salt []byte) []byte {
	mac := hmac.New(sha256.New, master)
	_, _ = mac.Write([]byte(bindingID))
	_, _ = mac.Write(salt)
	return mac.Sum(nil)
}

func storeRequestSignature(secret []byte, method, path string, timestamp int64, nonce string, body []byte) string {
	sum := sha256.Sum256(body)
	mac := hmac.New(sha256.New, secret)
	payload := strings.Join([]string{
		strings.ToUpper(method),
		path,
		strconv.FormatInt(timestamp, 10),
		nonce,
		hex.EncodeToString(sum[:]),
	}, "\n")
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func stationChallengeReceipt(installID, nonce string) string {
	mac := hmac.New(sha256.New, []byte(installID))
	_, _ = mac.Write([]byte(nonce))
	return hex.EncodeToString(mac.Sum(nil))
}

func loadOrCreateStoreFileKey(name string) ([]byte, error) {
	dir := filepath.Join(config.GetDataDir(), "store")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, name)
	if payload, err := os.ReadFile(path); err == nil && len(payload) >= 32 {
		return payload[:32], nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0600); err != nil {
		return nil, err
	}
	return key, nil
}

func sealStoreSecret(key, secret []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return append(nonce, gcm.Seal(nil, nonce, secret, nil)...), nil
}

func openStoreSecret(key, blob []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(blob) < gcm.NonceSize() {
		return nil, errors.New("绑定秘密无效")
	}
	nonce, ciphertext := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("绑定秘密无效")
	}
	return plain, nil
}

type storeDownloadClaims struct {
	LicenseID  int64
	ItemKind   string
	ItemID     string
	Version    string
	StorageKey string
	Source     string
	ExpiresAt  int64
	Nonce      string
}

func createStoreDownloadToken(claims storeDownloadClaims) (string, error) {
	secret, err := loadOrCreateStoreFileKey("binding-master.key")
	if err != nil {
		return "", err
	}
	if claims.ExpiresAt == 0 {
		claims.ExpiresAt = time.Now().Add(storeDownloadTTL).Unix()
	}
	if claims.Nonce == "" {
		buf := make([]byte, 16)
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		claims.Nonce = hex.EncodeToString(buf)
	}
	payload := strings.Join([]string{
		strconv.FormatInt(claims.LicenseID, 10),
		claims.ItemKind,
		claims.ItemID,
		claims.Version,
		claims.StorageKey,
		claims.Source,
		strconv.FormatInt(claims.ExpiresAt, 10),
		claims.Nonce,
	}, ".")
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func parseStoreDownloadToken(token string) (storeDownloadClaims, error) {
	var claims storeDownloadClaims
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return claims, errors.New("下载令牌无效")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, errors.New("下载令牌无效")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, errors.New("下载令牌无效")
	}
	secret, err := loadOrCreateStoreFileKey("binding-master.key")
	if err != nil {
		return claims, err
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return claims, errors.New("下载令牌无效")
	}
	fields := strings.Split(string(payload), ".")
	if len(fields) != 8 {
		return claims, errors.New("下载令牌无效")
	}
	claims.LicenseID, _ = strconv.ParseInt(fields[0], 10, 64)
	claims.ItemKind = fields[1]
	claims.ItemID = fields[2]
	claims.Version = fields[3]
	claims.StorageKey = fields[4]
	claims.Source = fields[5]
	claims.ExpiresAt, _ = strconv.ParseInt(fields[6], 10, 64)
	claims.Nonce = fields[7]
	return claims, nil
}
