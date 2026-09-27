package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPlainRegisterMessage(t *testing.T) {
	secret := "S3cret-pass"
	cases := []struct {
		in, want string
	}{
		{"无法连接源站", "网络不通，请稍后再试"},
		{"该邮箱已注册", "这个邮箱已经注册过了"},
		{"邮箱验证码错误", "验证码错误，请核对后再试"},
		{"邮箱验证码无效或已过期，请重新获取", "验证码无效或已过期，请重新获取"},
		{"该手机号已被使用", "这个手机号已经注册过了"},
		{"普通用户注册已关闭，请联系管理员", "源站暂时关闭了注册，请联系管理员"},
		{"参数错误，请检查邮箱格式和密码长度", "请检查邮箱、验证码和密码（至少 6 位）"},
		{"源站还没有这个接口", "源站暂时不能注册，请稍后再试"},
		{"该邮箱已注册 " + secret, "这个邮箱已经注册过了"},
	}
	for _, item := range cases {
		got := plainRegisterMessage(item.in, secret)
		if got != item.want {
			t.Fatalf("plain %q => %q, want %q", item.in, got, item.want)
		}
		if strings.Contains(got, secret) {
			t.Fatalf("message kept password: %s", got)
		}
	}
}

func TestStoreRegisterUsesOfficialHandlerAndRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	storeRegisterRate = storeRateWindow{}
	var calls int
	prev := invokeRegister
	invokeRegister = func(c *gin.Context) {
		calls++
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "注册成功"})
	}
	t.Cleanup(func() {
		invokeRegister = prev
		storeRegisterRate = storeRateWindow{}
	})

	for i := 0; i < storeRegisterSubmitLimit; i++ {
		code, msg := postJSONCode(t, StoreRegister, "/api/v1/store/register", `{"email":"a@example.com"}`)
		if code != 200 || msg != "注册成功" {
			t.Fatalf("call %d code=%d msg=%s", i, code, msg)
		}
	}
	if calls != storeRegisterSubmitLimit {
		t.Fatalf("official register calls=%d", calls)
	}
	code, msg := postJSONCode(t, StoreRegister, "/api/v1/store/register", `{}`)
	if code != 429 || msg != "注册太频繁，请 10 分钟后再试" {
		t.Fatalf("limited code=%d msg=%s", code, msg)
	}
	if calls != storeRegisterSubmitLimit {
		t.Fatal("rate limit still called official register")
	}

	var codeCalls int
	prevCode := invokeRegisterEmailCode
	invokeRegisterEmailCode = func(c *gin.Context) {
		codeCalls++
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "验证码已发送，请查收邮件"})
	}
	t.Cleanup(func() { invokeRegisterEmailCode = prevCode })
	code, msg = postJSONCode(t, StoreRegisterEmailCode, "/api/v1/store/register/email-code", `{"email":"a@example.com"}`)
	if code != 200 || codeCalls != 1 {
		t.Fatalf("email code should use its own limit, code=%d msg=%s calls=%d", code, msg, codeCalls)
	}
}

func TestBuyerRegisterFallsBackAndHidesPassword(t *testing.T) {
	secret := "S3cret-pass"
	var paths []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/api/v1/store/register" {
			http.NotFound(w, r)
			return
		}
		if !bytes.Contains(raw, []byte(secret)) {
			t.Errorf("legacy register did not receive the password")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":400,"msg":"该邮箱已注册 ` + secret + `"}`))
	}))
	defer server.Close()
	client := server.Client()
	useBuyerSourceForTest(t, server.URL, client)

	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	_, err := forwardBuyerRegister("/api/v1/store/register", "/api/user-panel/register", map[string]any{
		"email": "a@example.com", "password": secret,
	})
	if err == nil {
		t.Fatal("expected register rejection")
	}
	msg := plainRegisterMessage(err.Error(), secret)
	if msg != "这个邮箱已经注册过了" {
		t.Fatalf("msg=%s err=%v", msg, err)
	}
	if strings.Contains(msg, secret) || bytes.Contains(logs.Bytes(), []byte(secret)) {
		t.Fatalf("password leaked msg=%s log=%s", msg, logs.String())
	}
	if len(paths) != 2 || paths[0] != "/api/v1/store/register" || paths[1] != "/api/user-panel/register" {
		t.Fatalf("paths=%v", paths)
	}
}

func TestBuyerRegisterPrefersStoreAPI(t *testing.T) {
	var paths []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"msg":"注册成功","data":{"userId":1}}`))
	}))
	defer server.Close()
	useBuyerSourceForTest(t, server.URL, server.Client())
	envelope, err := forwardBuyerRegister("/api/v1/store/register", "/api/user-panel/register", map[string]any{
		"email": "a@example.com", "password": "S3cret-pass",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "/api/v1/store/register" {
		t.Fatalf("paths=%v", paths)
	}
	raw, _ := json.Marshal(envelope)
	if bytes.Contains(raw, []byte("S3cret-pass")) {
		t.Fatalf("envelope kept password: %s", raw)
	}
}

func TestBuyerStoreRegisterRejectsBadInputWithoutCallingSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := "S3cret-pass"
	hits := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.NotFound(w, r)
	}))
	defer server.Close()
	useBuyerSourceForTest(t, server.URL, server.Client())

	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	code, msg := postJSONCode(t, BuyerStoreRegister, "/api/store/register", `{"email":"not-email","emailCode":"123456","nickname":"张三","password":"`+secret+`"}`)
	if code != 400 || msg != "请输入有效的邮箱地址" {
		t.Fatalf("code=%d msg=%s", code, msg)
	}
	if hits != 0 {
		t.Fatalf("bad input reached source: %d", hits)
	}
	if strings.Contains(msg, secret) || bytes.Contains(logs.Bytes(), []byte(secret)) {
		t.Fatal("password leaked on validation error")
	}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/store/register", strings.NewReader(`{"email":"a@example.com","emailCode":"123456","nickname":"张三","password":"`+secret+`"}`))
	ctx.Request.Host = "localhost"
	ctx.Request.Header.Set("Content-Type", "application/json")
	BuyerStoreRegister(ctx)
	if strings.Contains(rec.Body.String(), secret) {
		t.Fatal("password leaked when domain is rejected")
	}
	if hits != 0 {
		t.Fatalf("rejected domain still called source: %d", hits)
	}
	if !strings.Contains(rec.Body.String(), "请用正式域名") {
		t.Fatalf("domain response %s", rec.Body.String())
	}
}

func TestBuyerRegisterCaptchaFallsBackToPublicConfig(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/store/captcha" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/api/system-config/public" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"msg":"","data":{"geetestEnabled":true,"geetestCaptchaId":"captcha-old"}}`))
	}))
	defer server.Close()
	useBuyerSourceForTest(t, server.URL, server.Client())
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/store/register/captcha", nil)
	BuyerStoreRegisterCaptcha(ctx)
	var body struct {
		Code int `json:"code"`
		Data struct {
			Enabled   bool   `json:"enabled"`
			CaptchaID string `json:"captchaId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 200 || !body.Data.Enabled || body.Data.CaptchaID != "captcha-old" {
		t.Fatalf("captcha %+v raw=%s", body, rec.Body.String())
	}
}

func postJSONCode(t *testing.T, handler gin.HandlerFunc, path, payload string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	handler(ctx)
	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return body.Code, body.Msg
}
