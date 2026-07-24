# Hamster Switch Server Integrations

Linux-only, administrator-confirmed patch delivery for the Hamster Switch
integrations maintained for sub2api and new-api.

## sub2api Hamster patch

The independently versioned `sub2api-hamster` channel adds Hamster Switch
subscription management to sub2api. Version `1.0.2` includes editable provider
icons and pricing multipliers (including a visible multiplier of `1`), strict
`settings_config` JSON handling, editable fields prefilled with their current
defaults, refreshed YAML provider discovery, and localized subscription
navigation. It also provides one prebuilt installer for both Docker Compose and
systemd deployments. The historical `sub2api-v1.0.x` channel remains available
but is not replaced by this channel.

## New API Hamster patch

The independent `new-api-hamster` channel adds stable Hamster Switch
configuration subscriptions backed by one billing token per selected group and
one fixed-channel provider credential per group/channel binding. Version
`1.0.0` is pinned to
`QuantumNous/new-api@1721144221ec5c94dd87891a7ae1bee228e7bb63` and includes
the current React frontend, templates, signed tutorials, dynamic providers,
strict structured settings, and administrator controls.

This channel does not migrate or support the historical `new-api-v1.0.0`
patch, its `5a6c53d` source baseline, tables, or `hs_` URLs. Install it only on
the exact pristine upstream commit named above. A drifted fork requires a new
patch version and its own prebuilt assets.

The updater consumes **formal GitHub Releases only**. Every release manifest is
signed with this repository's Ed25519 key and every downloaded asset is checked
with SHA-256 before it is inspected or applied. Unknown or locally drifted
upstream source trees are diagnostic-only: there is intentionally no force flag.

## Commands

```bash
hamster-integrations inspect sub2api --target /srv/sub2api
hamster-integrations check sub2api --target /srv/sub2api
hamster-integrations update sub2api --target /srv/sub2api --mode manual
hamster-integrations update sub2api --target /srv/sub2api --mode manual --release sub2api-hamster-v1.0.2
hamster-integrations rollback sub2api --target /srv/sub2api
hamster-integrations inspect new-api --target /srv/new-api --release new-api-hamster-v1.0.0
hamster-integrations check new-api --target /srv/new-api --release new-api-hamster-v1.0.0
hamster-integrations update new-api --target /srv/new-api --mode manual --release new-api-hamster-v1.0.0
```

`check` never applies a patch. `update` prints the exact affected files and
deployment actions, then requires an interactive confirmation. `manual` changes
only the verified source files and reports the remaining build/restart steps.
`systemd` and `docker-compose` accept only the structured, signed operations in
the release manifest; arbitrary shell hooks are not supported.

## One-command prebuilt installation

Prebuilt Releases contain a Docker image, a Linux amd64 binary with the frontend
embedded, a version-bound installer, and `SHA256SUMS`. The installer selects an
existing component systemd service or Docker Compose application automatically.
The target server does not run Bun, pnpm, `go build`, or `docker build`.

| Channel | Host | Architecture | Compose | systemd | Health |
| --- | --- | --- | --- | --- | --- |
| `sub2api-hamster` | Linux | amd64/x86_64 | Yes | Yes | `/health` |
| `new-api-hamster` | Linux | amd64/x86_64 | Yes | Yes | `/api/status` |

The following command installs `sub2api-hamster` v1.0.2:

```bash
tmp=$(mktemp -d) && cd "$tmp" && curl -fsSLO https://github.com/hamster-switch/server-integrations/releases/download/image-sub2api-hamster-v1.0.2/install-sub2api-hamster-v1.0.2.sh && curl -fsSLO https://github.com/hamster-switch/server-integrations/releases/download/image-sub2api-hamster-v1.0.2/SHA256SUMS && sha256sum -c SHA256SUMS --ignore-missing && sudo bash install-sub2api-hamster-v1.0.2.sh
```

