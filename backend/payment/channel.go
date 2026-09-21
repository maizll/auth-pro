// Package payment 定义 AuthPro 支付渠道插件契约（SPI）。
//
// 内置易支付 V1/V2 仍走 handler 中的既有实现，不经过本注册表，以保证行为不变。
// 官方商店插件（如支付宝当面付）在 init 中调用 Register，由 handler 在
// pay-options / 下单 / 异步通知中查询本注册表。
package payment

import (
	"database/sql"
	"sort"
	"sync"
)

const (
	// ModeRedirect 表示需要浏览器跳转到 PayURL（易支付类收银台）。
	ModeRedirect = "redirect"
	// ModeQRCode 表示需要在本站展示 QRCode（当面付正扫）。
	ModeQRCode = "qrcode"
)

// Option 是收银台可选的一条支付方式，JSON 字段与现有 pay-options 对齐。
type Option struct {
	Code    string `json:"code"`
	Channel string `json:"channel,omitempty"`
	PayType string `json:"payType,omitempty"`
	Label   string `json:"label"`
	Icon    string `json:"icon"`
	Color   string `json:"color"`
}

// CreateRequest 是渠道下单入参。NotifyURL 由核心按渠道 id 生成，插件不得改写路径。
type CreateRequest struct {
	OrderNo     string
	AmountCents int64
	Subject     string
	PayType     string
	NotifyURL   string
	ReturnURL   string
	ClientIP    string
}

// CreateResult 是渠道下单结果。QR 渠道填 QRCode + ModeQRCode；跳转渠道填 PayURL。
type CreateResult struct {
	Mode   string
	PayURL string
	QRCode string
}

// NotifyResult 是验签通过后的标准化支付结果，由核心写入既有订单表。
type NotifyResult struct {
	OrderNo        string
	AmountCents    int64
	PayType        string
	GatewayTradeNo string
	Success        bool
	RawPayload     string
}

// Channel 是支付渠道插件必须实现的最小契约。
type Channel interface {
	ID() string
	PluginID() string
	Options() []Option
	Available(db *sql.DB) bool
	CreatePayment(db *sql.DB, req CreateRequest) (CreateResult, error)
	ParseNotify(db *sql.DB, values map[string]string) (NotifyResult, error)
}

var (
	registryMu sync.RWMutex
	channels   = map[string]Channel{}
)

// Register 注册一个支付渠道。同一 id 重复注册会覆盖（便于测试）。
func Register(ch Channel) {
	if ch == nil || ch.ID() == "" {
		return
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	channels[ch.ID()] = ch
}

// Get 按渠道 id 查找已注册实现。
func Get(id string) (Channel, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	ch, ok := channels[id]
	return ch, ok
}

// Has 判断渠道 id 是否已注册。
func Has(id string) bool {
	_, ok := Get(id)
	return ok
}

// List 返回稳定排序后的已注册渠道。
func List() []Channel {
	registryMu.RLock()
	defer registryMu.RUnlock()
	ids := make([]string, 0, len(channels))
	for id := range channels {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Channel, 0, len(ids))
	for _, id := range ids {
		out = append(out, channels[id])
	}
	return out
}

// SupportsPayType 判断渠道声明的 Options 是否包含该 payType。
func SupportsPayType(ch Channel, payType string) bool {
	if ch == nil {
		return false
	}
	for _, opt := range ch.Options() {
		if opt.PayType == payType {
			return true
		}
	}
	return false
}
