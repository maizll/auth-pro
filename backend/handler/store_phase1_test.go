package handler

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

func TestStoreDomainShapeRejectsPrivateNames(t *testing.T) {
	for _, domain := range []string{"localhost", "127.0.0.1", "192.168.0.8", "app.local", "printer.lan", "host.internal"} {
		if !storeDomainShapeRejected(domain) {
			t.Fatalf("expected reject %s", domain)
		}
	}
	if storeDomainShapeRejected("shop.example.com") {
		t.Fatal("public domain was rejected")
	}
}

func TestDecideMainLicenseRejectsOccupiedDomain(t *testing.T) {
	_, _, err := decideMainLicense("user", 2, []mainLicenseMatch{{ID: 9, OwnerType: "user", OwnerID: 1, Type: "domain", Status: "active"}})
	if !errors.Is(err, errStoreDomainOccupied) {
		t.Fatalf("occupied err=%v", err)
	}
	action, id, err := decideMainLicense("user", 1, []mainLicenseMatch{{ID: 9, OwnerType: "user", OwnerID: 1, Type: "domain", Status: "active"}})
	if err != nil || action != "associate" || id != 9 {
		t.Fatalf("associate action=%s id=%d err=%v", action, id, err)
	}
}

func TestDomainChangeWindowAndGrace(t *testing.T) {
	last := time.Now().Add(-29 * 24 * time.Hour)
	if domainChangeAllowed(&last, time.Now(), false) {
		t.Fatal("29 days should be blocked")
	}
	older := time.Now().Add(-31 * 24 * time.Hour)
	if !domainChangeAllowed(&older, time.Now(), false) || !domainChangeAllowed(&last, time.Now(), true) {
		t.Fatal("admin and 31 days should pass")
	}
	now := time.Now()
	tier, reason := evaluatePaidAccess(paidAccessInput{
		SnapshotOK: true, Edition: storeEditionCommercial, SnapshotDomain: "shop.example.com", RequestDomain: "shop.example.com",
		Now: now, GraceUntil: now.Add(6 * 24 * time.Hour), Offline: true,
	})
	if tier != storeEditionCommercial {
		t.Fatalf("within grace tier=%s reason=%s", tier, reason)
	}
	tier, reason = evaluatePaidAccess(paidAccessInput{
		SnapshotOK: true, Edition: storeEditionCommercial, SnapshotDomain: "shop.example.com", RequestDomain: "shop.example.com",
		Now: now, GraceUntil: now.Add(-time.Hour), Offline: true,
	})
	if tier != storeEditionFree || reason != "grace_expired" {
		t.Fatalf("after grace tier=%s reason=%s", tier, reason)
	}
	tier, _ = evaluatePaidAccess(paidAccessInput{
		SnapshotOK: true, Edition: storeEditionCommercial, SnapshotDomain: "shop.example.com", RequestDomain: "shop.example.com",
		Now: now, GraceUntil: now.Add(24 * time.Hour), ExplicitRevoked: true, Offline: true,
	})
	if tier != storeEditionFree {
		t.Fatal("explicit revoke must ignore grace")
	}
	tier, reason = evaluatePaidAccess(paidAccessInput{
		SnapshotOK: true, Edition: storeEditionCommercial, SnapshotDomain: "old.example.com", RequestDomain: "new.example.com", Now: now,
	})
	if tier != storeEditionFree || reason != "domain_mismatch" {
		t.Fatalf("domain mismatch tier=%s reason=%s", tier, reason)
	}
}

