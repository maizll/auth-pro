//go:build !unix

// 非 Unix 平台不脱离子进程。一键整包更新只在 Linux amd64 上提供，这里留空实现以便编译。

package handler

import "os/exec"

func detachOnlineUpdateCommand(cmd *exec.Cmd) {}
