#!/usr/bin/env bash

set -euo pipefail

readonly REPOSITORY="hamster-switch/server-integrations"
readonly COMPONENT="__COMPONENT__"
readonly CHANNEL="__CHANNEL__"
readonly VERSION="__VERSION__"
if [[ -n "${CHANNEL}" ]]; then
  readonly RELEASE_NAME="${COMPONENT}-${CHANNEL}"
  readonly IMAGE_TAG="${CHANNEL}-v${VERSION}"
else
  readonly RELEASE_NAME="${COMPONENT}"
  readonly IMAGE_TAG="hs-v${VERSION}"
fi
readonly IMAGE="hamster-switch/${COMPONENT}:${IMAGE_TAG}"
readonly RELEASE_TAG="image-${RELEASE_NAME}-v${VERSION}"
readonly IMAGE_ASSET="${RELEASE_NAME}-image-v${VERSION}.tar.gz"
readonly BINARY_ASSET="${RELEASE_NAME}-linux-amd64-v${VERSION}.gz"
readonly RELEASE_BASE_URL="https://github.com/${REPOSITORY}/releases/download/${RELEASE_TAG}"

MODE="auto"
TARGET_DIR=""
COMPOSE_FILE=""
PROJECT_DIR=""
PROJECT_NAME=""
SERVICE_NAME=""
SYSTEMD_SERVICE="${COMPONENT}.service"
BINARY_PATH=""
HEALTH_URL=""
TARGET_EXPLICIT=0
COMPOSE_EXPLICIT=0
SERVICE_EXPLICIT=0
SYSTEMD_EXPLICIT=0
TEMP_DIR=""
NEXT_COMPOSE=""
BACKUP_FILE=""
COMPOSE_CHANGED=0
STAGED_BINARY=""
SYSTEMD_BACKUP=""
SYSTEMD_REPLACED=0
SYSTEMD_WAS_ACTIVE=0

usage() {
  cat <<EOF
Usage: $0 [--mode auto|compose|systemd] [deployment options]

With no arguments, installs the verified prebuilt release into an existing
${COMPONENT} systemd service or Docker Compose project and waits for health.
Docker image: ${IMAGE}
Systemd asset: ${BINARY_ASSET}

Systemd options:
  --systemd-service NAME   Service unit (default: ${COMPONENT}.service)
  --binary ABSOLUTE_PATH  Existing binary to replace (default: ExecStart path)
  --health-url URL         Health endpoint (default: SERVER_PORT or port 8080)

Compose options:
  --target ABSOLUTE_PATH
  --compose-file ABSOLUTE_PATH
  --service NAME
EOF
}

fail() {
  echo "install-prebuilt-image: $*" >&2
  exit 1
}

cleanup() {
  local exit_status=$?
  set +e
  if [[ "${exit_status}" -ne 0 && "${SYSTEMD_REPLACED}" -eq 1 ]]; then
    rollback_systemd
  fi
  if [[ -n "${STAGED_BINARY}" ]]; then
    rm -f -- "${STAGED_BINARY}"
  fi
  if [[ -n "${NEXT_COMPOSE}" ]]; then
    rm -f -- "${NEXT_COMPOSE}"
  fi
  if [[ -n "${TEMP_DIR}" ]]; then
    rm -rf -- "${TEMP_DIR}"
  fi
  return "${exit_status}"
}

