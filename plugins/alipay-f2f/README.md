# 支付宝当面付（官方支付渠道插件）

AuthPro 商店官方插件。安装/启用并填写开放平台凭证后，收银台会出现独立渠道 **支付宝当面付**（`alipay-f2f:alipay`）。

运行实现编译在 AuthPro 后端中（与易支付官方插件相同）。本 ZIP 提供清单与说明，供软件源上架与实例商店展示。

## 能力范围（P0）

| 项 | 说明 |
| --- | --- |
| 模式 | **正扫**：商家展示收款码，用户支付宝扫码 |
| 接口 | `alipay.trade.precreate` |
| 下单结果 | 返回 `qr_code`，前端弹窗展示二维码 |
| 入账 | 异步 `notify_url`：`/api/payment/alipay-f2f/notify`，RSA2 验签后结算既有订单表 |
| 反扫 | P1，本版本不实现（商家扫用户付款码） |

已接入的下单路径：用户/代理授权购买、用户/代理余额充值、用户升级代理。插件权益购买（独立 SKU）当前产品未落地，见设计文档后续项。

## 安装与启用

1. 应用商店找到「支付宝当面付」，若尚未内置则从软件源下载安装。
2. 启用插件（支付类插件可与易支付并存，不会互相挤掉）。
3. 打开配置页填写开放平台凭证（**不要把应用私钥提交到仓库或工单**）。
4. 保存后，用户购买页 pay-options 会出现「支付宝当面付」。

## 配置项（占位，勿填入真实生产密钥）

| 字段 | 说明 | 示例 / 占位 |
| --- | --- | --- |
| APPID | 开放平台应用 APPID | `2021000000000000` |
| 应用私钥 | RSA2（PKCS#1 或 PKCS#8 PEM） | 留空表示保持已保存值 |
| 支付宝公钥 | 公钥模式：开放平台「支付宝公钥」 | PEM 或裸 base64 |
| 网关 | 可留空 | 正式 `https://openapi.alipay.com/gateway.do` |
| 沙箱 | 打开后默认沙箱网关 | `https://openapi-sandbox.dl.alipaydev.com/gateway.do` |
| 异步通知 | 可留空，系统按当前域名生成 | `https://your-host/api/payment/alipay-f2f/notify` |
| 证书模式 | 可选。开启后需填应用公钥证书 SN、支付宝根证书 SN | 公钥仍用于验签 |

沙箱联调：在[支付宝开放平台沙箱](https://open.alipay.com/)创建应用，使用沙箱 APPID 与密钥，**不要**把生产私钥写入本仓库。

## 安全

- 应用私钥只存 `system_configs`（`payment` / `alipay_f2f_private_key`），管理端接口只返回 `privateKeySet`。
- 服务端日志不得打印私钥或完整 `sign`。
- 异步通知必须 RSA2 验签、校验 APPID、金额与 `out_trade_no` 后才入账；重复通知幂等。

## 打包上架

将本目录打成 ZIP（`plugin.json` 在根或一层子目录），托管到 HTTPS，计算 sha256，在源站按应用上架。清单必须符合 `docs/developer/plugin-package.md`。
