// 每个应用只绑一个私有仓库。五个位置是发布标签前缀，不上传空文件。
// 令牌只从存储管理已加密的密钥里取。本站在线更新不读这张表。

package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"auto_pro/config"
)

const (
	appRepoPrefixClient       = "client/"
	appRepoPrefixPluginFree   = "plugins/free/"
	appRepoPrefixPluginPaid   = "plugins/paid/"
	appRepoPrefixTemplateFree = "templates/free/"
	appRepoPrefixTemplatePaid = "templates/paid/"
	appRepoStatusReady        = "ready"
	appRepoStatusDegraded     = "degraded"
	appRepoTargetType         = "app_repo"
	appRepoUnboundText        = "请先为该应用绑定仓库"
	appRepoTokenMissingText   = "还没有可用的令牌。请到存储管理添加。也可以先创建应用，稍后再绑定。"
	appRepoTokenInvalidText   = "GitHub 令牌无效或已过期。请到存储管理更新令牌。"
	appRepoPermissionText     = "权限不足，不能创建仓库。请换一个已有的私有仓库，或到存储管理更新令牌。"
	appRepoBusyText           = "这个应用正在处理仓库，请等当前操作结束。"
	appRepoImportBusyText     = "这个应用正在导入或上传，请等它结束后再更换或解除绑定。"
	appRepoNamePatternText    = "仓库名只允许字母、数字、点、下划线和连字符"
	appRepoFormatText         = "请填写仓库，格式为 所有者/仓库"
	appRepoOwnerText          = "所有者只能是令牌对应的账号或其所属组织"
	appRepoPublicText         = "该仓库已存在，但是公开的，不能存放安装包"
	appRepoMissingText        = "找不到这个仓库，或令牌看不到它"
	appRepoTakenText          = "这个名字已经有一个私有仓库。请改用「绑定已有仓库」，或换一个名字。"
	appRepoTimeoutText        = "连接超时，请稍后再试"
	appRepoTimeoutSavedText   = "连接超时。应用已保存，仓库还没有绑上。"
	appRepoNoAppsText         = "还没有应用。请先创建应用。"
	appRepoEmptyFilesText     = "这个仓库里还没有安装包。"
	appRepoListFailText       = "列不出远程文件。请稍后再试。"
	appRepoNoReleaseText      = "这个位置还没有可导入的发布。"
	appRepoAppKeyMissingText  = "安装包清单里没有应用标识"
	appRepoAppKeyMismatchText = "这个安装包属于另一个应用"
	appRepoVersionSameText    = "已经有这个版本"
	appRepoVersionOlderText   = "版本不比当前已发布的新"
	// 只在官网这一次迁移里读取原来的客户交付仓库。导入和上传不再使用它。
	officialLegacyClientRepo = "maizll/auth-pro-client"
)

var (
	errAppRepoUnbound = errors.New(appRepoUnboundText)
	errAppRepoBusy    = errors.New(appRepoBusyText)

	appRepoBusy sync.Map

	appRepoMemory struct {
		mu   sync.Mutex
		on   bool
		rows map[int64]appRepoRow
		reqs map[string]int64
	}
)

