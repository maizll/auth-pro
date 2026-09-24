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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

func TestLicenseVerifyRateLimiterSlidesByIPAndAppKey(t *testing.T) {
	limiter := newLicenseVerifyRateLimiter(2, time.Minute)
	start := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	key := licenseVerifyRateKey("203.0.113.10", "demo-app")
	if !limiter.allow(key, start) || !limiter.allow(key, start.Add(30*time.Second)) {
		t.Fatal("hits inside the window were rejected")
	}
	if limiter.allow(key, start.Add(30*time.Second)) {
		t.Fatal("third hit inside the same window was accepted")
	}
	if !limiter.allow(licenseVerifyRateKey("203.0.113.10", "other-app"), start) {
		t.Fatal("a different app_key shared the window")
	}
	if !limiter.allow(licenseVerifyRateKey("203.0.113.11", "demo-app"), start) {
		t.Fatal("a different IP shared the window")
	}
	if !limiter.allow(key, start.Add(61*time.Second)) {
		t.Fatal("the oldest hit did not slide out of the window")
	}
	if limiter.allow(key, start.Add(61*time.Second)) {
		t.Fatal("the newer hit was dropped before the window elapsed")
	}
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
	licenseVerifyLimiter = newLicenseVerifyRateLimiter(limit, time.Minute)
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
