// 官网公开页的数据：导航、文档、更新日志、两边都有的能力，以及授权购买要展示的域名规则。
// 四个页面的正文由宿主读取这里的接口；模板只改颜色和排版，不在包里写这些页面。

package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

const (
	sitePagesDocsMigration         = "site_pages_docs_v1"
	sitePagesDocsRefreshMigration  = "site_pages_docs_v2"
	sitePagesDocsRefreshMigration3 = "site_pages_docs_v3"
	sitePagesDocsRefreshMigration4 = "site_pages_docs_v4"
	sitePagesDocsRefreshMigration5 = "site_pages_docs_v5"
	sitePagesDocsRefreshMigration6 = "site_pages_docs_v6"
	siteNavBuiltin                 = "builtin"
	siteNavExternal                = "external"
	siteChangelogRelease           = "release"
	siteChangelogManual            = "manual"
	siteTagAdded                   = "added"
	siteTagImproved                = "improved"
	siteTagFixed                   = "fixed"
	// 免费版安装包走源站下载接口，不写死版本号，也不露出仓库地址。
	freeEditionDownloadURL = "https://auth.maizll.com/api/v1/update/package/latest"
)

var (
	sitePagesSchemaMu           sync.Mutex
	sitePagesSchemaDB           = map[string]bool{}
	siteSlugPattern             = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	siteVersionPattern          = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	siteReleaseFilePattern      = regexp.MustCompile(`^release-notes-(\d+\.\d+\.\d+)\.txt$`)
	siteChangelogHeadingPattern = regexp.MustCompile(`(?m)^## \[v?(\d+\.\d+\.\d+)\] (\d{4}-\d{2}-\d{2})\b`)
)

var siteBuiltinNav = []struct {
	Key   string
	Label string
	Href  string
	Sort  int
}{
	{Key: "home", Label: "首页", Href: "/user/login", Sort: 10},
	{Key: "purchase", Label: "授权购买", Href: "/buy", Sort: 20},
	{Key: "compare", Label: "系统对比", Href: "/compare", Sort: 30},
	{Key: "docs", Label: "系统文档", Href: "/docs", Sort: 40},
	{Key: "changelog", Label: "更新日志", Href: "/changelog", Sort: 50},
}

// 两边都有、并且代码里确实存在的能力。差别行不写在这里，对照表跟 commercialCompareRows。
var siteDefaultSharedFeatures = []string{
	"授权校验不区分免费版和商业版",
	"已经创建的授权应用会保留",
	"域名授权、授权码和卡密",
	"用户注册、登录和个人中心",
	"授权状态查询",
	"代理商开码与财务流水",
	"工单",
	"在线更新",
	"应用版本检查与客户端下载",
	"免费首页模板可以安装",
}

// RegisterSitePagePublicRoutes 注册不登录也能读的官网接口。
// 数据库不可用时返回 code 500，不返回编造的价格。
func RegisterSitePagePublicRoutes(api *gin.RouterGroup) {
	api.GET("/v1/site/nav", SiteNavPublic)
	api.GET("/v1/site/docs", SiteDocsPublic)
	api.GET("/v1/site/docs/:slug", SiteDocPublic)
	api.GET("/v1/site/changelog", SiteChangelogPublic)
	api.GET("/v1/site/compare", SiteComparePublic)
	api.GET("/v1/site/purchase-info", SitePurchaseInfoPublic)
}

// RegisterSitePageAdminRoutes 注册「官网页面」管理接口，调用方须已套管理员鉴权。
func RegisterSitePageAdminRoutes(api *gin.RouterGroup) {
	api.GET("/system/site-pages/nav", SiteNavAdminList)
	api.POST("/system/site-pages/nav", SiteNavAdminCreate)
	api.PUT("/system/site-pages/nav/:id", SiteNavAdminUpdate)
	api.DELETE("/system/site-pages/nav/:id", SiteNavAdminDelete)

	api.GET("/system/site-pages/doc-categories", SiteDocCategoryAdminList)
	api.POST("/system/site-pages/doc-categories", SiteDocCategoryAdminCreate)
	api.PUT("/system/site-pages/doc-categories/:id", SiteDocCategoryAdminUpdate)
	api.DELETE("/system/site-pages/doc-categories/:id", SiteDocCategoryAdminDelete)

	api.GET("/system/site-pages/docs", SiteDocAdminList)
	api.POST("/system/site-pages/docs", SiteDocAdminCreate)
	api.PUT("/system/site-pages/docs/:id", SiteDocAdminUpdate)
	api.DELETE("/system/site-pages/docs/:id", SiteDocAdminDelete)

	api.GET("/system/site-pages/changelog", SiteChangelogAdminList)
	api.POST("/system/site-pages/changelog", SiteChangelogAdminCreate)
	api.PUT("/system/site-pages/changelog/:id", SiteChangelogAdminUpdate)
	api.DELETE("/system/site-pages/changelog/:id", SiteChangelogAdminDelete)

	api.GET("/system/site-pages/compare", SiteCompareAdminList)
	api.POST("/system/site-pages/compare", SiteCompareAdminCreate)
	api.PUT("/system/site-pages/compare/:id", SiteCompareAdminUpdate)
	api.DELETE("/system/site-pages/compare/:id", SiteCompareAdminDelete)
}

