package handler

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCommercialMysqlE2E 跑双站购买和 1.5.8 升级。本机没有 MySQL 时跳过。
func TestCommercialMysqlE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	script := filepath.Join("..", "..", "scripts", "commercial_mysql_e2e.py")
	cmd := exec.Command("python3", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err == nil {
		return
	}
	var exitErr *exec.ExitError
	if errorsAsExit(err, &exitErr) && exitErr.ExitCode() == 77 {
		t.Skip("MySQL 不可用，跳过商业版双站端到端")
	}
	t.Fatal(err)
}

func errorsAsExit(err error, target **exec.ExitError) bool {
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return false
	}
	*target = exitErr
	return true
}
