# Hamster Switch Server Integrations

Linux-only, administrator-confirmed patch delivery for the Hamster Switch
integrations maintained for sub2api and new-api.

## sub2api Hamster patch

The independently versioned `sub2api-hamster` channel adds Hamster Switch
subscription management to sub2api. Version `1.0.0` includes editable provider
icons and pricing multipliers (including a visible multiplier of `1`), strict
`settings_config` JSON handling, editable fields prefilled with their current
defaults, refreshed YAML provider discovery, and localized subscription
navigation. The historical `sub2api-v1.0.x` channel remains available but is
not replaced by this channel.

The updater consumes **formal GitHub Releases only**. Every release manifest is
signed with this repository's Ed25519 key and every downloaded asset is checked
with SHA-256 before it is inspected or applied. Unknown or locally drifted
upstream source trees are diagnostic-only: there is intentionally no force flag.

## Commands

```bash
hamster-integrations inspect sub2api --target /srv/sub2api
hamster-integrations check sub2api --target /srv/sub2api
hamster-integrations update sub2api --target /srv/sub2api --mode manual
hamster-integrations update sub2api --target /srv/sub2api --mode manual --release sub2api-hamster-v1.0.0
hamster-integrations rollback sub2api --target /srv/sub2api
```

`check` never applies a patch. `update` prints the exact affected files and
deployment actions, then requires an interactive confirmation. `manual` changes
only the verified source files and reports the remaining build/restart steps.
`systemd` and `docker-compose` accept only the structured, signed operations in
the release manifest; arbitrary shell hooks are not supported.

## Prebuilt image installation

Image Releases contain the compiled Docker image, a version-bound installer,
and `SHA256SUMS`. The installer verifies the image before `docker load`, changes
only the selected Compose service image, recreates that service with
`--no-build`, waits for health, and restores the Compose backup on failure.

The following one-line command installs `sub2api-hamster` v1.0.0. It loads a
prebuilt image, so the target server does not run `pnpm`, `go build`, or a local
Docker build:

```bash
tmp=$(mktemp -d) && cd "$tmp" && curl -fsSLO https://github.com/hamster-switch/server-integrations/releases/download/image-sub2api-hamster-v1.0.0/install-sub2api-hamster-v1.0.0.sh && curl -fsSLO https://github.com/hamster-switch/server-integrations/releases/download/image-sub2api-hamster-v1.0.0/SHA256SUMS && sha256sum -c SHA256SUMS --ignore-missing && sudo bash install-sub2api-hamster-v1.0.0.sh
```

Use `--target /absolute/path` when the deployment is elsewhere. The target must
already contain `deploy/docker-compose.yml`, `docker-compose.yml`, or
`compose.yml`; persistent database and Redis volumes are not modified.

## Release trust root

The embedded public key is documented in
[`docs/release-contract.md`](docs/release-contract.md). Its private counterpart
is stored only as the repository Actions secret
`HAMSTER_INTEGRATIONS_ED25519_PRIVATE_KEY`.

Prepared patch releases live under `releases/<component>/<version>` for the
historical default channel and `releases/<component>/<channel>/<version>` for
independent channels. Each includes an exact upstream fingerprint plus a
deterministic replacement bundle. It is not installable until the reviewed
commit is tagged and the signing workflow publishes the corresponding formal
Release.
