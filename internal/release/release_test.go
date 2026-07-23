package release

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/hamster-switch/server-integrations/internal/contract"
)

func TestVerifyManifestRejectsWrongSignature(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"schema_version":1}`)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, body))
	_, err = VerifyManifest(body, []byte(signature))
	if err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("expected signature failure, got %v", err)
	}
}

func TestVerifyReleaseIdentitySupportsLegacyAndHamsterChannels(t *testing.T) {
	for _, test := range []struct {
		name      string
		tag       string
		component string
		manifest  contract.Manifest
		wantErr   bool
	}{
		{
			name: "legacy",
			tag:  "sub2api-v1.0.0", component: "sub2api",
			manifest: contract.Manifest{Component: "sub2api", PatchVersion: "1.0.0", ReleaseID: "sub2api-v1.0.0"},
		},
		{
			name: "hamster",
			tag:  "sub2api-hamster-v1.0.0", component: "sub2api",
			manifest: contract.Manifest{Component: "sub2api", Channel: "hamster", PatchVersion: "1.0.0", ReleaseID: "sub2api-hamster-v1.0.0"},
		},
		{
			name: "wrong tag",
			tag:  "sub2api-v1.0.0", component: "sub2api",
			manifest: contract.Manifest{Component: "sub2api", Channel: "hamster", PatchVersion: "1.0.0", ReleaseID: "sub2api-hamster-v1.0.0"},
			wantErr:  true,
		},
		{
			name: "wrong component",
			tag:  "sub2api-hamster-v1.0.0", component: "new-api",
			manifest: contract.Manifest{Component: "sub2api", Channel: "hamster", PatchVersion: "1.0.0", ReleaseID: "sub2api-hamster-v1.0.0"},
			wantErr:  true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := verifyReleaseIdentity(test.tag, test.manifest, test.component)
			if (err != nil) != test.wantErr {
				t.Fatalf("verifyReleaseIdentity() = %v", err)
			}
		})
	}
}

func TestVerifyManifestRejectsMalformedSignature(t *testing.T) {
	_, err := VerifyManifest([]byte(`{}`), []byte("not-base64"))
	if err == nil || !strings.Contains(err.Error(), "encoding") {
		t.Fatalf("expected encoding failure, got %v", err)
	}
}

func TestCheckMinimumCLI(t *testing.T) {
	for _, test := range []struct {
		current  string
		required string
		wantErr  bool
	}{
		{current: "0.1.0", required: "0.1.0"},
		{current: "1.0.0", required: "0.9.9"},
		{current: "0.1.0", required: "0.2.0", wantErr: true},
		{current: "0.1.0-dev", required: "0.1.0", wantErr: true},
	} {
		err := CheckMinimumCLI(test.current, test.required)
		if (err != nil) != test.wantErr {
			t.Fatalf("CheckMinimumCLI(%q, %q) = %v", test.current, test.required, err)
		}
	}
}
