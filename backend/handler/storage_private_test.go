package handler

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"auto_pro/middleware"

	"github.com/golang-jwt/jwt/v5"
)

// fakeRepoHost 模拟代码托管站：记住仓库可见性，按令牌决定能不能改。
type fakeRepoHost struct {
	mu        sync.Mutex
	private   map[string]bool
	patchCode int    // PATCH 返回的状态码，0 表示正常改
	scopes    string // 经典令牌的 X-OAuth-Scopes
	patched   []string
}

func newFakeRepoHost(t *testing.T, repos map[string]bool) (*fakeRepoHost, *httptest.Server) {
	t.Helper()
	host := &fakeRepoHost{private: repos}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host.mu.Lock()
		defer host.mu.Unlock()
		path := strings.TrimPrefix(r.URL.Path, "/api/v5")
		if path == "/user" {
			if host.scopes != "" {
				w.Header().Set("X-OAuth-Scopes", host.scopes)
			}
			_, _ = w.Write([]byte(`{"login":"acme"}`))
			return
		}
		full := strings.TrimPrefix(path, "/repos/")
		private, ok := host.private[full]
		if !ok {
			http.NotFound(w, r)
			return
		}
		anonymous := r.Header.Get("Authorization") == "" && r.URL.Query().Get("access_token") == ""
		switch r.Method {
		case http.MethodGet:
			if anonymous && private {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"private": private, "permissions": map[string]bool{"push": true}, "permission": map[string]bool{"push": true}})
		case http.MethodPatch:
			if host.patchCode != 0 {
				w.WriteHeader(host.patchCode)
				_, _ = w.Write([]byte(`{"message":"Resource not accessible by personal access token"}`))
				return
			}
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			if body["private"] != true {
				w.WriteHeader(http.StatusUnprocessableEntity)
				return
			}
			host.private[full] = true
			host.patched = append(host.patched, full)
			_ = json.NewEncoder(w).Encode(map[string]any{"private": true})
		}
	}))
	t.Cleanup(server.Close)
	return host, server
}

