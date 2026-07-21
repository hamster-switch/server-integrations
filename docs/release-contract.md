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
`QuantumNous/new-api`) and requires an exact commit. The Release tag and signed
`release_id` must both equal `<component>-v<patch_version>`. A stable
`min_cli_version` newer than the running CLI is also a hard stop.

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
- Release IDs and patch versions are immutable after publication.
- A corrected payload receives a new version; Release assets are never replaced
  in place.
