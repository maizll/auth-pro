package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSourceReleaseSettingsTestRequiresCompleteFields(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/settings/release/test", admin,
		`{"provider":"github","owner":"","repo":"","token":""}`)
	if sourceBodyCode(t, rec) != 400 {
		t.Fatalf("incomplete must 400, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "请填写完整的 Provider / Owner / 仓库 / Token") {
		t.Fatalf("want ready hint, got %s", rec.Body.String())
	}
}

func TestSourceReleaseSettingsTestGitHubSuccess(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			w.WriteHeader(http.StatusCreated)
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/repos/acme/pkgs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer ghs_test_token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"message":"Bad credentials"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"full_name": "acme/pkgs",
			"html_url":  "https://github.com/acme/pkgs",
			"private":   true,
			"permissions": map[string]any{
				"admin": true, "push": true, "pull": true,
			},
		})
	}))
	t.Cleanup(server.Close)
	prev := sourceGitHubAPIBase
	sourceGitHubAPIBase = server.URL
	t.Cleanup(func() { sourceGitHubAPIBase = prev })

	rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/settings/release/test", admin,
		`{"provider":"github","owner":"acme","repo":"pkgs","token":"ghs_test_token"}`)
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("want 200, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "连接成功") {
		t.Fatalf("want 连接成功, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"fullName":"acme/pkgs"`) || !strings.Contains(rec.Body.String(), "https://github.com/acme/pkgs") {
		t.Fatalf("want repo metadata, got %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "ghs_test_token") {
		t.Fatalf("response leaked token: %s", rec.Body.String())
	}
	if posts != 0 {
		t.Fatalf("test must not create Release, posts=%d", posts)
	}
}

func TestSourceReleaseSettingsTestUsesStoredTokenAndDoesNotPersist(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	save := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/release", admin,
		`{"provider":"github","owner":"acme","repo":"pkgs","token":"ghs_stored_token","tagStrategy":"{id}-{version}","branch":"main"}`)
	if sourceBodyCode(t, save) != 200 {
		t.Fatalf("save=%s", save.Body.String())
	}
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("Authorization") != "Bearer ghs_stored_token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"message":"Bad credentials"}`)
			return
		}
		if r.URL.Path != "/repos/acme/other" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"full_name": "acme/other",
			"html_url":  "https://github.com/acme/other",
		})
	}))
	t.Cleanup(server.Close)
	prev := sourceGitHubAPIBase
	sourceGitHubAPIBase = server.URL
	t.Cleanup(func() { sourceGitHubAPIBase = prev })

	rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/settings/release/test", admin,
		`{"provider":"github","owner":"acme","repo":"other","token":"****oken"}`)
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("masked token must reuse stored, got %s", rec.Body.String())
	}
	if hits == 0 {
		t.Fatal("expected GitHub GET")
	}
	kept, err := store.GetReleaseSettings()
	if err != nil {
		t.Fatal(err)
	}
	if kept.Repo != "pkgs" || kept.Token != "ghs_stored_token" {
		t.Fatalf("test must not persist settings: %#v", kept)
	}
}

func TestSourceReleaseSettingsTestMapsGitHubStatusHints(t *testing.T) {
	cases := []struct {
		status int
		body   string
		hint   string
	}{
		{http.StatusUnauthorized, `{"message":"Bad credentials"}`, "令牌无效"},
		{http.StatusForbidden, `{"message":"Resource not accessible"}`, "权限不足"},
		{http.StatusNotFound, `{"message":"Not Found"}`, "仓库不存在或无权访问"},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			router, _ := sourceStationRouter(t)
			admin := sourceAdminToken(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(server.Close)
			prev := sourceGitHubAPIBase
			sourceGitHubAPIBase = server.URL
			t.Cleanup(func() { sourceGitHubAPIBase = prev })
			rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/settings/release/test", admin,
				`{"provider":"github","owner":"acme","repo":"pkgs","token":"ghs_bad"}`)
			if sourceBodyCode(t, rec) != 400 {
				t.Fatalf("want 400, got %s", rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.hint) {
				t.Fatalf("want hint %q, got %s", tc.hint, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "HTTP") {
				t.Fatalf("want HTTP status in msg, got %s", rec.Body.String())
			}
		})
	}
}

func TestSourceReleaseSettingsTestUpstreamErrorIs502(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"message":"boom"}`)
	}))
	t.Cleanup(server.Close)
	prev := sourceGitHubAPIBase
	sourceGitHubAPIBase = server.URL
	t.Cleanup(func() { sourceGitHubAPIBase = prev })
	rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/settings/release/test", admin,
		`{"provider":"github","owner":"acme","repo":"pkgs","token":"ghs_test"}`)
	if sourceBodyCode(t, rec) != 502 {
		t.Fatalf("want 502, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "HTTP 500") {
		t.Fatalf("want HTTP 500 in msg, got %s", rec.Body.String())
	}
}

func TestSourceReleaseSettingsTestGiteeSuccess(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			w.WriteHeader(http.StatusCreated)
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/repos/acme/pkgs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("access_token") != "gitee_token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"message":"401 Unauthorized"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"full_name": "acme/pkgs",
			"html_url":  "https://gitee.com/acme/pkgs",
			"permission": map[string]any{
				"admin": true, "push": true, "pull": true,
			},
		})
	}))
	t.Cleanup(server.Close)
	prev := sourceGiteeAPIBase
	sourceGiteeAPIBase = server.URL
	t.Cleanup(func() { sourceGiteeAPIBase = prev })

	rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/settings/release/test", admin,
		`{"provider":"gitee","owner":"acme","repo":"pkgs","token":"gitee_token","branch":"master"}`)
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("want 200, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "连接成功") || !strings.Contains(rec.Body.String(), "https://gitee.com/acme/pkgs") {
		t.Fatalf("want gitee success, got %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "gitee_token") {
		t.Fatalf("response leaked token: %s", rec.Body.String())
	}
	if posts != 0 {
		t.Fatalf("test must not create Release, posts=%d", posts)
	}
}
