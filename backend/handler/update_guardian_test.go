//go:build linux

package handler

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func writeGuardianStart(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "start.sh")
	if err := os.WriteFile(path, []byte(guardianStartScript), 0755); err != nil {
		t.Fatal(err)
	}
}

func writeHandoff(t *testing.T, dir, port, healthBody string) string {
	t.Helper()
	pending := filepath.Join(dir, "updates", "pending-restart")
	if err := os.MkdirAll(pending, 0755); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(dir, "updates", "job.result")
	logPath := filepath.Join(dir, "logs", "handoff.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(dir, "auth_pro.backup")
	script := fmt.Sprintf(`EXPECT_VERSION='2.0.0'
OLD_VERSION='1.0.0'
PORT='%s'
APP_BIN='%s'
BACKUP_BIN='%s'
JOB_RESULT='%s'
LOG_FILE='%s'
HEALTH_TRIES='3'
MAX_ATTEMPTS='2'
PENDING_DIR='%s'
FRONTEND_MODE=''
FRONTEND_CURRENT='%s'
FRONTEND_BACKUP=''
PREV_FRONTEND_TARGET=''
OVERLAY_MOVED=''
OVERLAY_PLACED=''
TS='test'
%s
handoff_health_ok() {
  child="$1"
  i=0
  while [ "$i" -lt "$HEALTH_TRIES" ]; do
    if ! kill -0 "$child" 2>/dev/null; then
      return 1
    fi
    body=$(curl -fsS --max-time 1 "http://127.0.0.1:${PORT}/api/system/version" 2>/dev/null || true)
    if printf '%%s' "$body" | grep -Fq '%s'; then
      return 0
    fi
    i=$((i + 1))
    sleep 1
  done
  return 1
}
`, port, filepath.Join(dir, "auth_pro"), backup, result, logPath, pending, dir, healthBody, `{"version":"2.0.0"}`)
	// The format string above uses %%s so the generated shell keeps printf '%s'.
	if err := os.WriteFile(filepath.Join(pending, "handoff.sh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return result
}

const handoffFunctionBody = `
log_handoff() { printf '%s %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$1" >> "$LOG_FILE"; }
finish_handoff() {
  printf '%s\n%s\n' "$1" "${2:-}" > "${JOB_RESULT}.tmp"
  mv -f "${JOB_RESULT}.tmp" "$JOB_RESULT"
}
handoff_wait_port_free() { return 0; }
handoff_mark_success() {
  finish_handoff success ""
  rm -rf "$PENDING_DIR"
}
handoff_rollback() {
  cp -a "$BACKUP_BIN" "$APP_BIN"
  chmod 755 "$APP_BIN"
  finish_handoff failed "$1"
  rm -rf "$PENDING_DIR"
}
`

func freeTCPPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func startGuardianScript(t *testing.T, dir string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(filepath.Join(dir, "start.sh"))
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	logFile, err := os.Create(filepath.Join(dir, "start.out"))
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_, _ = cmd.Process.Wait()
		logFile.Close()
	})
	return cmd
}

func waitForFileContains(t *testing.T, path, needle string) string {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			last = string(data)
			if strings.Contains(last, needle) {
				return last
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	out, _ := os.ReadFile(filepath.Join(filepath.Dir(path), "..", "start.out"))
	t.Fatalf("timed out waiting for %q in %s\nlast=%q\nstart.out=%s", needle, path, last, out)
	return ""
}

func TestGuardianStartRollsBackWhenBinaryExits(t *testing.T) {
	dir := t.TempDir()
	writeGuardianStart(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "baota.env"), []byte("PORT=9\nAUTO_PRO_PROCESS_MANAGER=supervisor\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth_pro"), []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(dir, "auth_pro.backup")
	if err := os.WriteFile(backup, []byte("#!/bin/sh\necho restored > \"$PWD/restored\"\nsleep 30\n"), 0755); err != nil {
		t.Fatal(err)
	}
	result := writeHandoff(t, dir, "9", handoffFunctionBody)
	startGuardianScript(t, dir)
	body := waitForFileContains(t, result, "failed")
	if !strings.Contains(body, "回滚") {
		t.Fatalf("rollback reason missing: %s", body)
	}
	waitForFileContains(t, filepath.Join(dir, "restored"), "restored")
	restored, err := os.ReadFile(filepath.Join(dir, "auth_pro"))
	if err != nil || !strings.Contains(string(restored), "restored") {
		t.Fatalf("binary was not restored from backup: %s %v", restored, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "updates", "pending-restart", "handoff.sh")); !os.IsNotExist(err) {
		t.Fatal("pending handoff should be removed after rollback")
	}
}

func TestGuardianStartKeepsHealthyVersion(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required")
	}
	dir := t.TempDir()
	port := freeTCPPort(t)
	writeGuardianStart(t, dir)
	env := fmt.Sprintf("PORT=%s\nAUTO_PRO_PROCESS_MANAGER=supervisor\n", port)
	if err := os.WriteFile(filepath.Join(dir, "baota.env"), []byte(env), 0600); err != nil {
		t.Fatal(err)
	}
	server := fmt.Sprintf(`#!/bin/sh
exec python3 -c '
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import os
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        body = b"{\"version\":\"2.0.0\"}"
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *args):
        return
ThreadingHTTPServer(("127.0.0.1", int(os.environ["PORT"])), H).serve_forever()
'
`)
	if err := os.WriteFile(filepath.Join(dir, "auth_pro"), []byte(server), 0755); err != nil {
		t.Fatal(err)
	}
	result := writeHandoff(t, dir, port, handoffFunctionBody)
	cmd := startGuardianScript(t, dir)
	body := waitForFileContains(t, result, "success")
	if strings.Contains(body, "failed") {
		t.Fatalf("healthy start was recorded as failed: %s", body)
	}
	time.Sleep(200 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatal("start.sh exited after a healthy handoff")
	}
}