func TestStoreOrderGrantAndDownloadRules(t *testing.T) {
	grant, err := shouldGrantStoreOrder("pending", 100, 90)
	if grant || !errors.Is(err, errStoreAmountMismatch) {
		t.Fatalf("mismatch grant=%v err=%v", grant, err)
	}
	grant, err = shouldGrantStoreOrder("paid", 100, 100)
	if grant || err != nil {
		t.Fatalf("duplicate grant=%v err=%v", grant, err)
	}
	if err := storeDownloadAllowed(time.Now().Add(time.Minute), time.Now(), false); !errors.Is(err, errStoreDownloadDenied) {
		t.Fatalf("refund/no entitlement err=%v", err)
	}
	if err := storeDownloadAllowed(time.Now().Add(-time.Second), time.Now(), true); err == nil {
		t.Fatal("expired token was accepted")
	}
	if !paidEnableAllowed(0, false, false) || paidEnableAllowed(100, false, false) {
		t.Fatal("free items must stay available and paid items stay blocked")
	}
	if !appCreateDecision(0, false) || appCreateDecision(1, false) || !appCreateDecision(3, true) {
		t.Fatal("app create decision mismatch")
	}
}

func TestTamperedSnapshotFailsVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	restore := useStoreSnapshotKeysForTest(pub, priv)
	defer restore()
	signed, err := signStoreSnapshot(storeSnapshot{BindingID: "sb_test", Domain: "shop.example.com", Edition: storeEditionCommercial, Features: []string{storeFeatureMultiApp}, Items: []storeSnapshotItem{}})
	if err != nil || !verifyStoreSnapshot(signed) {
		t.Fatalf("sign err=%v", err)
	}
	signed.Edition = storeEditionFree
	if verifyStoreSnapshot(signed) {
		t.Fatal("tampered snapshot verified")
	}
}

func TestPlaceholderPublicKeyRejectsSnapshots(t *testing.T) {
	if embeddedStoreSnapshotPublicKey != storeSnapshotPublicKeyPlaceholder || storeSnapshotPublicKeyConfigured() {
		t.Fatal("source build must keep the unconfigured placeholder")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	restore := useStoreSnapshotKeysForTest(pub, priv)
	signed, err := signStoreSnapshot(storeSnapshot{BindingID: "sb_placeholder", Domain: "shop.example.com", Edition: storeEditionCommercial, Features: []string{storeFeatureMultiApp}, Items: []storeSnapshotItem{}})
	if err != nil || !verifyStoreSnapshot(signed) {
		restore()
		t.Fatal("in-test key failed to sign")
	}
	restore()
	if verifyStoreSnapshot(signed) {
		t.Fatal("placeholder accepted a snapshot")
	}
	if _, err := signStoreSnapshot(storeSnapshot{BindingID: "sb_placeholder", Items: []storeSnapshotItem{}}); err == nil || !strings.Contains(err.Error(), "拒绝签发快照") {
		t.Fatal("source must refuse to sign when the public key is unconfigured")
	}
	view := currentBuyerAccess(nil)
	if view.Edition != storeEditionFree || !view.GraceWarning || view.Reason != storeReasonSnapshotKeyUnconfigured || view.SnapshotValid {
		t.Fatal("buyer must stay on the free tier and surface the unconfigured warning")
	}
}

func TestLdflagsPublicKeyRequiresMatchingPrivateFile(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	prev := embeddedStoreSnapshotPublicKey
	embeddedStoreSnapshotPublicKey = base64.StdEncoding.EncodeToString(pub)
	applyEmbeddedStoreSnapshotPublicKey()
	t.Cleanup(func() {
		embeddedStoreSnapshotPublicKey = prev
		applyEmbeddedStoreSnapshotPublicKey()
	})
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	if !storeSnapshotPublicKeyConfigured() {
		t.Fatal("ldflags public key was not applied")
	}
	if _, err := signStoreSnapshot(storeSnapshot{BindingID: "sb_ldflags", Items: []storeSnapshotItem{}}); err == nil || !strings.Contains(err.Error(), "未配置商店签名私钥") {
		t.Fatal("configured public key must still refuse to sign without the source private key")
	}
}

func TestStoreKeygenRefusesOverwriteAndPrintsOnlyPublicKey(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	if !wantsStoreKeygen([]string{"store-keygen"}) || !wantsStoreKeygen([]string{"--store-keygen"}) || wantsStoreKeygen(nil) {
		t.Fatal("keygen command detection")
	}
	if !storeKeygenForce([]string{"store-keygen", "--force"}) || storeKeygenForce([]string{"store-keygen"}) {
		t.Fatal("force flag detection")
	}
	var out bytes.Buffer
	if err := runStoreKeygen(&out, false); err != nil {
		t.Fatal(err)
	}
	path := storeSnapshotPrivateKeyPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("private key file mode %o", info.Mode().Perm())
	}
	secret, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	secretText := strings.TrimSpace(string(secret))
	raw, err := base64.StdEncoding.DecodeString(secretText)
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		t.Fatal("private key file is not an ed25519 private key")
	}
	wantPub := base64.StdEncoding.EncodeToString(ed25519.PrivateKey(raw).Public().(ed25519.PublicKey))
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 || lines[0] != wantPub || lines[1] != storeKeygenHint {
		t.Fatal("stdout must be only the public key and the Chinese hint")
	}
	if strings.Contains(out.String(), secretText) {
		t.Fatal("stdout included the private key")
	}
	out.Reset()
	if err := runStoreKeygen(&out, false); err == nil {
		t.Fatal("overwrite without force succeeded")
	}
	again, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(again, secret) {
		t.Fatal("refused overwrite changed the private key file")
	}
	if strings.Contains(out.String(), secretText) {
		t.Fatal("refused overwrite wrote the private key to stdout")
	}
	out.Reset()
	if err := runStoreKeygen(&out, true); err != nil {
		t.Fatal(err)
	}
	replaced, err := os.ReadFile(path)
	if err != nil || bytes.Equal(replaced, secret) {
		t.Fatal("force did not replace the private key file")
	}
	replacedText := strings.TrimSpace(string(replaced))
	if strings.Contains(out.String(), secretText) || strings.Contains(out.String(), replacedText) {
		t.Fatal("force stdout included a private key")
	}
	if filepath.Base(path) != "snapshot-ed25519.key" {
		t.Fatal("private key path changed")
	}
}

