package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPluginSourceManifestIgnoresHomeTemplates(t *testing.T) {
	index, err := parsePluginSourceManifest([]byte(`{"name":"Plugins","plugins":[{"id":"demo-plugin","name":"Demo","version":"1.0.0"}],"homeTemplates":[{"ignored":true}]}`))
	if err != nil || len(index.Plugins) != 1 || len(index.HomeTemplates) != 1 {
		t.Fatalf("index=%+v err=%v", index, err)
	}
}

func TestFetchPluginSourceManifestJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = response.Write([]byte(`{"name":"Plugins","plugins":[{"id":"demo-plugin","name":"Demo","version":"1.0.0"}]}`))
	}))
	defer server.Close()
	index, _, sourceType, err := fetchPluginSourceManifest(context.Background(), server.URL+"/index.json")
	if err != nil || sourceType != "json" || len(index.Plugins) != 1 {
		t.Fatalf("type=%q index=%+v err=%v", sourceType, index, err)
	}
}

func TestJSONCatalogURLIsNeverCloned(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.RequestURI())
		http.Error(response, "upstream down", http.StatusBadGateway)
	}))
	defer server.Close()
	rawURL := server.URL + "/software-source/app_4e85b4724223_2603/index.json"
	_, _, _, err := fetchPluginSourceManifest(context.Background(), rawURL)
	if err == nil {
		t.Fatal("expected json fetch error")
	}
	if strings.Contains(err.Error(), "fatal:") || strings.Contains(err.Error(), "Cloning") || strings.Contains(err.Error(), "Git 仓库") {
		t.Fatalf("json url fell through to git: %v", err)
	}
	if !strings.Contains(err.Error(), "JSON 目录拉取失败") {
		t.Fatalf("error=%v", err)
	}
	for _, path := range paths {
		if strings.Contains(path, "info/refs") {
			t.Fatalf("cloned json url, paths=%v", paths)
		}
	}
}

func TestJSONQueryURLAndGitSelectionAreCorrected(t *testing.T) {
	var refs int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "info/refs") {
			refs++
		}
		_, _ = io.WriteString(response, `{"name":"本站软件源","plugins":[{"id":"demo-plugin","name":"Demo","version":"1.0.0"}]}`)
	}))
	defer server.Close()
	rawURL := server.URL + "/software-source/index.json?app_key=demo"
	resolved, err := resolvePluginSource(context.Background(), rawURL, pluginSourceTypeGit)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.sourceType != pluginSourceTypeJSON || resolved.notice != pluginSourceCorrectedToJSON {
		t.Fatalf("resolved=%+v", resolved)
	}
	if refs != 0 {
		t.Fatalf("git clone requests=%d", refs)
	}
	message := pluginSourceAddedMessage(resolved.notice, len(resolved.index.Plugins))
	if !strings.Contains(message, pluginSourceCorrectedToJSON) || !strings.Contains(message, "发现 1 个插件") {
		t.Fatalf("message=%s", message)
	}
}

func TestNonJSONBodyOnJSONURLDoesNotClone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "info/refs") {
			t.Errorf("unexpected git request %s", request.URL.Path)
		}
		_, _ = io.WriteString(response, "<html>not a catalog</html>")
	}))
	defer server.Close()
	_, _, _, err := fetchPluginSourceManifest(context.Background(), server.URL+"/index.json")
	if err == nil || err.Error() != pluginSourceJSONInvalid {
		t.Fatalf("error=%v", err)
	}
}

func TestPlainURLGitCloneFailureIsChinese(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "info/refs") {
			// 哑协议 info/refs 里哈希长度不对时，git 会报 not valid: is this a git repository?
			_, _ = io.WriteString(response, "abc\trefs/heads/main\n")
			return
		}
		_, _ = io.WriteString(response, "not a catalog")
	}))
	defer server.Close()
	_, _, _, err := fetchPluginSourceManifest(context.Background(), server.URL+"/repository")
	if err == nil {
		t.Fatal("expected git failure")
	}
	if err.Error() != errPluginSourceNotGitRepo {
		t.Fatalf("error=%q", err.Error())
	}
	if strings.Contains(err.Error(), "fatal:") || strings.Contains(err.Error(), "Cloning") || strings.Contains(err.Error(), "/tmp/") {
		t.Fatalf("raw git output leaked: %s", err.Error())
	}
	if pluginSourceFailureMessage("软件源刷新失败：", err) != errPluginSourceNotGitRepo {
		t.Fatalf("refresh message=%s", pluginSourceFailureMessage("软件源刷新失败：", err))
	}
}

func TestExplainGitCloneFailureHidesRawOutput(t *testing.T) {
	raw := "Cloning into '/tmp/auth-pro-plugin-source-abc/repository'...\nfatal: https://auth.maizll.com/software-source/app_4e85b4724223_2603/index.json/info/refs not valid: is this a git repository?\n"
	err := explainGitCloneFailure(raw)
	if err.Error() != errPluginSourceNotGitRepo {
		t.Fatalf("error=%q", err.Error())
	}
	if strings.Contains(err.Error(), "fatal") || strings.Contains(err.Error(), "Cloning") {
		t.Fatal(err)
	}
	other := explainGitCloneFailure("fatal: unable to access 'https://example.com/repo/': The requested URL returned error: 403")
	if strings.Contains(other.Error(), "fatal") || strings.Contains(other.Error(), "403") {
		t.Fatalf("raw output leaked: %s", other.Error())
	}
}

func TestPluginSourceURLClassification(t *testing.T) {
	if !pluginSourceURLLooksLikeJSON("https://auth.maizll.com/software-source/app_4e85b4724223_2603/index.json") {
		t.Fatal("site index must be json")
	}
	if !pluginSourceURLLooksLikeJSON("https://auth.example.com/software-source/index.json?app_key=demo") {
		t.Fatal("query string must still be json")
	}
	if !pluginSourceURLLooksLikeJSON("https://auth.example.com/INDEX.JSON/") {
		t.Fatal("suffix and case")
	}
	if pluginSourceURLLooksLikeJSON("https://github.com/example/plugins.git") {
		t.Fatal("git remote is not json")
	}
	if looksLikeGitRepositoryURL("https://github.com/org/repo/index.json?raw=1") {
		t.Fatal("github json url must not look like git")
	}
	if !looksLikeGitRepositoryURL("https://github.com/org/repo") {
		t.Fatal("github repo should look like git")
	}
	if _, err := normalizePluginSourceType("svn"); err == nil || !strings.Contains(err.Error(), "源类型") {
		t.Fatalf("err=%v", err)
	}
	if pluginSourceTypeNotice(pluginSourceTypeJSON, pluginSourceTypeGit) != pluginSourceCorrectedToGit {
		t.Fatal("json request corrected to git")
	}
	if pluginSourceTypeNotice("", pluginSourceTypeJSON) != "" {
		t.Fatal("auto detection should not warn")
	}
}
