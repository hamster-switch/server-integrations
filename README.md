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
```

`check` never applies a patch. `update` prints the exact affected files and
deployment actions, then requires an interactive confirmation. `manual` changes
only the verified source files and reports the remaining build/restart steps.
`systemd` and `docker-compose` accept only the structured, signed operations in
the release manifest; arbitrary shell hooks are not supported.

## One-command prebuilt installation

Prebuilt Releases contain a Docker image, a Linux amd64 binary with the frontend
embedded, a version-bound installer, and `SHA256SUMS`. The installer selects an
existing `sub2api.service` or Docker Compose application automatically. The
target server does not run `pnpm`, `go build`, or `docker build`.

The following command installs `sub2api-hamster` v1.0.2:

```bash
tmp=$(mktemp -d) && cd "$tmp" && curl -fsSLO https://github.com/hamster-switch/server-integrations/releases/download/image-sub2api-hamster-v1.0.2/install-sub2api-hamster-v1.0.2.sh && curl -fsSLO https://github.com/hamster-switch/server-integrations/releases/download/image-sub2api-hamster-v1.0.2/SHA256SUMS && sha256sum -c SHA256SUMS --ignore-missing && sudo bash install-sub2api-hamster-v1.0.2.sh
```

### systemd deployments

When `sub2api.service` is loaded, the installer reads its `ExecStart` path and
downloads the verified Linux amd64 binary. It preserves the existing owner and
mode, keeps a timestamped backup beside the binary, performs an atomic
replacement, restarts the service, and checks `/health`. Any replacement,
restart, or health failure restores the previous binary. The service unit,
environment file, database, Redis, and application data are not changed.

For a custom unit, binary path, or health endpoint:

```bash
sudo bash install-sub2api-hamster-v1.0.2.sh --mode systemd --systemd-service custom-sub2api.service --binary /opt/sub2api/sub2api --health-url http://127.0.0.1:8080/health
```

The systemd binary currently supports Linux x86_64 (`amd64`).

### Docker Compose deployments

When no `sub2api.service` is loaded, the installer detects the running sub2api
container even when a custom theme changes its image or Compose service name,
then reads the real project path from Docker labels. It verifies the image,
changes only the selected application service, recreates it with `--no-build`,
waits for health, and restores the Compose backup on failure. PostgreSQL, Redis,
volumes, and unrelated services are never rewritten.

If more than one candidate exists, or the container is stopped, list the
Compose metadata:

```bash
sudo docker ps -a --format 'table {{.Names}}\t{{.Image}}\t{{.Label "com.docker.compose.service"}}\t{{.Label "com.docker.compose.project.working_dir"}}\t{{.Label "com.docker.compose.project.config_files"}}'
```

Then rerun the downloaded installer with the reported absolute Compose file
and service name:

```bash
sudo bash install-sub2api-hamster-v1.0.2.sh --mode compose --compose-file /absolute/path/to/docker-compose.yml --service sub2api
```

For a relative `config_files` label, resolve it below the reported
`project.working_dir`. Plain `docker run` deployments are not modified because
they have neither a systemd unit nor restorable Compose metadata.

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
