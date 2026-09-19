package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// 广告缓存是包级状态，每个用例开跑前必须清空，否则用例之间会互相命中缓存。
func resetAdvertisementCache(t *testing.T) {
	t.Helper()
	advertisementCacheL.Lock()
	defer advertisementCacheL.Unlock()
	advertisementCache = map[string]advertisementCacheEntry{}
}

// upstreamStub 起一个假的广告服务，并把 AUTO_PRO_ADVERTISEMENT_URL 指过去。
// config 每次读环境变量，所以不需要在生产代码里留测试钩子。
func upstreamStub(t *testing.T, handler http.HandlerFunc) *int32 {
	t.Helper()
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	t.Setenv("AUTO_PRO_ADVERTISEMENT_URL", server.URL)
	resetAdvertisementCache(t)
	return &calls
}

func callAdvertisements(t *testing.T, position string) (int, []advertisementRecord) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/advertisements?position="+position, nil)
	PublicAdvertisements(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP 状态应始终为 200，实际 %d", recorder.Code)
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			Records []advertisementRecord `json:"records"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应解析失败：%v，body=%s", err, recorder.Body.String())
	}
	return body.Code, body.Data.Records
}

func upstreamPayload(records string) string {
	return fmt.Sprintf(`{"code":200,"msg":"ok","data":{"records":[%s]}}`, records)
}

func windowedRecord(position, id string, weight int, start, end time.Time) string {
	return fmt.Sprintf(
		`{"id":%q,"title":"活动%s","imageUrl":"https://example.com/%s.png","destinationUrl":"https://example.com/%s","position":%q,"weight":%d,"startAt":%q,"endAt":%q,"description":""}`,
		id, id, id, id, position, weight, start.Format(time.RFC3339), end.Format(time.RFC3339),
	)
}

func activeRecord(position, id string, weight int) string {
	now := time.Now()
	return windowedRecord(position, id, weight, now.Add(-time.Hour), now.Add(time.Hour))
}

func TestPublicAdvertisementsPassesThroughUpstreamFields(t *testing.T) {
	upstreamStub(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("position"); got != "home-banner" {
			t.Errorf("上游收到的 position=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, upstreamPayload(activeRecord("home-banner", "1", 10)))
	})

	code, records := callAdvertisements(t, "home-banner")
	if code != 200 || len(records) != 1 {
		t.Fatalf("code=%d records=%d", code, len(records))
	}
	record := records[0]
	if record.ID != "1" || record.ImageURL != "https://example.com/1.png" ||
		record.DestinationURL != "https://example.com/1" || record.Weight != 10 {
		t.Fatalf("字段透传有损：%+v", record)
	}
}

func TestPublicAdvertisementsServesFromCacheWithinTTL(t *testing.T) {
	calls := upstreamStub(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, upstreamPayload(activeRecord("home-banner", "1", 0)))
	})

	callAdvertisements(t, "home-banner")
	callAdvertisements(t, "home-banner")

	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("TTL 内应只打一次上游，实际 %d 次", got)
	}
}

func TestPublicAdvertisementsCachesEmptyResult(t *testing.T) {
	calls := upstreamStub(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":200,"msg":"ok","data":{"records":null}}`)
	})

	code, records := callAdvertisements(t, "sidebar")
	if code != 200 || records == nil || len(records) != 0 {
		t.Fatalf("records 为 null 时应归一成空数组，code=%d records=%v", code, records)
	}
	callAdvertisements(t, "sidebar")
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("空投放同样要走负缓存，实际打了 %d 次上游", got)
	}
}

func TestPublicAdvertisementsFallsBackToStaleCache(t *testing.T) {
	var fail atomic.Bool
	upstreamStub(t, func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, upstreamPayload(activeRecord("popup", "1", 0)))
	})

	callAdvertisements(t, "popup")

	// 把缓存推到 TTL 之外、staleTTL 之内，逼出降级路径。
	advertisementCacheL.Lock()
	entry := advertisementCache["popup"]
	entry.fetchedAt = time.Now().Add(-10 * time.Minute)
	advertisementCache["popup"] = entry
	advertisementCacheL.Unlock()

	fail.Store(true)
	code, records := callAdvertisements(t, "popup")
	if code != 200 || len(records) != 1 || records[0].ID != "1" {
		t.Fatalf("上游失败时应返回旧缓存，code=%d records=%v", code, records)
	}
}

func TestPublicAdvertisementsReturnsEmptyWhenUpstreamDown(t *testing.T) {
	upstreamStub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})

	code, records := callAdvertisements(t, "home-banner")
	if code != 200 || len(records) != 0 {
		t.Fatalf("无缓存且上游挂掉时应返回空列表且不报错，code=%d records=%v", code, records)
	}
}

func TestPublicAdvertisementsFiltersAndSorts(t *testing.T) {
	now := time.Now()
	upstreamStub(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, upstreamPayload(
			activeRecord("home-banner", "low", 1)+","+
				windowedRecord("home-banner", "expired", 99, now.Add(-48*time.Hour), now.Add(-24*time.Hour))+","+
				windowedRecord("home-banner", "future", 99, now.Add(24*time.Hour), now.Add(48*time.Hour))+","+
				activeRecord("home-banner", "high", 5),
		))
	})

	code, records := callAdvertisements(t, "home-banner")
	if code != 200 || len(records) != 2 {
		t.Fatalf("过期与未开始的投放应被过滤，code=%d records=%v", code, records)
	}
	if records[0].ID != "high" || records[1].ID != "low" {
		t.Fatalf("应按 weight 倒序，实际 %s,%s", records[0].ID, records[1].ID)
	}
}

func TestNormalizeAdvertisementsKeepsRecordsWithoutTimeWindow(t *testing.T) {
	records := normalizeAdvertisements([]advertisementRecord{
		{ID: "no-window"},
		{ID: "bad-window", StartAt: "2026/09/01", EndAt: "not-a-time"},
		{ID: "other-slot", Position: "sidebar"},
	}, "home-banner", time.Now())

	if len(records) != 2 || records[0].ID != "no-window" || records[1].ID != "bad-window" {
		t.Fatalf("时间字段缺失或无法解析时不应误杀，实际 %v", records)
	}
}

func TestPublicAdvertisementsUsesLocalSourceByDefault(t *testing.T) {
	t.Setenv("AUTO_PRO_ADVERTISEMENT_URL", "")
	resetAdvertisementCache(t)

	code, records := callAdvertisements(t, "home-banner")
	if code != 200 || records == nil || len(records) != 0 {
		t.Fatalf("本站默认广告应返回空列表且不报错，code=%d records=%v", code, records)
	}
}

func TestPublicLocalAdvertisementsMatchesProxyShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public/advertisements?position=sidebar", nil)
	PublicLocalAdvertisements(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP 状态 = %d", recorder.Code)
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			Records []advertisementRecord `json:"records"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 200 || body.Data.Records == nil || len(body.Data.Records) != 0 {
		t.Fatalf("本站广告接口形状不兼容：%s", recorder.Body.String())
	}
}

func TestPublicAdvertisementsRejectsUnknownPosition(t *testing.T) {
	calls := upstreamStub(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, upstreamPayload(""))
	})

	code, _ := callAdvertisements(t, "footer")
	if code != 400 {
		t.Fatalf("非白名单位置应返回 400，实际 %d", code)
	}
	if got := atomic.LoadInt32(calls); got != 0 {
		t.Fatalf("非法位置不应请求上游，实际 %d 次", got)
	}
}
