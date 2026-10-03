package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 安装包文件页按发布标签分组：GitHub 文件要带上标签、大小，发布多于一页时要翻页列全。
func TestGitHubStorageObjectsCarryTagAndPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := 100
		if r.URL.Query().Get("page") == "2" {
			count = 1
		}
		parts := make([]string, 0, count)
		for i := 0; i < count; i++ {
			tag := fmt.Sprintf("client/v1.%s.%d", r.URL.Query().Get("page"), i)
			parts = append(parts, `{"tag_name":"`+tag+`","assets":[{"name":"latest.json","size":12,"updated_at":"2026-10-01T02:00:00Z"}]}`)
		}
		_, _ = w.Write([]byte("[" + strings.Join(parts, ",") + "]"))
	}))
	defer server.Close()
	previous := sourceGitHubAPIBase
	sourceGitHubAPIBase = server.URL
	t.Cleanup(func() { sourceGitHubAPIBase = previous })

	objects, err := listGitHubObjects(context.Background(), storageLocation{Owner: "acme", Repo: "packages"}, "token")
	if err != nil || len(objects) != 101 {
		t.Fatalf("应翻页列全 101 个: %d %v", len(objects), err)
	}
	last := objects[100]
	if last.Tag != "client/v1.2.0" || last.Key != "acme/packages/client/v1.2.0/latest.json" || last.Size != 12 || last.Updated.IsZero() {
		t.Fatalf("文件信息不全: %+v", last)
	}
}
