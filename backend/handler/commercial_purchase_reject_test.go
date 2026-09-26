package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestOrdinaryPurchaseRejectsCommercialProductPlan(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		role    string
		handler gin.HandlerFunc
	}{
		{name: "user", role: "user", handler: UserPurchase},
		{name: "agent", role: "agent", handler: AgentPanelPurchase},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &purchaseLicenseTypeTestState{
				mask:              purchaseLicenseTypeAll,
				appExists:         true,
				commercialProduct: 1,
			}
			db := openPurchaseLicenseTypeTestDB(t, state)
			usePurchaseLicenseTypeTestHooks(t, db)
			router := gin.New()
			router.POST("/purchase", func(c *gin.Context) {
				c.Set("role", tt.role)
				c.Set("user_id", uint(7))
				tt.handler(c)
			})
			body, _ := json.Marshal(map[string]any{
				"appId": 9, "planId": 3, "type": "domain", "domain": "shop.example.com", "payMethod": "balance",
			})
			request := httptest.NewRequest(http.MethodPost, "/purchase", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			var envelope struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Code != 400 || !strings.Contains(envelope.Msg, "本站商业版") || !strings.Contains(envelope.Msg, "升级商业版") {
				t.Fatalf("response = %d %q", envelope.Code, envelope.Msg)
			}
			state.mu.Lock()
			defer state.mu.Unlock()
			if state.execCount != 0 {
				t.Fatalf("rejected purchase still wrote %d statements", state.execCount)
			}
		})
	}
}
