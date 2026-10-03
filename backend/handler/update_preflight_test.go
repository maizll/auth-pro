//go:build linux

package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 宝塔布局：网站根里放页面和 backend/，程序和数据目录都在 backend/ 里。
func preflightBaotaLayout(t *testing.T) (siteRoot, dataDir, appBin string) {
	t.Helper()
	siteRoot = filepath.Join(t.TempDir(), "demo.example")
	dataDir = filepath.Join(siteRoot, "backend", "data")
	appBin = filepath.Join(siteRoot, "backend", "auth_pro")
	for _, dir := range []string{filepath.Join(siteRoot, "assets"), dataDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for path, body := range map[string]string{
		filepath.Join(siteRoot, "index.html"):       "old",
		filepath.Join(siteRoot, "assets", "app.js"): "old",
		appBin: "bin",
	} {
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return siteRoot, dataDir, appBin
}

func skipPreflightAsRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root 不受目录权限限制，测不出写不进去的情况")
	}
}

func TestOnlineUpdatePreflightPassesForWritableBaotaSite(t *testing.T) {
	siteRoot, dataDir, appBin := preflightBaotaLayout(t)
	if err := checkOnlineUpdateWritable(siteRoot, dataDir, appBin); err != nil {
		t.Fatalf("writable site should pass: %v", err)
	}
	// 预检只试写临时文件，不能留下任何东西。
	entries, _ := os.ReadDir(siteRoot)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".auth-pro-write-test-") {
			t.Fatalf("probe file left behind: %s", entry.Name())
		}
	}
}

// 线上 ce 站的情况：网站根和里面的目录不归运行用户。预检要点名目录，网站文件一个都不动。
func TestOnlineUpdatePreflightNamesReadOnlySiteRoot(t *testing.T) {
	skipPreflightAsRoot(t)
	siteRoot, dataDir, appBin := preflightBaotaLayout(t)
	for _, dir := range []string{filepath.Join(siteRoot, "assets"), siteRoot} {
		if err := os.Chmod(dir, 0555); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_ = os.Chmod(siteRoot, 0755)
		_ = os.Chmod(filepath.Join(siteRoot, "assets"), 0755)
	})
	err := checkOnlineUpdateWritable(siteRoot, dataDir, appBin)
	if err == nil {
		t.Fatal("read-only site root must fail the preflight")
	}
	msg := err.Error()
	for _, want := range []string{"网站没有任何改动", siteRoot + "（网站目录）", filepath.Join(siteRoot, "assets"), "chown -R"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q should contain %q", msg, want)
		}
	}
	if body, _ := os.ReadFile(filepath.Join(siteRoot, "index.html")); string(body) != "old" {
		t.Fatalf("preflight touched index.html: %q", body)
	}
}

func TestOnlineUpdatePreflightNamesDataDirAndProgramDir(t *testing.T) {
	skipPreflightAsRoot(t)
	siteRoot, dataDir, appBin := preflightBaotaLayout(t)
	backend := filepath.Dir(appBin)
	if err := os.Chmod(dataDir, 0555); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(backend, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dataDir, 0755)
		_ = os.Chmod(backend, 0755)
	})
	err := checkOnlineUpdateWritable(siteRoot, dataDir, appBin)
	if err == nil {
		t.Fatal("read-only data and program directories must fail")
	}
	for _, want := range []string{"更新暂存和备份目录", "程序所在目录", backend} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("message %q should contain %q", err.Error(), want)
		}
	}
}

// 整个目录改名的布局要求网站根的上一级可写。
func TestOnlineUpdatePreflightRenameNeedsParent(t *testing.T) {
	skipPreflightAsRoot(t)
	root := t.TempDir()
	parent := filepath.Join(root, "sites")
	siteRoot := filepath.Join(parent, "site")
	dataDir := filepath.Join(root, "data")
	appBin := filepath.Join(root, "bin", "auth_pro")
	for _, dir := range []string{siteRoot, dataDir, filepath.Dir(appBin)} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkOnlineUpdateWritable(siteRoot, dataDir, appBin); err != nil {
		t.Fatalf("writable rename layout should pass: %v", err)
	}
	if err := os.Chmod(parent, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0755) })
	err := checkOnlineUpdateWritable(siteRoot, dataDir, appBin)
	if err == nil || !strings.Contains(err.Error(), parent+"（网站目录的上一级）") {
		t.Fatalf("rename layout should name the parent directory, got %v", err)
	}
}

