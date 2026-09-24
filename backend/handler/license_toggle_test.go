package handler

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

type licenseToggleState struct {
	mu        sync.Mutex
	id        string
	status    string
	ownerType string
	ownerID   int64
	licenseNo string
	appName   string
	updates   int
}

type licenseToggleDriver struct{ state *licenseToggleState }
type licenseToggleConn struct{ state *licenseToggleState }
type licenseToggleResult struct{ affected int64 }
type licenseToggleRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

var licenseToggleDriverSeq atomic.Uint64

func openLicenseToggleDB(t *testing.T, state *licenseToggleState) {
	t.Helper()
	name := fmt.Sprintf("license-toggle-%d", licenseToggleDriverSeq.Add(1))
	sql.Register(name, licenseToggleDriver{state: state})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })
}

func (driver licenseToggleDriver) Open(string) (driver.Conn, error) {
	return &licenseToggleConn{state: driver.state}, nil
}

func (*licenseToggleConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare is not supported")
}
func (*licenseToggleConn) Close() error { return nil }
func (*licenseToggleConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("begin is not supported")
}

func (conn *licenseToggleConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	conn.state.mu.Lock()
	defer conn.state.mu.Unlock()
	if !strings.Contains(query, "FROM licenses") || !strings.Contains(query, "owner_type") {
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
	id, _ := namedString(args, 0)
	if id != conn.state.id {
		return &licenseToggleRows{columns: []string{"owner_type", "owner_id", "license_no", "app_name"}}, nil
	}
	return &licenseToggleRows{
		columns: []string{"owner_type", "owner_id", "license_no", "app_name"},
		values: [][]driver.Value{{
			conn.state.ownerType, conn.state.ownerID, conn.state.licenseNo, conn.state.appName,
		}},
	}, nil
}

func (conn *licenseToggleConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	conn.state.mu.Lock()
	defer conn.state.mu.Unlock()
	if !strings.Contains(query, "UPDATE licenses SET status") {
		return nil, fmt.Errorf("unexpected exec: %s", query)
	}
	if !strings.Contains(query, "status <>") {
		return nil, fmt.Errorf("UPDATE licenses must skip an unchanged status: %s", query)
	}
	newStatus, _ := namedString(args, 0)
	id, _ := namedString(args, 1)
	guard, _ := namedString(args, 2)
	if newStatus == "" || newStatus != guard {
		return nil, fmt.Errorf("status args = %#v", args)
	}
	if id != conn.state.id || newStatus == conn.state.status {
		return licenseToggleResult{affected: 0}, nil
	}
	conn.state.status = newStatus
	conn.state.updates++
	return licenseToggleResult{affected: 1}, nil
}

func (result licenseToggleResult) LastInsertId() (int64, error) { return 0, nil }
func (result licenseToggleResult) RowsAffected() (int64, error) { return result.affected, nil }
func (rows *licenseToggleRows) Columns() []string               { return rows.columns }
func (*licenseToggleRows) Close() error                         { return nil }
func (rows *licenseToggleRows) Next(dest []driver.Value) error {
	if rows.index >= len(rows.values) {
		return io.EOF
	}
	copy(dest, rows.values[rows.index])
	rows.index++
	return nil
}

func TestLicenseToggleNotifiesOwnerOnStatusChange(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("user", func(t *testing.T) {
		store := newMemoryNotificationStore()
		t.Cleanup(SetNotificationStoreForTest(store))
		state := &licenseToggleState{
			id: "7", status: "active", ownerType: "user", ownerID: 12,
			licenseNo: "LIC-12", appName: "演示应用",
		}
		openLicenseToggleDB(t, state)

		response := putLicenseToggle(t, "7", "disabled")
		if response.Code != 200 {
			t.Fatalf("toggle code = %d, body = %+v", response.Code, response)
		}
		if state.status != "revoked" || state.updates != 1 {
			t.Fatalf("state = %+v, want revoked after one update", state)
		}
		notices := licenseStatusNotices(t, notificationRoleUser, 12)
		if len(notices) != 1 {
			t.Fatalf("notices = %#v, want one status change", notices)
		}
		notice := notices[0]
		if notice.EventType != "license_status_changed" || notice.RefID != "LIC-12" || notice.Link != "/user/licenses" {
			t.Fatalf("notice = %#v", notice)
		}
		if !strings.Contains(notice.Body, "演示应用") || !strings.Contains(notice.Body, "LIC-12") || !strings.Contains(notice.Body, "已禁用") {
			t.Fatalf("notice body = %q", notice.Body)
		}
		if others := licenseStatusNotices(t, notificationRoleUser, 99); len(others) != 0 {
			t.Fatalf("another user received the notice: %#v", others)
		}

		response = putLicenseToggle(t, "7", "disabled")
		if response.Code != 200 {
			t.Fatalf("repeat toggle code = %d", response.Code)
		}
		if state.updates != 1 {
			t.Fatalf("unchanged status wrote again: updates = %d", state.updates)
		}
		if notices = licenseStatusNotices(t, notificationRoleUser, 12); len(notices) != 1 {
			t.Fatalf("repeat toggle added a notice: %#v", notices)
		}

		response = putLicenseToggle(t, "7", "active")
		if response.Code != 200 || state.status != "active" {
			t.Fatalf("enable code = %d status = %s", response.Code, state.status)
		}
		notices = licenseStatusNotices(t, notificationRoleUser, 12)
		if len(notices) != 2 || !strings.Contains(notices[0].Body, "正常") {
			t.Fatalf("enable notices = %#v", notices)
		}
	})

	t.Run("agent", func(t *testing.T) {
		store := newMemoryNotificationStore()
		t.Cleanup(SetNotificationStoreForTest(store))
		state := &licenseToggleState{
			id: "8", status: "active", ownerType: "agent", ownerID: 34,
			licenseNo: "LIC-34", appName: "代理应用",
		}
		openLicenseToggleDB(t, state)

		response := putLicenseToggle(t, "8", "disabled")
		if response.Code != 200 || state.status != "revoked" {
			t.Fatalf("toggle code = %d status = %s", response.Code, state.status)
		}
		notices := licenseStatusNotices(t, notificationRoleAgent, 34)
		if len(notices) != 1 || notices[0].Link != "/agent-panel/licenses" || !strings.Contains(notices[0].Body, "已禁用") {
			t.Fatalf("agent notices = %#v", notices)
		}
		if users := licenseStatusNotices(t, notificationRoleUser, 34); len(users) != 0 {
			t.Fatalf("user list saw the agent notice: %#v", users)
		}
	})

	t.Run("missing license", func(t *testing.T) {
		store := newMemoryNotificationStore()
		t.Cleanup(SetNotificationStoreForTest(store))
		state := &licenseToggleState{
			id: "7", status: "active", ownerType: "user", ownerID: 12,
			licenseNo: "LIC-12", appName: "演示应用",
		}
		openLicenseToggleDB(t, state)

		response := putLicenseToggle(t, "404", "disabled")
		if response.Code != 200 || state.status != "active" || state.updates != 0 {
			t.Fatalf("missing license response = %+v state = %+v", response, state)
		}
		if notices := licenseStatusNotices(t, notificationRoleUser, 12); len(notices) != 0 {
			t.Fatalf("missing license notified the owner: %#v", notices)
		}
	})
}

func putLicenseToggle(t *testing.T, id, status string) struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
} {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: id}}
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/license/"+id+"/toggle", bytes.NewBufferString(`{"status":"`+status+`"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	LicenseToggle(ctx)

	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
	return response
}

func licenseStatusNotices(t *testing.T, role string, userID int64) []inAppNotification {
	t.Helper()
	items, err := currentNotificationStore().List(notificationListFilter{
		notificationRecipient: notificationRecipient{Role: role, UserID: userID},
		Limit:                 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	matched := make([]inAppNotification, 0)
	for _, item := range items {
		if item.EventType == "license_status_changed" {
			matched = append(matched, item)
		}
	}
	return matched
}
