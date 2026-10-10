package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

// 广告位（设计 §3）。popup 已停用，不再进入可售/可展白名单。
// home-banner：公开页；sidebar：旧侧栏别名，与 console-sidebar 互通。
const (
	adSlotHomeBanner     = "home-banner"
	adSlotSidebarLegacy  = "sidebar"
	adSlotConsoleHome    = "console-home"
	adSlotConsoleSidebar = "console-sidebar"
	adSlotConsoleLogin   = "console-login"
	adSlotConsoleTopbar  = "console-topbar"
	adSlotConsoleRail    = "console-rail"
	adSlotStoreNative    = "store-native"
	adSlotUpdateDone     = "update-done"
	adSlotListFooter     = "list-footer"
	adSlotProfileSide    = "profile-side"
	adSlotDocsSide       = "docs-side"
	adSlotLockScreen     = "lock-screen"
)

type adSlotDef struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	PriceCents  int64  `json:"priceCents"`
	Capacity    int    `json:"capacity"`
	Sellable    bool   `json:"sellable"`
	Priority    int    `json:"priority"` // 同屏优先级，越小越优先
}

// 可展示/可售槽位（含公开页 home-banner 与侧栏别名）。默认价/名额来自设计示意。
var adSlotCatalog = []adSlotDef{
	{ID: adSlotConsoleHome, Name: "工作台卡片", Description: "统计与趋势之间", PriceCents: 3000, Capacity: 3, Sellable: true, Priority: 1},
	{ID: adSlotConsoleSidebar, Name: "侧栏底部", Description: "菜单底小卡", PriceCents: 2000, Capacity: 3, Sellable: true, Priority: 2},
	{ID: adSlotSidebarLegacy, Name: "侧栏（兼容）", Description: "旧 sidebar 别名", PriceCents: 2000, Capacity: 3, Sellable: false, Priority: 2},
	{ID: adSlotConsoleLogin, Name: "登录页侧边", Description: "≥1200 宽左半", PriceCents: 1500, Capacity: 3, Sellable: true, Priority: 3},
	{ID: adSlotConsoleTopbar, Name: "顶栏文字链", Description: "≥1280 顶栏", PriceCents: 1200, Capacity: 3, Sellable: true, Priority: 4},
	{ID: adSlotConsoleRail, Name: "工作台右栏", Description: "≥1600 右栏下", PriceCents: 1800, Capacity: 3, Sellable: true, Priority: 5},
	{ID: adSlotStoreNative, Name: "商店推荐卡", Description: "应用商店第 3 卡", PriceCents: 1500, Capacity: 2, Sellable: true, Priority: 6},
	{ID: adSlotUpdateDone, Name: "更新成功页", Description: "更新成功结果底", PriceCents: 600, Capacity: 1, Sellable: true, Priority: 7},
	{ID: adSlotListFooter, Name: "列表底部横条", Description: "白名单列表分页下", PriceCents: 1000, Capacity: 3, Sellable: true, Priority: 8},
	{ID: adSlotProfileSide, Name: "个人中心侧栏", Description: "个人中心左栏下", PriceCents: 500, Capacity: 2, Sellable: true, Priority: 9},
	{ID: adSlotDocsSide, Name: "文档侧边", Description: "帮助文档目录下", PriceCents: 800, Capacity: 2, Sellable: true, Priority: 10},
	{ID: adSlotLockScreen, Name: "锁屏", Description: "锁屏左下", PriceCents: 400, Capacity: 2, Sellable: true, Priority: 11},
	{ID: adSlotHomeBanner, Name: "公开页横幅", Description: "登录/公开站横幅", PriceCents: 0, Capacity: 3, Sellable: false, Priority: 12},
}

var (
	adSlotByID     map[string]adSlotDef
	adSlotEnabled  = map[string]bool{} // 官网远程开关，默认 true
	adSlotEnabledL sync.RWMutex
)

func init() {
	adSlotByID = make(map[string]adSlotDef, len(adSlotCatalog))
	for _, slot := range adSlotCatalog {
		adSlotByID[slot.ID] = slot
		adSlotEnabled[slot.ID] = true
	}
	// 重建 advertisementPositions / locks：去掉 popup，纳入全部可展位。
	advertisementPositions = make([]string, 0, len(adSlotCatalog))
	for _, slot := range adSlotCatalog {
		advertisementPositions = append(advertisementPositions, slot.ID)
	}
	advertisementLocks = map[string]*sync.Mutex{}
	for _, position := range advertisementPositions {
		advertisementLocks[position] = &sync.Mutex{}
	}
}

func canonicalizeAdSlot(id string) string {
	id = strings.TrimSpace(id)
	switch id {
	case adSlotSidebarLegacy:
		return adSlotConsoleSidebar
	case "popup":
		return "" // 停用
	default:
		return id
	}
}

