package alipayf2f

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
)

func testKeyPair(t *testing.T) (*rsa.PrivateKey, string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	priv := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	pub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	return key, string(priv), string(pub)
}

func TestSignAndVerifyNotify(t *testing.T) {
	key, _, pubPEM := testKeyPair(t)
	params := map[string]string{
		"app_id":       "2021000000000000",
		"charset":      "utf-8",
		"gmt_create":   "2026-09-21 12:00:00",
		"notify_id":    "notify-1",
		"notify_time":  "2026-09-21 12:00:01",
		"notify_type":  "trade_status_sync",
		"out_trade_no": "UP123",
		"sign_type":    "RSA2",
		"total_amount": "1.00",
		"trade_no":     "2026092122000000000",
		"trade_status": "TRADE_SUCCESS",
		"seller_id":    "2088xxx",
	}
	sign, err := SignRSA2Notify(params, key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	params["sign"] = sign
	publicKey, err := parsePublicKey(pubPEM)
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}
	if !VerifyRSA2(params, publicKey) {
		t.Fatal("expected notify signature to verify")
	}
}

func TestVerifyNotifyRejectsTamperedAmount(t *testing.T) {
	key, _, pubPEM := testKeyPair(t)
	params := map[string]string{
		"app_id":       "2021000000000000",
		"out_trade_no": "UP123",
		"total_amount": "1.00",
		"trade_no":     "T1",
		"trade_status": "TRADE_SUCCESS",
		"sign_type":    "RSA2",
	}
	sign, err := SignRSA2Notify(params, key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	params["sign"] = sign
	params["total_amount"] = "999.00"
	publicKey, err := parsePublicKey(pubPEM)
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}
	if VerifyRSA2(params, publicKey) {
		t.Fatal("tampered amount must fail verification")
	}
}

func TestParseNotifySettlesSuccessfulTrade(t *testing.T) {
	key, privPEM, pubPEM := testKeyPair(t)
	params := map[string]string{
		"app_id":       "2021000000000000",
		"out_trade_no": "UP123",
		"total_amount": "8.50",
		"trade_no":     "20260921T",
		"trade_status": "TRADE_SUCCESS",
		"sign_type":    "RSA2",
	}
	sign, err := SignRSA2Notify(params, key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	params["sign"] = sign
	got, err := ParseNotify(Config{AppID: "2021000000000000", PrivateKey: privPEM, AlipayPublicKey: pubPEM}, params)
	if err != nil {
		t.Fatalf("parse notify: %v", err)
	}
	if !got.Success || got.OrderNo != "UP123" || got.AmountCents != 850 || got.GatewayTradeNo != "20260921T" || got.PayType != PayTypeAlipay {
		t.Fatalf("unexpected notify result: %#v", got)
	}
	if strings.Contains(got.RawPayload, sign) {
		t.Fatal("notify payload must not keep raw signature")
	}
	if strings.Contains(got.RawPayload, privPEM) {
		t.Fatal("notify payload must never contain private key")
	}
}

func TestParseNotifyRejectsWrongAppID(t *testing.T) {
	key, privPEM, pubPEM := testKeyPair(t)
	params := map[string]string{
		"app_id":       "wrong-app",
		"out_trade_no": "UP123",
		"total_amount": "1.00",
		"trade_no":     "T1",
		"trade_status": "TRADE_SUCCESS",
		"sign_type":    "RSA2",
	}
	sign, err := SignRSA2Notify(params, key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	params["sign"] = sign
	if _, err := ParseNotify(Config{AppID: "2021000000000000", PrivateKey: privPEM, AlipayPublicKey: pubPEM}, params); err == nil {
		t.Fatal("expected app id mismatch to fail")
	}
}

func TestPublicViewOmitsPrivateKey(t *testing.T) {
	cfg := Config{AppID: "2021", PrivateKey: "-----BEGIN RSA PRIVATE KEY-----\nsecret\n-----END RSA PRIVATE KEY-----", AlipayPublicKey: "pub"}
	view := cfg.Public()
	if view.PrivateKeySet != true {
		t.Fatal("expected privateKeySet")
	}
	encoded := view.AppID + view.AlipayPublicKey + view.Gateway
	if strings.Contains(encoded, "BEGIN RSA PRIVATE KEY") || strings.Contains(encoded, "secret") {
		t.Fatal("public view leaked private key material")
	}
}
