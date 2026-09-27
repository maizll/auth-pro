// 用 GitHub 令牌识别所有者，确认仓库是私有的；仓库不存在时创建私有仓库，已经公开则拒绝。

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	githubPaidRepoPrivate = "private"
	githubPaidRepoPublic  = "public"
	githubPaidRepoMissing = "missing"
)

type githubPaidOwnerChoice struct {
	Login string `json:"login"`
	Kind  string `json:"kind"`
}

type githubPaidIdentity struct {
	Login  string
	Owners []githubPaidOwnerChoice
}

func githubPaidConnectedText(owner, repo string) string {
	return fmt.Sprintf("已连接：%s/%s（私有）", owner, repo)
}

func githubPaidPermissionError() error {
	return errors.New(githubPaidPermissionText + " " + githubPaidTokenCreateURL)
}

func validGitHubPaidName(value string) bool {
	return value != "" && sourceReleaseRepoPattern.MatchString(value)
}

func resolveGitHubPaidPlainToken(raw string) (token string, fromBody bool, err error) {
	token = strings.TrimSpace(raw)
	if token == "" || isMaskedSourceToken(token) {
		token, err = loadGitHubPaidToken()
		return token, false, err
	}
	return token, true, nil
}

func githubPaidAPI(path string) string {
	return strings.TrimRight(sourceGitHubAPIBase, "/") + path
}

func fetchGitHubPaidIdentity(ctx context.Context, token string) (githubPaidIdentity, error) {
	status, payload, err := githubPaidJSON(ctx, http.MethodGet, githubPaidAPI("/user"), token, nil)
	if err != nil {
		return githubPaidIdentity{}, errors.New("无法连接 GitHub，请稍后再试")
	}
	if status == http.StatusUnauthorized {
		return githubPaidIdentity{}, errGitHubPaidTokenInvalid
	}
	if status < 200 || status >= 300 {
		return githubPaidIdentity{}, errors.New("GitHub 令牌无法使用，请确认 Contents 为读写")
	}
	var user struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(payload, &user); err != nil || strings.TrimSpace(user.Login) == "" {
		return githubPaidIdentity{}, errors.New("无法识别令牌对应的账号")
	}
	login := strings.TrimSpace(user.Login)
	identity := githubPaidIdentity{
		Login:  login,
		Owners: []githubPaidOwnerChoice{{Login: login, Kind: "user"}},
	}
	orgStatus, orgPayload, orgErr := githubPaidJSON(ctx, http.MethodGet, githubPaidAPI("/user/orgs"), token, nil)
	if orgErr != nil || orgStatus == http.StatusUnauthorized || orgStatus < 200 || orgStatus >= 300 {
		return identity, nil
	}
	var orgs []struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(orgPayload, &orgs); err != nil {
		return identity, nil
	}
	for _, org := range orgs {
		name := strings.TrimSpace(org.Login)
		if name == "" || strings.EqualFold(name, login) {
			continue
		}
		identity.Owners = append(identity.Owners, githubPaidOwnerChoice{Login: name, Kind: "org"})
	}
	return identity, nil
}

