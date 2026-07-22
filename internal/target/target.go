package target

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hamster-switch/server-integrations/internal/contract"
)

type FileResult struct {
	Path           string `json:"path"`
	Expected       string `json:"expected_sha256"`
	AcceptedResult string `json:"accepted_result_sha256,omitempty"`
	Actual         string `json:"actual_sha256,omitempty"`
	Matches        bool   `json:"matches"`
	Installed      bool   `json:"installed,omitempty"`
	Error          string `json:"error,omitempty"`
	ExpectedAbsent bool   `json:"expected_absent,omitempty"`
}

type Report struct {
	Component    string       `json:"component"`
	Target       string       `json:"target"`
	Compatible   bool         `json:"compatible"`
	Installed    bool         `json:"installed"`
	Files        []FileResult `json:"files"`
	AnchorErrors []string     `json:"anchor_errors,omitempty"`
}

func Resolve(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("target is not accessible: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("target is not a directory: %s", absolute)
	}
	return filepath.Clean(absolute), nil
}

func Inspect(root string, manifest contract.Manifest) Report {
	report := Report{
		Component:  manifest.Component,
		Target:     root,
		Compatible: true,
		Installed:  len(manifest.Patch.Files) > 0,
	}
	patches := make(map[string]contract.PatchFile, len(manifest.Patch.Files))
	for _, patch := range manifest.Patch.Files {
		patches[patch.Path] = patch
	}
	contents := map[string][]byte{}
	installedPaths := map[string]bool{}
	for _, expected := range manifest.Upstream.Files {
		path := filepath.Join(root, filepath.FromSlash(expected.Path))
		body, err := os.ReadFile(path)
		result := FileResult{Path: expected.Path, Expected: expected.SHA256, ExpectedAbsent: expected.Absent}
		patch, patched := patches[expected.Path]
		if patched {
			result.AcceptedResult = patch.ResultSHA256
		}
		if expected.Absent {
			if errors.Is(err, os.ErrNotExist) {
				result.Matches = true
			} else if err != nil {
				result.Error = err.Error()
				report.Compatible = false
			} else {
				digest := sha256.Sum256(body)
				result.Actual = hex.EncodeToString(digest[:])
				result.Installed = patched && patch.Create && result.Actual == patch.ResultSHA256
				result.Matches = result.Installed
				installedPaths[expected.Path] = result.Installed
				if !result.Matches {
					result.Error = "path must be absent or match the signed patch result"
					report.Compatible = false
				}
			}
			if patched && !result.Installed {
				report.Installed = false
			}
			report.Files = append(report.Files, result)
			continue
		}
		if err != nil {
			result.Error = err.Error()
			report.Compatible = false
		} else {
			digest := sha256.Sum256(body)
			result.Actual = hex.EncodeToString(digest[:])
			result.Installed = patched && result.Actual == patch.ResultSHA256
			result.Matches = result.Actual == expected.SHA256 || result.Installed
			installedPaths[expected.Path] = result.Installed
			if !result.Matches {
				report.Compatible = false
			}
			if !result.Installed {
				contents[expected.Path] = body
			}
		}
		if patched && !result.Installed {
			report.Installed = false
		}
		report.Files = append(report.Files, result)
	}
	for _, anchor := range manifest.Upstream.Anchors {
		if installedPaths[anchor.Path] {
			continue
		}
		body, ok := contents[anchor.Path]
		if !ok || !strings.Contains(string(body), anchor.Contains) {
			report.AnchorErrors = append(report.AnchorErrors, fmt.Sprintf("%s: required anchor not found", anchor.Path))
			report.Compatible = false
		}
	}
	return report
}
