package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

// 存储检查发现仓库是公开的时，给超级管理员一个「改为私有」按钮：用存储管理里已存的令牌调代码托管站接口改可见性。
// 官网自更新来源仓库不在这里改：1.7.0 及更早的客户站直读它，只给文档步骤，并检查现有令牌改私有后能不能读。

const (
	storageActionMakePrivate = "make_private"
	storagePrivateDoc        = "docs/update-distribution.md 的「仓库改私有当天」"
)

// storageRepoPublicError 是探测存储时「仓库是公开的」这一种结果，存储检查据此给出「改为私有」按钮。
type storageRepoPublicError struct {
	kind, owner, repo, text string
}

func (e *storageRepoPublicError) Error() string { return e.text }

// withMakePrivate 给检查行挂上「改为私有」所需的仓库信息。
func (row storageHealthRow) withMakePrivate(kind, owner, repo string) storageHealthRow {
	row.Action = storageActionMakePrivate
	row.Kind = kind
	row.Owner = owner
	row.Repo = repo
	row.Official = kind == packageStorageGitHub && isOfficialUpdateRepo(owner, repo)
	return row
}

// isOfficialUpdateRepo 判断是不是官网自更新来源仓库（默认仓库，或环境变量改过的仓库）。
func isOfficialUpdateRepo(owner, repo string) bool {
	full := strings.ToLower(strings.TrimSpace(owner) + "/" + strings.TrimSpace(repo))
	if full == strings.ToLower(officialUpdateDefaultRepository) {
		return true
	}
	if o, r, err := officialUpdateRepository(); err == nil {
		return full == strings.ToLower(o+"/"+r)
	}
	return false
}

// githubRepoCall 调一次代码托管站接口，返回状态码、响应头和正文。token 为空时匿名。
func githubRepoCall(ctx context.Context, apiBase, method, path, token string, body any) (int, http.Header, []byte, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return 0, nil, nil, err
		}
		reader = strings.NewReader(string(payload))
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(apiBase, "/")+path, reader)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := sourceReleaseHTTPClient.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, resp.Header, payload, err
}

func repoPath(owner, repo string) string {
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
}

// officialRepoVisibility 匿名读一次官网更新来源仓库：public / hidden（私有或不存在）/ 空（连不上）。
func officialRepoVisibility(ctx context.Context, owner, repo string) string {
	status, _, payload, err := githubRepoCall(ctx, productUpdateGitHubAPI, http.MethodGet, repoPath(owner, repo), "", nil)
	if err != nil {
		return ""
	}
	if status == http.StatusNotFound {
		return "hidden"
	}
	var body struct {
		Private bool `json:"private"`
	}
	if status != http.StatusOK || json.Unmarshal(payload, &body) != nil {
		return ""
	}
	if body.Private {
		return "hidden"
	}
	return "public"
}

// officialTokenRead 是一个已存令牌对官网更新来源仓库的读取结论。
type officialTokenRead struct {
	Label string `json:"label"`
	// State：yes 能读；no 读不到；unknown 仓库公开时无法确认（细粒度令牌）。
	State string `json:"state"`
	Note  string `json:"note"`
}

