package handler

import (
	"archive/zip"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 存储管理接口。密钥只在保存和测试时从请求体读入，响应里只有「已保存」而没有明文。

type storageLocationInput struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Role      string `json:"role"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	Branch    string `json:"branch"`
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
	PathStyle bool   `json:"pathStyle"`
	AccessKey string `json:"accessKey"`
	Secret    string `json:"secret"`
}

func storageLocationView(loc storageLocation) gin.H {
	roleText := "备用"
	switch loc.Role {
	case storageRolePrimary:
		roleText = "主存储"
	case storageRoleDisabled:
		roleText = "停用"
	}
	kindText := "对象存储"
	switch loc.Kind {
	case packageStorageGitHub:
		kindText = "GitHub 私有仓库"
	case packageStorageGitee:
		kindText = "Gitee 私有仓库"
	case packageStorageWebDAV:
		kindText = "WebDAV 网盘"
	}
	return gin.H{
		"id": loc.ID, "name": loc.Name, "kind": loc.Kind, "kindText": kindText,
		"role": loc.Role, "roleText": roleText, "owner": loc.Owner, "repo": loc.Repo,
		"branch": loc.Branch, "endpoint": loc.Endpoint, "region": loc.Region,
		"bucket": loc.Bucket, "prefix": loc.KeyPrefix, "pathStyle": loc.PathStyle,
		"accessKey": maskSourceToken(loc.AccessKey), "hasSecret": loc.hasSecret(),
		"limitText": storageLimitText(loc.Kind), "legacy": loc.Legacy,
		"updatedAt": loc.UpdatedAt,
	}
}

func AdminStorageLocations(c *gin.Context) {
	blob, err := loadStorageBlob()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取存储位置失败"})
		return
	}
	list := make([]gin.H, 0, len(blob.Locations))
	for _, loc := range blob.Locations {
		list = append(list, storageLocationView(loc))
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"dualWrite": blob.DualWrite,
		"locations": list,
		"limits": gin.H{
			"github": storageLimitText(packageStorageGitHub),
			"gitee":  storageLimitText(packageStorageGitee),
			"s3":     storageLimitText(packageStorageS3),
			"webdav": storageLimitText(packageStorageWebDAV),
		},
		"reminder": storageReminder(blob),
	}})
}

func storageReminder(blob storageConfigBlob) string {
	if len(enabledStorageLocations(blob.Locations)) == 0 {
		return "安装包暂存在本站。请到存储管理添加主存储，测试连接后再保存。"
	}
	return ""
}

func AdminStorageLocationCreate(c *gin.Context) {
	var input storageLocationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	loc, secret, err := locationFromInput(storageLocation{}, input, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	blob, err := loadStorageBlob()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取存储位置失败"})
		return
	}
	now := time.Now().UTC()
	loc.ID = newStorageID()
	loc.CreatedAt = now
	loc.UpdatedAt = now
	if loc.Role == "" {
		if len(enabledStorageLocations(blob.Locations)) == 0 {
			loc.Role = storageRolePrimary
		} else {
			loc.Role = storageRoleBackup
		}
	}
	if loc.Role == storageRolePrimary {
		blob.Locations = demoteOtherPrimaries(blob.Locations, loc.ID)
	}
	blob.Locations = append(blob.Locations, loc)
	if err := saveStorageBlob(blob); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存存储位置失败"})
		return
	}
	_ = mirrorLegacyFromLocation(loc, secret)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已添加存储位置。密钥已加密，页面不能查看。", "data": storageLocationView(loc)})
}

func AdminStorageLocationUpdate(c *gin.Context) {
	var input storageLocationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	blob, err := loadStorageBlob()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取存储位置失败"})
		return
	}
	index := -1
	for i := range blob.Locations {
		if blob.Locations[i].ID == c.Param("id") {
			index = i
			break
		}
	}
	if index < 0 {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "找不到该存储位置"})
		return
	}
	loc, secret, err := locationFromInput(blob.Locations[index], input, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	loc.UpdatedAt = time.Now().UTC()
	if loc.Role == storageRolePrimary {
		blob.Locations = demoteOtherPrimaries(blob.Locations, loc.ID)
	}
	blob.Locations[index] = loc
	if err := saveStorageBlob(blob); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存存储位置失败"})
		return
	}
	if secret != "" {
		_ = mirrorLegacyFromLocation(loc, secret)
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已保存。密钥不能查看，留空表示不更换。", "data": storageLocationView(loc)})
}

func AdminStorageLocationDelete(c *gin.Context) {
	blob, err := loadStorageBlob()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取存储位置失败"})
		return
	}
	id := c.Param("id")
	for _, copy := range blob.Copies {
		if copy.LocationID == id {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "还有安装包记在这个存储上，请先复制到其他存储或删除那些文件"})
			return
		}
	}
	next := blob.Locations[:0]
	found := false
	for _, loc := range blob.Locations {
		if loc.ID == id {
			found = true
			continue
		}
		next = append(next, loc)
	}
	if !found {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "找不到该存储位置"})
		return
	}
	blob.Locations = next
	if err := saveStorageBlob(blob); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "删除存储位置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已删除存储位置"})
}

func AdminStorageOptions(c *gin.Context) {
	var body struct {
		DualWrite bool `json:"dualWrite"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	blob, err := loadStorageBlob()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取存储位置失败"})
		return
	}
	blob.DualWrite = body.DualWrite
	if err := saveStorageBlob(blob); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存失败"})
		return
	}
	msg := "上传时只写入一处，主存储失败再试备用"
	if body.DualWrite {
		msg = "上传成功后会同时再写一个备用存储"
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": msg, "data": gin.H{"dualWrite": blob.DualWrite}})
}

