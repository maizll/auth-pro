package handler

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

// 公开文档白名单。audit、superpowers、design、admin-navigation、发布说明和技能文件不进官网。
var sitePublicDocs = map[string]struct{}{
	"README.md":                                       {},
	"install.md":                                      {},
	"deployment.md":                                   {},
	"admin.md":                                        {},
	"commercial.md":                                   {},
	"security.md":                                     {},
	"api-sdk.md":                                      {},
	"home-template.md":                                {},
	"software-source-client-url.md":                   {},
	"source-station-repo-setup.md":                    {},
	"code-structure.md":                               {},
	"developer/README.md":                             {},
	"developer/template-package.md":                   {},
	"developer/plugin-package.md":                     {},
	"developer/charter.md":                            {},
	"developer/packaging.md":                          {},
	"developer/validation.md":                         {},
	"developer/versions.md":                           {},
	"developer/review-and-catalog.md":                 {},
	"developer/payment-channel-plugin.md":             {},
	"developer/starter/template-example/README.md":    {},
	"developer/starter/plugin-example/README.md":      {},
	"developer/starter/enterprise-template/README.md": {},
}

func siteDocAllowed(rel string) bool {
	rel = filepath.ToSlash(strings.TrimPrefix(rel, "./"))
	if _, ok := sitePublicDocs[rel]; ok {
		return true
	}
	return false
}

func siteDocCategoryFor(rel string) (name, slug string, sort int) {
	switch {
	case strings.HasPrefix(rel, "developer/"):
		return "开发者", "developer", 30
	case rel == "install.md" || rel == "deployment.md" || rel == "security.md" || rel == "code-structure.md" || rel == "source-station-repo-setup.md":
		return "部署与运维", "ops", 10
	default:
		return "使用说明", "guide", 20
	}
}

func seedSiteDocsOnce(db *sql.DB) error {
	pending, err := sourceStationMigrationPending(db, sitePagesDocsMigration)
	if err != nil || !pending {
		return err
	}
	root := findSiteDocsRoot()
	if root == "" {
		return nil
	}
	if err := seedSiteDocs(db, root); err != nil {
		return err
	}
	return markSourceStationMigration(db, sitePagesDocsMigration)
}

// siteDocSeedSort 让「安装部署」排在部署分类最前，其余仍按文件名顺序。
func siteDocSeedSort(rel string, fallback int) int {
	if rel == "install.md" {
		return 5
	}
	return fallback
}

