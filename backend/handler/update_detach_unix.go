//go:build unix

// Unix 上让更新子进程脱离当前服务，避免一键更新在重启时把正在跑的进程一起停掉。

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
