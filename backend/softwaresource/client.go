package softwaresource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"auto_pro/config"
)

const (
	catalogCacheTTL  = 5 * time.Minute
	catalogMaxBytes  = int64(4 << 20)
	templateMaxBytes = int64(2 << 20)
	previewMaxBytes  = int64(5 << 20)
)

var ErrUnavailable = errors.New("软件源服务暂时不可用")

type Author struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Email string `json:"email"`
}

type Source struct {
	ID         string     `json:"id"`
	LegacyID   *int64     `json:"legacyId,omitempty"`
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	State      string     `json:"state"`
	LastSyncAt *time.Time `json:"lastSyncAt,omitempty"`
}

type Template struct {
	ID            string    `json:"id"`
	TemplateKey   string    `json:"templateKey"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	PreviewURL    string    `json:"previewUrl"`
	Version       string    `json:"version"`
	Author        Author    `json:"author"`
	SchemaVersion int       `json:"schemaVersion"`
	SHA256        string    `json:"sha256"`
	ContentURL    string    `json:"contentUrl"`
	Source        Source    `json:"source"`
	Available     bool      `json:"available"`
	Published     bool      `json:"published"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type Catalog struct {
	FetchedAt time.Time  `json:"fetchedAt"`
	Revision  int64      `json:"revision"`
	Sources   []Source   `json:"sources"`
	Templates []Template `json:"templates"`
}

type ClientConfig struct {
	BaseURL    string
	CatalogKey string
	Timeout    time.Duration
	StaleTTL   time.Duration
	CacheDir   string
	HTTPClient *http.Client
}

type Client struct {
	baseURL    *url.URL
	catalogKey string
	staleTTL   time.Duration
	cachePath  string
	httpClient *http.Client
	mutex      sync.Mutex
	cached     Catalog
}

type APIError struct {
	Status    int
	Code      string
	Message   string
	RequestID string
}

func (err *APIError) Error() string {
	if err.Message != "" {
		return err.Message
	}
	return fmt.Sprintf("软件源服务返回 HTTP %d", err.Status)
}

var (
	defaultOnce   sync.Once
	defaultClient *Client
	defaultErr    error
	defaultMutex  sync.RWMutex
	testDefault   *Client
)

func Default() (*Client, error) {
	defaultMutex.RLock()
	override := testDefault
	defaultMutex.RUnlock()
	if override != nil {
		return override, nil
	}
	defaultOnce.Do(func() {
		defaultClient, defaultErr = NewClient(ClientConfig{
			BaseURL: config.GetSoftwareSourceURL(), CatalogKey: config.GetSoftwareSourceAPIKey(),
			Timeout:  config.GetSoftwareSourceTimeout(),
			StaleTTL: config.GetSoftwareSourceStaleTTL(),
			CacheDir: config.GetSoftwareSourceCacheDir(),
		})
	})
	return defaultClient, defaultErr
}

func SetDefaultForTest(client *Client) func() {
	defaultMutex.Lock()
	previous := testDefault
	testDefault = client
	defaultMutex.Unlock()
	return func() {
		defaultMutex.Lock()
		testDefault = previous
		defaultMutex.Unlock()
	}
}

func NewClient(clientConfig ClientConfig) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimRight(strings.TrimSpace(clientConfig.BaseURL), "/"))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" || (baseURL.Scheme != "http" && baseURL.Scheme != "https") {
		return nil, errors.New("软件源服务地址不合法")
	}
	if strings.TrimSpace(clientConfig.CatalogKey) == "" {
		return nil, errors.New("软件源目录 API Key 未配置")
	}
	if clientConfig.Timeout <= 0 {
		clientConfig.Timeout = 5 * time.Second
	}
	if clientConfig.StaleTTL <= 0 {
		clientConfig.StaleTTL = 24 * time.Hour
	}

	httpClient := clientConfig.HTTPClient
	if httpClient == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.MaxIdleConns = 20
		transport.MaxIdleConnsPerHost = 10
		transport.ResponseHeaderTimeout = clientConfig.Timeout
		httpClient = &http.Client{Transport: transport, Timeout: clientConfig.Timeout}
	}
	cachePath := ""
	if clientConfig.CacheDir != "" {
		cachePath = filepath.Join(clientConfig.CacheDir, "catalog.json")
	}
	return &Client{
		baseURL: baseURL, catalogKey: clientConfig.CatalogKey,
		staleTTL:  clientConfig.StaleTTL,
		cachePath: cachePath, httpClient: httpClient,
	}, nil
}

