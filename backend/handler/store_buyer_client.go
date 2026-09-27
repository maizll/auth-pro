// 买家站访问源站的 HTTP 客户端：给请求签名、刷新快照，并把付费包装进本机插件或模板目录。

package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"auto_pro/config"
	"auto_pro/softwaresource"

	"github.com/gin-gonic/gin"
)

func loadBuyerInstallID() string {
	path := filepath.Join(config.GetDataDir(), "store", "install-id")
	if payload, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(payload)); id != "" {
			return id
		}
	}
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	id := hex.EncodeToString(buf)
	_ = os.MkdirAll(filepath.Dir(path), 0750)
	_ = os.WriteFile(path, []byte(id), 0600)
	return id
}

func normalizeBuyerSource(base string) (string, error) {
	parsed, err := parseHTTPSBase(base)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func defaultBuyerSourceBase() (string, error) {
	return normalizeBuyerSource(buyerSourceDefault)
}

// buyerSourceBase 在正式程序里固定为 https://auth.maizll.com。
// 同包测试文件可以替换这个函数；发布构建不包含测试文件，也没有环境变量或配置文件入口。
var buyerSourceBase = defaultBuyerSourceBase

func defaultSourceHTTPClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Second, CheckRedirect: refuseSourceRedirect}
}

// newSourceHTTPClient 正式程序使用默认客户端。测试文件可以换成 httptest 客户端。
var newSourceHTTPClient = defaultSourceHTTPClient

func refuseSourceRedirect(*http.Request, []*http.Request) error {
	return errStoreNoRedirect
}

// errSourcePathMissing 表示源站没有这个地址。老版本没有商店注册接口时，买家改走官网注册。
var errSourcePathMissing = errors.New("源站还没有这个接口")