The corresponding New API Hamster v1.0.0 command is:

```bash
tmp=$(mktemp -d) && cd "$tmp" && curl -fsSLO https://github.com/hamster-switch/server-integrations/releases/download/image-new-api-hamster-v1.0.0/install-new-api-hamster-v1.0.0.sh && curl -fsSLO https://github.com/hamster-switch/server-integrations/releases/download/image-new-api-hamster-v1.0.0/SHA256SUMS && sha256sum -c SHA256SUMS --ignore-missing && sudo bash install-new-api-hamster-v1.0.0.sh
```

### systemd deployments

When the component service is loaded, the installer reads its `ExecStart` path and
downloads the verified Linux amd64 binary. It preserves the existing owner and
mode, keeps a timestamped backup beside the binary, performs an atomic
replacement, restarts the service, and checks the component health endpoint. Any replacement,
restart, or health failure restores the previous binary. The service unit,
environment file, database, Redis, and application data are not changed.

The defaults are `sub2api.service`, `SERVER_PORT` or port `8080`, and `/health`
for sub2api; and `new-api.service`, `PORT` or port `3000`, and `/api/status` for
New API. Absolute `ExecStart` binary paths containing spaces are supported.

For a custom unit, binary path, or health endpoint:

```bash
sudo bash install-sub2api-hamster-v1.0.2.sh --mode systemd --systemd-service custom-sub2api.service --binary /opt/sub2api/sub2api --health-url http://127.0.0.1:8080/health
sudo bash install-new-api-hamster-v1.0.0.sh --mode systemd --systemd-service custom-new-api.service --binary /opt/new-api/new-api --health-url http://127.0.0.1:3000/api/status
```

The systemd binary currently supports Linux x86_64 (`amd64`).

The public prebuilt binary is compiled from the exact upstream commit recorded
in the signed source Release. Use it only when the installation keeps that
upstream migration history. A private fork or theme that changed an already
applied migration must build its own binary from the patched fork on a build
machine; do not edit `schema_migrations` checksums to force the public binary to
start. The installer restores the previous binary if startup detects this kind
of incompatibility.

### Docker Compose deployments

When no matching systemd service is loaded, the installer detects the running
component container even when a custom image changes its Compose service name,
then reads the real project path from Docker labels. It verifies the image,
changes only the selected application service, recreates it with `--no-build`,
waits for health, and restores the Compose backup on failure. New API images
without a Docker healthcheck are probed inside the container at `/api/status`.
PostgreSQL, Redis, volumes, and unrelated services are never rewritten.

If more than one candidate exists, or the container is stopped, list the
Compose metadata:

```bash
sudo docker ps -a --format 'table {{.Names}}\t{{.Image}}\t{{.Label "com.docker.compose.service"}}\t{{.Label "com.docker.compose.project.working_dir"}}\t{{.Label "com.docker.compose.project.config_files"}}'
```

Then rerun the downloaded installer with the reported absolute Compose file
and service name:

```bash
sudo bash install-sub2api-hamster-v1.0.2.sh --mode compose --compose-file /absolute/path/to/docker-compose.yml --service sub2api
sudo bash install-new-api-hamster-v1.0.0.sh --mode compose --compose-file /absolute/path/to/docker-compose.yml --service new-api
```

For a relative `config_files` label, resolve it below the reported
`project.working_dir`. Plain `docker run` deployments are not modified because
they have neither a systemd unit nor restorable Compose metadata.

Each successful installation prints the timestamped backup path. For systemd,
restore that file to the `ExecStart` binary and restart the unit. For Compose,
restore the adjacent `*.hamster-switch.*.bak` file and run
`docker compose up -d --no-build <service>`. Confirm the active deployment with
`systemctl status <service>` or `docker compose ps`, then request the health
endpoint. Check the actual Compose image with
`docker inspect --format '{{.Config.Image}}' <container>`; do not rely only on a
stale Compose file or local image tag.

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
