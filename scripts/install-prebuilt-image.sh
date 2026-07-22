#!/usr/bin/env bash

set -euo pipefail

readonly REPOSITORY="hamster-switch/server-integrations"
readonly COMPONENT="__COMPONENT__"
readonly VERSION="__VERSION__"
readonly IMAGE="hamster-switch/${COMPONENT}:hs-v${VERSION}"
readonly RELEASE_TAG="image-${COMPONENT}-v${VERSION}"
readonly IMAGE_ASSET="${COMPONENT}-image-v${VERSION}.tar.gz"
readonly RELEASE_BASE_URL="https://github.com/${REPOSITORY}/releases/download/${RELEASE_TAG}"

TARGET_DIR="/srv/${COMPONENT}"
COMPOSE_FILE=""
TEMP_DIR=""
NEXT_COMPOSE=""
BACKUP_FILE=""
COMPOSE_CHANGED=0

usage() {
  cat <<EOF
Usage: $0 [--target ABSOLUTE_PATH] [--compose-file ABSOLUTE_PATH]

Loads the verified ${IMAGE} image, switches only the ${COMPONENT} Compose
service to that image, recreates it without building, and waits for health.
EOF
}

fail() {
  echo "install-prebuilt-image: $*" >&2
  exit 1
}

cleanup() {
  if [[ -n "${NEXT_COMPOSE}" ]]; then
    rm -f -- "${NEXT_COMPOSE}"
  fi
  if [[ -n "${TEMP_DIR}" ]]; then
    rm -rf -- "${TEMP_DIR}"
  fi
}

