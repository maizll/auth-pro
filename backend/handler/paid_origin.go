package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

const (
	paidOriginHealthOK          = "ok"
	paidOriginHealthUnavailable = "unavailable"
	paidOriginUnavailableHint   = "来源外链不可用，重新拉取将失败"
	paidOriginFetchTimeout      = 20 * time.Second
	paidOriginHealthInterval    = time.Hour
)

type paidImportResult struct {
	Ref     string
	SHA256  string
	Version string
	Origin  string
	Health  string
}

func importPaidPackageFromURL(ctx context.Context, kind, category, rawURL string) (paidImportResult, error) {
	rawURL = strings.TrimSpace(rawURL)
	payload, err := fetchPaidOriginZIP(ctx, rawURL)
	if err != nil {
		return paidImportResult{}, err
	}
	manifest, err := parseSourcePackageBytes("package.zip", payload, kind, category)
	if err != nil {
		return paidImportResult{}, err
	}
	ver := strings.TrimSpace(manifest.Version)
	ref, fileSHA, _, err := settlePaidZipBytes(ctx, sourceFirstNonEmpty(kind, manifest.Kind), manifest.ID, ver, payload)
	if err != nil {
		return paidImportResult{}, err
	}
	if !strings.EqualFold(manifest.SHA256, fileSHA) {
		removePaidPackageFile(ref)
		return paidImportResult{}, errors.New("保存后的校验码与清单不一致")
	}
	return paidImportResult{
		Ref: ref, SHA256: fileSHA, Version: manifest.Version, Origin: rawURL, Health: paidOriginHealthOK,
	}, nil
}

// settlePaidZipBytes 把校验过的收费 ZIP 放进站长的私有仓库。未配置仓库时暂存本站，并返回 local=true。
// 已配置时上传失败不会改回本站托管。
func settlePaidZipBytes(ctx context.Context, kind, id, version string, payload []byte) (string, string, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !isZipPayload(payload) {
		return "", "", false, errors.New("必须上传 ZIP 压缩包")
	}
	sum := sha256.Sum256(payload)
	fileSHA := hex.EncodeToString(sum[:])
	version = strings.TrimSpace(version)
	if version == "" {
		version = "1.0.0"
	}
	if githubPaidRepoConfigured() {
		ref, err := uploadPaidZipToStationRepo(ctx, kind, strings.TrimSpace(id), version, payload)
		if err != nil {
			return "", "", false, err
		}
		return ref, fileSHA, false, nil
	}
	ref, storedSHA, err := storePaidPackageBytes(payload)
	if err != nil {
		return "", "", false, err
	}
	return ref, storedSHA, true, nil
}

func readStationHostedZip(location string) ([]byte, error) {
	publicURL, _, err := stationPackageIdentity(location)
	if err != nil {
		return nil, err
	}
	name, ok := stationPackageNameFromURL(publicURL)
	if !ok {
		return nil, errors.New("本站托管地址不合法")
	}
	payload, err := os.ReadFile(filepath.Join(stationPackageDir(), name))
	if err != nil || len(payload) == 0 || !isZipPayload(payload) {
		return nil, errors.New("本站托管的 ZIP 不存在")
	}
	return payload, nil
}

func dropUnusedStationPackage(location, kind, itemID string) {
	publicURL, _, err := stationPackageIdentity(location)
	if err != nil {
		return
	}
	kept, err := otherCatalogItemUsesPackage(publicURL, kind, itemID)
	if err != nil || kept {
		return
	}
	name, ok := stationPackageNameFromURL(publicURL)
	if !ok {
		return
	}
	_ = os.Remove(filepath.Join(stationPackageDir(), name))
}

func fetchPaidOriginZIP(ctx context.Context, rawURL string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := safeHTTPGet(ctx, rawURL, safeFetchOptions{
		AllowPrivate: false,
		RequireHTTPS: true,
		MaxBytes:     pluginPackageMaxSize,
		Timeout:      paidOriginFetchTimeout,
		MaxRedirects: defaultSafeRedirects,
		UserAgent:    "auth-pro-paid-import",
		Accept:       "application/zip,*/*",
		BaseClient:   externalPackageClient,
	})
	if err != nil {
		return nil, paidOriginFetchError(err)
	}
	if !isZipPayload(payload) {
		return nil, errors.New("外链不是 ZIP")
	}
	return payload, nil
}