// refreshSiteDocsOnce 给已有站点补上 edited，并刷新没改过的白名单正文。
// 找不到 docs 目录时不记迁移，下次启动再试。用户改过（edited=1）或自己新建的文章不覆盖。
func refreshSiteDocsOnce(db *sql.DB) error {
	if err := ensureSourceStationColumn(db, "site_doc_articles", "edited",
		`ALTER TABLE site_doc_articles ADD COLUMN edited TINYINT NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	// v2 刷新安装命令，v3 去掉旧升级说明，v4 去掉公开文档里的仓库地址。都跳过用户改过的文章。
	for _, name := range []string{sitePagesDocsRefreshMigration, sitePagesDocsRefreshMigration3, sitePagesDocsRefreshMigration4} {
		if err := runSiteDocsRefresh(db, name); err != nil {
			return err
		}
	}
	return nil
}

func runSiteDocsRefresh(db *sql.DB, name string) error {
	pending, err := sourceStationMigrationPending(db, name)
	if err != nil || !pending {
		return err
	}
	root := findSiteDocsRoot()
	if root == "" {
		return nil
	}
	if err := refreshUneditedSiteDocs(db, root); err != nil {
		return err
	}
	return markSourceStationMigration(db, name)
}

func refreshUneditedSiteDocs(db *sql.DB, root string) error {
	// 这一列是后加的。保存过的行 updated_at 会晚于 created_at，视为用户改过。
	if _, err := db.Exec(`UPDATE site_doc_articles SET edited = 1 WHERE edited = 0 AND updated_at > created_at`); err != nil {
		return err
	}
	paths := make([]string, 0, len(sitePublicDocs))
	for rel := range sitePublicDocs {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		full := filepath.Join(root, filepath.FromSlash(rel))
		payload, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		body := strings.TrimSpace(string(payload))
		if body == "" {
			continue
		}
		title := trimSiteText(siteDocTitle(body, rel), 120)
		slug := siteDocSlug(rel)
		summary := trimSiteText(siteDocSummary(body), 300)
		var id int64
		var edited int
		err = db.QueryRow(`SELECT id, edited FROM site_doc_articles WHERE source_path = ? ORDER BY id ASC LIMIT 1`, rel).Scan(&id, &edited)
		if err == sql.ErrNoRows {
			// 只补本版新增的安装文档。其余缺失行视为用户删过，不再插回去。
			if rel != "install.md" {
				continue
			}
			var taken int
			if err := db.QueryRow(`SELECT COUNT(*) FROM site_doc_articles WHERE slug = ?`, slug).Scan(&taken); err != nil {
				return err
			}
			if taken > 0 {
				continue
			}
			catName, catSlug, catSort := siteDocCategoryFor(rel)
			categoryID, err := ensureSiteDocCategory(db, catName, catSlug, catSort)
			if err != nil {
				return err
			}
			if _, err := db.Exec(`INSERT INTO site_doc_articles
				(category_id, title, slug, summary, body, sort, hidden, source_path, edited)
				VALUES (?, ?, ?, ?, ?, ?, 0, ?, 0)`,
				categoryID, title, slug, summary, body, siteDocSeedSort(rel, 10), rel); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if edited != 0 {
			continue
		}
		// 显式写回 updated_at，避免这次刷新被下一轮误判成用户改过。
		// 只调整安装文档的排序，其它文章保持原有顺序。
		if rel == "install.md" {
			_, err = db.Exec(`UPDATE site_doc_articles
				SET title = ?, summary = ?, body = ?, sort = ?, updated_at = created_at
				WHERE id = ? AND edited = 0`,
				title, summary, body, siteDocSeedSort(rel, 10), id)
		} else {
			_, err = db.Exec(`UPDATE site_doc_articles
				SET title = ?, summary = ?, body = ?, updated_at = created_at
				WHERE id = ? AND edited = 0`,
				title, summary, body, id)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func seedSiteDocs(db *sql.DB, root string) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM site_doc_articles`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	paths := make([]string, 0, len(sitePublicDocs))
	for rel := range sitePublicDocs {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	order := map[string]int{}
	for _, rel := range paths {
		full := filepath.Join(root, filepath.FromSlash(rel))
		payload, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		body := strings.TrimSpace(string(payload))
		if body == "" {
			continue
		}
		catName, catSlug, catSort := siteDocCategoryFor(rel)
		categoryID, err := ensureSiteDocCategory(db, catName, catSlug, catSort)
		if err != nil {
			return err
		}
		order[catSlug]++
		title := siteDocTitle(body, rel)
		slug := siteDocSlug(rel)
		summary := siteDocSummary(body)
		if _, err := db.Exec(`INSERT INTO site_doc_articles
			(category_id, title, slug, summary, body, sort, hidden, source_path)
			VALUES (?, ?, ?, ?, ?, ?, 0, ?)`,
			categoryID, trimSiteText(title, 120), slug, trimSiteText(summary, 300), body, siteDocSeedSort(rel, order[catSlug]*10), rel); err != nil {
			return err
		}
	}
	return nil
}

func ensureSiteDocCategory(db *sql.DB, name, slug string, sort int) (int64, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM site_doc_categories WHERE slug = ?`, slug).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	result, err := db.Exec(`INSERT INTO site_doc_categories (name, slug, sort, hidden) VALUES (?, ?, ?, 0)`, name, slug, sort)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func siteDocTitle(body, rel string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	base := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	if base == "README" {
		return "说明"
	}
	return base
}

func siteDocSlug(rel string) string {
	rel = strings.TrimSuffix(filepath.ToSlash(rel), ".md")
	rel = strings.ReplaceAll(rel, "/", "-")
	rel = strings.ToLower(rel)
	if rel == "readme" {
		return "index"
	}
	return rel
}

func siteDocSummary(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "|") || strings.HasPrefix(line, "```") {
			continue
		}
		return line
	}
	return ""
}

func findSiteDocsRoot() string {
	candidates := make([]string, 0, 6)
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "docs"), filepath.Join(wd, "..", "docs"))
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(dir, "docs"), filepath.Join(dir, "..", "docs"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(filepath.Join(candidate, "deployment.md")); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// SiteDocsPublic 返回未隐藏的分类和文章目录，不含正文。
func SiteDocsPublic(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	categories, err := listSiteDocCategories(db, true)
	if err != nil {
		siteFail(c, 500, "读取文档失败")
		return
	}
	articles, err := listSiteDocArticles(db, true)
	if err != nil {
		siteFail(c, 500, "读取文档失败")
		return
	}
	siteOK(c, gin.H{"categories": categories, "articles": articles})
}

// SiteDocPublic 按 slug 返回一篇未隐藏的文档，并给出同分类里的上一篇和下一篇。
// 找不到或已隐藏时 code 为 404。
func SiteDocPublic(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	slug := strings.TrimSpace(c.Param("slug"))
	article, err := loadSiteDocArticle(db, slug, true)
	if err != nil {
		siteFail(c, 404, "文档不存在")
		return
	}
	prev, next := siteDocNeighbors(db, article)
	article["prev"] = prev
	article["next"] = next
	siteOK(c, article)
}

func SiteDocCategoryAdminList(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	list, err := listSiteDocCategories(db, false)
	if err != nil {
		siteFail(c, 500, "读取分类失败")
		return
	}
	siteOK(c, gin.H{"list": list})
}

func SiteDocCategoryAdminCreate(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
		Sort *int   `json:"sort"`
	}
	if !readSiteJSON(c, &req) {
		return
	}
	name := trimSiteText(req.Name, 40)
	slug, err := normalizeSiteSlug(req.Slug)
	if name == "" || err != nil {
		siteFail(c, 400, "请填写分类名称和英文标识")
		return
	}
	sort := 100
	if req.Sort != nil {
		sort = *req.Sort
	}
	result, err := db.Exec(`INSERT INTO site_doc_categories (name, slug, sort, hidden) VALUES (?, ?, ?, 0)`, name, slug, sort)
	if err != nil {
		siteFail(c, 400, "分类标识已存在")
		return
	}
	id, _ := result.LastInsertId()
	siteOK(c, gin.H{"id": id})
}