// doSourceRequest 用绑定账号的同一个 HTTP 客户端访问源站。不要把请求体写进日志，注册密码会经过这里。
func doSourceRequest(method, path string, body any, headers map[string]string) (int, []byte, error) {
	base, err := buyerSourceBase()
	if err != nil {
		return 0, nil, err
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, base+path, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := newSourceHTTPClient().Do(req)
	if err != nil {
		return 0, nil, errors.New("无法连接源站")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, raw, nil
}

func callSourceJSON(method, path string, body any, headers map[string]string, out any) error {
	_, raw, err := doSourceRequest(method, path, body, headers)
	if err != nil {
		return err
	}
	var envelope map[string]any
	if json.Unmarshal(raw, &envelope) != nil {
		return errors.New("源站响应无法解析")
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return err
		}
	}
	if err := sourceErrorFromEnvelope(envelope); err != nil {
		return err
	}
	return nil
}

// exchangeSource 读取源站 JSON。HTTP 404 当成接口不存在，方便注册在老源站上改走官网接口。
func exchangeSource(method, path string, body any) (map[string]any, error) {
	status, raw, err := doSourceRequest(method, path, body, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, errSourcePathMissing
	}
	var envelope map[string]any
	if json.Unmarshal(raw, &envelope) != nil {
		return nil, errors.New("源站响应无法解析")
	}
	if err := sourceErrorFromEnvelope(envelope); err != nil {
		return envelope, err
	}
	return envelope, nil
}

// forwardBuyerRegister 先调商店注册接口。老源站还没有这个地址时，再调官网同一套注册接口。
func forwardBuyerRegister(storePath, legacyPath string, body map[string]any) (map[string]any, error) {
	envelope, err := exchangeSource(http.MethodPost, storePath, body)
	if errors.Is(err, errSourcePathMissing) {
		envelope, err = exchangeSource(http.MethodPost, legacyPath, body)
	}
	return envelope, err
}

// plainRegisterMessage 把源站或网络错误改成用户能看懂的话。secret 若出现在原文里会被抹掉，避免密码漏到提示或日志。
func plainRegisterMessage(msg, secret string) string {
	msg = strings.TrimSpace(msg)
	if secret != "" {
		msg = strings.TrimSpace(strings.ReplaceAll(msg, secret, ""))
	}
	switch {
	case msg == "":
		return "注册没有完成，请稍后再试"
	case strings.Contains(msg, "无法连接源站"), strings.Contains(msg, "网络"):
		return "网络不通，请稍后再试"
	case strings.Contains(msg, "该邮箱已注册"):
		return "这个邮箱已经注册过了"
	case strings.Contains(msg, "该手机号"):
		return "这个手机号已经注册过了"
	case strings.Contains(msg, "验证码错误"):
		return "验证码错误，请核对后再试"
	case strings.Contains(msg, "验证码无效"), strings.Contains(msg, "已过期"):
		return "验证码无效或已过期，请重新获取"
	case strings.Contains(msg, "注册已关闭"):
		return "源站暂时关闭了注册，请联系管理员"
	case strings.Contains(msg, "参数错误"):
		return "请检查邮箱、验证码和密码（至少 6 位）"
	case strings.Contains(msg, "无法解析"):
		return "注册服务器没有正常返回，请稍后再试"
	case strings.Contains(msg, "还没有这个接口"):
		return "源站暂时不能注册，请稍后再试"
	case strings.Contains(msg, "邮件服务未配置"):
		return "验证码发不出去，请联系管理员检查邮箱设置"
	default:
		return msg
	}
}

// sourceResponseError 是源站业务拒绝。吊销只看 Reason / Revoked，不看中文 Msg。
type sourceResponseError struct {
	Code    int
	Msg     string
	Reason  string
	Revoked bool
}

// Error 返回源站信封里的 msg。空消息时给一句固定原因，避免页面看到空白错误。
func (e *sourceResponseError) Error() string {
	if e == nil || strings.TrimSpace(e.Msg) == "" {
		return "源站拒绝了请求"
	}
	return e.Msg
}

// buyerSnapshotTerminal 判断源站是不是明确说这条绑定不能再用。
// 以 revoked 字段和固定原因码为准，不用中文文案，避免源站改措辞后买家还留着已吊销的商业版。
func buyerSnapshotTerminal(reason string, revoked bool) bool {
	if revoked {
		return true
	}
	switch strings.TrimSpace(reason) {
	case "license_deleted", "binding_deleted", "binding_revoked", "binding_expired", "license_not_found", "license_revoked", "license_expired", "token_invalid":
		return true
	default:
		return false
	}
}

func envelopeStatus(envelope map[string]any) (int, string) {
	switch v := envelope["code"].(type) {
	case float64:
		return int(v), ""
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return 0, ""
		}
		if n, err := strconv.Atoi(text); err == nil {
			return n, ""
		}
		return 0, text
	default:
		return 0, ""
	}
}

func sourceEnvelopeFlags(envelope map[string]any) (string, bool) {
	revoked := false
	if v, ok := envelope["revoked"].(bool); ok && v {
		revoked = true
	}
	reason := ""
	data, _ := envelope["data"].(map[string]any)
	if data != nil {
		if text, ok := data["reason"].(string); ok {
			reason = strings.TrimSpace(text)
		}
		if v, ok := data["revoked"].(bool); ok && v {
			revoked = true
		}
	}
	return reason, revoked
}

func sourceErrorFromEnvelope(envelope map[string]any) error {
	if envelope == nil {
		return &sourceResponseError{Msg: "源站拒绝了请求"}
	}
	code, codedReason := envelopeStatus(envelope)
	if code == 200 && codedReason == "" {
		return nil
	}
	msg, _ := envelope["msg"].(string)
	reason, revoked := sourceEnvelopeFlags(envelope)
	if reason == "" {
		reason = codedReason
	}
	if buyerSnapshotTerminal(reason, revoked) || buyerSnapshotTerminal(codedReason, false) {
		revoked = true
	}
	if strings.TrimSpace(msg) == "" {
		msg = "源站拒绝了请求"
	}
	return &sourceResponseError{Code: code, Msg: msg, Reason: reason, Revoked: revoked}
}

const buyerBindingCheckPath = "/api/v1/store/binding"