func TestPaymentCallbacksRouteStoreBeforeRecharge(t *testing.T) {
	if onlineSettlementRoute("PP2026") != "store" || onlineSettlementRoute("AU1") != "upgrade" || onlineSettlementRoute("UR1") != "recharge_or_license" {
		t.Fatal("route table changed")
	}
	for _, file := range []string{"epay.go", "epay_v2.go", "payment_channel.go"} {
		payload, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(payload)
		if !strings.Contains(text, "dispatchVerifiedOnlinePayment") {
			t.Fatalf("%s does not dispatch store orders", file)
		}
	}
	for _, name := range []string{"func settleEpayCallback", "func settleEpayV2Callback", "func settleRegisteredChannelNotify"} {
		src, err := os.ReadFile(map[string]string{
			"func settleEpayCallback":            "epay.go",
			"func settleEpayV2Callback":          "epay_v2.go",
			"func settleRegisteredChannelNotify": "payment_channel.go",
		}[name])
		if err != nil {
			t.Fatal(err)
		}
		body := string(src)
		start := strings.Index(body, name)
		rest := body[start+len(name):]
		next := strings.Index(rest, "\nfunc ")
		if next < 0 {
			next = len(rest)
		}
		fn := rest[:next]
		if strings.Contains(fn, "settleRechargeOrder") {
			t.Fatalf("%s still settles recharge before store dispatch", name)
		}
	}
}

func TestStoreChallengeDoesNotFollowRedirect(t *testing.T) {
	secretHits := 0
	secret := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secretHits++
		_, _ = io.WriteString(w, "secret")
	}))
	defer secret.Close()
	redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, secret.URL+"/hidden", http.StatusFound)
	}))
	defer redirector.Close()
	stationChallengePermitAltPort = true
	t.Cleanup(func() { stationChallengePermitAltPort = false })
	raw := pinHostToServer(t, "shop.example.com", redirector, secret)
	_, err := fetchStationChallengeAt(context.Background(), raw+"/api/v1/public/station-verify/abc")
	if !errors.Is(err, errStoreNoRedirect) {
		t.Fatalf("redirect err=%v", err)
	}
	if secretHits != 0 {
		t.Fatalf("followed redirect, secret hits=%d", secretHits)
	}
}