func adSlotDefOf(id string) (adSlotDef, bool) {
	id = canonicalizeAdSlot(id)
	if id == "" {
		return adSlotDef{}, false
	}
	// 兼容查询 sidebar 时仍能解析到侧栏位定义
	if def, ok := adSlotByID[id]; ok {
		return def, true
	}
	if id == adSlotConsoleSidebar {
		return adSlotByID[adSlotConsoleSidebar], true
	}
	return adSlotDef{}, false
}

func isAdSlotEnabled(id string) bool {
	id = canonicalizeAdSlot(id)
	if id == "" {
		return false
	}
	adSlotEnabledL.RLock()
	defer adSlotEnabledL.RUnlock()
	enabled, ok := adSlotEnabled[id]
	if !ok {
		return true
	}
	return enabled
}

func setAdSlotEnabled(id string, enabled bool) {
	id = canonicalizeAdSlot(id)
	if id == "" {
		return
	}
	adSlotEnabledL.Lock()
	adSlotEnabled[id] = enabled
	adSlotEnabledL.Unlock()
}

const adSlotSettingKey = "ad_slot_switches"

func loadAdSlotSwitchesFromStore() {
	db, err := config.DB()
	if err != nil {
		return
	}
	var raw string
	err = db.QueryRow(`SELECT setting_value FROM source_station_settings WHERE setting_key=?`, adSlotSettingKey).Scan(&raw)
	if err != nil || strings.TrimSpace(raw) == "" {
		return
	}
	var switches map[string]bool
	if err := json.Unmarshal([]byte(raw), &switches); err != nil {
		return
	}
	adSlotEnabledL.Lock()
	defer adSlotEnabledL.Unlock()
	for id, enabled := range switches {
		canon := canonicalizeAdSlot(id)
		if canon == "" {
			continue
		}
		adSlotEnabled[canon] = enabled
		if id == adSlotSidebarLegacy {
			adSlotEnabled[adSlotSidebarLegacy] = enabled
		}
	}
}

