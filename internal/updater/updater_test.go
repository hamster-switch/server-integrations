package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/hamster-switch/server-integrations/internal/contract"
)

func TestManualApplyAndRollback(t *testing.T) {
	targetRoot := t.TempDir()
	stateRoot := t.TempDir()
	original := []byte("old")
	replacement := []byte("new")
	path := filepath.Join(targetRoot, "app.txt")
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatal(err)
	}
	manifest := contract.Manifest{
		Component: "sub2api", PatchVersion: "1.0.0", ReleaseID: "test",
		Patch: contract.Patch{Files: []contract.PatchFile{{
			Path: "app.txt", BundlePath: "payload/app.txt",
			SourceSHA256: sum(original), ResultSHA256: sum(replacement),
		}}},
		Deployment: contract.Deployment{AllowedModes: []string{"manual"}},
	}
	if err := Apply(context.Background(), targetRoot, stateRoot, "manual", manifest, bundle(t, "payload/app.txt", replacement)); err != nil {
		t.Fatal(err)
	}
	assertFile(t, path, replacement)
	if err := Rollback(context.Background(), stateRoot, "sub2api"); err != nil {
		t.Fatal(err)
	}
	assertFile(t, path, original)
}

func TestApplyRejectsDriftBeforeModification(t *testing.T) {
	targetRoot := t.TempDir()
	stateRoot := t.TempDir()
	path := filepath.Join(targetRoot, "app.txt")
	if err := os.WriteFile(path, []byte("locally changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	replacement := []byte("new")
	manifest := contract.Manifest{
		Component: "new-api", PatchVersion: "1.0.0", ReleaseID: "test",
		Patch: contract.Patch{Files: []contract.PatchFile{{
			Path: "app.txt", BundlePath: "payload/app.txt",
			SourceSHA256: sum([]byte("expected")), ResultSHA256: sum(replacement),
		}}},
		Deployment: contract.Deployment{AllowedModes: []string{"manual"}},
	}
	if err := Apply(context.Background(), targetRoot, stateRoot, "manual", manifest, bundle(t, "payload/app.txt", replacement)); err == nil {
		t.Fatal("expected drift rejection")
	}
	assertFile(t, path, []byte("locally changed"))
}

func TestManualApplyCreatesAndRollbackRemovesDeclaredFile(t *testing.T) {
	targetRoot := t.TempDir()
	stateRoot := t.TempDir()
	replacement := []byte("new managed file")
	manifest := contract.Manifest{
		Component: "sub2api", PatchVersion: "1.0.0", ReleaseID: "test",
		Patch: contract.Patch{Files: []contract.PatchFile{{
			Path: "internal/new.go", BundlePath: "payload/internal/new.go", Create: true, ResultSHA256: sum(replacement),
		}}},
		Deployment: contract.Deployment{AllowedModes: []string{"manual"}},
	}
	path := filepath.Join(targetRoot, "internal", "new.go")
	if err := Apply(context.Background(), targetRoot, stateRoot, "manual", manifest, bundle(t, "payload/internal/new.go", replacement)); err != nil {
		t.Fatal(err)
	}
	assertFile(t, path, replacement)
	if err := Rollback(context.Background(), stateRoot, "sub2api"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("created file survived rollback: %v", err)
	}
}

func bundle(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func sum(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func assertFile(t *testing.T, path string, expected []byte) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, expected) {
		t.Fatalf("got %q, expected %q", body, expected)
	}
}
