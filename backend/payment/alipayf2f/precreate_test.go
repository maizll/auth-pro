package alipayf2f

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"auto_pro/payment"
)

func TestPrecreateReturnsQRCode(t *testing.T) {
	_, privPEM, pubPEM := testKeyPair(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.Form.Get("method") != precreateMethod {
			t.Errorf("method = %q", r.Form.Get("method"))
		}
		if r.Form.Get("sign_type") != "RSA2" {
			t.Errorf("sign_type = %q", r.Form.Get("sign_type"))
		}
		if r.Form.Get("app_id") != "2021000000000000" {
			t.Errorf("app_id = %q", r.Form.Get("app_id"))
		}
		if strings.Contains(r.Form.Get("biz_content"), "BEGIN RSA") {
			t.Error("biz_content leaked private key")
		}
		body, _ := json.Marshal(map[string]any{
			"alipay_trade_precreate_response": map[string]string{
				"code":         "10000",
				"msg":          "Success",
				"out_trade_no": "UP123",
				"qr_code":      "https://qr.alipay.com/baxxxx",
			},
			"sign": "unused-in-p0",
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	cfg := Config{
		AppID:           "2021000000000000",
		PrivateKey:      privPEM,
		AlipayPublicKey: pubPEM,
		Gateway:         server.URL,
	}
	got, err := Precreate(cfg, payment.CreateRequest{
		OrderNo:     "UP123",
		AmountCents: 100,
		Subject:     "购买授权",
		NotifyURL:   "https://example.test/api/payment/alipay-f2f/notify",
		PayType:     PayTypeAlipay,
	}, server.Client())
	if err != nil {
		t.Fatalf("precreate: %v", err)
	}
	if got.Mode != payment.ModeQRCode || got.QRCode != "https://qr.alipay.com/baxxxx" {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestPrecreateSandboxGatewayDefault(t *testing.T) {
	cfg := Config{Sandbox: true}
	if cfg.gatewayURL() != SandboxGateway {
		t.Fatalf("sandbox gateway = %q", cfg.gatewayURL())
	}
	cfg.Sandbox = false
	if cfg.gatewayURL() != ProductionGateway {
		t.Fatalf("production gateway = %q", cfg.gatewayURL())
	}
}

func TestPrecreateRejectsIncompleteConfig(t *testing.T) {
	_, err := Precreate(Config{AppID: "x"}, payment.CreateRequest{OrderNo: "UP1", AmountCents: 1}, http.DefaultClient)
	if err == nil {
		t.Fatal("expected incomplete config to fail")
	}
}

func TestChannelOptionsAdvertiseF2F(t *testing.T) {
	opts := Channel{}.Options()
	if len(opts) != 1 || opts[0].Code != "alipay-f2f:alipay" || opts[0].Channel != ChannelID {
		t.Fatalf("unexpected options: %#v", opts)
	}
	if !payment.Has(ChannelID) {
		t.Fatal("init should register alipay-f2f")
	}
}

func TestParsePrecreateBodyErrorMessage(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"alipay_trade_precreate_response": map[string]string{
			"code":     "40004",
			"msg":      "Business Failed",
			"sub_code": "ACQ.INVALID_PARAMETER",
			"sub_msg":  "参数无效",
		},
	})
	if _, err := parsePrecreateBody(body); err == nil || !strings.Contains(err.Error(), "参数无效") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPClientDoesNotLogBodyOnSuccess(t *testing.T) {
	// 回归：读取响应必须 LimitReader，避免把证书/密钥大段写入内存日志。
	body := []byte(`{"alipay_trade_precreate_response":{"code":"10000","qr_code":"https://qr.alipay.com/ok"}}`)
	if _, err := parsePrecreateBody(body); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(strings.NewReader("ok")); err != nil {
		t.Fatal(err)
	}
}
