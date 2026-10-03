package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestReplayNonceCacheRejectsRepeatInsideWindow(t *testing.T) {
	cache := newReplayNonceCache(time.Minute, 3)
	start := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	if !cache.remember("a", start) || cache.remember("a", start.Add(30*time.Second)) {
		t.Fatal("同一窗口内的重复随机数应被拒")
	}
	if cache.remember("a", start.Add(90*time.Second)) {
		t.Fatal("上一代里的随机数仍应被拒")
	}
	if !cache.remember("a", start.Add(3*time.Minute)) {
		t.Fatal("两代过后应当忘掉")
	}
	full := newReplayNonceCache(time.Minute, 1)
	if !full.remember("x", start) || !full.remember("y", start) || !full.remember("y", start) {
		t.Fatal("一代记满后应只放行不记录")
	}
}

// useClientResponseKey 让测试以客户站身份签授权响应，私钥生成在临时数据目录。
func useClientResponseKey(t *testing.T) ed25519.PublicKey {
	t.Helper()
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	official := false
	responseKeyOfficialForTest = &official
	resetResponseSigningKeyCache()
	t.Cleanup(func() {
		responseKeyOfficialForTest = nil
		resetResponseSigningKeyCache()
	})
	encoded, err := LicenseResponsePublicKey()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		t.Fatalf("公钥 %q err=%v", encoded, err)
	}
	return raw
}

type licenseV3Response struct {
	Code int `json:"code"`
	Data struct {
		Result   string `json:"result"`
		Reason   string `json:"reason"`
		ExpireTs *int64 `json:"expireTs"`
		Proof    *struct {
			AppKey         string `json:"appKey"`
			Domain         string `json:"domain"`
			ServerIP       string `json:"serverIp"`
			LicenseKeyHash string `json:"licenseKeyHash"`
			Nonce          string `json:"nonce"`
			ServerTime     int64  `json:"serverTime"`
			Signature      string `json:"signature"`
		} `json:"proof"`
	} `json:"data"`
}

// proofFieldsFromResponse 按 docs/api-sdk.md 写的规则，用 SDK 自己发出的值和响应里的结果拼签名字段。
func proofFieldsFromResponse(req licenseVerifyRequest, resp licenseV3Response) []proofField {
	expireTs := ""
	if resp.Data.ExpireTs != nil {
		expireTs = unixText(*resp.Data.ExpireTs)
	}
	return []proofField{
		{"appKey", req.AppKey},
		{"domain", req.Domain},
		{"serverIp", req.ServerIP},
		{"licenseKeyHash", sha256SumHex([]byte(req.LicenseKey))},
		{"nonce", req.Nonce},
		{"serverTime", unixText(resp.Data.Proof.ServerTime)},
		{"result", resp.Data.Result},
		{"reason", resp.Data.Reason},
		{"expireTs", expireTs},
	}
}