// reconcileBuyerBinding 用本地绑定令牌问源站这条绑定还在不在。
// 没有本地令牌时不访问源站。源站明确说绑定已删除或令牌失效时，签名请求会清掉本地记录。
// 网络失败或源站其它拒绝不算失效，调用方应继续用本地快照。
func reconcileBuyerBinding() (invalidReason string, verified bool) {
	if _, _, err := openBuyerBindingSecret(); err != nil {
		return "", false
	}
	err := signedSourceJSON(http.MethodGet, buyerBindingCheckPath, nil, nil)
	if err == nil {
		return "", true
	}
	if !buyerRefreshFailureRevoked(err) {
		return "", false
	}
	reason := "revoked"
	var src *sourceResponseError
	if errors.As(err, &src) && strings.TrimSpace(src.Reason) != "" {
		reason = strings.TrimSpace(src.Reason)
	}
	return reason, false
}

func buyerRefreshIsNetwork(err error) bool {
	if err == nil {
		return false
	}
	var src *sourceResponseError
	if errors.As(err, &src) {
		return false
	}
	switch err.Error() {
	case "无法连接源站", "源站响应无法解析":
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

// applyBuyerRefreshFailure 把刷新失败写回本地快照。
// 授权删除、绑定删除和吊销立刻标记 ExplicitRevoked，不进宽限。
// 只有连不上源站或响应无法解析才把 LastRefreshOK 置假，从而进入离线宽限。
// 返回的 bool 表示要不要落盘。
func applyBuyerRefreshFailure(state buyerSnapshotState, err error) (buyerSnapshotState, bool) {
	var src *sourceResponseError
	if errors.As(err, &src) && buyerSnapshotTerminal(src.Reason, src.Revoked) {
		state.LastRefreshOK = false
		state.ExplicitRevoked = true
		if strings.TrimSpace(src.Reason) != "" {
			state.RevokeReason = src.Reason
		} else {
			state.RevokeReason = "revoked"
		}
		return state, true
	}
	if buyerRefreshIsNetwork(err) {
		state.LastRefreshOK = false
		return state, true
	}
	return state, false
}

func signedSourceJSON(method, path string, body any, out any) error {
	return signedSourceJSONPath(method, path, path, body, out)
}

func signedSourceJSONPath(method, signPath, requestPath string, body any, out any) error {
	secret, bindingID, err := openBuyerBindingSecret()
	if err != nil {
		return err
	}
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	nonce := randomHex(16)
	ts := time.Now().Unix()
	headers := map[string]string{
		"X-Store-Binding":   bindingID,
		"X-Store-Timestamp": strconv.FormatInt(ts, 10),
		"X-Store-Nonce":     nonce,
		"X-Store-Signature": storeRequestSignature(secret, method, signPath, ts, nonce, payload),
	}
	var send any
	if len(payload) > 0 {
		send = json.RawMessage(payload)
	}
	err = callSourceJSON(method, requestPath, send, headers, out)
	if err != nil {
		clearBuyerBindingIfTerminal(err)
	}
	return err
}

// clearBuyerLocalBinding 去掉本机绑定凭据和快照，效果同退出绑定。
// 源站地址、站点地址和信任代理仍留在系统配置里。
func clearBuyerLocalBinding() {
	_ = os.Remove(buyerSnapshotPath())
	_ = os.Remove(filepath.Join(config.GetDataDir(), "store", "binding.key"))
}

func clearBuyerBindingIfTerminal(err error) {
	var src *sourceResponseError
	if !errors.As(err, &src) || !buyerSnapshotTerminal(src.Reason, src.Revoked) {
		return
	}
	clearBuyerLocalBinding()
}

func openBuyerBindingSecret() ([]byte, string, error) {
	key, err := loadOrCreateStoreFileKey("master.key")
	if err != nil {
		return nil, "", err
	}
	blob, err := os.ReadFile(filepath.Join(config.GetDataDir(), "store", "binding.key"))
	if err != nil {
		return nil, "", errors.New("尚未绑定源站账号")
	}
	plain, err := openStoreSecret(key, blob)
	if err != nil {
		return nil, "", err
	}
	parts := bytes.SplitN(plain, []byte("\n"), 2)
	if len(parts) != 2 {
		return nil, "", errors.New("绑定秘密无效")
	}
	secret, err := hex.DecodeString(string(parts[1]))
	if err != nil {
		return nil, "", errors.New("绑定秘密无效")
	}
	return secret, string(parts[0]), nil
}

// buyerAccountLine 把源站返回的账号整理成「名字\n身份」。没有名字时返回空，避免刷新把旧快照里的账号清掉。
func buyerAccountLine(raw any) string {
	account, _ := raw.(map[string]any)
	if account == nil {
		return ""
	}
	name, _ := account["name"].(string)
	role, _ := account["role"].(string)
	name = strings.TrimSpace(name)
	role = strings.TrimSpace(role)
	if name == "" {
		return ""
	}
	return name + "\n" + role
}

// persistBuyerBind 把源站确认结果写成绑定凭据和快照。
// 源站没带回账号名或身份时，用这次登录提交的账号和身份补上，避免老的确认响应把快照里的名字写空。
func persistBuyerBind(envelope map[string]any, fallbackName, fallbackRole string) error {
	data, _ := envelope["data"].(map[string]any)
	bindingID, _ := data["bindingId"].(string)
	secretHex, _ := data["bindingSecret"].(string)
	if bindingID == "" || secretHex == "" {
		return errors.New("源站没有返回绑定")
	}
	key, err := loadOrCreateStoreFileKey("master.key")
	if err != nil {
		return err
	}
	sealed, err := sealStoreSecret(key, []byte(bindingID+"\n"+secretHex))
	if err != nil {
		return err
	}
	path := filepath.Join(config.GetDataDir(), "store", "binding.key")
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	if err := os.WriteFile(path, sealed, 0600); err != nil {
		return err
	}
	if snap, ok := data["snapshot"].(map[string]any); ok {
		account, _ := data["account"].(map[string]any)
		name, _ := account["name"].(string)
		role, _ := account["role"].(string)
		name = strings.TrimSpace(name)
		role = strings.TrimSpace(role)
		if name == "" {
			name = strings.TrimSpace(fallbackName)
		}
		if role == "" {
			role = strings.TrimSpace(fallbackRole)
		}
		accountLine := ""
		if name != "" {
			accountLine = name + "\n" + role
		}
		return saveSnapshotMap(snap, true, false, accountLine)
	}
	return nil
}

func saveSnapshotMap(raw map[string]any, refreshOK, revoked bool, accountLine string) error {
	payload, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	var snapshot storeSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return err
	}
	if !verifyStoreSnapshot(snapshot) {
		return errors.New("快照签名无效")
	}
	state, _ := loadBuyerSnapshot()
	state.Snapshot = snapshot
	state.VerifiedAt = time.Now().Unix()
	state.LastRefreshOK = refreshOK
	state.ExplicitRevoked = revoked
	if snapshot.GraceDays <= 0 {
		snapshot.GraceDays = storeGraceDefaultDays
	}
	state.GraceUntil = time.Now().Add(time.Duration(snapshot.GraceDays) * 24 * time.Hour).Unix()
	state.BindingID = snapshot.BindingID
	state.LicenseNo = snapshot.LicenseNo
	if accountLine != "" {
		parts := strings.SplitN(accountLine, "\n", 2)
		state.AccountName = parts[0]
		if len(parts) == 2 {
			state.AccountRole = parts[1]
		}
	}
	if revoked {
		state.RevokeReason = "revoked"
	}
	return saveBuyerSnapshot(state)
}

func refreshBuyerSnapshot(ctx context.Context, domain string) error {
	_ = ctx
	var payload map[string]any
	path := "/api/v1/store/status"
	if domain != "" {
		path += "?domain=" + domain
	}
	signPath := "/api/v1/store/status"
	requestPath := signPath
	if domain != "" {
		requestPath += "?domain=" + domain
	}
	err := signedSourceJSONPath(http.MethodGet, signPath, requestPath, nil, &payload)
	if err != nil {
		// 授权或绑定已终止时，签名请求已经清掉快照，不要再写回一份吊销快照。
		if buyerRefreshFailureRevoked(err) {
			return err
		}
		state, ok := loadBuyerSnapshot()
		if ok {
			if next, save := applyBuyerRefreshFailure(state, err); save {
				_ = saveBuyerSnapshot(next)
			}
		}
		return err
	}
	data, _ := payload["data"].(map[string]any)
	snap, _ := data["snapshot"].(map[string]any)
	return saveSnapshotMap(snap, true, false, buyerAccountLine(data["account"]))
}

// StartStoreSnapshotRefresher 在后台定期向源站核对快照。
// 失败后退避，最长约 6 小时，避免源站故障时打满请求。明确吊销由刷新函数清本地状态。
func StartStoreSnapshotRefresher() {
	storeRefreshOnce.Do(func() {
		go func() {
			timer := time.NewTimer(time.Minute)
			defer timer.Stop()
			backoff := time.Duration(0)
			for {
				<-timer.C
				err := refreshBuyerSnapshot(context.Background(), "")
				if err != nil {
					if backoff == 0 {
						backoff = 5 * time.Minute
					} else if backoff < 6*time.Hour {
						backoff *= 2
					}
					timer.Reset(backoff)
				} else {
					backoff = 0
					timer.Reset(6*time.Hour + time.Duration(time.Now().Unix()%600)*time.Second)
				}
				retryInstallQueue()
			}
		}()
	})
}

func installPaidPackage(ctx context.Context, kind, id string) error {
	var ticket struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := signedSourceJSON(http.MethodPost, "/api/v1/store/download-ticket", map[string]any{"kind": kind, "id": id}, &ticket); err != nil {
		return err
	}
	if ticket.Data.URL == "" || !strings.HasPrefix(ticket.Data.URL, "https://") {
		return errors.New("下载地址无效")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ticket.Data.URL, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return errors.New("下载付费包失败")
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, pluginPackageMaxSize))
	if err != nil {
		return err
	}
	if len(payload) > 0 && payload[0] == '{' {
		var envelope struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if json.Unmarshal(payload, &envelope) == nil && envelope.Code != 0 && envelope.Code != 200 {
			if strings.TrimSpace(envelope.Msg) == "" {
				return errors.New("下载付费包失败")
			}
			return errors.New(envelope.Msg)
		}
	}
	if kind == "plugin" {
		if err := installPluginZIP(payload, pluginInfo{ID: id, Name: id, Category: "other"}); err != nil {
			return err
		}
		return writeEntitlementFile(filepath.Join(config.GetPluginDir(), id), kind, id)
	}
	sum := sha256.Sum256(payload)
	remote := softwaresource.Template{ID: id, Name: id, Version: "paid", SHA256: hex.EncodeToString(sum[:])}
	dir, err := installCatalogHomeTemplateZIP(payload, remote)
	if err != nil {
		return err
	}
	return writeEntitlementFile(dir, kind, id)
}