func sourceOpsToken(t *testing.T) string {
	t.Helper()
	claims := middleware.Claims{
		UserID: 2, Username: "ops", Role: "admin", RoleCode: "R_ADMIN",
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(middleware.JWTSecret())
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func saveGitStorage(t *testing.T, kind, owner, repo, token string) {
	t.Helper()
	sealed, err := sealStorageSecret(token)
	if err != nil {
		t.Fatal(err)
	}
	loc := storageLocation{ID: newStorageID(), Name: "安装包仓库", Kind: kind, Role: storageRolePrimary, Owner: owner, Repo: repo, SecretSealed: sealed}
	if err := saveStorageBlob(storageConfigBlob{Locations: []storageLocation{loc}}); err != nil {
		t.Fatal(err)
	}
}

func findMakePrivateRow(rows []storageHealthRow) (storageHealthRow, bool) {
	for _, row := range rows {
		if row.Action == storageActionMakePrivate {
			return row, true
		}
	}
	return storageHealthRow{}, false
}

// 公开的 GitHub 存储仓库：检查给出按钮；要超级管理员、要输对仓库名；改完自动重新检查，按钮消失。
func TestStorageMakePrivateGitHub(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	router, _ := sourceStationRouter(t)
	host, server := newFakeRepoHost(t, map[string]bool{"acme/packages": false})
	previous := sourceGitHubAPIBase
	sourceGitHubAPIBase = server.URL
	t.Cleanup(func() { sourceGitHubAPIBase = previous })
	saveGitStorage(t, packageStorageGitHub, "acme", "packages", "ghp_storage_token_0123456789")

	checkStorageLocations(context.Background())
	blob, _ := loadStorageBlob()
	row, ok := findMakePrivateRow(blob.Health)
	if !ok || row.Kind != packageStorageGitHub || row.Owner != "acme" || row.Repo != "packages" || row.Official || row.Level != "problem" {
		t.Fatalf("公开仓库应带改为私有按钮: %+v", blob.Health)
	}

	body := `{"kind":"github","owner":"acme","repo":"packages","confirm":"acme/packages"}`
	state := newAdminSessionState(t)
	state.admins[2] = sessionAdmin{id: 2, username: "ops", roleID: 6, roleCode: "R_ADMIN", enabled: true}
	useAdminSessionDB(t, state)
	if got := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/storage/make-private", sourceOpsToken(t), body); sourceBodyCode(t, got) != 403 {
		t.Fatalf("普通管理员不能改: %s", got.Body.String())
	}
	check := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/storage/make-private/check", sourceAdminToken(t), body)
	if !strings.Contains(check.Body.String(), `"allowed":true`) || !strings.Contains(check.Body.String(), "存储「安装包仓库」的令牌") || strings.Contains(check.Body.String(), "ghp_storage") {
		t.Fatalf("检查结果不对: %s", check.Body.String())
	}
	wrong := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/storage/make-private", sourceAdminToken(t), `{"kind":"github","owner":"acme","repo":"packages","confirm":"packages"}`)
	if sourceBodyCode(t, wrong) != 400 || len(host.patched) != 0 {
		t.Fatalf("仓库名不对不能改: %s", wrong.Body.String())
	}

	host.patchCode = http.StatusForbidden
	denied := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/storage/make-private", sourceAdminToken(t), body)
	if sourceBodyCode(t, denied) != 400 || !strings.Contains(denied.Body.String(), "Administration 读写") {
		t.Fatalf("权限不足要说清缺什么: %s", denied.Body.String())
	}

	host.patchCode = 0
	done := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/storage/make-private", sourceAdminToken(t), body)
	if sourceBodyCode(t, done) != 200 || len(host.patched) != 1 {
		t.Fatalf("应改为私有: %s", done.Body.String())
	}
	if strings.Contains(done.Body.String(), storageActionMakePrivate) || !strings.Contains(done.Body.String(), "可以连接") {
		t.Fatalf("改完应自动重新检查，按钮消失: %s", done.Body.String())
	}
}

// Gitee 仓库同样可以一键改为私有。
func TestStorageMakePrivateGitee(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	router, _ := sourceStationRouter(t)
	host, server := newFakeRepoHost(t, map[string]bool{"acme/packages": false})
	previous := sourceGiteeAPIBase
	sourceGiteeAPIBase = server.URL + "/api/v5"
	t.Cleanup(func() { sourceGiteeAPIBase = previous })
	saveGitStorage(t, packageStorageGitee, "acme", "packages", "gitee_storage_token_0123456789")

	checkStorageLocations(context.Background())
	blob, _ := loadStorageBlob()
	if row, ok := findMakePrivateRow(blob.Health); !ok || row.Kind != packageStorageGitee {
		t.Fatalf("公开的 Gitee 仓库应带改为私有按钮: %+v", blob.Health)
	}
	body := `{"kind":"gitee","owner":"acme","repo":"packages","confirm":"acme/packages"}`
	done := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/storage/make-private", sourceAdminToken(t), body)
	if sourceBodyCode(t, done) != 200 || len(host.patched) != 1 || strings.Contains(done.Body.String(), storageActionMakePrivate) {
		t.Fatalf("Gitee 应改为私有并重新检查: %s", done.Body.String())
	}
}

// 官网自更新来源仓库：检查行只是灰色提醒；不允许一键改，只给文档步骤和令牌能否读取的结论。
func TestStorageMakePrivateRefusesOfficialRepo(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	router, _ := sourceStationRouter(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	previousKey := embeddedStoreSnapshotPublicKey
	embeddedStoreSnapshotPublicKey = base64.StdEncoding.EncodeToString(publicKey)
	t.Cleanup(func() { embeddedStoreSnapshotPublicKey = previousKey })
	writeSnapshotKey(t, privateKey)
	t.Setenv(officialUpdateRepoEnv, "")

	host, server := newFakeRepoHost(t, map[string]bool{"maizll/auth-pro": false, "acme/packages": true})
	previousAPI, previousSource := productUpdateGitHubAPI, sourceGitHubAPIBase
	productUpdateGitHubAPI, sourceGitHubAPIBase = server.URL, server.URL
	t.Cleanup(func() { productUpdateGitHubAPI, sourceGitHubAPIBase = previousAPI, previousSource })
	saveGitStorage(t, packageStorageGitHub, "acme", "packages", "ghp_storage_token_0123456789")

	checkStorageLocations(context.Background())
	blob, _ := loadStorageBlob()
	row, ok := findMakePrivateRow(blob.Health)
	if !ok || !row.Official || row.Level != "notice" || row.Owner != "maizll" || row.Repo != "auth-pro" {
		t.Fatalf("官网更新来源公开时应是灰色提醒: %+v", blob.Health)
	}

	body := `{"kind":"github","owner":"maizll","repo":"auth-pro","confirm":"maizll/auth-pro"}`
	host.scopes = "repo, workflow"
	check := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/storage/make-private/check", sourceAdminToken(t), body)
	text := check.Body.String()
	if !strings.Contains(text, `"allowed":false`) || !strings.Contains(text, "update-distribution.md") || !strings.Contains(text, `"state":"yes"`) || !strings.Contains(text, `"readable":true`) {
		t.Fatalf("官网仓库应只给步骤，并认出经典令牌能读: %s", text)
	}
	host.scopes = "workflow"
	if text := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/storage/make-private/check", sourceAdminToken(t), body).Body.String(); !strings.Contains(text, `"readable":false`) || !strings.Contains(text, "勾选 repo") {
		t.Fatalf("经典令牌缺 repo 时要说明: %s", text)
	}
	refused := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/storage/make-private", sourceAdminToken(t), body)
	if sourceBodyCode(t, refused) != 400 || len(host.patched) != 0 {
		t.Fatalf("官网仓库不能一键改: %s", refused.Body.String())
	}

	// 改私有以后：存储令牌能读就写明用哪个；都读不到时报问题。
	host.private["maizll/auth-pro"] = true
	checkStorageLocations(context.Background())
	blob, _ = loadStorageBlob()
	found := false
	for _, item := range blob.Health {
		if item.Target == "官网更新来源" {
			found = item.Level == "ok" && strings.Contains(item.Message, "存储「安装包仓库」的令牌")
		}
	}
	if !found {
		t.Fatalf("私有后应写明能读的令牌: %+v", blob.Health)
	}
}
