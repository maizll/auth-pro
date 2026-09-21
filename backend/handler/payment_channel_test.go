package handler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"auto_pro/payment"
	"auto_pro/payment/alipayf2f"
)

func TestPluginCatalogIncludesAlipayF2F(t *testing.T) {
	plugin, ok := findCatalogPlugin("alipay-f2f")
	if !ok {
		t.Fatal("alipay-f2f missing from plugin catalog")
	}
	if plugin.Category != "payment" || !plugin.Official || plugin.Name != "支付宝当面付" {
		t.Fatalf("unexpected catalog entry: %#v", plugin)
	}
	if !plugin.CanEnable {
		t.Fatal("official alipay-f2f catalog must be enableable (compiled Channel)")
	}
	if !pluginHasCompiledRuntime("alipay-f2f") {
		t.Fatal("alipay-f2f must report compiled runtime")
	}
}

func TestPaymentCategoryIsNotExclusive(t *testing.T) {
	if pluginCategoryIsExclusive("payment") {
		t.Fatal("payment plugins must be allowed to coexist")
	}
	if !pluginCategoryIsExclusive("realname") {
		t.Fatal("realname plugins remain mutually exclusive")
	}
}

func TestParseOnlinePaySelectionAcceptsAlipayF2F(t *testing.T) {
	got, ok := parseOnlinePaySelection("alipay-f2f:alipay")
	if !ok || got.Channel != alipayf2f.ChannelID || got.PayType != "alipay" {
		t.Fatalf("parse alipay-f2f:alipay = %#v, %v", got, ok)
	}
	got, ok = parseOnlinePaySelection("alipay-f2f")
	if !ok || got.Channel != alipayf2f.ChannelID || got.PayType != "alipay" {
		t.Fatalf("parse alipay-f2f = %#v, %v", got, ok)
	}
	if _, ok := parseOnlinePaySelection("alipay-f2f:wxpay"); ok {
		t.Fatal("wxpay is not a P0 F2F pay type")
	}
}

func TestDedupePayOptionsSoftDedupePrefersOfficial(t *testing.T) {
	options := dedupePayOptions([]payOption{
		{Code: "easypay:alipay", Channel: payChannelEpayV1, PayType: "alipay", Label: "支付宝"},
		{Code: "easypay-v2:alipay", Channel: payChannelEpayV2, PayType: "alipay", Label: "支付宝"},
		{Code: "easypay:wxpay", Channel: payChannelEpayV1, PayType: "wxpay", Label: "微信"},
		{Code: "alipay-f2f:alipay", Channel: alipayf2f.ChannelID, PayType: "alipay", Label: "支付宝当面付"},
		{Code: "balance", Label: "余额支付"},
	})
	if len(options) != 3 {
		t.Fatalf("got %d options, want official alipay + easypay wxpay + balance: %#v", len(options), options)
	}
	codes := map[string]bool{}
	for _, opt := range options {
		codes[opt.Code] = true
	}
	if !codes["alipay-f2f:alipay"] || !codes["easypay:wxpay"] || !codes["balance"] {
		t.Fatalf("unexpected codes: %#v", options)
	}
	if codes["easypay:alipay"] || codes["easypay-v2:alipay"] {
		t.Fatal("same pay method must prefer official face-to-face over easypay")
	}
}

func TestDedupePayOptionsCollapsesEpayVersions(t *testing.T) {
	options := dedupePayOptions([]payOption{
		{Code: "easypay:alipay", Channel: payChannelEpayV1, PayType: "alipay", Label: "支付宝"},
		{Code: "easypay-v2:alipay", Channel: payChannelEpayV2, PayType: "alipay", Label: "支付宝"},
		{Code: "balance", Label: "余额支付"},
	})
	if len(options) != 2 || options[0].Code != "easypay:alipay" || options[1].Code != "balance" {
		t.Fatalf("epay v1/v2 must hard-collapse to the first option, got %#v", options)
	}
}

func TestEpayHardExclusivePeer(t *testing.T) {
	peer, ok := epayHardExclusivePeer("epay")
	if !ok || peer != "epay-v2" {
		t.Fatalf("epay peer = %q %v", peer, ok)
	}
	peer, ok = epayHardExclusivePeer("epay-v2")
	if !ok || peer != "epay" {
		t.Fatalf("epay-v2 peer = %q %v", peer, ok)
	}
	if _, ok := epayHardExclusivePeer("alipay-f2f"); ok {
		t.Fatal("official direct channel must stay soft-coexistent with easypay")
	}
}

func TestAlipayF2FNotifySettlesLicensePurchase(t *testing.T) {
	state := &purchaseLicenseTypeTestState{
		appExists:          true,
		purchasePayChannel: alipayf2f.ChannelID,
		purchasePayMethod:  "alipay",
	}
	db := openPurchaseLicenseTypeTestDB(t, state)
	previousQueue := queuePurchaseSuccessMail
	queuePurchaseSuccessMail = func(string, int64, int64) {}
	t.Cleanup(func() { queuePurchaseSuccessMail = previousQueue })

	result := payment.NotifyResult{
		OrderNo:        "LP-existing",
		AmountCents:    1000,
		PayType:        "alipay",
		GatewayTradeNo: "20260921ALIPAY",
		Success:        true,
		RawPayload:     `{"trade_status":"TRADE_SUCCESS"}`,
	}
	if err := settleRegisteredChannelNotify(db, alipayf2f.ChannelID, result); err != nil {
		t.Fatalf("settle alipay f2f purchase: %v", err)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	paid := false
	for _, query := range state.execQueries {
		if strings.Contains(query, "SET status = 'paid'") {
			paid = true
		}
	}
	if !paid {
		t.Fatal("expected license purchase order to be marked paid")
	}
}

func TestOfficialAlipayF2FPluginManifestParses(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "plugins", "alipay-f2f", "plugin.json"))
	if err != nil {
		t.Fatalf("read plugin.json: %v", err)
	}
	manifest, err := fillPluginManifest(sourcePackageManifest{}, raw, "plugin.json")
	if err != nil {
		t.Fatalf("fill plugin manifest: %v", err)
	}
	if manifest.ID != "alipay-f2f" || manifest.Category != "payment" || manifest.Kind != sourceKindPlugin {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}

func TestRegisteredChannelIsPresent(t *testing.T) {
	if !payment.Has(alipayf2f.ChannelID) {
		t.Fatal("alipay-f2f channel should register on import")
	}
}

func TestAlipayF2FConfigViewOmitsPrivateKey(t *testing.T) {
	view := alipayF2FConfigView(alipayf2f.Config{
		AppID:           "2021000000000000",
		PrivateKey:      "-----BEGIN RSA PRIVATE KEY-----\nsecret-private\n-----END RSA PRIVATE KEY-----",
		AlipayPublicKey: "alipay-public",
		Sandbox:         true,
	})
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["privateKey"]; ok {
		t.Fatalf("admin GET must not return privateKey: %s", raw)
	}
	if payload["privateKeySet"] != true {
		t.Fatalf("privateKeySet=%v, want true", payload["privateKeySet"])
	}
	if payload["appId"] != "2021000000000000" || payload["alipayPublicKey"] != "alipay-public" {
		t.Fatalf("unexpected public fields: %s", raw)
	}
	if strings.Contains(string(raw), "secret-private") {
		t.Fatalf("leaked private key: %s", raw)
	}
}