func SiteDocCategoryAdminUpdate(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	id, ok := siteID(c)
	if !ok {
		return
	}
	var req struct {
		Name   string `json:"name"`
		Sort   *int   `json:"sort"`
		Hidden *bool  `json:"hidden"`
	}
	if !readSiteJSON(c, &req) {
		return
	}
	name := trimSiteText(req.Name, 40)
	if name == "" {
		siteFail(c, 400, "请填写分类名称")
		return
	}
	sort := 0
	hidden := 0
	if err := db.QueryRow(`SELECT sort, hidden FROM site_doc_categories WHERE id = ?`, id).Scan(&sort, &hidden); err != nil {
		siteFail(c, 404, "分类不存在")
		return
	}
	if req.Sort != nil {
		sort = *req.Sort
	}
	if req.Hidden != nil {
		if *req.Hidden {
			hidden = 1
		} else {
			hidden = 0
		}
	}
	if _, err := db.Exec(`UPDATE site_doc_categories SET name = ?, sort = ?, hidden = ? WHERE id = ?`, name, sort, hidden, id); err != nil {
		siteFail(c, 500, "保存分类失败")
		return
	}
	siteOK(c, gin.H{"id": id})
}

func SiteDocCategoryAdminDelete(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	id, ok := siteID(c)
	if !ok {
		return
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM site_doc_articles WHERE category_id = ?`, id).Scan(&count); err != nil {
		siteFail(c, 500, "删除分类失败")
		return
	}
	if count > 0 {
		siteFail(c, 400, "请先删除或移走该分类下的文章")
		return
	}
	result, err := db.Exec(`DELETE FROM site_doc_categories WHERE id = ?`, id)
	if err != nil {
		siteFail(c, 500, "删除分类失败")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		siteFail(c, 404, "分类不存在")
		return
	}
	siteOK(c, gin.H{"id": id})
}

func SiteDocAdminList(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	list, err := listSiteDocArticles(db, false)
	if err != nil {
		siteFail(c, 500, "读取文档失败")
		return
	}
	siteOK(c, gin.H{"list": list})
}

func SiteDocAdminCreate(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	req, ok := readSiteDocReq(c)
	if !ok {
		return
	}
	var maxSort int
	_ = db.QueryRow(`SELECT COALESCE(MAX(sort), 0) FROM site_doc_articles WHERE category_id = ?`, req.CategoryID).Scan(&maxSort)
	if req.Sort == 0 {
		req.Sort = maxSort + 10
	}
	result, err := db.Exec(`INSERT INTO site_doc_articles
		(category_id, title, slug, summary, body, sort, hidden, source_path)
		VALUES (?, ?, ?, ?, ?, ?, ?, '')`,
		req.CategoryID, req.Title, req.Slug, req.Summary, req.Body, req.Sort, req.Hidden)
	if err != nil {
		siteFail(c, 400, "保存文档失败，标识可能重复")
		return
	}
	id, _ := result.LastInsertId()
	siteOK(c, gin.H{"id": id})
}

func SiteDocAdminUpdate(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	id, ok := siteID(c)
	if !ok {
		return
	}
	req, ok := readSiteDocReq(c)
	if !ok {
		return
	}
	result, err := db.Exec(`UPDATE site_doc_articles
		SET category_id = ?, title = ?, slug = ?, summary = ?, body = ?, sort = ?, hidden = ?, edited = 1
		WHERE id = ?`,
		req.CategoryID, req.Title, req.Slug, req.Summary, req.Body, req.Sort, req.Hidden, id)
	if err != nil {
		siteFail(c, 400, "保存文档失败，标识可能重复")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		siteFail(c, 404, "文档不存在")
		return
	}
	siteOK(c, gin.H{"id": id})
}

func SiteDocAdminDelete(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	id, ok := siteID(c)
	if !ok {
		return
	}
	result, err := db.Exec(`DELETE FROM site_doc_articles WHERE id = ?`, id)
	if err != nil {
		siteFail(c, 500, "删除文档失败")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		siteFail(c, 404, "文档不存在")
		return
	}
	siteOK(c, gin.H{"id": id})
}

type siteDocWrite struct {
	CategoryID int64
	Title      string
	Slug       string
	Summary    string
	Body       string
	Sort       int
	Hidden     int
}

func readSiteDocReq(c *gin.Context) (siteDocWrite, bool) {
	var req struct {
		CategoryID int64  `json:"categoryId"`
		Title      string `json:"title"`
		Slug       string `json:"slug"`
		Summary    string `json:"summary"`
		Body       string `json:"body"`
		Sort       int    `json:"sort"`
		Hidden     bool   `json:"hidden"`
	}
	if !readSiteJSON(c, &req) {
		return siteDocWrite{}, false
	}
	title := trimSiteText(req.Title, 120)
	slug, err := normalizeSiteSlug(req.Slug)
	if title == "" || err != nil || req.CategoryID <= 0 || strings.TrimSpace(req.Body) == "" {
		siteFail(c, 400, "请填写分类、标题、英文标识和正文")
		return siteDocWrite{}, false
	}
	hidden := 0
	if req.Hidden {
		hidden = 1
	}
	return siteDocWrite{
		CategoryID: req.CategoryID,
		Title:      title,
		Slug:       slug,
		Summary:    trimSiteText(req.Summary, 300),
		Body:       req.Body,
		Sort:       req.Sort,
		Hidden:     hidden,
	}, true
}

func listSiteDocCategories(db *sql.DB, publicOnly bool) ([]gin.H, error) {
	query := `SELECT id, name, slug, sort, hidden FROM site_doc_categories`
	if publicOnly {
		query += ` WHERE hidden = 0`
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
		var name, slug string
		var sort, hidden int
		if err := rows.Scan(&id, &name, &slug, &sort, &hidden); err != nil {
			return nil, err
		}
		list = append(list, gin.H{"id": id, "name": name, "slug": slug, "sort": sort, "hidden": hidden == 1})
	}
	return list, rows.Err()
}

func listSiteDocArticles(db *sql.DB, publicOnly bool) ([]gin.H, error) {
	bodySelect := ""
	if !publicOnly {
		bodySelect = ", a.body"
	}
	query := `SELECT a.id, a.category_id, c.name, a.title, a.slug, a.summary, a.sort, a.hidden` + bodySelect + `
		FROM site_doc_articles a JOIN site_doc_categories c ON c.id = a.category_id`
	if publicOnly {
		query += ` WHERE a.hidden = 0 AND c.hidden = 0`
	}
	query += ` ORDER BY c.sort ASC, a.sort ASC, a.id ASC`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]gin.H, 0)
	for rows.Next() {
		var id, categoryID int64
		var category, title, slug, summary string
		var body sql.NullString
		var sort, hidden int
		var err error
		if publicOnly {
			err = rows.Scan(&id, &categoryID, &category, &title, &slug, &summary, &sort, &hidden)
		} else {
			err = rows.Scan(&id, &categoryID, &category, &title, &slug, &summary, &sort, &hidden, &body)
		}
		if err != nil {
			return nil, err
		}
		item := gin.H{
			"id": id, "categoryId": categoryID, "category": category, "title": title,
			"slug": slug, "summary": summary, "sort": sort, "hidden": hidden == 1,
		}
		if !publicOnly {
			item["body"] = body.String
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

func loadSiteDocArticle(db *sql.DB, slug string, publicOnly bool) (gin.H, error) {
	query := `SELECT a.id, a.category_id, c.name, a.title, a.slug, a.summary, a.body, a.sort
		FROM site_doc_articles a JOIN site_doc_categories c ON c.id = a.category_id
		WHERE a.slug = ?`
	if publicOnly {
		query += ` AND a.hidden = 0 AND c.hidden = 0`
	}
	var id, categoryID int64
	var category, title, articleSlug, summary, body string
	var sort int
	err := db.QueryRow(query, slug).Scan(&id, &categoryID, &category, &title, &articleSlug, &summary, &body, &sort)
	if err != nil {
		return nil, err
	}
	return gin.H{
		"id": id, "categoryId": categoryID, "category": category, "title": title,
		"slug": articleSlug, "summary": summary, "body": body, "sort": sort,
	}, nil
}

func siteDocNeighbors(db *sql.DB, article gin.H) (gin.H, gin.H) {
	categoryID, _ := article["categoryId"].(int64)
	sortValue, _ := article["sort"].(int)
	id, _ := article["id"].(int64)
	prev := querySiteDocNeighbor(db, `SELECT slug, title FROM site_doc_articles a
		JOIN site_doc_categories c ON c.id = a.category_id
		WHERE a.category_id = ? AND a.hidden = 0 AND c.hidden = 0
		  AND (a.sort < ? OR (a.sort = ? AND a.id < ?))
		ORDER BY a.sort DESC, a.id DESC LIMIT 1`, categoryID, sortValue, sortValue, id)
	next := querySiteDocNeighbor(db, `SELECT slug, title FROM site_doc_articles a
		JOIN site_doc_categories c ON c.id = a.category_id
		WHERE a.category_id = ? AND a.hidden = 0 AND c.hidden = 0
		  AND (a.sort > ? OR (a.sort = ? AND a.id > ?))
		ORDER BY a.sort ASC, a.id ASC LIMIT 1`, categoryID, sortValue, sortValue, id)
	return prev, next
}

func querySiteDocNeighbor(db *sql.DB, query string, args ...any) gin.H {
	var slug, title string
	if err := db.QueryRow(query, args...).Scan(&slug, &title); err != nil {
		return nil
	}
	return gin.H{"slug": slug, "title": title}
}

func ensureDefaultSharedFeatures(db *sql.DB) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM site_compare_features`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	for index, text := range siteDefaultSharedFeatures {
		if _, err := db.Exec(`INSERT INTO site_compare_features (body, sort, hidden) VALUES (?, ?, 0)`, text, (index+1)*10); err != nil {
			return err
		}
	}
	return nil
}

// SiteComparePublic 只返回「两个版本都具备」的未隐藏条目。差别行由前端 commercialCompareRows 提供。
func SiteComparePublic(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	list, err := listSiteCompare(db, true)
	if err != nil {
		siteFail(c, 500, "读取对比说明失败")
		return
	}
	siteOK(c, gin.H{"shared": list})
}

func SiteCompareAdminList(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	list, err := listSiteCompare(db, false)
	if err != nil {
		siteFail(c, 500, "读取对比说明失败")
		return
	}
	siteOK(c, gin.H{"list": list})
}

func listSiteCompare(db *sql.DB, publicOnly bool) ([]gin.H, error) {
	query := `SELECT id, body, sort, hidden FROM site_compare_features`
	if publicOnly {
		query += ` WHERE hidden = 0`
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
		var body string
		var sort, hidden int
		if err := rows.Scan(&id, &body, &sort, &hidden); err != nil {
			return nil, err
		}
		list = append(list, gin.H{"id": id, "body": body, "sort": sort, "hidden": hidden == 1})
	}
	return list, rows.Err()
}

func SiteCompareAdminCreate(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if !readSiteJSON(c, &req) {
		return
	}
	body := trimSiteText(req.Body, 200)
	if body == "" {
		siteFail(c, 400, "请填写能力说明")
		return
	}
	var maxSort int
	_ = db.QueryRow(`SELECT COALESCE(MAX(sort), 0) FROM site_compare_features`).Scan(&maxSort)
	result, err := db.Exec(`INSERT INTO site_compare_features (body, sort, hidden) VALUES (?, ?, 0)`, body, maxSort+10)
	if err != nil {
		siteFail(c, 500, "保存失败")
		return
	}
	id, _ := result.LastInsertId()
	siteOK(c, gin.H{"id": id})
}

func SiteCompareAdminUpdate(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	id, ok := siteID(c)
	if !ok {
		return
	}
	var req struct {
		Body   string `json:"body"`
		Sort   *int   `json:"sort"`
		Hidden *bool  `json:"hidden"`
	}
	if !readSiteJSON(c, &req) {
		return
	}
	body := trimSiteText(req.Body, 200)
	if body == "" {
		siteFail(c, 400, "请填写能力说明")
		return
	}
	sort := 0
	hidden := 0
	if err := db.QueryRow(`SELECT sort, hidden FROM site_compare_features WHERE id = ?`, id).Scan(&sort, &hidden); err != nil {
		siteFail(c, 404, "条目不存在")
		return
	}
	if req.Sort != nil {
		sort = *req.Sort
	}
	if req.Hidden != nil {
		if *req.Hidden {
			hidden = 1
		} else {
			hidden = 0
		}
	}
	if _, err := db.Exec(`UPDATE site_compare_features SET body = ?, sort = ?, hidden = ? WHERE id = ?`, body, sort, hidden, id); err != nil {
		siteFail(c, 500, "保存失败")
		return
	}
	siteOK(c, gin.H{"id": id})
}

func SiteCompareAdminDelete(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	id, ok := siteID(c)
	if !ok {
		return
	}
	result, err := db.Exec(`DELETE FROM site_compare_features WHERE id = ?`, id)
	if err != nil {
		siteFail(c, 500, "删除失败")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		siteFail(c, 404, "条目不存在")
		return
	}
	siteOK(c, gin.H{"id": id})
}

// SiteChangelogPublic 按版本返回未隐藏的更新日志。日期是 DATE，调用方按北京时间的发布日展示。
func SiteChangelogPublic(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	list, err := listSiteChangelog(db, true)
	if err != nil {
		siteFail(c, 500, "读取更新日志失败")
		return
	}
	siteOK(c, gin.H{"list": groupSiteChangelog(list)})
}

func SiteChangelogAdminList(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	list, err := listSiteChangelog(db, false)
	if err != nil {
		siteFail(c, 500, "读取更新日志失败")
		return
	}
	siteOK(c, gin.H{"list": list})
}

func SiteChangelogAdminCreate(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	req, ok := readChangelogReq(c)
	if !ok {
		return
	}
	fingerprint := siteChangelogFingerprint(req.Version, req.Body, fmt.Sprintf("manual-%d", time.Now().UnixNano()))
	result, err := db.Exec(`INSERT INTO site_changelog_entries
		(version, released_on, tag, body, hidden, edited, source, fingerprint, sort)
		VALUES (?, ?, ?, ?, 0, 1, ?, ?, ?)`,
		req.Version, nullableDate(req.ReleasedOn), req.Tag, req.Body, siteChangelogManual, fingerprint, req.Sort)
	if err != nil {
		siteFail(c, 500, "保存更新日志失败")
		return
	}
	id, _ := result.LastInsertId()
	siteOK(c, gin.H{"id": id})
}

func SiteChangelogAdminUpdate(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	id, ok := siteID(c)
	if !ok {
		return
	}
	req, ok := readChangelogReq(c)
	if !ok {
		return
	}
	hidden := 0
	if req.Hidden {
		hidden = 1
	}
	result, err := db.Exec(`UPDATE site_changelog_entries
		SET version = ?, released_on = ?, tag = ?, body = ?, hidden = ?, edited = 1, sort = ?
		WHERE id = ?`,
		req.Version, nullableDate(req.ReleasedOn), req.Tag, req.Body, hidden, req.Sort, id)
	if err != nil {
		siteFail(c, 500, "保存更新日志失败")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		siteFail(c, 404, "条目不存在")
		return
	}
	siteOK(c, gin.H{"id": id})
}

// SiteChangelogAdminDelete 删除手动条目。发版自动生成的条目记入跳过表，避免下次启动又导回来。
func SiteChangelogAdminDelete(c *gin.Context) {
	db, ok := sitePagesDB(c)
	if !ok {
		return
	}
	id, ok := siteID(c)
	if !ok {
		return
	}
	var fingerprint, source string
	if err := db.QueryRow(`SELECT fingerprint, source FROM site_changelog_entries WHERE id = ?`, id).Scan(&fingerprint, &source); err != nil {
		siteFail(c, 404, "条目不存在")
		return
	}
	if source == siteChangelogRelease {
		if _, err := db.Exec(`INSERT IGNORE INTO site_changelog_skips (fingerprint) VALUES (?)`, fingerprint); err != nil {
			siteFail(c, 500, "删除更新日志失败")
			return
		}
	}
	if _, err := db.Exec(`DELETE FROM site_changelog_entries WHERE id = ?`, id); err != nil {
		siteFail(c, 500, "删除更新日志失败")
		return
	}
	siteOK(c, gin.H{"id": id})
}

type changelogWrite struct {
	Version    string
	ReleasedOn string
	Tag        string
	Body       string
	Sort       int
	Hidden     bool
}

func readChangelogReq(c *gin.Context) (changelogWrite, bool) {
	var req struct {
		Version    string `json:"version"`
		ReleasedOn string `json:"releasedOn"`
		Tag        string `json:"tag"`
		Body       string `json:"body"`
		Sort       int    `json:"sort"`
		Hidden     bool   `json:"hidden"`
	}
	if !readSiteJSON(c, &req) {
		return changelogWrite{}, false
	}
	version := strings.TrimPrefix(strings.TrimSpace(req.Version), "v")
	tag := strings.TrimSpace(req.Tag)
	body := trimSiteText(req.Body, 2000)
	released := strings.TrimSpace(req.ReleasedOn)
	if !siteVersionPattern.MatchString(version) || !validSiteChangelogTag(tag) || body == "" {
		siteFail(c, 400, "请填写版本号、类型和内容")
		return changelogWrite{}, false
	}
	if released != "" {
		if _, err := time.Parse("2006-01-02", released); err != nil {
			siteFail(c, 400, "发布日期格式应为 YYYY-MM-DD")
			return changelogWrite{}, false
		}
	}
	return changelogWrite{Version: version, ReleasedOn: released, Tag: tag, Body: body, Sort: req.Sort, Hidden: req.Hidden}, true
}

func validSiteChangelogTag(tag string) bool {
	return tag == siteTagAdded || tag == siteTagImproved || tag == siteTagFixed
}

func listSiteChangelog(db *sql.DB, publicOnly bool) ([]gin.H, error) {
	query := `SELECT id, version, IFNULL(DATE_FORMAT(released_on, '%Y-%m-%d'), ''), tag, body, hidden, source, sort
		FROM site_changelog_entries`
	if publicOnly {
		query += ` WHERE hidden = 0`
	}
	query += ` ORDER BY released_on IS NULL, released_on DESC, version DESC, sort ASC, id ASC`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]gin.H, 0)
	for rows.Next() {
		var id int64
		var version, released, tag, body, source string
		var hidden, sort int
		if err := rows.Scan(&id, &version, &released, &tag, &body, &hidden, &source, &sort); err != nil {
			return nil, err
		}
		list = append(list, gin.H{
			"id": id, "version": version, "releasedOn": released, "tag": tag,
			"body": body, "hidden": hidden == 1, "source": source, "sort": sort,
		})
	}
	return list, rows.Err()
}

