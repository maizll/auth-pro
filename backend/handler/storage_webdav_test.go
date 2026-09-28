package handler

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestWebDAVKindAndLimit(t *testing.T) {
	if normalizeStorageKind("minio") != packageStorageS3 || normalizeStorageKind("oss") != packageStorageS3 {
		t.Fatal("minio should stay on s3")
	}
	for _, alias := range []string{"webdav", "nextcloud", "owncloud", "seafile", "alist", "cloudreve"} {
		if normalizeStorageKind(alias) != packageStorageWebDAV {
			t.Fatalf("%s kind=%s", alias, normalizeStorageKind(alias))
		}
	}
	if storageLimitText(packageStorageWebDAV) != "512 MB" {
		t.Fatalf("text=%s", storageLimitText(packageStorageWebDAV))
	}
	if storagePartLimit(packageStorageWebDAV) != storageLimitWebDAV-(1<<20) {
		t.Fatalf("part=%d", storagePartLimit(packageStorageWebDAV))
	}
	err := validateWebDAVEndpoint("https://alice:app-token@cloud.example/remote.php/dav/files/alice/")
	if err == nil || !strings.Contains(err.Error(), "地址里不要写账号密码") {
		t.Fatalf("userinfo err=%v", err)
	}
	if err := validateWebDAVEndpoint("ftp://cloud.example/dav"); err == nil || !strings.Contains(err.Error(), "http") {
		t.Fatalf("scheme err=%v", err)
	}
	if text := webdavStatusText(http.StatusRequestEntityTooLarge); text == nil || !strings.Contains(text.Error(), "512MB") {
		t.Fatalf("413=%v", text)
	}
	if text := webdavStatusText(http.StatusInsufficientStorage); text == nil || !strings.Contains(text.Error(), "空间不足") {
		t.Fatalf("507=%v", text)
	}
}

func TestWebDAVProbeChineseAndHidesSecret(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	router, _ := sourceStationRouter(t)
	password := "app-token-9f3c-not-for-buyers"
	unauthorized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RequestURI(), password) {
			t.Errorf("password appeared in request uri: %s", r.URL.RequestURI())
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(unauthorized.Close)

	userinfo := `{"name":"私人网盘","kind":"webdav","role":"backup","endpoint":"https://alice:` + password + `@cloud.example/dav","accessKey":"alice","secret":"` + password + `"}`
	rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/storage/test", sourceAdminToken(t), userinfo)
	if sourceBodyCode(t, rec) == 200 || !strings.Contains(rec.Body.String(), "地址里不要写账号密码") || strings.Contains(rec.Body.String(), password) {
		t.Fatalf("userinfo probe %s", rec.Body.String())
	}

	missingUser := `{"name":"私人网盘","kind":"webdav","role":"backup","endpoint":"https://cloud.example/dav","secret":"` + password + `"}`
	rec = sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/storage/test", sourceAdminToken(t), missingUser)
	if sourceBodyCode(t, rec) == 200 || !strings.Contains(rec.Body.String(), "请填写用户名") || strings.Contains(rec.Body.String(), password) {
		t.Fatalf("user probe %s", rec.Body.String())
	}

	body := fmt.Sprintf(`{"name":"私人网盘","kind":"webdav","role":"backup","endpoint":"%s/remote.php/dav/files/admin","accessKey":"admin","secret":"%s","prefix":"packages"}`, unauthorized.URL, password)
	rec = sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/storage/test", sourceAdminToken(t), body)
	if sourceBodyCode(t, rec) == 200 || !strings.Contains(rec.Body.String(), "用户名或密码") || strings.Contains(rec.Body.String(), password) {
		t.Fatalf("401 probe %s", rec.Body.String())
	}
}