type appRepoRow struct {
	AppID      int64
	AppKey     string
	AppName    string
	LocationID string
	Owner      string
	Repo       string
	Status     string
	Private    bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type appRepoPlan struct {
	AppID  int64
	Owner  string
	Repo   string
	Tag    string
	Prefix string
}

func appRepoPrefixes() []string {
	return []string{
		appRepoPrefixClient,
		appRepoPrefixPluginFree,
		appRepoPrefixPluginPaid,
		appRepoPrefixTemplateFree,
		appRepoPrefixTemplatePaid,
	}
}

func useAppRepoMemoryForTest(t interface{ Cleanup(func()) }) {
	appRepoMemory.mu.Lock()
	appRepoMemory.on = true
	appRepoMemory.rows = map[int64]appRepoRow{}
	appRepoMemory.reqs = map[string]int64{}
	appRepoMemory.mu.Unlock()
	t.Cleanup(func() {
		appRepoMemory.mu.Lock()
		appRepoMemory.on = false
		appRepoMemory.rows = nil
		appRepoMemory.reqs = nil
		appRepoMemory.mu.Unlock()
	})
}

func ensureAppRepoSchema(db *sql.DB) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS app_repo_bindings (
		app_id BIGINT NOT NULL,
		location_id VARCHAR(64) NOT NULL DEFAULT '',
		owner VARCHAR(64) NOT NULL,
		repo VARCHAR(120) NOT NULL,
		private_repo TINYINT(1) NOT NULL DEFAULT 1,
		status VARCHAR(20) NOT NULL DEFAULT 'ready',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (app_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='应用私有仓库绑定'`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS app_create_requests (
		request_id VARCHAR(80) NOT NULL,
		app_id BIGINT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (request_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='创建应用的幂等号'`)
	return err
}

func migrateAppRepoBindings(db *sql.DB) error {
	if err := ensureAppRepoSchema(db); err != nil {
		return err
	}
	// 官网升级时把已有仓库迁入「授权系统」。失败不改绑定，下次启动再试。
	if err := migrateOfficialAppRepo(context.Background(), db); err != nil {
		_ = currentSourceStationStore().AppendAudit(sourceAuditEntry{
			ActorType: "system", Action: "migrate_failed", TargetType: appRepoTargetType,
			TargetID: productUpdateAppKey, Detail: "官网仓库迁移没有完成：" + err.Error(),
		})
	}
	return nil
}

func lockAppRepo(appID int64) bool {
	_, loaded := appRepoBusy.LoadOrStore(appID, struct{}{})
	return !loaded
}

func unlockAppRepo(appID int64) {
	appRepoBusy.Delete(appID)
}

func appRepoBusyNow(appID int64) bool {
	_, ok := appRepoBusy.Load(appID)
	return ok
}

func splitOwnerRepo(raw string) (string, string, error) {
	raw = strings.Trim(strings.TrimSpace(raw), "/")
	parts := strings.Split(raw, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New(appRepoFormatText)
	}
	if !validGitHubPaidName(parts[0]) || !validGitHubPaidName(parts[1]) || len(parts[0]) > 39 || len(parts[1]) > 100 {
		return "", "", errors.New(appRepoNamePatternText)
	}
	return parts[0], parts[1], nil
}

func suggestAppRepoName(appName, appKey, owner string) string {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		owner = "owner"
	}
	slug := repoSlug(appName)
	if slug == "" {
		short := appKey
		if i := strings.LastIndex(appKey, "_"); i >= 0 && i+1 < len(appKey) {
			short = appKey[i+1:]
		}
		if len(short) > 8 {
			short = short[:8]
		}
		if short == "" {
			short = "app"
		}
		slug = "app-" + strings.ToLower(short)
	}
	return owner + "/" + slug
}

