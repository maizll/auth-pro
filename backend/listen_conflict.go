package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func isAddrInUse(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "address already in use")
}

// describeListenConflict 说明这个端口现在被谁占用，以及管理员该怎么处理。
// 读不到进程时也给出原因，避免启动失败只剩一句英文 bind 错误。
func describeListenConflict(addr string) string {
	port := addr
	if index := strings.LastIndex(addr, ":"); index >= 0 {
		port = addr[index+1:]
	}
	port = strings.TrimSpace(port)
	occupants := listenOccupants(port)
	if len(occupants) == 0 {
		return fmt.Sprintf("端口 %s 已被占用，本进程没有启动。当前用户看不到占用进程。请用 root 执行 ss -lptn，先在宝塔进程守护中停止本站点，确认端口空闲后再启动。不要结束其它目录下的 auth_pro。", port)
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("端口 %s 已被占用，本进程没有启动。", port))
	for _, item := range occupants {
		lines = append(lines, fmt.Sprintf("占用者 PID=%d PPID=%d 程序=%s 命令=%s", item.pid, item.ppid, item.exe, item.cmd))
	}
	lines = append(lines, "处理：先在宝塔进程守护中停止本站点，再确认该端口不再被本站 backend/auth_pro 监听。占用者如果是本站目录里的 auth_pro（包括父进程为 1 的孤儿），由在线更新或 baota-upgrade.sh 在停守护之后结束它。占用者如果是其它程序或其它站点的 auth_pro，不要结束，改检查是不是端口配重了。")
	return strings.Join(lines, "\n")
}

type listenOccupant struct {
	pid  int
	ppid int
	exe  string
	cmd  string
}

func listenOccupants(port string) []listenOccupant {
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return nil
	}
	inodes := listenInodes(number)
	if len(inodes) == 0 {
		return nil
	}
	var found []listenOccupant
	seen := map[int]bool{}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 || seen[pid] {
			continue
		}
		if !processHoldsInode(pid, inodes) {
			continue
		}
		seen[pid] = true
		found = append(found, listenOccupant{
			pid:  pid,
			ppid: processPPID(pid),
			exe:  processExe(pid),
			cmd:  processCmd(pid),
		})
	}
	return found
}

func listenInodes(port int) map[string]bool {
	hexPort := fmt.Sprintf("%04X", port)
	inodes := map[string]bool{}
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 10 || fields[0] == "sl" {
				continue
			}
			if !strings.EqualFold(fields[3], "0A") {
				continue
			}
			local := fields[1]
			colon := strings.LastIndex(local, ":")
			if colon < 0 || !strings.EqualFold(local[colon+1:], hexPort) {
				continue
			}
			inode := fields[9]
			if inode != "" && inode != "0" {
				inodes[inode] = true
			}
		}
	}
	return inodes
}

func processHoldsInode(pid int, inodes map[string]bool) bool {
	fdDir := fmt.Sprintf("/proc/%d/fd", pid)
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join(fdDir, entry.Name()))
		if err != nil || !strings.HasPrefix(target, "socket:[") || !strings.HasSuffix(target, "]") {
			continue
		}
		inode := strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")
		if inodes[inode] {
			return true
		}
	}
	return false
}

func processPPID(pid int) int {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "PPid:") {
			continue
		}
		ppid, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "PPid:")))
		return ppid
	}
	return 0
}

func processExe(pid int) string {
	target, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return "未知"
	}
	return strings.TrimSuffix(target, " (deleted)")
}

func processCmd(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || len(data) == 0 {
		return "未知"
	}
	parts := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
	return strings.Join(parts, " ")
}
