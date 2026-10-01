package handler

import (
	"database/sql"
	"strings"
	"sync/atomic"

	"auto_pro/config"
)

// hotPathSchemaReady 为 true 后，请求路径不再做建表、补列、清数据和菜单回填。
// 只在进程启动把这些步骤跑完之后置位。测试不置位，仍会执行真实 DDL。
var hotPathSchemaReady atomic.Bool

func hotPathSchemaSkipped() bool {
	return hotPathSchemaReady.Load()
}

// WarmHotPathSchema 在开始监听之前准备菜单、首页模板和登录相关结构。
// 升级后的第一次启动会按当前版本补一次菜单；同一版本再次启动不再重复。
// 菜单管理里改过的标题、图标和排序会保留。
func WarmHotPathSchema(db *sql.DB) error {
	if db == nil {
		return nil
	}
	if err := prepareCommercialProduct(db); err != nil {
		return err
	}
	if err := ensureUserAuthStorage(db); err != nil {
		return err
	}
	if err := ensureAppLicenseRequiredColumn(db); err != nil {
		return err
	}
	if err := ensurePluginStorage(db); err != nil {
		return err
	}
	if err := warmProductMenus(db); err != nil {
		return err
	}
	hotPathSchemaReady.Store(true)
	return nil
}

func warmProductMenus(db *sql.DB) error {
	name := "product_menus_" + strings.ReplaceAll(config.AppVersion, ".", "_")
	pending, err := sourceStationMigrationPending(db, name)
	if err != nil || pending {
		ensureProductMenus(db)
	}
	if err != nil || !pending {
		return err
	}
	return markSourceStationMigration(db, name)
}
