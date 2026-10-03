// 授权响应签名：授权校验类响应（客户站向官网刷新商业版、业务软件 SDK 校验授权）带 Ed25519 签名，
// 签名覆盖应用、域名、请求方发来的随机数和服务器时间，收到的一方用内置公钥验签，改 hosts 指向假服务器或重放旧响应都通不过。
//
// 用哪把私钥：
//   - 官网：就是商业版快照那把（数据目录 store/snapshot-ed25519.key，只在官网服务器上；公钥早已编进每个客户站程序）。
//     客户站刷新商业版、官网自己应用的 SDK 校验都用它。
//   - 客户站：给自己应用的 SDK 校验签名，用本机第一次需要时生成的 store/license-response-ed25519.key，公钥写进接入包。
//
// 和更新包签名分开：更新包私钥离线保存，只在发布流程里用；授权私钥必须在线签每个响应。
// 在线服务器被攻破时，攻击者最多伪造授权结果，推不了带木马的更新包；反过来发布流程泄漏也伪造不了授权。
package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"auto_pro/config"
)

const (
	responseProofLicense   = "auth-pro-license-v3"
	responseProofStoreBind = "auth-pro-store-bind-v1"
	responseProofStoreStat = "auth-pro-store-status-v1"
	responseProofPrefix    = "ed25519:"
)

var (
	responseKeyMu     sync.Mutex
	responseKeyCached ed25519.PrivateKey
	responseKeyDir    string
	// responseKeyOfficialForTest 让同包测试指定「当前是不是官网」，正式程序为 nil，按 officialSite() 判断。
	responseKeyOfficialForTest *bool
)

func licenseResponseKeyPath() string {
	return filepath.Join(config.GetDataDir(), "store", "license-response-ed25519.key")
}

// responseSigningKey 返回给授权响应签名的私钥。官网用快照私钥；客户站用本机授权响应私钥，没有就生成（0600）。
// 数据目录变了（测试换目录）时重新加载。
func responseSigningKey() (ed25519.PrivateKey, error) {
	responseKeyMu.Lock()
	defer responseKeyMu.Unlock()
	dir := config.GetDataDir()
	if responseKeyCached != nil && responseKeyDir == dir {
		return responseKeyCached, nil
	}
	official := officialSite()
	if responseKeyOfficialForTest != nil {
		official = *responseKeyOfficialForTest
	}
	var key ed25519.PrivateKey
	var err error
	if official {
		key, err = loadStoreSnapshotPrivateKey()
	} else {
		key, err = loadOrCreateLicenseResponseKey()
	}
	if err != nil {
		return nil, err
	}
	responseKeyCached, responseKeyDir = key, dir
	return key, nil
}

func resetResponseSigningKeyCache() {
	responseKeyMu.Lock()
	responseKeyCached, responseKeyDir = nil, ""
	responseKeyMu.Unlock()
}

func loadOrCreateLicenseResponseKey() (ed25519.PrivateKey, error) {
	path := licenseResponseKeyPath()
	if payload, err := os.ReadFile(path); err == nil {
		raw, decodeErr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(payload)))
		if decodeErr != nil || len(raw) != ed25519.PrivateKeySize {
			return nil, errors.New("授权响应签名私钥格式不正确")
		}
		key := ed25519.NewKeyFromSeed(raw[:ed25519.SeedSize])
		return key, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, err
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(base64.StdEncoding.EncodeToString(priv)+"\n"), 0600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	return priv, nil
}

// LicenseResponsePublicKey 返回本站授权响应公钥（标准 base64），写进接入包和接入文档。
func LicenseResponsePublicKey() (string, error) {
	key, err := responseSigningKey()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey)), nil
}

type proofField struct {
	Key, Value string
}

// responseProofText 是签名原文：第一行是用途，之后每行 key=value。值里的换行换成空格，不能借换行伪造字段。
// SDK 和客户站按同样的顺序拼出原文再验签，规则写在 docs/api-sdk.md。
func responseProofText(kind string, fields []proofField) string {
	var b strings.Builder
	b.WriteString(kind)
	b.WriteByte('\n')
	for _, f := range fields {
		b.WriteString(f.Key)
		b.WriteByte('=')
		b.WriteString(strings.NewReplacer("\r", " ", "\n", " ").Replace(f.Value))
		b.WriteByte('\n')
	}
	return b.String()
}

func signResponseProof(kind string, fields []proofField) (string, error) {
	key, err := responseSigningKey()
	if err != nil {
		return "", err
	}
	return signResponseProofWith(key, kind, fields), nil
}

func signResponseProofWith(key ed25519.PrivateKey, kind string, fields []proofField) string {
	sig := ed25519.Sign(key, []byte(responseProofText(kind, fields)))
	return responseProofPrefix + base64.StdEncoding.EncodeToString(sig)
}

func verifyResponseProof(pub ed25519.PublicKey, kind string, fields []proofField, signature string) bool {
	raw := strings.TrimPrefix(strings.TrimSpace(signature), responseProofPrefix)
	if len(pub) != ed25519.PublicKeySize || raw == signature || raw == "" {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return false
	}
	return ed25519.Verify(pub, []byte(responseProofText(kind, fields)), sig)
}

// validProofNonce 只接受 16–64 位字母、数字、下划线、短横线，签名原文里不会混进分隔符。
func validProofNonce(nonce string) bool {
	if len(nonce) < 16 || len(nonce) > 64 {
		return false
	}
	for _, r := range nonce {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func unixText(v int64) string {
	return strconv.FormatInt(v, 10)
}
