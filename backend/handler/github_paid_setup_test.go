package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type paidGitHubFake struct {
	mu           sync.Mutex
	login        string
	orgs         []string
	repos        map[string]bool
	forbidCreate bool
	unauthorized bool
	createCalls  []string
	createBodies []map[string]any
}

func (f *paidGitHubFake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if f.unauthorized {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"bad credentials"}`))
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/user":
		_ = json.NewEncoder(w).Encode(map[string]string{"login": f.login})
	case r.Method == http.MethodGet && r.URL.Path == "/user/orgs":
		orgs := make([]map[string]string, 0, len(f.orgs))
		for _, org := range f.orgs {
			orgs = append(orgs, map[string]string{"login": org})
		}
		_ = json.NewEncoder(w).Encode(orgs)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/repos/"):
		key := strings.TrimPrefix(r.URL.Path, "/repos/")
		private, ok := f.repos[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"private": private})
	case r.Method == http.MethodPost && (r.URL.Path == "/user/repos" || (strings.HasPrefix(r.URL.Path, "/orgs/") && strings.HasSuffix(r.URL.Path, "/repos"))):
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.createCalls = append(f.createCalls, r.URL.Path)
		f.createBodies = append(f.createBodies, body)
		if f.forbidCreate {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
			return
		}
		name, _ := body["name"].(string)
		owner := f.login
		if strings.HasPrefix(r.URL.Path, "/orgs/") {
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			if len(parts) >= 2 {
				owner = parts[1]
			}
		}
		if f.repos == nil {
			f.repos = map[string]bool{}
		}
		private, _ := body["private"].(bool)
		f.repos[owner+"/"+name] = private
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"private": private, "name": name})
	default:
		http.NotFound(w, r)
	}
}

func usePaidGitHubFake(t *testing.T, fake *paidGitHubFake) (*httptest.ResponseRecorder, func(method, path, body string) *httptest.ResponseRecorder) {
	t.Helper()
	if fake.repos == nil {
		fake.repos = map[string]bool{}
	}
	api := httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(api.Close)
	previous := sourceGitHubAPIBase
	sourceGitHubAPIBase = api.URL
	t.Cleanup(func() { sourceGitHubAPIBase = previous })
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		return sourceJSON(t, router, method, path, admin, body)
	}
	return nil, call
}

func useGitHubPaidPrivateRepoStub(t *testing.T, login string) {
	t.Helper()
	fake := &paidGitHubFake{login: login, repos: map[string]bool{}}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/repos/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"private":true}`))
			return
		}
		fake.serve(w, r)
	}))
	t.Cleanup(api.Close)
	previous := sourceGitHubAPIBase
	sourceGitHubAPIBase = api.URL
	t.Cleanup(func() { sourceGitHubAPIBase = previous })
}

