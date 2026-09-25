package handler

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"time"
)

const (
	storeFeatureMultiApp               = "multi_app"
	storeEditionFree                   = "free"
	storeEditionCommercial             = "commercial"
	storePeriodPermanent               = "permanent"
	storePeriodYearly                  = "yearly"
	storeOrderPrefix                   = "PP"
	storeGraceDefaultDays              = 7
	storeBindChallengeTTL              = 2 * time.Minute
	storeOrderTTL                      = 30 * time.Minute
	storeDownloadTTL                   = 10 * time.Minute
	storeSignWindow                    = 300 * time.Second
	storeDomainChangeWindow            = 30 * 24 * time.Hour
	storeBindingIdleTTL                = 180 * 24 * time.Hour
	storeEditionRequiredCode           = 402
	storeEditionRequiredMsg            = "该功能需要商业版"
	storeReasonSnapshotKeyUnconfigured = "snapshot_key_unconfigured"
)

var (
	errStoreAmountMismatch = errors.New("金额不符")
	errStoreOrderClosed    = errors.New("订单状态不可支付")
	errStoreOrderNotFound  = errors.New("订单不存在")
	errStoreNoRedirect     = errors.New("回调不跟随重定向")
	errStorePrivateDomain  = errors.New("请用正式域名（HTTPS）访问后台后再绑定")
	errStoreDomainOccupied = errors.New("域名已绑定在其他账号下")
	errStoreLicenseType    = errors.New("主授权只接受域名授权")
	errStoreDownloadDenied = errors.New("当前授权不能下载该付费包")
)

// storeDomainShapeRejected 拒绝 localhost、IP 字面量和内网保留域名。
// 解析结果是否私网由调用方再用 fetchIPAllowed 判断。
func storeDomainShapeRejected(domain string) bool {
	domain = normalizeLicenseDomain(domain)
	if domain == "" {
		return true
	}
	if net.ParseIP(domain) != nil {
		return true
	}
	host := strings.ToLower(domain)
	switch host {
	case "localhost", "localhost.localdomain":
		return true
	}
	if strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".localdomain") {
		return true
	}
	for _, suffix := range []string{".home.arpa", ".internal", ".intranet", ".lan"} {
		if host == strings.TrimPrefix(suffix, ".") || strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

type mainLicenseMatch struct {
	ID        int64
	OwnerType string
	OwnerID   int64
	Type      string
	Status    string
}

// decideMainLicense 决定绑定域名时关联、新建，还是拒绝。
// 只接受 domain 类型。域名属于其他账号时拒绝，不自动转移。
func decideMainLicense(accountType string, accountID int64, matches []mainLicenseMatch) (action string, licenseID int64, err error) {
	for _, item := range matches {
		if item.Status == "revoked" || item.Status == "expired" {
			continue
		}
		if item.Type != "domain" {
			if item.OwnerType != accountType || item.OwnerID != accountID {
				return "", 0, errStoreDomainOccupied
			}
			return "", 0, errStoreLicenseType
		}
		if item.OwnerType != accountType || item.OwnerID != accountID {
			return "", 0, errStoreDomainOccupied
		}
		return "associate", item.ID, nil
	}
	return "create", 0, nil
}

func domainChangeAllowed(last *time.Time, now time.Time, admin bool) bool {
	if admin || last == nil {
		return true
	}
	return now.Sub(*last) >= storeDomainChangeWindow
}

func shouldGrantStoreOrder(status string, amountCents, paidCents int64) (bool, error) {
	if paidCents != amountCents {
		return false, errStoreAmountMismatch
	}
	if status == "paid" {
		return false, nil
	}
	if status != "pending" {
		return false, errStoreOrderClosed
	}
	return true, nil
}

func onlineSettlementRoute(orderNo string) string {
	switch {
	case strings.HasPrefix(orderNo, "AU"):
		return "upgrade"
	case strings.HasPrefix(orderNo, storeOrderPrefix):
		return "store"
	default:
		return "recharge_or_license"
	}
}

func nextEditionExpiry(period string, current *time.Time, now time.Time) *time.Time {
	if period != storePeriodYearly {
		return nil
	}
	base := now
	if current != nil && current.After(now) {
		base = *current
	}
	next := base.AddDate(0, 0, 365)
	return &next
}

func appCreateDecision(existingCount int, commercial bool) bool {
	if existingCount < 1 {
		return true
	}
	return commercial
}

type paidAccessInput struct {
	SnapshotOK      bool
	Edition         string
	SnapshotDomain  string
	RequestDomain   string
	Now             time.Time
	GraceUntil      time.Time
	ExplicitRevoked bool
	Offline         bool
}

// evaluatePaidAccess 只读本地快照。明确吊销、过期、退款立即失去付费能力；
// 只有连不上源站才吃宽限。域名不一致按免费版处理，不消耗宽限。
func evaluatePaidAccess(in paidAccessInput) (tier, reason string) {
	if !in.SnapshotOK {
		return storeEditionFree, "no_snapshot"
	}
	requestDomain := normalizeLicenseDomain(in.RequestDomain)
	snapshotDomain := normalizeLicenseDomain(in.SnapshotDomain)
	if requestDomain == "" || requestDomain != snapshotDomain {
		return storeEditionFree, "domain_mismatch"
	}
	if in.ExplicitRevoked || in.Edition != storeEditionCommercial {
		if in.ExplicitRevoked {
			return storeEditionFree, "revoked"
		}
		return storeEditionFree, "free"
	}
	if in.Offline && in.Now.After(in.GraceUntil) {
		return storeEditionFree, "grace_expired"
	}
	return storeEditionCommercial, ""
}

func storeDownloadAllowed(expiresAt, now time.Time, entitled bool) error {
	if !now.Before(expiresAt) {
		return errors.New("下载令牌已过期")
	}
	if !entitled {
		return errStoreDownloadDenied
	}
	return nil
}

func acceptablePayURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	lower := strings.ToLower(raw)
	return strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "alipays://") || strings.HasPrefix(lower, "weixin://")
}

func parseHTTPSBase(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("源站地址必须是 https")
	}
	return parsed, nil
}

func sameOriginURL(base, raw string) bool {
	baseURL, err := parseHTTPSBase(base)
	if err != nil {
		return false
	}
	next, err := parseHTTPSBase(raw)
	if err != nil {
		return false
	}
	return strings.EqualFold(baseURL.Scheme, next.Scheme) && strings.EqualFold(baseURL.Host, next.Host)
}

func ownershipForPrice(priceCents int64, commercial, purchased bool) string {
	if priceCents <= 0 {
		return "free"
	}
	if purchased {
		return "purchased"
	}
	if commercial {
		return "included"
	}
	return "none"
}