// officialRepoTokenReads 逐个检查存储管理等处已存的 GitHub 令牌改私有后能不能读官网更新来源仓库。
// 仓库已私有：直接带令牌读一次。仓库仍公开：经典令牌看 X-OAuth-Scopes 有没有 repo；细粒度令牌公开时读不出结论。
func officialRepoTokenReads(ctx context.Context, owner, repo string, public bool) []officialTokenRead {
	reads := make([]officialTokenRead, 0)
	for _, source := range productUpdateTokenSources() {
		if source.Token == "" {
			continue
		}
		read := officialTokenRead{Label: source.Label}
		if !public {
			status, _, _, err := githubRepoCall(ctx, productUpdateGitHubAPI, http.MethodGet, repoPath(owner, repo), source.Token, nil)
			switch {
			case err != nil:
				read.State, read.Note = "unknown", "暂时连不上代码托管站"
			case status == http.StatusOK:
				read.State, read.Note = "yes", "可以读取"
			case status == http.StatusUnauthorized:
				read.State, read.Note = "no", "令牌无效或已过期"
			default:
				read.State, read.Note = "no", "读不到这个仓库：细粒度令牌要在 Repository access 里选中它并给 Contents 只读，经典令牌要勾选 repo"
			}
			reads = append(reads, read)
			continue
		}
		status, header, _, err := githubRepoCall(ctx, productUpdateGitHubAPI, http.MethodGet, "/user", source.Token, nil)
		switch {
		case err != nil:
			read.State, read.Note = "unknown", "暂时连不上代码托管站"
		case status == http.StatusUnauthorized:
			read.State, read.Note = "no", "令牌无效或已过期"
		case header.Get("X-OAuth-Scopes") != "" || strings.HasPrefix(source.Token, "ghp_"):
			if scopeListHas(header.Get("X-OAuth-Scopes"), "repo") {
				read.State, read.Note = "yes", "经典令牌带 repo 权限，改私有后能读"
			} else {
				read.State, read.Note = "no", "经典令牌没有 repo 权限，改私有后读不到；要给它勾选 repo"
			}
		default:
			read.State, read.Note = "unknown", "细粒度令牌：仓库公开时无法确认。请确认它在 Repository access 里选中了 "+owner+"/"+repo+"，改私有后存储检查会给出确定结果"
		}
		reads = append(reads, read)
	}
	return reads
}

func scopeListHas(scopes, want string) bool {
	for _, scope := range strings.Split(scopes, ",") {
		if strings.TrimSpace(scope) == want {
			return true
		}
	}
	return false
}

// officialUpdateHealthRow 是存储检查里「官网更新来源」那一行。只在官网出现；连不上代码托管站时不出这一行。
func officialUpdateHealthRow(ctx context.Context) (storageHealthRow, bool) {
	if !officialSite() {
		return storageHealthRow{}, false
	}
	owner, repo, err := officialUpdateRepository()
	if err != nil {
		return storageHealthRow{}, false
	}
	full := owner + "/" + repo
	row := storageHealthRow{LocationName: "官网更新来源", Target: "官网更新来源"}
	switch officialRepoVisibility(ctx, owner, repo) {
	case "public":
		// 公开不算故障：灰色提醒，不发通知。
		row.Level = "notice"
		row.Message = "官网更新来源 " + full + " 目前是公开仓库。1.7.0 及更早的客户站直接读它，改私有前要先按步骤准备。"
		return row.withMakePrivate(packageStorageGitHub, owner, repo), true
	case "hidden":
		for _, read := range officialRepoTokenReads(ctx, owner, repo, false) {
			if read.State == "yes" {
				row.Level = "ok"
				row.Message = "官网更新来源 " + full + " 是私有仓库，用" + read.Label + "可以读取。"
				return row, true
			}
		}
		row.Level = "problem"
		row.Message = "官网更新来源 " + full + " 读不到：存储管理里的令牌都没有它的读取权限，官网在线更新会失败。请给其中一个令牌加上该仓库。"
		return row, true
	}
	return storageHealthRow{}, false
}

type makePrivateRequest struct {
	Kind    string `json:"kind"`
	Owner   string `json:"owner"`
	Repo    string `json:"repo"`
	Confirm string `json:"confirm"`
}

func (req *makePrivateRequest) normalize() error {
	req.Kind = strings.TrimSpace(req.Kind)
	req.Owner = strings.TrimSpace(req.Owner)
	req.Repo = strings.TrimSpace(req.Repo)
	if req.Kind != packageStorageGitHub && req.Kind != packageStorageGitee {
		return errors.New("只支持 GitHub 和 Gitee 仓库")
	}
	if !sourceReleaseRepoPattern.MatchString(req.Owner) || !sourceReleaseRepoPattern.MatchString(req.Repo) {
		return errors.New("所有者或仓库名不合法")
	}
	return nil
}

