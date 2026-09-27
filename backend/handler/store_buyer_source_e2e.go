//go:build e2e

package handler

// 仅供 scripts/commercial_mysql_e2e.py 使用 -tags e2e 编译买家进程。
// 发布脚本不带这个标签，正式二进制不会包含本文件。
func init() {
	buyerSourceBase = func() (string, error) {
		return normalizeBuyerSource("https://source.auth-pro.test")
	}
}