func writeEntitlementFile(dir, kind, id string) error {
	if dir == "" {
		return nil
	}
	payload, _ := json.Marshal(map[string]string{"kind": kind, "id": id, "source": "commercial"})
	return os.WriteFile(filepath.Join(dir, ".entitlement.json"), payload, 0600)
}

func enqueueInstall(kind, id, reason string) {
	storeInstallMu.Lock()
	defer storeInstallMu.Unlock()
	job := storeInstallJobs[kind+":"+id]
	job.Kind = kind
	job.ID = id
	job.Attempts++
	job.LastError = reason
	shift := job.Attempts
	if shift > 6 {
		shift = 6
	}
	delay := time.Duration(1<<shift) * time.Minute
	job.NextTry = time.Now().Add(delay)
	storeInstallJobs[kind+":"+id] = job
}

func retryInstallQueue() {
	storeInstallMu.Lock()
	jobs := make([]storeInstallJob, 0, len(storeInstallJobs))
	now := time.Now()
	for _, job := range storeInstallJobs {
		if !job.NextTry.After(now) {
			jobs = append(jobs, job)
		}
	}
	storeInstallMu.Unlock()
	for _, job := range jobs {
		if err := installPaidPackage(context.Background(), job.Kind, job.ID); err != nil {
			enqueueInstall(job.Kind, job.ID, err.Error())
			continue
		}
		storeInstallMu.Lock()
		delete(storeInstallJobs, job.Kind+":"+job.ID)
		storeInstallMu.Unlock()
	}
}

func panelOwnerID(c *gin.Context, role string) (int64, bool) {
	if role == "agent" {
		id, ok := getAgentID(c)
		if !ok {
			return 0, false
		}
		return int64(id), true
	}
	id, ok := getUserPanelID(c)
	if !ok {
		return 0, false
	}
	return int64(id), true
}