// makePrivateToken 找改可见性要用的令牌：先找指向这个仓库的存储，再找同类存储；GitHub 最后用仓库绑定用的令牌。
func makePrivateToken(kind, owner, repo string) (string, string, bool) {
	blob, err := loadStorageBlob()
	if err == nil {
		var fallbackLabel, fallbackToken string
		for _, loc := range enabledStorageLocations(blob.Locations) {
			if loc.Kind != kind {
				continue
			}
			secret, openErr := locationSecret(loc)
			if openErr != nil || strings.TrimSpace(secret) == "" {
				continue
			}
			if strings.EqualFold(loc.Owner, owner) && strings.EqualFold(loc.Repo, repo) {
				return "存储「" + loc.Name + "」的令牌", secret, true
			}
			if fallbackToken == "" {
				fallbackLabel, fallbackToken = "存储「"+loc.Name+"」的令牌", secret
			}
		}
		if fallbackToken != "" {
			return fallbackLabel, fallbackToken, true
		}
	}
	if kind == packageStorageGitHub {
		if loc, token, ok := primaryGitHubBindingLocation(); ok {
			return loc.Name + "的令牌", token, true
		}
	}
	return "", "", false
}

// makePrivateConsequences 是弹框里说明的后果。
func makePrivateConsequences(kind string) []string {
	site := "GitHub"
	if kind == packageStorageGitee {
		site = "Gitee"
	}
	return []string{
		"仓库里的安装包和文件只有带令牌才能看到，别人打开链接会显示找不到。",
		"本站上传、导入和买家下载照常，本站一直用存储里的令牌读写。",
		"关注、收藏这个仓库的人会失去访问；" + site + " 上别人从它派生的公开副本会脱离，不会跟着变私有。",
		"以后需要可以在 " + site + " 仓库设置里改回公开。",
	}
}

func officialPrivateSteps(owner, repo string) []string {
	full := owner + "/" + repo
	return []string{
		"先确认已经没有 1.7.0 及更早的客户站：它们直接读 " + full + " 的发布，改私有后就收不到更新。",
		"看下面的令牌检查，确保至少一个令牌改私有后能读；读不了就先给它加上这个仓库。",
		"到 GitHub 仓库设置里把可见性改成私有。",
		"回到这里点「立即检查」，「官网更新来源」那行应写明用哪个令牌可以读取；再到「在线更新」检查一次更新。",
	}
}

// AdminStorageMakePrivateCheck 打开「改为私有」弹框前调用：说明后果、要用的令牌；官网更新来源仓库只给步骤和令牌检查。
func AdminStorageMakePrivateCheck(c *gin.Context) {
	var req makePrivateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if err := req.normalize(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	full := req.Owner + "/" + req.Repo
	if req.Kind == packageStorageGitHub && isOfficialUpdateRepo(req.Owner, req.Repo) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		public := officialRepoVisibility(ctx, req.Owner, req.Repo) != "hidden"
		reads := officialRepoTokenReads(ctx, req.Owner, req.Repo, public)
		readable := false
		for _, read := range reads {
			if read.State == "yes" {
				readable = true
			}
		}
		summary := "已有令牌改私有后能读，官网在线更新不受影响。"
		if !readable {
			summary = "还没有确认能读的令牌。改私有前请给存储管理里的 GitHub 令牌加上 " + full + "（细粒度令牌在 Repository access 里选中它，经典令牌勾选 repo），否则官网在线更新会失败。"
		}
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
			"kind": req.Kind, "owner": req.Owner, "repo": req.Repo, "official": true, "allowed": false,
			"reason":   "这是官网自更新的来源仓库，1.7.0 及更早的客户站也直接读它，不能一键改。请按 " + storagePrivateDoc + " 的步骤手动改。",
			"steps":    officialPrivateSteps(req.Owner, req.Repo),
			"tokens":   reads,
			"readable": readable,
			"summary":  summary,
		}})
		return
	}
	label, _, ok := makePrivateToken(req.Kind, req.Owner, req.Repo)
	data := gin.H{
		"kind": req.Kind, "owner": req.Owner, "repo": req.Repo, "official": false, "allowed": ok,
		"tokenLabel": label, "consequences": makePrivateConsequences(req.Kind),
	}
	if !ok {
		data["reason"] = "存储管理里没有可用的令牌，先添加该仓库的存储再来改。"
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": data})
}

