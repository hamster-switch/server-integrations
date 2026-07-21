package contract

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const SchemaVersion = 1

var (
	componentPattern = regexp.MustCompile(`^(sub2api|new-api)$`)
	sha256Pattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
	servicePattern   = regexp.MustCompile(`^[A-Za-z0-9_.@-]+$`)
	versionPattern   = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
)

type Manifest struct {
	SchemaVersion int          `json:"schema_version"`
	ReleaseID     string       `json:"release_id"`
	Component     string       `json:"component"`
	PatchVersion  string       `json:"patch_version"`
	MinCLI        string       `json:"min_cli_version"`
	PublishedAt   time.Time    `json:"published_at"`
	Upstream      Upstream     `json:"upstream"`
	Assets        []Asset      `json:"assets"`
	Patch         Patch        `json:"patch"`
	Deployment    Deployment   `json:"deployment"`
	Notes         []ImpactNote `json:"impact_notes"`
}

type Upstream struct {
	Repository string            `json:"repository"`
	Ref        string            `json:"ref"`
	Commit     string            `json:"commit"`
	Files      []FileFingerprint `json:"files"`
	Anchors    []Anchor          `json:"anchors"`
}

type FileFingerprint struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
	Absent bool   `json:"absent,omitempty"`
}

type Anchor struct {
	Path     string `json:"path"`
	Contains string `json:"contains"`
}

type Asset struct {
	Name      string `json:"name"`
	SHA256    string `json:"sha256"`
	MediaType string `json:"media_type"`
}

type Patch struct {
	BundleAsset string      `json:"bundle_asset"`
	Files       []PatchFile `json:"files"`
}

type PatchFile struct {
	Path         string `json:"path"`
	BundlePath   string `json:"bundle_path"`
	SourceSHA256 string `json:"source_sha256,omitempty"`
	ResultSHA256 string `json:"result_sha256"`
	Create       bool   `json:"create,omitempty"`
}

type Deployment struct {
	AllowedModes  []string       `json:"allowed_modes"`
	BuildSteps    []BuildStep    `json:"build_steps,omitempty"`
	Systemd       *SystemdConfig `json:"systemd,omitempty"`
	DockerCompose *ComposeConfig `json:"docker_compose,omitempty"`
	Health        *HealthCheck   `json:"health,omitempty"`
	ManualSteps   []string       `json:"manual_steps,omitempty"`
}

type BuildStep struct {
	Kind    string `json:"kind"`
	Workdir string `json:"workdir"`
}

type SystemdConfig struct {
	Service string `json:"service"`
}

type ComposeConfig struct {
	File        string   `json:"file"`
	ProjectName string   `json:"project_name,omitempty"`
	Services    []string `json:"services"`
}

