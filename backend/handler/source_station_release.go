package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	sourceGitHubAPIBase       = "https://api.github.com"
	sourceGiteeAPIBase        = "https://gitee.com/api/v5"
	sourceReleaseHTTPClient   = &http.Client{Timeout: 45 * time.Second}
	sourceReleaseRepoPattern  = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	sourceReleaseTagSanitizer = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
)

func defaultSourceReleaseSettings() sourceReleaseSettings {
	return sourceReleaseSettings{
		Provider:    "github",
		TagStrategy: "{id}-{version}",
		Branch:      "master",
	}
}

func normalizeReleaseSettings(settings sourceReleaseSettings) sourceReleaseSettings {
	settings.Provider = strings.ToLower(strings.TrimSpace(settings.Provider))
	if settings.Provider == "" {
		settings.Provider = "github"
	}
	settings.Owner = strings.TrimSpace(settings.Owner)
	settings.Repo = strings.TrimSpace(settings.Repo)
	settings.Token = strings.TrimSpace(settings.Token)
	settings.TagStrategy = strings.TrimSpace(settings.TagStrategy)
	if settings.TagStrategy == "" {
		settings.TagStrategy = "{id}-{version}"
	}
	settings.Branch = strings.TrimSpace(settings.Branch)
	if settings.Branch == "" {
		settings.Branch = "master"
	}
	return settings
}

func validateReleaseSettings(settings sourceReleaseSettings) error {
	settings = normalizeReleaseSettings(settings)
	if settings.Provider != "github" && settings.Provider != "gitee" {
		return errors.New("provider 仅支持 github 或 gitee")
	}
	if settings.Owner != "" && !sourceReleaseRepoPattern.MatchString(settings.Owner) {
		return errors.New("owner 不合法")
	}
	if settings.Repo != "" && !sourceReleaseRepoPattern.MatchString(settings.Repo) {
		return errors.New("repo 不合法")
	}
	return nil
}

func (settings sourceReleaseSettings) releaseReady() bool {
	settings = normalizeReleaseSettings(settings)
	return (settings.Provider == "github" || settings.Provider == "gitee") &&
		settings.Owner != "" && settings.Repo != "" && settings.Token != ""
}

func (settings sourceReleaseSettings) publicView() gin.H {
	settings = normalizeReleaseSettings(settings)
	return gin.H{
		"provider":    settings.Provider,
		"owner":       settings.Owner,
		"repo":        settings.Repo,
		"tagStrategy": settings.TagStrategy,
		"branch":      settings.Branch,
		"hasToken":    settings.Token != "",
		"tokenMasked": maskSourceToken(settings.Token),
		"configured":  settings.releaseReady(),
	}
}

func maskSourceToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if utf8Len := len([]rune(token)); utf8Len <= 8 {
		return "****"
	}
	runes := []rune(token)
	return "****" + string(runes[len(runes)-4:])
}

func isMaskedSourceToken(token string) bool {
	token = strings.TrimSpace(token)
	return token == "" || strings.HasPrefix(token, "****") || strings.Contains(token, "…")
}

func mergeReleaseSettings(current, incoming sourceReleaseSettings) sourceReleaseSettings {
	current = normalizeReleaseSettings(current)
	incoming = normalizeReleaseSettings(incoming)
	out := incoming
	if isMaskedSourceToken(incoming.Token) {
		out.Token = current.Token
	}
	return normalizeReleaseSettings(out)
}

func renderSourceReleaseTag(strategy, id, version, kind string) string {
	if strings.TrimSpace(strategy) == "" {
		strategy = "{id}-{version}"
	}
	tag := strategy
	tag = strings.ReplaceAll(tag, "{id}", id)
	tag = strings.ReplaceAll(tag, "{version}", version)
	tag = strings.ReplaceAll(tag, "{kind}", kind)
	tag = strings.Trim(sourceReleaseTagSanitizer.ReplaceAllString(tag, "-"), "-")
	if tag == "" {
		tag = id + "-" + version
	}
	if len(tag) > 100 {
		tag = tag[:100]
	}
	return tag
}

