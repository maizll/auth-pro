package handler

import (
	"net/http"
	"strings"

	"auto_pro/payment/alipayf2f"

	"github.com/gin-gonic/gin"
)

type updateAlipayF2FConfigRequest struct {
	AppID            string `json:"appId"`
	PrivateKey       string `json:"privateKey"`
	AlipayPublicKey  string `json:"alipayPublicKey"`
	Gateway          string `json:"gateway"`
	NotifyURL        string `json:"notifyUrl"`
	Sandbox          bool   `json:"sandbox"`
	CertMode         bool   `json:"certMode"`
	AppCertSN        string `json:"appCertSn"`
	AlipayRootCertSN string `json:"alipayRootCertSn"`
}

func alipayF2FConfigView(cfg alipayf2f.Config) alipayf2f.PublicView {
	view := cfg.Public()
	if view.Gateway == "" {
		if cfg.Sandbox {
			view.Gateway = alipayf2f.SandboxGateway
		} else {
			view.Gateway = alipayf2f.ProductionGateway
		}
	}
	return view
}

func AdminAlipayF2FConfig(c *gin.Context) {
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensureSystemConfigStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化系统配置失败"})
		return
	}
	cfg, err := alipayf2f.LoadConfig(db)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取支付宝当面付配置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": alipayF2FConfigView(cfg)})
}

func AdminAlipayF2FConfigUpdate(c *gin.Context) {
	var req updateAlipayF2FConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	req.AppID = strings.TrimSpace(req.AppID)
	req.PrivateKey = strings.TrimSpace(req.PrivateKey)
	req.AlipayPublicKey = strings.TrimSpace(req.AlipayPublicKey)
	req.Gateway = strings.TrimSpace(req.Gateway)
	req.NotifyURL = strings.TrimSpace(req.NotifyURL)
	req.AppCertSN = strings.TrimSpace(req.AppCertSN)
	req.AlipayRootCertSN = strings.TrimSpace(req.AlipayRootCertSN)

	if req.Gateway != "" {
		if err := validateOptionalHTTPURL(req.Gateway, "支付宝网关地址"); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
			return
		}
	}
	if err := validateOptionalHTTPURL(req.NotifyURL, "支付宝异步通知地址"); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	if req.CertMode && (req.AppCertSN == "" || req.AlipayRootCertSN == "") {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "证书模式请填写应用公钥证书 SN 与支付宝根证书 SN"})
		return
	}

	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensureSystemConfigStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化系统配置失败"})
		return
	}
	existing, err := alipayf2f.LoadConfig(db)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取支付宝当面付配置失败"})
		return
	}
	if req.PrivateKey == "" {
		req.PrivateKey = existing.PrivateKey
	}
	if req.AppID == "" || req.PrivateKey == "" || req.AlipayPublicKey == "" {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请填写 APPID、应用私钥和支付宝公钥"})
		return
	}

	tx, err := db.Begin()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存支付宝当面付配置失败"})
		return
	}
	defer tx.Rollback()

	items := []struct {
		key         string
		value       string
		description string
	}{
		{key: "alipay_f2f_app_id", value: req.AppID, description: "支付宝当面付 APPID"},
		{key: "alipay_f2f_private_key", value: req.PrivateKey, description: "支付宝当面付应用私钥"},
		{key: "alipay_f2f_alipay_public_key", value: req.AlipayPublicKey, description: "支付宝当面付支付宝公钥"},
		{key: "alipay_f2f_gateway", value: req.Gateway, description: "支付宝当面付网关地址"},
		{key: "alipay_f2f_notify_url", value: req.NotifyURL, description: "支付宝当面付异步通知地址"},
		{key: "alipay_f2f_sandbox", value: boolConfigValue(req.Sandbox), description: "支付宝当面付沙箱开关"},
		{key: "alipay_f2f_cert_mode", value: boolConfigValue(req.CertMode), description: "支付宝当面付证书模式"},
		{key: "alipay_f2f_app_cert_sn", value: req.AppCertSN, description: "支付宝当面付应用公钥证书 SN"},
		{key: "alipay_f2f_alipay_root_cert_sn", value: req.AlipayRootCertSN, description: "支付宝当面付根证书 SN"},
	}
	for _, item := range items {
		if _, err := tx.Exec(`
			INSERT INTO system_configs (`+"`group`"+`, `+"`key`"+`, value, description)
			VALUES ('payment', ?, ?, ?)
			ON DUPLICATE KEY UPDATE value = VALUES(value), description = VALUES(description)
		`, item.key, item.value, item.description); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存支付宝当面付配置失败"})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存支付宝当面付配置失败"})
		return
	}
	cfg, err := alipayf2f.LoadConfig(db)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取支付宝当面付配置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "支付宝当面付配置保存成功", "data": alipayF2FConfigView(cfg)})
}
