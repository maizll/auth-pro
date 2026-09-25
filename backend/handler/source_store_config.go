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
	storeConfigGroup         = "store"
	storeConfigProductAppKey = "store_product_app_key"
	storeConfigFreePlanID    = "store_free_plan_id"
)

var storeProductAppKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

type sourceStoreSettings struct {
	ProductAppKey string `json:"productAppKey"`
	FreePlanID    string `json:"freePlanId"`
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
	return sourceStoreSettings{ProductAppKey: key, FreePlanID: plan}, nil
}

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
		}
	}
	return settings, rows.Err()
}

func saveSourceStoreSettings(db *sql.DB, settings sourceStoreSettings) error {
	_, err := db.Exec(`INSERT INTO system_configs (`+"`group`"+`, `+"`key`"+`, value, description) VALUES
		(?, ?, ?, '付费目录所属产品应用标识'),
		(?, ?, ?, '免费套餐 ID')
		ON DUPLICATE KEY UPDATE value=VALUES(value)`,
		storeConfigGroup, storeConfigProductAppKey, settings.ProductAppKey,
		storeConfigGroup, storeConfigFreePlanID, settings.FreePlanID)
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