func pushSourcePackageRelease(ctx context.Context, settings sourceReleaseSettings, manifest sourcePackageManifest, payload []byte) (string, error) {
	settings = normalizeReleaseSettings(settings)
	if !settings.releaseReady() {
		return "", errors.New("未配置 GitHub/Gitee 仓库与令牌")
	}
	switch settings.Provider {
	case "github":
		return pushGitHubRelease(ctx, settings, manifest, payload)
	case "gitee":
		return pushGiteeRelease(ctx, settings, manifest, payload)
	default:
		return "", errors.New("provider 仅支持 github 或 gitee")
	}
}

type gitHubReleaseDTO struct {
	ID            int64            `json:"id"`
	UploadURL     string           `json:"upload_url"`
	HTMLURL       string           `json:"html_url"`
	Assets        []gitHubAssetDTO `json:"assets"`
	Message       string           `json:"message"`
	Documentation string           `json:"documentation_url"`
	Errors        json.RawMessage  `json:"errors"`
}

type gitHubAssetDTO struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func pushGitHubRelease(ctx context.Context, settings sourceReleaseSettings, manifest sourcePackageManifest, payload []byte) (string, error) {
	tag := renderSourceReleaseTag(settings.TagStrategy, manifest.ID, manifest.Version, manifest.Kind)
	filename := sourceReleaseAssetName(manifest)
	api := strings.TrimRight(sourceGitHubAPIBase, "/")
	owner, repo := url.PathEscape(settings.Owner), url.PathEscape(settings.Repo)
	createURL := api + "/repos/" + owner + "/" + repo + "/releases"
	body := map[string]any{
		"tag_name":   tag,
		"name":       manifest.Name + " " + manifest.Version,
		"body":       sourceFirstNonEmpty(manifest.Description, manifest.ID+" "+manifest.Version),
		"draft":      false,
		"prerelease": false,
	}
	status, raw, err := sourceReleaseJSON(ctx, http.MethodPost, createURL, githubHeaders(settings.Token), body)
	if err != nil {
		return "", err
	}
	var release gitHubReleaseDTO
	_ = json.Unmarshal(raw, &release)
	if status == http.StatusUnprocessableEntity || status == http.StatusConflict {
		getURL := api + "/repos/" + owner + "/" + repo + "/releases/tags/" + url.PathEscape(tag)
		status, raw, err = sourceReleaseJSON(ctx, http.MethodGet, getURL, githubHeaders(settings.Token), nil)
		if err != nil {
			return "", err
		}
		release = gitHubReleaseDTO{}
		_ = json.Unmarshal(raw, &release)
	}
	if status < 200 || status >= 300 || release.ID == 0 {
		return "", githubAPIError("创建或读取 Release 失败", status, raw, release.Message)
	}
	for _, asset := range release.Assets {
		if !strings.EqualFold(asset.Name, filename) {
			continue
		}
		delURL := api + "/repos/" + owner + "/" + repo + "/releases/assets/" + fmt.Sprintf("%d", asset.ID)
		_, _, _ = sourceReleaseJSON(ctx, http.MethodDelete, delURL, githubHeaders(settings.Token), nil)
	}
	uploadURL, err := githubAssetUploadURL(release.UploadURL, filename)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	for key, value := range githubHeaders(settings.Token) {
		req.Header.Set(key, value)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(payload))
	status, raw, err = sourceReleaseDo(req)
	if err != nil {
		return "", err
	}
	var asset gitHubAssetDTO
	_ = json.Unmarshal(raw, &asset)
	if status < 200 || status >= 300 || strings.TrimSpace(asset.BrowserDownloadURL) == "" {
		return "", githubAPIError("上传 Release 附件失败", status, raw, "")
	}
	if err := validateExternalHTTPS(asset.BrowserDownloadURL); err != nil {
		return "", errors.New("GitHub 返回的下载地址不是 https:// 外部地址")
	}
	return asset.BrowserDownloadURL, nil
}

type giteeReleaseDTO struct {
	ID      int64           `json:"id"`
	Assets  []giteeAssetDTO `json:"assets"`
	Message string          `json:"message"`
}

type giteeAssetDTO struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	DownloadURL        string `json:"download_url"`
}

