package alipayf2f

import (
	"database/sql"
	"strings"
)

const (
	ChannelID         = "alipay-f2f"
	PluginID          = "alipay-f2f"
	PayTypeAlipay     = "alipay"
	ProductionGateway = "https://openapi.alipay.com/gateway.do"
	SandboxGateway    = "https://openapi-sandbox.dl.alipaydev.com/gateway.do"
	configGroup       = "payment"
)

// Config 存放当面付凭证。PrivateKey 只在服务端使用，禁止写入日志或 API 响应。
type Config struct {
	AppID            string
	PrivateKey       string
	AlipayPublicKey  string
	Gateway          string
	NotifyURL        string
	Sandbox          bool
	CertMode         bool
	AppCertSN        string
	AlipayRootCertSN string
}

// PublicView 是管理端可读的配置视图，不含私钥明文。
type PublicView struct {
	AppID            string `json:"appId"`
	PrivateKeySet    bool   `json:"privateKeySet"`
	AlipayPublicKey  string `json:"alipayPublicKey"`
	Gateway          string `json:"gateway"`
	NotifyURL        string `json:"notifyUrl"`
	Sandbox          bool   `json:"sandbox"`
	CertMode         bool   `json:"certMode"`
	AppCertSN        string `json:"appCertSn"`
	AlipayRootCertSN string `json:"alipayRootCertSn"`
}

func (cfg Config) Public() PublicView {
	return PublicView{
		AppID:            cfg.AppID,
		PrivateKeySet:    strings.TrimSpace(cfg.PrivateKey) != "",
		AlipayPublicKey:  cfg.AlipayPublicKey,
		Gateway:          cfg.Gateway,
		NotifyURL:        cfg.NotifyURL,
		Sandbox:          cfg.Sandbox,
		CertMode:         cfg.CertMode,
		AppCertSN:        cfg.AppCertSN,
		AlipayRootCertSN: cfg.AlipayRootCertSN,
	}
}

func (cfg Config) gatewayURL() string {
	if gw := strings.TrimSpace(cfg.Gateway); gw != "" {
		return gw
	}
	if cfg.Sandbox {
		return SandboxGateway
	}
	return ProductionGateway
}

func (cfg Config) ready() bool {
	if strings.TrimSpace(cfg.AppID) == "" || strings.TrimSpace(cfg.PrivateKey) == "" || strings.TrimSpace(cfg.AlipayPublicKey) == "" {
		return false
	}
	if cfg.CertMode && (strings.TrimSpace(cfg.AppCertSN) == "" || strings.TrimSpace(cfg.AlipayRootCertSN) == "") {
		return false
	}
	return true
}

// LoadConfig 从 system_configs.payment 组读取当面付配置。
func LoadConfig(db *sql.DB) (Config, error) {
	cfg := Config{}
	if db == nil {
		return cfg, sql.ErrConnDone
	}
	rows, err := db.Query("SELECT `key`, value FROM system_configs WHERE `group` = ?", configGroup)
	if err != nil {
		return cfg, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return cfg, err
		}
		value = strings.TrimSpace(value)
		switch key {
		case "alipay_f2f_app_id":
			cfg.AppID = value
		case "alipay_f2f_private_key":
			cfg.PrivateKey = value
		case "alipay_f2f_alipay_public_key":
			cfg.AlipayPublicKey = value
		case "alipay_f2f_gateway":
			cfg.Gateway = value
		case "alipay_f2f_notify_url":
			cfg.NotifyURL = value
		case "alipay_f2f_sandbox":
			cfg.Sandbox = value == "1"
		case "alipay_f2f_cert_mode":
			cfg.CertMode = value == "1"
		case "alipay_f2f_app_cert_sn":
			cfg.AppCertSN = value
		case "alipay_f2f_alipay_root_cert_sn":
			cfg.AlipayRootCertSN = value
		}
	}
	return cfg, rows.Err()
}
