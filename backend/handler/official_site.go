package handler

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"strings"
)

// officialSite 判断当前进程是不是官网。
// 官网和客户站是同一份程序，不能靠域名、环境变量或数据库区分：那些客户自己都能改，改完就会被当成官网，从发布仓库拉更新。
// 商业版快照只有官网能签发。数据目录 store/snapshot-ed25519.key 必须是编译进程序的那把公钥对应的种子。
// 不能直接用 PrivateKey.Public()：它只返回文件后 32 字节，不从种子推算。比对交给 storeSnapshotKeyFromSeed。
// 文件不存在、读失败、内容损坏，或公钥对不上，一律按客户站处理。只看到文件还不够。
func officialSite() bool {
	payload, err := os.ReadFile(storeSnapshotPrivateKeyPath())
	if err != nil {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(payload)))
	if err != nil {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(strings.TrimSpace(embeddedStoreSnapshotPublicKey))
	if err != nil {
		return false
	}
	_, ok := storeSnapshotKeyFromSeed(ed25519.PrivateKey(raw), ed25519.PublicKey(want))
	return ok
}
