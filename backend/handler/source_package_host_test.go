package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeveloperPackageUploadStoresZipAndPublicURL(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	router, store := sourceStationRouter(t)
	_, dev, _ := sourceApproveDeveloper(t, router, "upload-dev", "secret")
	payload := sourcePluginTestZIP(t)
	rec := sourceMultipart(t, router, "/api/v1/source/developer/packages/upload", dev, "demo-plugin.zip", payload, map[string]string{"kind": "plugin"})
	if sourceBodyCode(t, rec) != 200 || !strings.Contains(rec.Body.String(), `"stored":true`) {
		t.Fatalf("upload=%s", rec.Body.String())
	}
	sha := sha256Hex(payload)
	publicURL := "/api/v1/public/source-packages/" + sha + ".zip"
	if !strings.Contains(rec.Body.String(), publicURL) || !strings.Contains(rec.Body.String(), sha) {
		t.Fatalf("missing url/sha: %s", rec.Body.String())
	}
	if plugins, err := store.ListPlugins(""); err != nil || len(plugins) != 0 {
		t.Fatalf("upload must not create a catalog row: %v %#v", err, plugins)
	}
	got := sourceJSON(t, router, http.MethodGet, publicURL, "", "")
	if got.Code != http.StatusOK || got.Body.String() != string(payload) {
		t.Fatalf("public file status=%d body=%s", got.Code, got.Body.String())
	}
}

func TestDeveloperSubmitValidatesExternalZip(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	router, _ := sourceStationRouter(t)
	_, dev, _ := sourceApproveDeveloper(t, router, "external-dev", "secret")
	payload := sourcePluginTestZIP(t)
	sha := sha256Hex(payload)
	var mode string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch mode {
		case "text":
			_, _ = w.Write([]byte("not a zip"))
		case "down":
			http.Error(w, "gone", http.StatusBadGateway)
		default:
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(payload)
		}
	}))
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = 0
	useExternalPackageClientForTest(t, client)

	save := func(id, url, sum string) {
		t.Helper()
		body := `{"appId":1,"id":"` + id + `","name":"外链插件","version":"1.0.0","description":"x","category":"other","downloadUrl":"` + url + `","sha256":"` + sum + `"}`
		if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins", dev, body); sourceBodyCode(t, rec) != 200 {
			t.Fatalf("save %s=%s", id, rec.Body.String())
		}
	}
	submit := func(id string) *httptest.ResponseRecorder {
		t.Helper()
		return sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins/"+id+"/submit", dev, "{}")
	}

	goodURL := server.URL + "/demo-plugin.zip"
	save("good-plugin", goodURL, sha)
	if rec := submit("good-plugin"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("matching zip must submit: %s", rec.Body.String())
	}

	save("bad-hash", goodURL, sourceTestSHA256())
	if rec := submit("bad-hash"); sourceBodyCode(t, rec) != 400 || !strings.Contains(rec.Body.String(), "不一致") {
		t.Fatalf("sha mismatch=%s", rec.Body.String())
	}

	mode = "text"
	save("not-zip", goodURL, sha)
	if rec := submit("not-zip"); sourceBodyCode(t, rec) != 400 || !strings.Contains(rec.Body.String(), "不是 ZIP") {
		t.Fatalf("non-zip=%s", rec.Body.String())
	}

	mode = "down"
	save("down-plugin", goodURL, sha)
	if rec := submit("down-plugin"); sourceBodyCode(t, rec) != 400 || !strings.Contains(rec.Body.String(), "不可达") {
		t.Fatalf("unreachable=%s", rec.Body.String())
	}
}