trap cleanup EXIT

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)
      [[ $# -ge 2 ]] || fail "--target requires a value"
      TARGET_DIR="$2"
      shift 2
      ;;
    --compose-file)
      [[ $# -ge 2 ]] || fail "--compose-file requires a value"
      COMPOSE_FILE="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

[[ "$(id -u)" -eq 0 ]] || fail "run this installer as root (for example with sudo)"
[[ "${TARGET_DIR}" == /* ]] || fail "--target must be an absolute path"
[[ -d "${TARGET_DIR}" ]] || fail "target directory does not exist: ${TARGET_DIR}"

for command_name in awk basename chmod chown cmp cp curl date dirname docker gzip mktemp mv sha256sum; do
  command -v "${command_name}" >/dev/null 2>&1 || fail "missing required command: ${command_name}"
done

if [[ -z "${COMPOSE_FILE}" ]]; then
  for candidate in \
    "${TARGET_DIR}/deploy/docker-compose.yml" \
    "${TARGET_DIR}/docker-compose.yml" \
    "${TARGET_DIR}/compose.yml"; do
    if [[ -f "${candidate}" ]]; then
      COMPOSE_FILE="${candidate}"
      break
    fi
  done
fi

[[ -n "${COMPOSE_FILE}" ]] || fail "no Compose file found below ${TARGET_DIR}"
[[ "${COMPOSE_FILE}" == /* ]] || fail "--compose-file must be an absolute path"
[[ -f "${COMPOSE_FILE}" ]] || fail "Compose file does not exist: ${COMPOSE_FILE}"

if docker compose version >/dev/null 2>&1; then
  COMPOSE_COMMAND=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE_COMMAND=(docker-compose)
else
  fail "Docker Compose is not available"
fi

rewrite_compose() {
  awk -v service="${COMPONENT}" -v image="${IMAGE}" '
    BEGIN { in_service = 0; changed = 0 }
    $0 ~ ("^  " service ":[[:space:]]*$") {
      in_service = 1
      print
      next
    }
    in_service && $0 ~ /^  [[:alnum:]_.-]+:[[:space:]]*$/ {
      in_service = 0
    }
    in_service && $0 ~ /^[[:space:]]+image:[[:space:]]*/ {
      prefix = $0
      sub(/image:.*/, "", prefix)
      print prefix "image: " image
      changed++
      next
    }
    { print }
    END {
      if (changed != 1) {
        exit 42
      }
    }
  ' "${COMPOSE_FILE}"
}

NEXT_COMPOSE="${COMPOSE_FILE}.hamster-switch.$$.tmp"
if ! rewrite_compose >"${NEXT_COMPOSE}"; then
  fail "expected exactly one image field in Compose service ${COMPONENT}"
fi
chmod --reference="${COMPOSE_FILE}" "${NEXT_COMPOSE}"
chown --reference="${COMPOSE_FILE}" "${NEXT_COMPOSE}"

TEMP_DIR="$(mktemp -d)"
readonly CHECKSUMS_FILE="${TEMP_DIR}/SHA256SUMS"
readonly IMAGE_ARCHIVE="${TEMP_DIR}/${IMAGE_ASSET}"

echo "Downloading ${IMAGE_ASSET}..."
curl --fail --location --silent --show-error --retry 3 \
  --connect-timeout 15 --max-time 1800 \
  --output "${CHECKSUMS_FILE}" "${RELEASE_BASE_URL}/SHA256SUMS"
curl --fail --location --silent --show-error --retry 3 \
  --connect-timeout 15 --max-time 1800 \
  --output "${IMAGE_ARCHIVE}" "${RELEASE_BASE_URL}/${IMAGE_ASSET}"

expected_sha256="$(awk -v name="${IMAGE_ASSET}" '$2 == name || $2 == "*" name { print $1 }' "${CHECKSUMS_FILE}")"
[[ "${expected_sha256}" =~ ^[0-9a-f]{64}$ ]] || fail "SHA256SUMS does not contain ${IMAGE_ASSET}"
actual_sha256="$(sha256sum "${IMAGE_ARCHIVE}" | awk '{ print $1 }')"
[[ "${actual_sha256}" == "${expected_sha256}" ]] || fail "image SHA-256 verification failed"
gzip -t "${IMAGE_ARCHIVE}"

echo "Loading ${IMAGE}..."
gzip -dc "${IMAGE_ARCHIVE}" | docker load
docker image inspect "${IMAGE}" >/dev/null 2>&1 || fail "loaded archive does not contain ${IMAGE}"

if ! cmp -s "${COMPOSE_FILE}" "${NEXT_COMPOSE}"; then
  BACKUP_FILE="${COMPOSE_FILE}.hamster-switch.$(date -u +%Y%m%dT%H%M%SZ).bak"
  cp -p -- "${COMPOSE_FILE}" "${BACKUP_FILE}"
  mv -- "${NEXT_COMPOSE}" "${COMPOSE_FILE}"
  NEXT_COMPOSE=""
  COMPOSE_CHANGED=1
fi

compose_dir="$(dirname "${COMPOSE_FILE}")"
compose_name="$(basename "${COMPOSE_FILE}")"

rollback_compose() {
  if [[ "${COMPOSE_CHANGED}" -eq 1 && -f "${BACKUP_FILE}" ]]; then
    echo "Restoring Compose backup ${BACKUP_FILE}..." >&2
    cp -p -- "${BACKUP_FILE}" "${COMPOSE_FILE}"
    (
      cd "${compose_dir}"
      "${COMPOSE_COMMAND[@]}" -f "${compose_name}" up -d --no-build "${COMPONENT}"
    ) || echo "warning: failed to restart the previous image" >&2
  fi
}

echo "Recreating ${COMPONENT} without a local build..."
if ! (
  cd "${compose_dir}"
  "${COMPOSE_COMMAND[@]}" -f "${compose_name}" up -d --no-build "${COMPONENT}"
); then
  rollback_compose
  fail "Docker Compose failed; the previous Compose file was restored"
fi

container_id="$(
  cd "${compose_dir}"
  "${COMPOSE_COMMAND[@]}" -f "${compose_name}" ps -q "${COMPONENT}"
)"
if [[ -z "${container_id}" ]]; then
  rollback_compose
  fail "Compose did not return a container for ${COMPONENT}"
fi

for _ in {1..60}; do
  health_status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "${container_id}" 2>/dev/null || true)"
  if [[ "${health_status}" == "healthy" || "${health_status}" == "running" ]]; then
    echo "Installed ${IMAGE}; container status: ${health_status}"
    exit 0
  fi
  if [[ "${health_status}" == "unhealthy" || "${health_status}" == "exited" || "${health_status}" == "dead" ]]; then
    break
  fi
  sleep 2
done

rollback_compose
fail "new container did not become healthy; the previous Compose file was restored"