func repoSlug(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.TrimSpace(name) {
		switch {
		case r == ' ' || r == '_' || r == '-':
			if b.Len() > 0 && !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.':
			if r <= unicode.MaxASCII {
				b.WriteRune(unicode.ToLower(r))
				prevDash = false
			}
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" || !validGitHubPaidName(out) {
		return ""
	}
	return out
}

func primaryGitHubBindingLocation() (storageLocation, string, bool) {
	blob, err := loadStorageBlob()
	if err != nil {
		return storageLocation{}, "", false
	}
	for _, loc := range enabledStorageLocations(blob.Locations) {
		if loc.Kind != packageStorageGitHub {
			continue
		}
		secret, openErr := locationSecret(loc)
		if openErr != nil || strings.TrimSpace(secret) == "" {
			continue
		}
		return loc, secret, true
	}
	owner, repo, token, err := loadGitHubPaidRepo()
	if err != nil || token == "" || owner == "" || repo == "" {
		return storageLocation{}, "", false
	}
	return storageLocation{ID: "github-paid", Kind: packageStorageGitHub, Owner: owner, Repo: repo, Name: "收费仓库"}, token, true
}

func loadAppRepo(appID int64) (appRepoRow, bool, error) {
	appRepoMemory.mu.Lock()
	if appRepoMemory.on {
		row, ok := appRepoMemory.rows[appID]
		appRepoMemory.mu.Unlock()
		return row, ok, nil
	}
	appRepoMemory.mu.Unlock()
	db, err := config.DB()
	if err != nil || db == nil {
		return appRepoRow{}, false, nil
	}
	if err := ensureAppRepoSchema(db); err != nil {
		return appRepoRow{}, false, err
	}
	var row appRepoRow
	var private int
	err = db.QueryRow(`SELECT app_id, location_id, owner, repo, private_repo, status FROM app_repo_bindings WHERE app_id = ?`, appID).
		Scan(&row.AppID, &row.LocationID, &row.Owner, &row.Repo, &private, &row.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return appRepoRow{}, false, nil
	}
	if err != nil {
		return appRepoRow{}, false, err
	}
	row.Private = private == 1
	return row, true, nil
}

func saveAppRepo(row appRepoRow) error {
	row.Status = strings.TrimSpace(row.Status)
	if row.Status == "" {
		row.Status = appRepoStatusReady
	}
	appRepoMemory.mu.Lock()
	if appRepoMemory.on {
		if appRepoMemory.rows == nil {
			appRepoMemory.rows = map[int64]appRepoRow{}
		}
		appRepoMemory.rows[row.AppID] = row
		appRepoMemory.mu.Unlock()
		return nil
	}
	appRepoMemory.mu.Unlock()
	db, err := config.DB()
	if err != nil || db == nil {
		return errors.New("数据库连接失败")
	}
	if err := ensureAppRepoSchema(db); err != nil {
		return err
	}
	private := 0
	if row.Private {
		private = 1
	}
	_, err = db.Exec(`INSERT INTO app_repo_bindings (app_id, location_id, owner, repo, private_repo, status)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE location_id = VALUES(location_id), owner = VALUES(owner), repo = VALUES(repo),
		private_repo = VALUES(private_repo), status = VALUES(status), updated_at = CURRENT_TIMESTAMP`,
		row.AppID, row.LocationID, row.Owner, row.Repo, private, row.Status)
	return err
}

func deleteAppRepo(appID int64) error {
	appRepoMemory.mu.Lock()
	if appRepoMemory.on {
		delete(appRepoMemory.rows, appID)
		appRepoMemory.mu.Unlock()
		return nil
	}
	appRepoMemory.mu.Unlock()
	db, err := config.DB()
	if err != nil || db == nil {
		return err
	}
	_, err = db.Exec(`DELETE FROM app_repo_bindings WHERE app_id = ?`, appID)
	return err
}

func repoTakenByOther(appID int64, owner, repo string) (string, bool) {
	appRepoMemory.mu.Lock()
	if appRepoMemory.on {
		for id, row := range appRepoMemory.rows {
			if id != appID && strings.EqualFold(row.Owner, owner) && strings.EqualFold(row.Repo, repo) {
				name := row.AppName
				if name == "" {
					name = row.AppKey
				}
				appRepoMemory.mu.Unlock()
				return name, true
			}
		}
		appRepoMemory.mu.Unlock()
		return "", false
	}
	appRepoMemory.mu.Unlock()
	db, err := config.DB()
	if err != nil || db == nil {
		return "", false
	}
	var name string
	err = db.QueryRow(`SELECT app_name FROM app_repo_bindings b JOIN apps a ON a.id = b.app_id
		WHERE b.app_id <> ? AND b.owner = ? AND b.repo = ? LIMIT 1`, appID, owner, repo).Scan(&name)
	if err != nil {
		return "", false
	}
	return name, true
}

func catalogAppID(kind, itemID string) int64 {
	store := currentSourceStationStore()
	switch kind {
	case sourceKindTemplate:
		item, err := store.GetTemplate(itemID)
		if err != nil {
			return 0
		}
		return item.AppID
	default:
		item, err := store.GetPlugin(itemID)
		if err != nil {
			return 0
		}
		return item.AppID
	}
}

func appRepoPrefixFor(kind string, paid bool) string {
	if kind == "app" || kind == "client" {
		return appRepoPrefixClient
	}
	if kind == sourceKindTemplate {
		if paid {
			return appRepoPrefixTemplatePaid
		}
		return appRepoPrefixTemplateFree
	}
	if paid {
		return appRepoPrefixPluginPaid
	}
	return appRepoPrefixPluginFree
}

func appRepoPaidPlan(kind, itemID, version string) (appRepoPlan, bool, error) {
	appID := catalogAppID(kind, itemID)
	if appID <= 0 {
		return appRepoPlan{}, false, nil
	}
	row, ok, err := loadAppRepo(appID)
	if err != nil {
		return appRepoPlan{}, true, err
	}
	if !ok {
		return appRepoPlan{}, true, errAppRepoUnbound
	}
	prefix := appRepoPrefixFor(kind, true)
	return appRepoPlan{
		AppID: appID, Owner: row.Owner, Repo: row.Repo, Prefix: prefix,
		Tag: prefix + strings.TrimSpace(itemID) + "-" + strings.TrimSpace(version),
	}, true, nil
}

func appRepoAudit(action, appKey, detail string) {
	if appKey == "" {
		appKey = "app"
	}
	_ = currentSourceStationStore().AppendAudit(sourceAuditEntry{
		ActorType: "admin", Action: action, TargetType: appRepoTargetType, TargetID: appKey, Detail: detail,
	})
}

func markAppRepoDegraded(appID int64) {
	row, ok, err := loadAppRepo(appID)
	if err != nil || !ok {
		return
	}
	row.Status = appRepoStatusDegraded
	_ = saveAppRepo(row)
}

func tokenFailure(err error) error {
	if err == nil {
		return nil
	}
	text := err.Error()
	if errors.Is(err, errGitHubPaidTokenInvalid) || strings.Contains(text, "无效") || strings.Contains(text, "过期") || strings.Contains(text, "401") {
		return errors.New(appRepoTokenInvalidText)
	}
	if strings.Contains(text, "权限不足") || strings.Contains(text, "403") || strings.Contains(text, "Forbidden") {
		return errors.New(appRepoPermissionText)
	}
	if strings.Contains(strings.ToLower(text), "timeout") || strings.Contains(text, "超时") || strings.Contains(text, "无法连接") {
		return errors.New(appRepoTimeoutText)
	}
	return err
}

func bindAppRepo(ctx context.Context, appID int64, appKey, appName, action, rawRepo string, create bool) (appRepoRow, error) {
	if !lockAppRepo(appID) {
		return appRepoRow{}, errAppRepoBusy
	}
	defer unlockAppRepo(appID)
	loc, token, ok := primaryGitHubBindingLocation()
	if !ok {
		return appRepoRow{}, errors.New(appRepoTokenMissingText)
	}
	owner, repo, err := splitOwnerRepo(rawRepo)
	if err != nil {
		return appRepoRow{}, err
	}
	if other, taken := repoTakenByOther(appID, owner, repo); taken {
		name := other
		if name == "" {
			name = "另一个应用"
		}
		return appRepoRow{}, fmt.Errorf("这个仓库已经绑给应用「%s」", name)
	}
	identity, idErr := fetchGitHubPaidIdentity(ctx, token)
	if idErr != nil {
		markAppRepoDegraded(appID)
		return appRepoRow{}, tokenFailure(idErr)
	}
	allowed := identity.Login == owner
	if !allowed {
		for _, choice := range identity.Owners {
			if choice.Login == owner {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		return appRepoRow{}, errors.New(appRepoOwnerText)
	}
	state, stateErr := lookupGitHubPaidRepoState(ctx, token, owner, repo)
	if stateErr != nil {
		return appRepoRow{}, tokenFailure(stateErr)
	}
	switch state {
	case githubPaidRepoPublic:
		return appRepoRow{}, errors.New(appRepoPublicText)
	case githubPaidRepoMissing:
		if !create {
			return appRepoRow{}, errors.New(appRepoMissingText)
		}
		if err := createGitHubPaidRepo(ctx, token, owner, repo, identity); err != nil {
			text := tokenFailure(err).Error()
			if strings.Contains(err.Error(), "422") || strings.Contains(err.Error(), "占用") {
				return appRepoRow{}, errors.New("仓库名已被占用，请换一个名字")
			}
			if strings.Contains(text, "权限") {
				return appRepoRow{}, errors.New(appRepoPermissionText)
			}
			return appRepoRow{}, errors.New(text)
		}
		again, againErr := lookupGitHubPaidRepoState(ctx, token, owner, repo)
		if againErr != nil {
			return appRepoRow{}, errors.New(appRepoTimeoutSavedText)
		}
		if again != githubPaidRepoPrivate {
			return appRepoRow{}, errors.New(appRepoTakenText)
		}
	case githubPaidRepoPrivate:
		if create {
			return appRepoRow{}, errors.New(appRepoTakenText)
		}
	}
	row := appRepoRow{
		AppID: appID, AppKey: appKey, AppName: appName, LocationID: loc.ID,
		Owner: owner, Repo: repo, Status: appRepoStatusReady, Private: true,
	}
	if err := saveAppRepo(row); err != nil {
		return appRepoRow{}, errors.New("仓库已在远程，绑定没有写上。可以再次绑定同一个名字。")
	}
	appRepoAudit("bind", appKey, "已绑定 "+owner+"/"+repo)
	return row, nil
}

func appRepoHealthRows(ctx context.Context) []storageHealthRow {
	rows := make([]storageHealthRow, 0)
	db, err := config.DB()
	if err != nil || db == nil {
		return rows
	}
	if err := ensureAppRepoSchema(db); err != nil {
		return rows
	}
	list, err := db.Query(`SELECT b.app_id, a.app_name, b.owner, b.repo FROM app_repo_bindings b JOIN apps a ON a.id = b.app_id`)
	if err != nil {
		return rows
	}
	defer list.Close()
	for list.Next() {
		var appID int64
		var name, owner, repo string
		if err := list.Scan(&appID, &name, &owner, &repo); err != nil {
			continue
		}
		_, token, ok := primaryGitHubBindingLocation()
		if !ok {
			markAppRepoDegraded(appID)
			rows = append(rows, storageHealthRow{
				Level: "problem", LocationName: "仓库绑定", Target: name, Message: name + "：令牌已失效。绑定仍保留，安装包不会自动下架。",
			})
			continue
		}
		state, stateErr := lookupGitHubPaidRepoState(ctx, token, owner, repo)
		if stateErr != nil || state != githubPaidRepoPrivate {
			markAppRepoDegraded(appID)
			rows = append(rows, storageHealthRow{
				Level: "problem", LocationName: "仓库绑定", Target: name, Message: name + "：" + owner + "/" + repo + " 暂时不可用。绑定仍保留，不会自动下架。",
			})
			continue
		}
		row, _, _ := loadAppRepo(appID)
		row.Status = appRepoStatusReady
		_ = saveAppRepo(row)
		rows = append(rows, storageHealthRow{
			Level: "ok", LocationName: "仓库绑定", Target: name, Message: name + "：" + owner + "/" + repo + " 是私有仓库，可以列出发布。",
		})
	}
	return rows
}

func bindingForImport(appID int64, kind string, paid bool) (appRepoRow, string, error) {
	if appID <= 0 {
		return appRepoRow{}, "", errors.New(appRepoUnboundText)
	}
	row, ok, err := loadAppRepo(appID)
	if err != nil {
		return appRepoRow{}, "", err
	}
	if !ok {
		return appRepoRow{}, "", errAppRepoUnbound
	}
	return row, appRepoPrefixFor(kind, paid), nil
}

func filterReleasesByPrefix(items []releaseImportListItem, prefix string) []releaseImportListItem {
	out := make([]releaseImportListItem, 0)
	for _, item := range items {
		if strings.HasPrefix(item.Tag, prefix) {
			out = append(out, item)
		}
	}
	return out
}

func rememberCreateRequest(requestID string, appID int64) (int64, bool) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return 0, false
	}
	appRepoMemory.mu.Lock()
	if appRepoMemory.on {
		if existing, ok := appRepoMemory.reqs[requestID]; ok {
			appRepoMemory.mu.Unlock()
			return existing, true
		}
		if appRepoMemory.reqs == nil {
			appRepoMemory.reqs = map[string]int64{}
		}
		appRepoMemory.reqs[requestID] = appID
		appRepoMemory.mu.Unlock()
		return appID, false
	}
	appRepoMemory.mu.Unlock()
	db, err := config.DB()
	if err != nil || db == nil {
		return 0, false
	}
	_ = ensureAppRepoSchema(db)
	var existing int64
	err = db.QueryRow(`SELECT app_id FROM app_create_requests WHERE request_id = ?`, requestID).Scan(&existing)
	if err == nil && existing > 0 {
		return existing, true
	}
	_, _ = db.Exec(`INSERT INTO app_create_requests (request_id, app_id) VALUES (?, ?)`, requestID, appID)
	return appID, false
}

func lookupCreateRequest(requestID string) (int64, bool) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return 0, false
	}
	appRepoMemory.mu.Lock()
	if appRepoMemory.on {
		id, ok := appRepoMemory.reqs[requestID]
		appRepoMemory.mu.Unlock()
		return id, ok
	}
	appRepoMemory.mu.Unlock()
	db, err := config.DB()
	if err != nil || db == nil {
		return 0, false
	}
	var id int64
	err = db.QueryRow(`SELECT app_id FROM app_create_requests WHERE request_id = ?`, requestID).Scan(&id)
	return id, err == nil && id > 0
}