func (client *Client) Catalog(ctx context.Context) (Catalog, error) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if !client.cached.FetchedAt.IsZero() && time.Since(client.cached.FetchedAt) < catalogCacheTTL {
		return cloneCatalog(client.cached), nil
	}
	fresh, err := client.refresh(ctx)
	if err == nil {
		return cloneCatalog(fresh), nil
	}
	if !client.cached.FetchedAt.IsZero() && time.Since(client.cached.FetchedAt) <= client.staleTTL {
		return cloneCatalog(client.cached), nil
	}
	if disk, diskErr := client.loadDisk(); diskErr == nil && time.Since(disk.FetchedAt) <= client.staleTTL {
		client.cached = disk
		return cloneCatalog(disk), nil
	}
	return Catalog{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
}

func (client *Client) Refresh(ctx context.Context) (Catalog, error) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	fresh, err := client.refresh(ctx)
	if err != nil {
		return Catalog{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return cloneCatalog(fresh), nil
}

func (client *Client) refresh(ctx context.Context) (Catalog, error) {
	fresh, err := client.fetchCatalog(ctx)
	if err != nil {
		return Catalog{}, err
	}
	client.cached = fresh
	_ = client.persist(fresh)
	return fresh, nil
}

func (client *Client) Invalidate() {
	client.mutex.Lock()
	client.cached = Catalog{}
	client.mutex.Unlock()
}

func (client *Client) fetchCatalog(ctx context.Context) (Catalog, error) {
	var sourceData struct {
		List     []Source `json:"list"`
		Revision int64    `json:"revision"`
	}
	if err := client.catalogJSON(ctx, "/api/v1/catalog/sources", &sourceData); err != nil {
		return Catalog{}, err
	}
	templates, templateRevision, err := client.fetchTemplates(ctx)
	if err != nil {
		return Catalog{}, err
	}
	revision := maxInt64(sourceData.Revision, templateRevision)
	result := Catalog{FetchedAt: time.Now().UTC(), Revision: revision, Sources: sourceData.List, Templates: templates}
	if err := validateCatalog(result); err != nil {
		return Catalog{}, err
	}
	return result, nil
}

func (client *Client) fetchTemplates(ctx context.Context) ([]Template, int64, error) {
	items := make([]Template, 0)
	revision := int64(0)
	for page := 1; ; page++ {
		var data struct {
			List     []Template `json:"list"`
			Total    int        `json:"total"`
			Revision int64      `json:"revision"`
		}
		if err := client.catalogJSON(ctx, "/api/v1/catalog/templates?page="+strconv.Itoa(page)+"&pageSize=100", &data); err != nil {
			return nil, 0, err
		}
		items = append(items, data.List...)
		revision = data.Revision
		if len(items) >= data.Total || len(data.List) == 0 {
			return items, revision, nil
		}
	}
}

func (client *Client) FindTemplate(ctx context.Context, catalogID string) (Template, error) {
	catalog, err := client.Catalog(ctx)
	if err != nil {
		return Template{}, err
	}
	for _, item := range catalog.Templates {
		if item.ID == catalogID {
			return item, nil
		}
	}
	return Template{}, &APIError{Status: http.StatusNotFound, Code: "NOT_FOUND", Message: "模板不在软件源目录中"}
}

func (client *Client) TemplateContent(ctx context.Context, template Template) ([]byte, error) {
	payload, headers, err := client.catalogBytes(ctx, "/api/v1/catalog/templates/"+url.PathEscape(template.ID)+"/content", templateMaxBytes)
	if err != nil {
		return nil, err
	}
	checksum := sha256Hex(payload)
	declared := strings.TrimSpace(template.SHA256)
	if declared == "" {
		declared = strings.TrimSpace(headers.Get("X-Checksum-SHA256"))
	}
	if declared == "" || !secureEqual(strings.ToLower(declared), checksum) {
		return nil, errors.New("软件源模板 SHA256 校验失败")
	}
	return payload, nil
}

func (client *Client) TemplatePreview(ctx context.Context, catalogID string) ([]byte, string, error) {
	payload, headers, err := client.catalogBytes(ctx, "/api/v1/catalog/templates/"+url.PathEscape(catalogID)+"/preview", previewMaxBytes)
	return payload, headers.Get("Content-Type"), err
}

func (client *Client) catalogJSON(ctx context.Context, requestPath string, target any) error {
	return client.requestJSON(ctx, http.MethodGet, requestPath, client.catalogKey, nil, target, true)
}

func (client *Client) requestJSON(ctx context.Context, method, requestPath, key string, body, target any, retry bool) error {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	attempts := 1
	if retry {
		attempts = 2
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		response, err := client.do(ctx, method, requestPath, key, payload)
		if err != nil {
			lastErr = err
		} else {
			responsePayload, readErr := readLimited(response.Body, catalogMaxBytes)
			response.Body.Close()
			if readErr != nil {
				lastErr = readErr
			} else if response.StatusCode < 200 || response.StatusCode >= 300 {
				lastErr = decodeAPIError(response.StatusCode, responsePayload)
				if response.StatusCode < 500 {
					return lastErr
				}
			} else {
				var envelope struct {
					Data json.RawMessage `json:"data"`
				}
				if err := json.Unmarshal(responsePayload, &envelope); err != nil {
					return errors.New("软件源服务响应格式错误")
				}
				if target == nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
					return nil
				}
				if err := json.Unmarshal(envelope.Data, target); err != nil {
					return errors.New("软件源目录数据格式错误")
				}
				return nil
			}
		}
		if attempt+1 < attempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(100+rand.Intn(201)) * time.Millisecond):
			}
		}
	}
	return lastErr
}