func TestWebDAVRoundTripListShardAndBuyerTicket(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	router, _ := sourceStationRouter(t)
	const (
		user     = "admin"
		password = "app-token-9f3c-not-for-buyers"
	)
	mem := newMemoryWebDAV(t, user, password)
	mem.mkdir("packages")

	loc := testWebDAVLocation(t, "网盘", storageRolePrimary, mem.server.URL+"/remote.php/dav/files/admin", "packages", password)
	if err := probeWebDAV(context.Background(), loc, password); err != nil {
		t.Fatal(err)
	}
	if _, ok := mem.files["packages/.auth-pro-connection-test"]; ok {
		t.Fatal("probe file was left behind")
	}

	client, err := webdavClientFor(loc, password)
	if err != nil {
		t.Fatal(err)
	}
	knownKey := "packages/plugin/demo/1.0.0.zip"
	if err := client.put(context.Background(), knownKey, []byte("zip")); err != nil {
		t.Fatal(err)
	}
	if err := client.put(context.Background(), "packages/orphan.zip", []byte("orphan")); err != nil {
		t.Fatal(err)
	}
	listed, err := client.listAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, entry := range listed {
		seen[entry.Key] = !entry.IsDir
	}
	if !seen[knownKey] || !seen["packages/orphan.zip"] {
		t.Fatalf("listed=%v", seen)
	}
	exists, err := objectExists(context.Background(), loc, password, knownKey)
	if err != nil || !exists {
		t.Fatalf("exists=%v err=%v", exists, err)
	}

	blob := storageConfigBlob{Locations: []storageLocation{loc}}
	recordStorageCopy(&blob, "plugin", "demo", "1.0.0", loc.ID, formatWebDAVPackageRef(loc.ID, knownKey), []byte("zip"))
	if err := saveStorageBlob(blob); err != nil {
		t.Fatal(err)
	}
	rec := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/storage/locations/"+loc.ID+"/objects", sourceAdminToken(t), "")
	if sourceBodyCode(t, rec) != 200 || strings.Contains(rec.Body.String(), password) || strings.Contains(rec.Body.String(), mem.server.URL) {
		t.Fatalf("objects %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), knownKey) || !strings.Contains(rec.Body.String(), `"orphan":true`) {
		t.Fatalf("browse %s", rec.Body.String())
	}

	payload := []byte("0123456789abcdefghij")
	ref, err := putZipOnLocation(context.Background(), loc, password, "plugin", "wide", "1.2.0", payload)
	if err != nil || !strings.HasPrefix(ref, "webdav:"+loc.ID+"/") {
		t.Fatalf("ref=%s err=%v", ref, err)
	}
	previous := storagePartLimitOf
	storagePartLimitOf = func(kind string) int64 {
		if kind == packageStorageWebDAV {
			return 8
		}
		return previous(kind)
	}
	t.Cleanup(func() { storagePartLimitOf = previous })
	sharded, err := putZipOnLocation(context.Background(), loc, password, "plugin", "wide", "1.2.1", payload)
	if err != nil || !strings.Contains(sharded, ".parts.json") {
		t.Fatalf("sharded=%s err=%v", sharded, err)
	}
	_, key, ok := parseWebDAVPackageRef(sharded)
	if !ok {
		t.Fatalf("ref=%s", sharded)
	}
	got, err := readLocationObject(context.Background(), loc, password, key)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("join err=%v got=%q", err, got)
	}

	saved, err := loadStorageBlob()
	if err != nil {
		t.Fatal(err)
	}
	recordStorageCopy(&saved, "plugin", "wide", "1.2.1", loc.ID, sharded, payload)
	if err := saveStorageBlob(saved); err != nil {
		t.Fatal(err)
	}
	readBack, err := readStoredPackageWithFallback(context.Background(), sharded)
	if err != nil || string(readBack) != string(payload) {
		t.Fatalf("fallback err=%v got=%q", err, readBack)
	}
	if signed, safe := buyerSafeRedirect(context.Background(), sharded); safe || signed != "" {
		t.Fatalf("redirect=%q safe=%v", signed, safe)
	}
	if raw, safe, err := signedLocationURL(context.Background(), loc, password, key, 0); err != nil || safe || raw != "" {
		t.Fatalf("signed=%q safe=%v err=%v", raw, safe, err)
	}
	if !catalogRepoHostBlocked(sharded) {
		t.Fatal("webdav ref should stay off the buyer redirect list")
	}
	token, err := createStoreDownloadToken(storeDownloadClaims{
		LicenseID: 7, ItemKind: "plugin", ItemID: "wide", Version: "1.2.1", StorageKey: sharded, Source: "commercial",
	})
	if err != nil {
		t.Fatal(err)
	}
	rawToken, err := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[0])
	if err != nil {
		t.Fatal(err)
	}
	ticket := token + string(rawToken)
	if strings.Contains(ticket, password) || strings.Contains(ticket, mem.server.URL) || strings.Contains(string(rawToken), "http") {
		t.Fatalf("ticket leaked disk credentials: %s", rawToken)
	}
	claims, err := parseStoreDownloadToken(token)
	if err != nil || claims.StorageKey != sharded {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}

	checkStorageLocations(context.Background())
	checked, err := loadStorageBlob()
	if err != nil {
		t.Fatal(err)
	}
	var connected bool
	for _, row := range checked.Health {
		if row.LocationID == loc.ID && row.Target == "连接" && row.Level == "ok" {
			connected = true
		}
		if strings.Contains(row.Message, password) {
			t.Fatalf("health leaked secret: %s", row.Message)
		}
	}
	if !connected {
		t.Fatalf("health=%+v", checked.Health)
	}
}

