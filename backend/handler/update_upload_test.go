package handler

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

func postOnlineUpdateUpload(t *testing.T, payload []byte) map[string]any {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "auth_pro-full.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(payload)
	_ = form.Close()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/system/update/upload", &body)
	c.Request.Header.Set("Content-Type", form.FormDataContentType())
	AdminOnlineUpdateUpload(c)
	var resp map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad response %s", recorder.Body.String())
	}
	return resp
}

// TestOnlineUpdateUploadRecognizesAndRejects 覆盖上传更新包：正确的包认出版本、适用端和说明；
// 改过的、没签名的、旧格式的、降级的、同版本的、另一端的、不是压缩包的都被拒，被拒的文件不留在服务器上。
func TestOnlineUpdateUploadRecognizesAndRejects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	onlineUpdatePublicKeyForTest = pub
	t.Cleanup(func() { onlineUpdatePublicKeyForTest = nil })
	previousVersion := config.AppVersion
	config.AppVersion = "1.0.0"
	t.Cleanup(func() { config.AppVersion = previousVersion })
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())

	read := func(path string) []byte {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	good := read(buildSignedOnlineUpdatePackage(t, priv, "1.0.1", "client", false))
	resp := postOnlineUpdateUpload(t, good)
	data, _ := resp["data"].(map[string]any)
	if resp["code"].(float64) != 200 || data["version"] != "1.0.1" || data["edition"] != "client" || data["editionLabel"] != "客户站" {
		t.Fatalf("good package: %v", resp)
	}
	if notes, _ := data["notes"].([]any); len(notes) != 1 || notes[0] != "修复问题" {
		t.Fatalf("notes: %v", data["notes"])
	}
	uploadPath, ok := onlineUpdateUploadPath(data["uploadId"].(string))
	if !ok {
		t.Fatalf("bad upload id %v", data["uploadId"])
	}
	if _, err := os.Stat(uploadPath); err != nil {
		t.Fatalf("accepted upload should be kept until confirm: %v", err)
	}

	cases := []struct {
		name    string
		payload []byte
		want    string
	}{
		{"签完又改过", read(buildSignedOnlineUpdatePackage(t, priv, "1.0.1", "client", true)), "没有通过官方签名校验"},
		{"没签名", read(buildSignedOnlineUpdatePackage(t, nil, "1.0.1", "client", false)), "没有官方签名"},
		{"旧格式没有发布信息", read(buildSignedOnlineUpdatePackage(t, priv, "1.0.1", "", false)), "旧格式的包"},
		{"降级", read(buildSignedOnlineUpdatePackage(t, priv, "0.9.0", "client", false)), "比当前版本 1.0.0 旧"},
		{"同版本", read(buildSignedOnlineUpdatePackage(t, priv, "1.0.0", "client", false)), "和当前版本一样"},
		{"官网的包", read(buildSignedOnlineUpdatePackage(t, priv, "1.0.1", "official", false)), "这是官网的更新包，客户站不能用"},
		{"不是压缩包", []byte("hello"), "这不是更新包文件"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postOnlineUpdateUpload(t, tc.payload)
			msg, _ := resp["msg"].(string)
			if resp["code"].(float64) != 400 || !strings.Contains(msg, tc.want) {
				t.Fatalf("want %q, got %v", tc.want, resp)
			}
			if strings.Contains(strings.ToLower(msg), "github") {
				t.Fatalf("message mentions the code host: %s", msg)
			}
			left, _ := filepath.Glob(filepath.Join(config.GetUpdateDir(), onlineUpdateUploadPrefix+"*.tar.gz"))
			if len(left) != 0 {
				t.Fatalf("rejected upload left files: %v", left)
			}
		})
	}
}

// TestOnlineUpdateUploadApplyUsesSamePipeline 确认安装上传的包时不下载，走和在线更新同一个任务流程。
func TestOnlineUpdateUploadApplyUsesSamePipeline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	onlineUpdatePublicKeyForTest = pub
	t.Cleanup(func() { onlineUpdatePublicKeyForTest = nil })
	previousVersion := config.AppVersion
	config.AppVersion = "1.0.0"
	t.Cleanup(func() { config.AppVersion = previousVersion })
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	t.Setenv("AUTO_PRO_FRONTEND_DIR", filepath.Join(t.TempDir(), "missing-frontend"))
	restore := stubOnlineUpdateApply(t, func(string, *onlineUpdateManifest) (string, error) {
		t.Fatal("uploaded package must not be downloaded")
		return "", nil
	})
	defer restore()

	good, err := os.ReadFile(buildSignedOnlineUpdatePackage(t, priv, "1.0.1", "client", false))
	if err != nil {
		t.Fatal(err)
	}
	resp := postOnlineUpdateUpload(t, good)
	data := resp["data"].(map[string]any)

	raw, _ := json.Marshal(map[string]string{"uploadId": data["uploadId"].(string)})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/system/update/upload/apply", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	AdminOnlineUpdateUploadApply(c)
	var applied struct {
		Code int              `json:"code"`
		Msg  string           `json:"msg"`
		Data *onlineUpdateJob `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &applied); err != nil || applied.Code != 200 || applied.Data == nil {
		t.Fatalf("apply: %s", recorder.Body.String())
	}
	deadline := time.Now().Add(10 * time.Second)
	var job *onlineUpdateJob
	for time.Now().Before(deadline) {
		job = snapshotOnlineUpdateJob(applied.Data.ID)
		if job != nil && job.Status != "running" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if job == nil || job.Status != "failed" {
		t.Fatalf("job should stop at backup or frontend dir in test env: %+v", job)
	}
	logs := strings.Join(job.Logs, "\n")
	if !strings.Contains(logs, "使用上传的更新包") || !strings.Contains(logs, "已核对更新包的大小、哈希、官方签名和适用端") || !strings.Contains(logs, "更新包结构校验通过") {
		t.Fatalf("uploaded package should go through the same checks: %s", logs)
	}

	recorder = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/system/update/upload/apply", strings.NewReader(`{"uploadId":"../../etc/passwd"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	AdminOnlineUpdateUploadApply(c)
	if !strings.Contains(recorder.Body.String(), "重新上传") {
		t.Fatalf("path traversal id should be rejected: %s", recorder.Body.String())
	}
}
