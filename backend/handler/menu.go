package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

type menuRow struct {
	ID         int64
	ParentID   int64
	Name       string
	Path       string
	Component  string
	Redirect   string
	Title      string
	Icon       string
	Sort       int
	IsHide     bool
	IsHideTab  bool
	IsFullPage bool
	KeepAlive  bool
	FixedTab   bool
	Roles      []string
}

type menuResponse struct {
	Name      string          `json:"name"`
	Path      string          `json:"path"`
	Component string          `json:"component,omitempty"`
	Redirect  string          `json:"redirect,omitempty"`
	Meta      menuMeta        `json:"meta"`
	Children  []*menuResponse `json:"children,omitempty"`
}

type menuMeta struct {
	Title      string   `json:"title"`
	Icon       string   `json:"icon,omitempty"`
	IsHide     bool     `json:"isHide,omitempty"`
	IsHideTab  bool     `json:"isHideTab,omitempty"`
	IsFullPage bool     `json:"isFullPage,omitempty"`
	KeepAlive  bool     `json:"keepAlive,omitempty"`
	FixedTab   bool     `json:"fixedTab,omitempty"`
	Roles      []string `json:"roles,omitempty"`
}

// GetMenuList 获取当前用户的菜单树
func GetMenuList(c *gin.Context) {
	userID, _ := c.Get("user_id")

	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	ensureProductMenus(db)

	// 查询用户的 role_id
	var roleID sql.NullInt64
	err = db.QueryRow("SELECT role_id FROM admins WHERE id = ?", userID).Scan(&roleID)
	if err != nil || !roleID.Valid {
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": []any{}})
		return
	}

	// 查询该角色关联的菜单
	rows, err := db.Query(`
		SELECT m.id, m.parent_id, m.name, m.path, m.component, m.redirect,
		       m.title, m.icon, m.sort, m.is_hide, m.is_hide_tab, m.is_full_page,
		       m.keep_alive, m.fixed_tab
		FROM menus m
		INNER JOIN role_menus rm ON rm.menu_id = m.id
		WHERE rm.role_id = ? AND m.enabled = 1
		ORDER BY m.sort ASC, m.id ASC
	`, roleID.Int64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询菜单失败"})
		return
	}
	defer rows.Close()

	var allMenus []menuRow
	for rows.Next() {
		var m menuRow
		err := rows.Scan(&m.ID, &m.ParentID, &m.Name, &m.Path, &m.Component, &m.Redirect,
			&m.Title, &m.Icon, &m.Sort, &m.IsHide, &m.IsHideTab, &m.IsFullPage,
			&m.KeepAlive, &m.FixedTab)
		if err != nil {
			continue
		}
		allMenus = append(allMenus, m)
	}

	rolesByName := productMenuRoleIndex()
	for i := range allMenus {
		allMenus[i].Roles = rolesByName[allMenus[i].Name]
	}

	// 组装树形结构
	tree := buildMenuTree(allMenus, 0)

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "",
		"data": tree,
	})
}

func buildMenuTree(menus []menuRow, parentID int64) []*menuResponse {
	var result []*menuResponse
	for _, m := range menus {
		if m.ParentID != parentID {
			continue
		}
		node := &menuResponse{
			Name:      m.Name,
			Path:      m.Path,
			Component: m.Component,
			Redirect:  m.Redirect,
			Meta: menuMeta{
				Title:      resolveMenuTitle(m.Title),
				Icon:       m.Icon,
				IsHide:     m.IsHide,
				IsHideTab:  m.IsHideTab,
				IsFullPage: m.IsFullPage,
				KeepAlive:  m.KeepAlive,
				FixedTab:   m.FixedTab,
				Roles:      m.Roles,
			},
		}
		children := buildMenuTree(menus, m.ID)
		if len(children) > 0 {
			node.Children = children
		}
		result = append(result, node)
	}
	return result
}

// ========== 菜单管理 CRUD ==========

type menuManageItem struct {
	ID         int64             `json:"id"`
	ParentID   int64             `json:"parentId"`
	Name       string            `json:"name"`
	Path       string            `json:"path"`
	Component  string            `json:"component"`
	Redirect   string            `json:"redirect"`
	Title      string            `json:"title"`
	Icon       string            `json:"icon"`
	Sort       int               `json:"sort"`
	IsHide     bool              `json:"isHide"`
	IsHideTab  bool              `json:"isHideTab"`
	IsFullPage bool              `json:"isFullPage"`
	KeepAlive  bool              `json:"keepAlive"`
	FixedTab   bool              `json:"fixedTab"`
	Enabled    bool              `json:"enabled"`
	Children   []*menuManageItem `json:"children,omitempty"`
}

