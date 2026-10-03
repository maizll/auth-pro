package handler

import (
	"database/sql"
	_ "embed"
	"errors"
	"net/http"
	"strings"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

//go:embed schema.sql
var schemaSQL string

//go:embed menu_seed.sql
var menuSeedSQL string

// AfterInstall 由 main 设为启动迁移。网页向导装完后同步跑一遍，
// 不重启也能用上迁移建的表（例如商业版设置表 app_commercial_settings，缺了新建出售商业版的应用会报 1146）。
var AfterInstall func()

// openInstallDatabase 供测试替换，生产环境仍打开 MySQL。
var openInstallDatabase = func(dsn string) (*sql.DB, error) {
	return sql.Open("mysql", dsn)
}

type dbRequest struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	Database string `json:"database"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type createAdminRequest struct {
	dbRequest
	AdminUsername string `json:"adminUsername"`
	AdminPassword string `json:"adminPassword"`
}

// InstallStatus 仅根据 install.lock 判断安装状态。
func InstallStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"installed": config.IsInstalled(),
	})
}

// InstallTestDB 测试数据库连接
func InstallTestDB(c *gin.Context) {
	var req dbRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	cfg := &config.DBConfig{
		Host:     req.Host,
		Port:     req.Port,
		Database: req.Database,
		Username: req.Username,
		Password: req.Password,
	}

	dsn := cfg.Username + ":" + cfg.Password + "@tcp(" + cfg.Host + ":" + cfg.Port + ")/" + cfg.Database + "?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := openInstallDatabase(dsn)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "连接失败: " + err.Error()})
		return
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "连接失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "连接成功"})
}

// InstallInitTables 初始化数据库表
func InstallInitTables(c *gin.Context) {
	var req dbRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	cfg := &config.DBConfig{
		Host:     req.Host,
		Port:     req.Port,
		Database: req.Database,
		Username: req.Username,
		Password: req.Password,
	}

	if refused := installExistingDatabaseRefusal(cfg); refused != "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": refused})
		return
	}

	dsn := config.GetDSN(cfg)
	db, err := openInstallDatabase(dsn)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "连接失败: " + err.Error()})
		return
	}
	defer db.Close()

	occupied, err := installDatabaseHasData(db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "无法确认数据库是否已有数据，已拒绝初始化"})
		return
	}
	if occupied {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "数据库已有业务数据，拒绝重新初始化"})
		return
	}

	if _, err := db.Exec(schemaSQL); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "建表失败: " + err.Error()})
		return
	}

	// 插入菜单种子数据
	if _, err := db.Exec(menuSeedSQL); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "初始化菜单失败: " + err.Error()})
		return
	}

	// 保存数据库配置
	if err := config.SaveDBConfig(cfg); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "保存配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "数据表安装完成"})
}

// InstallCreateAdmin 创建管理员并完成安装
func InstallCreateAdmin(c *gin.Context) {
	var req createAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	cfg, err := config.LoadDBConfig()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "读取配置失败: " + err.Error()})
		return
	}

	dsn := config.GetDSN(cfg)
	db, err := openInstallDatabase(dsn)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "连接失败: " + err.Error()})
		return
	}
	defer db.Close()

	occupied, err := installDatabaseHasData(db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "无法确认数据库是否已有数据，已拒绝创建管理员"})
		return
	}
	if occupied {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "系统已存在管理员或业务数据，拒绝重复创建超级管理员"})
		return
	}

	// 账号或密码没送到时不能当成安装成功。空字符串也能做出 bcrypt，
	// 接口仍会返回 200，但打印出来的密码永远登不进后台。
	adminUsername := strings.TrimSpace(req.AdminUsername)
	if adminUsername == "" || strings.TrimSpace(req.AdminPassword) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请填写管理员账号和密码"})
		return
	}

	// 插入默认角色（先建角色，管理员需要引用 role_id=1）
	_, _ = db.Exec(`INSERT IGNORE INTO roles (id, role_name, role_code, description, discount, enabled) VALUES
		(1, '超级管理员', 'R_SUPER', '系统超级管理员，拥有所有权限', 10.0, 1),
		(2, '代理商', 'R_AGENT', '代理商角色，可管理下级授权', 8.0, 1),
		(3, '服务商', 'R_SERVICE', '服务商角色，提供技术服务', 7.0, 1),
		(4, '合作商', 'R_PARTNER', '合作商角色，合作推广', 6.5, 1),
		(5, '开发者', 'R_DEVELOPER', '软件源开发者，可提交插件与首页模板元数据', 10.0, 1)`)

	// 哈希密码
	hash, err := bcrypt.GenerateFromPassword([]byte(req.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "密码加密失败"})
		return
	}

	// 插入管理员
	_, err = db.Exec(
		"INSERT INTO admins (username, password_hash, nickname, role_id, enabled) VALUES (?, ?, '超级管理员', 1, 1)",
		adminUsername, string(hash),
	)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "创建管理员失败: " + err.Error()})
		return
	}

	// 将全部菜单关联到超级管理员角色
	_, _ = db.Exec("INSERT IGNORE INTO role_menus (role_id, menu_id) SELECT 1, id FROM menus")
	// 给其他角色分配基础菜单（排除系统管理）
	_, _ = db.Exec("INSERT IGNORE INTO role_menus (role_id, menu_id) SELECT 2, id FROM menus WHERE name NOT LIKE 'System%' AND name NOT LIKE 'Menu%'")
	_, _ = db.Exec("INSERT IGNORE INTO role_menus (role_id, menu_id) SELECT 3, id FROM menus WHERE name NOT LIKE 'System%' AND name NOT LIKE 'Menu%'")
	_, _ = db.Exec("INSERT IGNORE INTO role_menus (role_id, menu_id) SELECT 4, id FROM menus WHERE name NOT LIKE 'System%' AND name NOT LIKE 'Menu%'")

	// 生成并持久化 JWT 签名密钥（已存在则复用，避免轮换导致旧 token 失效）
	if _, err := config.LoadOrCreateJWTSecret(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "生成 JWT 密钥失败: " + err.Error()})
		return
	}

	// 创建锁文件
	if err := config.CreateLockFile(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "创建锁文件失败: " + err.Error()})
		return
	}

	if AfterInstall != nil {
		AfterInstall()
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "安装完成"})
}

// installExistingDatabaseRefusal 防止 install.lock 丢失后被人改接到他自己准备的空库：
// 本机已经有数据库配置（db.json 或环境变量）且指向别的库时，先连现有配置看一眼，
// 现有库里有数据、或者连不上没法确认，就拒绝，不覆盖现有配置。返回空字符串表示可以继续。
// 现有配置指向的就是这次填的库时交给后面的「库里是否已有数据」检查；现有库是空的（上次装到一半）可以换库重装。
func installExistingDatabaseRefusal(requested *config.DBConfig) string {
	existing, err := config.LoadDBConfig()
	if err != nil || existing == nil || strings.TrimSpace(existing.Host) == "" {
		return ""
	}
	if sameInstallDatabase(existing, requested) {
		return ""
	}
	db, err := openInstallDatabase(config.GetDSN(existing))
	if err != nil {
		return "本站已有数据库配置，但现在连不上，无法确认是否已经安装，已拒绝改接别的数据库"
	}
	defer db.Close()
	occupied, err := installDatabaseHasData(db)
	if err != nil {
		return "本站已有数据库配置，但现在连不上，无法确认是否已经安装，已拒绝改接别的数据库"
	}
	if occupied {
		return "本站已经连着一个有数据的数据库，不能改接别的库。如果是 install.lock 丢了，请从备份恢复它"
	}
	return ""
}

func sameInstallDatabase(a, b *config.DBConfig) bool {
	return strings.EqualFold(strings.TrimSpace(a.Host), strings.TrimSpace(b.Host)) &&
		strings.TrimSpace(a.Port) == strings.TrimSpace(b.Port) &&
		strings.TrimSpace(a.Database) == strings.TrimSpace(b.Database)
}

// installDataTables 是安装写接口用来判断「库里已经有本系统数据」的表。
// 空库（表不存在，或这些表都是 0 行）仍走原来的建表和创建首个管理员流程。
var installDataTables = []string{
	"admins",
	"users",
	"agents",
	"apps",
	"licenses",
	"license_plans",
	"transactions",
	"license_purchase_orders",
}

// installDatabaseHasData 在锁文件丢失时阻止清库和再造超级管理员。
// 只要 admins 或业务表里已有行，就视为已经安装。探测失败时返回错误，调用方必须拒绝写操作。
func installDatabaseHasData(db *sql.DB) (bool, error) {
	for _, table := range installDataTables {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM `" + table + "`").Scan(&count)
		if err != nil {
			if isMissingInstallTable(err) {
				continue
			}
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

func isMissingInstallTable(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1146
}
