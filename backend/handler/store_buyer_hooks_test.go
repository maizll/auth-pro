package handler

import (
	"net/http"
	"strings"
	"time"
)

// 只在 go test 编译。正式 go build 不包含本文件，因此不能用环境变量或配置文件改源站。
var (
	buyerSourceBaseForTest  string
	sourceHTTPClientForTest *http.Client
)

func init() {
	buyerSourceBase = func() (string, error) {
		base := strings.TrimSpace(buyerSourceBaseForTest)
		if base == "" {
			base = buyerSourceDefault
		}
		return normalizeBuyerSource(base)
	}
	newSourceHTTPClient = func() *http.Client {
		if sourceHTTPClientForTest == nil {
			return defaultSourceHTTPClient()
		}
		client := *sourceHTTPClientForTest
		if client.Timeout == 0 {
			client.Timeout = 20 * time.Second
		}
		if client.CheckRedirect == nil {
			client.CheckRedirect = refuseSourceRedirect
		}
		return &client
	}
}
