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
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hamster-switch/server-integrations/internal/contract"
)

type State struct {
	SchemaVersion int                 `json:"schema_version"`
	Component     string              `json:"component"`
	Target        string              `json:"target"`
	PatchVersion  string              `json:"patch_version"`
	ReleaseID     string              `json:"release_id"`
	BackupDir     string              `json:"backup_dir"`
	Mode          string              `json:"mode"`
	Deployment    contract.Deployment `json:"deployment"`
	AppliedAt     time.Time           `json:"applied_at"`
	LastResult    string              `json:"last_result"`
	CreatedFiles  []string            `json:"created_files,omitempty"`
}

func StatePath(stateRoot, component string) string {
	return filepath.Join(stateRoot, component, "state.json")
}

func LoadState(stateRoot, component string) (State, error) {
	body, err := os.ReadFile(StatePath(stateRoot, component))
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(body, &state); err != nil {
		return State{}, err
	}
	if state.SchemaVersion != 1 || state.Component != component {
		return State{}, errors.New("unsupported or mismatched local state")
	}
	return state, nil
}

func Apply(ctx context.Context, targetRoot, stateRoot, mode string, manifest contract.Manifest, bundle []byte) error {
	if !manifest.Deployment.Allows(mode) {
		return fmt.Errorf("deployment mode %q is not allowed by this release", mode)
	}
	payloads, err := unpack(bundle, manifest.Patch.Files)
	if err != nil {
		return err
	}
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	backupDir := filepath.Join(stateRoot, manifest.Component, "backups", stamp+"-"+manifest.PatchVersion)
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	for _, patch := range manifest.Patch.Files {
		source := filepath.Join(targetRoot, filepath.FromSlash(patch.Path))
		if patch.Create {
			if _, err := os.Lstat(source); !errors.Is(err, os.ErrNotExist) {
				if err == nil {
					return fmt.Errorf("create target already exists: %s", patch.Path)
				}
				return fmt.Errorf("inspect create target %s: %w", patch.Path, err)
			}
			continue
		}
		body, err := os.ReadFile(source)
		if err != nil {
			return fmt.Errorf("read source %s: %w", patch.Path, err)
		}
		if digest(body) != patch.SourceSHA256 {
			return fmt.Errorf("source drift detected for %s", patch.Path)
		}
		backup := filepath.Join(backupDir, filepath.FromSlash(patch.Path))
		if err := os.MkdirAll(filepath.Dir(backup), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(backup, body, 0o600); err != nil {
			return fmt.Errorf("backup %s: %w", patch.Path, err)
		}
	}
	state := State{
		SchemaVersion: 1, Component: manifest.Component, Target: targetRoot,
		PatchVersion: manifest.PatchVersion, ReleaseID: manifest.ReleaseID,
		BackupDir: backupDir, Mode: mode, Deployment: manifest.Deployment,
		AppliedAt: time.Now().UTC(), LastResult: "applying",
	}
	for _, patch := range manifest.Patch.Files {
		if patch.Create {
			state.CreatedFiles = append(state.CreatedFiles, patch.Path)
		}
	}
	if err := writeState(stateRoot, state); err != nil {
		return err
	}
	for _, patch := range manifest.Patch.Files {
		destination := filepath.Join(targetRoot, filepath.FromSlash(patch.Path))
		if err := atomicReplace(destination, payloads[patch.BundlePath], patch.ResultSHA256, patch.Create); err != nil {
			state.LastResult = "source replacement failed"
			restoreErr := restore(state)
			_ = writeState(stateRoot, state)
			if restoreErr != nil {
				return fmt.Errorf("replace source: %v; restore backup: %w", err, restoreErr)
			}
			return fmt.Errorf("replace source; backup restored: %w", err)
		}
	}
	if err := Deploy(ctx, state.Target, state.Mode, state.Deployment); err != nil {
		state.LastResult = "deployment failed"
		restoreErr := restore(state)
		var recoveryErr error
		if restoreErr == nil && state.Mode != "manual" {
			recoveryErr = Deploy(ctx, state.Target, state.Mode, state.Deployment)
		}
		_ = writeState(stateRoot, state)
		if restoreErr != nil {
			return fmt.Errorf("deployment failed: %v; restore backup failed: %w", err, restoreErr)
		}
		if recoveryErr != nil {
			return fmt.Errorf("deployment failed: %v; source restored but old service recovery failed: %w", err, recoveryErr)
		}
		return fmt.Errorf("deployment failed; source and old service were restored: %w", err)
	}
	state.LastResult = "success"
	return writeState(stateRoot, state)
}

func Rollback(ctx context.Context, stateRoot, component string) error {
	state, err := LoadState(stateRoot, component)
	if err != nil {
		return fmt.Errorf("load rollback state: %w", err)
	}
	if err := restore(state); err != nil {
		return err
	}
	if state.Mode != "manual" {
		if err := Deploy(ctx, state.Target, state.Mode, state.Deployment); err != nil {
			return fmt.Errorf("source restored but deployment recovery failed: %w", err)
		}
	}
	state.LastResult = "rolled back"
	return writeState(stateRoot, state)
}

func Deploy(ctx context.Context, root, mode string, deployment contract.Deployment) error {
	if mode == "manual" {
		return nil
	}
	for _, step := range deployment.BuildSteps {
		workdir := filepath.Join(root, filepath.FromSlash(step.Workdir))
		name, args := buildCommand(step.Kind)
		if err := run(ctx, workdir, name, args...); err != nil {
			return err
		}
	}
	switch mode {
	case "systemd":
		if err := run(ctx, root, "systemctl", "restart", deployment.Systemd.Service); err != nil {
			return err
		}
		if err := run(ctx, root, "systemctl", "is-active", "--quiet", deployment.Systemd.Service); err != nil {
			return err
		}
	case "docker-compose":
		compose := deployment.DockerCompose
		args := []string{"compose", "-f", filepath.Join(root, filepath.FromSlash(compose.File))}
		if compose.ProjectName != "" {
			args = append(args, "--project-name", compose.ProjectName)
		}
		if err := run(ctx, root, "docker", append(args, append([]string{"build"}, compose.Services...)...)...); err != nil {
			return err
		}
		if err := run(ctx, root, "docker", append(args, append([]string{"up", "-d"}, compose.Services...)...)...); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported deployment mode %q", mode)
	}
	if deployment.Health != nil {
		return health(ctx, *deployment.Health)
	}
	return nil
}

func buildCommand(kind string) (string, []string) {
	switch kind {
	case "go-build":
		return "go", []string{"build", "./..."}
	case "go-build-server":
		return "go", []string{"build", "-o", "../sub2api", "./cmd/server"}
	case "go-build-root":
		return "go", []string{"build", "-o", "new-api", "."}
	case "npm-ci":
		return "npm", []string{"ci"}
	case "npm-build":
		return "npm", []string{"run", "build"}
	case "pnpm-install":
		return "pnpm", []string{"install", "--frozen-lockfile"}
	case "pnpm-build":
		return "pnpm", []string{"run", "build"}
	case "bun-install":
		return "bun", []string{"install", "--frozen-lockfile"}
	case "bun-build":
		return "bun", []string{"run", "build"}
	default:
		panic("build step was not validated")
	}
}

func run(ctx context.Context, dir, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}

func health(ctx context.Context, check contract.HealthCheck) error {
	timeout := time.Duration(check.TimeoutSeconds) * time.Second
	if timeout <= 0 || timeout > 120*time.Second {
		timeout = 30 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, check.URL, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		return fmt.Errorf("health check returned HTTP %d", response.StatusCode)
	}
	return nil
}

func unpack(bundle []byte, files []contract.PatchFile) (map[string][]byte, error) {
	expected := make(map[string]contract.PatchFile, len(files))
	for _, file := range files {
		expected[filepath.ToSlash(file.BundlePath)] = file
	}
	gzipReader, err := gzip.NewReader(bytes.NewReader(bundle))
	if err != nil {
		return nil, fmt.Errorf("open patch bundle: %w", err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	payloads := map[string][]byte{}
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		name := filepath.ToSlash(filepath.Clean(header.Name))
		file, ok := expected[name]
		if !ok {
			return nil, fmt.Errorf("unexpected bundle entry %q", header.Name)
		}
		if header.Typeflag != tar.TypeReg || header.Size < 0 || header.Size > 32<<20 {
			return nil, fmt.Errorf("unsafe bundle entry %q", header.Name)
		}
		body, err := io.ReadAll(io.LimitReader(reader, header.Size+1))
		if err != nil || int64(len(body)) != header.Size {
			return nil, fmt.Errorf("read bundle entry %q", header.Name)
		}
		if digest(body) != file.ResultSHA256 {
			return nil, fmt.Errorf("result hash mismatch for %q", file.Path)
		}
		payloads[file.BundlePath] = body
	}
	if len(payloads) != len(files) {
		return nil, errors.New("patch bundle is incomplete")
	}
	return payloads, nil
}

// VerifyBundle checks the complete signed replacement set without writing to a target.
func VerifyBundle(manifest contract.Manifest, bundle []byte) error {
	_, err := unpack(bundle, manifest.Patch.Files)
	return err
}

func atomicReplace(path string, body []byte, expected string, create bool) error {
	info, err := os.Stat(path)
	mode := os.FileMode(0o644)
	if err != nil && !(create && errors.Is(err, os.ErrNotExist)) {
		return err
	}
	if err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary := path + fmt.Sprintf(".hamster-%d.tmp", time.Now().UnixNano())
	if err := os.WriteFile(temporary, body, mode); err != nil {
		return err
	}
	if digest(body) != expected {
		_ = os.Remove(temporary)
		return errors.New("replacement content failed hash check")
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func restore(state State) error {
	for _, relative := range state.CreatedFiles {
		destination := filepath.Join(state.Target, filepath.FromSlash(relative))
		if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove created file %s: %w", relative, err)
		}
	}
	return filepath.WalkDir(state.BackupDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(state.BackupDir, path)
		if err != nil || strings.HasPrefix(relative, "..") {
			return errors.New("invalid backup path")
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		destination := filepath.Join(state.Target, relative)
		info, err := os.Stat(destination)
		if err != nil {
			return err
		}
		temporary := destination + ".hamster-rollback.tmp"
		if err := os.WriteFile(temporary, body, info.Mode().Perm()); err != nil {
			return err
		}
		if runtime.GOOS == "windows" {
			_ = os.Remove(destination)
		}
		return os.Rename(temporary, destination)
	})
}

func writeState(stateRoot string, state State) error {
	path := StatePath(stateRoot, state.Component)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, body, 0o600); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	return os.Rename(temporary, path)
}

func digest(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
