package handler

// 极验行为验证 4.0 服务端二次校验
// 前端流程：gt4.js initGeetest4(captchaId) -> onSuccess 取得
// lot_number / captcha_output / pass_token / gen_time 四要素随登录请求提交；
// 服务端使用 sign_token = HMAC-SHA256(lot_number, captchaKey) 调用极验校验接口确认结果。
// 参考文档：https://docs.geetest.com/gt4/apirefer/api/web

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	geetestValidateURL = "https://gcaptcha4.geetest.com/validate"
	geetestHTTPTimeout = 5 * time.Second
)

// geetestConfig 系统配置中的极验凭证（system_configs 表 captcha 分组）
type geetestConfig struct {
	Enabled    bool
	CaptchaID  string
	CaptchaKey string
}

func loadGeetestConfig(db *sql.DB) geetestConfig {
	cfg := geetestConfig{}
	if db == nil {
		return cfg
	}
	rows, err := db.Query("SELECT `key`, value FROM system_configs WHERE `group` = 'captcha'")
	if err != nil {
		return cfg
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		switch key {
		case "geetest_enabled":
			cfg.Enabled = value == "1"
		case "geetest_captcha_id":
			cfg.CaptchaID = strings.TrimSpace(value)
		case "geetest_captcha_key":
			cfg.CaptchaKey = strings.TrimSpace(value)
		}
	}
	return cfg
}

// geetestValidateParams 登录请求中携带的极验前端验证结果
type geetestValidateParams struct {
	LotNumber     string `json:"lot_number"`
	CaptchaOutput string `json:"captcha_output"`
	PassToken     string `json:"pass_token"`
	GenTime       string `json:"gen_time"`
}

func geetestSignToken(lotNumber, captchaKey string) string {
	mac := hmac.New(sha256.New, []byte(captchaKey))
	mac.Write([]byte(lotNumber))
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyGeetestLogin 在启用极验时对登录请求做服务端二次校验。
// 返回 false 表示应拦截登录，msg 为面向用户的提示文案。
// 极验服务不可达时按其宕机策略放行并记录日志，避免验证服务故障拖垮登录。
func verifyGeetestLogin(db *sql.DB, p geetestValidateParams) (bool, string) {
	cfg := loadGeetestConfig(db)
	if !cfg.Enabled {
		return true, ""
	}
	if cfg.CaptchaID == "" || cfg.CaptchaKey == "" {
		return false, "行为验证未正确配置，请联系管理员"
	}
	if strings.TrimSpace(p.LotNumber) == "" || strings.TrimSpace(p.CaptchaOutput) == "" ||
		strings.TrimSpace(p.PassToken) == "" || strings.TrimSpace(p.GenTime) == "" {
		return false, "请先完成行为验证"
	}

	form := url.Values{}
	form.Set("lot_number", p.LotNumber)
	form.Set("captcha_output", p.CaptchaOutput)
	form.Set("pass_token", p.PassToken)
	form.Set("gen_time", p.GenTime)
	form.Set("sign_token", geetestSignToken(p.LotNumber, cfg.CaptchaKey))

	endpoint := geetestValidateURL + "?captcha_id=" + url.QueryEscape(cfg.CaptchaID)
	client := &http.Client{Timeout: geetestHTTPTimeout}
	resp, err := client.PostForm(endpoint, form)
	if err != nil {
		log.Printf("[geetest] validate request failed, bypass: %v", err)
		return true, ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		log.Printf("[geetest] read validate response failed, bypass: %v", err)
		return true, ""
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("[geetest] validate returned status %d, bypass", resp.StatusCode)
		return true, ""
	}

	var result struct {
		Result string `json:"result"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("[geetest] invalid validate response, bypass: %v", err)
		return true, ""
	}
	if result.Result == "success" {
		return true, ""
	}
	log.Printf("[geetest] validate rejected, reason: %s", result.Reason)
	return false, "行为验证未通过，请重新完成验证"
}