func pushGiteeRelease(ctx context.Context, settings sourceReleaseSettings, manifest sourcePackageManifest, payload []byte) (string, error) {
	tag := renderSourceReleaseTag(settings.TagStrategy, manifest.ID, manifest.Version, manifest.Kind)
	filename := sourceReleaseAssetName(manifest)
	api := strings.TrimRight(sourceGiteeAPIBase, "/")
	owner, repo := url.PathEscape(settings.Owner), url.PathEscape(settings.Repo)
	createURL := api + "/repos/" + owner + "/" + repo + "/releases"
	body := map[string]any{
		"access_token":     settings.Token,
		"tag_name":         tag,
		"name":             manifest.Name + " " + manifest.Version,
		"body":             sourceFirstNonEmpty(manifest.Description, manifest.ID+" "+manifest.Version),
		"target_commitish": settings.Branch,
	}
	status, raw, err := sourceReleaseJSON(ctx, http.MethodPost, createURL, map[string]string{"Content-Type": "application/json"}, body)
	if err != nil {
		return "", err
	}
	var release giteeReleaseDTO
	_ = json.Unmarshal(raw, &release)
	if status >= 400 || release.ID == 0 {
		getURL := api + "/repos/" + owner + "/" + repo + "/releases/tags/" + url.PathEscape(tag) + "?access_token=" + url.QueryEscape(settings.Token)
		status, raw, err = sourceReleaseJSON(ctx, http.MethodGet, getURL, nil, nil)
		if err != nil {
			return "", err
		}
		release = giteeReleaseDTO{}
		_ = json.Unmarshal(raw, &release)
	}
	if status < 200 || status >= 300 || release.ID == 0 {
		return "", githubAPIError("创建或读取 Gitee Release 失败", status, raw, release.Message)
	}
	for _, asset := range release.Assets {
		if strings.EqualFold(asset.Name, filename) {
			if loc := sourceFirstNonEmpty(asset.BrowserDownloadURL, asset.DownloadURL); loc != "" {
				if err := validateExternalHTTPS(loc); err == nil {
					return loc, nil
				}
			}
		}
	}
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("access_token", settings.Token)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(payload); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	attachURL := api + "/repos/" + owner + "/" + repo + "/releases/" + fmt.Sprintf("%d", release.ID) + "/attach_files"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, attachURL, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	status, raw, err = sourceReleaseDo(req)
	if err != nil {
		return "", err
	}
	var asset giteeAssetDTO
	_ = json.Unmarshal(raw, &asset)
	location := sourceFirstNonEmpty(asset.BrowserDownloadURL, asset.DownloadURL)
	if status < 200 || status >= 300 || location == "" {
		return "", githubAPIError("上传 Gitee Release 附件失败", status, raw, "")
	}
	if err := validateExternalHTTPS(location); err != nil {
		return "", errors.New("Gitee 返回的下载地址不是 https:// 外部地址")
	}
	return location, nil
}

type gitHubRepoDTO struct {
	FullName    string `json:"full_name"`
	HTMLURL     string `json:"html_url"`
	Private     bool   `json:"private"`
	Message     string `json:"message"`
	Permissions struct {
		Admin bool `json:"admin"`
		Push  bool `json:"push"`
		Pull  bool `json:"pull"`
	} `json:"permissions"`
}

type giteeRepoDTO struct {
	FullName   string `json:"full_name"`
	HTMLURL    string `json:"html_url"`
	Private    bool   `json:"private"`
	Message    string `json:"message"`
	Permission struct {
		Admin bool `json:"admin"`
		Push  bool `json:"push"`
		Pull  bool `json:"pull"`
	} `json:"permission"`
}

func testSourceReleaseConnection(ctx context.Context, settings sourceReleaseSettings) (gin.H, int, error) {
	settings = normalizeReleaseSettings(settings)
	switch settings.Provider {
	case "github":
		return testGitHubRepo(ctx, settings)
	case "gitee":
		return testGiteeRepo(ctx, settings)
	default:
		return nil, http.StatusBadRequest, errors.New("provider 仅支持 github 或 gitee")
	}
}