func paidOriginFetchError(err error) error {
	switch {
	case errors.Is(err, errSafeHTTPSRequired):
		return errors.New("来源外链必须是 https:// 地址")
	case errors.Is(err, errSafePrivateAddress):
		return errSafePrivateAddress
	case errors.Is(err, errSafeMetadataAddress):
		return errSafeMetadataAddress
	case errors.Is(err, errSafeRedirect):
		return errors.New("外链重定向过多")
	case errors.Is(err, errSafeRedirectScheme):
		return errSafeRedirectScheme
	case errors.Is(err, errSafeBadURL):
		return errors.New("来源外链地址不合法")
	case errors.Is(err, errSafeTooLarge):
		return errors.New("外链 ZIP 超过 20 MiB")
	default:
		var status *safeStatusError
		if errors.As(err, &status) {
			return fmt.Errorf("外链返回状态码 %d", status.Code)
		}
		return errors.New("外链不可达")
	}
}

func storePaidPackageBytes(payload []byte) (string, string, error) {
	if !isZipPayload(payload) {
		return "", "", errors.New("必须上传 ZIP 压缩包")
	}
	name, err := newPaidPackageName()
	if err != nil {
		return "", "", err
	}
	dir := stationPaidPackageDir()
	finalPath := filepath.Join(dir, name)
	tmp, err := os.CreateTemp(dir, "paid-import-*.zip")
	if err != nil {
		return "", "", errors.New("保存付费包失败")
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.Write(payload)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tmpName)
		return "", "", errors.New("保存付费包失败")
	}
	if err := os.Rename(tmpName, finalPath); err != nil {
		_ = os.Remove(tmpName)
		return "", "", errors.New("保存付费包失败")
	}
	_ = os.Chmod(finalPath, 0640)
	ref, fileSHA, err := verifyPrivatePackage(privatePackageRef(name), "")
	if err != nil {
		_ = os.Remove(finalPath)
		return "", "", err
	}
	return ref, fileSHA, nil
}

func removePaidPackageFile(ref string) {
	name, ok := privatePackageName(ref)
	if !ok {
		return
	}
	_ = os.Remove(filepath.Join(stationPaidPackageDirPath(), name))
}

func adoptPaidItemLocation(kind, category, itemID, location, sha, version string, price int64) (string, string, string, string, string, error) {
	location = strings.TrimSpace(location)
	if price <= 0 {
		return location, sha, version, "", "", nil
	}
	if location == "" {
		if kept, keptSHA, origin, health, ok := existingPaidLocation(kind, itemID); ok {
			if strings.TrimSpace(sha) == "" {
				sha = keptSHA
			}
			return kept, sha, version, origin, health, nil
		}
		return "", sha, version, "", "", nil
	}
	if isGitHubPackageRef(location) {
		return location, sha, version, "", paidOriginHealthOK, nil
	}
	if isPrivatePackageRef(location) {
		origin, health := matchingPaidItemOrigin(kind, itemID, location)
		return location, sha, version, origin, health, nil
	}
	if isStationHostedPackageURL(location) {
		if !githubPaidRepoConfigured() {
			return location, sha, version, "", "", nil
		}
		payload, err := readStationHostedZip(location)
		if err != nil {
			return "", "", "", "", "", err
		}
		ref, fileSHA, _, err := settlePaidZipBytes(context.Background(), kind, itemID, version, payload)
		if err != nil {
			return "", "", "", "", "", err
		}
		if isGitHubPackageRef(ref) {
			dropUnusedStationPackage(location, kind, itemID)
		}
		return ref, fileSHA, version, "", paidOriginHealthOK, nil
	}
	if !isHTTPSLocation(location) {
		if hasURLScheme(location) {
			return "", "", "", "", "", errors.New("来源外链必须是 https:// 地址")
		}
		return "", "", "", "", "", errSourcePaidExternal
	}
	if reused, ok := reusePaidItemOrigin(kind, itemID, location); ok {
		ver := version
		if ver == "" {
			ver = reused.Version
		}
		return reused.Ref, reused.SHA256, ver, reused.Origin, reused.Health, nil
	}
	imported, err := importPaidPackageFromURL(context.Background(), kind, category, location)
	if err != nil {
		return "", "", "", "", "", err
	}
	ver := strings.TrimSpace(imported.Version)
	if ver == "" {
		ver = version
	}
	return imported.Ref, imported.SHA256, ver, imported.Origin, imported.Health, nil
}

