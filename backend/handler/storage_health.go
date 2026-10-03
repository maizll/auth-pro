package handler

import (
	"context"
	"strings"
	"time"

	"auto_pro/config"
)

// checkStorageLocations 检查每个启用的存储能不能连上，以及已登记的安装包是否还在。
// 只写检查结果和站内通知，不修改目录上下架。定时任务和监控页的「立即检查」都走这里。
func checkStorageLocations(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	blob, err := loadStorageBlob()
	if err != nil {
		return
	}
	seen := map[string]bool{}
	for _, row := range blob.Health {
		if row.Level == "problem" {
			seen[row.LocationID+"/"+row.Target] = true
		}
	}
	rows := make([]storageHealthRow, 0)
	for _, loc := range blob.Locations {
		if loc.Role == storageRoleDisabled {
			continue
		}
		secret, openErr := locationSecret(loc)
		if openErr != nil {
			rows = append(rows, storageHealthRow{
				Level: "problem", LocationID: loc.ID, LocationName: loc.Name, Target: "连接",
				Message: loc.Name + "：密钥无法读取。存储位置仍保留，相关安装包不会自动下架。",
			})
			continue
		}
		if probeErr := probeStorageLocation(ctx, loc, secret); probeErr != nil {
			rows = append(rows, storageHealthRow{
				Level: "problem", LocationID: loc.ID, LocationName: loc.Name, Target: "连接",
				Message: loc.Name + "：" + healthSentence(probeErr) + "相关安装包不会自动下架。",
			})
			continue
		}
		rows = append(rows, storageHealthRow{
			Level: "ok", LocationID: loc.ID, LocationName: loc.Name, Target: "连接",
			Message: loc.Name + "可以连接。",
		})
	}
	for _, copy := range blob.Copies {
		loc, key, ok := locationByRef(blob.Locations, copy.Ref)
		if !ok {
			rows = append(rows, storageHealthRow{
				Level: "problem", Target: copy.ItemID + "@" + copy.Version,
				Message: "安装包 " + copy.ItemID + " " + copy.Version + " 的存储位置已经不在列表里。条目仍在目录中，不会自动下架。",
			})
			continue
		}
		if loc.Role == storageRoleDisabled {
			continue
		}
		secret, openErr := locationSecret(loc)
		if openErr != nil {
			continue
		}
		exists, existsErr := objectExists(ctx, loc, secret, key)
		if existsErr == nil && exists {
			continue
		}
		text := "在「" + loc.Name + "」里找不到安装包 " + copy.ItemID + " " + copy.Version + "。条目仍在目录中，不会自动下架。"
		if existsErr != nil {
			text = "暂时无法确认「" + loc.Name + "」里的安装包 " + copy.ItemID + " " + copy.Version + "：" + healthSentence(existsErr) + "不会自动下架。"
		}
		rows = append(rows, storageHealthRow{
			Level: "problem", LocationID: loc.ID, LocationName: loc.Name,
			Target: copy.ItemID + "@" + copy.Version, Message: text,
		})
	}
	rows = append(rows, appRepoHealthRows(ctx)...)
	blob.Health = rows
	blob.CheckedAt = time.Now().UTC()
	_ = saveStorageBlob(blob)
	for _, row := range rows {
		if row.Level != "problem" || seen[row.LocationID+"/"+row.Target] {
			continue
		}
		notifyAllAdmins(notificationTabNotice, "存储检查发现问题", row.Message, "/source-station/storage-monitor", "storage_health", "storage", row.LocationID+"/"+row.Target)
	}
}

func storageReadyForPaid() bool {
	if githubPaidRepoConfigured() {
		return true
	}
	blob, err := loadStorageBlob()
	if err != nil {
		return false
	}
	return len(enabledStorageLocations(blob.Locations)) > 0
}

func knownStorageRefs(blob storageConfigBlob) map[string]storageCopy {
	out := map[string]storageCopy{}
	for _, copy := range blob.Copies {
		out[copy.Ref] = copy
		if _, key, ok := locationByRef(blob.Locations, copy.Ref); ok {
			out[key] = copy
		}
		if ref, ok := parseGitHubPackageRef(copy.Ref); ok {
			out[ref.Asset] = copy
			out[ref.Owner+"/"+ref.Repo+"/"+ref.Tag+"/"+ref.Asset] = copy
		}
		if ref, ok := parseGiteePackageRef(copy.Ref); ok {
			out[ref.Asset] = copy
			out[ref.Owner+"/"+ref.Repo+"/"+ref.Tag+"/"+ref.Asset] = copy
		}
	}
	return out
}

func catalogObjectIndex() map[string]string {
	out := map[string]string{}
	add := func(kind, id, version, location, objectKey string) {
		label := strings.TrimSpace(id)
		if version != "" {
			label += " " + version
		}
		if kind != "" {
			label = kind + " " + label
		}
		for _, key := range []string{strings.TrimSpace(location), strings.TrimSpace(objectKey)} {
			if key != "" {
				out[key] = strings.TrimSpace(label)
			}
		}
	}
	store := currentSourceStationStore()
	switch typed := store.(type) {
	case *memorySourceStore:
		typed.mu.Lock()
		defer typed.mu.Unlock()
		for id, versions := range typed.pluginVersions {
			for ver, rel := range versions {
				add(sourceKindPlugin, id, ver, rel.Location, rel.ObjectKey)
			}
		}
		for id, versions := range typed.templateVersions {
			for ver, rel := range versions {
				add(sourceKindTemplate, id, ver, rel.Location, rel.ObjectKey)
			}
		}
		for id, item := range typed.plugins {
			add(sourceKindPlugin, id, item.Version, item.DownloadURL, "")
		}
		for id, item := range typed.templates {
			add(sourceKindTemplate, id, item.Version, item.TemplateURL, "")
		}
		return out
	}
	db, err := config.DB()
	if err != nil {
		return out
	}
	rows, err := db.Query(`SELECT 'plugin', plugin_id, version, download_url, object_key FROM source_catalog_plugin_versions
		UNION ALL
		SELECT 'template', template_id, version, template_url, object_key FROM source_catalog_template_versions`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var kind, id, version, location, objectKey string
		if err := rows.Scan(&kind, &id, &version, &location, &objectKey); err != nil {
			continue
		}
		add(kind, id, version, location, objectKey)
	}
	return out
}

// healthSentence 把错误说明收成一句以句号结尾的话。底层错误多数自带句号，直接再补会出现「。。」。
func healthSentence(err error) string {
	return strings.TrimRight(strings.TrimSpace(err.Error()), "。.！!") + "。"
}
