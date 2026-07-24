# Release contract

`hamster-integrations` reads only non-draft, non-prerelease GitHub Releases.
The release must contain:

- `manifest.json`: UTF-8 JSON followed by one LF;
- `manifest.json.sig`: base64 Ed25519 signature over the exact manifest bytes;
- every asset declared by the signed manifest.

The CLI embeds this repository-specific Ed25519 public key:

```text
TDL6L4ETgOLaDXOOTlZVzipf5Xr6/YE/XBn0ZL2IBu0=
```

Each declared asset has a lowercase SHA-256 digest. A patch bundle is a gzip
tar archive containing regular files only. Its entry names and replacement
hashes are declared in the signed manifest; undeclared entries, links, oversized
entries, missing files, source hash drift and output hash drift are hard errors.
New files must be declared twice: the upstream fingerprint uses `absent: true`
and the patch entry uses `create: true`. Application fails if such a path
already exists, and rollback removes only those exact signed paths.

## Compatibility and non-bypassable failures

Compatibility is an exact match across repository/ref metadata, critical source
file hashes and anchor strings. `inspect` reports mismatches. `check` and
`update` stop. The CLI intentionally does not define `--force`, an arbitrary
shell hook, or a default-branch fallback.

The component fixes the upstream repository (`Wei-Shaw/sub2api` or
`QuantumNous/new-api`) and requires an exact commit. The optional signed
`channel` is restricted to a repository-defined allowlist. With no channel,
the Release tag and signed `release_id` must both equal
`<component>-v<patch_version>`; with a channel they must both equal
`<component>-<channel>-v<patch_version>`. Default release discovery continues
to select only the no-channel history. A channel Release must be selected by
its exact tag. A stable `min_cli_version` newer than the running CLI is also a
hard stop.

## Stored defaults and reactive editor compatibility

When a patch changes a persisted built-in template or draft, it must include an
exact-value migration for the old built-in value and a runtime compatibility
fallback for installations where migrations have not run yet. Both paths must
leave every customized value unchanged, and a contract test must keep the SQL
and runtime template constants identical.

Editors that derive controls from template metadata must be tested with legacy
missing, `null`, and blank presentation fields. Dynamic keys added after mount
must use framework-reactive membership checks, and mounted interaction tests
must focus each affected control and assert that its editable default appears.

Structured configuration remains an object at the API boundary. Escaped quotes
and newlines inside a JSON string are valid JSON encoding, not evidence of
double encoding. Regression tests must parse the editor value with a strict
JSON parser and parse the rendered YAML back to the expected object shape.

## Key rotation

A public-key change requires a reviewed CLI source release signed by the old
software distribution process. A patch Release alone cannot rotate the trust
root. The signing seed is stored only in the repository Actions secret
`HAMSTER_INTEGRATIONS_ED25519_PRIVATE_KEY`; it must never be committed or placed
in a Release asset.

## Release naming

- CLI/source tags: `cli-vMAJOR.MINOR.PATCH`.
- Component patch tags: `<component>-vMAJOR.MINOR.PATCH`.
- Component patch assets: `<component>-patch-<patch-version>.tar.gz`.
- Prebuilt image tags: `image-<component>-vMAJOR.MINOR.PATCH`.
- Independent channel patch tags:
  `<component>-<channel>-vMAJOR.MINOR.PATCH`.
- Independent channel patch assets:
  `<component>-<channel>-patch-<patch-version>.tar.gz`.
- Independent channel image tags:
  `image-<component>-<channel>-vMAJOR.MINOR.PATCH`.
- A prebuilt Release contains the compiled image archive, a Linux amd64 binary
  with embedded frontend, a version-bound installer, and `SHA256SUMS`; it is
  created once and never overwritten.
- In Compose mode, the prebuilt installer accepts only an existing absolute
  deployment target. It verifies the archive, loads the exact
  `hamster-switch/<component>:hs-v<version>` default-channel image or
  `hamster-switch/<component>:<channel>-v<version>` channel image, rewrites
  exactly one Compose service image, uses `--no-build`, waits for component
  health, and restores its Compose backup if deployment fails. New API uses an
  in-container HTTP probe of `/api/status` when the image has no Docker
  healthcheck; a merely running container is not sufficient.
- Automatic detection may match the component's Compose service, exact
  container name, or a custom image name containing the component. Once a
  container is selected, the verified image is applied to the exact Compose
  service and project paths reported by Docker labels. Ambiguous detection is
  rejected; administrators can provide both `--compose-file` and `--service`.
- When an existing component systemd service is loaded, automatic detection
  selects it before Compose. The installer takes the binary path from
  `ExecStart`, preserves owner and mode, keeps a timestamped backup, atomically
  replaces the binary, checks service state and HTTP health, and restores the
  previous binary on failure. It never rewrites the unit or environment file.
  Sub2api defaults to `SERVER_PORT`, port `8080`, and `/health`; New API defaults
  to `PORT`, port `3000`, and `/api/status`. Absolute `ExecStart` paths may
  contain spaces.
- A prebuilt systemd binary is compatible only with the upstream source and
  immutable migration history pinned by its signed source Release. Forks that
  changed an applied migration require a fork-built binary; installers and
  operators must not rewrite migration checksums to bypass startup validation.
- Release IDs and patch versions are immutable after publication.
- A corrected payload receives a new version; Release assets are never replaced
  in place.