func TestGitHubPaidRepoClosedLoop(t *testing.T) {
	const pat = "github_pat_closed_loop"
	const base = "/api/v1/source/admin/settings/github-paid"

	t.Run("test fills owner orgs and default repo", func(t *testing.T) {
		t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
		fake := &paidGitHubFake{login: "octocat", orgs: []string{"acme"}}
		_, call := usePaidGitHubFake(t, fake)
		got := call(http.MethodPost, base+"/test", `{"token":"`+pat+`","repo":"paid-plugins"}`)
		body := got.Body.String()
		for _, want := range []string{`"login":"octocat"`, `"owner":"octocat"`, `"repo":"paid-plugins"`, `"connected":false`, `"repoStatus":"missing"`, `"kind":"org"`, `"login":"acme"`, githubPaidRepoMissingText} {
			if !strings.Contains(body, want) {
				t.Fatalf("missing %s in %s", want, body)
			}
		}
		if strings.Contains(body, "settings/tokens") {
			t.Fatalf("token url leaked: %s", body)
		}
		if len(fake.createCalls) != 0 {
			t.Fatalf("test created a repo: %v", fake.createCalls)
		}
	})

	t.Run("create private repo for user", func(t *testing.T) {
		t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
		fake := &paidGitHubFake{login: "octocat"}
		_, call := usePaidGitHubFake(t, fake)
		got := call(http.MethodPost, base+"/repo", `{"token":"`+pat+`","repo":"paid-plugins"}`)
		body := got.Body.String()
		if sourceBodyCode(t, got) != 200 || !strings.Contains(body, "已连接：octocat/paid-plugins（私有）") || !strings.Contains(body, `"connected":true`) || strings.Contains(body, pat) {
			t.Fatalf("create: %s", body)
		}
		if len(fake.createCalls) != 1 || fake.createCalls[0] != "/user/repos" {
			t.Fatalf("calls=%v", fake.createCalls)
		}
		created := fake.createBodies[0]
		if created["name"] != "paid-plugins" || created["private"] != true || created["auto_init"] != true {
			t.Fatalf("body=%v", created)
		}
	})

	t.Run("create private repo for org", func(t *testing.T) {
		t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
		fake := &paidGitHubFake{login: "octocat", orgs: []string{"acme"}}
		_, call := usePaidGitHubFake(t, fake)
		got := call(http.MethodPost, base+"/repo", `{"token":"`+pat+`","owner":"acme","repo":"auth-pro-paid"}`)
		body := got.Body.String()
		if sourceBodyCode(t, got) != 200 || !strings.Contains(body, "已连接：acme/auth-pro-paid（私有）") {
			t.Fatalf("org create: %s", body)
		}
		if len(fake.createCalls) != 1 || fake.createCalls[0] != "/orgs/acme/repos" {
			t.Fatalf("calls=%v", fake.createCalls)
		}
	})

	t.Run("existing private repo needs no create permission", func(t *testing.T) {
		t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
		fake := &paidGitHubFake{
			login:        "octocat",
			forbidCreate: true,
			repos:        map[string]bool{"octocat/auth-pro-paid": true},
		}
		_, call := usePaidGitHubFake(t, fake)
		for _, item := range []struct{ method, path, payload string }{
			{http.MethodPost, base + "/test", `{"token":"` + pat + `","repo":"auth-pro-paid"}`},
			{http.MethodPost, base + "/repo", `{"token":"` + pat + `","repo":"auth-pro-paid"}`},
			{http.MethodPut, base, `{"token":"` + pat + `","repo":"auth-pro-paid"}`},
		} {
			got := call(item.method, item.path, item.payload)
			body := got.Body.String()
			if sourceBodyCode(t, got) != 200 || !strings.Contains(body, "已连接：octocat/auth-pro-paid（私有）") && item.method != http.MethodPut {
				t.Fatalf("%s %s: %s", item.method, item.path, body)
			}
			if item.method == http.MethodPut && (sourceBodyCode(t, got) != 200 || !strings.Contains(body, `"configured":true`) || strings.Contains(body, pat)) {
				t.Fatalf("save existing: %s", body)
			}
		}
		if len(fake.createCalls) != 0 {
			t.Fatalf("create was called without permission: %v", fake.createCalls)
		}
	})

	t.Run("public repo is refused", func(t *testing.T) {
		t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
		fake := &paidGitHubFake{login: "octocat", repos: map[string]bool{"octocat/auth-pro-paid": false}}
		_, call := usePaidGitHubFake(t, fake)
		for _, item := range []struct{ method, path string }{
			{http.MethodPost, base + "/test"},
			{http.MethodPost, base + "/repo"},
			{http.MethodPut, base},
		} {
			got := call(item.method, item.path, `{"token":"`+pat+`","repo":"auth-pro-paid"}`)
			body := got.Body.String()
			if sourceBodyCode(t, got) != 400 || !strings.Contains(body, "公开") || strings.Contains(body, pat) {
				t.Fatalf("%s %s: %s", item.method, item.path, body)
			}
		}
		if len(fake.createCalls) != 0 {
			t.Fatalf("public repo was created again: %v", fake.createCalls)
		}
		settings := call(http.MethodGet, base, "")
		if sourceBodyCode(t, settings) != 200 || !strings.Contains(settings.Body.String(), `"configured":false`) {
			t.Fatalf("public save persisted: %s", settings.Body.String())
		}
	})

	t.Run("create forbidden explains token permissions", func(t *testing.T) {
		t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
		fake := &paidGitHubFake{login: "octocat", forbidCreate: true}
		_, call := usePaidGitHubFake(t, fake)
		got := call(http.MethodPost, base+"/repo", `{"token":"`+pat+`","repo":"paid-plugins"}`)
		body := got.Body.String()
		if sourceBodyCode(t, got) != 400 || strings.Contains(body, pat) {
			t.Fatalf("forbidden: %s", body)
		}
		for _, want := range []string{"Administration", "Contents", "repo"} {
			if !strings.Contains(body, want) {
				t.Fatalf("missing %s in %s", want, body)
			}
		}
		var payload struct {
			Msg  string `json:"msg"`
			Data struct {
				TokenCreateURL string `json:"tokenCreateUrl"`
			} `json:"data"`
		}
		if err := json.Unmarshal(got.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(payload.Msg, "settings/tokens") || payload.Data.TokenCreateURL != "" {
			t.Fatalf("token url leaked msg=%s", payload.Msg)
		}
		if len(fake.createCalls) != 1 || fake.createCalls[0] != "/user/repos" {
			t.Fatalf("calls=%v", fake.createCalls)
		}
	})

	t.Run("save creates missing repo", func(t *testing.T) {
		t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
		fake := &paidGitHubFake{login: "octocat", orgs: []string{"acme"}}
		_, call := usePaidGitHubFake(t, fake)
		got := call(http.MethodPut, base, `{"token":"`+pat+`","repo":"paid-plugins"}`)
		body := got.Body.String()
		if sourceBodyCode(t, got) != 200 || !strings.Contains(body, `"owner":"octocat"`) || !strings.Contains(body, `"repo":"paid-plugins"`) || !strings.Contains(body, `"configured":true`) || !strings.Contains(body, `"connected":true`) || strings.Contains(body, pat) {
			t.Fatalf("save create: %s", body)
		}
		if len(fake.createCalls) != 1 || fake.createCalls[0] != "/user/repos" {
			t.Fatalf("calls=%v", fake.createCalls)
		}
		created := fake.createBodies[0]
		if created["private"] != true || created["auto_init"] != true || created["name"] != "paid-plugins" {
			t.Fatalf("body=%v", created)
		}
		plain, err := loadGitHubPaidToken()
		if err != nil || plain != pat {
			t.Fatalf("stored token=%q err=%v", plain, err)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
		fake := &paidGitHubFake{login: "octocat", unauthorized: true}
		_, call := usePaidGitHubFake(t, fake)
		got := call(http.MethodPost, base+"/test", `{"token":"`+pat+`"}`)
		body := got.Body.String()
		if sourceBodyCode(t, got) != 400 || !strings.Contains(body, "令牌无效") || strings.Contains(body, pat) {
			t.Fatalf("invalid: %s", body)
		}
	})
}