func AdminStorageTest(c *gin.Context) {
	var input storageLocationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	blob, _ := loadStorageBlob()
	current, _ := findStorageLocation(blob.Locations, storageFirstNonEmpty(c.Param("id"), input.ID))
	loc, secret, err := locationFromInput(current, input, strings.TrimSpace(input.Secret) == "" && current.ID == "")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	if secret == "" {
		secret, err = locationSecret(loc)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请填写密钥后再测试"})
			return
		}
	}
	if err := probeStorageLocation(c.Request.Context(), loc, secret); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "连接成功，读取和写入权限都可用"})
}

func locationFromInput(current storageLocation, input storageLocationInput, secretRequired bool) (storageLocation, string, error) {
	kind := normalizeStorageKind(input.Kind)
	if kind == "" {
		kind = current.Kind
	}
	if kind == "" {
		return storageLocation{}, "", errors.New("请选择存储类型")
	}
	loc := current
	loc.Kind = kind
	if name := strings.TrimSpace(input.Name); name != "" {
		loc.Name = name
	}
	if loc.Name == "" {
		loc.Name = defaultStorageName(kind)
	}
	if role := strings.TrimSpace(input.Role); role != "" {
		loc.Role = normalizeStorageRole(role)
	}
	loc.Owner = storageFirstNonEmpty(strings.TrimSpace(input.Owner), loc.Owner)
	loc.Repo = storageFirstNonEmpty(strings.TrimSpace(input.Repo), loc.Repo)
	loc.Branch = storageFirstNonEmpty(strings.TrimSpace(input.Branch), loc.Branch)
	loc.Endpoint = storageFirstNonEmpty(strings.TrimSpace(input.Endpoint), loc.Endpoint)
	loc.Region = storageFirstNonEmpty(strings.TrimSpace(input.Region), loc.Region)
	loc.Bucket = storageFirstNonEmpty(strings.TrimSpace(input.Bucket), loc.Bucket)
	if input.Prefix != "" || current.ID == "" {
		loc.KeyPrefix = strings.Trim(input.Prefix, "/")
	}
	loc.PathStyle = input.PathStyle
	if key := strings.TrimSpace(input.AccessKey); key != "" && !isMaskedSourceToken(key) {
		loc.AccessKey = key
	}
	secret := strings.TrimSpace(input.Secret)
	if secret != "" && !isMaskedSourceToken(secret) {
		sealed, err := sealStorageSecret(secret)
		if err != nil {
			return storageLocation{}, "", err
		}
		loc.SecretSealed = sealed
	} else if secretRequired || !loc.hasSecret() {
		return storageLocation{}, "", errors.New("请填写密钥。保存后不能查看，只能更换。")
	}
	if err := validateStorageLocation(loc); err != nil {
		return storageLocation{}, "", err
	}
	return loc, secret, nil
}