func TestWebDAVReadOnlyProbeAndPrimaryFallback(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	_, _ = sourceStationRouter(t)
	const password = "app-token-9f3c-not-for-buyers"
	readonly := newMemoryWebDAV(t, "admin", password)
	readonly.mkdir("packages")
	readonly.denyWrite = http.StatusForbidden
	loc := testWebDAVLocation(t, "只读", storageRolePrimary, readonly.server.URL+"/dav", "packages", password)
	err := probeWebDAV(context.Background(), loc, password)
	if err == nil || !strings.Contains(err.Error(), "可以读取，但不能写入") || strings.Contains(err.Error(), password) {
		t.Fatalf("readonly err=%v", err)
	}

	readonly.denyWrite = http.StatusInternalServerError
	backupSrv, backupHits := newMemoryS3(t, nil)
	backup := testS3Location(t, "备", storageRoleBackup, backupSrv.URL)
	if err := saveStorageBlob(storageConfigBlob{Locations: []storageLocation{loc, backup}}); err != nil {
		t.Fatal(err)
	}
	ref, err := putPaidWithLocations(context.Background(), "plugin", "demo", "1.0.0", []byte("package-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ref, "s3:"+backup.ID+"/") || backupHits.put.Load() == 0 {
		t.Fatalf("ref=%s puts=%d", ref, backupHits.put.Load())
	}

	webBackup := newMemoryWebDAV(t, "admin", password)
	webBackup.mkdir("packages")
	primarySrv, _ := newMemoryS3(t, nil)
	primary := testS3Location(t, "主", storageRolePrimary, primarySrv.URL)
	secondary := testWebDAVLocation(t, "网盘备", storageRoleBackup, webBackup.server.URL+"/dav", "packages", password)
	if err := saveStorageBlob(storageConfigBlob{DualWrite: true, Locations: []storageLocation{primary, secondary}}); err != nil {
		t.Fatal(err)
	}
	if _, err := putPaidWithLocations(context.Background(), "plugin", "demo", "1.0.2", []byte("package-bytes")); err != nil {
		t.Fatal(err)
	}
	if webBackup.puts.Load() == 0 {
		t.Fatal("dual write did not reach webdav")
	}
}

func TestWebDAVRedirectDropsAuthorization(t *testing.T) {
	const password = "app-token-9f3c-not-for-buyers"
	var saw string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(other.Close)
	main := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("origin request lost its own authorization")
		}
		http.Redirect(w, r, other.URL+"/files/secret", http.StatusFound)
	}))
	t.Cleanup(main.Close)
	client := webdavClient{Endpoint: main.URL + "/dav", Username: "admin", Password: password}
	status, _, err := client.do(context.Background(), http.MethodGet, "file", nil, "", nil)
	if err != nil || status != http.StatusNoContent {
		t.Fatalf("status=%d err=%v", status, err)
	}
	if saw != "" || strings.Contains(saw, password) {
		t.Fatalf("authorization followed redirect: %s", saw)
	}
}

type davMem struct {
	server    *httptest.Server
	user      string
	password  string
	denyWrite int
	puts      atomic.Int32
	mu        sync.Mutex
	files     map[string][]byte
	dirs      map[string]bool
}

func newMemoryWebDAV(t *testing.T, user, password string) *davMem {
	t.Helper()
	mem := &davMem{
		user: user, password: password,
		files: map[string][]byte{}, dirs: map[string]bool{},
	}
	mem.server = httptest.NewServer(http.HandlerFunc(mem.serve))
	t.Cleanup(mem.server.Close)
	return mem
}

func (m *davMem) mkdir(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirs[strings.Trim(key, "/")] = true
}

