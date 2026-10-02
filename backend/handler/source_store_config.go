package handler

import (
	"errors"
	"regexp"
	"strings"
)

// sourceStoreSettings 是某个商业版应用写进快照的设置：离线宽限天数、改密是否撤销绑定和开放的功能键。
type sourceStoreSettings struct {
	GraceDays              int      `json:"graceDays"`
	RevokeOnPasswordChange *bool    `json:"revokeOnPasswordChange"`
	CommercialFeatures     []string `json:"commercialFeatures"`
}

// normalizeStoreSettings 校验并补齐默认值：宽限 1 到 30 天（默认 7 天），改密默认撤销，功能键默认多应用。
func normalizeStoreSettings(in sourceStoreSettings) (sourceStoreSettings, error) {
	days := in.GraceDays
	if days == 0 {
		days = storeGraceDefaultDays
	}
	if days < 1 || days > 30 {
		return sourceStoreSettings{}, errors.New("离线宽限天数须在 1 到 30 之间")
	}
	revoke := true
	if in.RevokeOnPasswordChange != nil {
		revoke = *in.RevokeOnPasswordChange
	}
	features := in.CommercialFeatures
	if features == nil {
		features = []string{storeFeatureMultiApp}
	}
	cleaned := make([]string, 0, len(features))
	seen := map[string]bool{}
	for _, feature := range features {
		feature = strings.TrimSpace(feature)
		if feature == "" || seen[feature] {
			continue
		}
		if !storeFeatureKeyPattern.MatchString(feature) {
			return sourceStoreSettings{}, errors.New("功能键不合法")
		}
		seen[feature] = true
		cleaned = append(cleaned, feature)
	}
	return sourceStoreSettings{GraceDays: days, RevokeOnPasswordChange: &revoke, CommercialFeatures: cleaned}, nil
}

var storeFeatureKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

func splitStoreFeatures(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
