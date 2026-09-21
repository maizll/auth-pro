package handler

import (
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

func TestDedupePayOptionsKeepsPluginChannelAlongsideEasypay(t *testing.T) {
	options := dedupePayOptions([]payOption{
		{Code: "easypay:alipay", Channel: payChannelEpayV1, PayType: "alipay", Label: "支付宝"},
		{Code: "easypay-v2:alipay", Channel: payChannelEpayV2, PayType: "alipay", Label: "支付宝"},
		{Code: "alipay-f2f:alipay", Channel: alipayf2f.ChannelID, PayType: "alipay", Label: "支付宝当面付"},
		{Code: "balance", Label: "余额支付"},
	})
	if len(options) != 3 {
		t.Fatalf("got %d options, want 3 (easypay collapsed, plugin kept): %#v", len(options), options)
	}
	codes := map[string]bool{}
	for _, opt := range options {
		codes[opt.Code] = true
	}
	if !codes["easypay:alipay"] || !codes["alipay-f2f:alipay"] || !codes["balance"] {
		t.Fatalf("unexpected codes: %#v", options)
	}
	if codes["easypay-v2:alipay"] {
		t.Fatal("easypay v1/v2 alipay should still collapse")
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