func persistAdSlotSwitches() error {
	db, err := config.DB()
	if err != nil {
		return err
	}
	if err := ensureSourceStationStorage(db); err != nil {
		return err
	}
	adSlotEnabledL.RLock()
	payload := make(map[string]bool, len(adSlotEnabled))
	for id, enabled := range adSlotEnabled {
		payload[id] = enabled
	}
	adSlotEnabledL.RUnlock()
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO source_station_settings (setting_key, setting_value) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE setting_value=VALUES(setting_value)`, adSlotSettingKey, string(raw))
	return err
}

// AdminAdSlotOverview 官网广告位总览：远程开/关。
func AdminAdSlotOverview(c *gin.Context) {
	loadAdSlotSwitchesFromStore()
	list := make([]gin.H, 0, len(adSlotCatalog))
	for _, slot := range adSlotCatalog {
		if slot.ID == adSlotSidebarLegacy {
			continue // 总览只展示规范位
		}
		list = append(list, gin.H{
			"id":          slot.ID,
			"name":        slot.Name,
			"description": slot.Description,
			"priceCents":  slot.PriceCents,
			"capacity":    slot.Capacity,
			"sellable":    slot.Sellable,
			"enabled":     isAdSlotEnabled(slot.ID),
			"priority":    slot.Priority,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": list, "total": len(list)}})
}

type adSlotSwitchRequest struct {
	Enabled *bool `json:"enabled"`
}

// AdminAdSlotSwitch 官网远程开关单个广告位。
func AdminAdSlotSwitch(c *gin.Context) {
	id := canonicalizeAdSlot(c.Param("id"))
	if _, ok := adSlotDefOf(id); !ok {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "广告位不存在"})
		return
	}
	var req adSlotSwitchRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	loadAdSlotSwitchesFromStore()
	setAdSlotEnabled(id, *req.Enabled)
	if id == adSlotConsoleSidebar {
		setAdSlotEnabled(adSlotSidebarLegacy, *req.Enabled)
	}
	if err := persistAdSlotSwitches(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存失败"})
		return
	}
	invalidateAllAdvertisementCaches()
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已更新", "data": gin.H{"id": id, "enabled": *req.Enabled}})
}

// ClientAdSlotCatalog 客户站购买页：可售位 + 当日余量。
func ClientAdSlotCatalog(c *gin.Context) {
	loadAdSlotSwitchesFromStore()
	list := make([]gin.H, 0, len(adSlotCatalog))
	for _, slot := range adSlotCatalog {
		if !slot.Sellable || slot.ID == adSlotSidebarLegacy {
			continue
		}
		enabled := isAdSlotEnabled(slot.ID)
		left := 0
		if enabled {
			left = slot.Capacity // 细余量由日历接口给出；此处给满额提示
		}
		list = append(list, gin.H{
			"id":          slot.ID,
			"name":        slot.Name,
			"description": slot.Description,
			"priceCents":  slot.PriceCents,
			"capacity":    slot.Capacity,
			"enabled":     enabled,
			"leftHint":    left,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"list":            list,
		"noRefundNotice":  adNoRefundNotice,
		"maxOnScreen":     2,
		"currency":        "CNY",
	}})
}

const adNoRefundNotice = "虚拟产品，付款后不退款。审核不过请修改后重新提交；未展示天数顺延；违规下线剩余天数作废。"

func invalidateAllAdvertisementCaches() {
	advertisementCacheL.Lock()
	defer advertisementCacheL.Unlock()
	advertisementCache = map[string]advertisementCacheEntry{}
}

// adsHiddenFromSettings：商业版隐藏广告开关。仅当开关开且快照为商业版时生效。
func consoleAdsHiddenEnabled(db *sql.DB) bool {
	if db == nil {
		var err error
		db, err = openSystemConfigDB()
		if err != nil {
			return false
		}
	}
	var value string
	err := db.QueryRow(`SELECT value FROM system_configs WHERE `+"`group`"+` = 'ads' AND `+"`key`"+` = 'console_ads_hidden'`).Scan(&value)
	if err != nil {
		return false
	}
	return value == "1" || strings.EqualFold(value, "true")
}

func setConsoleAdsHidden(db *sql.DB, hidden bool) error {
	value := "0"
	if hidden {
		value = "1"
	}
	_, err := db.Exec(`INSERT INTO system_configs (`+"`group`"+`, `+"`key`"+`, value, description) VALUES ('ads', 'console_ads_hidden', ?, '商业版隐藏全部广告位')
		ON DUPLICATE KEY UPDATE value = VALUES(value)`, value)
	return err
}

// AdminAdsHideGet / AdminAdsHideSet：系统设置 › 广告 › 隐藏广告。
func AdminAdsHideGet(c *gin.Context) {
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库不可用"})
		return
	}
	defer db.Close()
	hidden := consoleAdsHiddenEnabled(db)
	commercial := siteLooksCommercial(db)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"hidden":     hidden && commercial,
		"switchOn":   hidden,
		"commercial": commercial,
		"effective":  hidden && commercial,
	}})
}

type adsHideRequest struct {
	Hidden *bool `json:"hidden"`
}

func AdminAdsHideSet(c *gin.Context) {
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库不可用"})
		return
	}
	defer db.Close()
	if !siteLooksCommercial(db) {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "免费版不可隐藏广告，请升级商业版"})
		return
	}
	var req adsHideRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Hidden == nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if err := setConsoleAdsHidden(db, *req.Hidden); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存失败"})
		return
	}
	invalidateAllAdvertisementCaches()
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已保存", "data": gin.H{"hidden": *req.Hidden, "effective": *req.Hidden}})
}

// siteLooksCommercial：至少有一项有效商业版授权/站点快照视为商业版（含待校验）。
func siteLooksCommercial(db *sql.DB) bool {
	if db == nil {
		return false
	}
	var n int
	// 商业版窗口：apps 或 license 带 commercial 标记；找不到表/列则保守 false。
	_ = db.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'commercial_editions'`).Scan(&n)
	if n > 0 {
		var active int
		err := db.QueryRow(`SELECT COUNT(*) FROM commercial_editions WHERE status IN ('active','pending') AND (expires_at IS NULL OR expires_at > NOW())`).Scan(&active)
		if err == nil && active > 0 {
			return true
		}
	}
	// 回退：system_configs 里 commercial_edition=1
	var v string
	if err := db.QueryRow(`SELECT value FROM system_configs WHERE `+"`group`"+` = 'site' AND `+"`key`"+` = 'commercial_edition'`).Scan(&v); err == nil {
		return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "commercial")
	}
	return false
}

func ensureAdSlotDaySchema(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS source_ad_slot_days (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
		slot_id VARCHAR(40) NOT NULL,
		day_date DATE NOT NULL,
		reserved INT NOT NULL DEFAULT 0,
		locked INT NOT NULL DEFAULT 0,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		UNIQUE KEY uk_ad_slot_day (slot_id, day_date)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='广告位按日名额'`)
	return err
}

func adSlotRemaining(db *sql.DB, slotID string, day time.Time) (int, error) {
	def, ok := adSlotDefOf(slotID)
	if !ok || !def.Sellable {
		return 0, errors.New("广告位不可售")
	}
	if !isAdSlotEnabled(slotID) {
		return 0, nil
	}
	if err := ensureAdSlotDaySchema(db); err != nil {
		return 0, err
	}
	var reserved, locked int
	err := db.QueryRow(`SELECT reserved, locked FROM source_ad_slot_days WHERE slot_id = ? AND day_date = ?`,
		canonicalizeAdSlot(slotID), day.Format("2006-01-02")).Scan(&reserved, &locked)
	if err == sql.ErrNoRows {
		return def.Capacity, nil
	}
	if err != nil {
		return 0, err
	}
	left := def.Capacity - reserved - locked
	if left < 0 {
		left = 0
	}
	return left, nil
}
