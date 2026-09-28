package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSnapshotKey(t *testing.T, key ed25519.PrivateKey) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AUTO_PRO_DATA_DIR", dir)
	path := filepath.Join(dir, "store", "snapshot-ed25519.key")
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(key) + "\n"
	if err := os.WriteFile(path, []byte(encoded), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestOfficialSiteMatchesSnapshotKey(t *testing.T) {
	if embeddedStoreSnapshotPublicKey != productionStoreSnapshotPublicKey {
		t.Fatalf("compiled public key = %q", embeddedStoreSnapshotPublicKey)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	previous := embeddedStoreSnapshotPublicKey
	embeddedStoreSnapshotPublicKey = base64.StdEncoding.EncodeToString(publicKey)
	t.Cleanup(func() { embeddedStoreSnapshotPublicKey = previous })
	writeSnapshotKey(t, privateKey)
	if !officialSite() {
		t.Fatal("private key matching the compiled public key should be the official site")
	}
}

func TestOfficialSiteRejectsMismatchedKey(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	writeSnapshotKey(t, privateKey)
	if officialSite() {
		t.Fatal("a different private key must stay a client site")
	}
}

func TestOfficialSiteRejectsForgedPublicKeySuffix(t *testing.T) {
	want, err := base64.StdEncoding.DecodeString(strings.TrimSpace(embeddedStoreSnapshotPublicKey))
	if err != nil || len(want) != ed25519.PublicKeySize {
		t.Fatalf("compiled public key: %v", err)
	}
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	forged := make(ed25519.PrivateKey, 0, ed25519.PrivateKeySize)
	forged = append(forged, seed...)
	forged = append(forged, want...)
	writeSnapshotKey(t, forged)
	if officialSite() {
		t.Fatal("random seed plus the real public key must stay a client site")
	}
	if _, err := loadStoreSnapshotPrivateKey(); err == nil {
		t.Fatal("forged private key must not be accepted for signing")
	}
}

func TestOfficialSiteMissingKeyIsClient(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	if officialSite() {
		t.Fatal("missing private key must stay a client site")
	}
}