func TestLicenseVerifyV3SignsResponseAndRejectsReplay(t *testing.T) {
	secret := "app-secret"
	script := &licenseVerifyLimitScript{appKey: "demo-app", secret: secret}
	useLicenseVerifyLimitDB(t, script)
	useLicenseVerifyLimitBudget(t, 20)
	pub := useClientResponseKey(t)

	req := licenseVerifyRequest{
		AppKey: "demo-app", Domain: "example.com", ServerIP: "192.0.2.10", LicenseKey: "license-key",
		Timestamp: time.Now().Unix(), SignVersion: licenseSignVersionV3, Nonce: "nonce_" + randomHex(10),
	}
	req.Sign = licenseVerifyV3Sign(req, secret)
	body := mustJSON(t, req)
	recorder := postLicenseVerify(body, "192.0.2.50:9")
	var resp licenseV3Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Reason != "license_not_found" || resp.Data.Proof == nil {
		t.Fatalf("v3 已签名请求应带 proof: %s", recorder.Body.String())
	}
	if resp.Data.Proof.Nonce != req.Nonce || resp.Data.Proof.AppKey != req.AppKey {
		t.Fatalf("proof 没有回显请求: %+v", resp.Data.Proof)
	}
	fields := proofFieldsFromResponse(req, resp)
	if !verifyResponseProof(pub, responseProofLicense, fields, resp.Data.Proof.Signature) {
		t.Fatalf("proof 验签失败: %s", recorder.Body.String())
	}
	// 假服务器把结果改成通过：签名对不上。
	forged := resp
	forged.Data.Result, forged.Data.Reason = "pass", ""
	if verifyResponseProof(pub, responseProofLicense, proofFieldsFromResponse(req, forged), resp.Data.Proof.Signature) {
		t.Fatal("改过结果的响应不应验签通过")
	}
	// 拿这份响应去应付另一个随机数的请求：对不上。
	other := req
	other.Nonce = "nonce_" + randomHex(10)
	if verifyResponseProof(pub, responseProofLicense, proofFieldsFromResponse(other, resp), resp.Data.Proof.Signature) {
		t.Fatal("旧响应不应能用于新的请求")
	}

	replayed := postLicenseVerify(body, "192.0.2.50:9")
	_, _, reason := decodeLicenseVerifyBody(t, replayed)
	if reason != "replayed_request" {
		t.Fatalf("重放同一请求应被拒: %s", replayed.Body.String())
	}

	v2 := req
	v2.SignVersion, v2.Nonce = licenseSignVersionV2, ""
	v2.Sign = licenseVerifyV2Sign(v2, secret)
	var v2Resp licenseV3Response
	if err := json.Unmarshal(postLicenseVerify(mustJSON(t, v2), "192.0.2.51:9").Body.Bytes(), &v2Resp); err != nil {
		t.Fatal(err)
	}
	if v2Resp.Data.Proof != nil || v2Resp.Data.Reason != "license_not_found" {
		t.Fatalf("v2 请求不应带 proof: %+v", v2Resp.Data)
	}

	badNonce := req
	badNonce.Nonce = "short"
	badNonce.Sign = licenseVerifyV3Sign(badNonce, secret)
	badRecorder := postLicenseVerify(mustJSON(t, badNonce), "192.0.2.52:9")
	_, msg, badReason := decodeLicenseVerifyBody(t, badRecorder)
	if badReason != "verify_failed" || msg != "授权校验失败" || strings.Contains(badRecorder.Body.String(), "proof") {
		t.Fatalf("v3 随机数不合规应按签名失败处理且不签名: %s", badRecorder.Body.String())
	}
	wrongSign := req
	wrongSign.Nonce = "nonce_" + randomHex(10)
	wrongSign.Sign = licenseVerifyV3Sign(wrongSign, "other-secret")
	if body := postLicenseVerify(mustJSON(t, wrongSign), "192.0.2.53:9").Body.String(); strings.Contains(body, "proof") {
		t.Fatalf("请求签名不对时不应返回带签名的拒绝: %s", body)
	}
}

func TestResponseProofTextCannotInjectFields(t *testing.T) {
	text := responseProofText("kind", []proofField{{"domain", "a.com\nresult=pass"}})
	if text != "kind\ndomain=a.com result=pass\n" {
		t.Fatalf("换行应被替换: %q", text)
	}
}

func TestClientResponseKeyIsStableAndPrivate(t *testing.T) {
	first := useClientResponseKey(t)
	resetResponseSigningKeyCache()
	encoded, err := LicenseResponsePublicKey()
	if err != nil || encoded != base64.StdEncoding.EncodeToString(first) {
		t.Fatalf("重启后公钥应不变: %q err=%v", encoded, err)
	}
	info, err := os.Stat(licenseResponseKeyPath())
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("私钥文件应为 0600: %v %v", info, err)
	}
}

