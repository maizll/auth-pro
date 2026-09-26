package alipayf2f

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"auto_pro/payment"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestPrecreateReturnsQRCode(t *testing.T) {
	priv, privPEM, pubPEM := testKeyPair(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("charset") != "utf-8" {
			t.Errorf("gateway query charset = %q", r.URL.Query().Get("charset"))
		}
		contentType := r.Header.Get("Content-Type")
		if !strings.Contains(strings.ToLower(contentType), "application/x-www-form-urlencoded") || !strings.Contains(strings.ToLower(contentType), "charset=utf-8") {
			t.Errorf("content-type = %q", contentType)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.Form.Get("method") != precreateMethod {
			t.Errorf("method = %q", r.Form.Get("method"))
		}
		if r.Form.Get("charset") != "utf-8" {
			t.Errorf("form charset = %q", r.Form.Get("charset"))
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
		params := map[string]string{}
		for key, values := range r.PostForm {
			if len(values) > 0 {
				params[key] = values[0]
			}
		}
		if err := verifyRequestSign(params, &priv.PublicKey); err != nil {
			t.Errorf("request sign: %v", err)
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
	if _, err := parsePrecreateBody("application/json;charset=utf-8", body); err == nil || !strings.Contains(err.Error(), "下单参数不正确") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParsePrecreateBodyDecodesGBKSignatureError(t *testing.T) {
	raw := []byte(`{"alipay_trade_precreate_response":{"code":"40004","msg":"Business Failed","sub_code":"ACQ.TRADE_HAS_SUCCESS","sub_msg":"交易已经支付"}}`)
	encoded, err := simplifiedchinese.GB18030.NewEncoder().Bytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	_, err = parsePrecreateBody("application/json;charset=GBK", encoded)
	if err == nil || err.Error() != "交易已经支付" {
		t.Fatalf("unexpected error: %v", err)
	}
	mapped := []byte(`{"alipay_trade_precreate_response":{"code":"40002","msg":"Invalid Arguments","sub_code":"isv.invalid-signature","sub_msg":"验签出错"}}`)
	gbk, err := simplifiedchinese.GB18030.NewEncoder().Bytes(mapped)
	if err != nil {
		t.Fatal(err)
	}
	_, err = parsePrecreateBody("application/json;charset=GBK", gbk)
	if err == nil || !strings.Contains(err.Error(), "验签失败") || strings.Contains(err.Error(), "\uFFFD") {
		t.Fatalf("unexpected mapped error: %v", err)
	}
}

func TestParsePrecreateBodyExplainsBadAppID(t *testing.T) {
	body := []byte(`{"error_response":{"code":"40002","msg":"Invalid Arguments","sub_code":"isv.invalid-app-id","sub_msg":"无效的AppID参数"}}`)
	_, err := parsePrecreateBody("application/json", body)
	if err == nil || !strings.Contains(err.Error(), "AppID 不正确") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func verifyRequestSign(params map[string]string, publicKey *rsa.PublicKey) error {
	signText := params["sign"]
	delete(params, "sign")
	source := signSource(params, false)
	prev := ""
	for _, part := range strings.Split(source, "&") {
		name, _, _ := strings.Cut(part, "=")
		if name == "sign" {
			return errors.New("签名串不应包含 sign")
		}
		if name < prev {
			return errors.New("签名串未按参数名排序")
		}
		prev = name
	}
	if !strings.Contains(source, "sign_type=RSA2") || !strings.Contains(source, "charset=utf-8") {
		return errors.New("签名串缺少 charset 或 sign_type")
	}
	signature, err := base64.StdEncoding.DecodeString(signText)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(source))
	return rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, sum[:], signature)
}

func TestHTTPClientDoesNotLogBodyOnSuccess(t *testing.T) {
	// 回归：读取响应必须 LimitReader，避免把证书/密钥大段写入内存日志。
	body := []byte(`{"alipay_trade_precreate_response":{"code":"10000","qr_code":"https://qr.alipay.com/ok"}}`)
	if _, err := parsePrecreateBody("application/json", body); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(strings.NewReader("ok")); err != nil {
		t.Fatal(err)
	}
}
