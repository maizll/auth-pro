# 插件示例

把本目录打成 ZIP 后，plugin.json 必须出现在压缩包根目录或一层子目录。

源站硬校验会拒绝：非 ZIP、超过 20 MiB、路径穿越、缺少必填字段、id/version 格式错误。失败不写库。

提交到源站时请另外提供：

- downloadUrl：HTTPS 外部地址（源站不托管这个 ZIP）
- sha256：整个 ZIP 的 64 位十六进制
- appId：目标应用（目录按应用隔离）
