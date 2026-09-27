package handler

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func writeTestBuyerBinding(t *testing.T, dir string) {
	t.Helper()
	key, err := loadOrCreateStoreFileKey("master.key")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := sealStoreSecret(key, []byte("sb_test\n"+hex.EncodeToString(bytesRepeat(32))))
	if err != nil {
		t.Fatal(err)
	}
	secretPath := filepath.Join(dir, "store", "binding.key")
	if err := os.WriteFile(secretPath, sealed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "store", "install-id"), []byte("install-keep"), 0600); err != nil {
		t.Fatal(err)
	}
}

func bytesRepeat(n int) []byte {
	return make([]byte, n)
}

func useBuyerSourceForTest(t *testing.T, base string, client *http.Client) {
	t.Helper()
	prevBase := buyerSourceBaseForTest
	prevClient := sourceHTTPClientForTest
	buyerSourceBaseForTest = base
	sourceHTTPClientForTest = client
	t.Cleanup(func() {
		buyerSourceBaseForTest = prevBase
		sourceHTTPClientForTest = prevClient
	})
}

func TestSignedRequestClearsBindingOnTerminalReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	t.Setenv("AUTO_PRO_DATA_DIR", dir)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	restore := useStoreSnapshotKeysForTest(pub, priv)
	t.Cleanup(restore)

	for _, reason := range []string{"binding_revoked", "binding_deleted", "license_deleted", "license_revoked", "binding_expired", "token_invalid"} {
		writeTestBuyerBinding(t, dir)
		signed, err := signStoreSnapshot(storeSnapshot{
			BindingID: "sb_test", Domain: "shop.example.com", Edition: storeEditionCommercial,
			Features: []string{storeFeatureMultiApp}, Items: []storeSnapshotItem{},
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
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 401,
				"msg":  "绑定已失效",
				"data": map[string]any{"reason": reason, "revoked": true},
			})
		}))
		client := server.Client()
		client.Timeout = 5 * time.Second
		useBuyerSourceForTest(t, server.URL, client)

		err = refreshBuyerSnapshot(context.Background(), "")
		server.Close()
		var src *sourceResponseError
		if !errors.As(err, &src) || src.Reason != reason {
			t.Fatalf("reason %s err=%v", reason, err)
		}
		if _, statErr := os.Stat(filepath.Join(dir, "store", "binding.key")); !os.IsNotExist(statErr) {
			t.Fatalf("reason %s left binding.key: %v", reason, statErr)
		}
		if _, statErr := os.Stat(buyerSnapshotPath()); !os.IsNotExist(statErr) {
			t.Fatalf("reason %s rewrote snapshot: %v", reason, statErr)
		}
		if payload, readErr := os.ReadFile(filepath.Join(dir, "store", "install-id")); readErr != nil || string(payload) != "install-keep" {
			t.Fatalf("install id changed: %v %q", readErr, payload)
		}
		if _, statErr := os.Stat(filepath.Join(dir, "store", "master.key")); statErr != nil {
			t.Fatalf("master key removed: %v", statErr)
		}

		writeTestBuyerBinding(t, dir)
		server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 401,
				"msg":  "绑定已失效",
				"data": map[string]any{"reason": reason, "revoked": true},
			})
		}))
		client = server.Client()
		useBuyerSourceForTest(t, server.URL, client)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/store/orders", strings.NewReader(`{"planId":3}`))
		ctx.Request.Header.Set("Content-Type", "application/json")
		BuyerStoreOrderCreate(ctx)
		server.Close()
		var body struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				Reason  string `json:"reason"`
				Revoked bool   `json:"revoked"`
				Rebind  bool   `json:"rebind"`
			} `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != 400 || body.Msg != buyerRebindMessage || body.Data.Reason != reason || !body.Data.Rebind || !body.Data.Revoked {
			t.Fatalf("order response %+v raw=%s", body, recorder.Body.String())
		}
	}
}

func TestSignedRequestKeepsBindingOnOrdinaryFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AUTO_PRO_DATA_DIR", dir)
	writeTestBuyerBinding(t, dir)
	if err := os.WriteFile(buyerSnapshotPath(), []byte(`{"keep":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 500, "msg": "创建订单失败"})
	}))
	defer server.Close()
	client := server.Client()
	useBuyerSourceForTest(t, server.URL, client)
	err := signedSourceJSON(http.MethodPost, "/api/v1/store/orders", map[string]any{"planId": 1}, nil)
	if err == nil || buyerRefreshFailureRevoked(err) {
		t.Fatalf("expected ordinary refusal, err=%v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "store", "binding.key")); statErr != nil {
		t.Fatal("ordinary error cleared binding.key")
	}
	if _, statErr := os.Stat(buyerSnapshotPath()); statErr != nil {
		t.Fatal("ordinary error cleared snapshot")
	}

	useBuyerSourceForTest(t, "https://127.0.0.1:1", &http.Client{Timeout: time.Second})
	err = signedSourceJSON(http.MethodGet, "/api/v1/store/status", nil, nil)
	if err == nil || err.Error() != "无法连接源站" {
		t.Fatalf("network err=%v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "store", "binding.key")); statErr != nil {
		t.Fatal("network error cleared binding.key")
	}
}
