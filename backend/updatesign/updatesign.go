// Package updatesign 是在线更新包的发布签名。
//
// 发布时（release.yml）用私钥给包内 manifest.json 签名：manifest 记下版本、安装路径和包内每个文件的 SHA256，
// 签名覆盖这些内容。程序内置公钥，下载完更新包后、解压之前逐个文件核对哈希并验签，
// 对不上就拒绝安装。签名在包里面，官网只原样存储和转发安装包，不参与签名，也拿不到私钥。
//
// 1.8.5 及更早的版本不认识 manifest.json 里的 files 和 signature 字段，解析时会忽略，仍按旧规则升级到 1.8.6。
// 1.8.6 起每次在线更新都必须带有效签名。
package updatesign

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// PublicKey 是发布签名公钥（标准 base64 的 32 字节 Ed25519 公钥）。
// 私钥只放在代码托管站的 Actions 机密 AUTH_PRO_UPDATE_SIGNING_KEY 和维护者本机备份里，不进仓库、不上官网服务器。
const PublicKey = "XdqSTM1ERgKflfBtxWnXAsFUU4b1OXBIqrqvRlPk0d4="

// ManifestName 是包根目录里的清单文件名。它自己不在 files 里。
const ManifestName = "manifest.json"

// ReleaseInfoName 是包内的发布信息（版本、适用端、更新说明），1.8.7 起打包时写入。
// 它和其它文件一样记在 files 里，受签名保护；在线更新只替换 backend/auth_pro，这个文件不会装到站点上。
const ReleaseInfoName = "backend/release.json"

// 包的适用端。
const (
	EditionOfficial = "official"
	EditionClient   = "client"
)

// ReleaseInfo 是 backend/release.json 的内容。
type ReleaseInfo struct {
	Version    string   `json:"version"`
	Edition    string   `json:"edition"`
	Channel    string   `json:"channel"`
	MinVersion string   `json:"minVersion"`
	ReleasedAt string   `json:"releasedAt"`
	Notes      []string `json:"notes"`
}

const (
	signaturePrefix  = "ed25519:"
	signedTextHeader = "auth-pro-update-v1"
	maxManifestBytes = 8 << 20
	maxReleaseInfo   = 256 << 10
)

var (
	// ErrUnsigned 表示包里没有发布签名（老打包方式或自己打的包）。
	ErrUnsigned = errors.New("更新包没有官方签名")
	// ErrBadSignature 表示签名不是这把公钥签的，或者签名覆盖的内容被改过。
	ErrBadSignature = errors.New("更新包签名不对")
	// ErrTampered 表示签名本身没问题，但包里的文件和签名时不一样（被改、被加或被删）。
	ErrTampered = errors.New("更新包内容和签名对不上")
)

// Manifest 是包根目录 manifest.json 的内容。前四个字段 1.6 起就有，在线更新解压后仍按它们安装。
type Manifest struct {
	Version       string            `json:"version"`
	FrontendDir   string            `json:"frontendDir"`
	BackendFile   string            `json:"backendFile"`
	RequiredFiles []string          `json:"requiredFiles"`
	Files         map[string]string `json:"files,omitempty"`
	Signature     string            `json:"signature,omitempty"`
}

// signedText 是签名覆盖的规范化文本：版本、安装路径、必需文件，再按路径排序列出每个文件的哈希。
// 改动任何一项（包括换版本号冒充新版本）签名都会失效。
func (m *Manifest) signedText() []byte {
	var b strings.Builder
	b.WriteString(signedTextHeader + "\n")
	b.WriteString("version=" + m.Version + "\n")
	b.WriteString("frontendDir=" + m.FrontendDir + "\n")
	b.WriteString("backendFile=" + m.BackendFile + "\n")
	for _, item := range m.RequiredFiles {
		b.WriteString("required=" + item + "\n")
	}
	names := make([]string, 0, len(m.Files))
	for name := range m.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		b.WriteString(m.Files[name] + " " + name + "\n")
	}
	return []byte(b.String())
}

// ParsePrivateKey 读取发布私钥：标准 base64 的 32 字节种子，首尾空白忽略。
func ParsePrivateKey(text string) (ed25519.PrivateKey, error) {
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("发布私钥必须是 32 字节种子的标准 base64")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// ParsePublicKey 读取标准 base64 的 32 字节公钥。
func ParsePublicKey(text string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("公钥必须是 32 字节的标准 base64")
	}
	return ed25519.PublicKey(raw), nil
}