// EnsureSitePagesSchema 建表、补齐五个内置导航，并在首次启动时灌入公开文档。
// 已有站点会刷新没改过的白名单文档，并补上安装部署；改过的文章不覆盖。
// 每次启动都会把新的 release notes 补进更新日志；已改过或已删除的条目不会被盖回去。
// 数据库执行失败时返回错误，调用方只记日志，不阻断已经在跑的进程。
func EnsureSitePagesSchema(db *sql.DB) error {
	if db == nil {
		return nil
	}
	var name string
	if err := db.QueryRow("SELECT DATABASE()").Scan(&name); err != nil {
		return err
	}
	sitePagesSchemaMu.Lock()
	defer sitePagesSchemaMu.Unlock()
	if sitePagesSchemaDB[name] {
		return nil
	}
	if err := ensureSitePagesTables(db); err != nil {
		return err
	}
	if err := ensureBuiltinSiteNav(db); err != nil {
		return err
	}
	if err := ensureDefaultSharedFeatures(db); err != nil {
		return err
	}
	if err := seedSiteDocsOnce(db); err != nil {
		return err
	}
	if err := refreshSiteDocsOnce(db); err != nil {
		return err
	}
	// 先删掉没改过、又带仓库地址的更新日志，再按仓库里的发布说明重新导入。
	if err := scrubLeakedRepositoryNotes(db); err != nil {
		return err
	}
	if err := importSiteReleaseNotes(db, findSiteDocsRoot()); err != nil {
		return err
	}
	sitePagesSchemaDB[name] = true
	return nil
}

func ensureSitePagesTables(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		name VARCHAR(100) NOT NULL,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (name)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='运行时结构迁移记录'`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS site_nav_items (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			label VARCHAR(40) NOT NULL,
			kind VARCHAR(16) NOT NULL,
			builtin_key VARCHAR(32) NOT NULL DEFAULT '',
			href VARCHAR(500) NOT NULL DEFAULT '',
			enabled TINYINT NOT NULL DEFAULT 1,
			sort INT NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			KEY idx_site_nav_sort (enabled, sort, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='官网顶部导航'`,
		`CREATE TABLE IF NOT EXISTS site_doc_categories (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			name VARCHAR(40) NOT NULL,
			slug VARCHAR(80) NOT NULL,
			sort INT NOT NULL DEFAULT 0,
			hidden TINYINT NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			UNIQUE KEY uk_site_doc_category_slug (slug)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='官网文档分类'`,
		`CREATE TABLE IF NOT EXISTS site_doc_articles (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			category_id BIGINT UNSIGNED NOT NULL,
			title VARCHAR(120) NOT NULL,
			slug VARCHAR(120) NOT NULL,
			summary VARCHAR(300) NOT NULL DEFAULT '',
			body MEDIUMTEXT NOT NULL,
			sort INT NOT NULL DEFAULT 0,
			hidden TINYINT NOT NULL DEFAULT 0,
			source_path VARCHAR(200) NOT NULL DEFAULT '',
			edited TINYINT NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			UNIQUE KEY uk_site_doc_slug (slug),
			KEY idx_site_doc_category (category_id, hidden, sort, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='官网文档'`,
		`CREATE TABLE IF NOT EXISTS site_changelog_entries (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			version VARCHAR(32) NOT NULL,
			released_on DATE NULL,
			tag VARCHAR(16) NOT NULL,
			body VARCHAR(2000) NOT NULL,
			hidden TINYINT NOT NULL DEFAULT 0,
			edited TINYINT NOT NULL DEFAULT 0,
			source VARCHAR(16) NOT NULL,
			fingerprint CHAR(64) NOT NULL,
			sort INT NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			UNIQUE KEY uk_site_changelog_fp (fingerprint),
			KEY idx_site_changelog_version (hidden, released_on, version)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='官网更新日志'`,
		`CREATE TABLE IF NOT EXISTS site_changelog_skips (
			fingerprint CHAR(64) NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (fingerprint)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='已从更新日志删除、不再自动导入的条目'`,
		`CREATE TABLE IF NOT EXISTS site_compare_features (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			body VARCHAR(200) NOT NULL,
			sort INT NOT NULL DEFAULT 0,
			hidden TINYINT NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			KEY idx_site_compare_sort (hidden, sort, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='两个版本都具备的能力'`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func sitePagesDB(c *gin.Context) (*sql.DB, bool) {
	db, err := config.DB()
	if err != nil {
		siteFail(c, 500, "数据库不可用")
		return nil, false
	}
	if err := EnsureSitePagesSchema(db); err != nil {
		siteFail(c, 500, "官网页面数据未就绪")
		return nil, false
	}
	return db, true
}

func siteOK(c *gin.Context, data any) {
	writeSystemConfig(c, http.StatusOK, gin.H{"code": 200, "msg": "", "data": data})
}

func siteFail(c *gin.Context, code int, msg string) {
	writeSystemConfig(c, http.StatusOK, gin.H{"code": code, "msg": msg})
}

func readSiteJSON(c *gin.Context, dest any) bool {
	if err := json.NewDecoder(c.Request.Body).Decode(dest); err != nil {
		siteFail(c, 400, "参数错误")
		return false
	}
	return true
}

func siteID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		siteFail(c, 400, "编号不正确")
		return 0, false
	}
	return id, true
}

