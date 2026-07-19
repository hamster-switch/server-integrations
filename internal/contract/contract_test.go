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
