package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"auto_pro/config"
)

const (
	sourceBillingFree       = "free"
	sourceBillingOneTime    = "one_time"
	sourceBillingYearly     = "yearly"
	sourceDeliveryZip       = "zip"
	sourceDeliveryBuiltin   = "builtin"
	sourcePaidPackagePrefix = "paid:"
	// 1 亿元，单位分。挡住误把「元」当成「分」再乘一次的输入。
	maxCatalogPriceCents int64 = 10_000_000_000
)

var (
	errSourcePaidListingClosed = errors.New("付费条目暂不能上架，购买与交付将在后续版本开放")
	errSourcePaidExternal      = errors.New("付费条目请上传 ZIP，或填写 HTTPS 外链由本站拉取托管")
	errSourcePaidYearly        = errors.New("年付尚未开放，当前只支持买断")
	errSourcePaidAlreadyPublic = errors.New("已公开的免费条目不能直接改为付费")
)

func stationPaidPackageDirPath() string {
	return filepath.Join(config.GetDataDir(), "source-packages-paid")
}

func stationPaidPackageDir() string {
	dir := stationPaidPackageDirPath()
	_ = os.MkdirAll(dir, 0750)
	return dir
}

func catalogBillingLabel(billing string) string {
	billing = strings.TrimSpace(billing)
	if billing == "" {
		return sourceBillingFree
	}
	return billing
}

func catalogDeliveryLabel(delivery string) string {
	delivery = strings.TrimSpace(delivery)
	if delivery == "" {
		return sourceDeliveryZip
	}
	return delivery
}

func normalizeCatalogPrice(cents int64, billing, delivery string) (int64, string, string, error) {
	if cents < 0 {
		return 0, "", "", errors.New("价格不能为负数")
	}
	if cents > maxCatalogPriceCents {
		return 0, "", "", errors.New("价格超出允许范围")
	}
	billing = strings.ToLower(strings.TrimSpace(billing))
	switch billing {
	case "permanent":
		billing = sourceBillingOneTime
	case "", sourceBillingFree, sourceBillingOneTime, sourceBillingYearly:
	default:
		return 0, "", "", errors.New("计费方式不合法")
	}
	if billing == sourceBillingYearly {
		return 0, "", "", errSourcePaidYearly
	}
	if cents == 0 {
		billing = sourceBillingFree
	} else if billing == "" || billing == sourceBillingOneTime {
		billing = sourceBillingOneTime
	} else {
		return 0, "", "", errors.New("付费条目计费方式须为买断")
	}
	delivery = strings.ToLower(strings.TrimSpace(delivery))
	if delivery == "" {
		delivery = sourceDeliveryZip
	}
	if delivery != sourceDeliveryZip && delivery != sourceDeliveryBuiltin {
		return 0, "", "", errors.New("交付方式不合法")
	}
	return cents, billing, delivery, nil
}

func rejectPaidPriceOnPublicItem(status, latestVersion string, existingPrice, newPrice int64) error {
	if newPrice <= 0 || existingPrice > 0 {
		return nil
	}
	if status == sourceItemPublished || status == sourceItemHidden || strings.TrimSpace(latestVersion) != "" {
		return errSourcePaidAlreadyPublic
	}
	return nil
}

func paidPublishError(developerID, price int64, delivery, location string) error {
	if price <= 0 {
		return nil
	}
	if developerID > 0 {
		return errSourcePaidListingClosed
	}
	if delivery == sourceDeliveryBuiltin {
		return nil
	}
	if !isStationHostedPackageURL(location) && !isPrivatePackageRef(location) && !isGitHubPackageRef(location) {
		return errSourcePaidExternal
	}
	return nil
}

func rejectPaidExternalLocation(price int64, location string) error {
	if price <= 0 || strings.TrimSpace(location) == "" {
		return nil
	}
	if isStationHostedPackageURL(location) || isPrivatePackageRef(location) || isGitHubPackageRef(location) {
		return nil
	}
	return errSourcePaidExternal
}

func isPrivatePackageRef(raw string) bool {
	_, ok := privatePackageName(raw)
	return ok
}

func privatePackageName(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if !strings.HasPrefix(value, sourcePaidPackagePrefix) {
		return "", false
	}
	name := strings.TrimPrefix(value, sourcePaidPackagePrefix)
	if strings.Contains(name, "/") || strings.Contains(name, "..") || len(name) != len("0123456789abcdef0123456789abcdef.zip") {
		return "", false
	}
	if !strings.HasSuffix(name, ".zip") {
		return "", false
	}
	stem := strings.TrimSuffix(name, ".zip")
	if len(stem) != 32 {
		return "", false
	}
	for _, ch := range stem {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return "", false
		}
	}
	return name, true
}

func privatePackageRef(name string) string {
	return sourcePaidPackagePrefix + name
}

func newPaidPackageName() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", errors.New("保存付费包失败")
	}
	return hex.EncodeToString(buf) + ".zip", nil
}

