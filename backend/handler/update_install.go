package handler

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	productUpdateInstallScriptName = "install.sh"
	// 安装脚本只是一段 shell，不接受异常大的成员，避免把安装包其余内容读进内存。
	productUpdateInstallScriptMax = 1 << 20
)

// 发布包里没有安装脚本时告诉客户这一句。其它失败仍用固定的「暂时无法获取更新」，不带路径。
var errProductUpdateInstallScriptMissing = errors.New("当前发布的安装包里没有安装脚本")

// RegisterPublicInstallRoute 在站点根注册 /install.sh。
// 正文原样来自「授权系统」当前已发布安装包，不按请求头、查询参数或环境变量改写。
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

func productUpdateInstallScriptBody(ctx context.Context) ([]byte, error) {
	manifest, err := productUpdateCachedManifest(ctx)
	if err != nil {
		return nil, err
	}
	version := strings.TrimPrefix(strings.TrimSpace(manifest.Version), "v")
	path, _, err := productUpdatePackageFile(ctx, version)
	if err != nil {
		return nil, err
	}
	return readPackageInstallScript(path)
}

// readPackageInstallScript 只读取压缩包根上的 install.sh。
// 名字里带 .. 或放在子目录的同名文件一律忽略，避免把别的文件当成脚本下发。
func readPackageInstallScript(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errProductUpdateUnavailable
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil, errProductUpdateUnavailable
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil, errProductUpdateInstallScriptMissing
		}
		if err != nil {
			return nil, errProductUpdateUnavailable
		}
		// 只认普通文件。发布包由 GNU tar 写成 '0'，不接受链接或旧的空类型标记。
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if !isPackageInstallScript(header.Name) {
			continue
		}
		if header.Size <= 0 || header.Size > productUpdateInstallScriptMax {
			return nil, errProductUpdateUnavailable
		}
		body, err := io.ReadAll(io.LimitReader(reader, header.Size+1))
		if err != nil || int64(len(body)) != header.Size {
			return nil, errProductUpdateUnavailable
		}
		if !strings.HasPrefix(string(body), "#!") {
			return nil, errProductUpdateUnavailable
		}
		// 脚本里如果夹进了仓库地址，或官网地址还能被改掉，整段不下发。
		if productUpdateBodyLeaks(body) || !installScriptPinsOfficialOrigin(body) {
			return nil, errProductUpdateUnavailable
		}
		return body, nil
	}
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

func isPackageInstallScript(name string) bool {
	if strings.Contains(name, `\`) || strings.Contains(name, "..") {
		return false
	}
	name = strings.TrimPrefix(name, "./")
	return name == productUpdateInstallScriptName
}