func adoptPaidVersionLocation(kind, itemID, version, location, sha string) (string, string, string, error) {
	location = strings.TrimSpace(location)
	price, err := catalogItemPrice(kind, itemID)
	if errors.Is(err, errSourceNotFound) {
		price = 0
		err = nil
	}
	if err != nil {
		return "", "", "", err
	}
	if price <= 0 {
		return location, sha, "", nil
	}
	if location == "" {
		if rel, err := currentSourceStationStore().GetVersion(kind, itemID, version); err == nil &&
			(isGitHubPackageRef(rel.Location) || isPrivatePackageRef(rel.Location)) {
			if strings.TrimSpace(sha) == "" {
				sha = rel.SHA256
			}
			return rel.Location, sha, rel.OriginURL, nil
		}
		return "", sha, "", nil
	}
	if isPrivatePackageRef(location) {
		origin := matchingPaidVersionOrigin(kind, itemID, version, location)
		return location, sha, origin, nil
	}
	if isGitHubPackageRef(location) {
		return location, sha, "", nil
	}
	if isStationHostedPackageURL(location) {
		if !githubPaidRepoConfigured() {
			return location, sha, "", nil
		}
		payload, err := readStationHostedZip(location)
		if err != nil {
			return "", "", "", err
		}
		ref, fileSHA, _, err := settlePaidZipBytes(context.Background(), kind, itemID, version, payload)
		if err != nil {
			return "", "", "", err
		}
		if isGitHubPackageRef(ref) {
			dropUnusedStationPackage(location, kind, itemID)
		}
		return ref, fileSHA, "", nil
	}
	if !isHTTPSLocation(location) {
		if hasURLScheme(location) {
			return "", "", "", errors.New("来源外链必须是 https:// 地址")
		}
		return "", "", "", errSourcePaidExternal
	}
	if rel, err := currentSourceStationStore().GetVersion(kind, itemID, version); err == nil &&
		strings.TrimSpace(rel.OriginURL) == location {
		if isGitHubPackageRef(rel.Location) && len(strings.TrimSpace(rel.SHA256)) == 64 {
			return rel.Location, rel.SHA256, rel.OriginURL, nil
		}
		if isPrivatePackageRef(rel.Location) {
			if _, fileSHA, verr := verifyPrivatePackage(rel.Location, ""); verr == nil {
				return rel.Location, fileSHA, rel.OriginURL, nil
			}
		}
	}
	imported, err := importPaidPackageFromURL(context.Background(), kind, paidItemCategory(kind, itemID), location)
	if err != nil {
		return "", "", "", err
	}
	return imported.Ref, imported.SHA256, imported.Origin, nil
}

func existingPaidLocation(kind, itemID string) (location, sha, origin, health string, ok bool) {
	switch kind {
	case sourceKindTemplate:
		item, err := currentSourceStationStore().GetTemplate(itemID)
		if err != nil {
			return "", "", "", "", false
		}
		location, sha, origin, health = item.TemplateURL, item.SHA256, item.OriginURL, item.OriginHealth
	default:
		item, err := currentSourceStationStore().GetPlugin(itemID)
		if err != nil {
			return "", "", "", "", false
		}
		location, sha, origin, health = item.DownloadURL, item.SHA256, item.OriginURL, item.OriginHealth
	}
	if isGitHubPackageRef(location) || isPrivatePackageRef(location) {
		return location, sha, origin, health, true
	}
	return "", "", "", "", false
}

func matchingPaidItemOrigin(kind, itemID, location string) (string, string) {
	switch kind {
	case sourceKindTemplate:
		item, err := currentSourceStationStore().GetTemplate(itemID)
		if err != nil || item.TemplateURL != location {
			return "", ""
		}
		return item.OriginURL, item.OriginHealth
	default:
		item, err := currentSourceStationStore().GetPlugin(itemID)
		if err != nil || item.DownloadURL != location {
			return "", ""
		}
		return item.OriginURL, item.OriginHealth
	}
}