func prepareCatalogPackage(price int64, location, sha, ignoreKind, ignoreID string) (string, string, error) {
	location = strings.TrimSpace(location)
	sha = strings.ToLower(strings.TrimSpace(sha))
	if isGitHubPackageRef(location) {
		if price <= 0 {
			return "", "", errors.New("私有 GitHub 仓库只用于收费条目")
		}
		if len(sha) != 64 {
			return "", "", errors.New("私有仓库安装包缺少校验码")
		}
		return location, sha, nil
	}
	if price <= 0 {
		if isPrivatePackageRef(location) {
			return unsealPaidPackage(location, sha)
		}
		return location, sha, nil
	}
	if err := rejectPaidExternalLocation(price, location); err != nil {
		return "", "", err
	}
	if location == "" {
		return "", sha, nil
	}
	if isPrivatePackageRef(location) {
		return verifyPrivatePackage(location, sha)
	}
	return sealStationPackage(location, sha, ignoreKind, ignoreID)
}

func validateCatalogPackage(price int64, kind, location, sha string) error {
	location = strings.TrimSpace(location)
	if price > 0 && location == "" {
		return errSourcePaidExternal
	}
	if err := rejectPaidExternalLocation(price, location); err != nil {
		return err
	}
	if isGitHubPackageRef(location) {
		if price <= 0 {
			return errors.New("私有 GitHub 仓库只用于收费条目")
		}
		if len(strings.ToLower(strings.TrimSpace(sha))) != 64 {
			return errors.New("私有仓库安装包缺少校验码")
		}
		return nil
	}
	if isPrivatePackageRef(location) {
		_, _, err := verifyPrivatePackage(location, sha)
		return err
	}
	return validatePackageForSubmit(kind, location, sha)
}

func verifyPrivatePackage(location, sha string) (string, string, error) {
	name, ok := privatePackageName(location)
	if !ok {
		return "", "", errors.New("付费包地址不合法")
	}
	path := filepath.Join(stationPaidPackageDirPath(), name)
	payload, err := os.ReadFile(path)
	if err != nil || len(payload) == 0 || !isZipPayload(payload) {
		return "", "", errors.New("本站托管的 ZIP 不存在")
	}
	sum := sha256.Sum256(payload)
	fileSHA := hex.EncodeToString(sum[:])
	sha = strings.ToLower(strings.TrimSpace(sha))
	if sha != "" && sha != fileSHA {
		return "", "", errors.New("sha256 与本站托管 ZIP 不一致")
	}
	return privatePackageRef(name), fileSHA, nil
}

func sealStationPackage(location, sha, ignoreKind, ignoreID string) (string, string, error) {
	publicURL, fileSHA, err := stationPackageIdentity(location)
	if err != nil {
		return "", "", err
	}
	sha = strings.ToLower(strings.TrimSpace(sha))
	if sha != "" && sha != fileSHA {
		return "", "", errors.New("sha256 与本站托管 ZIP 不一致")
	}
	name, ok := stationPackageNameFromURL(publicURL)
	if !ok {
		return "", "", errors.New("本站托管地址不合法")
	}
	publicPath := filepath.Join(stationPackageDir(), name)
	payload, err := os.ReadFile(publicPath)
	if err != nil || len(payload) == 0 {
		return "", "", errors.New("本站托管的 ZIP 不存在")
	}
	privateName, err := newPaidPackageName()
	if err != nil {
		return "", "", err
	}
	dir := stationPaidPackageDir()
	finalPath := filepath.Join(dir, privateName)
	tmp, err := os.CreateTemp(dir, "paid-*.zip")
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
	kept, err := otherCatalogItemUsesPackage(publicURL, ignoreKind, ignoreID)
	if err != nil {
		_ = os.Remove(finalPath)
		return "", "", errors.New("保存付费包失败")
	}
	if !kept {
		if err := os.Remove(publicPath); err != nil && !os.IsNotExist(err) {
			_ = os.Remove(finalPath)
			return "", "", errors.New("保存付费包失败")
		}
	}
	return privatePackageRef(privateName), fileSHA, nil
}

func unsealPaidPackage(location, sha string) (string, string, error) {
	ref, _, err := verifyPrivatePackage(location, sha)
	if err != nil {
		return "", "", err
	}
	name, _ := privatePackageName(ref)
	payload, err := os.ReadFile(filepath.Join(stationPaidPackageDirPath(), name))
	if err != nil {
		return "", "", errors.New("本站托管的 ZIP 不存在")
	}
	publicURL, storedSHA, err := storeStationPackage(payload)
	if err != nil {
		return "", "", err
	}
	_ = os.Remove(filepath.Join(stationPaidPackageDirPath(), name))
	return publicURL, storedSHA, nil
}