func groupSiteChangelog(rows []gin.H) []gin.H {
	order := make([]string, 0)
	grouped := map[string]gin.H{}
	for _, row := range rows {
		version, _ := row["version"].(string)
		item, ok := grouped[version]
		if !ok {
			order = append(order, version)
			item = gin.H{"version": version, "releasedOn": row["releasedOn"], "items": []gin.H{}}
			grouped[version] = item
		}
		items := item["items"].([]gin.H)
		item["items"] = append(items, gin.H{"id": row["id"], "tag": row["tag"], "body": row["body"]})
	}
	list := make([]gin.H, 0, len(order))
	for _, version := range order {
		list = append(list, grouped[version])
	}
	return list
}

// scrubLeakedRepositoryNotes 删掉没改过、又提到代码托管站的更新日志。
// 改过的条目保留。仓库全名不写在这里，避免打进发行二进制。
func scrubLeakedRepositoryNotes(db *sql.DB) error {
	_, err := db.Exec(`DELETE FROM site_changelog_entries
		WHERE edited = 0 AND (
			body LIKE '%github.com%'
			OR body LIKE '%githubusercontent%'
			OR (body LIKE '%github%' AND body LIKE '%auth-pro%')
		)`)
	if err != nil {
		return fmt.Errorf("scrub changelog repository notes: %w", err)
	}
	return nil
}

