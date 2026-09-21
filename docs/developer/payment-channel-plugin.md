# 如何实现一个支付渠道插件

本文面向要为 AuthPro 增加支付渠道的开发者。参考实现：仓库内官方插件 [`plugins/alipay-f2f/`](../../plugins/alipay-f2f/) + `backend/payment/alipayf2f`。

商店 ZIP 的 `plugin.json` 规则仍以 [插件开发指南](./plugin-package.md) 为准。支付渠道 **额外** 需要编译进后端的 Channel 实现；下载的 ZIP 不会执行代码。

## 1. 你要交付的三块

1. **清单**：`plugin.json`，`id` 稳定，`category` 必须是 `payment`。
2. **运行时**：实现 `payment.Channel`，在 `init()` 里 `payment.Register`。
3. **配置 UI**：管理端只写密钥、不回显私钥；商店「配置」跳到该页。

官方插件把 1 放在 `plugins/<id>/`，2 放在 `backend/payment/<id>/`，由 AuthPro 发版带上。第三方若不能改核心二进制，只能先提 PR 或等热加载（当前不支持）。

## 2. 契约摘要

见 [设计说明](../design/payment-channel-plugin.md)。最小接口：

- `ID()` / `PluginID()`
- `Options()` → 一条或多条 `{code,channel,payType,label,icon,color}`
- `Available(db)` → 凭证是否齐全（核心还会检查商店启用）
- `CreatePayment` → `redirect` 的 `payUrl` 或 `qrcode` 的 `qrCode`
- `ParseNotify` → 验签后的 `orderNo/amountCents/gatewayTradeNo/success`

通知 URL 由核心生成：`/api/payment/{ID()}/notify`。不要改路径结构。

## 3. 分步：以支付宝当面付为模板

### 3.1 选 id

使用 `^[a-z0-9][a-z0-9-]{1,58}$`。官方当面付为 `alipay-f2f`，渠道 id 与插件 id 相同。

### 3.2 写 Channel

```go
package mychannel

func init() { payment.Register(Channel{}) }

type Channel struct{}

func (Channel) ID() string       { return "my-channel" }
func (Channel) PluginID() string { return "my-channel" }
```

`Options` 的 `code` 必须是 `{channel}:{payType}`，与前端 `payMethod` 一致。

`CreatePayment` 使用核心传入的 `NotifyURL`，不要读请求里的用户输入覆盖 notify。

`ParseNotify` 必须验签。失败返回 error，核心对支付宝类网关写 `fail`。

### 3.3 注册进二进制

在会被 `main` 导入的包里 blank import：

```go
import _ "auto_pro/payment/alipayf2f"
```

当前由 `backend/handler/payment_channel.go` 完成。

### 3.4 商店 catalog 与配置页

把插件加入 `pluginCatalog`（`backend/handler/plugin.go`），`Official: true`，`Category: "payment"`，**`CanEnable: true`**（运行时必须编译进后端）。`pluginConfigured()` 判断凭证是否已填。

支付分类 **允许同时启用多个插件**（与实名互斥不同）。

**需要凭证的官方支付插件必须有真实配置页**，与易支付相同，并且必须收口到唯一支付配置页。硬规范：新支付插件配置必须进入 `/system/epay-config`，已启用的插件在该页按分段**同时展示、同时可配**，禁止用单一 `activeVersion` 互相隐藏；商店 `configTarget` 指向该页并带 `channel=<插件 id>`（只滚动定位）。禁止再增加平行系统菜单。未来官方微信等支付插件同样只加分段和 `configTarget`，不要新侧栏菜单。

1. 配置表单并入 `/system/epay-config`。`epay`、`epay-v2`、`alipay-f2f` 各自按是否 local+enabled 出区块，可同时出现。
2. 侧栏只保留「支付配置」（`EpayConfig` → `/system/epay-config`）。不要新增「支付宝当面付」或「微信支付」这类系统菜单。
3. 商店 `pluginConfigPaths['alipay-f2f'] = '/system/epay-config?channel=alipay-f2f'`（`epay` / `epay-v2` 同理带 `channel`），启用后卡片显示「配置」。
4. `GET/PUT /api/system/alipay-f2f-config`；GET **永不**返回私钥，只给 `privateKeySet`。

纯 ZIP 上传没有 Go 实现时商店会显示「需运行实现」。官方当面付不能走这条路：内置 catalog 覆盖同 ID 的历史 ZIP。`config.schema.json` 仅作文档，产品不据此渲染表单。

路径：启用 → 系统设置「支付配置」填写凭证。

### 3.5 配置存储

沿用 `system_configs`，`group=payment`，key 带渠道前缀（如 `alipay_f2f_`）。GET 接口只返回 `privateKeySet`，PUT 时空私钥表示不修改。

### 3.6 前端

- 商店 `pluginConfigPaths[id]` 指向 `/system/epay-config?channel=<插件 id>`。该页同时渲染所有已启用支付插件的配置分段，`channel` 只负责滚到对应分段。
- 下单响应若 `checkoutMode === 'qrcode'`（或带 `qrCode`），弹窗展示二维码并轮询订单状态；否则 `window.location.href = payUrl`。

## 4. 当面付正扫要点

1. 调 `alipay.trade.precreate`，`biz_content` 含 `out_trade_no`、`total_amount`、`subject`。
2. 成功 `code=10000` 时取 `qr_code`。
3. 用户扫码后支付宝 POST 表单到 notify；用 **支付宝公钥** RSA2 验签（排除 `sign`、`sign_type`）。
4. `trade_status` 为 `TRADE_SUCCESS` 或 `TRADE_FINISHED` 才结算。
5. 沙箱：配置页打开「沙箱」，或把网关写成 `https://openapi-sandbox.dl.alipaydev.com/gateway.do`。使用沙箱 APPID/密钥，**不要提交真实商户密钥**。

证书模式：打开开关并填写两个 SN，请求会带 `app_cert_sn` / `alipay_root_cert_sn`。P0 不解析证书文件。

## 5. 测试清单

- 注册表：`Register` 后 `pay-options` 能贡献 `alipay-f2f:alipay`。
- 商店：内置 `alipay-f2f` 的 `canEnable=true`；历史同 ID ZIP 仍只出现一张可启用卡片。
- 验签：合法 notify 通过；篡改金额失败。
- 结算：`settleRegisteredChannelNotify` 对 `license_purchase_orders` 入账且幂等。
- 配置视图 JSON 不含 `privateKey`，只有 `privateKeySet`。
- `plugin.json` 能通过源站 `fillPluginManifest`。

运行：

```bash
cd backend
go test ./payment/... ./handler/ -count=1
```

## 6. 常见错误

| 现象 | 原因 |
| --- | --- |
| 商店只有「需运行实现」、没有启用/配置 | 只上传了 ZIP，当前二进制没有官方 catalog，或把当面付当成第三方热加载。请部署含内置 `alipay-f2f` 的版本：内置条目 `canEnable=true`，同 ID ZIP 不会盖掉启用/配置 |
| 商店能启用但收银台没有 | 未配置 APPID/密钥，或 `Available()` 为 false |
| 启用当面付后易支付消失 | 旧逻辑支付分类互斥；现行支付分类已允许并存。请更新到含本契约的版本 |
| 扫码后一直 pending | notify 公网不可达，或验签公钥与网关环境（沙箱/正式）不一致 |
| 与易支付支付宝抢同一按钮 | `dedupePayOptions` 误按 payType 折叠；插件必须用自己的 `code` |
