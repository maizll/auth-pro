package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestFetchPluginSourceManifestJSON(t *testing.T) {
	templatePayload := []byte(`{"schemaVersion":1,"hero":{"title":"远程首页"}}`)
	checksum := sha256.Sum256(templatePayload)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(writer, `{"name":"test","homeTemplates":[{"id":"remote-home","name":"远程首页","description":"test","version":"1.0.0","schemaVersion":1,"sha256":"%s","templateUrl":"template.json"}]}`, hex.EncodeToString(checksum[:]))
	}))
	defer server.Close()

	index, payload, sourceType, err := fetchPluginSourceManifest(context.Background(), server.URL+"/index.json")
	if err != nil {
		t.Fatalf("fetchPluginSourceManifest() error = %v", err)
	}
	if sourceType != "json" || len(payload) == 0 || len(index.HomeTemplates) != 1 {
		t.Fatalf("unexpected manifest result: type=%q payload=%d templates=%d", sourceType, len(payload), len(index.HomeTemplates))
	}
}

func TestFetchPluginSourceManifestFromEnvironment(t *testing.T) {
	sourceURL := os.Getenv("AUTO_PRO_HOME_TEMPLATE_TEST_SOURCE_URL")
	if sourceURL == "" {
		t.Skip("AUTO_PRO_HOME_TEMPLATE_TEST_SOURCE_URL is not set")
	}
	index, payload, sourceType, err := fetchPluginSourceManifest(context.Background(), sourceURL)
	if err != nil {
		t.Fatalf("fetchPluginSourceManifest() error = %v", err)
	}
	if sourceType != "json" || len(payload) == 0 || len(index.HomeTemplates) != 2 {
		t.Fatalf("unexpected source result: type=%q payload=%d templates=%d", sourceType, len(payload), len(index.HomeTemplates))
	}
}

func TestFetchJSONSourceManifestUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	serverURL := server.URL
	server.Close()
	if _, _, err := fetchJSONSourceManifest(context.Background(), serverURL+"/index.json"); err == nil {
		t.Fatal("unavailable source should return an error")
	}
}

func TestParsePluginSourceManifestRejectsMalformedTemplates(t *testing.T) {
	if _, err := parsePluginSourceManifest([]byte(`not-json`)); err == nil {
		t.Fatal("invalid JSON should be rejected")
	}
	manifest := []byte(`{"homeTemplates":[{"id":"broken-home","name":"Broken","version":"1.0.0","schemaVersion":1,"sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}]}`)
	if _, err := parsePluginSourceManifest(manifest); err == nil {
		t.Fatal("template without templateUrl or templatePath should be rejected")
	}
}

func TestFetchLimitedHTTPRejectsOversizedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("12345"))
	}))
	defer server.Close()
	if _, err := fetchLimitedHTTP(context.Background(), server.URL, 4, time.Second); err == nil {
		t.Fatal("oversized response should be rejected")
	}
}

func TestFetchGitSourceManifest(t *testing.T) {
	repositoryDir := t.TempDir()
	manifest := `{"name":"git-test","homeTemplates":[]}`
	if err := os.WriteFile(filepath.Join(repositoryDir, "index.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, repositoryDir, "init")
	runGitTestCommand(t, repositoryDir, "add", "index.json")
	runGitTestCommand(t, repositoryDir, "-c", "user.name=Codex Test", "-c", "user.email=codex@example.com", "commit", "-m", "init")

	index, payload, err := fetchGitSourceManifest(context.Background(), repositoryDir)
	if err != nil {
		t.Fatalf("fetchGitSourceManifest() error = %v", err)
	}
	if index.Name != "git-test" || string(payload) != manifest {
		t.Fatalf("unexpected git manifest: name=%q payload=%q", index.Name, string(payload))
	}
}

func TestValidateRemoteHomeTemplates(t *testing.T) {
	valid := remoteHomeTemplate{
		ID: "clean-home", Name: "Clean Home", Version: "1.0.0", SchemaVersion: 1,
		SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", TemplatePath: "templates/clean.json",
	}
	if err := validateRemoteHomeTemplates([]remoteHomeTemplate{valid}); err != nil {
		t.Fatalf("valid template rejected: %v", err)
	}
	invalidChecksum := valid
	invalidChecksum.ID = "bad-checksum"
	invalidChecksum.SHA256 = "abc"
	if err := validateRemoteHomeTemplates([]remoteHomeTemplate{invalidChecksum}); err == nil {
		t.Fatal("invalid checksum should be rejected")
	}
	if err := validateRemoteHomeTemplates([]remoteHomeTemplate{valid, valid}); err == nil {
		t.Fatal("duplicate template id should be rejected")
	}
}

func TestValidateHomeTemplateDocument(t *testing.T) {
	if err := validateHomeTemplateDocument([]byte(`{"schemaVersion":1,"hero":{"title":"首页"}}`)); err != nil {
		t.Fatalf("valid document rejected: %v", err)
	}
	if err := validateHomeTemplateDocument([]byte(`{"schemaVersion":1,"hero":{"title":"首页"},"scripts":[]}`)); err == nil {
		t.Fatal("scripts field should be rejected")
	}
	if err := validateHomeTemplateDocument([]byte(`{"schemaVersion":2,"hero":{"title":"首页"}}`)); err == nil {
		t.Fatal("unsupported schema should be rejected")
	}
}

func TestSafeRepositoryPath(t *testing.T) {
	repositoryDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repositoryDir, "templates"), 0755); err != nil {
		t.Fatal(err)
	}
	expectedPath := filepath.Join(repositoryDir, "templates", "home.json")
	if err := os.WriteFile(expectedPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	pathValue, err := safeRepositoryPath(repositoryDir, "templates/home.json")
	if err != nil || pathValue != expectedPath {
		t.Fatalf("safeRepositoryPath() = %q, %v", pathValue, err)
	}
	if _, err := safeRepositoryPath(repositoryDir, "../outside.json"); err == nil {
		t.Fatal("parent traversal should be rejected")
	}
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "outside.json")
	if err := os.WriteFile(outsidePath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(repositoryDir, "templates", "linked.json")
	if err := os.Symlink(outsidePath, symlinkPath); err == nil {
		if _, err := safeRepositoryPath(repositoryDir, "templates/linked.json"); err == nil {
			t.Fatal("symlink escaping repository should be rejected")
		}
	}
}

func TestResolveTemplateURL(t *testing.T) {
	resolved, err := resolveTemplateURL("https://example.com/store/index.json", "templates/home.json")
	if err != nil || resolved != "https://example.com/store/templates/home.json" {
		t.Fatalf("resolveTemplateURL() = %q, %v", resolved, err)
	}
	if _, err := resolveTemplateURL("https://example.com/store/index.json", "file:///tmp/home.json"); err == nil {
		t.Fatal("non-http template URL should be rejected")
	}
}

func runGitTestCommand(t *testing.T, repositoryDir string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", repositoryDir}, args...)
	command := exec.Command("git", commandArgs...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v: %s", args, err, output)
	}
}
