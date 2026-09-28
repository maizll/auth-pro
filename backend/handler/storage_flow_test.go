package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// 两个都填时：收费仓库是主存储，发布仓库是备用。接口不回显令牌。
func TestMigrateLegacyStorageKeepsBothAndHidesSecrets(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	router, store := sourceStationRouter(t)
	if err := writeGitHubPaidSetting(githubPaidOwnerSettingKey, "acme"); err != nil {
		t.Fatal(err)
	}
	if err := writeGitHubPaidSetting(githubPaidRepoSettingKey, "paid-packages"); err != nil {
		t.Fatal(err)
	}
	sealed, err := sealGitHubPaidToken("ghp_paid_secret_token_value")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeGitHubPaidTokenSealed(sealed); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveReleaseSettings(sourceReleaseSettings{
		Provider: "github", Owner: "acme", Repo: "public-releases", Token: "ghs_release_secret_value",
	}); err != nil {
		t.Fatal(err)
	}

	blob, err := loadStorageBlob()
	if err != nil {
		t.Fatal(err)
	}
	if len(blob.Locations) != 2 {
		t.Fatalf("locations=%d", len(blob.Locations))
	}
	if blob.Locations[0].Legacy != storageLegacyPaid || blob.Locations[0].Role != storageRolePrimary {
		t.Fatalf("paid=%+v", blob.Locations[0])
	}
	if blob.Locations[1].Legacy != storageLegacyRelease || blob.Locations[1].Role != storageRoleBackup {
		t.Fatalf("release=%+v", blob.Locations[1])
	}
	paidSecret, err := openStorageSecret(blob.Locations[0].SecretSealed)
	if err != nil || paidSecret != "ghp_paid_secret_token_value" {
		t.Fatalf("paid secret err=%v", err)
	}

	rec := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/storage/locations", sourceAdminToken(t), "")
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("code body %s", rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "ghp_paid") || strings.Contains(body, "ghs_release") || strings.Contains(body, "secretSealed") {
		t.Fatalf("response leaked secret: %s", body)
	}
	if !strings.Contains(body, "主存储") || !strings.Contains(body, "备用") {
		t.Fatalf("roles missing: %s", body)
	}
}

func TestStoragePrimaryFallsBackAndDualWrite(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	_, _ = sourceStationRouter(t)
	primarySrv, primaryHits := newMemoryS3(t, func(method string) int {
		if method == http.MethodPut {
			return http.StatusInternalServerError
		}
		return http.StatusOK
	})
	backupSrv, backupHits := newMemoryS3(t, nil)
	primary := testS3Location(t, "主", storageRolePrimary, primarySrv.URL)
	backup := testS3Location(t, "备", storageRoleBackup, backupSrv.URL)
	if err := saveStorageBlob(storageConfigBlob{Locations: []storageLocation{primary, backup}}); err != nil {
		t.Fatal(err)
	}
	payload := []byte("package-bytes-primary-fail")
	ref, err := putPaidWithLocations(context.Background(), "plugin", "demo", "1.0.0", payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ref, "s3:"+backup.ID+"/") {
		t.Fatalf("ref=%s", ref)
	}
	if primaryHits.put.Load() == 0 || backupHits.put.Load() == 0 {
		t.Fatalf("hits primary=%d backup=%d", primaryHits.put.Load(), backupHits.put.Load())
	}

	bothA, _ := newMemoryS3(t, nil)
	bothB, bothBHits := newMemoryS3(t, nil)
	a := testS3Location(t, "主2", storageRolePrimary, bothA.URL)
	b := testS3Location(t, "备2", storageRoleBackup, bothB.URL)
	if err := saveStorageBlob(storageConfigBlob{DualWrite: true, Locations: []storageLocation{a, b}}); err != nil {
		t.Fatal(err)
	}
	if _, err := putPaidWithLocations(context.Background(), "plugin", "demo", "1.0.1", payload); err != nil {
		t.Fatal(err)
	}
	if bothBHits.put.Load() == 0 {
		t.Fatal("dual write did not reach backup")
	}
	saved, err := loadStorageBlob()
	if err != nil {
		t.Fatal(err)
	}
	copies := 0
	for _, copy := range saved.Copies {
		if copy.Version == "1.0.1" {
			copies++
		}
	}
	if copies != 2 {
		t.Fatalf("dual copies=%d", copies)
	}
}

func TestStorageShardRoundTripAndMismatch(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	_, _ = sourceStationRouter(t)
	previous := storagePartLimitOf
	storagePartLimitOf = func(string) int64 { return 8 }
	t.Cleanup(func() { storagePartLimitOf = previous })

	parts, err := splitPayload([]byte("0123456789abcdefghij"), 8)
	if err != nil || len(parts) != 3 {
		t.Fatalf("parts=%d err=%v", len(parts), err)
	}
	joined, err := joinShards(parts, storageSHA256([]byte("0123456789abcdefghij")), 20)
	if err != nil || string(joined) != "0123456789abcdefghij" {
		t.Fatalf("join err=%v", err)
	}
	if _, err := joinShards(parts, strings.Repeat("ab", 32), 20); err == nil || !strings.Contains(err.Error(), "校验码") {
		t.Fatalf("mismatch err=%v", err)
	}

	server, hits := newMemoryS3(t, nil)
	loc := testS3Location(t, "分片", storageRolePrimary, server.URL)
	payload := []byte("0123456789abcdefghij")
	ref, err := putZipOnLocation(context.Background(), loc, "s3-secret-value", "plugin", "wide", "1.2.0", payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ref, ".parts.json") {
		t.Fatalf("ref=%s", ref)
	}
	if hits.put.Load() < 4 {
		t.Fatalf("puts=%d want manifest plus parts", hits.put.Load())
	}
	_, key, ok := parseS3PackageRef(ref)
	if !ok {
		t.Fatalf("ref=%s", ref)
	}
	got, err := readLocationObject(context.Background(), loc, "s3-secret-value", key)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("read err=%v got=%q", err, got)
	}
}

