package handler

import (
	"strings"
	"time"
)

// migrateLegacyStorageLocked 把旧的两处设置收成存储列表。
// 收费仓库（github_paid_*）原来只给收费安装包用，有配置时作为主存储。
// 发布仓库（release 设置）原来只在勾选「推送 Release」时使用，和收费仓库互不兜底。
// 两处都填过时各成一条：收费仓库为主，发布仓库为备用。只填了一处时那一条就是主存储。
// 令牌从旧字段读出后重新加密，旧字段仍保留，已有上传和发布路径还能读。
func migrateLegacyStorageLocked(blob *storageConfigBlob) error {
	if blob == nil {
		return nil
	}
	now := time.Now().UTC()
	if owner, repo, token, err := loadGitHubPaidRepo(); err == nil && owner != "" && repo != "" && token != "" {
		sealed, sealErr := sealStorageSecret(token)
		if sealErr != nil {
			return sealErr
		}
		blob.Locations = append(blob.Locations, storageLocation{
			ID: newStorageID(), Name: "收费仓库", Kind: packageStorageGitHub, Role: storageRolePrimary,
			Legacy: storageLegacyPaid, Owner: owner, Repo: repo, SecretSealed: sealed,
			CreatedAt: now, UpdatedAt: now,
		})
	}
	settings, err := currentSourceStationStore().GetReleaseSettings()
	if err != nil {
		return err
	}
	settings = normalizeReleaseSettings(settings)
	if settings.releaseReady() {
		sealed, sealErr := sealStorageSecret(settings.Token)
		if sealErr != nil {
			return sealErr
		}
		role := storageRolePrimary
		if len(enabledStorageLocations(blob.Locations)) > 0 {
			role = storageRoleBackup
		}
		name := "发布仓库"
		if settings.Provider == packageStorageGitee {
			name = "发布仓库（Gitee）"
		} else {
			name = "发布仓库（GitHub）"
		}
		blob.Locations = append(blob.Locations, storageLocation{
			ID: newStorageID(), Name: name, Kind: settings.Provider, Role: role,
			Legacy: storageLegacyRelease, Owner: settings.Owner, Repo: settings.Repo,
			Branch: settings.Branch, SecretSealed: sealed, CreatedAt: now, UpdatedAt: now,
		})
	}
	return nil
}

// syncLegacyStorageLocations 在旧设置接口保存成功后，把对应的那一条存储位置一起更新。
// 这样测试和尚未改完的调用方写旧字段时，存储列表不会还停在迁移前的令牌。
func syncLegacyStorageLocations() error {
	blob, err := loadStorageBlob()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	changed := false
	if owner, repo, token, readErr := loadGitHubPaidRepo(); readErr == nil && owner != "" && repo != "" && token != "" {
		sealed, sealErr := sealStorageSecret(token)
		if sealErr != nil {
			return sealErr
		}
		if !upsertLegacyLocation(&blob, storageLegacyPaid, storageLocation{
			Name: "收费仓库", Kind: packageStorageGitHub, Owner: owner, Repo: repo, SecretSealed: sealed,
		}, now) {
			changed = true
		} else {
			changed = true
		}
	}
	settings, err := currentSourceStationStore().GetReleaseSettings()
	if err != nil {
		return err
	}
	settings = normalizeReleaseSettings(settings)
	if settings.releaseReady() {
		sealed, sealErr := sealStorageSecret(settings.Token)
		if sealErr != nil {
			return sealErr
		}
		name := "发布仓库（GitHub）"
		if settings.Provider == packageStorageGitee {
			name = "发布仓库（Gitee）"
		}
		upsertLegacyLocation(&blob, storageLegacyRelease, storageLocation{
			Name: name, Kind: settings.Provider, Owner: settings.Owner, Repo: settings.Repo,
			Branch: settings.Branch, SecretSealed: sealed,
		}, now)
		changed = true
	}
	if !changed {
		return nil
	}
	return saveStorageBlob(blob)
}

func upsertLegacyLocation(blob *storageConfigBlob, legacy string, incoming storageLocation, now time.Time) bool {
	for i := range blob.Locations {
		if blob.Locations[i].Legacy != legacy {
			continue
		}
		loc := blob.Locations[i]
		loc.Name = incoming.Name
		loc.Kind = incoming.Kind
		loc.Owner = incoming.Owner
		loc.Repo = incoming.Repo
		loc.Branch = incoming.Branch
		loc.SecretSealed = incoming.SecretSealed
		loc.UpdatedAt = now
		if loc.Role == "" {
			loc.Role = storageRoleBackup
		}
		blob.Locations[i] = loc
		return true
	}
	role := storageRoleBackup
	if len(enabledStorageLocations(blob.Locations)) == 0 {
		role = storageRolePrimary
	}
	if legacy == storageLegacyPaid {
		role = storageRolePrimary
		blob.Locations = demoteOtherPrimaries(blob.Locations, "")
	}
	incoming.ID = newStorageID()
	incoming.Legacy = legacy
	incoming.Role = role
	incoming.CreatedAt = now
	incoming.UpdatedAt = now
	if role == storageRolePrimary {
		blob.Locations = demoteOtherPrimaries(blob.Locations, incoming.ID)
	}
	blob.Locations = append(blob.Locations, incoming)
	return false
}

func mirrorLegacyFromLocation(loc storageLocation, secret string) error {
	secret = strings.TrimSpace(secret)
	switch loc.Legacy {
	case storageLegacyPaid:
		if loc.Kind != packageStorageGitHub || secret == "" {
			return nil
		}
		sealed, err := sealGitHubPaidToken(secret)
		if err != nil {
			return err
		}
		if err := writeGitHubPaidTokenSealed(sealed); err != nil {
			return err
		}
		if err := writeGitHubPaidSetting(githubPaidOwnerSettingKey, loc.Owner); err != nil {
			return err
		}
		return writeGitHubPaidSetting(githubPaidRepoSettingKey, loc.Repo)
	case storageLegacyRelease:
		current, err := currentSourceStationStore().GetReleaseSettings()
		if err != nil {
			return err
		}
		current = normalizeReleaseSettings(current)
		current.Provider = loc.Kind
		current.Owner = loc.Owner
		current.Repo = loc.Repo
		if strings.TrimSpace(loc.Branch) != "" {
			current.Branch = loc.Branch
		}
		if secret != "" {
			current.Token = secret
		}
		return currentSourceStationStore().SaveReleaseSettings(current)
	default:
		return nil
	}
}