// WriteManifest 给打包目录写 manifest.json：算出目录里每个文件的 SHA256，key 不为空时签名。
// 打包脚本只走这一处写清单，签不签名的格式一致。
func WriteManifest(dir, version string, key ed25519.PrivateKey) error {
	m := &Manifest{Version: version, FrontendDir: ".", BackendFile: "backend/auth_pro", RequiredFiles: []string{}, Files: map[string]string{}}
	err := filepath.WalkDir(dir, func(full string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dir, full)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() || rel == ManifestName {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("打包目录里不能有链接或特殊文件：%s", rel)
		}
		sum, err := hashFile(full)
		if err != nil {
			return err
		}
		m.Files[rel] = sum
		return nil
	})
	if err != nil {
		return err
	}
	if key != nil {
		m.Signature = signaturePrefix + base64.StdEncoding.EncodeToString(ed25519.Sign(key, m.signedText()))
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ManifestName), append(raw, '\n'), 0644)
}

// VerifyPackage 在解压之前核对 tar.gz：InspectPackage 通过，且签名的版本等于 version。
// 返回的错误可以用 errors.Is 区分 ErrUnsigned、ErrBadSignature、ErrTampered。
func VerifyPackage(packagePath, version string, pub ed25519.PublicKey) error {
	m, _, err := InspectPackage(packagePath, pub)
	if err != nil {
		return err
	}
	if m.Version != version {
		return fmt.Errorf("%w：签名的版本是 %s，不是 %s", ErrTampered, m.Version, version)
	}
	return nil
}

// InspectPackage 读一遍 tar.gz 并验签：清单签名用 pub 验证通过，包里每个普通文件都在清单里且哈希一致，
// 清单列出的文件一个不少，也没有重名文件。验过之后返回清单和包内发布信息（老包没有发布信息时为 nil）。
// 只读不写盘。上传更新包时靠它从包本身认出版本，不需要另外的签名文件。
func InspectPackage(packagePath string, pub ed25519.PublicKey) (*Manifest, *ReleaseInfo, error) {
	file, err := os.Open(packagePath)
	if err != nil {
		return nil, nil, fmt.Errorf("读取更新包失败：%w", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil, nil, fmt.Errorf("%w：不是有效的 tar.gz", ErrTampered)
	}
	defer gz.Close()

	var manifestRaw, infoRaw []byte
	seen := map[string]string{}
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("%w：包已损坏", ErrTampered)
		}
		name := path.Clean(strings.TrimPrefix(header.Name, "./"))
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != 0 {
			return nil, nil, fmt.Errorf("%w：包里有不允许的文件类型 %s", ErrTampered, header.Name)
		}
		if _, dup := seen[name]; dup || (name == ManifestName && manifestRaw != nil) {
			return nil, nil, fmt.Errorf("%w：包里有重名文件 %s", ErrTampered, name)
		}
		if name == ManifestName {
			manifestRaw, err = io.ReadAll(io.LimitReader(reader, maxManifestBytes+1))
			if err != nil || len(manifestRaw) > maxManifestBytes {
				return nil, nil, fmt.Errorf("%w：清单读取失败", ErrTampered)
			}
			continue
		}
		hash := sha256.New()
		var source io.Reader = reader
		var info bytes.Buffer
		if name == ReleaseInfoName {
			source = io.TeeReader(io.LimitReader(reader, maxReleaseInfo+1), &info)
		}
		if _, err := io.Copy(hash, source); err != nil {
			return nil, nil, fmt.Errorf("%w：包已损坏", ErrTampered)
		}
		if name == ReleaseInfoName {
			if info.Len() > maxReleaseInfo {
				return nil, nil, fmt.Errorf("%w：发布信息过大", ErrTampered)
			}
			infoRaw = info.Bytes()
		}
		seen[name] = hex.EncodeToString(hash.Sum(nil))
	}
	if manifestRaw == nil {
		return nil, nil, ErrUnsigned
	}
	var m Manifest
	if err := json.Unmarshal(bytes.TrimPrefix(manifestRaw, []byte{0xEF, 0xBB, 0xBF}), &m); err != nil {
		return nil, nil, fmt.Errorf("%w：清单不是有效的 JSON", ErrTampered)
	}
	encoded := strings.TrimPrefix(strings.TrimSpace(m.Signature), signaturePrefix)
	if encoded == "" || encoded == strings.TrimSpace(m.Signature) {
		return nil, nil, ErrUnsigned
	}
	sig, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(sig) != ed25519.SignatureSize || !ed25519.Verify(pub, m.signedText(), sig) {
		return nil, nil, ErrBadSignature
	}
	if len(seen) != len(m.Files) {
		return nil, nil, fmt.Errorf("%w：文件数量不一致", ErrTampered)
	}
	for name, sum := range seen {
		if m.Files[name] != sum {
			return nil, nil, fmt.Errorf("%w：%s", ErrTampered, name)
		}
	}
	if infoRaw == nil {
		return &m, nil, nil
	}
	var info ReleaseInfo
	if err := json.Unmarshal(bytes.TrimPrefix(infoRaw, []byte{0xEF, 0xBB, 0xBF}), &info); err != nil {
		return nil, nil, fmt.Errorf("%w：发布信息不是有效的 JSON", ErrTampered)
	}
	return &m, &info, nil
}

func hashFile(full string) (string, error) {
	file, err := os.Open(full)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