// MenuManageList 菜单管理列表（全量树形）
func MenuManageList(c *gin.Context) {
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	ensureProductMenus(db)

	rows, err := db.Query(`SELECT id, parent_id, name, path, component, redirect, title, icon, sort,
		is_hide, is_hide_tab, is_full_page, keep_alive, fixed_tab, enabled
		FROM menus ORDER BY sort ASC, id ASC`)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询失败"})
		return
	}
	defer rows.Close()

	var all []menuManageItem
	for rows.Next() {
		var m menuManageItem
		rows.Scan(&m.ID, &m.ParentID, &m.Name, &m.Path, &m.Component, &m.Redirect,
			&m.Title, &m.Icon, &m.Sort, &m.IsHide, &m.IsHideTab, &m.IsFullPage,
			&m.KeepAlive, &m.FixedTab, &m.Enabled)
		m.Title = resolveMenuTitle(m.Title)
		if isDemoProductMenu(m.Name) {
			continue
		}
		all = append(all, m)
	}

	tree := buildManageTree(all, 0)

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": tree})
}

func buildManageTree(menus []menuManageItem, parentID int64) []*menuManageItem {
	var result []*menuManageItem
	for i := range menus {
		if menus[i].ParentID != parentID {
			continue
		}
		node := &menuManageItem{}
		*node = menus[i]
		children := buildManageTree(menus, menus[i].ID)
		if len(children) > 0 {
			node.Children = children
		}
		result = append(result, node)
	}
	return result
}

// MenuManageCreate 创建菜单
func MenuManageCreate(c *gin.Context) {
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}

	var req struct {
		ParentID   int64  `json:"parentId"`
		Name       string `json:"name"`
		Path       string `json:"path"`
		Component  string `json:"component"`
		Redirect   string `json:"redirect"`
		Title      string `json:"title"`
		Icon       string `json:"icon"`
		Sort       int    `json:"sort"`
		IsHide     bool   `json:"isHide"`
		IsHideTab  bool   `json:"isHideTab"`
		IsFullPage bool   `json:"isFullPage"`
		KeepAlive  bool   `json:"keepAlive"`
		FixedTab   bool   `json:"fixedTab"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if req.Name == "" || req.Path == "" {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "名称和路径不能为空"})
		return
	}
	if req.Title == "" {
		req.Title = req.Name
	}
	if invalidMenuParent(0, req.ParentID, loadMenuParentIndex(db)) {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "不能选择自身或下级菜单作为上级"})
		return
	}

	result, err := db.Exec(`INSERT INTO menus (parent_id, name, path, component, redirect, title, icon, sort, is_hide, is_hide_tab, is_full_page, keep_alive, fixed_tab)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ParentID, req.Name, req.Path, req.Component, req.Redirect, req.Title, req.Icon, req.Sort,
		req.IsHide, req.IsHideTab, req.IsFullPage, req.KeepAlive, req.FixedTab)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "创建失败: " + err.Error()})
		return
	}

	id, _ := result.LastInsertId()

	// 自动关联到超级管理员角色
	db.Exec("INSERT IGNORE INTO role_menus (role_id, menu_id) VALUES (1, ?)", id)

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "创建成功", "data": gin.H{"id": id}})
}

// MenuManageUpdate 更新菜单
func MenuManageUpdate(c *gin.Context) {
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}

	id := c.Param("id")
	var req struct {
		ParentID   int64  `json:"parentId"`
		Name       string `json:"name"`
		Path       string `json:"path"`
		Component  string `json:"component"`
		Redirect   string `json:"redirect"`
		Title      string `json:"title"`
		Icon       string `json:"icon"`
		Sort       int    `json:"sort"`
		IsHide     bool   `json:"isHide"`
		IsHideTab  bool   `json:"isHideTab"`
		IsFullPage bool   `json:"isFullPage"`
		KeepAlive  bool   `json:"keepAlive"`
		FixedTab   bool   `json:"fixedTab"`
		Enabled    bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	menuID, err := strconv.ParseInt(id, 10, 64)
	if err != nil || menuID == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if invalidMenuParent(menuID, req.ParentID, loadMenuParentIndex(db)) {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "不能选择自身或下级菜单作为上级"})
		return
	}
	if req.Title == "" {
		req.Title = req.Name
	}

	enabledInt := 0
	if req.Enabled {
		enabledInt = 1
	}

	_, err = db.Exec(`UPDATE menus SET parent_id=?, name=?, path=?, component=?, redirect=?, title=?, icon=?, sort=?,
		is_hide=?, is_hide_tab=?, is_full_page=?, keep_alive=?, fixed_tab=?, enabled=? WHERE id=?`,
		req.ParentID, req.Name, req.Path, req.Component, req.Redirect, req.Title, req.Icon, req.Sort,
		req.IsHide, req.IsHideTab, req.IsFullPage, req.KeepAlive, req.FixedTab, enabledInt, id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "更新失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "更新成功"})
}

