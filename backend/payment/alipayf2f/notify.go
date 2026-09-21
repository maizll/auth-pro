package alipayf2f

import (
	"encoding/json"
	"errors"
	"strings"

	"auto_pro/payment"
)

func parseAmountToCents(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("金额为空")
	}
	negative := false
	if strings.HasPrefix(raw, "-") {
		negative = true
		raw = strings.TrimPrefix(raw, "-")
	}
	parts := strings.SplitN(raw, ".", 2)
	yuan := int64(0)
	for _, ch := range parts[0] {
		if ch < '0' || ch > '9' {
			return 0, errors.New("金额格式错误")
		}
		yuan = yuan*10 + int64(ch-'0')
	}
	cent := int64(0)
	if len(parts) == 2 {
		frac := parts[1]
		if len(frac) == 1 {
			frac += "0"
		}
		if len(frac) > 2 {
			frac = frac[:2]
		}
		for _, ch := range frac {
			if ch < '0' || ch > '9' {
				return 0, errors.New("金额格式错误")
			}
			cent = cent*10 + int64(ch-'0')
		}
	}
	total := yuan*100 + cent
	if negative {
		total = -total
	}
	return total, nil
}

func formatCents(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return sign + itoa(cents/100) + "." + pad2(cents%100)
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func pad2(v int64) string {
	return string([]byte{'0' + byte(v/10), '0' + byte(v%10)})
}

// ParseNotify 校验支付宝异步通知并标准化为 NotifyResult。
func ParseNotify(cfg Config, values map[string]string) (payment.NotifyResult, error) {
	publicKey, err := parsePublicKey(cfg.AlipayPublicKey)
	if err != nil {
		return payment.NotifyResult{}, errors.New("支付宝公钥无效")
	}
	if !VerifyRSA2(values, publicKey) {
		return payment.NotifyResult{}, errors.New("验签失败")
	}
	if appID := strings.TrimSpace(values["app_id"]); appID != "" && appID != cfg.AppID {
		return payment.NotifyResult{}, errors.New("应用 APPID 不匹配")
	}
	status := strings.ToUpper(strings.TrimSpace(values["trade_status"]))
	success := status == "TRADE_SUCCESS" || status == "TRADE_FINISHED"
	orderNo := strings.TrimSpace(values["out_trade_no"])
	tradeNo := strings.TrimSpace(values["trade_no"])
	amountRaw := strings.TrimSpace(values["total_amount"])
	if amountRaw == "" {
		amountRaw = strings.TrimSpace(values["receipt_amount"])
	}
	cents, err := parseAmountToCents(amountRaw)
	if orderNo == "" || tradeNo == "" || err != nil || cents <= 0 {
		return payment.NotifyResult{}, errors.New("回调参数不完整")
	}
	payload, _ := json.Marshal(redactNotify(values))
	return payment.NotifyResult{
		OrderNo:        orderNo,
		AmountCents:    cents,
		PayType:        PayTypeAlipay,
		GatewayTradeNo: tradeNo,
		Success:        success,
		RawPayload:     string(payload),
	}, nil
}

func redactNotify(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for k, v := range values {
		switch k {
		case "sign", "alipay_cert_sn":
			out[k] = "[redacted]"
		default:
			out[k] = v
		}
	}
	return out
}
