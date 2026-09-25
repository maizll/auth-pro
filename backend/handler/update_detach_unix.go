//go:build unix

package handler

import (
	"os/exec"
	"syscall"
)

func detachOnlineUpdateCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// 新会话，避免旧进程退出时把更新脚本一起带走（SIGHUP）。
	cmd.SysProcAttr.Setsid = true
}
