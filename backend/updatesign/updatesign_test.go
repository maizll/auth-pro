package updatesign

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writePackageDir 造一个和正式包同样结构的打包目录。
func writePackageDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"index.html":       "<html>v</html>",
		"version.json":     `{"version":"9.9.9"}`,
		"assets/app.js":    "console.log(1)",
		"backend/auth_pro": "binary",
	}
	for name, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// tarDir 把目录打成 tar.gz；extra 里的条目追加在最后，用来模拟加文件或重名文件。
func tarDir(t *testing.T, dir string, extra map[string]string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "pkg.tar.gz")
	file, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	err = filepath.Walk(dir, func(full string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, full)
		body, _ := os.ReadFile(full)
		return writeEntry(tw, "./"+filepath.ToSlash(rel), string(body))
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range extra {
		if err := writeEntry(tw, name, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return out
}

func writeEntry(tw *tar.Writer, name, body string) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	_, err := tw.Write([]byte(body))
	return err
}

func newKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func TestVerifyPackage(t *testing.T) {
	pub, priv := newKey(t)
	otherPub, otherPriv := newKey(t)

	signed := writePackageDir(t)
	if err := WriteManifest(signed, "9.9.9", priv); err != nil {
		t.Fatal(err)
	}
	good := tarDir(t, signed, nil)

	t.Run("正常签名包通过", func(t *testing.T) {
		if err := VerifyPackage(good, "9.9.9", pub); err != nil {
			t.Fatalf("signed package rejected: %v", err)
		}
	})
	t.Run("换一把公钥就不认", func(t *testing.T) {
		if err := VerifyPackage(good, "9.9.9", otherPub); !errors.Is(err, ErrBadSignature) {
			t.Fatalf("want ErrBadSignature, got %v", err)
		}
	})
	t.Run("别的私钥签的包被拒", func(t *testing.T) {
		dir := writePackageDir(t)
		if err := WriteManifest(dir, "9.9.9", otherPriv); err != nil {
			t.Fatal(err)
		}
		if err := VerifyPackage(tarDir(t, dir, nil), "9.9.9", pub); !errors.Is(err, ErrBadSignature) {
			t.Fatalf("want ErrBadSignature, got %v", err)
		}
	})
	t.Run("没签名的包被拒", func(t *testing.T) {
		dir := writePackageDir(t)
		if err := WriteManifest(dir, "9.9.9", nil); err != nil {
			t.Fatal(err)
		}
		if err := VerifyPackage(tarDir(t, dir, nil), "9.9.9", pub); !errors.Is(err, ErrUnsigned) {
			t.Fatalf("want ErrUnsigned, got %v", err)
		}
	})
	t.Run("签名后改文件被拒", func(t *testing.T) {
		dir := writePackageDir(t)
		if err := WriteManifest(dir, "9.9.9", priv); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("evil()"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := VerifyPackage(tarDir(t, dir, nil), "9.9.9", pub); !errors.Is(err, ErrTampered) {
			t.Fatalf("want ErrTampered, got %v", err)
		}
	})
	t.Run("加文件被拒", func(t *testing.T) {
		pkg := tarDir(t, signed, map[string]string{"./assets/extra.js": "evil()"})
		if err := VerifyPackage(pkg, "9.9.9", pub); !errors.Is(err, ErrTampered) {
			t.Fatalf("want ErrTampered, got %v", err)
		}
	})
	t.Run("重名文件被拒", func(t *testing.T) {
		pkg := tarDir(t, signed, map[string]string{"./backend/auth_pro": "evil"})
		if err := VerifyPackage(pkg, "9.9.9", pub); !errors.Is(err, ErrTampered) {
			t.Fatalf("want ErrTampered, got %v", err)
		}
	})
	t.Run("删文件被拒", func(t *testing.T) {
		dir := writePackageDir(t)
		if err := WriteManifest(dir, "9.9.9", priv); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(dir, "assets", "app.js")); err != nil {
			t.Fatal(err)
		}
		if err := VerifyPackage(tarDir(t, dir, nil), "9.9.9", pub); !errors.Is(err, ErrTampered) {
			t.Fatalf("want ErrTampered, got %v", err)
		}
	})
	t.Run("拿旧版本的签名包冒充新版本被拒", func(t *testing.T) {
		if err := VerifyPackage(good, "9.9.10", pub); !errors.Is(err, ErrTampered) {
			t.Fatalf("want ErrTampered, got %v", err)
		}
	})
}

func TestBuiltInPublicKeyParses(t *testing.T) {
	if _, err := ParsePublicKey(PublicKey); err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePrivateKey("bm90LWEta2V5"); err == nil {
		t.Fatal("short private key accepted")
	}
}
