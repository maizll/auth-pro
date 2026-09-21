package payment

import (
	"database/sql"
	"testing"
)

type stubChannel struct {
	id       string
	pluginID string
	options  []Option
}

func (s stubChannel) ID() string             { return s.id }
func (s stubChannel) PluginID() string       { return s.pluginID }
func (s stubChannel) Options() []Option      { return s.options }
func (s stubChannel) Available(*sql.DB) bool { return true }
func (s stubChannel) CreatePayment(*sql.DB, CreateRequest) (CreateResult, error) {
	return CreateResult{Mode: ModeQRCode, QRCode: "https://qr.example/test"}, nil
}
func (s stubChannel) ParseNotify(*sql.DB, map[string]string) (NotifyResult, error) {
	return NotifyResult{Success: true, OrderNo: "UP1", AmountCents: 100, PayType: "alipay", GatewayTradeNo: "t"}, nil
}

func TestRegisterContributesPayOptions(t *testing.T) {
	Register(stubChannel{
		id:       "alipay-f2f",
		pluginID: "alipay-f2f",
		options: []Option{{
			Code:    "alipay-f2f:alipay",
			Channel: "alipay-f2f",
			PayType: "alipay",
			Label:   "支付宝当面付",
			Icon:    "ri:alipay-fill",
			Color:   "#1677ff",
		}},
	})

	ch, ok := Get("alipay-f2f")
	if !ok {
		t.Fatal("expected alipay-f2f to be registered")
	}
	if !Has("alipay-f2f") {
		t.Fatal("Has(alipay-f2f) = false")
	}
	opts := ch.Options()
	if len(opts) != 1 || opts[0].Code != "alipay-f2f:alipay" {
		t.Fatalf("unexpected options: %#v", opts)
	}
	if !SupportsPayType(ch, "alipay") {
		t.Fatal("channel should support alipay pay type")
	}
	if SupportsPayType(ch, "wxpay") {
		t.Fatal("channel should not advertise wxpay in P0")
	}

	found := false
	for _, item := range List() {
		if item.ID() == "alipay-f2f" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("List() missing alipay-f2f")
	}
}
