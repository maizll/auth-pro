package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBalancePurchaseDeductsRelativelyUnderRowLock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		role    string
		table   string
		handler gin.HandlerFunc
	}{
		{name: "user", role: "user", table: "users", handler: UserPurchase},
		{name: "agent", role: "agent", table: "agents", handler: AgentPanelPurchase},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := &purchaseLicenseTypeTestState{
				mask:      purchaseLicenseTypeKey,
				appExists: true,
			}
			db := openPurchaseLicenseTypeTestDB(t, state)
			usePurchaseLicenseTypeTestHooks(t, db)

			router := gin.New()
			router.POST("/purchase", func(c *gin.Context) {
				c.Set("role", test.role)
				c.Set("user_id", uint(7))
				test.handler(c)
			})
			request := httptest.NewRequest(http.MethodPost, "/purchase", strings.NewReader(`{"appId":9,"planId":3,"type":"key","payMethod":"balance"}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			var envelope struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode response: %v body=%s", err, response.Body.String())
			}
			if envelope.Code != 200 {
				t.Fatalf("purchase failed: code=%d msg=%q", envelope.Code, envelope.Msg)
			}

			state.mu.Lock()
			defer state.mu.Unlock()
			foundLock := false
			for _, query := range state.queries {
				if strings.Contains(query, "SELECT balance FROM "+test.table) && strings.Contains(query, "FOR UPDATE") {
					foundLock = true
				}
			}
			if !foundLock {
				t.Fatalf("missing SELECT balance ... FOR UPDATE: %v", state.queries)
			}
			foundRelative := false
			for index, query := range state.execQueries {
				if strings.Contains(query, "UPDATE "+test.table+" SET balance = ?") {
					t.Fatalf("absolute balance write: %s", query)
				}
				if !strings.Contains(query, "UPDATE "+test.table+" SET balance = balance - ?") || !strings.Contains(query, "balance >= ?") {
					continue
				}
				foundRelative = true
				args := state.execArgs[index]
				if len(args) < 3 || args[0].Value != args[2].Value {
					t.Fatalf("deduct args = %#v, want the cost as both the delta and the threshold", args)
				}
			}
			if !foundRelative {
				t.Fatalf("missing relative balance deduct: %v", state.execQueries)
			}
		})
	}
}

func TestUserBalancePurchaseDoesNotIssueLicenseWhenDeductMisses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	state := &purchaseLicenseTypeTestState{
		mask:                purchaseLicenseTypeKey,
		appExists:           true,
		rejectBalanceDeduct: true,
	}
	db := openPurchaseLicenseTypeTestDB(t, state)
	usePurchaseLicenseTypeTestHooks(t, db)

	router := gin.New()
	router.POST("/purchase", func(c *gin.Context) {
		c.Set("role", "user")
		c.Set("user_id", uint(7))
		UserPurchase(c)
	})
	request := httptest.NewRequest(http.MethodPost, "/purchase", strings.NewReader(`{"appId":9,"planId":3,"type":"key","payMethod":"balance"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v body=%s", err, response.Body.String())
	}
	if envelope.Code == 200 {
		t.Fatalf("missed deduct still succeeded: %s", response.Body.String())
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, query := range state.execQueries {
		if strings.Contains(query, "INSERT INTO licenses") {
			t.Fatal("license was inserted after the balance update affected 0 rows")
		}
	}
	if state.commits != 0 {
		t.Fatalf("transaction committed after a missed deduct: commits=%d", state.commits)
	}
}