func otherCatalogItemUsesPackage(publicURL, ignoreKind, ignoreID string) (bool, error) {
	publicURL = strings.TrimSpace(publicURL)
	if publicURL == "" {
		return false, nil
	}
	store := currentSourceStationStore()
	plugins, err := store.ListPlugins("")
	if err != nil {
		return false, err
	}
	for _, item := range plugins {
		if ignoreKind == sourceKindPlugin && item.ID == ignoreID {
			continue
		}
		if sameStationPackageURL(item.DownloadURL, publicURL) {
			return true, nil
		}
		versions, err := store.ListVersions(sourceKindPlugin, item.ID)
		if err != nil {
			return false, err
		}
		for _, rel := range versions {
			if sameStationPackageURL(rel.Location, publicURL) {
				return true, nil
			}
		}
	}
	templates, err := store.ListTemplates("")
	if err != nil {
		return false, err
	}
	for _, item := range templates {
		if ignoreKind == sourceKindTemplate && item.ID == ignoreID {
			continue
		}
		if sameStationPackageURL(item.TemplateURL, publicURL) {
			return true, nil
		}
		versions, err := store.ListVersions(sourceKindTemplate, item.ID)
		if err != nil {
			return false, err
		}
		for _, rel := range versions {
			if sameStationPackageURL(rel.Location, publicURL) {
				return true, nil
			}
		}
	}
	return false, nil
}

func sameStationPackageURL(left, right string) bool {
	leftName, leftOK := stationPackageNameFromURL(left)
	rightName, rightOK := stationPackageNameFromURL(right)
	return leftOK && rightOK && leftName == rightName
}

func catalogItemPrice(kind, itemID string) (int64, error) {
	if kind == sourceKindTemplate {
		item, err := currentSourceStationStore().GetTemplate(itemID)
		if err != nil {
			return 0, err
		}
		return item.PriceCents, nil
	}
	item, err := currentSourceStationStore().GetPlugin(itemID)
	if err != nil {
		return 0, err
	}
	return item.PriceCents, nil
}

func finalizePluginPackage(plugin sourcePlugin) (sourcePlugin, error) {
	loc, sha, err := prepareCatalogPackage(plugin.PriceCents, plugin.DownloadURL, plugin.SHA256, sourceKindPlugin, plugin.ID)
	if err != nil {
		return plugin, err
	}
	plugin.DownloadURL = loc
	plugin.SHA256 = sha
	return plugin, nil
}

func finalizeTemplatePackage(item sourceTemplate) (sourceTemplate, error) {
	loc, sha, err := prepareCatalogPackage(item.PriceCents, item.TemplateURL, item.SHA256, sourceKindTemplate, item.ID)
	if err != nil {
		return item, err
	}
	item.TemplateURL = loc
	item.SHA256 = sha
	return item, nil
}

func finalizeReleasePackage(price int64, rel sourceRelease) (sourceRelease, error) {
	loc, sha, err := prepareCatalogPackage(price, rel.Location, rel.SHA256, rel.Kind, rel.ItemID)
	if err != nil {
		return rel, err
	}
	rel.Location = loc
	rel.SHA256 = sha
	return rel, nil
}

func applyCatalogPrice(cents int64, billing, delivery string) (int64, string, string, error) {
	return normalizeCatalogPrice(cents, billing, delivery)
}

func parseCatalogPriceCents(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	cents, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || cents < 0 {
		return 0, errors.New("价格须为非负整数，单位为分")
	}
	return cents, nil
}

func guardAndFinalizePlugin(plugin sourcePlugin) (sourcePlugin, error) {
	status, latest, existingPrice, err := pluginPriceContext(plugin.ID)
	if err != nil {
		return plugin, err
	}
	if err := rejectPaidPriceOnPublicItem(status, latest, existingPrice, plugin.PriceCents); err != nil {
		return plugin, err
	}
	return finalizePluginPackage(plugin)
}

func guardAndFinalizeTemplate(item sourceTemplate) (sourceTemplate, error) {
	status, latest, existingPrice, err := templatePriceContext(item.ID)
	if err != nil {
		return item, err
	}
	if err := rejectPaidPriceOnPublicItem(status, latest, existingPrice, item.PriceCents); err != nil {
		return item, err
	}
	return finalizeTemplatePackage(item)
}

func guardReleasePackage(rel sourceRelease) (sourceRelease, error) {
	price, err := catalogItemPrice(rel.Kind, rel.ItemID)
	if errors.Is(err, errSourceNotFound) {
		return rel, nil
	}
	if err != nil {
		return rel, err
	}
	return finalizeReleasePackage(price, rel)
}

func pluginPriceContext(id string) (status, latest string, price int64, err error) {
	item, err := currentSourceStationStore().GetPlugin(id)
	if errors.Is(err, errSourceNotFound) {
		return "", "", 0, nil
	}
	if err != nil {
		return "", "", 0, err
	}
	return item.Status, item.LatestVersion, item.PriceCents, nil
}

func templatePriceContext(id string) (status, latest string, price int64, err error) {
	item, err := currentSourceStationStore().GetTemplate(id)
	if errors.Is(err, errSourceNotFound) {
		return "", "", 0, nil
	}
	if err != nil {
		return "", "", 0, err
	}
	return item.Status, item.LatestVersion, item.PriceCents, nil
}

func publicStationPackagePath(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if !stationPackageNamePattern.MatchString(name) {
		return "", false
	}
	dir := stationPackageDir()
	path := filepath.Join(dir, name)
	if !pathInsideDir(dir, path) || pathInsideDir(stationPaidPackageDirPath(), path) {
		return "", false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", false
	}
	return path, true
}

func pathInsideDir(dir, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(target))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != "."
}
