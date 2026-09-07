package appstore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestAdminRoutesRequireAdminJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &fakeTemplateRepository{items: []Template{{
		ID: "default", TemplateID: "default", Name: "默认首页模板", Description: "内置模板",
		PreviewImage: "/preview.svg", Version: "1.0.0", Author: Author{Name: "auth_pro 官方"},
		Enabled: true, Available: true, Installed: true,
	}}}
	router := gin.New()
	NewServer(repository).RegisterAdminRoutes(router.Group("/api"))

	request := httptest.NewRequest(http.MethodGet, "/api/app-store/templates", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/app-store/templates", nil)
	request.Header.Set("Authorization", "Bearer "+signedToken(t, "user", ""))
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if responseCode(t, recorder) != 403 {
		t.Fatalf("user response = %s", recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/app-store/templates", nil)
	request.Header.Set("Authorization", "Bearer "+signedToken(t, "admin", "R_ADMIN"))
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if responseCode(t, recorder) != 403 {
		t.Fatalf("R_ADMIN response = %s", recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/app-store/templates", nil)
	request.Header.Set("Authorization", "Bearer "+signedToken(t, "admin", "R_SUPER"))
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if responseCode(t, recorder) != 200 {
		t.Fatalf("admin response = %s", recorder.Body.String())
	}
	var response struct {
		Data struct {
			List []Template `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.List) != 1 {
		t.Fatalf("template list = %+v", response.Data.List)
	}
	template := response.Data.List[0]
	if template.Name == "" || template.Description == "" || template.PreviewImage == "" || template.Version == "" || template.Author.Name == "" {
		t.Fatalf("template metadata is incomplete: %+v", template)
	}
}

func TestAdminRoutesDashboardAndMutations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &fakeTemplateRepository{items: []Template{{ID: "default", Enabled: true, Available: true}}}
	router := gin.New()
	NewServer(repository).RegisterAdminRoutes(router.Group("/api"))
	token := signedToken(t, "admin", "R_SUPER")

	for _, testCase := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/app-store/dashboard"},
		{http.MethodPut, "/api/app-store/templates/12/enable"},
		{http.MethodPut, "/api/app-store/templates/12/disable"},
	} {
		request := httptest.NewRequest(testCase.method, testCase.path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if responseCode(t, recorder) != 200 {
			t.Fatalf("%s %s response = %s", testCase.method, testCase.path, recorder.Body.String())
		}
	}
	if repository.enabledID != "12" || repository.disabledID != "12" {
		t.Fatalf("mutations not delegated: enabled=%q disabled=%q", repository.enabledID, repository.disabledID)
	}

	repository.enableErr = ClientError("模板不存在", nil)
	request := httptest.NewRequest(http.MethodPut, "/api/app-store/templates/99/enable", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if responseCode(t, recorder) != 400 {
		t.Fatalf("missing template response = %s", recorder.Body.String())
	}
}

func signedToken(t *testing.T, role, roleCode string) string {
	t.Helper()
	claims := middleware.Claims{
		UserID: 1, Username: "test-admin", Role: role, RoleCode: roleCode,
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(middleware.JWTSecret())
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func responseCode(t *testing.T, recorder *httptest.ResponseRecorder) int {
	t.Helper()
	var response struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid response: %s", recorder.Body.String())
	}
	return response.Code
}
