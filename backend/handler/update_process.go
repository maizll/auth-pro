package handler

import (
	"context"
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"auto_pro/config"
)

//go:embed update_restart.sh.tmpl
var onlineUpdateScriptTemplate string

const (
	processManagerSupervisor = "supervisor"
	processManagerSystemd    = "systemd"
	processManagerBaota      = "baota"
	processManagerNone       = "none"

	processManagerFileName = "process-manager"
)

// resolveOnlineUpdateProcessManager 判断当前进程该交给谁拉起。
// 显式 AUTO_PRO_PROCESS_MANAGER 优先。未设置时看数据目录标记、父进程和 systemd 单元。
// 没有任何守护迹象时才自行拉起，避免在宝塔进程守护下再 fork 出一个脱离守护的进程。
func resolveOnlineUpdateProcessManager() string {
	if mode, ok := normalizeProcessManager(os.Getenv("AUTO_PRO_PROCESS_MANAGER")); ok {
		return mode
	}
	if mode, ok := readProcessManagerFile(); ok {
		return mode
	}
	switch parentProcessComm() {
	case "supervisord", "supervisor":
		return processManagerSupervisor
	case "systemd":
		return processManagerSystemd
	}
	if systemdUnitExists(config.GetServiceName()) {
		return processManagerSystemd
	}
	return processManagerNone
}

func normalizeProcessManager(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case processManagerSupervisor, "supervisord":
		return processManagerSupervisor, true
	case processManagerSystemd:
		return processManagerSystemd, true
	case processManagerBaota, "宝塔":
		return processManagerBaota, true
	case processManagerNone, "standalone", "self":
		return processManagerNone, true
	default:
		return "", false
	}
}

func readProcessManagerFile() (string, bool) {
	path := filepath.Join(config.GetDataDir(), processManagerFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	line := data
	if index := strings.IndexByte(string(data), '\n'); index >= 0 {
		line = data[:index]
	}
	return normalizeProcessManager(string(line))
}

func parentProcessComm() string {
	ppid := os.Getppid()
	if ppid <= 1 {
		return ""
	}
	data, err := os.ReadFile("/proc/" + strconv.Itoa(ppid) + "/comm")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func systemdUnitExists(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemctl", "cat", name)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

func onlineUpdateHealthTries() int {
	raw := strings.TrimSpace(os.Getenv("AUTO_PRO_UPDATE_HEALTH_TRIES"))
	if raw == "" {
		return 60
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 5 || parsed > 180 {
		return 60
	}
	return parsed
}

func onlineUpdateSupervisorProgram() string {
	if name := strings.TrimSpace(os.Getenv("AUTO_PRO_SUPERVISOR_PROGRAM")); name != "" {
		return name
	}
	return "auth_pro"
}

func describeProcessManager(mode string) string {
	switch mode {
	case processManagerSupervisor, processManagerBaota:
		return "检测到进程守护（" + mode + "）。替换文件后退出当前进程，由守护拉起，不再自行 nohup"
	case processManagerSystemd:
		return "检测到 systemd。替换文件后由 systemctl 重启，不再自行 nohup"
	default:
		return "未检测到进程守护。将在旧进程释放端口后自行拉起新进程"
	}
}
