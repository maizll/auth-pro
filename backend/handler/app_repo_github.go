// 用存储管理里已经保存的 GitHub 令牌，列出这个账号能访问的仓库。
// 只给管理员读。开发者路由不注册这个接口。令牌明文不进响应。

package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

const (
	gitHubRepoListPageSize = 100
	gitHubRepoListMaxPage  = 10
	gitHubRepoListCacheTTL = 45 * time.Second
	gitHubRepoTokenText    = "GitHub 令牌无效或已过期。请到存储管理更新令牌。"
	gitHubRepoMissingText  = "还没有可用的令牌。请到存储管理更新令牌。"
	gitHubRepoTimeoutText  = "连接超时，请稍后再试。"
	gitHubRepoEmptyText    = "没有可访问的仓库。"
)

var errGitHubRepoToken = errors.New("github repo token")

type gitHubRepoChoice struct {
	Repo      string    `json:"repo"`
	Private   bool      `json:"private"`
	UpdatedAt time.Time `json:"updatedAt"`
	BoundApp  string    `json:"boundApp,omitempty"`
}

type gitHubRepoCache struct {
	mu    sync.Mutex
	key   string
	at    time.Time
	items []gitHubRepoChoice
}

var accessibleGitHubRepoCache gitHubRepoCache

// AdminListGitHubRepos 列出令牌能访问的仓库，并标出已被其他应用占用的名字。
// 成功时 data.status 为 ok 或 empty。令牌缺失、过期是 token，网络失败是 timeout。
func AdminListGitHubRepos(c *gin.Context) {
	_, token, ok := primaryGitHubBindingLocation()
	if !ok || strings.TrimSpace(token) == "" {
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
			"status": "token", "message": gitHubRepoMissingText, "list": []gitHubRepoChoice{},
		}})
		return
	}
	items, err := listGitHubReposForToken(c.Request.Context(), token)
	if errors.Is(err, errGitHubRepoToken) {
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
			"status": "token", "message": gitHubRepoTokenText, "list": []gitHubRepoChoice{},
		}})
		return
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
			"status": "timeout", "message": gitHubRepoTimeoutText, "list": []gitHubRepoChoice{},
		}})
		return
	}
	currentApp, _ := strconv.ParseInt(c.Query("appId"), 10, 64)
	annotateGitHubRepoBindings(items, loadGitHubRepoBindings(), currentApp)
	query := strings.ToLower(strings.TrimSpace(c.Query("q")))
	if query != "" {
		filtered := items[:0:0]
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.Repo), query) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	status := "ok"
	message := ""
	if len(items) == 0 && query == "" {
		status = "empty"
		message = gitHubRepoEmptyText
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"status": status, "message": message, "list": items,
	}})
}

func listGitHubReposForToken(ctx context.Context, token string) ([]gitHubRepoChoice, error) {
	key := gitHubRepoCacheKey(token)
	if items, ok := accessibleGitHubRepoCache.get(key); ok {
		return items, nil
	}
	var all []gitHubRepoChoice
	for page := 1; page <= gitHubRepoListMaxPage; page++ {
		rawURL := githubPaidAPI("/user/repos?" + url.Values{
			"per_page":    {strconv.Itoa(gitHubRepoListPageSize)},
			"sort":        {"updated"},
			"direction":   {"desc"},
			"page":        {strconv.Itoa(page)},
			"affiliation": {"owner,collaborator,organization_member"},
		}.Encode())
		status, payload, err := githubPaidJSON(ctx, http.MethodGet, rawURL, token, nil)
		if err != nil {
			return nil, err
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return nil, errGitHubRepoToken
		}
		if status < 200 || status >= 300 {
			return nil, errors.New("list github repos")
		}
		var pageItems []struct {
			FullName  string    `json:"full_name"`
			Private   bool      `json:"private"`
			UpdatedAt time.Time `json:"updated_at"`
		}
		if err := json.Unmarshal(payload, &pageItems); err != nil {
			return nil, err
		}
		for _, item := range pageItems {
			repo := strings.Trim(strings.TrimSpace(item.FullName), "/")
			if repo == "" || strings.Count(repo, "/") != 1 {
				continue
			}
			all = append(all, gitHubRepoChoice{Repo: repo, Private: item.Private, UpdatedAt: item.UpdatedAt.UTC()})
		}
		if len(pageItems) < gitHubRepoListPageSize {
			break
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].UpdatedAt.After(all[j].UpdatedAt)
	})
	accessibleGitHubRepoCache.put(key, all)
	return append([]gitHubRepoChoice(nil), all...), nil
}

func gitHubRepoCacheKey(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func (c *gitHubRepoCache) get(key string) ([]gitHubRepoChoice, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.key != key || time.Since(c.at) > gitHubRepoListCacheTTL || c.items == nil {
		return nil, false
	}
	return append([]gitHubRepoChoice(nil), c.items...), true
}

func (c *gitHubRepoCache) put(key string, items []gitHubRepoChoice) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.key = key
	c.at = time.Now()
	c.items = append([]gitHubRepoChoice(nil), items...)
}

type gitHubRepoBinding struct {
	appID int64
	name  string
}

func loadGitHubRepoBindings() map[string]gitHubRepoBinding {
	out := map[string]gitHubRepoBinding{}
	db, err := config.DB()
	if err != nil || db == nil {
		return out
	}
	if err := ensureAppRepoSchema(db); err != nil {
		return out
	}
	rows, err := db.Query(`SELECT b.owner, b.repo, b.app_id, a.app_name
		FROM app_repo_bindings b INNER JOIN apps a ON a.id = b.app_id`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var owner, repo, name string
		var appID int64
		if err := rows.Scan(&owner, &repo, &appID, &name); err != nil {
			continue
		}
		key := strings.ToLower(strings.Trim(owner, "/") + "/" + strings.Trim(repo, "/"))
		out[key] = gitHubRepoBinding{appID: appID, name: name}
	}
	return out
}

func annotateGitHubRepoBindings(items []gitHubRepoChoice, bound map[string]gitHubRepoBinding, currentApp int64) {
	for i := range items {
		item, ok := bound[strings.ToLower(items[i].Repo)]
		if !ok || item.name == "" || item.appID == currentApp {
			continue
		}
		items[i].BoundApp = item.name
	}
}
