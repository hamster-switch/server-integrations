package contract

import (
	"strings"
	"testing"
	"time"
)

const testHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func validManifest() Manifest {
	return Manifest{
		SchemaVersion: SchemaVersion,
		ReleaseID:     "sub2api-v1.0.0",
		Component:     "sub2api",
		PatchVersion:  "1.0.0",
		MinCLI:        "0.1.0",
		PublishedAt:   time.Now().UTC(),
		Upstream: Upstream{
			Repository: "https://github.com/Wei-Shaw/sub2api",
			Ref:        "v1.2.3",
			Commit:     "0123456789abcdef",
			Files:      []FileFingerprint{{Path: "backend/main.go", SHA256: testHash}},
			Anchors:    []Anchor{{Path: "backend/main.go", Contains: "package main"}},
		},
		Assets: []Asset{{Name: "sub2api-patch-1.0.0.tar.gz", SHA256: testHash, MediaType: "application/gzip"}},
		Patch: Patch{
			BundleAsset: "sub2api-patch-1.0.0.tar.gz",
			Files: []PatchFile{{
				Path: "backend/main.go", BundlePath: "payload/backend/main.go",
				SourceSHA256: testHash, ResultSHA256: testHash,
			}},
		},
		Deployment: Deployment{AllowedModes: []string{"manual"}},
	}
}

func TestManifestRejectsWrongRepositoryAndEscapingPath(t *testing.T) {
	manifest := validManifest()
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	manifest.Upstream.Repository = "https://github.com/example/lookalike"
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "must pin repository") {
		t.Fatalf("expected repository rejection, got %v", err)
	}
	manifest = validManifest()
	manifest.Patch.Files[0].Path = "../outside"
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "escapes target") {
		t.Fatalf("expected path rejection, got %v", err)
	}
}

func TestManifestRejectsArbitraryBuildStep(t *testing.T) {
	manifest := validManifest()
	manifest.Deployment.BuildSteps = []BuildStep{{Kind: "shell", Workdir: "."}}
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported build step") {
		t.Fatalf("expected arbitrary build rejection, got %v", err)
	}
}

func TestManifestReleaseChannels(t *testing.T) {
	legacy := validManifest()
	if got := legacy.ExpectedReleaseTag(); got != "sub2api-v1.0.0" {
		t.Fatalf("legacy release tag = %q", got)
	}
	if err := legacy.Validate(); err != nil {
		t.Fatalf("legacy manifest rejected: %v", err)
	}

	hamster := validManifest()
	hamster.Channel = "hamster"
	hamster.ReleaseID = "sub2api-hamster-v1.0.0"
	if got := hamster.ExpectedReleaseTag(); got != hamster.ReleaseID {
		t.Fatalf("hamster release tag = %q", got)
	}
	if err := hamster.Validate(); err != nil {
		t.Fatalf("hamster manifest rejected: %v", err)
	}
}

func TestManifestRejectsInvalidChannelAndReleaseID(t *testing.T) {
	manifest := validManifest()
	manifest.Channel = "preview"
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported release channel") {
		t.Fatalf("expected channel rejection, got %v", err)
	}

	manifest = validManifest()
	manifest.Channel = "hamster"
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "release_id must be") {
		t.Fatalf("expected release_id rejection, got %v", err)
	}
}

func TestManifestAcceptsExplicitCreateAndRejectsCreateHash(t *testing.T) {
	manifest := validManifest()
	manifest.Upstream.Files = append(manifest.Upstream.Files, FileFingerprint{Path: "backend/new.go", Absent: true})
	manifest.Patch.Files = append(manifest.Patch.Files, PatchFile{Path: "backend/new.go", BundlePath: "payload/backend/new.go", Create: true, ResultSHA256: testHash})
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid create rejected: %v", err)
	}
	manifest.Patch.Files[1].SourceSHA256 = testHash
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected create source hash to be rejected")
	}
}
