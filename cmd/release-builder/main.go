package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hamster-switch/server-integrations/internal/contract"
	"github.com/hamster-switch/server-integrations/internal/release"
	"github.com/hamster-switch/server-integrations/internal/updater"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "release-builder:", err)
		os.Exit(1)
	}
}

func run() error {
	input := flag.String("manifest", "", "manifest JSON path")
	assetsDir := flag.String("assets", "", "release assets directory")
	output := flag.String("output", "", "output directory")
	flag.Parse()
	if *input == "" || *assetsDir == "" || *output == "" {
		return errors.New("--manifest, --assets and --output are required")
	}
	body, err := os.ReadFile(*input)
	if err != nil {
		return err
	}
	var manifest contract.Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return err
	}
	for index := range manifest.Assets {
		assetPath := filepath.Join(*assetsDir, filepath.Base(manifest.Assets[index].Name))
		assetBody, err := os.ReadFile(assetPath)
		if err != nil {
			return fmt.Errorf("read asset %s: %w", manifest.Assets[index].Name, err)
		}
		digest := sha256.Sum256(assetBody)
		manifest.Assets[index].SHA256 = hex.EncodeToString(digest[:])
	}
	bundle, err := os.ReadFile(filepath.Join(*assetsDir, filepath.Base(manifest.Patch.BundleAsset)))
	if err != nil {
		return err
	}
	if err := updater.VerifyBundle(manifest, bundle); err != nil {
		return fmt.Errorf("verify patch bundle: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	canonical, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	canonical = append(canonical, '\n')
	secret := strings.TrimSpace(os.Getenv("HAMSTER_INTEGRATIONS_ED25519_PRIVATE_KEY"))
	seed, err := base64.StdEncoding.DecodeString(secret)
	if err != nil || len(seed) != ed25519.SeedSize {
		return errors.New("signing secret must be a base64 Ed25519 seed")
	}
	signature := ed25519.Sign(ed25519.NewKeyFromSeed(seed), canonical)
	encoded := append([]byte(base64.StdEncoding.EncodeToString(signature)), '\n')
	if _, err := release.VerifyManifest(canonical, encoded); err != nil {
		return fmt.Errorf("signing key does not match the embedded release trust root: %w", err)
	}
	if err := os.MkdirAll(*output, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*output, "manifest.json"), canonical, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(*output, "manifest.json.sig"), encoded, 0o644)
}
