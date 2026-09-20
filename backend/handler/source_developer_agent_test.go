package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"auto_pro/middleware"

	"github.com/golang-jwt/jwt/v5"
)

func TestSourceDeveloperApplyRequiresAgentAuth(t *testing.T) {
	router, _ := sourceStationRouter(t)

	unauth := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/apply", "",
		`{"username":"legacy","password":"secret1","email":"a@b.c","displayName":"x","reason":"old"}`)
	if sourceBodyCode(t, unauth) != 410 {
		t.Fatalf("unauthenticated apply=%s", unauth.Body.String())
	}

	admin := sourceAdminToken(t)
	asAdmin := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/apply", admin, `{}`)
	if sourceBodyCode(t, asAdmin) != 403 {
		t.Fatalf("admin apply=%s", asAdmin.Body.String())
	}
}

func TestSourceDeveloperAgentApplyWithoutBodyCreatesPending(t *testing.T) {
	router, store := sourceStationRouter(t)
	agentID, email, token := sourceNextAgent(t, "apply-agent")

	apply := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/apply", token, `{}`)
	if sourceBodyCode(t, apply) != 200 || !strings.Contains(apply.Body.String(), `"pending"`) {
		t.Fatalf("apply=%s", apply.Body.String())
	}
	if strings.Contains(apply.Body.String(), "password") {
		t.Fatalf("apply must not echo password: %s", apply.Body.String())
	}

	var body struct {
		Data struct {
			ID       int64  `json:"id"`
			AgentID  int64  `json:"agentId"`
			Username string `json:"username"`
			Status   string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(apply.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.AgentID != int64(agentID) || body.Data.Username != email || body.Data.Status != sourceApplicationPending {
		t.Fatalf("apply data=%+v email=%s agentID=%d", body.Data, email, agentID)
	}

	app, err := store.GetApplication(body.Data.ID)
	if err != nil {
		t.Fatal(err)
	}
	if app.PasswordHash != "" {
		t.Fatalf("new apply must not store a password hash: %q", app.PasswordHash)
	}

	again := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/apply", token, "")
	if sourceBodyCode(t, again) != 400 || !strings.Contains(again.Body.String(), "审核") {
		t.Fatalf("duplicate pending apply=%s", again.Body.String())
	}

	status := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/apply/status", token, "")
	if sourceBodyCode(t, status) != 200 || !strings.Contains(status.Body.String(), `"pending"`) {
		t.Fatalf("status=%s", status.Body.String())
	}
	if strings.Contains(status.Body.String(), `"username":"legacy"`) {
		t.Fatalf("status must be for current agent, not a username query: %s", status.Body.String())
	}
}

func TestSourceDeveloperApprovedAgentTokenAccessesDeveloperAPIs(t *testing.T) {
	router, store := sourceStationRouter(t)
	_, agentToken, _ := sourceApproveDeveloper(t, router, "approved-agent", "")

	me := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/me", agentToken, "")
	if sourceBodyCode(t, me) != 200 || !strings.Contains(me.Body.String(), sourceDeveloperRoleCode) {
		t.Fatalf("approved agent /me=%s", me.Body.String())
	}

	items := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/items", agentToken, "")
	if sourceBodyCode(t, items) != 200 {
		t.Fatalf("approved agent /items=%s", items.Body.String())
	}

	dev, err := store.GetDeveloperByAgentID(int64(parseAgentIDFromToken(t, agentToken)))
	if err != nil || !dev.Enabled || dev.PasswordHash != "" {
		t.Fatalf("developer binding=%+v err=%v", dev, err)
	}
	listed, err := store.ListDevelopers()
	if err != nil || len(listed) != 1 || listed[0].AgentID != dev.AgentID {
		t.Fatalf("list developers=%+v err=%v", listed, err)
	}
}

func TestSourceDeveloperUnapprovedAgentCannotAccessDeveloperAPIs(t *testing.T) {
	router, _ := sourceStationRouter(t)
	_, _, token := sourceNextAgent(t, "no-apply")
	me := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/me", token, "")
	if sourceBodyCode(t, me) != 403 {
		t.Fatalf("unapproved agent /me=%s", me.Body.String())
	}

	pendingToken := func() string {
		_, _, tok := sourceNextAgent(t, "pending-only")
		apply := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/apply", tok, `{}`)
		if sourceBodyCode(t, apply) != 200 {
			t.Fatalf("pending apply=%s", apply.Body.String())
		}
		return tok
	}()
	pendingMe := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/me", pendingToken, "")
	if sourceBodyCode(t, pendingMe) != 403 {
		t.Fatalf("pending agent /me=%s", pendingMe.Body.String())
	}
}

func TestSourceDeveloperRejectThenReapply(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	_, _, token := sourceNextAgent(t, "reapply-agent")
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
		admin, `{"note":"incomplete"}`)
	if sourceBodyCode(t, reject) != 200 {
		t.Fatalf("reject=%s", reject.Body.String())
	}
	listed := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/applications", admin, "")
	if sourceContainsID(sourceListIDs(t, listed), applyBody.Data.ID) {
		t.Fatalf("rejected application still listed: %s", listed.Body.String())
	}
	status := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/apply/status", token, "")
	if sourceBodyCode(t, status) != 404 {
		t.Fatalf("rejected status=%s", status.Body.String())
	}
	again := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/apply", token, `{}`)
	if sourceBodyCode(t, again) != 200 || !strings.Contains(again.Body.String(), `"pending"`) {
		t.Fatalf("reapply after reject=%s", again.Body.String())
	}
}

func TestSourceDeveloperCancelThenReapplyAndApprove(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin, token, developerID := sourceApproveDeveloper(t, router, "cancel-reapply", "")
	cancel := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/developers/"+itoa64(developerID)+"/freeze", admin, `{}`)
	if sourceBodyCode(t, cancel) != 200 || !strings.Contains(cancel.Body.String(), "已取消并删除开发者资格") {
		t.Fatalf("cancel=%s", cancel.Body.String())
	}
	devs := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/developers", admin, "")
	if sourceContainsID(sourceListIDs(t, devs), developerID) {
		t.Fatalf("cancelled developer still listed: %s", devs.Body.String())
	}
	me := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/me", token, "")
	if sourceBodyCode(t, me) != 403 {
		t.Fatalf("cancelled /me=%s", me.Body.String())
	}
	again := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/apply", token, `{}`)
	if sourceBodyCode(t, again) != 200 || !strings.Contains(again.Body.String(), `"pending"`) {
		t.Fatalf("reapply after cancel=%s", again.Body.String())
	}
	var body struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(again.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	approve := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/applications/"+itoa64(body.Data.ID)+"/approve", admin, `{}`)
	if sourceBodyCode(t, approve) != 200 {
		t.Fatalf("re-approve=%s", approve.Body.String())
	}
	ok := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/me", token, "")
	if sourceBodyCode(t, ok) != 200 {
		t.Fatalf("re-approved /me=%s", ok.Body.String())
	}
}

func TestSourceDeveloperApproveDeletesApplication(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	_, _, token := sourceNextAgent(t, "approve-deletes-app")
	apply := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/apply", token, `{}`)
	var applyBody struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(apply.Body.Bytes(), &applyBody); err != nil {
		t.Fatal(err)
	}
	pending := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/applications", admin, "")
	if !sourceContainsID(sourceListIDs(t, pending), applyBody.Data.ID) {
		t.Fatalf("pending application missing: %s", pending.Body.String())
	}
	approve := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/applications/"+itoa64(applyBody.Data.ID)+"/approve", admin, `{}`)
	if sourceBodyCode(t, approve) != 200 {
		t.Fatalf("approve=%s", approve.Body.String())
	}
	listed := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/applications", admin, "")
	if sourceContainsID(sourceListIDs(t, listed), applyBody.Data.ID) {
		t.Fatalf("approved application still listed: %s", listed.Body.String())
	}
	approved := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/applications?status=approved", admin, "")
	if sourceBodyCode(t, approved) != 400 {
		t.Fatalf("non-pending applications list=%s", approved.Body.String())
	}
}

func TestSourceDeveloperCancelDeletesLeftoverApplication(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin, token, developerID := sourceApproveDeveloper(t, router, "cancel-leftover", "")
	agentID := int64(parseAgentIDFromToken(t, token))
	store.mu.Lock()
	store.applications[9001] = sourceApplication{
		ID: 9001, AgentID: agentID, Username: "cancel-leftover@agents.test",
		Status: sourceApplicationApproved, CreatedAt: time.Now().UTC(),
	}
	store.mu.Unlock()
	cancel := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/developers/"+itoa64(developerID)+"/freeze", admin, `{}`)
	if sourceBodyCode(t, cancel) != 200 {
		t.Fatalf("cancel=%s", cancel.Body.String())
	}
	if _, err := store.GetApplication(9001); !errors.Is(err, errSourceNotFound) {
		t.Fatalf("leftover application after cancel: err=%v", err)
	}
	if _, err := store.GetDeveloperByID(developerID); !errors.Is(err, errSourceNotFound) {
		t.Fatalf("developer still exists after cancel: err=%v", err)
	}
	devs := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/developers", admin, "")
	if sourceContainsID(sourceListIDs(t, devs), developerID) {
		t.Fatalf("cancelled developer still listed: %s", devs.Body.String())
	}
	apps := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/applications", admin, "")
	if sourceContainsID(sourceListIDs(t, apps), 9001) {
		t.Fatalf("cancelled leftover application still listed: %s", apps.Body.String())
	}
	status := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/apply/status", token, "")
	if sourceBodyCode(t, status) != 404 {
		t.Fatalf("cancelled apply status still visible: %s", status.Body.String())
	}
}

func TestSourceDeveloperListOmitsDisabledGhosts(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	store.mu.Lock()
	store.developers[8801] = sourceDeveloper{
		ID: 8801, Username: "zxcv25", Email: "37710566@qq.com",
		DisplayName: "ghost", Enabled: false, CreatedAt: time.Now().UTC(),
	}
	store.developers[8802] = sourceDeveloper{
		ID: 8802, Username: "active-dev", Email: "active@test.com",
		DisplayName: "active", Enabled: true, CreatedAt: time.Now().UTC(),
	}
	store.mu.Unlock()

	listed, err := store.ListDevelopers()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != 8802 || !listed[0].Enabled {
		t.Fatalf("ListDevelopers should return only enabled developers: %+v", listed)
	}

	devs := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/developers", admin, "")
	ids := sourceListIDs(t, devs)
	if sourceContainsID(ids, 8801) {
		t.Fatalf("disabled developer ghost listed: %s", devs.Body.String())
	}
	if !sourceContainsID(ids, 8802) {
		t.Fatalf("enabled developer missing: %s", devs.Body.String())
	}
}

func TestMysqlPurgeTargetsRealDeveloperTables(t *testing.T) {
	if strings.Contains(mysqlPurgeInactiveApplicationsSQL, "source_applications") {
		t.Fatal("purge must not target nonexistent source_applications")
	}
	if !strings.Contains(mysqlPurgeInactiveApplicationsSQL, "source_developer_applications") {
		t.Fatalf("purge must delete from source_developer_applications: %s", mysqlPurgeInactiveApplicationsSQL)
	}
	for _, status := range []string{"frozen", "cancelled", "approved", "rejected"} {
		if !strings.Contains(mysqlPurgeInactiveApplicationsSQL, "'"+status+"'") {
			t.Fatalf("purge must include status %q: %s", status, mysqlPurgeInactiveApplicationsSQL)
		}
	}
	if !strings.Contains(mysqlPurgeDisabledDevelopersSQL, "source_developers") ||
		!strings.Contains(mysqlPurgeDisabledDevelopersSQL, "enabled=0") {
		t.Fatalf("purge must delete enabled=0 rows from source_developers: %s", mysqlPurgeDisabledDevelopersSQL)
	}
}

func TestSourceDeveloperCleanupRemovesFrozenLeftovers(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	agentID, email, token := sourceNextAgent(t, "zxcv25")
	createdAt := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	store.mu.Lock()
	store.applications[9901] = sourceApplication{
		ID: 9901, AgentID: int64(agentID), Username: email, Email: "37710566@qq.com",
		Status: sourceApplicationFrozen, ReviewNote: "管理员取消开发者资格", CreatedAt: createdAt,
	}
	store.applications[9903] = sourceApplication{
		ID: 9903, AgentID: int64(agentID) + 1, Username: "keep-pending@agents.test",
		Status: sourceApplicationPending, CreatedAt: createdAt,
	}
	store.developers[9902] = sourceDeveloper{
		ID: 9902, AgentID: int64(agentID), Username: email, Email: "37710566@qq.com",
		Enabled: false, CreatedAt: createdAt,
	}
	store.developers[9904] = sourceDeveloper{
		ID: 9904, Username: "keep-enabled", Email: "keep@test.com",
		Enabled: true, CreatedAt: createdAt,
	}
	store.mu.Unlock()

	if err := store.purgeInactiveDeveloperQualifications(); err != nil {
		t.Fatal(err)
	}

	if _, err := store.GetApplication(9901); !errors.Is(err, errSourceNotFound) {
		t.Fatalf("frozen leftover application still present: err=%v", err)
	}
	if _, err := store.GetDeveloperByID(9902); !errors.Is(err, errSourceNotFound) {
		t.Fatalf("disabled leftover developer still present: err=%v", err)
	}
	if _, err := store.GetApplication(9903); err != nil {
		t.Fatalf("pending application must survive cleanup: err=%v", err)
	}
	if _, err := store.GetDeveloperByID(9904); err != nil {
		t.Fatalf("enabled developer must survive cleanup: err=%v", err)
	}

	apps := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/applications", admin, "")
	if sourceContainsID(sourceListIDs(t, apps), 9901) {
		t.Fatalf("frozen leftover still listed: %s", apps.Body.String())
	}
	if !sourceContainsID(sourceListIDs(t, apps), 9903) {
		t.Fatalf("pending application missing after cleanup: %s", apps.Body.String())
	}
	devs := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/developers", admin, "")
	if sourceContainsID(sourceListIDs(t, devs), 9902) {
		t.Fatalf("disabled leftover still listed: %s", devs.Body.String())
	}
	if !sourceContainsID(sourceListIDs(t, devs), 9904) {
		t.Fatalf("enabled developer missing after cleanup: %s", devs.Body.String())
	}
	status := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/apply/status", token, "")
	if sourceBodyCode(t, status) != 404 {
		t.Fatalf("frozen leftover still visible on apply status: %s", status.Body.String())
	}
}

func parseAgentIDFromToken(t *testing.T, token string) uint {
	t.Helper()
	claims := &middleware.Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(tkn *jwt.Token) (interface{}, error) {
		return middleware.JWTSecret(), nil
	})
	if err != nil || parsed == nil || !parsed.Valid {
		t.Fatalf("parse agent token: %v", err)
	}
	return claims.UserID
}