func defaultStorageName(kind string) string {
	switch kind {
	case packageStorageGitHub:
		return "GitHub 私有仓库"
	case packageStorageGitee:
		return "Gitee 私有仓库"
	case packageStorageWebDAV:
		return "WebDAV 网盘"
	default:
		return "对象存储"
	}
}

func validateStorageLocation(loc storageLocation) error {
	switch loc.Kind {
	case packageStorageGitHub, packageStorageGitee:
		if loc.Owner != "" && !sourceReleaseRepoPattern.MatchString(loc.Owner) {
			return errors.New("所有者不合法")
		}
		if loc.Repo != "" && !sourceReleaseRepoPattern.MatchString(loc.Repo) {
			return errors.New("仓库名不合法")
		}
		if loc.Owner == "" || loc.Repo == "" {
			return errors.New("请填写所有者和仓库名")
		}
	case packageStorageS3:
		if loc.Endpoint == "" || loc.Region == "" || loc.Bucket == "" || loc.AccessKey == "" {
			return errors.New("对象存储要填写 Endpoint、地域、Bucket 和 AccessKey")
		}
	case packageStorageWebDAV:
		if err := validateWebDAVEndpoint(loc.Endpoint); err != nil {
			return err
		}
		if strings.TrimSpace(loc.AccessKey) == "" {
			return errors.New("请填写用户名。应用令牌填在密码里，用户名仍要填账号。")
		}
	default:
		return errors.New("不支持的存储类型")
	}
	return nil
}

func storageFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func AdminStorageObjects(c *gin.Context) {
	blob, err := loadStorageBlob()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取存储位置失败"})
		return
	}
	loc, ok := findStorageLocation(blob.Locations, c.Param("id"))
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "找不到该存储位置"})
		return
	}
	secret, err := locationSecret(loc)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "该存储还没有密钥"})
		return
	}
	objects, err := listLocationObjects(c.Request.Context(), loc, secret)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	known := knownStorageRefs(blob)
	catalog := catalogObjectIndex()
	rows := make([]gin.H, 0, len(objects))
	for _, object := range objects {
		ref := objectRefOf(loc, object.Key)
		copy, registered := known[object.Key]
		if !registered {
			copy, registered = known[ref]
		}
		if !registered {
			copy, registered = known[object.Name]
		}
		label := catalog[ref]
		if label == "" {
			label = catalog[object.Key]
		}
		if label == "" && registered {
			label = copy.ItemID + " " + copy.Version
		}
		when := object.Updated
		sha := ""
		if registered {
			sha = copy.SHA256
			if when.IsZero() {
				when = copy.UploadedAt
			}
		}
		rows = append(rows, gin.H{
			"key": object.Key, "name": object.Name, "size": object.Size,
			"updatedAt": when, "item": label, "sha256": sha,
			"orphan": !registered && label == "", "tag": object.Tag,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": rows}})
}

func objectRefOf(loc storageLocation, key string) string {
	switch loc.Kind {
	case packageStorageGitHub:
		if ref, ok := parseGitHubPackageRef(githubPackagePrefix + key); ok {
			return formatGitHubPackageRef(ref)
		}
	case packageStorageGitee:
		if ref, ok := parseGiteePackageRef(giteePackagePrefix + key); ok {
			return formatGiteePackageRef(ref)
		}
	case packageStorageS3:
		return formatS3PackageRef(loc.ID, key)
	case packageStorageWebDAV:
		return formatWebDAVPackageRef(loc.ID, key)
	}
	return key
}

func AdminStorageZip(c *gin.Context) {
	loc, secret, key, ok := storageObjectTarget(c)
	if !ok {
		return
	}
	payload, err := readLocationObject(c.Request.Context(), loc, secret, key)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	files, preview, err := inspectZip(payload)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"files": files, "preview": preview}})
}

