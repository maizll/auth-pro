package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"auto_pro/payment"
	_ "auto_pro/payment/alipayf2f"

	"github.com/gin-gonic/gin"
)

func pluginCategoryIsExclusive(category string) bool {
	return strings.TrimSpace(category) != "payment"
}

func pluginPayOptions(db *sql.DB) []payOption {
	if db == nil {
		return nil
	}
	out := []payOption{}
	for _, ch := range payment.List() {
		if !isPluginEnabled(db, ch.PluginID()) || !ch.Available(db) {
			continue
		}
		for _, opt := range ch.Options() {
			out = append(out, payOption{
				Code:    opt.Code,
				Channel: opt.Channel,
				PayType: opt.PayType,
				Label:   opt.Label,
				Icon:    opt.Icon,
				Color:   opt.Color,
			})
		}
	}
	return out
}

func isRegisteredPayChannel(id string) bool {
	return payment.Has(strings.TrimSpace(id))
}

func checkoutData(orderNo, amount, payType string, result payment.CreateResult) gin.H {
	data := gin.H{
		"orderNo":      orderNo,
		"amount":       amount,
		"payType":      payType,
		"payUrl":       result.PayURL,
		"checkoutMode": result.Mode,
	}
	if result.QRCode != "" {
		data["qrCode"] = result.QRCode
	}
	if result.Mode == "" && result.PayURL != "" {
		data["checkoutMode"] = payment.ModeRedirect
	}
	return data
}

func createRegisteredChannelPayment(c *gin.Context, db *sql.DB, selection onlinePaySelection, orderNo string, amountCents int64, subject, returnPath string) (payment.CreateResult, string, error) {
	ch, ok := payment.Get(selection.Channel)
	if !ok {
		return payment.CreateResult{}, "", errors.New("该支付方式未开启")
	}
	if !isPluginEnabled(db, ch.PluginID()) || !ch.Available(db) {
		return payment.CreateResult{}, "", errors.New("该支付方式未开启")
	}
	if !payment.SupportsPayType(ch, selection.PayType) {
		return payment.CreateResult{}, "", errors.New("该支付方式未开启")
	}
	notifyURL := buildRequestURL(c, "/api/payment/"+ch.ID()+"/notify")
	frontendReturnURL := buildFrontendReturnURL(c, orderNo, returnPath)
	result, err := ch.CreatePayment(db, payment.CreateRequest{
		OrderNo:     orderNo,
		AmountCents: amountCents,
		Subject:     subject,
		PayType:     selection.PayType,
		NotifyURL:   notifyURL,
		ReturnURL:   frontendReturnURL,
		ClientIP:    c.ClientIP(),
	})
	return result, frontendReturnURL, err
}

func collectNotifyValues(c *gin.Context) map[string]string {
	params := map[string]string{}
	if c.Request.Method == http.MethodPost {
		_ = c.Request.ParseForm()
	}
	for key, values := range c.Request.Form {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}
	for key, values := range c.Request.URL.Query() {
		if _, exists := params[key]; exists {
			continue
		}
		if len(values) > 0 {
			params[key] = values[0]
		}
	}
	if c.Request.PostForm != nil {
		for key, values := range c.Request.PostForm {
			if len(values) > 0 {
				params[key] = values[0]
			}
		}
	}
	return params
}

func settleRegisteredChannelNotify(db *sql.DB, channelID string, result payment.NotifyResult) error {
	if !result.Success {
		return errors.New("交易未成功")
	}
	if strings.HasPrefix(result.OrderNo, "AU") {
		return settleAgentUpgradeOnlinePayment(db, result.OrderNo, result.AmountCents, channelID, result.PayType, result.GatewayTradeNo, result.RawPayload)
	}
	if err := settleRechargeOrder(db, result.OrderNo, result.AmountCents, result.GatewayTradeNo, result.PayType, result.RawPayload); err == nil {
		return nil
	}
	return settleLicensePurchaseOrder(db, result.OrderNo, result.AmountCents, channelID, result.GatewayTradeNo, result.PayType, result.RawPayload)
}

// PaymentChannelNotify 支付渠道插件异步通知入口。路径 /api/payment/:channel/notify。
func PaymentChannelNotify(c *gin.Context) {
	channelID := strings.TrimSpace(c.Param("channel"))
	ch, ok := payment.Get(channelID)
	if !ok {
		c.String(http.StatusOK, "fail")
		return
	}
	db, err := openSystemConfigDB()
	if err != nil {
		c.String(http.StatusOK, "fail")
		return
	}
	values := collectNotifyValues(c)
	result, err := ch.ParseNotify(db, values)
	if err != nil {
		c.String(http.StatusOK, "fail")
		return
	}
	if err := settleRegisteredChannelNotify(db, ch.ID(), result); err != nil {
		c.String(http.StatusOK, "fail")
		return
	}
	c.String(http.StatusOK, "success")
}
