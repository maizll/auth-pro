package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

const (
	monitorJobExternalURL   = "external-url-health"
	monitorJobLicenseVerify = "license-verify-probe"
	monitorJobPaymentRoutes = "payment-callback-probe"
	monitorJobOnlineUpdate  = "online-update-probe"
	monitorJobDiskUsage     = "disk-usage"
)

type monitorJobDef struct {
	ID          string
	Name        string
	Description string
	Interval    time.Duration
	Run         func() (string, error)
}

type monitorJobRecord struct {
	ID          string    `json:"id"`
	Enabled     bool      `json:"enabled"`
	LastRunAt   time.Time `json:"lastRunAt"`
	LastStatus  string    `json:"lastStatus"`
	LastMessage string    `json:"lastMessage"`
	NextRunAt   time.Time `json:"nextRunAt"`
}

type monitorStateFile struct {
	Jobs []monitorJobRecord `json:"jobs"`
}

type monitorJobView struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Enabled         bool   `json:"enabled"`
	IntervalSeconds int    `json:"intervalSeconds"`
	LastRunAt       string `json:"lastRunAt"`
	LastStatus      string `json:"lastStatus"`
	LastMessage     string `json:"lastMessage"`
	NextRunAt       string `json:"nextRunAt"`
	Running         bool   `json:"running"`
}

var (
	monitorMu                sync.Mutex
	monitorLoaded            bool
	monitorRecords           = map[string]monitorJobRecord{}
	monitorRunning           = map[string]bool{}
	monitorStatePathOverride string
	monitorSchedulerOnce     sync.Once
	monitorStop              chan struct{}

	probeOnlineUpdate = func() (string, error) {
		manifest, err := fetchOnlineUpdateManifest()
		if err != nil {
			return "", err
		}
		version := strings.TrimSpace(manifest.Version)
		if version == "" {
			version = "未知版本"
		}
		return "在线更新源可访问，清单版本 " + version, nil
	}
)

func monitorJobCatalog() []monitorJobDef {
	return []monitorJobDef{
		{ID: monitorJobExternalURL, Name: "外链健康检查", Interval: 30 * time.Minute, Run: runExternalURLHealth,
			Description: "只检查已上架条目的外部 HTTPS。失败则从软件源目录下架并通知开发者。本站托管的 ZIP 不检查。"},
		{ID: monitorJobLicenseVerify, Name: "授权校验探测", Interval: 10 * time.Minute, Run: probeLicenseVerifyRoute,
			Description: "向本机授权校验接口发送缺字段请求，确认路由可响应。不签发、不消耗授权。"},
		{ID: monitorJobPaymentRoutes, Name: "支付回调路由探测", Interval: 10 * time.Minute, Run: probePaymentCallbackRoutes,
			Description: "探测本机支付通知与同步回跳路由是否可响应。空请求不会入账，也不访问第三方支付。"},
		{ID: monitorJobOnlineUpdate, Name: "在线更新源探测", Interval: time.Hour, Run: probeOnlineUpdate,
			Description: "拉取当前配置的在线更新清单，确认更新源可访问。只读检查，不执行更新。"},
		{ID: monitorJobDiskUsage, Name: "磁盘用量", Interval: 15 * time.Minute, Run: probeDiskUsage,
			Description: "统计上传、安装包和日志目录占用，并检查数据盘剩余空间。"},
	}
}

func monitorDef(id string) (monitorJobDef, bool) {
	for _, item := range monitorJobCatalog() {
		if item.ID == id {
			return item, true
		}
	}
	return monitorJobDef{}, false
}

func monitorStatePath() string {
	if strings.TrimSpace(monitorStatePathOverride) != "" {
		return monitorStatePathOverride
	}
	return filepath.Join(config.GetDataDir(), "system-monitor.json")
}

func loadMonitorState() {
	if monitorLoaded {
		return
	}
	monitorLoaded = true
	for _, def := range monitorJobCatalog() {
		monitorRecords[def.ID] = monitorJobRecord{
			ID: def.ID, Enabled: true, NextRunAt: time.Now().UTC().Add(def.Interval),
		}
	}
	payload, err := os.ReadFile(monitorStatePath())
	if err != nil {
		return
	}
	var stored monitorStateFile
	if json.Unmarshal(payload, &stored) != nil {
		return
	}
	for _, item := range stored.Jobs {
		def, ok := monitorDef(item.ID)
		if !ok {
			continue
		}
		if item.NextRunAt.IsZero() && item.Enabled {
			item.NextRunAt = time.Now().UTC().Add(def.Interval)
		}
		monitorRecords[item.ID] = item
	}
}

