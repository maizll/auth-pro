package handler

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

const defaultSafeRedirects = 3

var (
	errSafePrivateAddress  = errors.New("拒绝访问非公网地址")
	errSafeMetadataAddress = errors.New("拒绝访问链路本地或云元数据地址")
	errSafeHTTPSRequired   = errors.New("必须是 https:// 外部地址")
	errSafeRedirect        = errors.New("重定向过多")
	errSafeRedirectScheme  = errors.New("外链重定向离开 https")
	errSafeBadURL          = errors.New("地址不合法")
	errSafeTooLarge        = errors.New("响应超过大小限制")

	errPluginSHAMissing  = errors.New("远程插件包缺少 sha256，已拒绝安装")
	errPluginSHAMismatch = errors.New("插件包 SHA256 与清单不一致")
)

var safePolicyErrors = []error{
	errSafePrivateAddress,
	errSafeMetadataAddress,
	errSafeHTTPSRequired,
	errSafeRedirect,
	errSafeRedirectScheme,
	errSafeBadURL,
}

type safeFetchOptions struct {
	AllowPrivate bool
	RequireHTTPS bool
	MaxBytes     int64
	Timeout      time.Duration
	MaxRedirects int
	UserAgent    string
	Accept       string
	BaseClient   *http.Client
}

type safeFetchHooks struct {
	resolve func(context.Context, string) ([]net.IP, error)
	dial    func(context.Context, string, string) (net.Conn, error)
	tls     *tls.Config
}

// safeFetchHookState 让测试把解析和拨号换成固定结果。生产环境保持为空。
var safeFetchHookState atomic.Value

func init() {
	safeFetchHookState.Store(safeFetchHooks{})
}

func currentSafeFetchHooks() safeFetchHooks {
	return safeFetchHookState.Load().(safeFetchHooks)
}

func setSafeFetchHooks(hooks safeFetchHooks) {
	safeFetchHookState.Store(hooks)
}

func isSafeFetchPolicyError(err error) bool {
	for _, sentinel := range safePolicyErrors {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return false
}

func normalizeFetchError(err error) error {
	for _, sentinel := range safePolicyErrors {
		if errors.Is(err, sentinel) {
			return sentinel
		}
	}
	if errors.Is(err, errSafeTooLarge) {
		return errSafeTooLarge
	}
	return err
}

func isMetadataHostname(host string) bool {
	switch strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".") {
	case "metadata.google.internal", "metadata.goog":
		return true
	default:
		return false
	}
}

func isMetadataIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, raw := range []string{"169.254.169.254", "169.254.170.2", "fd00:ec2::254"} {
		if ip.Equal(net.ParseIP(raw)) {
			return true
		}
	}
	return false
}

func isCGNAT(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return ip4[0] == 100 && ip4[1]&0xc0 == 64
}

func fetchIPAllowed(ip net.IP, allowPrivate bool) error {
	if ip == nil {
		return errSafePrivateAddress
	}
	if isMetadataIP(ip) || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || isCGNAT(ip) {
		return errSafeMetadataAddress
	}
	if ip.IsUnspecified() || ip.IsMulticast() || ip.Equal(net.IPv4bcast) {
		return errSafePrivateAddress
	}
	if ip.IsLoopback() || ip.IsPrivate() {
		if allowPrivate {
			return nil
		}
		return errSafePrivateAddress
	}
	if !ip.IsGlobalUnicast() {
		return errSafePrivateAddress
	}
	return nil
}

func activeResolve(ctx context.Context, host string) ([]net.IP, error) {
	if resolve := currentSafeFetchHooks().resolve; resolve != nil {
		return resolve(ctx, host)
	}
	return defaultSafeResolve(ctx, host)
}

func defaultSafeResolve(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	if isMetadataHostname(host) {
		return nil, errSafeMetadataAddress
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		if addr.IP != nil {
			ips = append(ips, addr.IP)
		}
	}
	return ips, nil
}

func activeDial(ctx context.Context, network, address string) (net.Conn, error) {
	if dial := currentSafeFetchHooks().dial; dial != nil {
		return dial(ctx, network, address)
	}
	return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, address)
}

func selectPinnedIP(ips []net.IP, allowPrivate bool) (net.IP, error) {
	if len(ips) == 0 {
		return nil, errSafePrivateAddress
	}
	var chosen net.IP
	for _, ip := range ips {
		if err := fetchIPAllowed(ip, allowPrivate); err != nil {
			return nil, err
		}
		if chosen == nil {
			chosen = ip
		}
	}
	return chosen, nil
}