// SitePurchaseInfoPublic 返回免费版下载地址和给客户看的域名说明。
// 商业版价格由前端另读 /api/v1/store/edition-plans，再按套餐拼免费更换次数。
func SitePurchaseInfoPublic(c *gin.Context) {
	days := int(storeDomainChangeWindow / (24 * time.Hour))
	siteOK(c, gin.H{
		"freeDownloadUrl": freeEditionDownloadURL,
		"freePriceCents":  0,
		"selfServiceDays": days,
		"rules":           siteDomainRules(days),
	})
}

// siteDomainRules 给客户看的域名说明。不写字段名，也不列举内网后缀，避免手机上被截断。
// 套餐免费次数和超出单价由购买页按套餐数据另写，这里只保留和套餐无关的规则。
func siteDomainRules(days int) []string {
	if days < 1 {
		days = 1
	}
	return []string{
		"www 和不带 www 算两个域名。",
		"不支持 IP、本地和内网地址。",
		"必须使用 HTTPS。",
		fmt.Sprintf("每 %d 天可自助更换一次。", days),
	}
}

// SiteNavPublic 返回已开启的导航，按排序。内置项的地址固定，外链只接受 http/https。
func SiteNavPublic(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	list, err := listSiteNav(db, true)
	if err != nil {
		siteFail(c, 500, "读取导航失败")
		return
	}
	siteOK(c, gin.H{"list": list})
}

func SiteNavAdminList(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	list, err := listSiteNav(db, false)
	if err != nil {
		siteFail(c, 500, "读取导航失败")
		return
	}
	siteOK(c, gin.H{"list": list})
}

func listSiteNav(db *sql.DB, enabledOnly bool) ([]gin.H, error) {
	query := `SELECT id, label, kind, builtin_key, href, enabled, sort FROM site_nav_items`
	if enabledOnly {
		query += ` WHERE enabled = 1`
	}
	query += ` ORDER BY sort ASC, id ASC`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]gin.H, 0)
	for rows.Next() {
		var id int64
		var label, kind, builtinKey, href string
		var enabled, sort int
		if err := rows.Scan(&id, &label, &kind, &builtinKey, &href, &enabled, &sort); err != nil {
			return nil, err
		}
		list = append(list, gin.H{
			"id": id, "label": label, "kind": kind, "builtinKey": builtinKey,
			"href": href, "enabled": enabled == 1, "sort": sort,
		})
	}
	return list, rows.Err()
}

