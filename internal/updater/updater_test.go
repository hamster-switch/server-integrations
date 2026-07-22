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

func TestApplyRejectsLateDriftBeforeWritingEarlierTarget(t *testing.T) {
	targetRoot := t.TempDir()
	stateRoot := t.TempDir()
	firstOriginal := []byte("first old")
	firstReplacement := []byte("first new")
	secondExpected := []byte("second expected")
	secondDrifted := []byte("second locally changed")
	writeUpdaterFile(t, targetRoot, "first.txt", firstOriginal)
	writeUpdaterFile(t, targetRoot, "second.txt", secondDrifted)
	manifest := contract.Manifest{
		Component: "new-api", PatchVersion: "1.0.0", ReleaseID: "test",
		Patch: contract.Patch{Files: []contract.PatchFile{
			{Path: "first.txt", BundlePath: "payload/first.txt", SourceSHA256: sum(firstOriginal), ResultSHA256: sum(firstReplacement)},
			{Path: "second.txt", BundlePath: "payload/second.txt", SourceSHA256: sum(secondExpected), ResultSHA256: sum([]byte("second new"))},
		}},
		Deployment: contract.Deployment{AllowedModes: []string{"manual"}},
	}
	bundle := multiBundle(t, map[string][]byte{
		"payload/first.txt":  firstReplacement,
		"payload/second.txt": []byte("second new"),
	})

	if err := Apply(context.Background(), targetRoot, stateRoot, "manual", manifest, bundle); err == nil {
		t.Fatal("expected late drift rejection")
	}
	assertFile(t, filepath.Join(targetRoot, "first.txt"), firstOriginal)
	assertFile(t, filepath.Join(targetRoot, "second.txt"), secondDrifted)
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

func TestManualApplyAcceptsExactInstalledResults(t *testing.T) {
	targetRoot := t.TempDir()
	stateRoot := t.TempDir()
	original := []byte("old")
	replacement := []byte("new")
	created := []byte("managed")
	writeUpdaterFile(t, targetRoot, "app.txt", replacement)
	writeUpdaterFile(t, targetRoot, "internal/new.go", created)
	manifest := contract.Manifest{
		Component: "sub2api", PatchVersion: "1.0.0", ReleaseID: "test",
		Patch: contract.Patch{Files: []contract.PatchFile{
			{Path: "app.txt", BundlePath: "payload/app.txt", SourceSHA256: sum(original), ResultSHA256: sum(replacement)},
			{Path: "internal/new.go", BundlePath: "payload/internal/new.go", Create: true, ResultSHA256: sum(created)},
		}},
		Deployment: contract.Deployment{AllowedModes: []string{"manual"}},
	}

	if err := Apply(context.Background(), targetRoot, stateRoot, "manual", manifest, multiBundle(t, map[string][]byte{
		"payload/app.txt":         replacement,
		"payload/internal/new.go": created,
	})); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(targetRoot, "app.txt"), replacement)
	assertFile(t, filepath.Join(targetRoot, "internal", "new.go"), created)
	if err := Rollback(context.Background(), stateRoot, "sub2api"); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(targetRoot, "app.txt"), replacement)
	assertFile(t, filepath.Join(targetRoot, "internal", "new.go"), created)
}

func bundle(t *testing.T, name string, body []byte) []byte {
	return multiBundle(t, map[string][]byte{name: body})
}

func multiBundle(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, body := range files {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func writeUpdaterFile(t *testing.T, root, relative string, body []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
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
