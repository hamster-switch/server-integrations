package target

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/hamster-switch/server-integrations/internal/contract"
)

func TestInspectRequiresHashAndAnchor(t *testing.T) {
	root := t.TempDir()
	body := []byte("before\nrequired anchor\nafter\n")
	if err := os.WriteFile(filepath.Join(root, "source.go"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	manifest := contract.Manifest{
		Component: "sub2api",
		Upstream: contract.Upstream{
			Files:   []contract.FileFingerprint{{Path: "source.go", SHA256: hex.EncodeToString(digest[:])}},
			Anchors: []contract.Anchor{{Path: "source.go", Contains: "required anchor"}},
		},
	}
	report := Inspect(root, manifest)
	if !report.Compatible {
		t.Fatalf("expected compatible report: %#v", report)
	}
	manifest.Upstream.Anchors[0].Contains = "missing"
	report = Inspect(root, manifest)
	if report.Compatible || len(report.AnchorErrors) != 1 {
		t.Fatalf("expected anchor mismatch: %#v", report)
	}
}

func TestInspectAcceptsDeclaredAbsentCreateTarget(t *testing.T) {
	root := t.TempDir()
	manifest := contract.Manifest{Component: "sub2api", Upstream: contract.Upstream{Files: []contract.FileFingerprint{{Path: "backend/new.go", Absent: true}}}}
	report := Inspect(root, manifest)
	if !report.Compatible || !report.Files[0].Matches {
		t.Fatalf("expected absent path to match: %#v", report)
	}
	if err := os.MkdirAll(filepath.Join(root, "backend"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "backend", "new.go"), []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	if Inspect(root, manifest).Compatible {
		t.Fatal("expected existing create target to be rejected")
	}
}

func TestInspectAcceptsExactInstalledPatchResults(t *testing.T) {
	root := t.TempDir()
	original := []byte("before\nrequired anchor\nafter\n")
	replacement := []byte("signed replacement without the old anchor\n")
	created := []byte("signed created file\n")
	writeTestFile(t, root, "source.go", replacement)
	writeTestFile(t, root, "backend/new.go", created)
	manifest := patchManifest(original, replacement, created)

	report := Inspect(root, manifest)
	if !report.Compatible || !report.Installed {
		t.Fatalf("expected installed patch to be compatible: %#v", report)
	}
	for _, file := range report.Files {
		if !file.Matches || !file.Installed {
			t.Fatalf("expected installed file result: %#v", file)
		}
	}
}

func TestInspectAcceptsPartialExactPatchResultsButDoesNotMarkInstalled(t *testing.T) {
	root := t.TempDir()
	original := []byte("before\nrequired anchor\nafter\n")
	replacement := []byte("signed replacement without the old anchor\n")
	created := []byte("signed created file\n")
	writeTestFile(t, root, "source.go", replacement)
	manifest := patchManifest(original, replacement, created)

	report := Inspect(root, manifest)
	if !report.Compatible || report.Installed {
		t.Fatalf("expected compatible partial patch result: %#v", report)
	}
}

func TestInspectRejectsDriftedPatchResult(t *testing.T) {
	root := t.TempDir()
	original := []byte("before\nrequired anchor\nafter\n")
	replacement := []byte("signed replacement without the old anchor\n")
	created := []byte("signed created file\n")
	writeTestFile(t, root, "source.go", []byte("locally changed\n"))
	writeTestFile(t, root, "backend/new.go", created)

	report := Inspect(root, patchManifest(original, replacement, created))
	if report.Compatible || report.Installed {
		t.Fatalf("expected drifted patch result to be rejected: %#v", report)
	}
}

func patchManifest(original, replacement, created []byte) contract.Manifest {
	return contract.Manifest{
		Component: "sub2api",
		Upstream: contract.Upstream{
			Files: []contract.FileFingerprint{
				{Path: "source.go", SHA256: hash(original)},
				{Path: "backend/new.go", Absent: true},
			},
			Anchors: []contract.Anchor{{Path: "source.go", Contains: "required anchor"}},
		},
		Patch: contract.Patch{Files: []contract.PatchFile{
			{Path: "source.go", SourceSHA256: hash(original), ResultSHA256: hash(replacement)},
			{Path: "backend/new.go", Create: true, ResultSHA256: hash(created)},
		}},
	}
}

func writeTestFile(t *testing.T, root, relative string, body []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func hash(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
