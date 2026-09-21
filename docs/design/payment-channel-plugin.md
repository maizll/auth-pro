# 支付渠道插件契约

| 项 | 值 |
| --- | --- |
| 文档状态 | 已锁定（对照本仓库支付渠道 SPI 首版） |
| 读者 | 实现工程师、插件作者、源站运营 |
| 范围 | 支付渠道插件的生命周期、SPI、安全、从易支付迁出路径 |
| 不在范围 | 易支付 V1/V2 协议细节、插件权益独立 SKU、反扫（商家扫用户付款码）实现 |

**锁定决策：支付渠道是可并存的官方商店插件，不是替换易支付的全局开关。** 易支付继续用原配置与回调；官方支付宝当面付启用后作为额外渠道出现在 `pay-options`。

---

## 1. 背景

今天收银台渠道是 **硬编码** 的：

| 渠道 id | 实现位置 | 启用条件 |
| --- | --- | --- |
| `easypay` | `backend/handler/epay.go` | `system_configs.payment.easypay_*` |
| `easypay-v2` | `backend/handler/epay_v2.go` | `easypay_v2_*` |
| `balance` / `quota` | 购买 handler 内同步扣款 | 始终可用（配额需有余量） |

应用商店虽有 `epay` / `epay-v2` 官方插件条目，但 **支付时并不读取 `plugins.enabled`**，只看网关配置。商店 ZIP 安装也不会加载 Go 代码。

要让「安装/启用后出现支付渠道」成立，必须补一层 **进程内 SPI**：官方插件的运行时仍编译进后端，清单走商店，启用状态写入 `plugins` 表。

## 2. 目标

1. 定义可测试的 **Payment Channel Plugin Contract**。
2. 官方支付宝当面付通过该契约注册渠道 `alipay-f2f`，P0 只做正扫 `alipay.trade.precreate`。
3. 不破坏易支付创建订单、回调、结算。
4. 至少一条既有订单类型可被当面付 notify 结算（授权购买）；充值与代理升级同步接入。

## 3. 架构

```text
收银台 pay-options
        │
        ├─ balance / quota          （核心，非插件）
        ├─ easypay / easypay-v2     （核心硬编码，配置开关）
        └─ payment.List()           （插件注册表：alipay-f2f …）
                │
                ▼
        创建 pending 订单（既有表）
                │
        Channel.CreatePayment
                │
        ┌───────┴────────┐
        redirect payUrl   qrcode 弹窗
                │
        /api/payment/{channel}/notify
                │
        Channel.ParseNotify（验签）
                │
        settle* 既有函数（充值 / 授权购买 / 代理升级）
```

## 4. 生命周期

| 阶段 | 行为 |
| --- | --- |
| 编译 | 官方渠道 `init()` 调用 `payment.Register` |
| 上架 | 软件源 `plugin.json`（`id` 与 catalog 一致，`category=payment`） |
| 安装 | 内置插件始终 local；远程 ZIP 只落数据目录，不执行代码 |
| 启用 | `POST /api/system/plugins/alipay-f2f/toggle`。**支付分类不再互斥**，可与易支付同时启用 |
| 配置 | `GET/PUT /api/system/alipay-f2f-config`，密钥进 `system_configs.group=payment` |
| 出现在收银台 | `plugins.enabled=1` **且** 渠道 `Available()`（凭证齐全） |
| 停用 | 不再出现在 pay-options；已发出的 pending 订单仍接受合法 notify（与易支付 `validateForNotify` 一致） |

## 5. SPI

包：`auto_pro/payment`。

```go
type Channel interface {
    ID() string
    PluginID() string
    Options() []Option
    Available(db *sql.DB) bool
    CreatePayment(db *sql.DB, req CreateRequest) (CreateResult, error)
    ParseNotify(db *sql.DB, values map[string]string) (NotifyResult, error)
}
```

| 字段 | 约定 |
| --- | --- |
| `ID` | 渠道 id，出现在 `pay_channel` 与 `/api/payment/{id}/notify` |
| `PluginID` | 商店 catalog id，通常与 ID 相同 |
| `Option.Code` | `{channel}:{payType}`，例如 `alipay-f2f:alipay` |
| `CreateResult.Mode` | `redirect` 或 `qrcode` |
| `NotifyResult.Success` | 仅 `TRADE_SUCCESS` / `TRADE_FINISHED`（或渠道等价状态）为 true |

易支付 **不** 迁入该注册表（避免行为漂移）。`parseOnlinePaySelection` 先识别注册表渠道，再回退 `easypay` / `easypay-v2`。

`dedupePayOptions` 只折叠易支付 V1/V2 的同一 `payType`；插件渠道按 `code` 保留，因此「支付宝」与「支付宝当面付」可同时出现。

## 6. 配置 schema（当面付）

存 `system_configs`，`group=payment`：

| key | 说明 |
| --- | --- |
| `alipay_f2f_app_id` | APPID |
| `alipay_f2f_private_key` | 应用 RSA2 私钥（只写不读） |
| `alipay_f2f_alipay_public_key` | 支付宝公钥 |
| `alipay_f2f_gateway` | 可空 |
| `alipay_f2f_notify_url` | 可空，默认 `/api/payment/alipay-f2f/notify` |
| `alipay_f2f_sandbox` | `1` 时默认沙箱网关 |
| `alipay_f2f_cert_mode` | `1` 证书模式 |
| `alipay_f2f_app_cert_sn` | 应用公钥证书 SN |
| `alipay_f2f_alipay_root_cert_sn` | 支付宝根证书 SN |

管理端响应 **永不** 返回私钥，只给 `privateKeySet`。证书模式 P0 只携带 SN，不上传证书文件。

## 7. 安全

1. 禁止把私钥写入日志、错误信息、`notify_payload` 明文 sign。
2. notify 必须 RSA2 验签；校验 `app_id`、金额、`out_trade_no`。
3. 结算走既有行锁 + 幂等：`settleLicensePurchaseOrder` / `settleRechargeOrder` / `settleAgentUpgradeOnlinePayment`。
4. 密钥沿用 `system_configs`，与易支付、实名一致。

## 8. 从易支付迁移

| 策略 | 说明 |
| --- | --- |
| 并存（默认） | 易支付配置不变；当面付是新选项。适合平滑上线 |
| 只开当面付 | 关闭易支付 `easypay_enabled` / 清空 pay types；启用 `alipay-f2f` |
| 不支持的 | 把历史 `pay_channel=easypay` 订单改写成 `alipay-f2f`。回调仍按下单时渠道验签 |

收银台 code 从「裸 `alipay`」演进到「`{channel}:{payType}`」已存在；当面付只增加 `alipay-f2f:alipay`。遗留裸 `alipay` 仍走易支付，避免把当面付误判为默认支付宝。

## 9. 订单结算映射

| 订单前缀 / 表 | 结算函数 | 本版 |
| --- | --- | --- |
| `UP*` / `LP*` `license_purchase_orders` | `settleLicensePurchaseOrder` | **已接入**（首选） |
| `UR*` `recharge_orders` | `settleRechargeOrder` | 已接入 |
| `AU*` `agent_upgrade_orders` | `settleAgentUpgradeOnlinePayment` | 已接入 |
| 插件权益 SKU | 无表 | **后续**：产品尚未有 `plugin_purchase` |

## 10. 后续（明确不做）

- P1 反扫：`alipay.trade.pay`（auth_code）。
- 第三方支付插件热加载（从 ZIP 执行 Go/脚本）。
- 把 easypay 改写成 Channel 实现（可在确认回归集后再做）。
- 插件付费安装 / 授权绑定支付。