func reusePaidItemOrigin(kind, itemID, rawURL string) (paidImportResult, bool) {
	var origin, health, location, version string
	switch kind {
	case sourceKindTemplate:
		item, err := currentSourceStationStore().GetTemplate(itemID)
		if err != nil {
			return paidImportResult{}, false
		}
		origin, health, location, version = item.OriginURL, item.OriginHealth, item.TemplateURL, item.Version
	default:
		item, err := currentSourceStationStore().GetPlugin(itemID)
		if err != nil {
			return paidImportResult{}, false
		}
		origin, health, location, version = item.OriginURL, item.OriginHealth, item.DownloadURL, item.Version
	}
	if strings.TrimSpace(origin) != strings.TrimSpace(rawURL) {
		return paidImportResult{}, false
	}
	if isGitHubPackageRef(location) {
		sha := ""
		switch kind {
		case sourceKindTemplate:
			item, err := currentSourceStationStore().GetTemplate(itemID)
			if err == nil {
				sha = item.SHA256
			}
		default:
			item, err := currentSourceStationStore().GetPlugin(itemID)
			if err == nil {
				sha = item.SHA256
			}
		}
		if health == "" {
			health = paidOriginHealthOK
		}
		return paidImportResult{Ref: location, SHA256: sha, Version: version, Origin: origin, Health: health}, sha != ""
	}
	if !isPrivatePackageRef(location) {
		return paidImportResult{}, false
	}
	ref, fileSHA, err := verifyPrivatePackage(location, "")
	if err != nil {
		return paidImportResult{}, false
	}
	if health == "" {
		health = paidOriginHealthOK
	}
	return paidImportResult{Ref: ref, SHA256: fileSHA, Version: version, Origin: origin, Health: health}, true
}

func matchingPaidVersionOrigin(kind, itemID, version, location string) string {
	rel, err := currentSourceStationStore().GetVersion(kind, itemID, version)
	if err != nil || rel.Location != location {
		return ""
	}
	return rel.OriginURL
}

func paidItemCategory(kind, itemID string) string {
	if kind == sourceKindTemplate {
		item, err := currentSourceStationStore().GetTemplate(itemID)
		if err != nil {
			return ""
		}
		return item.Category
	}
	item, err := currentSourceStationStore().GetPlugin(itemID)
	if err != nil {
		return ""
	}
	return item.Category
}

func isHTTPSLocation(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Host != "" && strings.EqualFold(parsed.Scheme, "https") && parsed.User == nil
}

func paidOriginForLocation(origin, storedLocation, incomingLocation string) string {
	if strings.TrimSpace(incomingLocation) == "" || incomingLocation == storedLocation {
		return origin
	}
	return ""
}

func hasURLScheme(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme != "" && strings.Contains(raw, "://")
}

func refetchPaidCatalogPackage(ctx context.Context, kind, id string) error {
	id = strings.TrimSpace(id)
	origin, location, category, version := "", "", "", ""
	var price int64
	switch kind {
	case sourceKindTemplate:
		item, err := currentSourceStationStore().GetTemplate(id)
		if err != nil {
			return err
		}
		origin, location, category, version, price = item.OriginURL, item.TemplateURL, item.Category, item.Version, item.PriceCents
	default:
		kind = sourceKindPlugin
		item, err := currentSourceStationStore().GetPlugin(id)
		if err != nil {
			return err
		}
		origin, location, category, version, price = item.OriginURL, item.DownloadURL, item.Category, item.Version, item.PriceCents
	}
	if price <= 0 || strings.TrimSpace(origin) == "" {
		return errors.New("这条没有可重新拉取的来源外链")
	}
	imported, err := importPaidPackageFromURL(ctx, kind, category, origin)
	if err != nil {
		return err
	}
	nextVersion := strings.TrimSpace(imported.Version)
	if nextVersion == "" {
		nextVersion = version
	}
	if err := currentSourceStationStore().ReplacePaidItemPackage(kind, id, imported.Ref, imported.SHA256, nextVersion, imported.Origin, imported.Health); err != nil {
		removePaidPackageFile(imported.Ref)
		return err
	}
	if location != imported.Ref {
		removePaidPackageFile(location)
	}
	return nil
}

func (store *memorySourceStore) ReplacePaidItemPackage(kind, id, location, sha, version, origin, health string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	now := time.Now().UTC()
	if kind == sourceKindTemplate {
		item, ok := store.templates[id]
		if !ok {
			return errSourceNotFound
		}
		item.TemplateURL = location
		item.SHA256 = sha
		if strings.TrimSpace(version) != "" {
			item.Version = version
		}
		item.OriginURL = origin
		item.OriginHealth = health
		item.UpdatedAt = now
		store.templates[id] = item
		store.rememberPaidVersionLocked(kind, id, item.Version, location, sha, origin, now)
		return nil
	}
	item, ok := store.plugins[id]
	if !ok {
		return errSourceNotFound
	}
	item.DownloadURL = location
	item.SHA256 = sha
	if strings.TrimSpace(version) != "" {
		item.Version = version
	}
	item.OriginURL = origin
	item.OriginHealth = health
	item.UpdatedAt = now
	store.plugins[id] = item
	store.rememberPaidVersionLocked(kind, id, item.Version, location, sha, origin, now)
	return nil
}