// finishAppCreateRepo 在应用行已经写入之后绑定仓库。失败不删应用，也不删远程仓库。
func finishAppCreateRepo(ctx context.Context, appID int64, appKey, appName, action, repo string) (bool, string) {
	action = strings.TrimSpace(action)
	if action == "" || action == "skip" {
		return false, ""
	}
	create := action == "create"
	row, err := bindAppRepo(ctx, appID, appKey, appName, action, repo, create)
	if err != nil {
		appRepoAudit("create_repo_failed", appKey, "应用已创建，仓库没有建好："+err.Error())
		if strings.Contains(err.Error(), "超时") {
			return false, appRepoTimeoutSavedText
		}
		return false, "应用已创建，仓库没有建好：" + err.Error()
	}
	_ = row
	return true, ""
}

func validateAppRepoChoice(ctx context.Context, action, repo string) error {
	action = strings.TrimSpace(action)
	if action == "" || action == "skip" {
		return nil
	}
	if _, _, err := splitOwnerRepo(repo); err != nil {
		return err
	}
	loc, token, ok := primaryGitHubBindingLocation()
	_ = loc
	if !ok || token == "" {
		return errors.New(appRepoTokenMissingText)
	}
	owner, name, _ := splitOwnerRepo(repo)
	state, err := lookupGitHubPaidRepoState(ctx, token, owner, name)
	if err != nil {
		return tokenFailure(err)
	}
	if state == githubPaidRepoPublic {
		return errors.New(appRepoPublicText)
	}
	if action == "bind" && state == githubPaidRepoMissing {
		return errors.New(appRepoMissingText)
	}
	if action == "create" && state == githubPaidRepoPrivate {
		return errors.New(appRepoTakenText)
	}
	return nil
}

