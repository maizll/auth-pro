package handler

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestStoreHandoffURLRejectsOpenRedirect(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	link, err := buildStoreHandoffURL("https://auth.maizll.com", "user", token)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link, "https://auth.maizll.com/user/handoff#") || strings.Contains(link, "?") {
		t.Fatalf("user link = %s", link)
	}
	if _, err := buildStoreHandoffURL("https://auth.maizll.com", "user", "short"); err == nil {
		t.Fatal("short token was accepted")
	}
	agent, err := buildStoreHandoffURL("https://auth.maizll.com/", "agent", token)
	if err != nil || !strings.HasPrefix(agent, "https://auth.maizll.com/agent-panel/handoff#") {
		t.Fatalf("agent link = %s err=%v", agent, err)
	}
	if _, _, ok := storeHandoffPaths("admin"); ok {
		t.Fatal("admin must not get a handoff path")
	}
	allowed, err := buyerManageLinkAllowed(link)
	if err != nil || allowed != link {
		t.Fatalf("allowed = %s err=%v", allowed, err)
	}
	for _, raw := range []string{
		"https://auth.maizll.com/user/licenses#" + token,
		"https://evil.example/user/handoff#" + token,
		"https://auth.maizll.com/user/handoff?next=https://evil.example#" + token,
		"http://auth.maizll.com/user/handoff#" + token,
		"https://auth.maizll.com/user/handoff/" + token,
		"https://auth.maizll.com/user/handoff",
	} {
		if got, err := buyerManageLinkAllowed(raw); err == nil {
			t.Fatalf("accepted %s -> %s", raw, got)
		}
	}
	sum := hashStoreHandoffToken(token)
	if sum == token || len(sum) != 64 {
		t.Fatalf("hash = %s", sum)
	}
}

func TestStoreHandoffClaimAndSession(t *testing.T) {
	now := time.Now()
	if !handoffClaimable(false, now.Add(time.Minute), now) {
		t.Fatal("fresh ticket should be claimable")
	}
	if handoffClaimable(true, now.Add(time.Minute), now) {
		t.Fatal("used ticket was claimable")
	}
	if handoffClaimable(false, now.Add(-time.Second), now) {
		t.Fatal("expired ticket was claimable")
	}
	claims := storeHandoffClaims(7, "buyer@example.com", "user", now)
	if claims.Act != "" || claims.Role != "user" || claims.OperatorID != 0 {
		t.Fatalf("claims = %+v", claims)
	}
	if claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time) != storeHandoffSessionTTL {
		t.Fatalf("ttl = %s", claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time))
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(middleware.JWTSecret())
	if err != nil {
		t.Fatal(err)
	}
	parsed := &middleware.Claims{}
	if _, err := jwt.ParseWithClaims(signed, parsed, func(token *jwt.Token) (any, error) {
		return middleware.JWTSecret(), nil
	}); err != nil {
		t.Fatal(err)
	}
	if parsed.Act != "" || parsed.UserID != 7 {
		t.Fatalf("parsed = %+v", parsed)
	}
}

func TestStoreHandoffIssueRateLimitAndConsumeDoesNotLogToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	storeHandoffRate = storeRateWindow{}
	t.Cleanup(func() { storeHandoffRate = storeRateWindow{} })

	for i := 0; i < storeHandoffIssueIP; i++ {
		code, msg := postJSONCode(t, StoreLoginHandoffIssue, "/api/v1/store/auth/handoff", `{}`)
		if code == 429 {
			t.Fatalf("call %d was limited early: %s", i, msg)
		}
	}
	code, msg := postJSONCode(t, StoreLoginHandoffIssue, "/api/v1/store/auth/handoff", `{}`)
	if code != 429 || msg != "打开太频繁，请稍后再试" {
		t.Fatalf("limited code=%d msg=%s", code, msg)
	}

	token := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/store/auth/handoff/consume", strings.NewReader(`{"ticket":"`+token+`"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	StoreLoginHandoffConsume(ctx)
	if strings.Contains(rec.Body.String(), token) || bytes.Contains(logs.Bytes(), []byte(token)) {
		t.Fatalf("token leaked body=%s log=%s", rec.Body.String(), logs.String())
	}
	if strings.Contains(rec.Body.String(), "数据库") {
		t.Fatalf("raw database error: %s", rec.Body.String())
	}
}