func (store *memorySourceStore) rememberPaidVersionLocked(kind, id, version, location, sha, origin string, now time.Time) {
	version = strings.TrimSpace(version)
	if version == "" {
		return
	}
	bucket := store.versionMap(kind)
	if bucket[id] == nil {
		return
	}
	rel, ok := bucket[id][version]
	if !ok {
		return
	}
	rel.Location = location
	rel.SHA256 = sha
	rel.OriginURL = origin
	rel.UpdatedAt = now
	bucket[id][version] = rel
}

func (mysqlSourceStore) ReplacePaidItemPackage(kind, id, location, sha, version, origin, health string) error {
	db, err := config.DB()
	if err != nil {
		return err
	}
	version = strings.TrimSpace(version)
	if kind == sourceKindTemplate {
		if _, err := currentSourceStationStore().GetTemplate(id); err != nil {
			return err
		}
		if _, err := db.Exec(`UPDATE source_catalog_templates SET template_url=?, sha256=?, version=IF(?='', version, ?), origin_url=?, origin_health=? WHERE id=?`,
			location, sha, version, version, origin, health, id); err != nil {
			return err
		}
	} else {
		kind = sourceKindPlugin
		if _, err := currentSourceStationStore().GetPlugin(id); err != nil {
			return err
		}
		if _, err := db.Exec(`UPDATE source_catalog_plugins SET download_url=?, sha256=?, version=IF(?='', version, ?), origin_url=?, origin_health=? WHERE id=?`,
			location, sha, version, version, origin, health, id); err != nil {
			return err
		}
	}
	if version == "" {
		return nil
	}
	rel, err := (mysqlSourceStore{}).GetVersion(kind, id, version)
	if errors.Is(err, errSourceNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	rel.Location = location
	rel.SHA256 = sha
	rel.OriginURL = origin
	return mysqlWriteVersion(db, kind, id, version, rel, rel.Status)
}

func (mysqlSourceStore) SetPaidOriginHealth(kind, id, health string) error {
	db, err := config.DB()
	if err != nil {
		return err
	}
	table := "source_catalog_plugins"
	if kind == sourceKindTemplate {
		table = "source_catalog_templates"
	}
	locationColumn := "download_url"
	if kind == sourceKindTemplate {
		locationColumn = "template_url"
	}
	_, err = db.Exec(`UPDATE `+table+` SET origin_health=? WHERE id=? AND (origin_url<>'' OR `+locationColumn+` LIKE 'github:%')`, health, id)
	return err
}

func (store *memorySourceStore) SetPaidOriginHealth(kind, id, health string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if kind == sourceKindTemplate {
		item, ok := store.templates[id]
		if !ok {
			return errSourceNotFound
		}
		if strings.TrimSpace(item.OriginURL) == "" && !isGitHubPackageRef(item.TemplateURL) {
			return nil
		}
		item.OriginHealth = health
		store.templates[id] = item
		return nil
	}
	item, ok := store.plugins[id]
	if !ok {
		return errSourceNotFound
	}
	if strings.TrimSpace(item.OriginURL) == "" && !isGitHubPackageRef(item.DownloadURL) {
		return nil
	}
	item.OriginHealth = health
	store.plugins[id] = item
	return nil
}

func probePaidOrigin(ctx context.Context, rawURL string) error {
	_, err := safeHTTPGet(ctx, rawURL, safeFetchOptions{
		AllowPrivate: false,
		RequireHTTPS: true,
		MaxBytes:     1,
		Timeout:      15 * time.Second,
		MaxRedirects: defaultSafeRedirects,
		UserAgent:    "auth-pro-origin-check",
		BaseClient:   externalPackageClient,
	})
	if err == nil || errors.Is(err, errSafeTooLarge) {
		return nil
	}
	return err
}

func checkPaidOriginHealth(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	store := currentSourceStationStore()
	plugins, err := store.ListPlugins("")
	if err == nil {
		for _, item := range plugins {
			touchPaidOriginHealth(ctx, store, sourceKindPlugin, item.ID, item.PriceCents, item.OriginURL, item.DownloadURL, item.OriginHealth)
			touchGitHubPaidHealth(ctx, store, sourceKindPlugin, item.ID, item.Name, item.PriceCents, item.DownloadURL, item.OriginHealth)
		}
	}
	templates, err := store.ListTemplates("")
	if err == nil {
		for _, item := range templates {
			touchPaidOriginHealth(ctx, store, sourceKindTemplate, item.ID, item.PriceCents, item.OriginURL, item.TemplateURL, item.OriginHealth)
			touchGitHubPaidHealth(ctx, store, sourceKindTemplate, item.ID, item.Name, item.PriceCents, item.TemplateURL, item.OriginHealth)
		}
	}
}

func touchPaidOriginHealth(ctx context.Context, store sourceStationStore, kind, id string, price int64, origin, location, current string) {
	if price <= 0 || strings.TrimSpace(origin) == "" || !isPrivatePackageRef(location) {
		return
	}
	health := paidOriginHealthOK
	if err := probePaidOrigin(ctx, origin); err != nil {
		health = paidOriginHealthUnavailable
	}
	if health == current {
		return
	}
	_ = store.SetPaidOriginHealth(kind, id, health)
}

func StartPaidOriginHealthCheck() {
	go func() {
		timer := time.NewTimer(time.Minute)
		defer timer.Stop()
		ticker := time.NewTicker(paidOriginHealthInterval)
		defer ticker.Stop()
		for {
			select {
			case <-timer.C:
				checkPaidOriginHealth(context.Background())
			case <-ticker.C:
				checkPaidOriginHealth(context.Background())
			}
		}
	}()
}

func pullPaidCatalogItem(c *gin.Context, kind string, asAdmin bool) {
	id := strings.TrimSpace(c.Param("id"))
	if !asAdmin {
		developer, err := currentSourceDeveloper(c)
		if err != nil {
			writeCurrentSourceDeveloperError(c, err)
			return
		}
		if kind == sourceKindTemplate {
			item, err := currentSourceStationStore().GetTemplate(id)
			if err != nil {
				writeSourceDeveloperStoreError(c, err)
				return
			}
			if item.DeveloperID != developer.ID {
				c.JSON(200, gin.H{"code": 403, "msg": errSourceForbidden.Error()})
				return
			}
		} else {
			item, err := currentSourceStationStore().GetPlugin(id)
			if err != nil {
				writeSourceDeveloperStoreError(c, err)
				return
			}
			if item.DeveloperID != developer.ID {
				c.JSON(200, gin.H{"code": 403, "msg": errSourceForbidden.Error()})
				return
			}
		}
	}
	if err := refetchPaidCatalogPackage(c.Request.Context(), kind, id); err != nil {
		code := 400
		if errors.Is(err, errSourceNotFound) {
			code = 404
		}
		c.JSON(200, gin.H{"code": code, "msg": err.Error()})
		return
	}
	if kind == sourceKindTemplate {
		item, err := currentSourceStationStore().GetTemplate(id)
		if err != nil {
			writeSourceDeveloperStoreError(c, err)
			return
		}
		view := sourceTemplateView(item)
		if !asAdmin {
			concealDeveloperPaidStorage(view)
		}
		c.JSON(200, gin.H{"code": 200, "msg": "已重新拉取并更新托管包", "data": view})
		return
	}
	item, err := currentSourceStationStore().GetPlugin(id)
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	view := sourcePluginView(item)
	if !asAdmin {
		concealDeveloperPaidStorage(view)
	}
	c.JSON(200, gin.H{"code": 200, "msg": "已重新拉取并更新托管包", "data": view})
}

func SourceDeveloperPullPlugin(c *gin.Context) {
	pullPaidCatalogItem(c, sourceKindPlugin, false)
}

func SourceDeveloperPullTemplate(c *gin.Context) {
	pullPaidCatalogItem(c, sourceKindTemplate, false)
}

func AdminSourcePullPlugin(c *gin.Context) {
	pullPaidCatalogItem(c, sourceKindPlugin, true)
}

func AdminSourcePullTemplate(c *gin.Context) {
	pullPaidCatalogItem(c, sourceKindTemplate, true)
}

func attachPaidOriginView(view gin.H, origin, health string) {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return
	}
	view["originUrl"] = origin
	view["originHealth"] = health
	if health == paidOriginHealthUnavailable {
		view["originHint"] = paidOriginUnavailableHint
	}
}