func saveMonitorStateLocked() {
	jobs := make([]monitorJobRecord, 0, len(monitorRecords))
	for _, def := range monitorJobCatalog() {
		if item, ok := monitorRecords[def.ID]; ok {
			jobs = append(jobs, item)
		}
	}
	payload, err := json.MarshalIndent(monitorStateFile{Jobs: jobs}, "", "  ")
	if err != nil {
		return
	}
	path := monitorStatePath()
	_ = os.MkdirAll(filepath.Dir(path), 0750)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0640); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

func resetMonitorState() {
	monitorMu.Lock()
	defer monitorMu.Unlock()
	monitorLoaded = false
	monitorRecords = map[string]monitorJobRecord{}
	monitorRunning = map[string]bool{}
}

func StartSystemMonitor() {
	monitorSchedulerOnce.Do(func() {
		monitorStop = make(chan struct{})
		go monitorLoop()
	})
}

func monitorLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-monitorStop:
			return
		case <-ticker.C:
			runDueMonitorJobs()
		}
	}
}

func runDueMonitorJobs() {
	now := time.Now().UTC()
	for _, def := range monitorJobCatalog() {
		monitorMu.Lock()
		loadMonitorState()
		record := monitorRecords[def.ID]
		if !record.Enabled || monitorRunning[def.ID] || record.NextRunAt.After(now) {
			monitorMu.Unlock()
			continue
		}
		monitorRunning[def.ID] = true
		monitorMu.Unlock()
		go finishMonitorJob(def)
	}
}

func finishMonitorJob(def monitorJobDef) {
	message, err := def.Run()
	status := "ok"
	if err != nil {
		status = "fail"
		message = err.Error()
	}
	now := time.Now().UTC()
	monitorMu.Lock()
	defer monitorMu.Unlock()
	loadMonitorState()
	record := monitorRecords[def.ID]
	record.LastRunAt = now
	record.LastStatus = status
	record.LastMessage = truncateText(message, 500)
	if record.Enabled {
		record.NextRunAt = now.Add(def.Interval)
	}
	monitorRecords[def.ID] = record
	monitorRunning[def.ID] = false
	saveMonitorStateLocked()
}

func listMonitorJobViews() []monitorJobView {
	monitorMu.Lock()
	defer monitorMu.Unlock()
	loadMonitorState()
	views := make([]monitorJobView, 0, len(monitorJobCatalog()))
	for _, def := range monitorJobCatalog() {
		record := monitorRecords[def.ID]
		next := ""
		if record.Enabled && !record.NextRunAt.IsZero() {
			next = record.NextRunAt.UTC().Format(time.RFC3339)
		}
		last := ""
		if !record.LastRunAt.IsZero() {
			last = record.LastRunAt.UTC().Format(time.RFC3339)
		}
		views = append(views, monitorJobView{
			ID: def.ID, Name: def.Name, Description: def.Description,
			Enabled: record.Enabled, IntervalSeconds: int(def.Interval.Seconds()),
			LastRunAt: last, LastStatus: record.LastStatus, LastMessage: record.LastMessage,
			NextRunAt: next, Running: monitorRunning[def.ID],
		})
	}
	return views
}

func AdminMonitorJobs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"jobs": listMonitorJobViews()}})
}

func AdminMonitorJobUpdate(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	def, ok := monitorDef(id)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "任务不存在"})
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请提供 enabled"})
		return
	}
	monitorMu.Lock()
	loadMonitorState()
	record := monitorRecords[id]
	record.ID = id
	record.Enabled = *req.Enabled
	if record.Enabled {
		if record.NextRunAt.IsZero() || record.NextRunAt.Before(time.Now().UTC()) {
			record.NextRunAt = time.Now().UTC().Add(def.Interval)
		}
	} else {
		record.NextRunAt = time.Time{}
	}
	monitorRecords[id] = record
	saveMonitorStateLocked()
	monitorMu.Unlock()
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已更新", "data": gin.H{"jobs": listMonitorJobViews()}})
}

func AdminMonitorJobRun(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	def, ok := monitorDef(id)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "任务不存在"})
		return
	}
	monitorMu.Lock()
	loadMonitorState()
	if monitorRunning[id] {
		monitorMu.Unlock()
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "任务正在执行"})
		return
	}
	monitorRunning[id] = true
	monitorMu.Unlock()
	finishMonitorJob(def)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已执行", "data": gin.H{"jobs": listMonitorJobViews()}})
}

