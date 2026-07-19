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
