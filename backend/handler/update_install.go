package handler

import (
	"context"
	_ "embed"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// 官网 /install.sh 下发这一份。唯一源就是同目录的 install.sh，不从客户发布包读取，构建也不再复制。
//
//go:embed install.sh
var productInstallScript []byte

const (
	// 安装脚本只是一段 shell，不接受异常大的正文。
	productUpdateInstallScriptMax = 1 << 20
)

// RegisterPublicInstallRoute 在站点根注册 /install.sh。
// 正文是构建时嵌入的 backend/handler/install.sh，不从客户发布包读取，也不按请求头、查询参数或环境变量改写。
// 脚本里的官网地址必须是写死的 https://auth.maizll.com，带可覆盖入口的脚本不下发。
// 安装包下载仍走 /api/v1/update/package/:version：不登录、不收令牌，只发这个应用的已发布客户端包。
// 付费插件走软件目录的另一条下载接口，不会从这里发出去。
func RegisterPublicInstallRoute(engine *gin.Engine) {
	engine.GET("/install.sh", productUpdateInstallScript)
}

func productUpdateInstallScript(c *gin.Context) {
	if !productUpdateAllow(c, productUpdateJSONLimiter) {
		return
	}
	body, err := productUpdateInstallScriptBody(c.Request.Context())
	if err != nil {
		productUpdateFail(c, err)
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "text/x-shellscript; charset=utf-8", body)
}

func productUpdateInstallScriptBody(_ context.Context) ([]byte, error) {
	body := productInstallScript
	if len(body) == 0 || len(body) > productUpdateInstallScriptMax {
		return nil, errProductUpdateUnavailable
	}
	if !strings.HasPrefix(string(body), "#!") {
		return nil, errProductUpdateUnavailable
	}
	// 脚本里如果夹进了仓库地址，或官网地址还能被改掉，整段不下发。
	if productUpdateBodyLeaks(body) || !installScriptPinsOfficialOrigin(body) {
		return nil, errProductUpdateUnavailable
	}
	out := make([]byte, len(body))
	copy(out, body)
	return out, nil
}

// installScriptPinsOfficialOrigin 要求更新地址是写死的官网，不能从环境变量拼出来。
// 版本号仍可以是清单里的 ${VERSION}。注释行不参与判断。
func installScriptPinsOfficialOrigin(body []byte) bool {
	latest := false
	pkg := false
	text := string(body)
	if strings.Contains(text, "AUTH_PRO_UPDATE_BASE") {
		return false
	}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.Contains(line, "/api/v1/update/") {
			continue
		}
		if !strings.Contains(line, "https://auth.maizll.com/api/v1/update/") {
			return false
		}
		rest := strings.ReplaceAll(line, "${VERSION}", "")
		if strings.Contains(rest, "${") || strings.Contains(rest, "$AUTH") || strings.Contains(rest, "$BASE") {
			return false
		}
		if strings.Contains(line, "https://auth.maizll.com/api/v1/update/latest.json") {
			latest = true
		}
		if strings.Contains(line, "https://auth.maizll.com/api/v1/update/package/") {
			pkg = true
		}
	}
	return latest && pkg
}
