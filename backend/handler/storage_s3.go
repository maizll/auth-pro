package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// S3 兼容协议的签名和请求。阿里云 OSS、腾讯云 COS、Cloudflare R2、MinIO 都走这一套 AWS Signature V4。
// 本文件不保存密钥，调用方传入已经解开的 secret。

type s3Object struct {
	Key          string
	Size         int64
	LastModified time.Time
}

type s3Client struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	Secret    string
	PathStyle bool
	HTTP      *http.Client
}

func (c s3Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return sourceReleaseHTTPClient
}

func (c s3Client) objectURL(key string, query url.Values) (string, error) {
	endpoint := strings.TrimSpace(c.Endpoint)
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", errors.New("对象存储地址不合法，请填写 https 开头的 Endpoint")
	}
	key = strings.TrimLeft(key, "/")
	if c.PathStyle {
		parsed.Path = "/" + c.Bucket + "/" + encodeS3Path(key)
	} else {
		parsed.Host = c.Bucket + "." + parsed.Host
		parsed.Path = "/" + encodeS3Path(key)
	}
	parsed.RawPath = parsed.Path
	if query != nil {
		parsed.RawQuery = query.Encode()
	}
	return parsed.String(), nil
}

func encodeS3Path(key string) string {
	parts := strings.Split(key, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func (c s3Client) do(ctx context.Context, method, key string, query url.Values, body []byte, contentType string) (int, []byte, http.Header, error) {
	return c.doLimited(ctx, method, key, query, body, contentType, 8<<20)
}

func (c s3Client) doLimited(ctx context.Context, method, key string, query url.Values, body []byte, contentType string, maxRead int64) (int, []byte, http.Header, error) {
	rawURL, err := c.objectURL(key, query)
	if err != nil {
		return 0, nil, nil, err
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return 0, nil, nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if err := signS3Request(req, c, body); err != nil {
		return 0, nil, nil, err
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return 0, nil, nil, errors.New("无法连接对象存储，请检查 Endpoint 或稍后再试")
	}
	defer resp.Body.Close()
	if maxRead <= 0 {
		maxRead = 8 << 20
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxRead+1))
	if err == nil && int64(len(payload)) > maxRead {
		return resp.StatusCode, nil, resp.Header, errors.New("对象超过可读取的大小")
	}
	if err != nil {
		return resp.StatusCode, nil, resp.Header, errors.New("对象存储响应不完整")
	}
	return resp.StatusCode, payload, resp.Header, nil
}

func signS3Request(req *http.Request, c s3Client, body []byte) error {
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	shortDate := now.Format("20060102")
	sum := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(sum[:])
	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	canonicalHeaders, signedHeaders := canonicalS3Headers(req.Header)
	canonical := strings.Join([]string{
		req.Method,
		canonicalS3URI(req.URL.EscapedPath()),
		canonicalS3Query(req.URL.Query()),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	scope := shortDate + "/" + c.Region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hexSHA256([]byte(canonical))
	key := hmacSHA256(hmacSHA256(hmacSHA256(hmacSHA256([]byte("AWS4"+c.Secret), shortDate), c.Region), "s3"), "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(key, stringToSign))
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", c.AccessKey, scope, signedHeaders, signature))
	return nil
}

func canonicalS3URI(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func canonicalS3Query(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		vals := append([]string(nil), values[key]...)
		sort.Strings(vals)
		for _, val := range vals {
			parts = append(parts, awsEscape(key)+"="+awsEscape(val))
		}
	}
	return strings.Join(parts, "&")
}

func canonicalS3Headers(header http.Header) (string, string) {
	keys := make([]string, 0, len(header))
	lower := map[string]string{}
	for key, values := range header {
		name := strings.ToLower(key)
		keys = append(keys, name)
		lower[name] = strings.TrimSpace(strings.Join(values, ","))
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+":"+lower[key])
	}
	return strings.Join(lines, "\n") + "\n", strings.Join(keys, ";")
}

func awsEscape(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

func hexSHA256(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func (c s3Client) presign(key string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if ttl > time.Hour {
		ttl = time.Hour
	}
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	shortDate := now.Format("20060102")
	scope := shortDate + "/" + c.Region + "/s3/aws4_request"
	query := url.Values{}
	query.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	query.Set("X-Amz-Credential", c.AccessKey+"/"+scope)
	query.Set("X-Amz-Date", amzDate)
	query.Set("X-Amz-Expires", fmt.Sprintf("%d", int(ttl.Seconds())))
	query.Set("X-Amz-SignedHeaders", "host")
	rawURL, err := c.objectURL(key, query)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	canonical := strings.Join([]string{
		http.MethodGet,
		canonicalS3URI(parsed.EscapedPath()),
		canonicalS3Query(parsed.Query()),
		"host:" + parsed.Host + "\n",
		"host",
		"UNSIGNED-PAYLOAD",
	}, "\n")
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hexSHA256([]byte(canonical))
	keyBytes := hmacSHA256(hmacSHA256(hmacSHA256(hmacSHA256([]byte("AWS4"+c.Secret), shortDate), c.Region), "s3"), "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(keyBytes, stringToSign))
	query.Set("X-Amz-Signature", signature)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func s3ErrorText(status int, body []byte) string {
	var payload struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	_ = xml.Unmarshal(body, &payload)
	code := strings.TrimSpace(payload.Code)
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden || code == "AccessDenied" || code == "InvalidAccessKeyId" || code == "SignatureDoesNotMatch":
		return "密钥无效，或没有这个桶的读写权限。请核对 AccessKey、Secret 和地域。"
	case status == http.StatusNotFound || code == "NoSuchBucket" || code == "NoSuchKey":
		return "找不到这个桶或对象，请核对 Bucket 和 Endpoint。"
	case status == http.StatusBadRequest && (code == "AuthorizationHeaderMalformed" || code == "InvalidArgument"):
		return "签名无法通过，请核对地域是否和 Endpoint 一致。"
	case status >= 500:
		return "对象存储暂时不可用，请稍后再试。"
	default:
		if msg := strings.TrimSpace(payload.Message); msg != "" {
			return "对象存储拒绝了请求：" + trimRunes(msg, 80)
		}
		return fmt.Sprintf("对象存储返回了状态码 %d", status)
	}
}

func trimRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

type s3ListResult struct {
	Contents []struct {
		Key          string `xml:"Key"`
		Size         int64  `xml:"Size"`
		LastModified string `xml:"LastModified"`
	} `xml:"Contents"`
	IsTruncated bool `xml:"IsTruncated"`
}

func (c s3Client) list(ctx context.Context, prefix string, max int) ([]s3Object, error) {
	if max <= 0 || max > 100 {
		max = 50
	}
	query := url.Values{}
	query.Set("list-type", "2")
	query.Set("max-keys", fmt.Sprintf("%d", max))
	if prefix != "" {
		query.Set("prefix", prefix)
	}
	status, body, _, err := c.do(ctx, http.MethodGet, "", query, nil, "")
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, errors.New(s3ErrorText(status, body))
	}
	var listed s3ListResult
	if err := xml.Unmarshal(body, &listed); err != nil {
		return nil, errors.New("无法读取对象列表")
	}
	out := make([]s3Object, 0, len(listed.Contents))
	for _, item := range listed.Contents {
		when, _ := time.Parse(time.RFC3339, item.LastModified)
		out = append(out, s3Object{Key: item.Key, Size: item.Size, LastModified: when})
	}
	return out, nil
}

func (c s3Client) put(ctx context.Context, key string, body []byte) error {
	status, payload, _, err := c.do(ctx, http.MethodPut, key, nil, body, "application/octet-stream")
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return errors.New(s3ErrorText(status, payload))
	}
	return nil
}

func (c s3Client) get(ctx context.Context, key string, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = 8 << 20
	}
	status, payload, _, err := c.doLimited(ctx, http.MethodGet, key, nil, nil, "", limit)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, errCatalogPackageMissing
	}
	if status < 200 || status >= 300 {
		return nil, errors.New(s3ErrorText(status, payload))
	}
	if limit > 0 && int64(len(payload)) > limit {
		return nil, errors.New("对象超过可读取的大小")
	}
	return payload, nil
}

func (c s3Client) head(ctx context.Context, key string) (bool, int64, error) {
	status, payload, header, err := c.do(ctx, http.MethodHead, key, nil, nil, "")
	if err != nil {
		return false, 0, err
	}
	if status == http.StatusNotFound {
		return false, 0, nil
	}
	if status < 200 || status >= 300 {
		return false, 0, errors.New(s3ErrorText(status, payload))
	}
	size := int64(0)
	if raw := header.Get("Content-Length"); raw != "" {
		fmt.Sscan(raw, &size)
	}
	return true, size, nil
}

func (c s3Client) delete(ctx context.Context, key string) error {
	status, payload, _, err := c.do(ctx, http.MethodDelete, key, nil, nil, "")
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || status == http.StatusNoContent || (status >= 200 && status < 300) {
		return nil
	}
	return errors.New(s3ErrorText(status, payload))
}