type HealthCheck struct {
	URL            string `json:"url"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type ImpactNote struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported manifest schema %d", m.SchemaVersion)
	}
	if !componentPattern.MatchString(m.Component) {
		return fmt.Errorf("unsupported component %q", m.Component)
	}
	if strings.TrimSpace(m.ReleaseID) == "" || !versionPattern.MatchString(m.PatchVersion) || !versionPattern.MatchString(m.MinCLI) {
		return errors.New("release_id, patch_version and min_cli_version are required")
	}
	if m.PublishedAt.IsZero() || strings.TrimSpace(m.Upstream.Repository) == "" || strings.TrimSpace(m.Upstream.Ref) == "" {
		return errors.New("published_at and upstream repository/ref are required")
	}
	expectedRepository := map[string]string{
		"sub2api": "https://github.com/Wei-Shaw/sub2api",
		"new-api": "https://github.com/QuantumNous/new-api",
	}[m.Component]
	if strings.TrimSuffix(m.Upstream.Repository, ".git") != expectedRepository || strings.TrimSpace(m.Upstream.Commit) == "" {
		return fmt.Errorf("component %s must pin repository %s and an exact commit", m.Component, expectedRepository)
	}
	if len(m.Upstream.Files) == 0 || len(m.Upstream.Anchors) == 0 || len(m.Patch.Files) == 0 {
		return errors.New("upstream files, anchors and patch files must not be empty")
	}
	assets := make(map[string]Asset, len(m.Assets))
	for _, asset := range m.Assets {
		if asset.Name == "" || filepath.Base(asset.Name) != asset.Name || !sha256Pattern.MatchString(asset.SHA256) {
			return fmt.Errorf("invalid asset %q", asset.Name)
		}
		if _, exists := assets[asset.Name]; exists {
			return fmt.Errorf("duplicate asset %q", asset.Name)
		}
		assets[asset.Name] = asset
	}
	if _, ok := assets[m.Patch.BundleAsset]; !ok {
		return fmt.Errorf("patch bundle asset %q is not declared", m.Patch.BundleAsset)
	}
	seen := map[string]FileFingerprint{}
	for _, file := range m.Upstream.Files {
		if err := validateRelativePath(file.Path); err != nil || (file.Absent && file.SHA256 != "") || (!file.Absent && !sha256Pattern.MatchString(file.SHA256)) {
			return fmt.Errorf("invalid upstream file %q", file.Path)
		}
		if _, exists := seen[file.Path]; exists {
			return fmt.Errorf("duplicate upstream file %q", file.Path)
		}
		seen[file.Path] = file
	}
	for _, anchor := range m.Upstream.Anchors {
		if err := validateRelativePath(anchor.Path); err != nil || anchor.Contains == "" {
			return fmt.Errorf("invalid anchor for %q", anchor.Path)
		}
		if file, ok := seen[anchor.Path]; !ok || file.Absent {
			return fmt.Errorf("anchor path %q has no upstream fingerprint", anchor.Path)
		}
	}
	for _, file := range m.Patch.Files {
		if err := validateRelativePath(file.Path); err != nil {
			return err
		}
		if err := validateRelativePath(file.BundlePath); err != nil {
			return err
		}
		upstream, ok := seen[file.Path]
		if !ok || !sha256Pattern.MatchString(file.ResultSHA256) || (file.Create && (file.SourceSHA256 != "" || !upstream.Absent)) || (!file.Create && (!sha256Pattern.MatchString(file.SourceSHA256) || upstream.Absent || file.SourceSHA256 != upstream.SHA256)) {
			return fmt.Errorf("invalid patch hashes for %q", file.Path)
		}
	}
	return m.Deployment.validate()
}

func (d Deployment) validate() error {
	if len(d.AllowedModes) == 0 {
		return errors.New("at least one deployment mode is required")
	}
	seen := map[string]struct{}{}
	for _, mode := range d.AllowedModes {
		if mode != "manual" && mode != "systemd" && mode != "docker-compose" {
			return fmt.Errorf("unsupported deployment mode %q", mode)
		}
		if _, ok := seen[mode]; ok {
			return fmt.Errorf("duplicate deployment mode %q", mode)
		}
		seen[mode] = struct{}{}
	}
	for _, step := range d.BuildSteps {
		switch step.Kind {
		case "go-build", "go-build-server", "npm-ci", "npm-build", "pnpm-install", "pnpm-build":
		default:
			return fmt.Errorf("unsupported build step %q", step.Kind)
		}
		if step.Workdir != "" {
			if err := validateRelativePath(step.Workdir); err != nil {
				return err
			}
		}
	}
	if _, ok := seen["systemd"]; ok {
		if d.Systemd == nil || !servicePattern.MatchString(d.Systemd.Service) {
			return errors.New("systemd mode requires a safe service name")
		}
	}
	if _, ok := seen["docker-compose"]; ok {
		if d.DockerCompose == nil || len(d.DockerCompose.Services) == 0 {
			return errors.New("docker-compose mode requires services")
		}
		if err := validateRelativePath(d.DockerCompose.File); err != nil {
			return err
		}
		for _, service := range d.DockerCompose.Services {
			if !servicePattern.MatchString(service) {
				return fmt.Errorf("unsafe compose service %q", service)
			}
		}
	}
	return nil
}

func validateRelativePath(value string) error {
	if value == "" || filepath.IsAbs(value) {
		return fmt.Errorf("path must be relative: %q", value)
	}
	cleaned := filepath.Clean(filepath.FromSlash(value))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes target: %q", value)
	}
	return nil
}

func (d Deployment) Allows(mode string) bool {
	for _, allowed := range d.AllowedModes {
		if allowed == mode {
			return true
		}
	}
	return false
}
