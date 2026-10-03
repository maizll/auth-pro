package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func resetGuard() {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	guard.attempts = make(map[string]*loginAttempt)
}

func TestAccountLocksAfterMaxFailures(t *testing.T) {
	resetGuard()
	ip, account := "1.2.3.4", "admin"

	for i := 0; i < maxLoginFailuresPerAccount-1; i++ {
		RecordLoginFailure(ip, account)
		if got := LoginLockRemaining(ip, account); got != 0 {
			t.Fatalf("failure %d: expected unlocked, got %v", i+1, got)
		}
	}

	RecordLoginFailure(ip, account)
	if got := LoginLockRemaining(ip, account); got <= 0 {
		t.Fatal("expected account to be locked after reaching threshold")
	}
}

func TestOtherAccountSameIPLockedByIPLimit(t *testing.T) {
	resetGuard()
	ip := "5.6.7.8"

	for i := 0; i < maxLoginFailuresPerIP; i++ {
		RecordLoginFailure(ip, "user"+string(rune('a'+i%26))+string(rune('a'+(i/26)%26)))
	}

	if got := LoginLockRemaining(ip, "brand_new_account"); got <= 0 {
		t.Fatal("expected IP-level lock to affect new accounts from the same IP")
	}
	if got := LoginLockRemaining("9.9.9.9", "brand_new_account"); got != 0 {
		t.Fatal("expected other IPs to remain unlocked")
	}
}

func TestSuccessClearsAccountCounter(t *testing.T) {
	resetGuard()
	ip, account := "1.2.3.4", "admin"

	for i := 0; i < maxLoginFailuresPerAccount-1; i++ {
		RecordLoginFailure(ip, account)
	}
	RecordLoginSuccess(ip, account)

	for i := 0; i < maxLoginFailuresPerAccount-1; i++ {
		RecordLoginFailure(ip, account)
		if got := LoginLockRemaining(ip, account); got != 0 {
			t.Fatalf("expected counter reset after success, locked with %v remaining", got)
		}
	}
}

func TestLockExpires(t *testing.T) {
	resetGuard()
	ip, account := "1.2.3.4", "admin"

	for i := 0; i < maxLoginFailuresPerAccount; i++ {
		RecordLoginFailure(ip, account)
	}

	// 手动把锁定结束时间改到过去，模拟锁定期结束
	guard.mu.Lock()
	guard.attempts[loginAccountKey(ip, account)].lockedUntil = time.Now().Add(-time.Second)
	guard.mu.Unlock()

	if got := LoginLockRemaining(ip, account); got != 0 {
		t.Fatalf("expected lock to expire, got %v", got)
	}
}

// 换很多真实 IP 猜同一个账号，也会被按账号的计数锁住。
func TestAccountLockedAcrossManyIPs(t *testing.T) {
	resetGuard()
	for i := 0; i < maxLoginFailuresAnyIP; i++ {
		RecordLoginFailure("198.51.100."+strconv.Itoa(i%250), "admin")
	}
	if got := LoginLockRemaining("203.0.113.77", "admin"); got <= 0 {
		t.Fatal("分布式失败达到上限后，新 IP 登录同一账号应被锁定")
	}
	if got := LoginLockRemaining("203.0.113.77", "other"); got != 0 {
		t.Fatal("别的账号不应受影响")
	}
}

// 复现 G5：同一个连接每次带随机 X-Forwarded-For 连续输错 20 次，账号仍然被锁。
func TestForgedForwardedForCannotBypassLoginLock(t *testing.T) {
	resetGuard()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	if err := TrustLocalProxiesOnly(engine); err != nil {
		t.Fatal(err)
	}
	engine.POST("/login", func(c *gin.Context) {
		if LoginLockRemaining(c.ClientIP(), "admin") > 0 {
			c.Status(http.StatusTooManyRequests)
			return
		}
		RecordLoginFailure(c.ClientIP(), "admin")
		c.Status(http.StatusUnauthorized)
	})
	locked := false
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "198.51.100.7:5000"
		req.Header.Set("X-Forwarded-For", "10.0."+strconv.Itoa(i)+".1")
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			locked = true
		}
	}
	if !locked {
		t.Fatal("轮换 X-Forwarded-For 后账号没有被锁定")
	}
}
