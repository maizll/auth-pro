package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// WebDAV 网盘。Nextcloud、ownCloud、Seafile、Alist、Cloudreve 都提供这套接口。
// 自建 MinIO 不走这里，仍用 S3 兼容类型。
// 这类网盘没有短时签名地址。账号密码只放在服务端请求的 Authorization 里，
// 买家拿到的仍是官网票据。本站先取回文件、核对 sha256，再写给买家，避免把口令编进下载地址。

type webdavClient struct {
	Endpoint string
	Username string
	Password string
	Prefix   string
	HTTP     *http.Client
}

type webdavEntry struct {
	Key     string
	Size    int64
	Updated time.Time
	IsDir   bool
}

func webdavClientFor(loc storageLocation, secret string) (webdavClient, error) {
	if err := validateWebDAVEndpoint(loc.Endpoint); err != nil {
		return webdavClient{}, err
	}
	user := strings.TrimSpace(loc.AccessKey)
	secret = strings.TrimSpace(secret)
	if user == "" || secret == "" {
		return webdavClient{}, errors.New("请填写用户名和密码。应用令牌填在密码里，用户名仍要填账号。")
	}
	return webdavClient{
		Endpoint: loc.Endpoint, Username: user, Password: secret, Prefix: loc.KeyPrefix,
	}, nil
}

func validateWebDAVEndpoint(raw string) error {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return errors.New("网盘地址不合法，请填写 http 或 https 开头的 WebDAV 地址")
	}
	if parsed.User != nil {
		return errors.New("地址里不要写账号密码，请分开填写用户名和密码或应用令牌")
	}
	if strings.Contains(parsed.Path, "..") {
		return errors.New("网盘地址不合法")
	}
	return nil
}

func (c webdavClient) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{
		Timeout: 2 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || req == nil || req.URL == nil {
				return errors.New("网盘重定向过多")
			}
			// 跳到别的主机时丢掉口令，避免 Basic 认证被带到第三方。
			if len(via) > 0 && via[0].URL != nil && !strings.EqualFold(via[0].URL.Host, req.URL.Host) {
				req.Header.Del("Authorization")
			}
			return nil
		},
	}
}

func (c webdavClient) objectURL(key string) (string, error) {
	if err := validateWebDAVEndpoint(c.Endpoint); err != nil {
		return "", err
	}
	parsed, err := url.Parse(strings.TrimSpace(c.Endpoint))
	if err != nil {
		return "", errors.New("网盘地址不合法")
	}
	key = strings.TrimLeft(key, "/")
	if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	if key != "" {
		parsed.Path += encodeS3Path(key)
	}
	parsed.RawPath = parsed.Path
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (c webdavClient) do(ctx context.Context, method, key string, body []byte, contentType string, extra http.Header) (int, []byte, error) {
	rawURL, err := c.objectURL(key)
	if err != nil {
		return 0, nil, err
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return 0, nil, errors.New("无法连接网盘")
	}
	token := base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + c.Password))
	req.Header.Set("Authorization", "Basic "+token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for key, values := range extra {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return 0, nil, errors.New("无法连接网盘，请检查地址或稍后再试")
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, storageReadLimit+1))
	if err != nil {
		return resp.StatusCode, nil, errors.New("网盘响应不完整")
	}
	if int64(len(payload)) > storageReadLimit {
		return resp.StatusCode, nil, errors.New("网盘上的文件超过可读取的大小")
	}
	return resp.StatusCode, payload, nil
}

func webdavStatusText(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return errors.New("用户名或密码不正确。应用令牌填在密码里，用户名仍要填账号。")
	case http.StatusForbidden:
		return errors.New("这个账号没有该目录的权限")
	case http.StatusNotFound:
		return errors.New("找不到这个目录，请核对网盘地址和路径前缀")
	case http.StatusRequestEntityTooLarge:
		return errors.New("文件超过网盘的单次上传限制。本站已按 512MB 分片，请把网盘的上传限制调到不低于这个大小。")
	case http.StatusInsufficientStorage:
		return errors.New("网盘空间不足")
	default:
		if status >= 500 {
			return errors.New("网盘暂时不可用，请稍后再试")
		}
		if status >= 400 {
			return errors.New("网盘拒绝了请求，请检查地址、账号和读写权限")
		}
		return nil
	}
}

func (c webdavClient) propfind(ctx context.Context, key string, depth int) ([]webdavEntry, error) {
	header := make(http.Header)
	header.Set("Depth", strconv.Itoa(depth))
	body := []byte(`<?xml version="1.0" encoding="utf-8"?><D:propfind xmlns:D="DAV:"><D:prop><D:getcontentlength/><D:getlastmodified/><D:resourcetype/></D:prop></D:propfind>`)
	status, payload, err := c.do(ctx, "PROPFIND", key, body, "application/xml; charset=utf-8", header)
	if err != nil {
		return nil, err
	}
	if status != http.StatusMultiStatus && (status < 200 || status >= 300) {
		if text := webdavStatusText(status); text != nil {
			return nil, text
		}
		return nil, errors.New("无法读取网盘目录")
	}
	return parseWebDAVEntries(payload, c)
}

