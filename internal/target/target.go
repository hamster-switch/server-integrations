package target

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hamster-switch/server-integrations/internal/contract"
)

type FileResult struct {
	Path     string `json:"path"`
	Expected string `json:"expected_sha256"`
	Actual   string `json:"actual_sha256,omitempty"`
	Matches  bool   `json:"matches"`
	Error    string `json:"error,omitempty"`
}

type Report struct {
	Component    string       `json:"component"`
	Target       string       `json:"target"`
	Compatible   bool         `json:"compatible"`
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
	report := Report{Component: manifest.Component, Target: root, Compatible: true}
	contents := map[string][]byte{}
	for _, expected := range manifest.Upstream.Files {
		path := filepath.Join(root, filepath.FromSlash(expected.Path))
		body, err := os.ReadFile(path)
		result := FileResult{Path: expected.Path, Expected: expected.SHA256}
		if err != nil {
			result.Error = err.Error()
			report.Compatible = false
		} else {
			digest := sha256.Sum256(body)
			result.Actual = hex.EncodeToString(digest[:])
			result.Matches = result.Actual == expected.SHA256
			if !result.Matches {
				report.Compatible = false
			}
			contents[expected.Path] = body
		}
		report.Files = append(report.Files, result)
	}
	for _, anchor := range manifest.Upstream.Anchors {
		body, ok := contents[anchor.Path]
		if !ok || !strings.Contains(string(body), anchor.Contains) {
			report.AnchorErrors = append(report.AnchorErrors, fmt.Sprintf("%s: required anchor not found", anchor.Path))
			report.Compatible = false
		}
	}
	return report
}
