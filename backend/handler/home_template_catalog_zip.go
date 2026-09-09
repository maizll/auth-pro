package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"auto_pro/softwaresource"
)

// The catalog hashes the original ZIP; the entry hash covers HTML after local
// resource rebasing. Keep both so updates do not disable a working installation.
type catalogTemplateInstallation struct {
	SHA256      string `json:"sha256"`
	Version     string `json:"version,omitempty"`
	EntrySHA256 string `json:"entrySHA256"`
}

const catalogTemplateInstallationFile = ".catalog-template.json"
const catalogTemplatePackageFile = ".template-package.zip"

func installCatalogHomeTemplateZIP(payload []byte, remote softwaresource.Template) (string, error) {
	checksum := actualChecksumString(sha256.Sum256(payload))
	if !strings.EqualFold(checksum, remote.SHA256) {
		return "", errors.New("软件源模板 ZIP 的 SHA256 校验失败")
	}
	installedPath, entryChecksum, err := installUploadedHomeTemplateZIP(payload)
	if err != nil {
		return "", err
	}
	saved := false
	defer func() {
		if !saved {
			_ = os.RemoveAll(filepath.Dir(installedPath))
		}
	}()
	schemaVersion := 1
	if filepath.Ext(installedPath) == ".html" {
		schemaVersion = 0
	}
	if remote.SchemaVersion != schemaVersion {
		return "", errors.New("软件源模板目录与 ZIP 入口类型不一致，请重新发布")
	}
	record, err := json.Marshal(catalogTemplateInstallation{SHA256: checksum, EntrySHA256: entryChecksum, Version: remote.Version})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(installedPath), catalogTemplateInstallationFile), record, 0600); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(installedPath), catalogTemplatePackageFile), payload, 0600); err != nil {
		return "", err
	}
	saved = true
	return installedPath, nil
}

func readCatalogTemplateInstallation(installedPath string) (catalogTemplateInstallation, error) {
	var installation catalogTemplateInstallation
	root, err := uploadedHomeTemplateRoot(installedPath)
	if err != nil {
		return installation, err
	}
	payload, err := readLimitedFile(filepath.Join(root, catalogTemplateInstallationFile), 4096)
	if err != nil {
		return installation, err
	}
	if err := json.Unmarshal(payload, &installation); err != nil {
		return installation, err
	}
	for _, checksum := range []string{installation.SHA256, installation.EntrySHA256} {
		if decoded, err := hex.DecodeString(checksum); err != nil || len(decoded) != sha256.Size {
			return installation, errors.New("模板安装校验记录损坏，请重新安装")
		}
	}
	return installation, nil
}