func parseWebDAVEntries(payload []byte, c webdavClient) ([]webdavEntry, error) {
	dec := xml.NewDecoder(bytes.NewReader(payload))
	var (
		entries        []webdavEntry
		current        webdavEntry
		inResponse     bool
		inResourceType bool
	)
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("无法读取网盘目录")
		}
		switch el := tok.(type) {
		case xml.StartElement:
			switch el.Name.Local {
			case "response":
				inResponse = true
				current = webdavEntry{}
			case "href":
				if !inResponse {
					continue
				}
				var href string
				if err := dec.DecodeElement(&href, &el); err == nil {
					current.Key = c.relativeKey(href)
				}
			case "getcontentlength":
				var text string
				if err := dec.DecodeElement(&text, &el); err == nil {
					current.Size, _ = strconv.ParseInt(strings.TrimSpace(text), 10, 64)
				}
			case "getlastmodified":
				var text string
				if err := dec.DecodeElement(&text, &el); err == nil {
					current.Updated, _ = http.ParseTime(strings.TrimSpace(text))
				}
			case "resourcetype":
				inResourceType = true
			case "collection":
				if inResourceType {
					current.IsDir = true
				}
			}
		case xml.EndElement:
			switch el.Name.Local {
			case "response":
				if inResponse {
					entries = append(entries, current)
				}
				inResponse = false
			case "resourcetype":
				inResourceType = false
			}
		}
	}
	return entries, nil
}

// listAll 按一层层 PROPFIND 收集文件。不用 Depth: infinity，不少网盘会拒绝无限深度。
func (c webdavClient) listAll(ctx context.Context) ([]webdavEntry, error) {
	return c.listFiles(ctx, strings.Trim(c.Prefix, "/"), 0)
}

func (c webdavClient) listFiles(ctx context.Context, key string, level int) ([]webdavEntry, error) {
	if level > 6 {
		return nil, nil
	}
	entries, err := c.propfind(ctx, key, 1)
	if err != nil {
		return nil, err
	}
	out := make([]webdavEntry, 0)
	self := strings.Trim(key, "/")
	for _, entry := range entries {
		if entry.Key == "" || entry.Key == self {
			continue
		}
		if entry.IsDir {
			nested, nestErr := c.listFiles(ctx, entry.Key, level+1)
			if nestErr != nil {
				return nil, nestErr
			}
			out = append(out, nested...)
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

func (c webdavClient) relativeKey(href string) string {
	href = strings.TrimSpace(href)
	path := href
	if parsed, err := url.Parse(href); err == nil && parsed.Path != "" {
		path = parsed.Path
	}
	if unescaped, err := url.PathUnescape(path); err == nil {
		path = unescaped
	}
	base, err := url.Parse(strings.TrimSpace(c.Endpoint))
	if err == nil {
		basePath := strings.TrimRight(base.Path, "/")
		path = strings.TrimPrefix(path, basePath)
	}
	return strings.Trim(path, "/")
}

func (c webdavClient) ensureParents(ctx context.Context, key string) error {
	parts := strings.Split(strings.Trim(key, "/"), "/")
	if len(parts) <= 1 {
		return nil
	}
	dir := ""
	for _, part := range parts[:len(parts)-1] {
		if part == "" || part == "." || part == ".." {
			return errors.New("对象键不合法")
		}
		if dir == "" {
			dir = part
		} else {
			dir += "/" + part
		}
		status, _, err := c.do(ctx, "MKCOL", dir, nil, "", nil)
		if err != nil {
			return err
		}
		// 201 是新建，405/301 常见于目录已经存在。
		if status == http.StatusCreated || status == http.StatusMethodNotAllowed || status == http.StatusMovedPermanently || status == http.StatusOK || status == http.StatusNoContent || status == http.StatusConflict {
			continue
		}
		if status >= 400 {
			if text := webdavStatusText(status); text != nil {
				return text
			}
			return errors.New("无法在网盘上创建目录，请检查写入权限和路径前缀")
		}
	}
	return nil
}

func (c webdavClient) put(ctx context.Context, key string, body []byte) error {
	if strings.Contains(key, "..") {
		return errors.New("对象键不合法")
	}
	if err := c.ensureParents(ctx, key); err != nil {
		return err
	}
	status, _, err := c.do(ctx, http.MethodPut, key, body, "application/octet-stream", nil)
	if err != nil {
		return err
	}
	if status == http.StatusOK || status == http.StatusCreated || status == http.StatusNoContent {
		return nil
	}
	if text := webdavStatusText(status); text != nil {
		return text
	}
	return errors.New("上传到网盘失败")
}

func (c webdavClient) get(ctx context.Context, key string) ([]byte, error) {
	status, payload, err := c.do(ctx, http.MethodGet, key, nil, "", nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, errors.New("在网盘里找不到该安装包")
	}
	if status < 200 || status >= 300 {
		if text := webdavStatusText(status); text != nil {
			return nil, text
		}
		return nil, errors.New("无法从网盘读取安装包")
	}
	return payload, nil
}

func (c webdavClient) delete(ctx context.Context, key string) error {
	status, _, err := c.do(ctx, http.MethodDelete, key, nil, "", nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || status == http.StatusNoContent || (status >= 200 && status < 300) {
		return nil
	}
	if text := webdavStatusText(status); text != nil {
		return text
	}
	return errors.New("删除网盘文件失败")
}

func probeWebDAV(ctx context.Context, loc storageLocation, secret string) error {
	client, err := webdavClientFor(loc, secret)
	if err != nil {
		return err
	}
	if _, err := client.propfind(ctx, strings.Trim(loc.KeyPrefix, "/"), 0); err != nil {
		return err
	}
	probeKey := joinStoragePrefix(loc.KeyPrefix, ".auth-pro-connection-test")
	if err := client.put(ctx, probeKey, []byte("ok")); err != nil {
		if strings.Contains(err.Error(), "用户名或密码") || strings.Contains(err.Error(), "没有该目录的权限") {
			return errors.New("可以读取，但不能写入。请给这个账号目录的写入权限。")
		}
		return err
	}
	if err := client.delete(ctx, probeKey); err != nil {
		return errors.New("写入测试文件后无法删除，请检查删除权限。")
	}
	return nil
}