func TestStorageObjectsMarkOrphansAndChineseProbe(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	router, _ := sourceStationRouter(t)
	server, _ := newMemoryS3(t, nil)
	loc := testS3Location(t, "对象", storageRolePrimary, server.URL)
	secret, err := openStorageSecret(loc.SecretSealed)
	if err != nil {
		t.Fatal(err)
	}
	client, err := s3ClientFor(loc, secret)
	if err != nil {
		t.Fatal(err)
	}
	knownKey := "plugin/demo/1.0.0.zip"
	if err := client.put(context.Background(), knownKey, []byte("zip")); err != nil {
		t.Fatal(err)
	}
	if err := client.put(context.Background(), "stray.zip", []byte("orphan")); err != nil {
		t.Fatal(err)
	}
	blob := storageConfigBlob{Locations: []storageLocation{loc}}
	recordStorageCopy(&blob, "plugin", "demo", "1.0.0", loc.ID, formatS3PackageRef(loc.ID, knownKey), []byte("zip"))
	if err := saveStorageBlob(blob); err != nil {
		t.Fatal(err)
	}
	rec := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/storage/locations/"+loc.ID+"/objects", sourceAdminToken(t), "")
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("body %s", rec.Body.String())
	}
	var parsed struct {
		Data struct {
			List []struct {
				Key    string `json:"key"`
				Orphan bool   `json:"orphan"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	var known, stray bool
	for _, row := range parsed.Data.List {
		if row.Key == knownKey && !row.Orphan {
			known = true
		}
		if row.Key == "stray.zip" && row.Orphan {
			stray = true
		}
	}
	if !known || !stray {
		t.Fatalf("rows=%+v", parsed.Data.List)
	}

	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<Error><Code>AccessDenied</Code><Message>no</Message></Error>`))
	}))
	t.Cleanup(denied.Close)
	body := `{"name":"测","kind":"s3","role":"backup","endpoint":"` + denied.URL + `","region":"oss-cn-hangzhou","bucket":"pkg","accessKey":"AKIAtest","secret":"secret-value","pathStyle":true}`
	probe := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/storage/test", sourceAdminToken(t), body)
	if sourceBodyCode(t, probe) == 200 || !strings.Contains(probe.Body.String(), "密钥无效") {
		t.Fatalf("probe %s", probe.Body.String())
	}
}

type s3HitCount struct {
	put atomic.Int32
}

func newMemoryS3(t *testing.T, statusOf func(method string) int) (*httptest.Server, *s3HitCount) {
	t.Helper()
	hits := &s3HitCount{}
	var mu sync.Mutex
	objects := map[string][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			hits.put.Add(1)
		}
		if statusOf != nil {
			if code := statusOf(r.Method); code >= 400 {
				w.WriteHeader(code)
				_, _ = w.Write([]byte(`<Error><Code>InternalError</Code><Message>down</Message></Error>`))
				return
			}
		}
		key := strings.TrimPrefix(r.URL.Path, "/pkg/")
		key = strings.Trim(key, "/")
		switch r.Method {
		case http.MethodPut:
			buf, _ := io.ReadAll(r.Body)
			mu.Lock()
			objects[key] = append([]byte(nil), buf...)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			if r.URL.Query().Get("list-type") == "2" {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/xml")
				_, _ = w.Write([]byte(`<ListBucketResult>`))
				prefix := r.URL.Query().Get("prefix")
				for name, body := range objects {
					if prefix != "" && !strings.HasPrefix(name, prefix) {
						continue
					}
					_, _ = w.Write([]byte(`<Contents><Key>` + name + `</Key><Size>` + strconv.Itoa(len(body)) + `</Size><LastModified>2026-09-28T00:00:00Z</LastModified></Contents>`))
				}
				_, _ = w.Write([]byte(`</ListBucketResult>`))
				return
			}
			mu.Lock()
			body, ok := objects[key]
			mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(body)
		case http.MethodHead:
			mu.Lock()
			body, ok := objects[key]
			mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			mu.Lock()
			delete(objects, key)
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)
	return server, hits
}

func testS3Location(t *testing.T, name, role, endpoint string) storageLocation {
	t.Helper()
	sealed, err := sealStorageSecret("s3-secret-value")
	if err != nil {
		t.Fatal(err)
	}
	return storageLocation{
		ID: newStorageID(), Name: name, Kind: packageStorageS3, Role: role,
		Endpoint: endpoint, Region: "us-east-1", Bucket: "pkg", AccessKey: "AKIAtest",
		PathStyle: true, SecretSealed: sealed,
	}
}
