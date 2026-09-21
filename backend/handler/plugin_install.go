package handler

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"auto_pro/config"
)

// ponytail: serialize installs per instance; use per-plugin locks if install throughput matters.
var pluginInstallMu sync.Mutex

func installPluginZIP(payload []byte, plugin pluginInfo) error {
	if !pluginIDPattern.MatchString(plugin.ID) {
		return errors.New("插件标识不合法")
	}
	if pluginHasCompiledRuntime(plugin.ID) {
		return errors.New("不能覆盖内置插件")
	}
	pluginInstallMu.Lock()
	defer pluginInstallMu.Unlock()
	root := config.GetPluginDir()
	stage, err := os.MkdirTemp(root, ".install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := extractPackageZIP(payload, stage); err != nil {
		return err
	}
	content, err := packageContentRoot(stage)
	if err != nil {
		return err
	}
	if manifest, err := readLimitedFile(filepath.Join(content, "plugin.json"), pluginManifestMaxSize); err == nil {
		var metadata struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(manifest, &metadata) != nil || metadata.ID != plugin.ID {
			return errors.New("plugin.json 的插件 ID 必须与软件源一致")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// Installation is data-only: never run scripts from downloaded archives.
	plugin.Local, plugin.Remote, plugin.Official, plugin.CanEnable = true, false, false, false
	plugin.Enabled, plugin.Configured = false, false
	plugin.Category = normalizePluginCategory(plugin.Category)
	if strings.TrimSpace(plugin.Name) == "" {
		plugin.Name = plugin.ID
	}
	if plugin.Icon == "" {
		plugin.Icon = "ri:puzzle-line"
	}
	if plugin.Version == "" {
		plugin.Version = "0.0.0"
	}
	metadata, err := json.Marshal(plugin)
	if err != nil {
		return err
	}
	marker := filepath.Join(content, ".installed.json")
	if _, err := os.Lstat(marker); !errors.Is(err, os.ErrNotExist) {
		return errors.New("ZIP 不能包含系统安装状态文件 .installed.json")
	}
	if err := os.WriteFile(marker, metadata, 0644); err != nil {
		return err
	}

	target := filepath.Join(root, plugin.ID)
	// Preserve old plugin.pkg-only downloads until the new installation is complete.
	backup := stage + "-previous"
	hadPrevious := false
	if _, err := os.Lstat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return err
		}
		hadPrevious = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(content, target); err != nil {
		if hadPrevious {
			if restoreErr := os.Rename(backup, target); restoreErr != nil {
				return errors.Join(err, restoreErr)
			}
		}
		return err
	}
	if hadPrevious {
		_ = os.RemoveAll(backup)
	}
	return nil
}

func loadLocalPlugins() ([]pluginInfo, error) {
	plugins := listedCatalogPlugins()
	for i := range plugins {
		plugins[i].Local, plugins[i].CanEnable, plugins[i].Source = true, true, "builtin"
	}
	root := config.GetPluginDir()
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return plugins, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !pluginIDPattern.MatchString(entry.Name()) {
			continue
		}
		// 内置 catalog / 已编译支付渠道优先：同 ID 的历史 ZIP 不得盖掉启用与配置入口。
		if pluginHasCompiledRuntime(entry.Name()) {
			continue
		}
		payload, err := readLimitedFile(filepath.Join(root, entry.Name(), ".installed.json"), pluginManifestMaxSize)
		if err != nil {
			continue
		}
		var plugin pluginInfo
		if json.Unmarshal(payload, &plugin) != nil || plugin.ID != entry.Name() || plugin.Name == "" {
			continue
		}
		if pluginHasCompiledRuntime(plugin.ID) {
			continue
		}
		plugin.Local, plugin.Remote, plugin.Official, plugin.CanEnable = true, false, false, false
		plugins = append(plugins, plugin)
	}
	return plugins, nil
}
