package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type buyerAccountBody struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Bound                bool   `json:"bound"`
		ExplicitRevoked      bool   `json:"explicitRevoked"`
		BindingInvalid       bool   `json:"bindingInvalid"`
		BindingInvalidReason string `json:"bindingInvalidReason"`
		SourceVerified       bool   `json:"sourceVerified"`
		Reason               string `json:"reason"`
		Rebind               bool   `json:"rebind"`
		Edition              string `json:"edition"`
	} `json:"data"`
}

func seedBoundBuyer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AUTO_PRO_DATA_DIR", dir)
	writeTestBuyerBinding(t, dir)
	signed, err := signStoreSnapshot(storeSnapshot{
		BindingID: "sb_test", LicenseNo: "LIC1", Domain: "shop.example.com",
		Edition: storeEditionCommercial, Features: []string{storeFeatureMultiApp},
		Items: []storeSnapshotItem{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := saveBuyerSnapshot(buyerSnapshotState{
		Snapshot: signed, VerifiedAt: time.Now().Unix(), LastRefreshOK: true,
		BindingID: "sb_test", LicenseNo: "LIC1", AccountName: "买家", AccountRole: "user",
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func callBuyerAccount(t *testing.T, verify bool) buyerAccountBody {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	path := "/api/store/account"
	if verify {
		path += "?verify=1"
	}
	ctx.Request = httptest.NewRequest(http.MethodGet, path, nil)
	ctx.Request.Host = "shop.example.com"
	BuyerStoreAccount(ctx)
	var body buyerAccountBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.String(), err)
	}
	if recorder.Code != http.StatusOK || body.Code != 200 {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	return body
}

func assertBindingFiles(t *testing.T, dir string, kept bool) {
	t.Helper()
	_, keyErr := os.Stat(filepath.Join(dir, "store", "binding.key"))
	_, snapErr := os.Stat(buyerSnapshotPath())
	if kept {
		if keyErr != nil || snapErr != nil {
			t.Fatalf("绑定应保留 key=%v snap=%v", keyErr, snapErr)
		}
		return
	}
	if !os.IsNotExist(keyErr) || !os.IsNotExist(snapErr) {
		t.Fatalf("绑定应清掉 key=%v snap=%v", keyErr, snapErr)
	}
}

func TestBuyerAccountVerifyFollowsSourceBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	restore := useStoreSnapshotKeysForTest(pub, priv)
	t.Cleanup(restore)

	t.Run("source binding valid", func(t *testing.T) {
		dir := seedBoundBuyer(t)
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != buyerBindingCheckPath {
				t.Errorf("path %s", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"valid": true}})
		}))
		defer server.Close()
		client := server.Client()
		useBuyerSourceForTest(t, server.URL, client)
		body := callBuyerAccount(t, true)
		if !body.Data.Bound || !body.Data.SourceVerified || body.Data.BindingInvalid || body.Data.ExplicitRevoked || body.Data.Rebind {
			t.Fatalf("有效绑定 %+v", body.Data)
		}
		if buyerSnapshotTerminal(body.Data.Reason, false) {
			t.Fatalf("成功响应不应带终止原因 %q", body.Data.Reason)
		}
		assertBindingFiles(t, dir, true)
	})

	t.Run("source binding deleted", func(t *testing.T) {
		dir := seedBoundBuyer(t)
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 401,
				"msg":  "绑定不存在",
				"data": map[string]any{"reason": "binding_deleted", "revoked": true},
			})
		}))
		defer server.Close()
		useBuyerSourceForTest(t, server.URL, server.Client())
		body := callBuyerAccount(t, true)
		if body.Data.Bound || body.Data.SourceVerified || !body.Data.BindingInvalid || body.Data.BindingInvalidReason != "binding_deleted" || body.Data.ExplicitRevoked || body.Data.Rebind {
			t.Fatalf("已删除绑定 %+v", body.Data)
		}
		if buyerSnapshotTerminal(body.Data.Reason, false) {
			t.Fatalf("账号接口不应把终止原因放进 reason，否则前端会当成请求失败 %q", body.Data.Reason)
		}
		assertBindingFiles(t, dir, false)
	})

	t.Run("token invalid", func(t *testing.T) {
		dir := seedBoundBuyer(t)
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 401,
				"msg":  "绑定令牌已失效",
				"data": map[string]any{"reason": "token_invalid", "revoked": true},
			})
		}))
		defer server.Close()
		useBuyerSourceForTest(t, server.URL, server.Client())
		body := callBuyerAccount(t, true)
		if body.Data.Bound || body.Data.SourceVerified || !body.Data.BindingInvalid || body.Data.BindingInvalidReason != "token_invalid" || body.Data.Rebind {
			t.Fatalf("令牌失效 %+v", body.Data)
		}
		assertBindingFiles(t, dir, false)
	})

	t.Run("network keeps local binding", func(t *testing.T) {
		dir := seedBoundBuyer(t)
		useBuyerSourceForTest(t, "https://127.0.0.1:1", &http.Client{Timeout: time.Second})
		body := callBuyerAccount(t, true)
		if !body.Data.Bound || body.Data.SourceVerified || body.Data.BindingInvalid {
			t.Fatalf("网络失败应保留本地绑定 %+v", body.Data)
		}
		assertBindingFiles(t, dir, true)
	})

	t.Run("without verify does not ask source", func(t *testing.T) {
		dir := seedBoundBuyer(t)
		hit := false
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hit = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 401,
				"data": map[string]any{"reason": "binding_deleted", "revoked": true},
			})
		}))
		defer server.Close()
		useBuyerSourceForTest(t, server.URL, server.Client())
		body := callBuyerAccount(t, false)
		if hit {
			t.Fatal("未要求核对时不应访问源站")
		}
		if !body.Data.Bound || body.Data.SourceVerified || body.Data.BindingInvalid {
			t.Fatalf("本地快照 %+v", body.Data)
		}
		assertBindingFiles(t, dir, true)
	})
}

func TestRejectMismatchedStoreSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/store/binding", nil)
	rejectMismatchedStoreSignature(ctx)
	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Reason  string `json:"reason"`
			Revoked bool   `json:"revoked"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 401 || body.Data.Reason != "token_invalid" || !body.Data.Revoked || body.Msg != "绑定令牌已失效" {
		t.Fatalf("%+v", body)
	}
	if !buyerSnapshotTerminal(body.Data.Reason, body.Data.Revoked) {
		t.Fatal("令牌失效应视为终止")
	}
}

func TestBuyerLogoutWhenSourceBindingGone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	restore := useStoreSnapshotKeysForTest(pub, priv)
	t.Cleanup(restore)

	logout := func(t *testing.T) {
		t.Helper()
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/store/logout", nil)
		BuyerStoreLogout(ctx)
		var body struct {
			Code int `json:"code"`
			Data struct {
				OK bool `json:"ok"`
			} `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s %v", recorder.Body.String(), err)
		}
		if body.Code != 200 || !body.Data.OK {
			t.Fatalf("解除绑定应成功 %+v raw=%s", body, recorder.Body.String())
		}
	}

	t.Run("no local token skips source", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("AUTO_PRO_DATA_DIR", dir)
		hit := false
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hit = true
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()
		useBuyerSourceForTest(t, server.URL, server.Client())
		logout(t)
		if hit {
			t.Fatal("本地没有令牌时不应再请求源站删除")
		}
	})

	for _, reason := range []string{"binding_deleted", "token_invalid"} {
		t.Run(reason, func(t *testing.T) {
			dir := seedBoundBuyer(t)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code": 401,
					"msg":  "绑定已失效",
					"data": map[string]any{"reason": reason, "revoked": true},
				})
			}))
			defer server.Close()
			useBuyerSourceForTest(t, server.URL, server.Client())
			logout(t)
			assertBindingFiles(t, dir, false)
		})
	}
}

func TestCurrentBuyerAccessClearsEditionOnlyRevoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	restore := useStoreSnapshotKeysForTest(pub, priv)
	t.Cleanup(restore)
	dir := seedBoundBuyer(t)
	state, ok := loadBuyerSnapshot()
	if !ok {
		t.Fatal("缺少快照")
	}
	state.ExplicitRevoked = true
	state.RevokeReason = "edition_revoked"
	if err := saveBuyerSnapshot(state); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/store/account", nil)
	ctx.Request.Host = "shop.example.com"
	view := currentBuyerAccess(ctx)
	if !view.Bound || view.ExplicitRevoked {
		t.Fatalf("未开通商业版不应变成绑定失效 bound=%v revoked=%v reason=%s", view.Bound, view.ExplicitRevoked, view.Reason)
	}
	saved, ok := loadBuyerSnapshot()
	if !ok || saved.ExplicitRevoked || saved.RevokeReason != "" {
		t.Fatalf("磁盘上的误标应被清掉 ok=%v %+v", ok, saved.RevokeReason)
	}
	if _, err := os.Stat(filepath.Join(dir, "store", "binding.key")); err != nil {
		t.Fatal(err)
	}
}