func ensureBuiltinSiteNav(db *sql.DB) error {
	for _, item := range siteBuiltinNav {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM site_nav_items WHERE kind = ? AND builtin_key = ?`, siteNavBuiltin, item.Key).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		if _, err := db.Exec(`INSERT INTO site_nav_items (label, kind, builtin_key, href, enabled, sort) VALUES (?, ?, ?, ?, 1, ?)`,
			item.Label, siteNavBuiltin, item.Key, item.Href, item.Sort); err != nil {
			return err
		}
	}
	return nil
}

// SiteNavAdminCreate 只允许添加外链。内置入口由系统补齐，不能再造一个同名页面。
// 地址必须是 http 或 https。成功返回新编号。
func SiteNavAdminCreate(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	var req struct {
		Label string `json:"label"`
		Href  string `json:"href"`
	}
	if !readSiteJSON(c, &req) {
		return
	}
	label := trimSiteText(req.Label, 40)
	href, err := normalizeExternalHref(req.Href)
	if label == "" || err != nil {
		siteFail(c, 400, "请填写名称和 http 或 https 链接")
		return
	}
	var maxSort int
	_ = db.QueryRow(`SELECT COALESCE(MAX(sort), 0) FROM site_nav_items`).Scan(&maxSort)
	result, err := db.Exec(`INSERT INTO site_nav_items (label, kind, builtin_key, href, enabled, sort) VALUES (?, ?, '', ?, 1, ?)`,
		label, siteNavExternal, href, maxSort+10)
	if err != nil {
		siteFail(c, 500, "保存导航失败")
		return
	}
	id, _ := result.LastInsertId()
	siteOK(c, gin.H{"id": id})
}

// SiteNavAdminUpdate 可改名称、开关和排序。内置项的地址保持不变，外链可改地址。
func SiteNavAdminUpdate(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	id, ok := siteID(c)
	if !ok {
		return
	}
	var kind, href string
	if err := db.QueryRow(`SELECT kind, href FROM site_nav_items WHERE id = ?`, id).Scan(&kind, &href); err != nil {
		siteFail(c, 404, "导航不存在")
		return
	}
	var req struct {
		Label   string `json:"label"`
		Href    string `json:"href"`
		Enabled *bool  `json:"enabled"`
		Sort    *int   `json:"sort"`
	}
	if !readSiteJSON(c, &req) {
		return
	}
	label := trimSiteText(req.Label, 40)
	if label == "" {
		siteFail(c, 400, "请填写名称")
		return
	}
	if kind == siteNavExternal {
		next, err := normalizeExternalHref(req.Href)
		if err != nil {
			siteFail(c, 400, "请填写 http 或 https 链接")
			return
		}
		href = next
	}
	enabled := 1
	if req.Enabled != nil && !*req.Enabled {
		enabled = 0
	}
	sort := 0
	if req.Sort != nil {
		sort = *req.Sort
	} else {
		_ = db.QueryRow(`SELECT sort FROM site_nav_items WHERE id = ?`, id).Scan(&sort)
	}
	if _, err := db.Exec(`UPDATE site_nav_items SET label = ?, href = ?, enabled = ?, sort = ? WHERE id = ?`,
		label, href, enabled, sort, id); err != nil {
		siteFail(c, 500, "保存导航失败")
		return
	}
	siteOK(c, gin.H{"id": id})
}

// SiteNavAdminDelete 只删除外链。内置的五个入口不能删，以免模板把页面入口拿掉。
func SiteNavAdminDelete(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	id, ok := siteID(c)
	if !ok {
		return
	}
	var kind string
	if err := db.QueryRow(`SELECT kind FROM site_nav_items WHERE id = ?`, id).Scan(&kind); err != nil {
		siteFail(c, 404, "导航不存在")
		return
	}
	if kind != siteNavExternal {
		siteFail(c, 400, "内置导航不能删除，可以关闭或改名")
		return
	}
	if _, err := db.Exec(`DELETE FROM site_nav_items WHERE id = ?`, id); err != nil {
		siteFail(c, 500, "删除导航失败")
		return
	}
	siteOK(c, gin.H{"id": id})
}

func normalizeExternalHref(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return "", errors.New("invalid")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("invalid")
	}
	return parsed.String(), nil
}

func trimSiteText(value string, max int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max])
}

func normalizeSiteSlug(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")
	if !siteSlugPattern.MatchString(value) || len(value) > 80 {
		return "", errors.New("invalid")
	}
	return value, nil
}