func probeLicenseVerifyRoute() (string, error) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/license/verify", strings.NewReader(`{}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	LicenseVerify(ctx)
	if recorder.Code != http.StatusOK {
		return "", fmt.Errorf("授权校验接口返回 HTTP %d", recorder.Code)
	}
	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Reason string `json:"reason"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		return "", errors.New("授权校验接口没有返回 JSON")
	}
	if body.Code != 400 || body.Data.Reason != "bad_request" {
		return "", fmt.Errorf("授权校验接口响应异常：code=%d %s", body.Code, body.Msg)
	}
	return "授权校验接口可用：缺字段请求返回参数错误", nil
}

func probePaymentCallbackRoutes() (string, error) {
	checks := []struct {
		name string
		path string
		call func(*gin.Context)
	}{
		{"easypay/notify", "/api/payment/easypay/notify", EpayNotify},
		{"easypay/return", "/api/payment/easypay/return", EpayReturn},
		{"easypay-v2/notify", "/api/payment/easypay-v2/notify", EpayV2Notify},
		{"easypay-v2/return", "/api/payment/easypay-v2/return", EpayV2Return},
		{"alipay-f2f/notify", "/api/payment/alipay-f2f/notify", PaymentChannelNotify},
	}
	names := make([]string, 0, len(checks))
	for _, item := range checks {
		recorder := httptest.NewRecorder()
		ctx, engine := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, item.path, nil)
		if item.name == "alipay-f2f/notify" {
			ctx.Params = gin.Params{{Key: "channel", Value: "alipay-f2f"}}
			_ = engine
		}
		item.call(ctx)
		if recorder.Code == 0 || recorder.Body.Len() == 0 && recorder.Code < 300 {
			if recorder.Code != http.StatusFound && recorder.Code != http.StatusOK {
				return "", fmt.Errorf("%s 没有响应", item.name)
			}
		}
		if recorder.Code >= 500 {
			return "", fmt.Errorf("%s 返回 HTTP %d", item.name, recorder.Code)
		}
		names = append(names, item.name)
	}
	return "本地回调路由可响应：" + strings.Join(names, "、"), nil
}

func probeDiskUsage() (string, error) {
	uploadDirs := []string{advertisementImageDir(), stationPackageDir()}
	packageDirs := []string{config.GetAppReleaseDir(), config.GetUpdateDir()}
	logDir := filepath.Join(config.GetDataDir(), "logs")
	uploadBytes := sumDirBytes(uploadDirs)
	packageBytes := sumDirBytes(packageDirs)
	logBytes := sumDirBytes([]string{logDir})
	anchor := config.GetDataDir()
	usedPercent, freeBytes, totalBytes, err := diskUsage(anchor)
	if err != nil {
		return "", err
	}
	message := fmt.Sprintf("上传 %s，安装包 %s，日志 %s，数据盘已用 %d%%（剩余 %s / %s）",
		formatByteSize(uploadBytes), formatByteSize(packageBytes), formatByteSize(logBytes),
		usedPercent, formatByteSize(int64(freeBytes)), formatByteSize(int64(totalBytes)))
	if usedPercent >= 95 {
		return "", errors.New(message + "，剩余空间不足")
	}
	return message, nil
}

func sumDirBytes(dirs []string) int64 {
	var total int64
	for _, dir := range dirs {
		total += dirSize(dir)
	}
	return total
}

func dirSize(root string) int64 {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return 0
	}
	var total int64
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}

func diskUsage(path string) (usedPercent int, freeBytes, totalBytes uint64, err error) {
	var stat syscall.Statfs_t
	if err = syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0, fmt.Errorf("读取磁盘用量失败：%w", err)
	}
	bsize := uint64(stat.Bsize)
	if bsize == 0 {
		return 0, 0, 0, errors.New("读取磁盘用量失败：块大小为 0")
	}
	totalBytes = stat.Blocks * bsize
	freeBytes = stat.Bavail * bsize
	if totalBytes == 0 {
		return 0, freeBytes, totalBytes, nil
	}
	used := totalBytes - freeBytes
	usedPercent = int(used * 100 / totalBytes)
	return usedPercent, freeBytes, totalBytes, nil
}

func formatByteSize(size int64) string {
	if size < 0 {
		size = 0
	}
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	value := float64(size)
	suffixes := []string{"KB", "MB", "GB", "TB"}
	for _, suffix := range suffixes {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PB", value/unit)
}