trap cleanup EXIT

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode)
      [[ $# -ge 2 ]] || fail "--mode requires a value"
      MODE="$2"
      shift 2
      ;;
    --target)
      [[ $# -ge 2 ]] || fail "--target requires a value"
      TARGET_DIR="$2"
      TARGET_EXPLICIT=1
      shift 2
      ;;
    --compose-file)
      [[ $# -ge 2 ]] || fail "--compose-file requires a value"
      COMPOSE_FILE="$2"
      COMPOSE_EXPLICIT=1
      shift 2
      ;;
    --service)
      [[ $# -ge 2 ]] || fail "--service requires a value"
      SERVICE_NAME="$2"
      SERVICE_EXPLICIT=1
      shift 2
      ;;
    --systemd-service)
      [[ $# -ge 2 ]] || fail "--systemd-service requires a value"
      SYSTEMD_SERVICE="$2"
      SYSTEMD_EXPLICIT=1
      shift 2
      ;;
    --binary)
      [[ $# -ge 2 ]] || fail "--binary requires a value"
      BINARY_PATH="$2"
      SYSTEMD_EXPLICIT=1
      shift 2
      ;;
    --health-url)
      [[ $# -ge 2 ]] || fail "--health-url requires a value"
      HEALTH_URL="$2"
      SYSTEMD_EXPLICIT=1
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

case "${MODE}" in
  auto|compose|systemd) ;;
  *) fail "--mode must be auto, compose, or systemd" ;;
esac

if [[ "${SERVICE_EXPLICIT}" -eq 1 && ! "${SERVICE_NAME}" =~ ^[A-Za-z0-9_.-]+$ ]]; then
  fail "--service contains unsupported characters"
fi
if [[ ! "${SYSTEMD_SERVICE}" =~ ^[A-Za-z0-9_.@-]+\.service$ ]]; then
  fail "--systemd-service must name a .service unit"
fi
if [[ -n "${BINARY_PATH}" && "${BINARY_PATH}" != /* ]]; then
  fail "--binary must be an absolute path"
fi
if [[ -n "${HEALTH_URL}" && ! "${HEALTH_URL}" =~ ^https?://[^[:space:]]+$ ]]; then
  fail "--health-url must be an HTTP or HTTPS URL without spaces"
fi

if [[ "${MODE}" == "systemd" && ( "${TARGET_EXPLICIT}" -eq 1 || "${COMPOSE_EXPLICIT}" -eq 1 || "${SERVICE_EXPLICIT}" -eq 1 ) ]]; then
  fail "Compose options cannot be used with --mode systemd"
fi
if [[ "${MODE}" == "compose" && "${SYSTEMD_EXPLICIT}" -eq 1 ]]; then
  fail "systemd options cannot be used with --mode compose"
fi

[[ "$(id -u)" -eq 0 ]] || fail "run this installer as root (for example with sudo)"

for command_name in awk basename chmod chown cmp cp curl date dirname grep gzip mktemp mv sha256sum tail; do
  command -v "${command_name}" >/dev/null 2>&1 || fail "missing required command: ${command_name}"
done

install_systemd() {
  command -v systemctl >/dev/null 2>&1 || fail "systemctl is not available"
  command -v uname >/dev/null 2>&1 || fail "uname is not available"
  [[ "$(uname -m)" == "x86_64" ]] || fail "the prebuilt systemd binary supports Linux x86_64 only"

  local load_state
  load_state="$(systemctl show "${SYSTEMD_SERVICE}" --property=LoadState --value 2>/dev/null || true)"
  [[ "${load_state}" == "loaded" ]] || fail "systemd service is not loaded: ${SYSTEMD_SERVICE}"

  if [[ -z "${BINARY_PATH}" ]]; then
    local exec_start path_part
    exec_start="$(systemctl show "${SYSTEMD_SERVICE}" --property=ExecStart --value)"
    if [[ "${exec_start}" == *"path="* ]]; then
      path_part="${exec_start#*path=}"
      BINARY_PATH="${path_part%%[ ;]*}"
    fi
  fi
  [[ -n "${BINARY_PATH}" ]] || fail "could not read an absolute binary path from ${SYSTEMD_SERVICE} ExecStart; use --binary"
  [[ "${BINARY_PATH}" == /* ]] || fail "systemd ExecStart binary is not an absolute path: ${BINARY_PATH}"
  [[ -f "${BINARY_PATH}" && ! -L "${BINARY_PATH}" ]] || fail "systemd binary must be an existing regular file, not a symlink: ${BINARY_PATH}"

  if [[ -z "${HEALTH_URL}" ]]; then
    local server_port="8080"
    local environment_files environment_file candidate_port
    environment_files="$(systemctl show "${SYSTEMD_SERVICE}" --property=EnvironmentFiles --value 2>/dev/null || true)"
    for environment_file in ${environment_files}; do
      [[ "${environment_file}" == /* ]] || continue
      [[ -f "${environment_file}" ]] || continue
      candidate_port="$(awk -F= '$1 == "SERVER_PORT" {
        value=$0
        sub(/^[^=]*=/, "", value)
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
        quote=sprintf("%c", 34)
        single_quote=sprintf("%c", 39)
        first=substr(value, 1, 1)
        last=substr(value, length(value), 1)
        if ((first == quote && last == quote) || (first == single_quote && last == single_quote)) {
          value=substr(value, 2, length(value) - 2)
        }
        print value
      }' "${environment_file}" | tail -n 1)"
      if [[ "${candidate_port}" =~ ^[0-9]{1,5}$ && "${candidate_port}" -ge 1 && "${candidate_port}" -le 65535 ]]; then
        server_port="${candidate_port}"
      fi
    done
    HEALTH_URL="http://127.0.0.1:${server_port}/health"
  fi

  TEMP_DIR="$(mktemp -d)"
  local checksums_file="${TEMP_DIR}/SHA256SUMS"
  local binary_archive="${TEMP_DIR}/${BINARY_ASSET}"
  echo "Detected systemd service: ${SYSTEMD_SERVICE}"
  echo "Using binary: ${BINARY_PATH}"
  echo "Using health endpoint: ${HEALTH_URL}"
  echo "Downloading ${BINARY_ASSET}..."
  curl --fail --location --silent --show-error --retry 3 \
    --connect-timeout 15 --max-time 1800 \
    --output "${checksums_file}" "${RELEASE_BASE_URL}/SHA256SUMS"
  curl --fail --location --silent --show-error --retry 3 \
    --connect-timeout 15 --max-time 1800 \
    --output "${binary_archive}" "${RELEASE_BASE_URL}/${BINARY_ASSET}"

  local expected_sha256 actual_sha256
  expected_sha256="$(awk -v name="${BINARY_ASSET}" '$2 == name || $2 == "*" name { print $1 }' "${checksums_file}")"
  [[ "${expected_sha256}" =~ ^[0-9a-f]{64}$ ]] || fail "SHA256SUMS does not contain ${BINARY_ASSET}"
  actual_sha256="$(sha256sum "${binary_archive}" | awk '{ print $1 }')"
  [[ "${actual_sha256}" == "${expected_sha256}" ]] || fail "binary SHA-256 verification failed"
  gzip -t "${binary_archive}"

  STAGED_BINARY="$(mktemp "${BINARY_PATH}.hamster-switch.XXXXXX")"
  gzip -dc "${binary_archive}" >"${STAGED_BINARY}"
  [[ -s "${STAGED_BINARY}" ]] || fail "decompressed binary is empty"
  chmod --reference="${BINARY_PATH}" "${STAGED_BINARY}"
  chown --reference="${BINARY_PATH}" "${STAGED_BINARY}"

  SYSTEMD_BACKUP="${BINARY_PATH}.hamster-switch.$(date -u +%Y%m%dT%H%M%SZ).bak"
  cp -p -- "${BINARY_PATH}" "${SYSTEMD_BACKUP}"
  if systemctl is-active --quiet "${SYSTEMD_SERVICE}"; then
    SYSTEMD_WAS_ACTIVE=1
  fi
  SYSTEMD_REPLACED=1
  systemctl stop "${SYSTEMD_SERVICE}"
  mv -- "${STAGED_BINARY}" "${BINARY_PATH}"
  STAGED_BINARY=""
  systemctl start "${SYSTEMD_SERVICE}"

  local health_status=""
  for _ in {1..60}; do
    if ! systemctl is-active --quiet "${SYSTEMD_SERVICE}"; then
      break
    fi
    health_status="$(curl --silent --show-error --max-time 5 --output /dev/null --write-out '%{http_code}' "${HEALTH_URL}" || true)"
    if [[ "${health_status}" =~ ^2[0-9][0-9]$ ]]; then
      SYSTEMD_REPLACED=0
      echo "Installed ${RELEASE_NAME} v${VERSION}; ${SYSTEMD_SERVICE} is healthy."
      echo "Previous binary backup: ${SYSTEMD_BACKUP}"
      return 0
    fi
    sleep 2
  done
  fail "${SYSTEMD_SERVICE} did not become healthy at ${HEALTH_URL}"
}

rollback_systemd() {
  [[ -n "${SYSTEMD_BACKUP}" && -f "${SYSTEMD_BACKUP}" && -n "${BINARY_PATH}" ]] || return 0
  echo "Restoring systemd binary backup ${SYSTEMD_BACKUP}..." >&2
  systemctl stop "${SYSTEMD_SERVICE}" >/dev/null 2>&1 || true
  local restore_path="${BINARY_PATH}.hamster-switch.restore.$$"
  cp -p -- "${SYSTEMD_BACKUP}" "${restore_path}" || return 0
  mv -- "${restore_path}" "${BINARY_PATH}" || return 0
  if [[ "${SYSTEMD_WAS_ACTIVE}" -eq 1 ]]; then
    systemctl start "${SYSTEMD_SERVICE}" || echo "warning: failed to restart the previous systemd binary" >&2
  fi
  SYSTEMD_REPLACED=0
}

if [[ "${MODE}" == "systemd" || ( "${MODE}" == "auto" && "${SYSTEMD_EXPLICIT}" -eq 1 ) ]]; then
  install_systemd
  exit 0
fi

if [[ "${MODE}" == "auto" && "${TARGET_EXPLICIT}" -eq 0 && "${COMPOSE_EXPLICIT}" -eq 0 && "${SERVICE_EXPLICIT}" -eq 0 ]] && command -v systemctl >/dev/null 2>&1; then
  if [[ "$(systemctl show "${SYSTEMD_SERVICE}" --property=LoadState --value 2>/dev/null || true)" == "loaded" ]]; then
    install_systemd
    exit 0
  fi
fi

command -v docker >/dev/null 2>&1 || fail "Docker is not available and no ${COMPONENT} systemd service was detected"

if docker compose version >/dev/null 2>&1; then
  COMPOSE_COMMAND=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE_COMMAND=(docker-compose)
else
  fail "Docker Compose is not available"
fi

if [[ "${COMPOSE_EXPLICIT}" -eq 1 ]]; then
  [[ "${COMPOSE_FILE}" == /* ]] || fail "--compose-file must be an absolute path"
  [[ -f "${COMPOSE_FILE}" ]] || fail "Compose file does not exist: ${COMPOSE_FILE}"
  if [[ "${TARGET_EXPLICIT}" -eq 0 ]]; then
    TARGET_DIR="$(dirname "${COMPOSE_FILE}")"
  fi
fi

if [[ "${TARGET_EXPLICIT}" -eq 0 && "${COMPOSE_EXPLICIT}" -eq 0 ]]; then
  mapfile -t compose_containers < <(
    docker ps --format '{{.ID}}|{{.Image}}|{{.Names}}|{{.Label "com.docker.compose.service"}}' |
      awk -F '|' -v component="${COMPONENT}" -v requested_service="${SERVICE_NAME}" '
        $4 == component || (requested_service != "" && $4 == requested_service) ||
        $3 == component || index($2, component) > 0 { print $1 }
      '
  )
  if [[ "${#compose_containers[@]}" -gt 1 ]]; then
    fail "multiple running ${COMPONENT} containers found; use --compose-file and --service to select one"
  fi
  if [[ "${#compose_containers[@]}" -eq 1 ]]; then
    container_id="${compose_containers[0]}"
    PROJECT_DIR="$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.project.working_dir" }}' "${container_id}")"
    TARGET_DIR="${PROJECT_DIR}"
    PROJECT_NAME="$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.project" }}' "${container_id}")"
    compose_files="$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.project.config_files" }}' "${container_id}")"
    detected_service="$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.service" }}' "${container_id}")"
    [[ "${TARGET_DIR}" != "<no value>" ]] || TARGET_DIR=""
    [[ "${PROJECT_NAME}" != "<no value>" ]] || PROJECT_NAME=""
    [[ "${compose_files}" != "<no value>" ]] || compose_files=""
    [[ "${detected_service}" != "<no value>" ]] || detected_service=""
    [[ -n "${detected_service}" ]] || fail "the detected ${COMPONENT} container is not managed by Docker Compose"
    if [[ "${SERVICE_EXPLICIT}" -eq 1 && "${SERVICE_NAME}" != "${detected_service}" ]]; then
      fail "--service ${SERVICE_NAME} does not match detected Compose service ${detected_service}"
    fi
    SERVICE_NAME="${detected_service}"
    COMPOSE_FILE="${compose_files%%,*}"
    if [[ -n "${COMPOSE_FILE}" && "${COMPOSE_FILE}" != /* ]]; then
      COMPOSE_FILE="${TARGET_DIR}/${COMPOSE_FILE}"
    fi
  fi
fi

if [[ -z "${SERVICE_NAME}" ]]; then
  SERVICE_NAME="${COMPONENT}"
fi

if [[ -z "${TARGET_DIR}" ]]; then
  declare -A seen_compose_files=()
  filesystem_matches=()
  for candidate in \
    "${PWD}" \
    "/srv/${COMPONENT}" \
    "/opt/${COMPONENT}" \
    "/data/${COMPONENT}" \
    "/var/lib/${COMPONENT}" \
    "/root/${COMPONENT}" \
    /home/*/"${COMPONENT}"; do
    if [[ -d "${candidate}" ]]; then
      for compose_candidate in "${candidate}/deploy/docker-compose.yml" "${candidate}/docker-compose.yml" "${candidate}/compose.yml" "${candidate}/compose.yaml"; do
        if [[ -f "${compose_candidate}" ]] && grep -Eq "^[[:space:]]{2}${SERVICE_NAME}:[[:space:]]*$" "${compose_candidate}"; then
          if [[ -z "${seen_compose_files[${compose_candidate}]+x}" ]]; then
            seen_compose_files["${compose_candidate}"]=1
            filesystem_matches+=("${candidate}|${compose_candidate}")
          fi
        fi
      done
    fi
  done
  if [[ "${#filesystem_matches[@]}" -gt 1 ]]; then
    fail "multiple ${COMPONENT} Compose projects found; use --compose-file and --service to select one"
  fi
  if [[ "${#filesystem_matches[@]}" -eq 1 ]]; then
    IFS='|' read -r TARGET_DIR COMPOSE_FILE <<<"${filesystem_matches[0]}"
  fi
fi

[[ -n "${TARGET_DIR}" ]] || fail "could not auto-detect the ${COMPONENT} Compose project; use --compose-file /absolute/path/to/compose.yml"
[[ "${TARGET_DIR}" == /* ]] || fail "--target must be an absolute path"
[[ -d "${TARGET_DIR}" ]] || fail "target directory does not exist: ${TARGET_DIR}"

if [[ -z "${COMPOSE_FILE}" ]]; then
  for candidate in \
    "${TARGET_DIR}/deploy/docker-compose.yml" \
    "${TARGET_DIR}/docker-compose.yml" \
    "${TARGET_DIR}/compose.yml" \
    "${TARGET_DIR}/compose.yaml"; do
    if [[ -f "${candidate}" ]]; then
      COMPOSE_FILE="${candidate}"
      break
    fi
  done
fi

[[ -n "${COMPOSE_FILE}" ]] || fail "no Compose file found below ${TARGET_DIR}"
[[ "${COMPOSE_FILE}" == /* ]] || fail "--compose-file must be an absolute path"
[[ -f "${COMPOSE_FILE}" ]] || fail "Compose file does not exist: ${COMPOSE_FILE}"

if [[ -z "${PROJECT_DIR}" ]]; then
  PROJECT_DIR="${TARGET_DIR}"
fi
[[ "${PROJECT_DIR}" == /* && -d "${PROJECT_DIR}" ]] || fail "Compose project working directory is invalid: ${PROJECT_DIR}"

COMPOSE_ARGS=(-f "${COMPOSE_FILE}")
if [[ -n "${PROJECT_NAME}" ]]; then
  COMPOSE_ARGS=(-p "${PROJECT_NAME}" "${COMPOSE_ARGS[@]}")
fi

echo "Detected Compose project: ${TARGET_DIR}"
echo "Using Compose file: ${COMPOSE_FILE}"
echo "Using Compose working directory: ${PROJECT_DIR}"
echo "Using Compose service: ${SERVICE_NAME}"
if [[ -n "${PROJECT_NAME}" ]]; then
  echo "Using Compose project name: ${PROJECT_NAME}"
fi

rewrite_compose() {
  awk -v service="${SERVICE_NAME}" -v image="${IMAGE}" '
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
  fail "expected exactly one image field in Compose service ${SERVICE_NAME}"
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

rollback_compose() {
  if [[ "${COMPOSE_CHANGED}" -eq 1 && -f "${BACKUP_FILE}" ]]; then
    echo "Restoring Compose backup ${BACKUP_FILE}..." >&2
    cp -p -- "${BACKUP_FILE}" "${COMPOSE_FILE}"
    (
      cd "${PROJECT_DIR}"
      "${COMPOSE_COMMAND[@]}" "${COMPOSE_ARGS[@]}" up -d --no-build "${SERVICE_NAME}"
    ) || echo "warning: failed to restart the previous image" >&2
  fi
}

echo "Recreating ${SERVICE_NAME} without a local build..."
if ! (
  cd "${PROJECT_DIR}"
  "${COMPOSE_COMMAND[@]}" "${COMPOSE_ARGS[@]}" up -d --no-build "${SERVICE_NAME}"
); then
  rollback_compose
  fail "Docker Compose failed; the previous Compose file was restored"
fi

container_id="$(
  cd "${PROJECT_DIR}"
  "${COMPOSE_COMMAND[@]}" "${COMPOSE_ARGS[@]}" ps -q "${SERVICE_NAME}"
)"
if [[ -z "${container_id}" ]]; then
  rollback_compose
  fail "Compose did not return a container for ${SERVICE_NAME}"
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