// signedTestSnapshot 用测试快照私钥签一份快照。
func signedTestSnapshot(t *testing.T, binding string, serverTime int64) (storeSnapshot, map[string]any) {
	t.Helper()
	snap, err := signStoreSnapshot(storeSnapshot{BindingID: binding, Domain: "shop.example.com", Edition: storeEditionCommercial,
		Features: []string{storeFeatureMultiApp}, Items: []storeSnapshotItem{}, ServerTime: serverTime, GraceDays: 7})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(snap)
	var raw map[string]any
	_ = json.Unmarshal(payload, &raw)
	return snap, raw
}

func TestCheckSourceSnapshotProof(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	restore := useStoreSnapshotKeysForTest(pub, priv)
	t.Cleanup(restore)
	now := time.Now().Unix()
	snap, raw := signedTestSnapshot(t, "sb_proof", now)
	nonce := randomHex(16)
	proof := signResponseProofWith(priv, responseProofStoreStat, storeSnapshotProofFields("sb_proof", "app-a", nonce, snap))
	check := sourceProofCheck{Kind: responseProofStoreStat, Binding: "sb_proof", Product: "app-a", Nonce: nonce, Data: map[string]any{"snapshotProof": proof}}

	if _, proved, err := checkSourceSnapshot(raw, check, buyerSnapshotState{}); err != nil || !proved {
		t.Fatalf("正常响应应通过: proved=%v err=%v", proved, err)
	}
	replay := check
	replay.Nonce = randomHex(16)
	if _, _, err := checkSourceSnapshot(raw, replay, buyerSnapshotState{}); err != errSourceProofInvalid {
		t.Fatalf("重放旧响应应被拒: %v", err)
	}
	wrongKind := check
	wrongKind.Kind = responseProofStoreBind
	if _, _, err := checkSourceSnapshot(raw, wrongKind, buyerSnapshotState{}); err != errSourceProofInvalid {
		t.Fatalf("绑定响应的 proof 不能当刷新用: %v", err)
	}
	_, fakePriv, _ := ed25519.GenerateKey(rand.Reader)
	fake := check
	fake.Data = map[string]any{"snapshotProof": signResponseProofWith(fakePriv, responseProofStoreStat, storeSnapshotProofFields("sb_proof", "app-a", nonce, snap))}
	if _, _, err := checkSourceSnapshot(raw, fake, buyerSnapshotState{}); err != errSourceProofInvalid {
		t.Fatalf("假服务器的签名应被拒: %v", err)
	}
	legacy := check
	legacy.Data = map[string]any{}
	if _, proved, err := checkSourceSnapshot(raw, legacy, buyerSnapshotState{}); err != nil || proved {
		t.Fatalf("老官网不带 proof 时应照旧接受: proved=%v err=%v", proved, err)
	}
	if _, _, err := checkSourceSnapshot(raw, legacy, buyerSnapshotState{SourceProof: true}); err != errSourceProofInvalid {
		t.Fatalf("收到过 proof 后再不带应被拒: %v", err)
	}
	newer, _ := signedTestSnapshot(t, "sb_proof", now+3600)
	if _, _, err := checkSourceSnapshot(raw, legacy, buyerSnapshotState{Snapshot: newer}); err != errSourceProofInvalid {
		t.Fatalf("比本机快照旧一小时的快照应被拒: %v", err)
	}
	slightlyNewer, _ := signedTestSnapshot(t, "sb_proof", now+60)
	if _, _, err := checkSourceSnapshot(raw, legacy, buyerSnapshotState{Snapshot: slightlyNewer}); err != nil {
		t.Fatalf("时钟小幅回拨应容忍: %v", err)
	}
}

func TestBuyerRefreshTreatsBadProofAsOffline(t *testing.T) {
	if !buyerRefreshIsNetwork(errSourceProofInvalid) {
		t.Fatal("验签失败应按连不上源站处理，进入离线宽限")
	}
	state, save := applyBuyerRefreshFailure(buyerSnapshotState{LastRefreshOK: true, GraceUntil: time.Now().Add(time.Hour).Unix()}, errSourceProofInvalid)
	if !save || state.LastRefreshOK || state.ExplicitRevoked {
		t.Fatalf("验签失败应只标记刷新失败: %+v save=%v", state, save)
	}
}