// AdminStorageMakePrivate 把仓库改为私有：要输入完整仓库名确认，写操作日志，改完自动重新检查。
func AdminStorageMakePrivate(c *gin.Context) {
	var req makePrivateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if err := req.normalize(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	full := req.Owner + "/" + req.Repo
	if strings.TrimSpace(req.Confirm) != full {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请输入完整仓库名 " + full + " 确认"})
		return
	}
	if req.Kind == packageStorageGitHub && isOfficialUpdateRepo(req.Owner, req.Repo) {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "这是官网自更新的来源仓库，不能一键改。请按 " + storagePrivateDoc + " 的步骤手动改。"})
		return
	}
	label, token, ok := makePrivateToken(req.Kind, req.Owner, req.Repo)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "存储管理里没有可用的令牌，先添加该仓库的存储再来改。"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	if err := setRepoPrivate(ctx, req.Kind, req.Owner, req.Repo, token); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	logRepoMadePrivate(c, req.Kind, full, label)
	checkStorageLocations(ctx)
	blob, _ := loadStorageBlob()
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": full + " 已改为私有，已重新检查", "data": gin.H{
		"checkedAt": blob.CheckedAt,
		"list":      blob.Health,
	}})
}

// setRepoPrivate 调代码托管站接口改可见性，失败时用大白话说缺什么权限。
func setRepoPrivate(ctx context.Context, kind, owner, repo, token string) error {
	if kind == packageStorageGitee {
		body := map[string]any{"access_token": token, "name": repo, "private": true}
		status, payload, err := sourceReleaseJSON(ctx, http.MethodPatch, strings.TrimRight(sourceGiteeAPIBase, "/")+repoPath(owner, repo), map[string]string{"Content-Type": "application/json"}, body)
		if err != nil {
			return errors.New("连不上 Gitee，请稍后再试")
		}
		switch {
		case status >= 200 && status < 300:
			var result struct {
				Private bool `json:"private"`
			}
			if json.Unmarshal(payload, &result) == nil && !result.Private {
				return errors.New("Gitee 没有把仓库改成私有，请到仓库设置里手动改")
			}
			return nil
		case status == http.StatusUnauthorized:
			return errors.New("Gitee 令牌无效或已过期")
		case status == http.StatusForbidden:
			return errors.New("Gitee 令牌没有修改仓库设置的权限：要用仓库管理员的账号生成令牌，并勾选 projects 权限")
		case status == http.StatusNotFound:
			return errors.New("Gitee 令牌看不到这个仓库：确认令牌属于仓库管理员，并勾选 projects 权限")
		}
		return fmt.Errorf("Gitee 拒绝了修改%s", remoteMessage(payload))
	}
	status, _, payload, err := githubRepoCall(ctx, sourceGitHubAPIBase, http.MethodPatch, repoPath(owner, repo), token, map[string]any{"private": true})
	if err != nil {
		return errors.New("连不上 GitHub，请稍后再试")
	}
	switch {
	case status >= 200 && status < 300:
		var result struct {
			Private bool `json:"private"`
		}
		if json.Unmarshal(payload, &result) == nil && !result.Private {
			return errors.New("GitHub 没有把仓库改成私有，请到仓库设置里手动改")
		}
		return nil
	case status == http.StatusUnauthorized:
		return errors.New("GitHub 令牌无效或已过期")
	case status == http.StatusForbidden:
		return errors.New("令牌没有修改仓库设置的权限：细粒度令牌要给这个仓库加上 Administration 读写，经典令牌要勾选 repo")
	case status == http.StatusNotFound:
		return errors.New("令牌改不了这个仓库：细粒度令牌要在 Repository access 里选中它并给 Administration 读写，经典令牌要勾选 repo，而且账号要是仓库管理员")
	}
	return fmt.Errorf("GitHub 拒绝了修改%s", remoteMessage(payload))
}

func remoteMessage(payload []byte) string {
	var body struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(payload, &body) == nil && strings.TrimSpace(body.Message) != "" {
		return "：" + truncateText(strings.TrimSpace(body.Message), 120)
	}
	return "，请稍后再试"
}

func logRepoMadePrivate(c *gin.Context, kind, full, label string) {
	db, err := config.DB()
	if err != nil {
		log.Printf("storage make private log skipped: %v", err)
		return
	}
	detail, _ := json.Marshal(map[string]string{"kind": kind, "repo": full, "token": label})
	if _, err := db.Exec(`
		INSERT INTO operation_logs (operator_type, operator_id, action, target_type, target_id, detail, ip)
		VALUES ('admin', ?, 'storage_repo_make_private', 'storage_repo', NULL, ?, ?)
	`, c.GetUint("user_id"), string(detail), c.ClientIP()); err != nil {
		log.Printf("storage make private log failed: %v", err)
	}
}