func testGitHubRepo(ctx context.Context, settings sourceReleaseSettings) (gin.H, int, error) {
	api := strings.TrimRight(sourceGitHubAPIBase, "/")
	owner, repo := url.PathEscape(settings.Owner), url.PathEscape(settings.Repo)
	status, raw, err := sourceReleaseJSON(ctx, http.MethodGet, api+"/repos/"+owner+"/"+repo, githubHeaders(settings.Token), nil)
	if err != nil {
		return nil, 0, err
	}
	var payload gitHubRepoDTO
	_ = json.Unmarshal(raw, &payload)
	if status < 200 || status >= 300 {
		return nil, status, releaseTestAPIError(status, raw, payload.Message)
	}
	return sourceReleaseTestView(settings, payload.FullName, payload.HTMLURL, payload.Private, payload.Permissions.Admin, payload.Permissions.Push, payload.Permissions.Pull), status, nil
}

func testGiteeRepo(ctx context.Context, settings sourceReleaseSettings) (gin.H, int, error) {
	api := strings.TrimRight(sourceGiteeAPIBase, "/")
	owner, repo := url.PathEscape(settings.Owner), url.PathEscape(settings.Repo)
	getURL := api + "/repos/" + owner + "/" + repo + "?access_token=" + url.QueryEscape(settings.Token)
	status, raw, err := sourceReleaseJSON(ctx, http.MethodGet, getURL, nil, nil)
	if err != nil {
		return nil, 0, err
	}
	var payload giteeRepoDTO
	_ = json.Unmarshal(raw, &payload)
	if status < 200 || status >= 300 {
		return nil, status, releaseTestAPIError(status, raw, payload.Message)
	}
	return sourceReleaseTestView(settings, payload.FullName, payload.HTMLURL, payload.Private, payload.Permission.Admin, payload.Permission.Push, payload.Permission.Pull), status, nil
}

func sourceReleaseTestView(settings sourceReleaseSettings, fullName, htmlURL string, private, admin, push, pull bool) gin.H {
	settings = normalizeReleaseSettings(settings)
	if strings.TrimSpace(fullName) == "" {
		fullName = settings.Owner + "/" + settings.Repo
	}
	return gin.H{
		"provider": settings.Provider,
		"owner":    settings.Owner,
		"repo":     settings.Repo,
		"fullName": fullName,
		"htmlUrl":  htmlURL,
		"private":  private,
		"permissions": gin.H{
			"admin": admin,
			"push":  push,
			"pull":  pull,
		},
	}
}

func releaseTestAPIError(status int, raw []byte, message string) error {
	prefix := "连接失败"
	switch status {
	case http.StatusUnauthorized:
		prefix = "连接失败：令牌无效"
	case http.StatusForbidden:
		prefix = "连接失败：权限不足"
	case http.StatusNotFound:
		prefix = "连接失败：仓库不存在或无权访问"
	}
	return githubAPIError(prefix, status, raw, message)
}

func githubHeaders(token string) map[string]string {
	return map[string]string{
		"Accept":               "application/vnd.github+json",
		"Authorization":        "Bearer " + token,
		"X-GitHub-Api-Version": "2022-11-28",
		"Content-Type":         "application/json",
	}
}

func githubAssetUploadURL(raw, filename string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("GitHub Release 缺少 upload_url")
	}
	raw = strings.ReplaceAll(raw, "{?name,label}", "")
	raw = strings.ReplaceAll(raw, "{?name}", "")
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("GitHub upload_url 不合法")
	}
	query := parsed.Query()
	query.Set("name", filename)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func githubAPIError(prefix string, status int, raw []byte, message string) error {
	msg := strings.TrimSpace(message)
	if msg == "" {
		var payload struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &payload)
		msg = strings.TrimSpace(payload.Message)
	}
	if msg == "" {
		msg = strings.TrimSpace(string(raw))
	}
	if utf8Short := []rune(msg); len(utf8Short) > 180 {
		msg = string(utf8Short[:180])
	}
	if msg == "" {
		return fmt.Errorf("%s（HTTP %d）", prefix, status)
	}
	return fmt.Errorf("%s（HTTP %d）：%s", prefix, status, msg)
}

func sourceReleaseJSON(ctx context.Context, method, rawURL string, headers map[string]string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return 0, nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return sourceReleaseDo(req)
}

func sourceReleaseDo(req *http.Request) (int, []byte, error) {
	resp, err := sourceReleaseHTTPClient.Do(req)
	if err != nil {
		return 0, nil, errors.New("访问 GitHub/Gitee 失败：" + err.Error())
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, raw, nil
}
