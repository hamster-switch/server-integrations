package release

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hamster-switch/server-integrations/internal/contract"
)

const PublicKeyBase64 = "TDL6L4ETgOLaDXOOTlZVzipf5Xr6/YE/XBn0ZL2IBu0="

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type githubRelease struct {
	TagName    string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

type Verified struct {
	Tag      string
	Manifest contract.Manifest
	Assets   map[string]Asset
}

type Client struct {
	HTTP    *http.Client
	BaseURL string
}

func NewClient() Client {
	return Client{
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		BaseURL: "https://api.github.com/repos/hamster-switch/server-integrations",
	}
}

func (c Client) Fetch(ctx context.Context, tag, component string) (Verified, error) {
	if tag != "" {
		body, err := c.get(ctx, c.BaseURL+"/releases/tags/"+url.PathEscape(tag))
		if err != nil {
			return Verified{}, err
		}
		var published githubRelease
		if err := json.Unmarshal(body, &published); err != nil {
			return Verified{}, fmt.Errorf("decode GitHub release: %w", err)
		}
		return c.verifyPublished(ctx, published, component)
	}
	body, err := c.get(ctx, c.BaseURL+"/releases?per_page=100")
	if err != nil {
		return Verified{}, err
	}
	var releases []githubRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return Verified{}, fmt.Errorf("decode GitHub releases: %w", err)
	}
	prefix := component + "-v"
	for _, published := range releases {
		if published.Draft || published.Prerelease || !strings.HasPrefix(published.TagName, prefix) {
			continue
		}
		verified, err := c.verifyPublished(ctx, published, component)
		if err != nil {
			return Verified{}, fmt.Errorf("refuse invalid formal release %s: %w", published.TagName, err)
		}
		return verified, nil
	}
	return Verified{}, fmt.Errorf("no formal %s patch release is available", component)
}

func (c Client) verifyPublished(ctx context.Context, published githubRelease, component string) (Verified, error) {
	if published.Draft || published.Prerelease {
		return Verified{}, errors.New("draft and prerelease releases are not accepted")
	}
	assets := make(map[string]Asset, len(published.Assets))
	for _, asset := range published.Assets {
		assets[asset.Name] = asset
	}
	manifestAsset, ok := assets["manifest.json"]
	if !ok {
		return Verified{}, errors.New("release does not contain manifest.json")
	}
	signatureAsset, ok := assets["manifest.json.sig"]
	if !ok {
		return Verified{}, errors.New("release does not contain manifest.json.sig")
	}
	manifestBytes, err := c.get(ctx, manifestAsset.BrowserDownloadURL)
	if err != nil {
		return Verified{}, fmt.Errorf("download manifest: %w", err)
	}
	signatureBytes, err := c.get(ctx, signatureAsset.BrowserDownloadURL)
	if err != nil {
		return Verified{}, fmt.Errorf("download manifest signature: %w", err)
	}
	manifest, err := VerifyManifest(manifestBytes, signatureBytes)
	if err != nil {
		return Verified{}, err
	}
	if err := verifyReleaseIdentity(published.TagName, manifest, component); err != nil {
		return Verified{}, err
	}
	for _, declared := range manifest.Assets {
		if _, ok := assets[declared.Name]; !ok {
			return Verified{}, fmt.Errorf("release is missing declared asset %q", declared.Name)
		}
	}
	return Verified{Tag: published.TagName, Manifest: manifest, Assets: assets}, nil
}

func verifyReleaseIdentity(publishedTag string, manifest contract.Manifest, component string) error {
	if manifest.Component != component {
		return fmt.Errorf("signed manifest component is %s, expected %s", manifest.Component, component)
	}
	expectedTag := manifest.ExpectedReleaseTag()
	if publishedTag != expectedTag || manifest.ReleaseID != expectedTag {
		return fmt.Errorf("release tag, release_id, channel and patch_version are not bound to the same component version")
	}
	return nil
}

func CheckMinimumCLI(current, required string) error {
	currentParts, err := parseVersion(current)
	if err != nil {
		return fmt.Errorf("invalid current CLI version: %w", err)
	}
	requiredParts, err := parseVersion(required)
	if err != nil {
		return fmt.Errorf("invalid minimum CLI version in signed manifest: %w", err)
	}
	for index := range currentParts {
		if currentParts[index] > requiredParts[index] {
			return nil
		}
		if currentParts[index] < requiredParts[index] {
			return fmt.Errorf("CLI %s is older than required version %s", current, required)
		}
	}
	return nil
}

func parseVersion(value string) ([3]int, error) {
	var parsed [3]int
	if strings.ContainsAny(value, "+-") {
		return parsed, errors.New("only stable MAJOR.MINOR.PATCH versions are accepted")
	}
	parts := strings.Split(strings.TrimPrefix(value, "v"), ".")
	if len(parts) != 3 {
		return parsed, errors.New("version must use MAJOR.MINOR.PATCH")
	}
	for index, part := range parts {
		if part == "" {
			return parsed, errors.New("empty version component")
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return parsed, errors.New("version contains a non-numeric component")
			}
			parsed[index] = parsed[index]*10 + int(character-'0')
		}
	}
	return parsed, nil
}

func (c Client) DownloadVerifiedAsset(ctx context.Context, verified Verified, name string) ([]byte, error) {
	var expected string
	for _, asset := range verified.Manifest.Assets {
		if asset.Name == name {
			expected = asset.SHA256
			break
		}
	}
	if expected == "" {
		return nil, fmt.Errorf("asset %q is not declared", name)
	}
	asset, ok := verified.Assets[name]
	if !ok {
		return nil, fmt.Errorf("asset %q is missing", name)
	}
	body, err := c.get(ctx, asset.BrowserDownloadURL)
	if err != nil {
		return nil, err
	}
	actual := sha256.Sum256(body)
	if hex.EncodeToString(actual[:]) != expected {
		return nil, fmt.Errorf("SHA-256 mismatch for %q", name)
	}
	return body, nil
}

func (c Client) get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "hamster-integrations")
	response, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request release endpoint: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return nil, fmt.Errorf("release endpoint returned HTTP %d", response.StatusCode)
	}
	const maxAssetSize = 256 << 20
	reader := io.LimitReader(response.Body, maxAssetSize+1)
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if len(body) > maxAssetSize {
		return nil, errors.New("release asset exceeds 256 MiB")
	}
	return body, nil
}

func VerifyManifest(manifestBytes, signatureBytes []byte) (contract.Manifest, error) {
	publicKey, err := base64.StdEncoding.DecodeString(PublicKeyBase64)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return contract.Manifest{}, errors.New("embedded release public key is invalid")
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(signatureBytes)))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return contract.Manifest{}, errors.New("manifest signature encoding is invalid")
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), manifestBytes, signature) {
		return contract.Manifest{}, errors.New("manifest signature verification failed")
	}
	var manifest contract.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return contract.Manifest{}, fmt.Errorf("decode signed manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return contract.Manifest{}, fmt.Errorf("invalid signed manifest: %w", err)
	}
	return manifest, nil
}
