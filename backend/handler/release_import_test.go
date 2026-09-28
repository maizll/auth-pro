package handler

import "testing"

func TestReleaseImportVersionAndChangelog(t *testing.T) {
	if got := releaseImportVersion("v1.7.2"); got != "1.7.2" {
		t.Fatalf("version=%s", got)
	}
	body := "修复登录。\n详见 https://github.com/maizll/auth-pro-client/releases/tag/v1.7.2\n补上到期时间。"
	got := releaseImportChangelog(body)
	if got != "修复登录。\n补上到期时间。" {
		t.Fatalf("changelog=%q", got)
	}
}
