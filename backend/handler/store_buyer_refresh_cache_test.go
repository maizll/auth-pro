package handler

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestBuyerSnapshotRefreshThrottle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	restore := useStoreSnapshotKeysForTest(pub, priv)
	t.Cleanup(restore)
	t.Cleanup(func() { buyerSnapshotNow = time.Now })

	t.Run("cache and force window", func(t *testing.T) {
		seedBoundBuyer(t)
		now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
		buyerSnapshotNow = func() time.Time { return now }
		hits := 0
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/store/status" {
				t.Errorf("path %s", r.URL.Path)
			}
			hits++
			writeSignedStatus(t, w, storeEditionCommercial)
		}))
		defer server.Close()
		useBuyerSourceForTest(t, server.URL, server.Client())

		if err := refreshBuyerSnapshot(context.Background(), "shop.example.com"); err != nil {
			t.Fatal(err)
		}
		if hits != 1 {
			t.Fatalf("首次刷新应请求官网，实际 %d", hits)
		}
		now = now.Add(20 * time.Second)
		if err := refreshBuyerSnapshot(context.Background(), "shop.example.com"); err != nil {
			t.Fatal(err)
		}
		if hits != 1 {
			t.Fatalf("45 秒内重复刷新不应再请求官网，实际 %d", hits)
		}
		if err := refreshBuyerSnapshotForced(context.Background(), "shop.example.com"); err != nil {
			t.Fatal(err)
		}
		if hits != 2 {
			t.Fatalf("立即刷新应绕过缓存，实际 %d", hits)
		}
		now = now.Add(10 * time.Second)
		if err := refreshBuyerSnapshotForced(context.Background(), "shop.example.com"); err != nil {
			t.Fatal(err)
		}
		if hits != 2 {
			t.Fatalf("立即刷新 30 秒内应限频，实际 %d", hits)
		}
		now = now.Add(50 * time.Second)
		if err := refreshBuyerSnapshot(context.Background(), "shop.example.com"); err != nil {
			t.Fatal(err)
		}
		if hits != 3 {
			t.Fatalf("缓存过期后应再次请求官网，实际 %d", hits)
		}
	})

	t.Run("concurrent calls share one request", func(t *testing.T) {
		seedBoundBuyer(t)
		buyerSnapshotNow = time.Now
		var hits atomic.Int32
		entered := make(chan struct{})
		release := make(chan struct{})
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hits.Add(1) == 1 {
				close(entered)
			}
			<-release
			writeSignedStatus(t, w, storeEditionCommercial)
		}))
		defer server.Close()
		useBuyerSourceForTest(t, server.URL, server.Client())

		start := make(chan struct{})
		errCh := make(chan error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errCh <- refreshBuyerSnapshot(context.Background(), "shop.example.com")
			}()
		}
		close(start)
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("官网请求没有发出")
		}
		deadline := time.Now().Add(2 * time.Second)
		for {
			buyerSnapshotRefresh.mu.Lock()
			waiting := buyerSnapshotRefresh.waiting
			buyerSnapshotRefresh.mu.Unlock()
			if waiting >= 1 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("第二个刷新没有合并到同一次官网请求")
			}
			time.Sleep(5 * time.Millisecond)
		}
		close(release)
		wg.Wait()
		close(errCh)
		for err := range errCh {
			if err != nil {
				t.Fatal(err)
			}
		}
		if hits.Load() != 1 {
			t.Fatalf("并发刷新应合并为一次官网请求，实际 %d", hits.Load())
		}
	})

	t.Run("official failure keeps local snapshot", func(t *testing.T) {
		dir := seedBoundBuyer(t)
		now := time.Date(2026, 9, 27, 13, 0, 0, 0, time.UTC)
		buyerSnapshotNow = func() time.Time { return now }
		hits := 0
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 500, "msg": "读取配置失败"})
		}))
		defer server.Close()
		useBuyerSourceForTest(t, server.URL, server.Client())

		err := refreshBuyerSnapshotForced(context.Background(), "shop.example.com")
		if err == nil {
			t.Fatal("官网失败应返回错误")
		}
		state, ok := loadBuyerSnapshot()
		if !ok || state.Snapshot.Edition != storeEditionCommercial {
			edition := ""
			if ok {
				edition = state.Snapshot.Edition
			}
			t.Fatalf("官网失败应沿用本地商业版 ok=%v edition=%s", ok, edition)
		}
		assertBindingFiles(t, dir, true)
		now = now.Add(5 * time.Second)
		if err := refreshBuyerSnapshotForced(context.Background(), "shop.example.com"); err != nil {
			t.Fatal(err)
		}
		if hits != 1 {
			t.Fatalf("失败后的立即刷新仍应限频，实际 %d", hits)
		}
		state, ok = loadBuyerSnapshot()
		if !ok || state.Snapshot.Edition != storeEditionCommercial {
			t.Fatal("限频后的立即刷新不应清掉本地商业版")
		}
	})

	t.Run("forced refresh pulls latest edition", func(t *testing.T) {
		dir := seedBoundBuyer(t)
		now := time.Date(2026, 9, 27, 14, 0, 0, 0, time.UTC)
		buyerSnapshotNow = func() time.Time { return now }
		hits := 0
		edition := storeEditionFree
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/store/status" {
				t.Errorf("path %s", r.URL.Path)
			}
			hits++
			writeSignedStatus(t, w, edition)
		}))
		defer server.Close()
		useBuyerSourceForTest(t, server.URL, server.Client())

		body := callBuyerRefresh(t, true)
		if !body.Data.Bound || body.Data.BindingInvalid || body.Data.Edition != storeEditionFree {
			t.Fatalf("立即刷新应拉到免费版 %+v", body.Data)
		}
		if hits != 1 {
			t.Fatalf("立即刷新应请求官网一次，实际 %d", hits)
		}
		edition = storeEditionCommercial
		now = now.Add(5 * time.Second)
		body = callBuyerRefresh(t, true)
		if hits != 1 {
			t.Fatalf("限频内的立即刷新不应再请求官网，实际 %d", hits)
		}
		if body.Data.Edition != storeEditionFree {
			t.Fatalf("限频时应沿用刚才的本地快照 %+v", body.Data)
		}
		assertBindingFiles(t, dir, true)
	})

	t.Run("json body force bypasses cache", func(t *testing.T) {
		seedBoundBuyer(t)
		now := time.Date(2026, 9, 27, 15, 0, 0, 0, time.UTC)
		buyerSnapshotNow = func() time.Time { return now }
		hits := 0
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			writeSignedStatus(t, w, storeEditionFree)
		}))
		defer server.Close()
		useBuyerSourceForTest(t, server.URL, server.Client())

		if err := refreshBuyerSnapshot(context.Background(), "shop.example.com"); err != nil {
			t.Fatal(err)
		}
		body := callBuyerRefreshJSON(t, `{"force":1}`)
		if hits != 2 || body.Data.Edition != storeEditionFree || !body.Data.Bound {
			t.Fatalf("JSON 立即刷新应绕过缓存 hits=%d body=%+v", hits, body.Data)
		}
		now = now.Add(5 * time.Second)
		body = callBuyerRefreshJSON(t, `{"force":1}`)
		if hits != 2 || body.Data.Edition != storeEditionFree {
			t.Fatalf("JSON 立即刷新限频后应沿用本地快照 hits=%d body=%+v", hits, body.Data)
		}
	})
}

func callBuyerRefresh(t *testing.T, force bool) buyerAccountBody {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	path := "/api/store/refresh"
	if force {
		path += "?force=1"
	}
	ctx.Request = httptest.NewRequest(http.MethodPost, path, nil)
	ctx.Request.Host = "shop.example.com"
	BuyerStoreRefresh(ctx)
	var body buyerAccountBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.String(), err)
	}
	if recorder.Code != http.StatusOK || body.Code != 200 {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	return body
}

func callBuyerRefreshJSON(t *testing.T, payload string) buyerAccountBody {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/store/refresh", strings.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Host = "shop.example.com"
	BuyerStoreRefresh(ctx)
	var body buyerAccountBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.String(), err)
	}
	if recorder.Code != http.StatusOK || body.Code != 200 {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	return body
}