func importSiteReleaseNotes(db *sql.DB, root string) error {
	if root == "" || db == nil {
		return nil
	}
	dates := changelogDatesFromFile(filepath.Join(filepath.Dir(root), "CHANGELOG.md"))
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := siteReleaseFilePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		version := match[1]
		payload, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			continue
		}
		notes := splitReleaseNoteParagraphs(string(payload))
		if err := syncReleaseChangelog(db, version, dates[version], notes); err != nil {
			return err
		}
	}
	return nil
}

// releaseNotesManaged 为真时，这一版的公开更新日志只认仓库里的发布说明。
func releaseNotesManaged(version string) bool {
	root := findSiteDocsRoot()
	if root == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(root, "release-notes-"+version+".txt"))
	return err == nil && !info.IsDir()
}

// releaseEntryDropped 判断没改过的发布说明是否已经不在当前文件里。
// 管理员改过的条目保留。
func releaseEntryDropped(edited int, source, fingerprint string, keep map[string]struct{}) bool {
	if edited != 0 || source != siteChangelogRelease {
		return false
	}
	_, ok := keep[fingerprint]
	return !ok
}

// syncReleaseChangelog 用当前发布说明覆盖没改过的旧段落，避免同一版本留下多组说明。
func syncReleaseChangelog(db *sql.DB, version, releasedOn string, notes []string) error {
	keep := map[string]struct{}{}
	for index, note := range notes {
		body := trimSiteText(strings.TrimSpace(note), 2000)
		if body == "" {
			continue
		}
		tag := classifyReleaseNote(body)
		if !validSiteChangelogTag(tag) {
			continue
		}
		keep[siteChangelogFingerprint(version, body, "")] = struct{}{}
		if err := insertReleaseChangelog(db, version, releasedOn, tag, body, index); err != nil {
			return err
		}
	}
	rows, err := db.Query(`SELECT id, fingerprint, edited FROM site_changelog_entries WHERE version = ? AND source = ?`, version, siteChangelogRelease)
	if err != nil {
		return err
	}
	defer rows.Close()
	drop := make([]int64, 0)
	for rows.Next() {
		var id int64
		var fingerprint string
		var edited int
		if err := rows.Scan(&id, &fingerprint, &edited); err != nil {
			return err
		}
		if releaseEntryDropped(edited, siteChangelogRelease, fingerprint, keep) {
			drop = append(drop, id)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range drop {
		if _, err := db.Exec(`DELETE FROM site_changelog_entries WHERE id = ? AND edited = 0`, id); err != nil {
			return err
		}
	}
	return nil
}

// rememberSiteChangelogFromReleases 把在线更新读到的发布说明写进官网更新日志。
// 日期换成北京时间的日历日。没有数据库时直接返回，不影响更新检查。
func rememberSiteChangelogFromReleases(releases []onlineUpdateRelease) {
	db, err := config.DB()
	if err != nil {
		return
	}
	if err := EnsureSitePagesSchema(db); err != nil {
		return
	}
	dates := map[string]string{}
	if root := findSiteDocsRoot(); root != "" {
		dates = changelogDatesFromFile(filepath.Join(filepath.Dir(root), "CHANGELOG.md"))
	}
	for _, release := range releases {
		version := strings.TrimPrefix(strings.TrimSpace(release.Version), "v")
		if !siteVersionPattern.MatchString(version) {
			continue
		}
		date := dates[version]
		if date == "" {
			date = shanghaiReleaseDate(release.ReleasedAt)
		}
		if releaseNotesManaged(version) {
			continue
		}
		for index, note := range release.Notes {
			_ = insertReleaseChangelog(db, version, date, classifyReleaseNote(note), note, index)
		}
	}
}

func insertReleaseChangelog(db *sql.DB, version, releasedOn, tag, body string, sort int) error {
	body = trimSiteText(strings.TrimSpace(body), 2000)
	if body == "" || !validSiteChangelogTag(tag) {
		return nil
	}
	fingerprint := siteChangelogFingerprint(version, body, "")
	var skipped int
	if err := db.QueryRow(`SELECT COUNT(*) FROM site_changelog_skips WHERE fingerprint = ?`, fingerprint).Scan(&skipped); err != nil {
		return err
	}
	if skipped > 0 {
		return nil
	}
	_, err := db.Exec(`INSERT IGNORE INTO site_changelog_entries
		(version, released_on, tag, body, hidden, edited, source, fingerprint, sort)
		VALUES (?, ?, ?, ?, 0, 0, ?, ?, ?)`,
		version, nullableDate(releasedOn), tag, body, siteChangelogRelease, fingerprint, (sort+1)*10)
	if err != nil {
		return err
	}
	if releasedOn == "" {
		return nil
	}
	_, err = db.Exec(`UPDATE site_changelog_entries
		SET released_on = ?
		WHERE fingerprint = ? AND edited = 0 AND released_on IS NULL`, releasedOn, fingerprint)
	return err
}

func nullableDate(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func siteChangelogFingerprint(version, body, extra string) string {
	normalized := strings.Join(strings.Fields(body), " ")
	sum := sha256.Sum256([]byte(version + "\n" + normalized + "\n" + extra))
	return hex.EncodeToString(sum[:])
}

func splitReleaseNoteParagraphs(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	chunks := strings.Split(text, "\n\n")
	notes := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" || isReleaseNoteTitle(chunk) {
			continue
		}
		notes = append(notes, expandReleaseNoteBullets(chunk)...)
	}
	return notes
}

// expandReleaseNoteBullets 把「- 一句话」拆成独立条目。没有列表符时保持原段。
func expandReleaseNoteBullets(chunk string) []string {
	lines := strings.Split(chunk, "\n")
	bullets := make([]string, 0, len(lines))
	plain := make([]string, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			item := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* "))
			if item != "" {
				bullets = append(bullets, item)
			}
			continue
		}
		plain = append(plain, line)
	}
	if len(bullets) == 0 {
		return []string{strings.TrimSpace(chunk)}
	}
	return append(plain, bullets...)
}

