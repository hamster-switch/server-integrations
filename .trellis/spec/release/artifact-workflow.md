# Artifact Workflow

## Source layout

Default-channel candidates live at `releases/<component>/<version>/`; independent channel candidates live at `releases/<component>/<channel>/<version>/`. Each contains `manifest.template.json` and a channel-qualified patch asset when applicable. The template pins the exact upstream repository, ref, commit, critical file fingerprints, anchors, patch file mapping, deployment operations, and impact notes.

Historical examples exist under `releases/sub2api/1.0.0` through `1.0.8` and `releases/new-api/1.0.0`; copy structure only after checking the current contract rather than carrying forward stale hashes or operations.

## Deterministic build and signing

- `cmd/patch-bundle-builder/main.go` must build from a pristine upstream tree plus a fully patched result tree. It sorts bundle entries and zeroes tar/gzip timestamps.
- `cmd/release-builder/main.go` recomputes asset hashes, verifies the bundle, validates the full manifest, writes indented UTF-8 JSON followed by one LF, and signs those exact bytes.
- `HAMSTER_INTEGRATIONS_ED25519_PRIVATE_KEY` is CI-only secret material. Never write it to source, task notes, logs, fixtures, or Release assets.
- The signing key must match `release.PublicKeyBase64`; key rotation requires a reviewed CLI release, not a patch manifest edit.

## Scenario: Build and validate a New API image

### 1. Scope / Trigger

- Trigger: a `new-api` patch changes `Dockerfile`, frontend files, or `.github/workflows/release-image.yml` and therefore requires a reproducible Linux amd64 image check.

### 2. Signatures

- Docker build arguments: `BUN_REGISTRY` defaults to `https://registry.npmjs.org`; `BUN_NETWORK_CONCURRENCY` defaults to `8`.
- Runtime contract: image entrypoint `/new-api`, port `3000`, health endpoint `GET /api/status`.
- Frontend validation commands: `bun run format:check` followed by `bun run copyright:check`.

### 3. Contracts

- The release workflow explicitly passes `BUN_REGISTRY=https://registry.npmjs.org`; local diagnostic builds may override the argument with a mirror without changing the committed default or workflow.
- Bun installs use `--frozen-lockfile`, the selected registry, and bounded network concurrency. Registry or integrity failure must fail the image build.
- Run the protected-header format check and copyright check sequentially. The formatter temporarily removes and restores protected headers, so concurrent execution can expose a transient headerless file to the copyright checker.
- Validate the resulting image as `linux/amd64`, extract `/new-api`, and require an HTTP 200 JSON response from `/api/status` before treating the candidate as usable.

### 4. Validation & Error Matrix

- Lockfile drift or dependency integrity failure -> Docker build fails; do not retry with an unfrozen install.
- Image architecture other than `linux/amd64` -> reject the candidate.
- Missing or non-x86-64 `/new-api` -> reject the candidate.
- `/api/status` timeout, non-200 response, or non-JSON body -> reject the candidate and remove the temporary container.
- Concurrent `format:check` and `copyright:check` -> invalid test orchestration even when a retry passes.

### 5. Good/Base/Bad Cases

- Good: CI builds with the official registry, bounded concurrency, then verifies the amd64 binary and live health endpoint.
- Base: a local build uses an explicit registry mirror for diagnosis but leaves Dockerfile defaults and CI arguments unchanged.
- Bad: remove `--frozen-lockfile`, raise concurrency without evidence, accept a build without live health validation, or run the two protected-header checks concurrently.

### 6. Tests Required

- Run `bun run format:check` and only after it exits run `bun run copyright:check`; assert both exit zero.
- Build with `docker build --platform linux/amd64` and the release workflow arguments; assert image architecture, `/new-api` machine type, and `GET /api/status` HTTP 200 JSON.
- Stop and remove every temporary validation container, including failure paths.

### 7. Wrong vs Correct

#### Wrong

```text
run format:check and copyright:check concurrently
docker build without --platform or a live health request
```

#### Correct

```text
bun run format:check
bun run copyright:check
docker build --platform linux/amd64 --build-arg BUN_REGISTRY=https://registry.npmjs.org ...
GET /api/status -> HTTP 200 JSON
```

## Publication boundaries

- CLI tags use `cli-vMAJOR.MINOR.PATCH`. Default patch/image tags use `<component>-vMAJOR.MINOR.PATCH` and `image-<component>-vMAJOR.MINOR.PATCH`; independent channels insert `-<channel>` before `-v`.
- `.github/workflows/release-patch.yml` requires an annotated pushed tag and publishes only after tests and signing succeed.
- `.github/workflows/release-image.yml` builds at the pinned upstream commit, applies the signed patch, creates the version-bound image and installer, and refuses an existing image Release.
- Treat published tags, manifests, bundles, images, installers, and checksums as immutable. Corrections always receive a new version.

## Scenario: Patch a cross-layer subscription contract

### 1. Scope / Trigger

- Trigger: a release patch changes the subscription template API, editor state, YAML renderer, or downstream YAML consumed by Hamster Switch.
- The patch must keep backend presentation fields, frontend editors, preview output, saved subscription output, and consumer parsing aligned.

### 2. Signatures

- Template state responses expose `provider_fields: string[]` so the UI renders only fields referenced by the active template.
- Provider presentation accepts `settings_config?: object`; a missing value preserves the generated default, while a present value overrides it.
- Group state may expose `default_settings_config: object` to initialize an editable override without replacing it with an empty object.

### 3. Contracts

- `settings_config` is structured JSON data at the API boundary and must serialize as valid JSON when embedded in YAML.
- Only documented provider tokens may be resolved in subscription output. For this contract, `{{provider.api_key}}` resolves to the subscription client key.
- Preview and published subscription rendering use the same renderer and validation path.
- Unsupported or incomplete groups are omitted silently; diagnostic text must never be appended to a valid YAML document.

### 4. Validation & Error Matrix

- Invalid JSON in the editor -> reject before save/preview with a field-level validation error.
- Unknown token in `settings_config` -> rendering fails; do not publish partial YAML.
- Missing required model mapping -> omit that group from preview and editor state.
- Renderer failure -> return a hard `RENDER_FAILED` diagnostic outside the YAML payload.

### 5. Good/Base/Bad Cases

- Good: a customized JSON object with multiline Codex TOML remains valid JSON after YAML rendering and resolves the API-key token.
- Base: absent `settings_config` uses the server-generated default configuration.
- Bad: adding presentation fields that the YAML template never reads, emitting raw `GROUP_PREVIEW_SKIPPED` text, or accepting unknown template tokens.

### 6. Tests Required

- Backend unit tests assert provider-field extraction, override precedence, strict JSON validity, multiline preservation, allowed token resolution, and unknown-token rejection.
- Handler tests assert preview/save/publish share field metadata and omit invalid groups without contaminating YAML.
- Frontend typecheck, lint, tests, and production build must pass with the dynamic editor fields.
- Consumer tests must parse the final subscription output successfully.

### 7. Wrong vs Correct

#### Wrong

```yaml
settings_config: {"config":"model = "custom""}
GROUP_PREVIEW_SKIPPED: group "default" must configure models
```

#### Correct

```yaml
settings_config: '{"auth":{"OPENAI_API_KEY":"resolved-key"},"config":"model = \"custom\"\n"}'
```
