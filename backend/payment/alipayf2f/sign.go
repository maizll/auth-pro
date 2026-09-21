package alipayf2f

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"sort"
	"strings"
)

// SignRSA2 按支付宝开放平台请求签名规则生成 RSA2 签名（包含 sign_type，排除 sign）。
func SignRSA2(params map[string]string, privateKey *rsa.PrivateKey) (string, error) {
	return signRSA2(params, privateKey, false)
}

// SignRSA2Notify 按异步通知验签材料规则签名（排除 sign 与 sign_type），仅用于测试夹具。
func SignRSA2Notify(params map[string]string, privateKey *rsa.PrivateKey) (string, error) {
	return signRSA2(params, privateKey, true)
}

func signRSA2(params map[string]string, privateKey *rsa.PrivateKey, notify bool) (string, error) {
	if privateKey == nil {
		return "", errors.New("缺少商户私钥")
	}
	sum := sha256.Sum256([]byte(signSource(params, notify)))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

// VerifyRSA2 校验支付宝异步通知签名。验签材料排除 sign 与 sign_type。
func VerifyRSA2(params map[string]string, publicKey *rsa.PublicKey) bool {
	if publicKey == nil {
		return false
	}
	signText := strings.TrimSpace(params["sign"])
	if signText == "" {
		return false
	}
	signature, err := base64.StdEncoding.DecodeString(signText)
	if err != nil {
		return false
	}
	sum := sha256.Sum256([]byte(signSource(params, true)))
	return rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, sum[:], signature) == nil
}

func signSource(params map[string]string, notify bool) string {
	keys := make([]string, 0, len(params))
	for name, value := range params {
		if name == "sign" || strings.TrimSpace(value) == "" {
			continue
		}
		if notify && name == "sign_type" {
			continue
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, name := range keys {
		parts = append(parts, name+"="+params[name])
	}
	return strings.Join(parts, "&")
}

func parsePrivateKey(raw string) (*rsa.PrivateKey, error) {
	der, err := normalizeKey(raw, true)
	if err != nil {
		return nil, err
	}
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("应用私钥必须是 RSA 私钥")
		}
		return rsaKey, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	return nil, errors.New("应用私钥解析失败，请确认是 RSA 私钥（PKCS1/PKCS8）")
}

func parsePublicKey(raw string) (*rsa.PublicKey, error) {
	der, err := normalizeKey(raw, false)
	if err != nil {
		return nil, err
	}
	if key, err := x509.ParsePKIXPublicKey(der); err == nil {
		rsaKey, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("支付宝公钥必须是 RSA 公钥")
		}
		return rsaKey, nil
	}
	if key, err := x509.ParsePKCS1PublicKey(der); err == nil {
		return key, nil
	}
	return nil, errors.New("支付宝公钥解析失败，请确认是 RSA 公钥")
}

func normalizeKey(raw string, isPrivate bool) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if isPrivate {
			return nil, errors.New("应用私钥不能为空")
		}
		return nil, errors.New("支付宝公钥不能为空")
	}
	raw = strings.ReplaceAll(raw, "\\n", "\n")
	if block, _ := pem.Decode([]byte(raw)); block != nil {
		return block.Bytes, nil
	}
	compact := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
			return -1
		}
		return r
	}, raw)
	der, err := base64.StdEncoding.DecodeString(compact)
	if err != nil {
		if isPrivate {
			return nil, errors.New("应用私钥不是有效的 PEM 或 base64 格式")
		}
		return nil, errors.New("支付宝公钥不是有效的 PEM 或 base64 格式")
	}
	return der, nil
}

func encodeJSON(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
