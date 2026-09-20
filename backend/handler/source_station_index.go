package handler

import (
	"net"
	"net/url"
	"strings"
)

func persistIndexSnapshot(actor string) {
	publishPublicSoftwareSourceIndex(actor)
}

// publishPublicSoftwareSourceIndex rebuilds each app's public catalog from
// currently published rows and writes the snapshot consumers (and the manual
// regenerate button) share. Local software-source URLs that point at this
// instance skip the 5-minute HTTP cache and read the live index instead.
func publishPublicSoftwareSourceIndex(actor string) {
	apps, err := currentSourceStationStore().ListCatalogApps()
	if err != nil {
		return
	}
	for _, app := range apps {
		payload, _, err := sourceCatalogJSONForApp(app)
		if err != nil {
			continue
		}
		_ = currentSourceStationStore().SaveIndexSnapshot(string(payload), actor)
	}
	refreshLocalSoftwareSourceCaches()
}

func lookupLocalSoftwareSourceIndex(rawURL string) ([]byte, *remotePluginIndex, bool) {
	appKey := parsePublicSoftwareSourceAppKey(rawURL)
	if appKey == "" || !isLoopbackSoftwareSourceURL(rawURL) {
		return nil, nil, false
	}
	app, err := currentSourceStationStore().GetCatalogAppByKey(appKey)
	if err != nil {
		return nil, nil, false
	}
	payload, _, err := sourceCatalogJSONForApp(app)
	if err != nil {
		return nil, nil, false
	}
	index, err := parsePluginSourceManifest(payload)
	if err != nil {
		return nil, nil, false
	}
	return payload, index, true
}

func parsePublicSoftwareSourceAppKey(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Path == "" {
		return ""
	}
	path := strings.TrimSuffix(parsed.Path, "/")
	for _, prefix := range []string{"/software-source/", "/auth-pro/"} {
		rest, ok := strings.CutPrefix(path, prefix)
		if !ok {
			continue
		}
		appKey, found := strings.CutSuffix(rest, "/index.json")
		if !found || appKey == "" || strings.Contains(appKey, "/") {
			continue
		}
		return appKey
	}
	if path == "/software-source/index.json" || path == "/auth-pro/index.json" {
		if key := strings.TrimSpace(parsed.Query().Get("app_key")); key != "" {
			return key
		}
		return strings.TrimSpace(parsed.Query().Get("appKey"))
	}
	return ""
}

func isLoopbackSoftwareSourceURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return true
	}
	if host == "localhost" || host == "0.0.0.0" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func refreshLocalSoftwareSourceCaches() {
	db, err := openSystemConfigDB()
	if err != nil {
		return
	}
	if err := ensurePluginStorage(db); err != nil {
		return
	}
	sources, err := listPluginSources(db)
	if err != nil {
		return
	}
	for _, source := range sources {
		payload, _, ok := lookupLocalSoftwareSourceIndex(source.URL)
		if !ok {
			continue
		}
		_ = cachePluginSourceManifest(db, source.ID, "json", payload, "")
	}
}
