package handler

import (
	"strings"
	"testing"
)

// 复现 H2：昵称是一段脚本，开通邮件的 HTML 内容原样带上它，后台邮件日志预览就会执行。
func TestMailTemplateEscapesUserInputInHTML(t *testing.T) {
	vars := map[string]string{
		"{{ownerName}}":  `<img src=x onerror=alert(1)>`,
		"{{appName}}":    `A&B "应用"`,
		"{{licenseKey}}": "{{ownerName}}",
	}
	tpl := `<p>您好 {{ownerName}}，{{appName}} 已开通，密钥 {{licenseKey}}</p>`
	got := renderMailTemplate(tpl, vars, true)
	if strings.Contains(got, "<img") || !strings.Contains(got, "&lt;img src=x onerror=alert(1)&gt;") {
		t.Fatalf("HTML 邮件没有转义昵称: %s", got)
	}
	if !strings.Contains(got, "A&amp;B &#34;应用&#34;") {
		t.Fatalf("应用名没有转义: %s", got)
	}
	if !strings.Contains(got, "密钥 {{ownerName}}") {
		t.Fatalf("变量值里的占位符被二次展开: %s", got)
	}
	plain := renderMailTemplate("您好 {{ownerName}}", vars, false)
	if plain != "您好 <img src=x onerror=alert(1)>" {
		t.Fatalf("纯文本邮件不应转义: %s", plain)
	}
}

func TestDisplayNameRejectsMarkup(t *testing.T) {
	for _, bad := range []string{"", `<img src=x onerror=alert(1)>`, "a\x00b", strings.Repeat("长", 51)} {
		if displayNameError(bad) == "" {
			t.Fatalf("%q 应被拒绝", bad)
		}
	}
	for _, good := range []string{"张三", "Tom & Jerry", "O'Brien", strings.Repeat("长", 50)} {
		if msg := displayNameError(good); msg != "" {
			t.Fatalf("%q 被误拒: %s", good, msg)
		}
	}
}