func inspectZip(payload []byte) ([]gin.H, gin.H, error) {
	if !isZipPayload(payload) {
		return nil, nil, errors.New("这不是 ZIP 压缩包")
	}
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return nil, nil, errors.New("无法读取压缩包")
	}
	files := make([]gin.H, 0, len(reader.File))
	preview := gin.H{}
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		files = append(files, gin.H{"name": file.Name, "size": file.UncompressedSize64})
		base := strings.ToLower(file.Name)
		if strings.Contains(base, "/") {
			base = base[strings.LastIndex(base, "/")+1:]
		}
		switch base {
		case "plugin.json", "template.json", "readme.md", "说明.txt", "manifest.json":
			if _, exists := preview[base]; exists {
				continue
			}
			rc, openErr := file.Open()
			if openErr != nil {
				continue
			}
			text, _ := io.ReadAll(io.LimitReader(rc, 16<<10))
			_ = rc.Close()
			preview[base] = string(text)
		}
	}
	return files, preview, nil
}

func AdminStorageDownload(c *gin.Context) {
	loc, secret, key, ok := storageObjectTarget(c)
	if !ok {
		return
	}
	if signed, safe, err := signedLocationURL(c.Request.Context(), loc, secret, key, 10*time.Minute); err == nil && safe && signed != "" {
		raw, readErr := readLocationObjectRaw(c.Request.Context(), loc, secret, key)
		if readErr == nil {
			if _, sharded := parseShardManifest(raw); !sharded {
				c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已生成 10 分钟内有效的签名地址", "data": gin.H{"url": signed, "expiresIn": 600}})
				return
			}
		}
	}
	ticket, err := signStorageTicket(loc.ID, key, 10*time.Minute)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "无法生成下载地址"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已生成 10 分钟内有效的下载地址", "data": gin.H{
		"url": "/api/v1/source/admin/storage/objects/raw?ticket=" + ticket, "expiresIn": 600,
	}})
}

func AdminStorageRaw(c *gin.Context) {
	id, key, err := openStorageTicket(c.Query("ticket"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	blob, err := loadStorageBlob()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取存储位置失败"})
		return
	}
	loc, ok := findStorageLocation(blob.Locations, id)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "找不到该存储位置"})
		return
	}
	secret, err := locationSecret(loc)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "密钥无法读取"})
		return
	}
	payload, err := readLocationObject(c.Request.Context(), loc, secret, key)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": err.Error()})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/octet-stream", payload)
}

func AdminStorageDeleteObject(c *gin.Context) {
	var body struct {
		LocationID string `json:"locationId"`
		Key        string `json:"key"`
		Confirm    string `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	name := body.Key
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		name = name[slash+1:]
	}
	if strings.TrimSpace(body.Confirm) != name {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请再次输入文件名以确认删除"})
		return
	}
	loc, secret, key, ok := storageObjectFrom(c, body.LocationID, body.Key)
	if !ok {
		return
	}
	if err := deleteLocationObject(c.Request.Context(), loc, secret, key); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	blob, err := loadStorageBlob()
	if err == nil {
		ref := objectRefOf(loc, key)
		kept := blob.Copies[:0]
		for _, copy := range blob.Copies {
			if copy.Ref == ref || copy.LocationID == loc.ID && strings.HasSuffix(copy.Ref, name) {
				continue
			}
			kept = append(kept, copy)
		}
		blob.Copies = kept
		_ = saveStorageBlob(blob)
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已删除。目录里的条目不会因此下架。"})
}

func AdminStorageVerify(c *gin.Context) {
	var body struct {
		LocationID string `json:"locationId"`
		Key        string `json:"key"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	loc, secret, key, ok := storageObjectFrom(c, body.LocationID, body.Key)
	if !ok {
		return
	}
	payload, err := readLocationObject(c.Request.Context(), loc, secret, key)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error() + "。条目仍在目录中，不会自动下架。"})
		return
	}
	sum := sha256.Sum256(payload)
	got := hex.EncodeToString(sum[:])
	blob, _ := loadStorageBlob()
	ref := objectRefOf(loc, key)
	expect := ""
	for _, copy := range blob.Copies {
		if copy.Ref == ref {
			expect = copy.SHA256
			break
		}
	}
	if expect != "" && !strings.EqualFold(expect, got) {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "校验码和登记的不一致。条目仍在目录中，不会自动下架。", "data": gin.H{"sha256": got, "expected": expect}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "校验通过", "data": gin.H{"sha256": got}})
}

