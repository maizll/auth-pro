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

func buyerSourceBase() (string, error) {
	base := "https://auth.maizll.com"
	if db, err := config.DB(); err == nil {
		if value := strings.TrimSpace(configValue(db, storeConfigGroup, storeConfigSourceBase)); value != "" {
			base = value
		}
	}
	parsed, err := parseHTTPSBase(base)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func callSourceJSON(method, path string, body any, headers map[string]string, out any) error {
	base, err := buyerSourceBase()
	if err != nil {
		return err
	}
	var payload []byte
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequest(method, base+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errStoreNoRedirect
	}}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("无法连接源站")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
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
	code, _ := envelope["code"].(float64)
	if int(code) != 200 {
		msg, _ := envelope["msg"].(string)
		if msg == "" {
			msg = "源站拒绝了请求"
		}
		return errors.New(msg)
	}
	return nil
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
	return callSourceJSON(method, requestPath, send, headers, out)
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

func persistBuyerBind(envelope map[string]any) error {
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
		return saveSnapshotMap(snap, true, false, name+"\n"+role)
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
		state, ok := loadBuyerSnapshot()
		if ok {
			state.LastRefreshOK = false
			if strings.Contains(err.Error(), "失效") || strings.Contains(err.Error(), "吊销") || strings.Contains(err.Error(), "过期") {
				state.ExplicitRevoked = true
				state.RevokeReason = err.Error()
			}
			_ = saveBuyerSnapshot(state)
		}
		return err
	}
	data, _ := payload["data"].(map[string]any)
	snap, _ := data["snapshot"].(map[string]any)
	return saveSnapshotMap(snap, true, false, "")
}

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

func proxySourcePanel(c *gin.Context, path string) {
	body, _ := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	base, err := buyerSourceBase()
	if err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	req, err := http.NewRequest(http.MethodPost, base+path, bytes.NewReader(body))
	if err != nil {
		storeFail(c, 500, "转发失败")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		storeFail(c, 400, "无法连接源站")
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	c.Data(http.StatusOK, "application/json; charset=utf-8", raw)
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
