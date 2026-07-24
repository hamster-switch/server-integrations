package updater

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamster-switch/server-integrations/internal/contract"
	"github.com/hamster-switch/server-integrations/internal/target"
)

func TestConfiguredPatchCandidateApplyAndRollback(t *testing.T) {
	manifestPath := strings.TrimSpace(os.Getenv("TEST_PATCH_MANIFEST"))
	bundlePath := strings.TrimSpace(os.Getenv("TEST_PATCH_BUNDLE"))
	targetRoot := strings.TrimSpace(os.Getenv("TEST_PATCH_TARGET"))
	if manifestPath == "" || bundlePath == "" || targetRoot == "" {
		t.Skip("TEST_PATCH_MANIFEST, TEST_PATCH_BUNDLE, and TEST_PATCH_TARGET are required")
	}

	manifestBody, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest contract.Manifest
	if err := json.Unmarshal(manifestBody, &manifest); err != nil {
		t.Fatal(err)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	bundle, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}

	before := target.Inspect(targetRoot, manifest)
	if !before.Compatible || before.Installed {
		t.Fatalf("target must be pristine and compatible before apply: %+v", before)
	}

	stateRoot := t.TempDir()
	applied := false
	t.Cleanup(func() {
		if applied {
			_ = Rollback(context.Background(), stateRoot, manifest.Component)
		}
	})
	if err := Apply(context.Background(), targetRoot, stateRoot, "manual", manifest, bundle); err != nil {
		t.Fatal(err)
	}
	applied = true

	afterApply := target.Inspect(targetRoot, manifest)
	if !afterApply.Compatible || !afterApply.Installed {
		t.Fatalf("target must contain the exact installed result after apply: %+v", afterApply)
	}
	state, err := LoadState(stateRoot, manifest.Component)
	if err != nil {
		t.Fatal(err)
	}
	if state.LastResult != "success" || filepath.Clean(state.Target) != filepath.Clean(targetRoot) {
		t.Fatalf("unexpected apply state: %+v", state)
	}

	if err := Rollback(context.Background(), stateRoot, manifest.Component); err != nil {
		t.Fatal(err)
	}
	applied = false
	afterRollback := target.Inspect(targetRoot, manifest)
	if !afterRollback.Compatible || afterRollback.Installed {
		t.Fatalf("target must be pristine and compatible after rollback: %+v", afterRollback)
	}
}
