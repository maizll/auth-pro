package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

const (
	storeConfigGroup            = "store"
	storeConfigProductAppKey    = "store_product_app_key"
	storeConfigFreePlanID       = "store_free_plan_id"
	storeConfigGraceDays        = "store_grace_days"
	storeConfigRevokeOnPassword = "store_revoke_on_password_change"
	storeConfigFeatures         = "store_commercial_features"
	storeConfigSourceBase       = "store_source_base"
	storeConfigTrustProxy       = "store_trust_proxy"
	storeConfigSiteURL          = "store_site_url"
	storeConfigBindingID        = "store_binding_id"
	storeConfigAccountLabel     = "store_account_label"
	storeConfigAccountRole      = "store_account_role"
	storeConfigLicenseNo        = "store_license_no"
	storeConfigInstallID        = "store_install_id"
)

var storeProductAppKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

type sourceStoreSettings struct {
	ProductAppKey          string   `json:"productAppKey"`
	FreePlanID             string   `json:"freePlanId"`
	GraceDays              int      `json:"graceDays"`
	RevokeOnPasswordChange *bool    `json:"revokeOnPasswordChange"`
	CommercialFeatures     []string `json:"commercialFeatures"`
}

func normalizeStoreSettings(in sourceStoreSettings) (sourceStoreSettings, error) {
	key := strings.TrimSpace(in.ProductAppKey)
	plan := strings.TrimSpace(in.FreePlanID)
	if key != "" && !storeProductAppKeyPattern.MatchString(key) {
		return sourceStoreSettings{}, errors.New("产品应用标识不合法")
	}
	if plan != "" {
		n, err := strconv.ParseInt(plan, 10, 64)
		if err != nil || n <= 0 {
			return sourceStoreSettings{}, errors.New("免费套餐 ID 须为正整数")
		}
		plan = strconv.FormatInt(n, 10)
	}
	days := in.GraceDays
	if days == 0 {
		days = storeGraceDefaultDays
	}
	if days < 1 || days > 30 {
		return sourceStoreSettings{}, errors.New("离线宽限天数须在 1 到 30 之间")
	}
	revoke := true
	if in.RevokeOnPasswordChange != nil {
		revoke = *in.RevokeOnPasswordChange
	}
	features := in.CommercialFeatures
	if features == nil {
		features = []string{storeFeatureMultiApp}
	}
	cleaned := make([]string, 0, len(features))
	seen := map[string]bool{}
	for _, feature := range features {
		feature = strings.TrimSpace(feature)
		if feature == "" || seen[feature] || !storeFeatureKeyPattern.MatchString(feature) {
			if feature != "" && !storeFeatureKeyPattern.MatchString(feature) {
				return sourceStoreSettings{}, errors.New("功能键不合法")
			}
			continue
		}
		seen[feature] = true
		cleaned = append(cleaned, feature)
	}
	revokeCopy := revoke
	return sourceStoreSettings{
		ProductAppKey: key, FreePlanID: plan, GraceDays: days,
		RevokeOnPasswordChange: &revokeCopy, CommercialFeatures: cleaned,
	}, nil
}

var storeFeatureKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

func loadSourceStoreSettings(db *sql.DB) (sourceStoreSettings, error) {
	rows, err := db.Query("SELECT `key`, value FROM system_configs WHERE `group`=?", storeConfigGroup)
	if err != nil {
		return sourceStoreSettings{}, err
	}
	defer rows.Close()
	var settings sourceStoreSettings
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return sourceStoreSettings{}, err
		}
		switch key {
		case storeConfigProductAppKey:
			settings.ProductAppKey = strings.TrimSpace(value)
		case storeConfigFreePlanID:
			settings.FreePlanID = strings.TrimSpace(value)
		case storeConfigGraceDays:
			if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				settings.GraceDays = n
			}
		case storeConfigRevokeOnPassword:
			flag := value != "0"
			settings.RevokeOnPasswordChange = &flag
		case storeConfigFeatures:
			settings.CommercialFeatures = splitStoreFeatures(value)
		}
	}
	return settings, rows.Err()
}

func splitStoreFeatures(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func saveSourceStoreSettings(db *sql.DB, settings sourceStoreSettings) error {
	revoke := "1"
	if settings.RevokeOnPasswordChange != nil && !*settings.RevokeOnPasswordChange {
		revoke = "0"
	}
	features := strings.Join(settings.CommercialFeatures, ",")
	_, err := db.Exec(`INSERT INTO system_configs (`+"`group`"+`, `+"`key`"+`, value, description) VALUES
		(?, ?, ?, '付费目录所属产品应用标识'),
		(?, ?, ?, '免费套餐 ID'),
		(?, ?, ?, '离线宽限天数'),
		(?, ?, ?, '改密码是否撤销站点绑定'),
		(?, ?, ?, '商业版功能键')
		ON DUPLICATE KEY UPDATE value=VALUES(value)`,
		storeConfigGroup, storeConfigProductAppKey, settings.ProductAppKey,
		storeConfigGroup, storeConfigFreePlanID, settings.FreePlanID,
		storeConfigGroup, storeConfigGraceDays, strconv.Itoa(settings.GraceDays),
		storeConfigGroup, storeConfigRevokeOnPassword, revoke,
		storeConfigGroup, storeConfigFeatures, features)
	return err
}

func AdminSourceStoreSettings(c *gin.Context) {
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取商店设置失败"})
		return
	}
	if err := ensureSystemConfigStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取商店设置失败"})
		return
	}
	settings, err := loadSourceStoreSettings(db)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取商店设置失败"})
		return
	}
	settings, err = normalizeStoreSettings(settings)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": settings})
}

func AdminSourceStoreSettingsSave(c *gin.Context) {
	var incoming sourceStoreSettings
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	settings, err := normalizeStoreSettings(incoming)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存商店设置失败"})
		return
	}
	if err := ensureSystemConfigStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存商店设置失败"})
		return
	}
	if err := saveSourceStoreSettings(db, settings); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存商店设置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已保存商店设置", "data": settings})
}