// MenuManageDelete 删除菜单
func MenuManageDelete(c *gin.Context) {
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}

	id := c.Param("id")

	// 检查是否有子菜单
	var childCount int
	db.QueryRow("SELECT COUNT(*) FROM menus WHERE parent_id=?", id).Scan(&childCount)
	if childCount > 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "该菜单下有子菜单，请先删除子菜单"})
		return
	}

	// 删除角色关联
	db.Exec("DELETE FROM role_menus WHERE menu_id=?", id)
	// 删除菜单
	db.Exec("DELETE FROM menus WHERE id=?", id)

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
}

func loadMenuParentIndex(db *sql.DB) map[int64]int64 {
	out := map[int64]int64{}
	if db == nil {
		return out
	}
	rows, err := db.Query("SELECT id, parent_id FROM menus")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, parentID int64
		if rows.Scan(&id, &parentID) == nil {
			out[id] = parentID
		}
	}
	return out
}

func invalidMenuParent(menuID, parentID int64, parentByID map[int64]int64) bool {
	if parentID == 0 {
		return false
	}
	if menuID != 0 && parentID == menuID {
		return true
	}
	seen := map[int64]bool{}
	if menuID != 0 {
		seen[menuID] = true
	}
	cur := parentID
	for cur != 0 {
		if seen[cur] {
			return true
		}
		seen[cur] = true
		next, ok := parentByID[cur]
		if !ok {
			return false
		}
		cur = next
	}
	return false
}

func removeHomeTemplateMenu(db *sql.DB) {
	_, _ = db.Exec(`
		DELETE rm FROM role_menus rm
		INNER JOIN menus m ON m.id = rm.menu_id
		WHERE m.name = 'HomeTemplate' OR m.path = '/home-template'
	`)
	_, _ = db.Exec("DELETE FROM menus WHERE name = 'HomeTemplate' OR path = '/home-template'")
}

func cleanupPurchaseLimitCampaignMenu(db *sql.DB) {
	_, _ = db.Exec("DELETE FROM role_menus WHERE menu_id = 213")
	_, _ = db.Exec("DELETE FROM menus WHERE id = 213 OR name = 'PurchaseLimitCampaigns'")
}

// removeSystemMonitorMenu 删除「定时任务 / 系统监控」。
// 页面、接口和进程内调度已移除；老库里的菜单行和角色绑定一并清掉。
func removeSystemMonitorMenu(db *sql.DB) {
	_, _ = db.Exec(`
		DELETE rm FROM role_menus rm
		INNER JOIN menus m ON m.id = rm.menu_id
		WHERE m.name = 'SystemMonitor'
			OR m.path IN ('monitor', '/system/monitor')
			OR m.component = '/system/monitor'
	`)
	_, _ = db.Exec(`
		DELETE FROM menus
		WHERE name = 'SystemMonitor'
			OR path IN ('monitor', '/system/monitor')
			OR component = '/system/monitor'
	`)
}

// removeAlipayF2FConfigMenu 删除「支付宝当面付」独立侧栏。
// 配置已并入 EpayConfig；id 213 也曾被该菜单占用，名称/路径/组件一并清掉，避免老库仍显示。
func removeAlipayF2FConfigMenu(db *sql.DB) {
	_, _ = db.Exec(`
		DELETE rm FROM role_menus rm
		INNER JOIN menus m ON m.id = rm.menu_id
		WHERE m.name = 'AlipayF2FConfig'
			OR m.path IN ('alipay-f2f-config', '/system/alipay-f2f-config')
			OR m.component = '/system/alipay-f2f-config'
	`)
	_, _ = db.Exec(`
		DELETE FROM menus
		WHERE name = 'AlipayF2FConfig'
			OR path IN ('alipay-f2f-config', '/system/alipay-f2f-config')
			OR component = '/system/alipay-f2f-config'
	`)
}