func isReleaseNoteTitle(text string) bool {
	line := strings.TrimSpace(strings.Split(text, "\n")[0])
	lower := strings.ToLower(line)
	return strings.HasPrefix(lower, "auth-pro ") && siteVersionPattern.MatchString(strings.TrimPrefix(strings.TrimPrefix(lower, "auth-pro "), "v"))
}

func classifyReleaseNote(text string) string {
	switch {
	case strings.Contains(text, "修复") || strings.Contains(text, "修正") || strings.Contains(text, "缺陷"):
		return siteTagFixed
	case strings.Contains(text, "优化") || strings.Contains(text, "改进") || strings.Contains(text, "提升") || strings.Contains(text, "调整"):
		return siteTagImproved
	default:
		return siteTagAdded
	}
}

func changelogDatesFromFile(path string) map[string]string {
	payload, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	return changelogDatesFromMarkdown(string(payload))
}

func changelogDatesFromMarkdown(text string) map[string]string {
	dates := map[string]string{}
	for _, match := range siteChangelogHeadingPattern.FindAllStringSubmatch(text, -1) {
		if len(match) == 3 {
			dates[match[1]] = match[2]
		}
	}
	return dates
}

func shanghaiReleaseDate(rfc3339 string) string {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(rfc3339))
	if err != nil {
		return ""
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("CST", 8*3600)
	}
	return parsed.In(location).Format("2006-01-02")
}