func (client *Client) catalogBytes(ctx context.Context, requestPath string, maxBytes int64) ([]byte, http.Header, error) {
	response, err := client.do(ctx, http.MethodGet, requestPath, client.catalogKey, nil)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	payload, err := readLimited(response.Body, maxBytes)
	if err != nil {
		return nil, nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, nil, decodeAPIError(response.StatusCode, payload)
	}
	return payload, response.Header.Clone(), nil
}

func (client *Client) do(ctx context.Context, method, requestPath, key string, body []byte) (*http.Response, error) {
	requestURL := client.baseURL.ResolveReference(&url.URL{Path: requestPath})
	if strings.Contains(requestPath, "?") {
		parsed, err := url.Parse(requestPath)
		if err != nil {
			return nil, err
		}
		requestURL = client.baseURL.ResolveReference(parsed)
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json, application/octet-stream, image/*")
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("X-Software-Source-Key", key)
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("连接软件源服务失败: %w", err)
	}
	return response, nil
}

func decodeAPIError(status int, payload []byte) error {
	var response struct {
		Message   string `json:"msg"`
		Code      string `json:"error"`
		RequestID string `json:"requestId"`
	}
	_ = json.Unmarshal(payload, &response)
	if response.Message == "" {
		response.Message = fmt.Sprintf("软件源服务返回 HTTP %d", status)
	}
	return &APIError{Status: status, Code: response.Code, Message: response.Message, RequestID: response.RequestID}
}

func readLimited(reader io.Reader, maxBytes int64) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > maxBytes {
		return nil, errors.New("软件源服务响应超过大小限制")
	}
	return payload, nil
}

func (client *Client) persist(catalog Catalog) error {
	if client.cachePath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(client.cachePath), 0750); err != nil {
		return err
	}
	payload, err := json.Marshal(catalog)
	if err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(filepath.Dir(client.cachePath), ".catalog-*.tmp")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if err := tempFile.Chmod(0600); err != nil {
		tempFile.Close()
		return err
	}
	if _, err := tempFile.Write(payload); err != nil {
		tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, client.cachePath)
}

func (client *Client) loadDisk() (Catalog, error) {
	if client.cachePath == "" {
		return Catalog{}, os.ErrNotExist
	}
	payload, err := os.ReadFile(client.cachePath)
	if err != nil {
		return Catalog{}, err
	}
	var catalog Catalog
	if err := json.Unmarshal(payload, &catalog); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func cloneCatalog(source Catalog) Catalog {
	source.Sources = append([]Source(nil), source.Sources...)
	source.Templates = append([]Template(nil), source.Templates...)
	return source
}

func sha256Hex(payload []byte) string {
	checksum := sha256.Sum256(payload)
	return hex.EncodeToString(checksum[:])
}

func secureEqual(left, right string) bool {
	if len(left) == 0 || len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func maxInt64(values ...int64) int64 {
	var result int64
	for _, value := range values {
		if value > result {
			result = value
		}
	}
	return result
}

func validateCatalog(catalog Catalog) error {
	sourceIDs := make(map[string]struct{}, len(catalog.Sources))
	for _, source := range catalog.Sources {
		if strings.TrimSpace(source.ID) == "" || strings.TrimSpace(source.Name) == "" || strings.TrimSpace(source.Type) == "" {
			return errors.New("软件源目录 source 字段不完整")
		}
		sourceIDs[source.ID] = struct{}{}
	}
	templateIDs := make(map[string]struct{}, len(catalog.Templates))
	for _, template := range catalog.Templates {
		if template.ID == "" || template.TemplateKey == "" || template.Name == "" || template.Version == "" ||
			template.Source.ID == "" || template.SchemaVersion != 1 || len(template.SHA256) != 64 {
			return errors.New("软件源模板目录字段不完整或 schema 不兼容")
		}
		if _, exists := sourceIDs[template.Source.ID]; !exists {
			return errors.New("软件源模板引用了未知 source")
		}
		if _, err := hex.DecodeString(template.SHA256); err != nil {
			return errors.New("软件源模板 SHA256 格式错误")
		}
		if _, exists := templateIDs[template.ID]; exists {
			return errors.New("软件源模板目录包含重复 ID")
		}
		templateIDs[template.ID] = struct{}{}
	}
	return nil
}