func assertSafeFetchHost(ctx context.Context, host string, allowPrivate bool) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return errSafeBadURL
	}
	if isMetadataHostname(host) {
		return errSafeMetadataAddress
	}
	ips, err := activeResolve(ctx, host)
	if err != nil {
		if isSafeFetchPolicyError(err) {
			return normalizeFetchError(err)
		}
		return err
	}
	_, err = selectPinnedIP(ips, allowPrivate)
	return err
}

func validateSafeFetchURL(parsed *url.URL, opts safeFetchOptions, previousScheme string) error {
	if parsed == nil || parsed.Hostname() == "" || parsed.User != nil {
		return errSafeBadURL
	}
	if strings.Contains(parsed.Path, "..") || strings.Contains(parsed.EscapedPath(), "..") {
		return errSafeBadURL
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return errSafeBadURL
	}
	if opts.RequireHTTPS && scheme != "https" {
		return errSafeHTTPSRequired
	}
	if strings.EqualFold(previousScheme, "https") && scheme != "https" {
		return errSafeRedirectScheme
	}
	return nil
}

// pluginSourceAllowsPrivate 只认管理员写在源地址里的字面量回环或私网 IP。
// 主机名一律按公网策略处理，避免域名重新绑定后把公网源当成内网源。
func pluginSourceAllowsPrivate(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	ip := net.ParseIP(parsed.Hostname())
	if ip == nil {
		return false
	}
	if fetchIPAllowed(ip, true) != nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}

func safeHTTPGet(ctx context.Context, rawURL string, opts safeFetchOptions) ([]byte, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 20 * time.Second
	}
	if opts.MaxRedirects <= 0 {
		opts.MaxRedirects = defaultSafeRedirects
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, errSafeBadURL
	}
	if err := validateSafeFetchURL(parsed, opts, ""); err != nil {
		return nil, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	if err := assertSafeFetchHost(reqCtx, parsed.Hostname(), opts.AllowPrivate); err != nil {
		return nil, normalizeFetchError(err)
	}
	request, err := http.NewRequestWithContext(reqCtx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, errSafeBadURL
	}
	if opts.UserAgent != "" {
		request.Header.Set("User-Agent", opts.UserAgent)
	}
	if opts.Accept != "" {
		request.Header.Set("Accept", opts.Accept)
	}
	response, err := newSafeHTTPClient(opts).Do(request)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil {
		return nil, normalizeFetchError(err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &safeStatusError{Code: response.StatusCode}
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, opts.MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if opts.MaxBytes > 0 && int64(len(payload)) > opts.MaxBytes {
		return nil, errSafeTooLarge
	}
	return payload, nil
}

type safeStatusError struct {
	Code int
}

func (e *safeStatusError) Error() string {
	return fmt.Sprintf("下载地址返回状态码 %d", e.Code)
}

func newSafeHTTPClient(opts safeFetchOptions) *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           pinnedDial(opts.AllowPrivate),
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: opts.Timeout,
		IdleConnTimeout:       opts.Timeout,
		MaxIdleConns:          2,
		TLSNextProto:          map[string]func(string, *tls.Conn) http.RoundTripper{},
	}
	if opts.BaseClient != nil {
		if base, ok := opts.BaseClient.Transport.(*http.Transport); ok && base.TLSClientConfig != nil {
			transport.TLSClientConfig = base.TLSClientConfig.Clone()
		}
	}
	if transport.TLSClientConfig == nil {
		if cfg := currentSafeFetchHooks().tls; cfg != nil {
			transport.TLSClientConfig = cfg.Clone()
		}
	}
	return &http.Client{
		Timeout:   opts.Timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= opts.MaxRedirects {
				return errSafeRedirect
			}
			if req == nil || req.URL == nil {
				return errSafeBadURL
			}
			previous := ""
			if len(via) > 0 && via[len(via)-1] != nil && via[len(via)-1].URL != nil {
				previous = via[len(via)-1].URL.Scheme
			}
			if err := validateSafeFetchURL(req.URL, opts, previous); err != nil {
				return err
			}
			return assertSafeFetchHost(req.Context(), req.URL.Hostname(), opts.AllowPrivate)
		},
	}
}

func pinnedDial(allowPrivate bool) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errSafeBadURL
		}
		if isMetadataHostname(host) {
			return nil, errSafeMetadataAddress
		}
		ips, err := activeResolve(ctx, host)
		if err != nil {
			return nil, normalizeFetchError(err)
		}
		chosen, err := selectPinnedIP(ips, allowPrivate)
		if err != nil {
			return nil, err
		}
		return activeDial(ctx, network, net.JoinHostPort(chosen.String(), port))
	}
}
