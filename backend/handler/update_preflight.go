package handler

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"auto_pro/config"
)

// 在线更新开始前的可写预检。
// 更新脚本以运行程序的用户（宝塔进程守护通常是 www）执行。目录写不进去时，脚本会在中途失败再回滚。
// 这里在下载之前逐个试写，任何一项不通过就直接结束任务，网站文件一个都不动。
var onlineUpdatePreflight = func() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法确定程序位置：%w", err)
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	return checkOnlineUpdateWritable(config.GetFrontendDir(), config.GetDataDir(), executable)
}

// onlineUpdateWritableTarget 是一个需要写入的目录和它在说明里的叫法。
type onlineUpdateWritableTarget struct {
	dir         string
	label       string
	cannotEnter bool
}

// checkOnlineUpdateWritable 按更新脚本实际会写的位置逐个试写。
// 判定网站根模式的规则和 update_restart.sh.tmpl 保持一致：程序或数据目录在网站根里面就是 overlay，
// 网站根是符号链接或名为 current 是 release，其余是整个目录改名的 rename。
func checkOnlineUpdateWritable(frontendDir, dataDir, appBin string) error {
	frontendDir = filepath.Clean(frontendDir)
	dataDir = filepath.Clean(dataDir)
	info, err := os.Lstat(frontendDir)
	if err != nil {
		return fmt.Errorf("更新没有开始，网站没有任何改动。未找到前端目录 %s", frontendDir)
	}
	overlay := onlineUpdatePathWithin(appBin, frontendDir) || onlineUpdatePathWithin(dataDir, frontendDir)
	release := !overlay && (info.Mode()&os.ModeSymlink != 0 || filepath.Base(frontendDir) == "current")

	targets := []onlineUpdateWritableTarget{
		{dir: filepath.Join(dataDir, "updates"), label: "更新暂存和备份目录"},
		// 程序先往这里写数据库备份，/www/backup 进不去时前端备份也放这里。用 root 跑过备份等命令后它可能归 root。
		{dir: filepath.Join(dataDir, "updates", "backups"), label: "更新备份目录"},
		{dir: filepath.Dir(appBin), label: "程序所在目录"},
	}
	if overlay {
		targets = append(targets, onlineUpdateWritableTarget{dir: frontendDir, label: "网站目录"})
		// overlay 会把网站根下的每个子目录整个移进备份。移动目录要求目录本身可写。
		entries, _ := os.ReadDir(frontendDir)
		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == "backend" {
				continue
			}
			targets = append(targets, onlineUpdateWritableTarget{dir: filepath.Join(frontendDir, entry.Name()), label: "网站目录"})
		}
	} else {
		label := "网站目录的上一级"
		if release {
			label = "前端发布目录"
		}
		targets = append(targets, onlineUpdateWritableTarget{dir: filepath.Dir(frontendDir), label: label})
		if !release {
			targets = append(targets, onlineUpdateWritableTarget{dir: frontendDir, label: "网站目录"})
		}
	}

	var failed []onlineUpdateWritableTarget
	for _, target := range targets {
		if err := probeOnlineUpdateWritable(target.dir); err != nil {
			target.cannotEnter = errors.Is(err, errOnlineUpdateCannotEnter)
			failed = append(failed, target)
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return errors.New(describeOnlineUpdateWritableFailure(failed, frontendDir, overlay))
}

// errOnlineUpdateCannotEnter 表示目录缺执行位（例如归运行用户但权限是 600），能看到却进不去。
var errOnlineUpdateCannotEnter = errors.New("cannot enter directory")

func onlineUpdatePathWithin(path, root string) bool {
	path = filepath.Clean(path)
	if path == root {
		return true
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

// probeOnlineUpdateWritable 在目录里建一个临时文件再删掉。目录不存在时先试着建出来（只对 updates 有意义）。
func probeOnlineUpdateWritable(dir string) error {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	// 目录缺执行位时，查里面任何名字都是「权限不够」而不是「不存在」。单独说成「无法进入」。
	if _, err := os.Lstat(filepath.Join(dir, ".auth-pro-enter-test")); err != nil && errors.Is(err, fs.ErrPermission) {
		return errOnlineUpdateCannotEnter
	}
	file, err := os.CreateTemp(dir, ".auth-pro-write-test-*")
	if err != nil {
		return err
	}
	name := file.Name()
	_ = file.Close()
	return os.Remove(name)
}

// describeOnlineUpdateWritableFailure 用一句话说明哪个目录写不进去、该运行哪条命令。
func describeOnlineUpdateWritableFailure(failed []onlineUpdateWritableTarget, frontendDir string, overlay bool) string {
	runUser := onlineUpdateRunUser()
	labels := map[string]string{}
	enter := map[string]bool{}
	for _, target := range failed {
		if _, ok := labels[target.dir]; !ok {
			labels[target.dir] = target.label
		}
		enter[target.dir] = enter[target.dir] || target.cannotEnter
	}
	dirs := onlineUpdateUniqueDirs(failed)
	var enterParts, writeParts []string
	for _, dir := range dirs {
		part := fmt.Sprintf("%s（%s）", dir, labels[dir])
		if enter[dir] {
			enterParts = append(enterParts, part)
		} else {
			writeParts = append(writeParts, part)
		}
	}
	var problems []string
	if len(enterParts) > 0 {
		problems = append(problems, "无法进入："+strings.Join(enterParts, "、"))
	}
	if len(writeParts) > 0 {
		problems = append(problems, "没有权限写入："+strings.Join(writeParts, "、"))
	}
	message := fmt.Sprintf("更新没有开始，网站没有任何改动。程序以 %s 用户运行，%s。", runUser, strings.Join(problems, "；"))
	if overlay && filepath.Dir(frontendDir) == "/www/wwwroot" {
		return message + "请用 root 登录服务器运行下面这条命令修好权限，再回来点「立即更新」：" + onlineUpdateRepairCommand(frontendDir)
	}
	joined := strings.Join(dirs, " ")
	if len(enterParts) > 0 {
		return message + fmt.Sprintf("请用 root 把这些目录交给 %s 并补上目录的进入权限（例如 chown -R %s %s && chmod u+rwx %s），再回来点「立即更新」。", runUser, runUser, joined, joined)
	}
	return message + fmt.Sprintf("请用 root 把这些目录交给 %s（例如 chown -R %s %s），再回来点「立即更新」。", runUser, runUser, joined)
}

// onlineUpdateRepairCommand 是宝塔站点的一键修复命令。安装脚本来自本站的更新源，不写死地址。
func onlineUpdateRepairCommand(frontendDir string) string {
	base := "https://" + onlineUpdateTrustHost()
	if parsed, err := url.Parse(strings.TrimSpace(onlineUpdateManifestURL())); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		base = parsed.Scheme + "://" + parsed.Host
	}
	return fmt.Sprintf("curl -fsSL %s/install.sh | bash -s -- --repair-update-perms %s", base, filepath.Base(frontendDir))
}

func onlineUpdateUniqueDirs(failed []onlineUpdateWritableTarget) []string {
	seen := map[string]bool{}
	dirs := make([]string, 0, len(failed))
	for _, target := range failed {
		if seen[target.dir] {
			continue
		}
		seen[target.dir] = true
		dirs = append(dirs, target.dir)
	}
	return dirs
}

func onlineUpdateRunUser() string {
	if current, err := user.Current(); err == nil && current.Username != "" {
		return current.Username
	}
	return strconv.Itoa(os.Getuid())
}