// 宝塔站点给出一键修复命令，地址取本站的更新源，域名取网站根的目录名。
func TestOnlineUpdatePreflightBaotaRepairCommand(t *testing.T) {
	msg := describeOnlineUpdateWritableFailure(
		[]onlineUpdateWritableTarget{{dir: "/www/wwwroot/ce.example.com", label: "网站目录"}},
		"/www/wwwroot/ce.example.com", true)
	want := "/install.sh | bash -s -- --repair-update-perms ce.example.com"
	if !strings.Contains(msg, want) || !strings.Contains(msg, "curl -fsSL https://") || !strings.Contains(msg, "root") {
		t.Fatalf("baota message should carry the repair command, got %q", msg)
	}
	if strings.Contains(strings.ToLower(msg), "github") {
		t.Fatalf("customer-facing message mentions the code host: %q", msg)
	}
}

// 预检不通过时，下载、解压都不做。
func TestExecuteOnlineUpdateStopsBeforeDownloadWhenPreflightFails(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	previous := onlineUpdatePreflight
	onlineUpdatePreflight = func() error { return errors.New("更新没有开始，网站没有任何改动。预检不通过") }
	t.Cleanup(func() { onlineUpdatePreflight = previous })

	downloaded := false
	restore := stubOnlineUpdateApply(t, func(string, *onlineUpdateManifest) (string, error) {
		downloaded = true
		return "", errors.New("should not download")
	})
	defer restore()
	sum := sha256.Sum256([]byte("x"))
	manifest := signedOnlineUpdateManifest(hex.EncodeToString(sum[:]), 1)
	manifest.Actions.BackupDatabase = false
	err := executeOnlineUpdate("job-preflight", manifest)
	if err == nil || !strings.Contains(err.Error(), "预检不通过") {
		t.Fatalf("want preflight error, got %v", err)
	}
	if downloaded {
		t.Fatal("package was downloaded although the preflight failed")
	}
}

// ce 站的线上故障：宝塔网站根的上一级只有 root 能写、/www/backup 进不去。
// 1.8.9 起 overlay 备份退到本站数据目录，不再往网站根旁边放，更新能完成。
func TestOnlineUpdateOverlayBackupFallsBackToDataDir(t *testing.T) {
	skipPreflightAsRoot(t)
	shell, err := exec.LookPath("/bin/sh")
	if err != nil {
		t.Skip("/bin/sh is not available")
	}
	if _, err := os.Stat("/www/backup"); err == nil {
		t.Skip("本机有 /www/backup，备份会放到中央目录")
	}
	root := t.TempDir()
	wwwroot := filepath.Join(root, "wwwroot")
	liveDir := filepath.Join(wwwroot, "demo.example")
	dataDir := filepath.Join(liveDir, "backend", "data")
	sourceDir := filepath.Join(root, "incoming")
	writeFrontendTree(t, liveDir, "old-index", "old-asset")
	writeFrontendTree(t, sourceDir, "new-index", "new-asset")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTO_PRO_DATA_DIR", dataDir)
	t.Setenv("AUTO_PRO_SERVICE_NAME", "auth_pro_test")
	scriptPath := writeFrontendSwitchScript(t, sourceDir, liveDir, "1.8.9")
	if err := os.Chmod(wwwroot, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(wwwroot, 0755) })

	if err := checkOnlineUpdateWritable(liveDir, dataDir, filepath.Join(liveDir, "backend", "auth_pro")); err != nil {
		t.Fatalf("site owned by the runtime user should pass even if wwwroot is read-only: %v", err)
	}
	if output, err := exec.Command(shell, scriptPath, "--frontend-only").CombinedOutput(); err != nil {
		t.Fatalf("overlay switch failed: %v\n%s", err, output)
	}
	assertFrontendTree(t, liveDir, "new-index", "new-asset")
	backups, err := filepath.Glob(filepath.Join(dataDir, "updates", "backups", "overlay", "demo.example.overlay-backup.*"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("overlay backup should be in the data dir, got %v %v", backups, err)
	}
	assertFrontendTree(t, backups[0], "old-index", "old-asset")
	if sideways, _ := filepath.Glob(liveDir + ".overlay-backup.*"); len(sideways) != 0 {
		t.Fatalf("backup was placed next to the site root: %v", sideways)
	}
}

