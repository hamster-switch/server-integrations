package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

func TestNewAPIBuildCommandsAreClosedAndDeterministic(t *testing.T) {
	tests := []struct {
		kind string
		name string
		args []string
	}{
		{kind: "bun-install", name: "bun", args: []string{"install", "--frozen-lockfile"}},
		{kind: "bun-build", name: "bun", args: []string{"run", "build"}},
		{kind: "go-build-root", name: "go", args: []string{"build", "-o", "new-api", "."}},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			name, args := buildCommand(tt.kind)
			if name != tt.name || !equalStrings(args, tt.args) {
				t.Fatalf("got %s %v, expected %s %v", name, args, tt.name, tt.args)
			}
		})
	}
}

func TestPreparedReleaseApplyAndRollback(t *testing.T) {
	manifestPath := os.Getenv("HAMSTER_PATCH_SMOKE_MANIFEST")
	upstreamRoot := os.Getenv("HAMSTER_PATCH_SMOKE_UPSTREAM")
	if manifestPath == "" || upstreamRoot == "" {
		t.Skip("set HAMSTER_PATCH_SMOKE_MANIFEST and HAMSTER_PATCH_SMOKE_UPSTREAM")
	}
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest contract.Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(filepath.Dir(manifestPath), "assets", manifest.Patch.BundleAsset)
	bundleBody, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	targetRoot := t.TempDir()
	stateRoot := t.TempDir()
	for _, file := range manifest.Upstream.Files {
		if file.Absent {
			continue
		}
		source, err := os.ReadFile(filepath.Join(upstreamRoot, filepath.FromSlash(file.Path)))
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(targetRoot, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, source, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := Apply(context.Background(), targetRoot, stateRoot, "manual", manifest, bundleBody); err != nil {
		t.Fatal(err)
	}
	for _, file := range manifest.Patch.Files {
		assertHash(t, filepath.Join(targetRoot, filepath.FromSlash(file.Path)), file.ResultSHA256)
	}
	if err := Rollback(context.Background(), stateRoot, manifest.Component); err != nil {
		t.Fatal(err)
	}
	for _, file := range manifest.Upstream.Files {
		path := filepath.Join(targetRoot, filepath.FromSlash(file.Path))
		if file.Absent {
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("created file survived rollback: %s (%v)", file.Path, err)
			}
			continue
		}
		assertHash(t, path, file.SHA256)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
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

func assertHash(t *testing.T, path, expected string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if actual := sum(body); actual != expected {
		t.Fatalf("hash for %s is %s, expected %s", path, actual, expected)
	}
}
