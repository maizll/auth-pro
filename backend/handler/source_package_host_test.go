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

func TestExternalURLHealthDelistsOnlyExternalLinks(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	router, store := sourceStationRouter(t)
	admin, dev, _ := sourceApproveDeveloper(t, router, "health-dev", "secret")
	payload := sourcePluginTestZIP(t)
	sha := sha256Hex(payload)
	alive := true
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !alive {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	client := server.Client()
	useExternalPackageClientForTest(t, client)

	upload := sourceMultipart(t, router, "/api/v1/source/developer/packages/upload", dev, "demo-plugin.zip", payload, map[string]string{"kind": "plugin"})
	if sourceBodyCode(t, upload) != 200 {
		t.Fatalf("upload=%s", upload.Body.String())
	}
	hosted := "/api/v1/public/source-packages/" + sha + ".zip"
	hostedBody := `{"appId":1,"id":"hosted-plugin","name":"托管插件","version":"1.0.0","description":"x","category":"other","downloadUrl":"` + hosted + `","sha256":"` + sha + `"}`
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins", dev, hostedBody); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("save hosted=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins/hosted-plugin/submit", dev, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("submit hosted=%s", rec.Body.String())
	}

	externalURL := server.URL + "/demo-plugin.zip"
	externalBody := `{"appId":1,"id":"remote-plugin","name":"外链插件","version":"1.0.0","description":"x","category":"other","downloadUrl":"` + externalURL + `","sha256":"` + sha + `"}`
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins", dev, externalBody); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("save external=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins/remote-plugin/submit", dev, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("submit external=%s", rec.Body.String())
	}
	for _, id := range []string{"hosted-plugin", "remote-plugin"} {
		if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/"+id+"/approve", admin, "{}"); sourceBodyCode(t, rec) != 200 {
			t.Fatalf("approve %s=%s", id, rec.Body.String())
		}
		if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/"+id+"/shelf", admin, "{}"); sourceBodyCode(t, rec) != 200 {
			t.Fatalf("shelf %s=%s", id, rec.Body.String())
		}
	}

	alive = false
	message, err := runExternalURLHealth()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "下架 1") || !strings.Contains(message, "跳过") {
		t.Fatalf("health message=%s", message)
	}
	remote, err := store.GetPlugin("remote-plugin")
	if err != nil || remote.Status != sourceItemHidden {
		t.Fatalf("external must be delisted: %+v %v", remote, err)
	}
	hostedItem, err := store.GetPlugin("hosted-plugin")
	if err != nil || hostedItem.Status != sourceItemPublished {
		t.Fatalf("station upload must stay listed: %+v %v", hostedItem, err)
	}
	index := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if strings.Contains(index.Body.String(), "remote-plugin") || !strings.Contains(index.Body.String(), "hosted-plugin") {
		t.Fatalf("catalog index=%s", index.Body.String())
	}
	if !notificationContains(notificationListEvents(t, router, dev, "tab=message"), "external_package_delisted") {
		t.Fatal("developer was not notified")
	}
}
