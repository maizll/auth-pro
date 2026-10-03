package handler

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

func TestLicenseVerifyRateLimiterBucketsByIPAndAppKey(t *testing.T) {
	limiter := newRateLimiter(2, time.Minute)
	start := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	key := licenseVerifyRateKey("203.0.113.10", "demo-app")
	for i := 0; i < 2; i++ {
		if !limiter.allow(key, start) {
			t.Fatal("桶容量以内的请求被拒绝")
		}
	}
	if limiter.allow(key, start.Add(time.Second)) {
		t.Fatal("桶空了还放行")
	}
	if !limiter.allow(licenseVerifyRateKey("203.0.113.10", "other-app"), start) {
		t.Fatal("不同 app_key 共用了一个桶")
	}
	if !limiter.allow(licenseVerifyRateKey("203.0.113.11", "demo-app"), start) {
		t.Fatal("不同 IP 共用了一个桶")
	}
	// 每分钟 2 次 = 每 30 秒回补 1 个。
	if !limiter.allow(key, start.Add(30*time.Second)) {
		t.Fatal("过了 30 秒没有回补令牌")
	}
	if limiter.allow(key, start.Add(31*time.Second)) {
		t.Fatal("回补速度超过每分钟 2 次")
	}
	for i := 0; i < 2; i++ {
		if !limiter.allow(key, start.Add(10*time.Minute)) {
			t.Fatal("长时间不来后桶没有回满")
		}
	}
	if limiter.allow(key, start.Add(10*time.Minute)) {
		t.Fatal("回满后也不能超过容量")
	}
}

// 复现 P2：来源超过 1024 个后，老限流器每个请求都在全局锁里扫一遍全部来源，每个来源还预分配 1200 个时间戳。
// 新限流器 5000 个来源轮换时的耗时应和单一来源同一量级，记住的来源数也有上限。
func TestRateLimiterManySourcesStayFastAndBounded(t *testing.T) {
	const requests = 200000
	start := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	measure := func(sources int) time.Duration {
		limiter := newRateLimiter(1200, time.Minute)
		keys := make([]string, sources)
		for index := range keys {
			keys[index] = licenseVerifyRateKey("198.51."+strconv.Itoa(index/250)+"."+strconv.Itoa(index%250), "demo-app")
		}
		began := time.Now()
		for index := 0; index < requests; index++ {
			limiter.allow(keys[index%sources], start.Add(time.Duration(index)*time.Millisecond))
		}
		return time.Since(began)
	}
	single := measure(1)
	many := measure(5000)
	if many > single*4+50*time.Millisecond {
		t.Fatalf("5000 个来源耗时 %v，单一来源 %v，来源多时明显变慢", many, single)
	}

	limiter := newRateLimiter(1200, time.Minute)
	for index := 0; index < 400000; index++ {
		limiter.allow("forged-"+strconv.Itoa(index), start)
	}
	if got := limiter.size(); got > rateLimiterShards*rateLimiterShardMaxKey {
		t.Fatalf("记住的来源 %d 个，超过上限 %d", got, rateLimiterShards*rateLimiterShardMaxKey)
	}
	// 一个窗口后旧来源被清掉。
	limiter.allow("fresh", start.Add(2*time.Minute))
	for index := 0; index < rateLimiterShards*4; index++ {
		limiter.allow("fresh-"+strconv.Itoa(index), start.Add(2*time.Minute))
	}
	if got := limiter.size(); got > rateLimiterShards*8 {
		t.Fatalf("过了一个窗口仍记着 %d 个来源", got)
	}
}

func BenchmarkRateLimiter5000Sources(b *testing.B) {
	limiter := newRateLimiter(1200, time.Minute)
	keys := make([]string, 5000)
	for index := range keys {
		keys[index] = "198.51.100." + strconv.Itoa(index)
	}
	now := time.Now()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		index := 0
		for pb.Next() {
			limiter.allow(keys[index%len(keys)], now)
			index++
		}
	})
}

