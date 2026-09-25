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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"auto_pro/config"
)

// storeSnapshotPublicKeyPlaceholder 表示发行包没有打入验签公钥。
// 买方会把所有快照当成无效并保持免费版；源站会拒绝签发。
const storeSnapshotPublicKeyPlaceholder = "PLACEHOLDER_NOT_CONFIGURED"

// embeddedStoreSnapshotPublicKey 可在构建时覆盖，只填源站 store-keygen 打印的公钥：
//
//	go build -ldflags "-X auto_pro/handler.embeddedStoreSnapshotPublicKey=<打印出的公钥>"
//
// 私钥只由源站 store-keygen 写到数据目录，不要放进 ldflags、环境变量或仓库。
var embeddedStoreSnapshotPublicKey = storeSnapshotPublicKeyPlaceholder

var (
	storeSnapshotKeyMu   sync.RWMutex
	storeSnapshotPublic  ed25519.PublicKey
	storeSnapshotPrivate ed25519.PrivateKey
)

func init() {
	applyEmbeddedStoreSnapshotPublicKey()
}

func applyEmbeddedStoreSnapshotPublicKey() {
	storeSnapshotKeyMu.Lock()
	defer storeSnapshotKeyMu.Unlock()
	storeSnapshotPublic = nil
	encoded := strings.TrimSpace(embeddedStoreSnapshotPublicKey)
	if encoded == "" || encoded == storeSnapshotPublicKeyPlaceholder {
		return
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return
	}
	storeSnapshotPublic = ed25519.PublicKey(raw)
}

func storeSnapshotPublicKeyConfigured() bool {
	storeSnapshotKeyMu.RLock()
	defer storeSnapshotKeyMu.RUnlock()
	return len(storeSnapshotPublic) == ed25519.PublicKeySize
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

func storeSnapshotPrivateKeyPath() string {
	return filepath.Join(config.GetDataDir(), "store", "snapshot-ed25519.key")
}

func loadStoreSnapshotPrivateKey() (ed25519.PrivateKey, error) {
	storeSnapshotKeyMu.RLock()
	pub := storeSnapshotPublic
	priv := storeSnapshotPrivate
	storeSnapshotKeyMu.RUnlock()
	if len(pub) != ed25519.PublicKeySize {
		return nil, errors.New("商店签名公钥未配置，拒绝签发快照。请在源站执行 store-keygen，把打印出的公钥交给维护者，用 ldflags 打进发行包后再发布")
	}
	if priv != nil {
		if !bytes.Equal(priv.Public().(ed25519.PublicKey), pub) {
			return nil, errors.New("商店签名私钥与发行包公钥不匹配")
		}
		return priv, nil
	}

	path := storeSnapshotPrivateKeyPath()
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("未配置商店签名私钥")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(payload)))
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return nil, errors.New("商店签名私钥格式不正确")
	}
	key := ed25519.PrivateKey(raw)
	if !bytes.Equal(key.Public().(ed25519.PublicKey), pub) {
		return nil, errors.New("商店签名私钥与发行包公钥不匹配")
	}
	return key, nil
}

// generateStoreSnapshotKey 在源站数据目录生成一对新的 Ed25519 密钥。
// 只把私钥写入 store/snapshot-ed25519.key（0600），返回值只有公钥。
func generateStoreSnapshotKey(force bool) (string, error) {
	path := storeSnapshotPrivateKeyPath()
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return "", err
	}
	if _, err := os.Lstat(path); err == nil {
		if !force {
			return "", errors.New("商店签名私钥已存在，拒绝覆盖。确认更换请加上 --force")
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(priv)
	tmp, err := os.CreateTemp(filepath.Dir(path), ".snapshot-ed25519-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if _, err := io.WriteString(tmp, encoded+"\n"); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return "", err
	}
	cleanup = false
	if err := os.Chmod(path, 0600); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(pub), nil
}

const storeKeygenHint = "请把上面这一行公钥交给维护者，用 ldflags 打进发行包。私钥已写入数据目录，不要复制或外传。"

func wantsStoreKeygen(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "store-keygen", "--store-keygen", "-store-keygen":
			return true
		}
	}
	return false
}

func storeKeygenForce(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "--force", "-force", "-f":
			return true
		}
	}
	return false
}

func runStoreKeygen(w io.Writer, force bool) error {
	pub, err := generateStoreSnapshotKey(force)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, pub); err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, storeKeygenHint)
	return err
}

// DispatchStoreKeygen 在启动 HTTP 服务之前处理一次性的 store-keygen 命令。
func DispatchStoreKeygen(args []string) {
	if !wantsStoreKeygen(args) {
		return
	}
	if err := runStoreKeygen(os.Stdout, storeKeygenForce(args)); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	os.Exit(0)
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