func lookupGitHubPaidRepoState(ctx context.Context, token, owner, repo string) (string, error) {
	if !validGitHubPaidName(owner) || !validGitHubPaidName(repo) {
		return "", errors.New("请填写私有仓库的所有者和仓库名")
	}
	rawURL := githubPaidAPI("/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo))
	status, payload, err := githubPaidJSON(ctx, http.MethodGet, rawURL, token, nil)
	if err != nil {
		return "", errors.New("无法连接 GitHub，请稍后再试")
	}
	if status == http.StatusUnauthorized {
		return "", errGitHubPaidTokenInvalid
	}
	if status == http.StatusNotFound {
		return githubPaidRepoMissing, nil
	}
	if status == http.StatusForbidden {
		return "", errors.New("找不到该私有仓库，或令牌没有 Contents 读写权限")
	}
	if status < 200 || status >= 300 {
		return "", errors.New("读取 GitHub 仓库失败，请稍后再试")
	}
	var body struct {
		Private bool `json:"private"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return "", errors.New("读取 GitHub 仓库失败，请稍后再试")
	}
	if body.Private {
		return githubPaidRepoPrivate, nil
	}
	return githubPaidRepoPublic, nil
}

func createGitHubPaidRepo(ctx context.Context, token, owner, repo string, identity githubPaidIdentity) error {
	path := "/user/repos"
	if owner != identity.Login {
		org := false
		for _, choice := range identity.Owners {
			if choice.Kind == "org" && choice.Login == owner {
				org = true
				break
			}
		}
		if !org {
			return errors.New("所有者必须是令牌对应的账号或其所属组织")
		}
		path = "/orgs/" + url.PathEscape(owner) + "/repos"
	}
	status, _, err := githubPaidJSON(ctx, http.MethodPost, githubPaidAPI(path), token, map[string]any{
		"name":      repo,
		"private":   true,
		"auto_init": true,
	})
	if err != nil {
		return errors.New("无法连接 GitHub，请稍后再试")
	}
	if status == http.StatusUnauthorized {
		return errGitHubPaidTokenInvalid
	}
	if status == http.StatusForbidden {
		return githubPaidPermissionError()
	}
	if status == http.StatusUnprocessableEntity {
		return nil
	}
	if status < 200 || status >= 300 {
		return errors.New("创建私有仓库失败，请稍后再试")
	}
	return nil
}

func ensureGitHubPaidPrivateRepo(ctx context.Context, token, owner, repo string, known *githubPaidIdentity) error {
	state, err := lookupGitHubPaidRepoState(ctx, token, owner, repo)
	if err != nil {
		return err
	}
	switch state {
	case githubPaidRepoPrivate:
		return nil
	case githubPaidRepoPublic:
		return errors.New(githubPaidPublicRepoText)
	}
	identity := githubPaidIdentity{}
	if known != nil {
		identity = *known
	}
	if identity.Login == "" {
		identity, err = fetchGitHubPaidIdentity(ctx, token)
		if err != nil {
			return err
		}
	}
	if err := createGitHubPaidRepo(ctx, token, owner, repo, identity); err != nil {
		return err
	}
	state, err = lookupGitHubPaidRepoState(ctx, token, owner, repo)
	if err != nil {
		return err
	}
	switch state {
	case githubPaidRepoPrivate:
		return nil
	case githubPaidRepoPublic:
		return errors.New(githubPaidPublicRepoText)
	default:
		return errors.New("创建私有仓库失败，请稍后再试")
	}
}

func githubPaidConnectData(identity githubPaidIdentity, owner, repo, status string, connected bool) gin.H {
	view := githubPaidSettingsView()
	if owner != "" {
		view["owner"] = owner
	}
	if repo != "" {
		view["repo"] = repo
	}
	owners := identity.Owners
	if owners == nil {
		owners = []githubPaidOwnerChoice{}
	}
	view["login"] = identity.Login
	view["owners"] = owners
	view["repoStatus"] = status
	view["private"] = status == githubPaidRepoPrivate
	view["connected"] = connected
	if connected {
		view["reminder"] = ""
	}
	if status == githubPaidRepoMissing {
		view["hint"] = githubPaidRepoMissingText
	}
	return view
}

func githubPaidRequestTarget(c *gin.Context, withIdentity bool) (token, owner, repo string, identity githubPaidIdentity, ok bool) {
	var body struct {
		Token string `json:"token"`
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	}
	_ = c.ShouldBindJSON(&body)
	var err error
	token, _, err = resolveGitHubPaidPlainToken(body.Token)
	if err != nil {
		msg := err.Error()
		if errors.Is(err, errGitHubPaidTokenMissing) {
			msg = "请粘贴 GitHub 令牌"
		}
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": msg})
		return "", "", "", githubPaidIdentity{}, false
	}
	if withIdentity {
		identity, err = fetchGitHubPaidIdentity(c.Request.Context(), token)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
			return "", "", "", githubPaidIdentity{}, false
		}
	}
	owner = strings.TrimSpace(body.Owner)
	repo = strings.TrimSpace(body.Repo)
	if owner == "" {
		owner = identity.Login
	}
	if owner == "" || repo == "" {
		savedOwner, savedRepo, _, _ := loadGitHubPaidRepo()
		if owner == "" {
			owner = savedOwner
		}
		if repo == "" {
			repo = savedRepo
		}
	}
	if repo == "" {
		repo = githubPaidDefaultRepo
	}
	if !validGitHubPaidName(owner) || !validGitHubPaidName(repo) {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请填写私有仓库的所有者和仓库名", "data": githubPaidConnectData(identity, owner, repo, "", false)})
		return "", "", "", identity, false
	}
	return token, owner, repo, identity, true
}
