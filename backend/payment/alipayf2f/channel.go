package alipayf2f

import (
	"database/sql"

	"auto_pro/payment"
)

func init() {
	payment.Register(Channel{})
}

// Channel 是支付宝当面付（正扫）官方支付渠道。
type Channel struct{}

func (Channel) ID() string { return ChannelID }

func (Channel) PluginID() string { return PluginID }

func (Channel) Options() []payment.Option {
	return []payment.Option{{
		Code:    ChannelID + ":" + PayTypeAlipay,
		Channel: ChannelID,
		PayType: PayTypeAlipay,
		Label:   "支付宝当面付",
		Icon:    "ri:alipay-fill",
		Color:   "#1677ff",
	}}
}

func (Channel) Available(db *sql.DB) bool {
	cfg, err := LoadConfig(db)
	return err == nil && cfg.ready()
}

func (Channel) CreatePayment(db *sql.DB, req payment.CreateRequest) (payment.CreateResult, error) {
	cfg, err := LoadConfig(db)
	if err != nil {
		return payment.CreateResult{}, err
	}
	return Precreate(cfg, req, httpClient)
}

func (Channel) ParseNotify(db *sql.DB, values map[string]string) (payment.NotifyResult, error) {
	cfg, err := LoadConfig(db)
	if err != nil {
		return payment.NotifyResult{}, err
	}
	return ParseNotify(cfg, values)
}