func TestLicenseVerifyRateLimitSkipsVerifyLog(t *testing.T) {
	script := &licenseVerifyLimitScript{appKey: "demo-app", secret: "app-secret"}
	useLicenseVerifyLimitDB(t, script)
	useLicenseVerifyLimitBudget(t, 2)

	body := licenseVerifyLimitBody(t, "demo-app", "not-a-valid-sign", time.Now().Unix())
	for attempt := 1; attempt <= 2; attempt++ {
		recorder := postLicenseVerify(body, "203.0.113.20:4000")
		code, msg, reason := decodeLicenseVerifyBody(t, recorder)
		if recorder.Code != http.StatusOK || code != 403 || msg != "授权校验失败" || reason != "verify_failed" {
			t.Fatalf("attempt %d = %d code=%d msg=%q reason=%q body=%s", attempt, recorder.Code, code, msg, reason, recorder.Body.String())
		}
	}
	if script.verifyLogs != 2 {
		t.Fatalf("verify_logs = %d, want 2", script.verifyLogs)
	}

	blocked := postLicenseVerify(body, "203.0.113.20:4000")
	code, msg, reason := decodeLicenseVerifyBody(t, blocked)
	if blocked.Code != http.StatusTooManyRequests || code != 429 || msg != "请求过于频繁，请稍后再试" || reason != "rate_limited" {
		t.Fatalf("rate limit = %d code=%d msg=%q reason=%q", blocked.Code, code, msg, reason)
	}
	if script.verifyLogs != 2 {
		t.Fatalf("rejected attempt wrote verify_logs, count=%d", script.verifyLogs)
	}

	otherKey := postLicenseVerify(licenseVerifyLimitBody(t, "other-app", "not-a-valid-sign", time.Now().Unix()), "203.0.113.20:4000")
	if otherKey.Code == http.StatusTooManyRequests {
		t.Fatal("another app_key was blocked by the same window")
	}
	otherIP := postLicenseVerify(body, "203.0.113.21:4000")
	if otherIP.Code == http.StatusTooManyRequests {
		t.Fatal("another IP was blocked by the same window")
	}
}

func TestLicenseVerifyUnsignedFailuresUseTheSameCopy(t *testing.T) {
	script := &licenseVerifyLimitScript{appKey: "demo-app", secret: "app-secret"}
	useLicenseVerifyLimitDB(t, script)
	useLicenseVerifyLimitBudget(t, 10)

	missing := postLicenseVerify(licenseVerifyLimitBody(t, "missing-app", "not-a-valid-sign", 1), "198.51.100.8:9")
	badSign := postLicenseVerify(licenseVerifyLimitBody(t, "demo-app", "not-a-valid-sign", 1), "198.51.100.9:9")
	if missing.Code != http.StatusOK || badSign.Code != http.StatusOK {
		t.Fatalf("status missing=%d bad=%d", missing.Code, badSign.Code)
	}
	if missing.Body.String() != badSign.Body.String() {
		t.Fatalf("app-missing and bad-signature differed:\nmissing=%s\nbad=%s", missing.Body.String(), badSign.Body.String())
	}
	_, msg, reason := decodeLicenseVerifyBody(t, missing)
	if msg != "授权校验失败" || reason != "verify_failed" {
		t.Fatalf("unsigned failure msg=%q reason=%q", msg, reason)
	}
	if strings.Contains(missing.Body.String(), "app_not_found") || strings.Contains(badSign.Body.String(), "invalid_sign") || strings.Contains(missing.Body.String(), "应用不存在") || strings.Contains(badSign.Body.String(), "签名错误") {
		t.Fatalf("unsigned failure still distinguishes the cause: %s", badSign.Body.String())
	}
	if script.verifyLogs != 1 {
		t.Fatalf("verify_logs = %d, want 1 for the bad signature only", script.verifyLogs)
	}
}

func TestLicenseVerifyDetailedReasonRequiresValidSignature(t *testing.T) {
	secret := "app-secret"
	script := &licenseVerifyLimitScript{appKey: "demo-app", secret: secret}
	useLicenseVerifyLimitDB(t, script)
	useLicenseVerifyLimitBudget(t, 10)

	now := time.Now().Unix()
	valid := licenseVerifyRequest{
		AppKey:      "demo-app",
		Domain:      "example.com",
		ServerIP:    "192.0.2.10",
		LicenseKey:  "license-key",
		Timestamp:   now,
		SignVersion: licenseSignVersionV2,
	}
	valid.Sign = licenseVerifyV2Sign(valid, secret)
	recorder := postLicenseVerify(mustJSON(t, valid), "192.0.2.40:9")
	_, msg, reason := decodeLicenseVerifyBody(t, recorder)
	if recorder.Code != http.StatusOK || reason != "license_not_found" || msg == "授权校验失败" {
		t.Fatalf("signed miss = %d msg=%q reason=%q body=%s", recorder.Code, msg, reason, recorder.Body.String())
	}

	stale := valid
	stale.Timestamp = now - 1000
	stale.Sign = licenseVerifyV2Sign(stale, secret)
	staleRecorder := postLicenseVerify(mustJSON(t, stale), "192.0.2.41:9")
	_, staleMsg, staleReason := decodeLicenseVerifyBody(t, staleRecorder)
	if staleReason != "invalid_timestamp" || staleMsg != "请求已过期" {
		t.Fatalf("signed stale timestamp = msg=%q reason=%q", staleMsg, staleReason)
	}
}