// 更新脚本在停旧进程之前失败（例如建不出备份目录）时，旧进程不会重启。
// 以前内存里的任务一直是「重启中」，页面看不到原因，再点更新提示「已有更新任务正在执行」，只能重启站点。
// 现在查询任务或开始下一次更新时，按脚本写下的结果文件把它结掉。
func TestOnlineUpdateRestartingJobSettlesFromResultWithoutRestart(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	updateStore.mu.Lock()
	previousJobs, previousRunning := updateStore.jobs, updateStore.runningID
	updateStore.jobs = map[string]*onlineUpdateJob{}
	updateStore.runningID = ""
	updateStore.mu.Unlock()
	t.Cleanup(func() {
		updateStore.mu.Lock()
		updateStore.jobs, updateStore.runningID = previousJobs, previousRunning
		updateStore.mu.Unlock()
	})

	job := createOnlineUpdateJob("1.9.0")
	finishOnlineUpdateJob(job.ID, "restarting", "服务正在切换并重启", nil)
	if reserveOnlineUpdateJob() {
		t.Fatal("a job without a result must still block the next update")
	}
	if got := snapshotOnlineUpdateJob(job.ID); got == nil || got.Status != "restarting" {
		t.Fatalf("job without a result should stay restarting: %+v", got)
	}

	reason := "无法创建前端备份目录 /www/backup/auth-pro/demo/overlay ，线上目录未改动"
	if err := os.WriteFile(onlineUpdateJobStatePath(job.ID)+".result", []byte("failed\n"+reason+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got := snapshotOnlineUpdateJob(job.ID)
	if got == nil || got.Status != "failed" || got.Error != reason {
		t.Fatalf("job should settle from the result file: %+v", got)
	}
	if !reserveOnlineUpdateJob() {
		t.Fatal("the next update should be allowed after the failed result")
	}
}

// 用 root 跑过备份等命令后，数据目录里的 updates/backups 可能归 root。程序写数据库备份就会失败，预检要先点名。
func TestOnlineUpdatePreflightNamesReadOnlyBackupDir(t *testing.T) {
	skipPreflightAsRoot(t)
	siteRoot, dataDir, appBin := preflightBaotaLayout(t)
	backups := filepath.Join(dataDir, "updates", "backups")
	if err := os.MkdirAll(backups, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(backups, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(backups, 0755) })
	err := checkOnlineUpdateWritable(siteRoot, dataDir, appBin)
	if err == nil || !strings.Contains(err.Error(), backups+"（更新备份目录）") {
		t.Fatalf("read-only backup dir must be named: %v", err)
	}
}

// 线上见过备份目录归运行用户、权限却是 600：能看到进不去。预检要说「无法进入」，并给出补进入权限的命令。
func TestOnlineUpdatePreflightNamesUnenterableDir(t *testing.T) {
	skipPreflightAsRoot(t)
	root := t.TempDir()
	siteRoot := filepath.Join(root, "sites", "site")
	dataDir := filepath.Join(root, "data")
	appBin := filepath.Join(root, "bin", "auth_pro")
	backups := filepath.Join(dataDir, "updates", "backups")
	for _, dir := range []string{siteRoot, backups, filepath.Dir(appBin)} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(backups, 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(backups, 0755) })
	err := checkOnlineUpdateWritable(siteRoot, dataDir, appBin)
	if err == nil {
		t.Fatal("a 600 backup directory must fail the preflight")
	}
	msg := err.Error()
	for _, want := range []string{"网站没有任何改动", "无法进入：" + backups + "（更新备份目录）", "chmod u+rwx " + backups} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q should contain %q", msg, want)
		}
	}
	if strings.Contains(msg, "没有权限写入") {
		t.Fatalf("only the unenterable directory failed, message should not list write failures: %q", msg)
	}
}