func appRepoImpact(appID int64, owner, repo string) (published, drafts int) {
	needle := owner + "/" + repo
	store := currentSourceStationStore()
	if plugins, err := store.ListPlugins(""); err == nil {
		for _, item := range plugins {
			if item.AppID != appID || !strings.Contains(item.DownloadURL, needle) {
				continue
			}
			if item.Status == sourceItemHidden || item.Status == "draft" {
				drafts++
			} else {
				published++
			}
		}
	}
	if templates, err := store.ListTemplates(""); err == nil {
		for _, item := range templates {
			if item.AppID != appID || !strings.Contains(item.TemplateURL, needle) {
				continue
			}
			if item.Status == sourceItemHidden || item.Status == "draft" {
				drafts++
			} else {
				published++
			}
		}
	}
	db, err := config.DB()
	if err != nil || db == nil {
		return published, drafts
	}
	var versions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM app_versions WHERE app_id = ? AND download_url LIKE ?`, appID, "%"+needle+"%").Scan(&versions); err == nil {
		published += versions
	}
	return published, drafts
}

func copyRepoPrefix(ctx context.Context, fromOwner, fromRepo, toOwner, toRepo, prefix string) (int, error) {
	items, err := listReleaseImportReleases(ctx, fromOwner, fromRepo)
	if err != nil {
		return 0, errors.New(appRepoListFailText)
	}
	copied := 0
	for _, item := range items {
		if !strings.HasPrefix(item.Tag, prefix) || item.AssetName == "" {
			continue
		}
		payload, err := fetchGitHubReleaseAsset(ctx, fromOwner, fromRepo, "tags/"+urlPathTag(item.Tag), item.AssetName)
		if err != nil {
			return copied, fmt.Errorf("复制 %s 失败", item.Tag)
		}
		sum := sha256.Sum256(payload)
		want := hex.EncodeToString(sum[:])
		settings := sourceReleaseSettings{Provider: "github", Owner: toOwner, Repo: toRepo}
		token, tokenErr := tokenForGitHubRepo(toOwner, toRepo)
		if tokenErr != nil || token == "" {
			_, token, _ = primaryGitHubBindingLocation()
		}
		settings.Token = token
		manifest := sourcePackageManifest{ID: item.AssetName, Version: "0", Kind: "plugin", Name: item.AssetName, Filename: item.AssetName, ReleaseTag: item.Tag}
		if _, err := pushGitHubRelease(ctx, settings, manifest, payload); err != nil {
			return copied, fmt.Errorf("写入 %s 失败", item.Tag)
		}
		again, err := fetchGitHubReleaseAsset(ctx, toOwner, toRepo, "tags/"+urlPathTag(item.Tag), item.AssetName)
		if err != nil {
			return copied, fmt.Errorf("核对 %s 失败", item.Tag)
		}
		gotSum := sha256.Sum256(again)
		if hex.EncodeToString(gotSum[:]) != want {
			return copied, fmt.Errorf("核对 %s 不一致", item.Tag)
		}
		copied++
	}
	return copied, nil
}

func urlPathTag(tag string) string {
	return strings.ReplaceAll(tag, "/", "%2F")
}

func migrateOfficialAppRepo(ctx context.Context, db *sql.DB) error {
	if !officialSite() {
		return nil
	}
	var appID int64
	var appName string
	err := db.QueryRow(`SELECT id, app_name FROM apps WHERE app_key = ? LIMIT 1`, productUpdateAppKey).Scan(&appID, &appName)
	if err != nil {
		return nil
	}
	if _, ok, _ := loadAppRepo(appID); ok {
		return nil
	}
	loc, _, ok := primaryGitHubBindingLocation()
	if !ok || loc.Owner == "" || loc.Repo == "" {
		return nil
	}
	// 先把升级前没有位置前缀的发布改写到五个位置，核对通过后才绑定。
	if err := copyClassifiedReleases(ctx, loc.Owner, loc.Repo, loc.Owner, loc.Repo, legacyPaidReleaseTag); err != nil && !strings.Contains(err.Error(), appRepoListFailText) {
		return err
	}
	prefixes := []string{appRepoPrefixPluginPaid, appRepoPrefixTemplatePaid, appRepoPrefixPluginFree, appRepoPrefixTemplateFree}
	for _, prefix := range prefixes {
		if _, err := copyRepoPrefix(ctx, loc.Owner, loc.Repo, loc.Owner, loc.Repo, prefix); err != nil && !strings.Contains(err.Error(), appRepoListFailText) {
			return err
		}
	}
	clientOwner, clientRepo := "", ""
	if raw := strings.Trim(strings.TrimSpace(productUpdateRepositoryRaw()), "/"); raw != "" {
		parts := strings.Split(raw, "/")
		if len(parts) == 2 {
			clientOwner, clientRepo = parts[0], parts[1]
		}
	}
	if clientOwner == "" {
		parts := strings.Split(officialLegacyClientRepo, "/")
		clientOwner, clientRepo = parts[0], parts[1]
	}
	if err := copyClassifiedReleases(ctx, clientOwner, clientRepo, loc.Owner, loc.Repo, legacyClientReleaseTag); err != nil && !strings.Contains(err.Error(), appRepoListFailText) {
		return err
	}
	if _, err := copyRepoPrefix(ctx, clientOwner, clientRepo, loc.Owner, loc.Repo, appRepoPrefixClient); err != nil && !strings.Contains(err.Error(), appRepoListFailText) {
		return err
	}
	if err := writeClientAppKeyManifest(ctx, loc.Owner, loc.Repo, productUpdateAppKey); err != nil && !strings.Contains(err.Error(), appRepoListFailText) {
		return err
	}
	row := appRepoRow{
		AppID: appID, AppKey: productUpdateAppKey, AppName: appName, LocationID: loc.ID,
		Owner: loc.Owner, Repo: loc.Repo, Status: appRepoStatusReady, Private: true,
	}
	if err := saveAppRepo(row); err != nil {
		return err
	}
	appRepoAudit("migrate", productUpdateAppKey, "已迁入"+appName)
	return nil
}

func productUpdateRepositoryRaw() string {
	return strings.Trim(strings.TrimSpace(os.Getenv(productUpdateRepoEnv)), "/")
}

// legacyPaidReleaseTag 把升级前的收费、免费标签改到对应位置。已经带前缀的留给 copyRepoPrefix。
func legacyPaidReleaseTag(tag string) (string, bool) {
	switch {
	case strings.HasPrefix(tag, "paid-plugin-"):
		return appRepoPrefixPluginPaid + strings.TrimPrefix(tag, "paid-plugin-"), true
	case strings.HasPrefix(tag, "paid-template-"):
		return appRepoPrefixTemplatePaid + strings.TrimPrefix(tag, "paid-template-"), true
	case strings.HasPrefix(tag, "free-plugin-"):
		return appRepoPrefixPluginFree + strings.TrimPrefix(tag, "free-plugin-"), true
	case strings.HasPrefix(tag, "free-template-"):
		return appRepoPrefixTemplateFree + strings.TrimPrefix(tag, "free-template-"), true
	default:
		return "", false
	}
}

// legacyClientReleaseTag 只认纯版本号，避免把插件标签误放进 client/。
func legacyClientReleaseTag(tag string) (string, bool) {
	if strings.HasPrefix(tag, appRepoPrefixClient) || !isPlainVersionTag(tag) {
		return "", false
	}
	return appRepoPrefixClient + tag, true
}

func isPlainVersionTag(tag string) bool {
	raw := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if raw == "" || strings.ContainsAny(raw, "/ ") {
		return false
	}
	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func copyClassifiedReleases(ctx context.Context, fromOwner, fromRepo, toOwner, toRepo string, classify func(string) (string, bool)) error {
	items, err := listReleaseImportReleases(ctx, fromOwner, fromRepo)
	if err != nil {
		return errors.New(appRepoListFailText)
	}
	for _, item := range items {
		target, ok := classify(item.Tag)
		if !ok || item.AssetName == "" || target == item.Tag {
			continue
		}
		if err := copyOneRelease(ctx, fromOwner, fromRepo, toOwner, toRepo, item.Tag, target, item.AssetName); err != nil {
			return err
		}
	}
	return nil
}

func copyOneRelease(ctx context.Context, fromOwner, fromRepo, toOwner, toRepo, fromTag, toTag, asset string) error {
	payload, err := fetchGitHubReleaseAsset(ctx, fromOwner, fromRepo, "tags/"+urlPathTag(fromTag), asset)
	if err != nil {
		return fmt.Errorf("复制 %s 失败", fromTag)
	}
	return pushVerifiedRelease(ctx, toOwner, toRepo, toTag, asset, payload)
}

func pushVerifiedRelease(ctx context.Context, owner, repo, tag, asset string, payload []byte) error {
	sum := sha256.Sum256(payload)
	want := hex.EncodeToString(sum[:])
	token, tokenErr := tokenForGitHubRepo(owner, repo)
	if tokenErr != nil || token == "" {
		_, token, _ = primaryGitHubBindingLocation()
	}
	settings := sourceReleaseSettings{Provider: "github", Owner: owner, Repo: repo, Token: token}
	manifest := sourcePackageManifest{ID: asset, Version: "0", Kind: "plugin", Name: asset, Filename: asset, ReleaseTag: tag}
	if _, err := pushGitHubRelease(ctx, settings, manifest, payload); err != nil {
		return fmt.Errorf("写入 %s 失败", tag)
	}
	again, err := fetchGitHubReleaseAsset(ctx, owner, repo, "tags/"+urlPathTag(tag), asset)
	if err != nil {
		return fmt.Errorf("核对 %s 失败", tag)
	}
	got := sha256.Sum256(again)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("核对 %s 不一致", tag)
	}
	return nil
}

// writeClientAppKeyManifest 在最新的客户安装包发布上补上应用标识。没有客户包时不写空文件。
func writeClientAppKeyManifest(ctx context.Context, owner, repo, appKey string) error {
	items, err := listReleaseImportReleases(ctx, owner, repo)
	if err != nil {
		return errors.New(appRepoListFailText)
	}
	tag := ""
	for _, item := range items {
		if strings.HasPrefix(item.Tag, appRepoPrefixClient) {
			tag = item.Tag
			break
		}
	}
	if tag == "" {
		return nil
	}
	doc := map[string]any{}
	if body, err := fetchGitHubReleaseAsset(ctx, owner, repo, "tags/"+urlPathTag(tag), "latest.json"); err == nil {
		_ = json.Unmarshal(body, &doc)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	doc["appKey"] = appKey
	payload, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return pushVerifiedRelease(ctx, owner, repo, tag, "latest.json", payload)
}

type appRepoTagCtxKey struct{}

func withAppRepoTag(ctx context.Context, tag string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, appRepoTagCtxKey{}, strings.TrimSpace(tag))
}

func appRepoTagFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	tag, _ := ctx.Value(appRepoTagCtxKey{}).(string)
	return strings.TrimSpace(tag)
}