func (m *davMem) serve(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.URL.RequestURI(), m.password) || r.URL.User != nil {
		http.Error(w, "password in url", http.StatusBadRequest)
		return
	}
	user, pass, ok := r.BasicAuth()
	if !ok || user != m.user || pass != m.password {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	base, key := splitDavPath(r.URL.Path)
	switch r.Method {
	case "MKCOL":
		if m.denyWrite >= 400 {
			w.WriteHeader(m.denyWrite)
			return
		}
		m.mu.Lock()
		m.dirs[key] = true
		m.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	case http.MethodPut:
		m.puts.Add(1)
		if m.denyWrite >= 400 {
			w.WriteHeader(m.denyWrite)
			return
		}
		buf, _ := io.ReadAll(r.Body)
		m.mu.Lock()
		m.files[key] = append([]byte(nil), buf...)
		m.noteParentsLocked(key)
		m.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	case http.MethodGet:
		m.mu.Lock()
		body, found := m.files[key]
		m.mu.Unlock()
		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(body)
	case http.MethodDelete:
		m.mu.Lock()
		_, found := m.files[key]
		delete(m.files, key)
		m.mu.Unlock()
		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "PROPFIND":
		m.writePropfind(w, base, key, r.Header.Get("Depth") != "0")
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func splitDavPath(raw string) (base, key string) {
	raw = "/" + strings.Trim(raw, "/")
	for _, mount := range []string{"/remote.php/dav/files/admin", "/dav"} {
		if raw == mount || strings.HasPrefix(raw, mount+"/") {
			return mount, strings.Trim(strings.TrimPrefix(raw, mount), "/")
		}
	}
	return "", strings.Trim(raw, "/")
}

func (m *davMem) noteParentsLocked(key string) {
	parts := strings.Split(key, "/")
	dir := ""
	for _, part := range parts[:len(parts)-1] {
		if dir == "" {
			dir = part
		} else {
			dir += "/" + part
		}
		m.dirs[dir] = true
	}
}

func (m *davMem) writePropfind(w http.ResponseWriter, base, key string, children bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if key != "" && !m.dirs[key] && m.files[key] == nil && !m.hasChildLocked(key) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	file, isFile := m.files[key]
	type row struct {
		key  string
		dir  bool
		size int
	}
	rows := []row{{key: key, dir: !isFile, size: len(file)}}
	if children && !isFile {
		seen := map[string]bool{}
		add := func(name string, dir bool, size int) {
			if name == "" || name == key || seen[name] {
				return
			}
			parent := ""
			if slash := strings.LastIndex(name, "/"); slash >= 0 {
				parent = name[:slash]
			}
			if parent != key {
				return
			}
			seen[name] = true
			rows = append(rows, row{key: name, dir: dir, size: size})
		}
		for name := range m.dirs {
			add(name, true, 0)
		}
		for name, body := range m.files {
			add(name, false, len(body))
		}
	}
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = w.Write([]byte(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">`))
	for _, row := range rows {
		href := davHref(base, row.key, row.dir)
		_, _ = w.Write([]byte(`<d:response><d:href>` + href + `</d:href><d:propstat><d:prop>`))
		if row.dir {
			_, _ = w.Write([]byte(`<d:resourcetype><d:collection/></d:resourcetype>`))
		} else {
			_, _ = w.Write([]byte(`<d:getcontentlength>` + strconv.Itoa(row.size) + `</d:getcontentlength><d:resourcetype/>`))
		}
		_, _ = w.Write([]byte(`<d:getlastmodified>Mon, 28 Sep 2026 00:00:00 GMT</d:getlastmodified></d:prop></d:propstat></d:response>`))
	}
	_, _ = w.Write([]byte(`</d:multistatus>`))
}

func davHref(base, key string, dir bool) string {
	path := strings.TrimRight(base, "/") + "/"
	if key != "" {
		path += key
	}
	if dir && !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path
}

func (m *davMem) hasChildLocked(key string) bool {
	prefix := key + "/"
	for name := range m.files {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	for name := range m.dirs {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func testWebDAVLocation(t *testing.T, name, role, endpoint, prefix, secret string) storageLocation {
	t.Helper()
	sealed, err := sealStorageSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	return storageLocation{
		ID: newStorageID(), Name: name, Kind: packageStorageWebDAV, Role: role,
		Endpoint: endpoint, AccessKey: "admin", KeyPrefix: prefix, SecretSealed: sealed,
	}
}
