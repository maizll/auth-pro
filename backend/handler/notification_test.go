package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"auto_pro/middleware"

	"github.com/golang-jwt/jwt/v5"
)

func notificationToken(t *testing.T, role string, userID uint, username string) string {
	t.Helper()
	claims := middleware.Claims{
		UserID: userID, Username: username, Role: role,
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}
	if role == "admin" {
		claims.RoleCode = "R_SUPER"
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(middleware.JWTSecret())
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func notificationListEvents(t *testing.T, router http.Handler, token, query string) []string {
	t.Helper()
	path := "/api/v1/notifications"
	if query != "" {
		path += "?" + query
	}
	rec := sourceJSON(t, router, http.MethodGet, path, token, "")
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("list %s = %s", query, rec.Body.String())
	}
	var body struct {
		Data struct {
			List []struct {
				EventType string `json:"eventType"`
				Title     string `json:"title"`
				Read      bool   `json:"read"`
				ID        int64  `json:"id"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	events := make([]string, 0, len(body.Data.List))
	for _, item := range body.Data.List {
		events = append(events, item.EventType)
	}
	return events
}

func notificationContains(events []string, want string) bool {
	for _, event := range events {
		if event == want {
			return true
		}
	}
	return false
}

func TestNotificationApplySubmittedNotifiesAdminNotApplicant(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	_, _, agent := sourceNextAgent(t, "notify-apply")

	apply := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/apply", agent, `{}`)
	if sourceBodyCode(t, apply) != 200 {
		t.Fatalf("apply=%s", apply.Body.String())
	}

	adminEvents := notificationListEvents(t, router, admin, "tab=notice")
	if !notificationContains(adminEvents, "developer_apply_submitted") {
		t.Fatalf("admin notice missing apply event: %v body=%s", adminEvents,
			sourceJSON(t, router, http.MethodGet, "/api/v1/notifications?tab=notice", admin, "").Body.String())
	}
	if notificationContains(notificationListEvents(t, router, agent, ""), "developer_apply_submitted") {
		t.Fatal("applicant must not receive the admin pending notice")
	}
}

func TestNotificationApproveApplyNotifiesDeveloperAgent(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin, agent, _ := sourceApproveDeveloper(t, router, "notify-approved", "")

	events := notificationListEvents(t, router, agent, "tab=message")
	if !notificationContains(events, "developer_apply_approved") {
		t.Fatalf("approved agent missing message: %v", events)
	}
	if notificationContains(notificationListEvents(t, router, admin, "tab=message"), "developer_apply_approved") {
		t.Fatal("admin must not receive the applicant's approval message")
	}
}

func TestNotificationDeprecatePluginNotifiesOwner(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin, dev, _ := sourceApproveDeveloper(t, router, "notify-deprecate", "")
	sha := sourceTestSHA256()
	pluginBody := `{"appId":1,"id":"notify-plugin","name":"通知插件","version":"1.0.0","description":"notify","category":"other","downloadUrl":"https://cdn.example.com/notify-plugin.zip","sha256":"` + sha + `"}`
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins", dev, pluginBody); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("save plugin=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins/notify-plugin/submit", dev, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("submit=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/notify-plugin/approve", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("approve=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/notify-plugin/shelf", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("shelf=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/notify-plugin/deprecate", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("deprecate=%s", rec.Body.String())
	}

	events := notificationListEvents(t, router, dev, "tab=message")
	if !notificationContains(events, "catalog_item_deprecated") {
		t.Fatalf("owner missing deprecate message: %v", events)
	}
	if !notificationContains(notificationListEvents(t, router, admin, "tab=notice"), "catalog_item_submitted") {
		t.Fatal("admin should see catalog submit notice")
	}
}

func TestNotificationListReadAndIsolation(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	user := notificationToken(t, "user", 88, "user88")
	agent := notificationToken(t, "agent", 77, "agent77")

	notifyRole(notificationRoleUser, 88, notificationTabMessage, "订单已支付", "授权已开通", "/user/licenses", "order_paid", "order", "88")
	notifyRole(notificationRoleAgent, 77, notificationTabMessage, "等级已变更", "升级为金牌", "/agent-panel/profile", "agent_level_changed", "agent", "77")
	notifyRole(notificationRoleAdmin, 1, notificationTabNotice, "新工单", "用户提交了工单", "/tickets", "ticket_created", "ticket", "1")

	if notificationContains(notificationListEvents(t, router, user, ""), "agent_level_changed") ||
		notificationContains(notificationListEvents(t, router, user, ""), "ticket_created") {
		t.Fatal("user list leaked another role")
	}
	if notificationContains(notificationListEvents(t, router, agent, ""), "order_paid") ||
		notificationContains(notificationListEvents(t, router, agent, ""), "ticket_created") {
		t.Fatal("agent list leaked another role")
	}
	if notificationContains(notificationListEvents(t, router, admin, ""), "order_paid") ||
		notificationContains(notificationListEvents(t, router, admin, ""), "agent_level_changed") {
		t.Fatal("admin list leaked another role")
	}

	userList := sourceJSON(t, router, http.MethodGet, "/api/v1/notifications?unread=1", user, "")
	var listed struct {
		Data struct {
			List []struct {
				ID    int64  `json:"id"`
				Read  bool   `json:"read"`
				Title string `json:"title"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(userList.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Data.List) != 1 || listed.Data.List[0].Read {
		t.Fatalf("user unread=%s", userList.Body.String())
	}
	read := sourceJSON(t, router, http.MethodPost, "/api/v1/notifications/"+itoa64(listed.Data.List[0].ID)+"/read", user, "")
	if sourceBodyCode(t, read) != 200 {
		t.Fatalf("read=%s", read.Body.String())
	}
	stolen := sourceJSON(t, router, http.MethodPost, "/api/v1/notifications/"+itoa64(listed.Data.List[0].ID)+"/read", agent, "")
	if sourceBodyCode(t, stolen) != 404 {
		t.Fatalf("cross-role read must 404, got %s", stolen.Body.String())
	}

	count := sourceJSON(t, router, http.MethodGet, "/api/v1/notifications/unread-count", user, "")
	if sourceBodyCode(t, count) != 200 || !strings.Contains(count.Body.String(), `"count":0`) {
		t.Fatalf("user unread after read=%s", count.Body.String())
	}

	notifyRole(notificationRoleUser, 88, notificationTabNotice, "密码已修改", "请确认是本人操作", "/user/profile", "password_changed", "user", "88")
	all := sourceJSON(t, router, http.MethodPost, "/api/v1/notifications/read-all", user, "")
	if sourceBodyCode(t, all) != 200 {
		t.Fatalf("read-all=%s", all.Body.String())
	}
	after := sourceJSON(t, router, http.MethodGet, "/api/v1/notifications/unread-count", user, "")
	if !strings.Contains(after.Body.String(), `"count":0`) {
		t.Fatalf("unread after read-all=%s", after.Body.String())
	}
}

func TestNotificationRejectApplyNotifiesAgent(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	_, _, token := sourceNextAgent(t, "notify-reject")
	apply := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/apply", token, `{}`)
	var applyBody struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(apply.Body.Bytes(), &applyBody); err != nil {
		t.Fatal(err)
	}
	reject := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/applications/"+itoa64(applyBody.Data.ID)+"/reject",
		admin, `{"note":"资料不全"}`)
	if sourceBodyCode(t, reject) != 200 {
		t.Fatalf("reject=%s", reject.Body.String())
	}
	events := notificationListEvents(t, router, token, "tab=message")
	if !notificationContains(events, "developer_apply_rejected") {
		t.Fatalf("rejected agent missing message: %v", events)
	}
}
