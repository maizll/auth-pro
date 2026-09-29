package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReleaseImportVersionAndChangelog(t *testing.T) {
	if got := releaseImportVersion("v1.7.2"); got != "1.7.2" {
		t.Fatalf("version=%s", got)
	}
	body := "修复登录。\n详见 https://github.com/maizll/auth-pro-client/releases/tag/v1.7.2\n## What's Changed\n* 自动说明 by @bot in https://github.com/acme/widgets/pull/1\n**Full Changelog**: https://github.com/acme/widgets/compare/v1.7.1...v1.7.2\n补上到期时间。"
	got := releaseImportChangelog(body)
	if got != "修复登录。\n补上到期时间。" {
		t.Fatalf("changelog=%q", got)
	}
	if strings.Contains(strings.ToLower(got), "github.com") || strings.Contains(got, "What's Changed") {
		t.Fatalf("template leaked: %q", got)
	}
}

func TestReleaseImportPrefersLatestNotes(t *testing.T) {
	notes := `{"version":"1.7.8","notes":["修复进程守护仍去下载旧的面板脚本。","启动前把 backend 交给网站运行用户。","证书失败时写明原因，并可单独申请。"]}`
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/acme/widgets/releases" && r.URL.Query().Get("per_page") == "20":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{
				"tag_name":"v1.7.8",
				"name":"v1.7.8",
				"body":"## What's Changed\n* something by @bot in https://github.com/acme/widgets/pull/9\n\n**Full Changelog**: https://github.com/acme/widgets/compare/v1.7.7...v1.7.8\n",
				"published_at":"2026-09-29T00:00:00Z",
				"draft":false,
				"assets":[
					{"name":"latest.json","size":120},
					{"name":"auth_pro-full-v1.7.8.tar.gz","size":99}
				]
			}]`))
		case r.URL.Path == "/repos/acme/widgets/releases/tags/v1.7.8":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"assets":[{"id":7,"name":"latest.json","url":"` + server.URL + `/asset/latest.json"},{"id":8,"name":"auth_pro-full-v1.7.8.tar.gz","url":"` + server.URL + `/asset/pkg"}]}`))
		case r.URL.Path == "/asset/latest.json":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte(notes))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	previous := productUpdateGitHubAPI
	productUpdateGitHubAPI = server.URL
	t.Cleanup(func() { productUpdateGitHubAPI = previous })

	list, err := listReleaseImportReleases(context.Background(), "acme", "widgets")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("releases=%d", len(list))
	}
	item := list[0]
	if item.AssetName != "auth_pro-full-v1.7.8.tar.gz" {
		t.Fatalf("asset=%s", item.AssetName)
	}
	if !strings.Contains(item.Title, "修复进程守护") {
		t.Fatalf("title=%q", item.Title)
	}
	if !strings.Contains(item.Changelog, "启动前把 backend") || !strings.Contains(item.Changelog, "证书失败时写明原因") {
		t.Fatalf("changelog=%q", item.Changelog)
	}
	if strings.Contains(strings.ToLower(item.Changelog), "github.com") || strings.Contains(item.Changelog, "What's Changed") || strings.Contains(item.Title, "What's Changed") {
		t.Fatalf("template leaked title=%q changelog=%q", item.Title, item.Changelog)
	}
}
