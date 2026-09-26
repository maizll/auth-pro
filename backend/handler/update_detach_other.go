//go:build !unix

package handler

import "os/exec"

func detachOnlineUpdateCommand(cmd *exec.Cmd) {}
