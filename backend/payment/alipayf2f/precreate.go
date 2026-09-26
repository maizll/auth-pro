package alipayf2f

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"auto_pro/payment"

	"golang.org/x/text/encoding/simplifiedchinese"
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
	endpoint, err := gatewayEndpoint(cfg.gatewayURL())
	if err != nil {
		return payment.CreateResult{}, err
	}
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	httpResp, err := postGateway(client, endpoint, form)
	if err != nil {
		return payment.CreateResult{}, fmt.Errorf("请求支付宝网关失败")
	}
	defer httpResp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	if err != nil {
		return payment.CreateResult{}, errors.New("读取支付宝响应失败")
	}
	qrCode, err := parsePrecreateBody(httpResp.Header.Get("Content-Type"), body)
	if err != nil {
		return payment.CreateResult{}, err
	}
	return payment.CreateResult{Mode: payment.ModeQRCode, QRCode: qrCode}, nil
}

// gatewayEndpoint 把 charset=utf-8 放进网关 URL 查询串。
// 支付宝网关在查询串里看不到 charset 时按 GBK 验签，UTF-8 签名会被判失败，错误正文也是 GBK。
func gatewayEndpoint(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("支付宝网关地址无效")
	}
	query := parsed.Query()
	query.Set("charset", "utf-8")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func postGateway(client *http.Client, endpoint string, form url.Values) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")
	return client.Do(req)
}

func parsePrecreateBody(contentType string, body []byte) (string, error) {
	body = decodeGatewayCharset(contentType, body)
	var wrapped map[string]json.RawMessage
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return "", errors.New("支付宝响应解析失败")
	}
	raw, ok := wrapped["alipay_trade_precreate_response"]
	if !ok {
		raw, ok = wrapped["error_response"]
	}
	if !ok {
		if _, hasCode := wrapped["code"]; hasCode {
			raw = body
			ok = true
		}
	}
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
		return "", errors.New(explainAlipayFailure(node.SubCode, node.SubMsg, node.Msg))
	}
	qr := strings.TrimSpace(node.QRCode)
	if qr == "" {
		return "", errors.New("支付宝未返回收款码")
	}
	return qr, nil
}

func decodeGatewayCharset(contentType string, body []byte) []byte {
	charset := charsetFromContentType(contentType)
	switch charset {
	case "gbk", "gb2312", "gb18030":
		return decodeGB18030(body)
	case "utf-8", "utf8":
		if utf8.Valid(body) {
			return body
		}
		return decodeGB18030(body)
	default:
		if utf8.Valid(body) {
			return body
		}
		return decodeGB18030(body)
	}
}

func charsetFromContentType(contentType string) string {
	if strings.TrimSpace(contentType) == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(params["charset"]))
}

func decodeGB18030(body []byte) []byte {
	decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(body)
	if err != nil || len(decoded) == 0 {
		return body
	}
	return decoded
}

func explainAlipayFailure(subCode, subMsg, msg string) string {
	switch strings.TrimSpace(subCode) {
	case "isv.invalid-signature":
		return "验签失败。请核对应用私钥与开放平台的应用公钥是否为一对，支付宝公钥要填开放平台给出的「支付宝公钥」，不要填成应用公钥。"
	case "isv.missing-signature":
		return "请求缺少签名。"
	case "isv.invalid-signature-type":
		return "签名类型不正确。当面付需要使用 RSA2。"
	case "isv.invalid-app-id", "isv.missing-app-id", "isv.app-not-exist":
		return "AppID 不正确。请核对开放平台应用的 APPID。"
	case "isv.insufficient-isv-permissions", "isv.insufficient-user-permissions":
		return "该应用没有当面付权限。请在开放平台为这个应用开通「当面付」。"
	case "isv.missing-method":
		return "请求缺少接口名称。"
	case "ACQ.INVALID_PARAMETER":
		return "下单参数不正确。" + suffixAlipayDetail(subMsg)
	}
	detail := strings.TrimSpace(subMsg)
	if detail == "" {
		detail = strings.TrimSpace(msg)
	}
	if detail == "" {
		return "预下单失败"
	}
	return detail
}

func suffixAlipayDetail(subMsg string) string {
	detail := strings.TrimSpace(subMsg)
	if detail == "" {
		return ""
	}
	return "（" + detail + "）"
}
