package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/hamster-switch/server-integrations/internal/contract"
	"github.com/hamster-switch/server-integrations/internal/target"
	"github.com/hamster-switch/server-integrations/internal/updater"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "patch-bundle-builder:", err)
		os.Exit(1)
	}
}

func run() error {
	manifestPath := flag.String("manifest", "", "manifest template path")
	upstreamRoot := flag.String("upstream", "", "exact pristine upstream root")
	resultRoot := flag.String("result", "", "fully patched result root")
	assetsDir := flag.String("assets", "", "release assets output directory")
	flag.Parse()
	if *manifestPath == "" || *upstreamRoot == "" || *resultRoot == "" || *assetsDir == "" {
		return errors.New("--manifest, --upstream, --result and --assets are required")
	}
	body, err := os.ReadFile(*manifestPath)
	if err != nil {
		return err
	}
	var manifest contract.Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return err
	}
	upstreamByPath := map[string]*contract.FileFingerprint{}
	for index := range manifest.Upstream.Files {
		upstreamByPath[manifest.Upstream.Files[index].Path] = &manifest.Upstream.Files[index]
	}
	patchByBundle := append([]contract.PatchFile(nil), manifest.Patch.Files...)
	sort.Slice(patchByBundle, func(i, j int) bool { return patchByBundle[i].BundlePath < patchByBundle[j].BundlePath })
	if err := os.MkdirAll(*assetsDir, 0o755); err != nil {
		return err
	}
	bundlePath := filepath.Join(*assetsDir, filepath.Base(manifest.Patch.BundleAsset))
	bundleFile, err := os.Create(bundlePath)
	if err != nil {
		return err
	}
	gzipWriter, err := gzip.NewWriterLevel(bundleFile, gzip.BestCompression)
	if err != nil {
		_ = bundleFile.Close()
		return err
	}
	gzipWriter.Header.ModTime = time.Unix(0, 0).UTC()
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	for patchIndex := range patchByBundle {
		patch := &patchByBundle[patchIndex]
		fingerprint := upstreamByPath[patch.Path]
		if fingerprint == nil {
			return fmt.Errorf("patch path %s has no upstream declaration", patch.Path)
		}
		upstreamPath := filepath.Join(*upstreamRoot, filepath.FromSlash(patch.Path))
		if patch.Create {
			if _, err := os.Stat(upstreamPath); !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("create path must be absent upstream: %s", patch.Path)
			}
			fingerprint.Absent, fingerprint.SHA256, patch.SourceSHA256 = true, "", ""
		} else {
			source, err := os.ReadFile(upstreamPath)
			if err != nil {
				return err
			}
			fingerprint.Absent, fingerprint.SHA256 = false, digest(source)
			patch.SourceSHA256 = fingerprint.SHA256
		}
		result, err := os.ReadFile(filepath.Join(*resultRoot, filepath.FromSlash(patch.Path)))
		if err != nil {
			return fmt.Errorf("read result %s: %w", patch.Path, err)
		}
		patch.ResultSHA256 = digest(result)
		header := &tar.Header{Name: filepath.ToSlash(patch.BundlePath), Mode: 0o644, Size: int64(len(result)), Typeflag: tar.TypeReg, ModTime: time.Unix(0, 0).UTC(), AccessTime: time.Unix(0, 0).UTC(), ChangeTime: time.Unix(0, 0).UTC()}
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tarWriter.Write(result); err != nil {
			return err
		}
		for index := range manifest.Patch.Files {
			if manifest.Patch.Files[index].Path == patch.Path {
				manifest.Patch.Files[index] = *patch
				break
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		return err
	}
	if err := gzipWriter.Close(); err != nil {
		return err
	}
	if err := bundleFile.Close(); err != nil {
		return err
	}
	bundleBody, err := os.ReadFile(bundlePath)
	if err != nil {
		return err
	}
	for index := range manifest.Assets {
		if manifest.Assets[index].Name == manifest.Patch.BundleAsset {
			manifest.Assets[index].SHA256 = digest(bundleBody)
		}
	}
	if err := updater.VerifyBundle(manifest, bundleBody); err != nil {
		return fmt.Errorf("verify deterministic bundle: %w", err)
	}
	report := target.Inspect(*upstreamRoot, manifest)
	if !report.Compatible {
		return fmt.Errorf("pristine upstream failed exact fingerprint or anchor inspection: %+v", report)
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return os.WriteFile(*manifestPath, encoded, 0o644)
}

func digest(body []byte) string { value := sha256.Sum256(body); return hex.EncodeToString(value[:]) }
