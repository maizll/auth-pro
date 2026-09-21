package alipayf2f

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"auto_pro/payment"
)

const precreateMethod = "alipay.trade.precreate"

var httpClient = &http.Client{Timeout: 15 * time.Second}

type precreateBiz struct {
	OutTradeNo  string `json:"out_trade_no"`
	TotalAmount string `json:"total_amount"`
	Subject     string `json:"subject"`
}

// Precreate 调用 alipay.trade.precreate，返回收款二维码内容。
func Precreate(cfg Config, req payment.CreateRequest, client *http.Client) (payment.CreateResult, error) {
	if !cfg.ready() {
		return payment.CreateResult{}, errors.New("支付宝当面付未完成配置")
	}
	if req.OrderNo == "" || req.AmountCents <= 0 {
		return payment.CreateResult{}, errors.New("订单参数不完整")
	}
	privateKey, err := parsePrivateKey(cfg.PrivateKey)
	if err != nil {
		return payment.CreateResult{}, errors.New("应用私钥无效")
	}
	notifyURL := strings.TrimSpace(req.NotifyURL)
	if notifyURL == "" {
		notifyURL = strings.TrimSpace(cfg.NotifyURL)
	}
	biz, err := encodeJSON(precreateBiz{
		OutTradeNo:  req.OrderNo,
		TotalAmount: formatCents(req.AmountCents),
		Subject:     strings.TrimSpace(req.Subject),
	})
	if err != nil {
		return payment.CreateResult{}, err
	}
	params := map[string]string{
		"app_id":      cfg.AppID,
		"method":      precreateMethod,
		"format":      "JSON",
		"charset":     "utf-8",
		"sign_type":   "RSA2",
		"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
		"version":     "1.0",
		"biz_content": biz,
	}
	if notifyURL != "" {
		params["notify_url"] = notifyURL
	}
	if cfg.CertMode {
		params["app_cert_sn"] = cfg.AppCertSN
		params["alipay_root_cert_sn"] = cfg.AlipayRootCertSN
	}
	sign, err := SignRSA2(params, privateKey)
	if err != nil {
		return payment.CreateResult{}, errors.New("签名失败")
	}
	params["sign"] = sign

	if client == nil {
		client = httpClient
	}
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	httpResp, err := client.PostForm(cfg.gatewayURL(), form)
	if err != nil {
		return payment.CreateResult{}, fmt.Errorf("请求支付宝网关失败")
	}
	defer httpResp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	if err != nil {
		return payment.CreateResult{}, errors.New("读取支付宝响应失败")
	}
	qrCode, err := parsePrecreateBody(body)
	if err != nil {
		return payment.CreateResult{}, err
	}
	return payment.CreateResult{Mode: payment.ModeQRCode, QRCode: qrCode}, nil
}

func parsePrecreateBody(body []byte) (string, error) {
	var wrapped map[string]json.RawMessage
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return "", errors.New("支付宝响应解析失败")
	}
	raw, ok := wrapped["alipay_trade_precreate_response"]
	if !ok {
		return "", errors.New("支付宝未返回预下单结果")
	}
	var node struct {
		Code    string `json:"code"`
		Msg     string `json:"msg"`
		SubCode string `json:"sub_code"`
		SubMsg  string `json:"sub_msg"`
		QRCode  string `json:"qr_code"`
	}
	if err := json.Unmarshal(raw, &node); err != nil {
		return "", errors.New("支付宝预下单结果解析失败")
	}
	if node.Code != "10000" {
		detail := strings.TrimSpace(node.SubMsg)
		if detail == "" {
			detail = strings.TrimSpace(node.Msg)
		}
		if detail == "" {
			detail = "预下单失败"
		}
		return "", errors.New(detail)
	}
	qr := strings.TrimSpace(node.QRCode)
	if qr == "" {
		return "", errors.New("支付宝未返回收款码")
	}
	return qr, nil
}