func TestRejectPrivateDNSForStoreDomain(t *testing.T) {
	previous := currentSafeFetchHooks()
	setSafeFetchHooks(safeFetchHooks{resolve: func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.1.2.3")}, nil
	}})
	t.Cleanup(func() { setSafeFetchHooks(previous) })
	if err := rejectStoreDomain(context.Background(), "shop.example.com"); err == nil {
		t.Fatal("private DNS was accepted")
	}
}

func TestFreeTierAppCreateGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	prev := appPurchaseLicenseTypesOK
	appPurchaseLicenseTypesOK = true
	t.Cleanup(func() { appPurchaseLicenseTypesOK = prev })

	blocked := openPhaseDB(t, phaseScript{count: 1})
	config.SetDBOverrideForTest(blocked)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })
	body := postApp(t, "shop.example.com", `{"name":"第二个","enabled":true}`)
	if !strings.Contains(body, `"code":402`) || !strings.Contains(body, storeFeatureMultiApp) || strings.Contains(body, "删除") {
		t.Fatalf("second app body=%s", body)
	}

	allowed := openPhaseDB(t, phaseScript{count: 0, allowInsert: true})
	config.SetDBOverrideForTest(allowed)
	body = postApp(t, "shop.example.com", `{"name":"第一个","enabled":true}`)
	if !strings.Contains(body, `"code":200`) {
		t.Fatalf("first app body=%s", body)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	restore := useStoreSnapshotKeysForTest(pub, priv)
	defer restore()
	signed, err := signStoreSnapshot(storeSnapshot{
		BindingID: "sb_commercial", Domain: "shop.example.com", Edition: storeEditionCommercial,
		Features: []string{storeFeatureMultiApp}, Items: []storeSnapshotItem{}, AllPaidItems: true, ServerTime: time.Now().Unix(), GraceDays: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := saveBuyerSnapshot(buyerSnapshotState{Snapshot: signed, VerifiedAt: time.Now().Unix(), GraceUntil: time.Now().Add(7 * 24 * time.Hour).Unix(), LastRefreshOK: true, BindingID: signed.BindingID}); err != nil {
		t.Fatal(err)
	}
	many := openPhaseDB(t, phaseScript{count: 4, allowInsert: true, allowConfig: true})
	config.SetDBOverrideForTest(many)
	body = postApp(t, "shop.example.com", `{"name":"更多","enabled":true}`)
	if !strings.Contains(body, `"code":200`) {
		t.Fatalf("commercial body=%s", body)
	}

	_ = os.Remove(buyerSnapshotPath())
	downgrade := openPhaseDB(t, phaseScript{count: 4})
	config.SetDBOverrideForTest(downgrade)
	body = postApp(t, "shop.example.com", `{"name":"降级后再创建","enabled":true}`)
	if !strings.Contains(body, `"code":402`) {
		t.Fatalf("downgrade body=%s", body)
	}
	licenseSrc, err := os.ReadFile("license_verify.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(licenseSrc), "writeEditionRequired") || strings.Contains(string(licenseSrc), storeFeatureMultiApp) {
		t.Fatal("license verify must not consult the commercial gate")
	}
}

func TestDispatchStoreOrderRejectsAmountMismatch(t *testing.T) {
	db := openPhaseDB(t, phaseScript{orderStatus: "pending", orderAmount: 100, migrationsApplied: true})
	err := dispatchVerifiedOnlinePayment(db, "PP100", 90, "easypay", "alipay", "trade", "{}")
	if !errors.Is(err, errStoreAmountMismatch) {
		t.Fatalf("err=%v", err)
	}
	db = openPhaseDB(t, phaseScript{orderStatus: "paid", orderAmount: 100, migrationsApplied: true})
	if err := dispatchVerifiedOnlinePayment(db, "PP100", 100, "easypay", "alipay", "trade", "{}"); err != nil {
		t.Fatalf("duplicate err=%v", err)
	}
}

func postApp(t *testing.T, host, payload string) string {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/app/create", strings.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Host = host
	AppCreate(c)
	return w.Body.String()
}

type phaseScript struct {
	count             int64
	allowInsert       bool
	allowConfig       bool
	orderStatus       string
	orderAmount       int64
	migrationsApplied bool
}

func openPhaseDB(t *testing.T, script phaseScript) *sql.DB {
	t.Helper()
	name := "phase-store-" + strings.ReplaceAll(t.Name(), "/", "-") + "-" + randomHex(4)
	driverName := name
	sql.Register(driverName, &phaseDriver{script: script})
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type phaseDriver struct{ script phaseScript }
type phaseConn struct{ script phaseScript }
type phaseTx struct{ script phaseScript }
type phaseRows struct {
	cols []string
	vals []driver.Value
	done bool
}

func (d *phaseDriver) Open(string) (driver.Conn, error) { return &phaseConn{script: d.script}, nil }
func (c *phaseConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare unsupported")
}
func (c *phaseConn) Close() error              { return nil }
func (c *phaseConn) Begin() (driver.Tx, error) { return &phaseTx{script: c.script}, nil }
func (c *phaseConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return c.Begin()
}
func (tx *phaseTx) Commit() error   { return nil }
func (tx *phaseTx) Rollback() error { return nil }

func (c *phaseConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(strings.ToLower(query), "insert into apps") {
		if !c.script.allowInsert {
			return nil, errors.New("unexpected insert")
		}
		return phaseResult(1), nil
	}
	if strings.Contains(query, "DELETE") || strings.Contains(query, "UPDATE apps") {
		return nil, errors.New("app rows must not be deleted")
	}
	return phaseResult(1), nil
}
func (tx *phaseTx) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return (&phaseConn{script: tx.script}).ExecContext(ctx, query, args)
}
func (c *phaseConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	lower := strings.ToLower(query)
	switch {
	case strings.Contains(lower, "from apps"):
		return &phaseRows{cols: []string{"n"}, vals: []driver.Value{c.script.count}}, nil
	case strings.Contains(lower, "schema_migrations"):
		count := int64(0)
		if c.script.migrationsApplied {
			count = 1
		}
		return &phaseRows{cols: []string{"n"}, vals: []driver.Value{count}}, nil
	case strings.Contains(lower, "store_purchase_orders"):
		return &phaseRows{cols: []string{"id", "owner_type", "owner_id", "license_id", "item_kind", "item_id", "period", "amount_cents", "status"}, vals: []driver.Value{
			int64(1), "user", int64(1), int64(1), "edition", "1", "permanent", c.script.orderAmount, c.script.orderStatus,
		}}, nil
	case strings.Contains(lower, "system_configs"):
		if !c.script.allowConfig && c.script.orderStatus == "" {
			return nil, errors.New("unexpected config query: " + query)
		}
		return &phaseRows{cols: []string{"value"}, vals: []driver.Value{""}}, nil
	default:
		if c.script.orderStatus != "" || c.script.allowInsert {
			return &phaseRows{cols: []string{"value"}, vals: []driver.Value{""}}, nil
		}
		return nil, errors.New("unexpected query: " + query)
	}
}
func (tx *phaseTx) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return (&phaseConn{script: tx.script}).QueryContext(ctx, query, args)
}

func (r *phaseRows) Columns() []string { return r.cols }
func (r *phaseRows) Close() error      { return nil }
func (r *phaseRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	copy(dest, r.vals)
	return nil
}

type phaseResult int64

func (r phaseResult) LastInsertId() (int64, error) { return int64(r), nil }
func (r phaseResult) RowsAffected() (int64, error) { return 1, nil }

var phaseRegisterMu sync.Mutex

func init() {
	_ = json.Marshal
	_ = phaseRegisterMu
}
