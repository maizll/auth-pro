package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListGitHubReposPaginatesSortsAndCaches(t *testing.T) {
	accessibleGitHubRepoCache = gitHubRepoCache{}
	var pages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/user/repos") {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer ghp_list" {
			t.Errorf("authorization = %s", r.Header.Get("Authorization"))
		}
		pages = append(pages, r.URL.Query().Get("page"))
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"full_name": "acme/newer", "private": false, "updated_at": "2026-09-30T08:00:00Z"},
			})
			return
		}
		first := make([]map[string]any, gitHubRepoListPageSize)
		for i := range first {
			first[i] = map[string]any{
				"full_name":  "acme/old",
				"private":    true,
				"updated_at": "2026-09-01T08:00:00Z",
			}
		}
		first[0]["full_name"] = "acme/paid-packages"
		_ = json.NewEncoder(w).Encode(first)
	}))
	defer server.Close()
	previous := sourceGitHubAPIBase
	sourceGitHubAPIBase = server.URL
	t.Cleanup(func() {
		sourceGitHubAPIBase = previous
		accessibleGitHubRepoCache = gitHubRepoCache{}
	})

	items, err := listGitHubReposForToken(context.Background(), "ghp_list")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || pages[0] != "1" || pages[1] != "2" {
		t.Fatalf("pages = %#v", pages)
	}
	if items[0].Repo != "acme/newer" || items[0].Private || items[0].UpdatedAt.Format(time.RFC3339) != "2026-09-30T08:00:00Z" {
		t.Fatalf("first = %#v", items[0])
	}
	if _, err := listGitHubReposForToken(context.Background(), "ghp_list"); err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("cache missed, pages = %#v", pages)
	}
}

func TestListGitHubReposTokenAndTimeout(t *testing.T) {
	accessibleGitHubRepoCache = gitHubRepoCache{}
	t.Cleanup(func() { accessibleGitHubRepoCache = gitHubRepoCache{} })
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer denied.Close()
	previous := sourceGitHubAPIBase
	sourceGitHubAPIBase = denied.URL
	t.Cleanup(func() { sourceGitHubAPIBase = previous })
	if _, err := listGitHubReposForToken(context.Background(), "ghp_old"); !errorsIsToken(err) {
		t.Fatalf("401 err = %v", err)
	}

	hung := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer hung.Close()
	sourceGitHubAPIBase = hung.URL
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := listGitHubReposForToken(ctx, "ghp_slow"); err == nil || errorsIsToken(err) {
		t.Fatalf("timeout err = %v", err)
	}
}

func TestAnnotateGitHubRepoBindingsSkipsCurrentApp(t *testing.T) {
	items := []gitHubRepoChoice{{Repo: "acme/auth-system"}, {Repo: "acme/shop"}}
	annotateGitHubRepoBindings(items, map[string]gitHubRepoBinding{
		"acme/auth-system": {appID: 7, name: "授权系统"},
		"acme/shop":        {appID: 8, name: "客户门户"},
	}, 7)
	if items[0].BoundApp != "" || items[1].BoundApp != "客户门户" {
		t.Fatalf("bound = %#v", items)
	}
}

func errorsIsToken(err error) bool {
	return err != nil && strings.Contains(err.Error(), errGitHubRepoToken.Error())
}
