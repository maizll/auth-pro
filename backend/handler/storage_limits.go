package handler

// 各存储的单文件上限，按公开文档取值，不把 Git 仓库的 100MB 对象限制套到 Release 附件上。
//
// GitHub Release：每个附件必须小于 2 GiB。
// https://docs.github.com/repositories/releasing-projects-on-github/about-releases
//
// Gitee 发行版：单个附件不能超过 100MB（GVP 项目 200MB，这里按社区版普通仓库）。
// https://help.gitee.com/repository/release/create
//
// 阿里云 OSS、腾讯云 COS、Cloudflare R2 以及按 S3 协议实现的 MinIO，
// 单次 PutObject 上限都是 5 GiB。再大要走分片上传，本站在达到上限前先自己切开。
const (
	storageLimitGitHub int64 = 2 << 30
	storageLimitGitee  int64 = 100 << 20
	storageLimitS3     int64 = 5 << 30
)

// storagePartLimitOf 是实际上传用的分片大小。测试会临时换成很小的值，避免造上百兆的包。
// 生产环境不要改这个变量。
var storagePartLimitOf = defaultStoragePartLimit

// storagePartLimit 返回实际上传用的分片大小。
// 比文档上限略小：Gitee 的附件接口是 multipart，边界会占掉一部分；其余留 1 MiB，避免刚好顶满被拒绝。
func storagePartLimit(kind string) int64 {
	return storagePartLimitOf(kind)
}

func defaultStoragePartLimit(kind string) int64 {
	switch kind {
	case packageStorageGitHub:
		return storageLimitGitHub - (1 << 20)
	case packageStorageGitee:
		return 95 << 20
	default:
		return storageLimitS3 - (1 << 20)
	}
}

func storageLimitText(kind string) string {
	switch kind {
	case packageStorageGitHub:
		return "2 GiB"
	case packageStorageGitee:
		return "100 MB"
	default:
		return "5 GiB"
	}
}