func AdminStorageCopy(c *gin.Context) {
	var body struct {
		LocationID string `json:"locationId"`
		Key        string `json:"key"`
		TargetID   string `json:"targetId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	loc, secret, key, ok := storageObjectFrom(c, body.LocationID, body.Key)
	if !ok {
		return
	}
	payload, err := readLocationObject(c.Request.Context(), loc, secret, key)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	blob, err := loadStorageBlob()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取存储位置失败"})
		return
	}
	target, found := findStorageLocation(blob.Locations, body.TargetID)
	if !found {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "找不到目标存储"})
		return
	}
	targetSecret, err := locationSecret(target)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "目标存储还没有密钥"})
		return
	}
	kind, id, version := "plugin", "copied", strconv.FormatInt(time.Now().Unix(), 10)
	ref := objectRefOf(loc, key)
	for _, copy := range blob.Copies {
		if copy.Ref == ref {
			kind, id, version = copy.Kind, copy.ItemID, copy.Version
			break
		}
	}
	stored, err := putZipOnLocation(c.Request.Context(), target, targetSecret, kind, id, version, payload)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	recordStorageCopy(&blob, kind, id, version, target.ID, stored, payload)
	if err := saveStorageBlob(blob); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "已复制，但登记副本失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已复制到目标存储", "data": gin.H{"ref": stored}})
}

func AdminStorageHealth(c *gin.Context) {
	blob, err := loadStorageBlob()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取检查结果失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"checkedAt": blob.CheckedAt,
		"list":      blob.Health,
		"interval":  "每小时检查一次。出问题只提醒，不会自动下架。",
	}})
}

func AdminStorageHealthRun(c *gin.Context) {
	checkStorageLocations(c.Request.Context())
	AdminStorageHealth(c)
}

func storageObjectTarget(c *gin.Context) (storageLocation, string, string, bool) {
	return storageObjectFrom(c, c.Query("locationId"), c.Query("key"))
}

func storageObjectFrom(c *gin.Context, locationID, key string) (storageLocation, string, string, bool) {
	if c.Request.Method != http.MethodGet {
		var body struct {
			LocationID string `json:"locationId"`
			Key        string `json:"key"`
		}
		if locationID == "" || key == "" {
			_ = c.ShouldBindJSON(&body)
			if locationID == "" {
				locationID = body.LocationID
			}
			if key == "" {
				key = body.Key
			}
		}
	}
	blob, err := loadStorageBlob()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取存储位置失败"})
		return storageLocation{}, "", "", false
	}
	loc, ok := findStorageLocation(blob.Locations, locationID)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "找不到该存储位置"})
		return storageLocation{}, "", "", false
	}
	secret, err := locationSecret(loc)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "该存储还没有密钥"})
		return storageLocation{}, "", "", false
	}
	if strings.TrimSpace(key) == "" || strings.Contains(key, "..") {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "对象键不合法"})
		return storageLocation{}, "", "", false
	}
	return loc, secret, key, true
}

func signStorageTicket(locationID, key string, ttl time.Duration) (string, error) {
	secret, err := loadOrCreateStoreFileKey("storage-locations.key")
	if err != nil {
		return "", err
	}
	payload := locationID + "\n" + key + "\n" + strconv.FormatInt(time.Now().Add(ttl).Unix(), 10)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func openStorageTicket(ticket string) (string, string, error) {
	parts := strings.Split(ticket, ".")
	if len(parts) != 2 {
		return "", "", errors.New("下载地址无效或已过期")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", errors.New("下载地址无效或已过期")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", errors.New("下载地址无效或已过期")
	}
	secret, err := loadOrCreateStoreFileKey("storage-locations.key")
	if err != nil {
		return "", "", errors.New("下载地址无效或已过期")
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return "", "", errors.New("下载地址无效或已过期")
	}
	fields := strings.Split(string(payload), "\n")
	if len(fields) != 3 {
		return "", "", errors.New("下载地址无效或已过期")
	}
	expires, _ := strconv.ParseInt(fields[2], 10, 64)
	if time.Now().Unix() > expires {
		return "", "", errors.New("下载地址已过期，请重新获取")
	}
	return fields[0], fields[1], nil
}