func useLicenseVerifyLimitDB(t *testing.T, script *licenseVerifyLimitScript) {
	t.Helper()
	name := licenseVerifyLimitDriverName.Add(1)
	driverName := "license-verify-limit-" + jsonNumber(name)
	sql.Register(driverName, &licenseVerifyLimitDriver{script: script})
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })
}

func useLicenseVerifyLimitBudget(t *testing.T, limit int) {
	t.Helper()
	previousLimiter := licenseVerifyLimiter
	previousReady := appLicenseRequiredOK
	licenseVerifyLimiter = newRateLimiter(limit, time.Minute)
	appLicenseRequiredOK = true
	t.Cleanup(func() {
		licenseVerifyLimiter = previousLimiter
		appLicenseRequiredOK = previousReady
	})
}

func postLicenseVerify(body, remoteAddr string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/license/verify", LicenseVerify)
	request := httptest.NewRequest(http.MethodPost, "/api/license/verify", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = remoteAddr
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func licenseVerifyLimitBody(t *testing.T, appKey, sign string, timestamp int64) string {
	t.Helper()
	return mustJSON(t, map[string]any{
		"appKey":      appKey,
		"domain":      "example.com",
		"serverIp":    "192.0.2.10",
		"licenseKey":  "license-key",
		"timestamp":   timestamp,
		"signVersion": "v2",
		"sign":        sign,
	})
}

func decodeLicenseVerifyBody(t *testing.T, recorder *httptest.ResponseRecorder) (int, string, string) {
	t.Helper()
	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Reason string `json:"reason"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v body=%s", err, recorder.Body.String())
	}
	return body.Code, body.Msg, body.Data.Reason
}

func jsonNumber(value uint64) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

var licenseVerifyLimitDriverName atomic.Uint64

type licenseVerifyLimitScript struct {
	mu         sync.Mutex
	appKey     string
	secret     string
	verifyLogs int
}

type licenseVerifyLimitDriver struct {
	script *licenseVerifyLimitScript
}

type licenseVerifyLimitConn struct {
	script *licenseVerifyLimitScript
}

type licenseVerifyLimitRows struct {
	cols []string
	data [][]driver.Value
	idx  int
}

type licenseVerifyLimitResult struct{}

func (d *licenseVerifyLimitDriver) Open(string) (driver.Conn, error) {
	return &licenseVerifyLimitConn{script: d.script}, nil
}

func (c *licenseVerifyLimitConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}
func (c *licenseVerifyLimitConn) Close() error { return nil }
func (c *licenseVerifyLimitConn) Begin() (driver.Tx, error) {
	return nil, errors.New("begin is not supported")
}

func (c *licenseVerifyLimitConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "FROM apps") {
		appKey, _ := args[0].Value.(string)
		if appKey != c.script.appKey {
			return &licenseVerifyLimitRows{cols: []string{"id", "app_name", "app_secret", "license_required"}}, nil
		}
		return &licenseVerifyLimitRows{
			cols: []string{"id", "app_name", "app_secret", "license_required"},
			data: [][]driver.Value{{int64(7), "Demo", c.script.secret, int64(1)}},
		}, nil
	}
	return &licenseVerifyLimitRows{cols: []string{"value"}}, nil
}

func (c *licenseVerifyLimitConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "INSERT INTO verify_logs") {
		c.script.mu.Lock()
		c.script.verifyLogs++
		c.script.mu.Unlock()
		return licenseVerifyLimitResult{}, nil
	}
	return nil, errors.New("unexpected exec")
}

func (r *licenseVerifyLimitRows) Columns() []string { return r.cols }
func (r *licenseVerifyLimitRows) Close() error      { return nil }
func (r *licenseVerifyLimitRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.idx])
	r.idx++
	return nil
}

func (licenseVerifyLimitResult) LastInsertId() (int64, error) { return 1, nil }
func (licenseVerifyLimitResult) RowsAffected() (int64, error) { return 1, nil }
