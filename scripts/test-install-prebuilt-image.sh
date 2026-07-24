#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
test_root="$(mktemp -d)"
trap 'rm -rf -- "${test_root}"' EXIT

if LC_ALL=C grep -q $'\r' "${repo_root}/scripts/install-prebuilt-image.sh"; then
  echo 'installer template must use LF line endings' >&2
  exit 1
fi

installer="${test_root}/install-sub2api-v1.2.3.sh"
sed \
  -e 's/__COMPONENT__/sub2api/g' \
  -e 's/__CHANNEL__//g' \
  -e 's/__VERSION__/1.2.3/g' \
  -e 's/__DEFAULT_PORT__/8080/g' \
  -e 's/__PORT_ENV_NAME__/SERVER_PORT/g' \
  -e 's#__HEALTH_PATH__#/health#g' \
  -e 's/__REQUIRE_HTTP_HEALTH__/0/g' \
  "${repo_root}/scripts/install-prebuilt-image.sh" >"${installer}"
chmod 0755 "${installer}"
bash -n "${installer}"

hamster_installer="${test_root}/install-sub2api-hamster-v1.2.3.sh"
sed \
  -e 's/__COMPONENT__/sub2api/g' \
  -e 's/__CHANNEL__/hamster/g' \
  -e 's/__VERSION__/1.2.3/g' \
  -e 's/__DEFAULT_PORT__/8080/g' \
  -e 's/__PORT_ENV_NAME__/SERVER_PORT/g' \
  -e 's#__HEALTH_PATH__#/health#g' \
  -e 's/__REQUIRE_HTTP_HEALTH__/0/g' \
  "${repo_root}/scripts/install-prebuilt-image.sh" >"${hamster_installer}"
bash -n "${hamster_installer}"
bash "${hamster_installer}" --help | grep -q 'hamster-switch/sub2api:hamster-v1.2.3'
if grep -q '__CHANNEL__' "${hamster_installer}"; then
  echo 'channel placeholder was not replaced' >&2
  exit 1
fi

new_api_installer="${test_root}/install-new-api-hamster-v1.2.3.sh"
sed \
  -e 's/__COMPONENT__/new-api/g' \
  -e 's/__CHANNEL__/hamster/g' \
  -e 's/__VERSION__/1.2.3/g' \
  -e 's/__DEFAULT_PORT__/3000/g' \
  -e 's/__PORT_ENV_NAME__/PORT/g' \
  -e 's#__HEALTH_PATH__#/api/status#g' \
  -e 's/__REQUIRE_HTTP_HEALTH__/1/g' \
  "${repo_root}/scripts/install-prebuilt-image.sh" >"${new_api_installer}"
chmod 0755 "${new_api_installer}"
bash -n "${new_api_installer}"
bash "${new_api_installer}" --help | grep -q 'hamster-switch/new-api:hamster-v1.2.3'
bash "${new_api_installer}" --help | grep -q 'PORT or port 3000, path /api/status'
if grep -Eq '__[A-Z_]+__' "${new_api_installer}"; then
  echo 'new-api installer contains an unreplaced placeholder' >&2
  exit 1
fi

fixture_root="${test_root}/release"
fake_bin="${test_root}/bin"
mkdir -p "${fixture_root}" "${fake_bin}"
printf 'fake docker image archive\n' | gzip >"${fixture_root}/sub2api-image-v1.2.3.tar.gz"
printf 'fake hamster channel image archive\n' | gzip >"${fixture_root}/sub2api-hamster-image-v1.2.3.tar.gz"
printf 'patched systemd binary\n' | gzip >"${fixture_root}/sub2api-linux-amd64-v1.2.3.gz"
printf 'patched hamster systemd binary\n' | gzip >"${fixture_root}/sub2api-hamster-linux-amd64-v1.2.3.gz"
printf 'fake new-api hamster image archive\n' | gzip >"${fixture_root}/new-api-hamster-image-v1.2.3.tar.gz"
printf 'patched new-api systemd binary\n' | gzip >"${fixture_root}/new-api-hamster-linux-amd64-v1.2.3.gz"
(
  cd "${fixture_root}"
  sha256sum \
    sub2api-image-v1.2.3.tar.gz \
    sub2api-hamster-image-v1.2.3.tar.gz \
    sub2api-linux-amd64-v1.2.3.gz \
    sub2api-hamster-linux-amd64-v1.2.3.gz \
    new-api-hamster-image-v1.2.3.tar.gz \
    new-api-hamster-linux-amd64-v1.2.3.gz >SHA256SUMS
)

cat >"${fake_bin}/id" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == "-u" ]]; then
  echo 0
  exit 0
fi
exec /usr/bin/id "$@"
EOF

cat >"${fake_bin}/chown" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

cat >"${fake_bin}/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
output=""
url=""
write_out=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    --write-out)
      write_out="$2"
      shift 2
      ;;
    http://*|https://*)
      url="$1"
      shift
      ;;
    *)
      shift
      ;;
  esac
done
[[ -n "${output}" && -n "${url}" ]]
if [[ -n "${write_out}" ]]; then
  if [[ -n "${CURL_LOG:-}" ]]; then
    echo "${url}" >>"${CURL_LOG}"
  fi
  printf '%s' "${MOCK_HTTP_STATUS:-200}"
  exit 0
fi
cp "${FIXTURE_ROOT}/${url##*/}" "${output}"
EOF

cat >"${fake_bin}/uname" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == "-m" ]]; then
  echo "${MOCK_UNAME:-x86_64}"
  exit 0
fi
exec /usr/bin/uname "$@"
EOF

cat >"${fake_bin}/sleep" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

cat >"${fake_bin}/systemctl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ -n "${SYSTEMD_LOG:-}" ]]; then
  echo "$*" >>"${SYSTEMD_LOG}"
fi
if [[ "$*" == *"--property=LoadState --value"* ]]; then
  if [[ "${MOCK_SYSTEMD_LOADED:-0}" == "1" ]]; then
    echo loaded
  else
    echo not-found
  fi
  exit 0
fi
if [[ "$*" == *"--property=ExecStart --value"* ]]; then
  echo "{ path=${MOCK_BINARY_PATH}; argv[]=${MOCK_BINARY_PATH}; ignore_errors=no ; }"
  exit 0
fi
if [[ "$*" == *"--property=EnvironmentFiles --value"* ]]; then
  echo "${MOCK_ENV_FILE:-}"
  exit 0
fi
case "${1:-}" in
  is-active)
    [[ "$(cat "${SYSTEMD_STATE_FILE}")" == active ]]
    ;;
  stop)
    echo inactive >"${SYSTEMD_STATE_FILE}"
    ;;
  start)
    echo active >"${SYSTEMD_STATE_FILE}"
    ;;
  *)
    exit 0
    ;;
esac
EOF

cat >"${fake_bin}/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "$*" >>"${DOCKER_LOG}"
if [[ "$*" == "compose version" ]]; then
  exit 0
fi
if [[ "$*" == "ps --format {{.ID}}|{{.Image}}|{{.Names}}|{{.Label \"com.docker.compose.service\"}}" ]]; then
  if [[ "${MOCK_MULTIPLE_CONTAINERS:-0}" == "1" ]]; then
    printf 'container-one|hamster-switch/sub2api:hs-v1.0.0|sub2api-one|sub2api\ncontainer-two|hamster-switch/sub2api:hs-v1.0.0|sub2api-two|sub2api\n'
  elif [[ -n "${MOCK_COMPOSE_TARGET:-}" ]]; then
    echo "running-container-id|${MOCK_IMAGE:-hamster-switch/sub2api:hs-v1.0.0}|${MOCK_CONTAINER_NAME:-sub2api}|${MOCK_SERVICE:-sub2api}"
  fi
  exit 0
fi
if [[ "$*" == inspect*com.docker.compose.project.working_dir* ]]; then
  echo "${MOCK_COMPOSE_TARGET}"
  exit 0
fi
if [[ "$*" == inspect*com.docker.compose.project* && "$*" != *config_files* && "$*" != *working_dir* ]]; then
  echo 'original-project-name'
  exit 0
fi
if [[ "$*" == inspect*com.docker.compose.project.config_files* ]]; then
  echo "${MOCK_COMPOSE_TARGET}/deploy/docker-compose.yml"
  exit 0
fi
if [[ "$*" == inspect*com.docker.compose.service* ]]; then
  echo "${MOCK_SERVICE:-sub2api}"
  exit 0
fi
if [[ "$*" == "load" ]]; then
  cat >/dev/null
  exit 0
fi
if [[ "$*" == image\ inspect* ]]; then
  exit 0
fi
if [[ "$*" == compose*" ps -q "* ]]; then
  echo fake-container-id
  exit 0
fi
if [[ "$*" == inspect\ --format* ]]; then
  echo "${MOCK_HEALTH:-healthy}"
  exit 0
fi
exit 0
EOF

chmod 0755 "${fake_bin}"/*

systemd_success="${test_root}/systemd-success"
systemd_success_log="${test_root}/systemd-success.log"
systemd_success_curl_log="${test_root}/systemd-success-curl.log"
systemd_success_state="${test_root}/systemd-success.state"
mkdir -p "${systemd_success}"
printf 'previous systemd binary\n' >"${systemd_success}/sub2api"
chmod 0755 "${systemd_success}/sub2api"
printf 'SERVER_PORT=8181\n' >"${systemd_success}/sub2api.env"
echo active >"${systemd_success_state}"
FIXTURE_ROOT="${fixture_root}" MOCK_SYSTEMD_LOADED=1 \
  MOCK_BINARY_PATH="${systemd_success}/sub2api" \
  MOCK_ENV_FILE="${systemd_success}/sub2api.env (ignore_errors=no)" \
  SYSTEMD_LOG="${systemd_success_log}" SYSTEMD_STATE_FILE="${systemd_success_state}" \
  CURL_LOG="${systemd_success_curl_log}" MOCK_HTTP_STATUS=200 \
  PATH="${fake_bin}:${PATH}" \
  bash "${hamster_installer}"

grep -q 'patched hamster systemd binary' "${systemd_success}/sub2api"
grep -q 'stop sub2api.service' "${systemd_success_log}"
grep -q 'start sub2api.service' "${systemd_success_log}"
grep -q 'http://127.0.0.1:8181/health' "${systemd_success_curl_log}"
compgen -G "${systemd_success}/sub2api.hamster-switch.*.bak" >/dev/null
[[ "$(cat "${systemd_success_state}")" == active ]]

new_api_systemd="${test_root}/new api systemd success"
new_api_systemd_log="${test_root}/new-api-systemd-success.log"
new_api_systemd_curl_log="${test_root}/new-api-systemd-success-curl.log"
new_api_systemd_state="${test_root}/new-api-systemd-success.state"
new_api_systemd_env="${test_root}/new-api-systemd.env"
mkdir -p "${new_api_systemd}"
printf 'previous new-api binary\n' >"${new_api_systemd}/new-api"
chmod 0755 "${new_api_systemd}/new-api"
printf 'PORT=3300\n' >"${new_api_systemd_env}"
echo active >"${new_api_systemd_state}"
FIXTURE_ROOT="${fixture_root}" MOCK_SYSTEMD_LOADED=1 \
  MOCK_BINARY_PATH="${new_api_systemd}/new-api" \
  MOCK_ENV_FILE="${new_api_systemd_env} (ignore_errors=no)" \
  SYSTEMD_LOG="${new_api_systemd_log}" SYSTEMD_STATE_FILE="${new_api_systemd_state}" \
  CURL_LOG="${new_api_systemd_curl_log}" MOCK_HTTP_STATUS=200 \
  PATH="${fake_bin}:${PATH}" \
  bash "${new_api_installer}"

grep -q 'patched new-api systemd binary' "${new_api_systemd}/new-api"
grep -q 'stop new-api.service' "${new_api_systemd_log}"
grep -q 'start new-api.service' "${new_api_systemd_log}"
grep -q 'http://127.0.0.1:3300/api/status' "${new_api_systemd_curl_log}"
compgen -G "${new_api_systemd}/new-api.hamster-switch.*.bak" >/dev/null

new_api_wrong_arch="${test_root}/new-api-wrong-arch"
mkdir -p "${new_api_wrong_arch}"
printf 'previous new-api binary\n' >"${new_api_wrong_arch}/new-api"
chmod 0755 "${new_api_wrong_arch}/new-api"
if FIXTURE_ROOT="${fixture_root}" MOCK_UNAME=aarch64 \
  PATH="${fake_bin}:${PATH}" \
  bash "${new_api_installer}" --mode systemd --binary "${new_api_wrong_arch}/new-api"; then
  echo 'expected non-x86_64 new-api installation to fail' >&2
  exit 1
fi
grep -q 'previous new-api binary' "${new_api_wrong_arch}/new-api"

systemd_rollback="${test_root}/systemd-rollback"
systemd_rollback_log="${test_root}/systemd-rollback.log"
systemd_rollback_state="${test_root}/systemd-rollback.state"
mkdir -p "${systemd_rollback}"
printf 'previous systemd binary\n' >"${systemd_rollback}/sub2api"
chmod 0755 "${systemd_rollback}/sub2api"
echo active >"${systemd_rollback_state}"
if FIXTURE_ROOT="${fixture_root}" MOCK_SYSTEMD_LOADED=1 \
  MOCK_BINARY_PATH="${systemd_rollback}/sub2api" \
  SYSTEMD_LOG="${systemd_rollback_log}" SYSTEMD_STATE_FILE="${systemd_rollback_state}" \
  MOCK_HTTP_STATUS=503 PATH="${fake_bin}:${PATH}" \
  bash "${hamster_installer}" --mode systemd --health-url http://127.0.0.1:9090/health; then
  echo 'expected unhealthy systemd installation to fail' >&2
  exit 1
fi

grep -q 'previous systemd binary' "${systemd_rollback}/sub2api"
[[ "$(grep -c 'start sub2api.service' "${systemd_rollback_log}")" -eq 2 ]]
[[ "$(cat "${systemd_rollback_state}")" == active ]]

write_compose() {
  local target="$1"
  local service="${2:-sub2api}"
  local image="${3:-hamster-switch/sub2api:hs-v1.0.0}"
  mkdir -p "${target}/deploy"
  cat >"${target}/deploy/docker-compose.yml" <<EOF
services:
  ${service}:
    image: ${image}
    restart: unless-stopped
  postgres:
    image: postgres:18-alpine
EOF
}

success_target="${test_root}/success"
success_log="${test_root}/success-docker.log"
write_compose "${success_target}"
FIXTURE_ROOT="${fixture_root}" DOCKER_LOG="${success_log}" MOCK_HEALTH=healthy \
  MOCK_COMPOSE_TARGET="${success_target}" \
  PATH="${fake_bin}:${PATH}" \
  bash "${installer}"

grep -q 'image: hamster-switch/sub2api:hs-v1.2.3' "${success_target}/deploy/docker-compose.yml"
grep -q 'image: postgres:18-alpine' "${success_target}/deploy/docker-compose.yml"
grep -q 'load' "${success_log}"
grep -q 'compose -p original-project-name -f .* up -d --no-build sub2api' "${success_log}"
compgen -G "${success_target}/deploy/docker-compose.yml.hamster-switch.*.bak" >/dev/null

hamster_target="${test_root}/hamster-success"
hamster_log="${test_root}/hamster-success-docker.log"
write_compose "${hamster_target}"
FIXTURE_ROOT="${fixture_root}" DOCKER_LOG="${hamster_log}" MOCK_HEALTH=healthy \
  MOCK_COMPOSE_TARGET="${hamster_target}" \
  PATH="${fake_bin}:${PATH}" \
  bash "${hamster_installer}"

grep -q 'image: hamster-switch/sub2api:hamster-v1.2.3' "${hamster_target}/deploy/docker-compose.yml"
grep -q 'image: postgres:18-alpine' "${hamster_target}/deploy/docker-compose.yml"
grep -q 'load' "${hamster_log}"

new_api_target="${test_root}/new-api compose success"
new_api_log="${test_root}/new-api-success-docker.log"
write_compose "${new_api_target}" api 'private/new-api-themed:latest'
FIXTURE_ROOT="${fixture_root}" DOCKER_LOG="${new_api_log}" MOCK_HEALTH=running \
  MOCK_COMPOSE_TARGET="${new_api_target}" MOCK_SERVICE=api \
  MOCK_IMAGE='private/new-api-themed:latest' MOCK_CONTAINER_NAME='custom-new-api' \
  PATH="${fake_bin}:${PATH}" \
  bash "${new_api_installer}"

grep -q 'image: hamster-switch/new-api:hamster-v1.2.3' "${new_api_target}/deploy/docker-compose.yml"
grep -q 'image: postgres:18-alpine' "${new_api_target}/deploy/docker-compose.yml"
grep -q 'up -d --no-build api' "${new_api_log}"
grep -q 'exec fake-container-id sh -c' "${new_api_log}"

themed_target="${test_root}/themed-success"
themed_log="${test_root}/themed-success-docker.log"
write_compose "${themed_target}" backend 'private/sub2api-themed:latest'
FIXTURE_ROOT="${fixture_root}" DOCKER_LOG="${themed_log}" MOCK_HEALTH=healthy \
  MOCK_COMPOSE_TARGET="${themed_target}" MOCK_SERVICE=backend \
  MOCK_IMAGE='private/sub2api-themed:latest' MOCK_CONTAINER_NAME='custom-router' \
  PATH="${fake_bin}:${PATH}" \
  bash "${hamster_installer}"

grep -q 'image: hamster-switch/sub2api:hamster-v1.2.3' "${themed_target}/deploy/docker-compose.yml"
grep -q 'image: postgres:18-alpine' "${themed_target}/deploy/docker-compose.yml"
grep -q 'up -d --no-build backend' "${themed_log}"

explicit_target="${test_root}/explicit-success"
explicit_log="${test_root}/explicit-success-docker.log"
write_compose "${explicit_target}" backend 'private/renamed-router:latest'
FIXTURE_ROOT="${fixture_root}" DOCKER_LOG="${explicit_log}" MOCK_HEALTH=healthy \
  PATH="${fake_bin}:${PATH}" \
  bash "${hamster_installer}" \
    --compose-file "${explicit_target}/deploy/docker-compose.yml" \
    --service backend

grep -q 'image: hamster-switch/sub2api:hamster-v1.2.3' "${explicit_target}/deploy/docker-compose.yml"
grep -q 'up -d --no-build backend' "${explicit_log}"

stopped_target="${test_root}/stopped-success"
stopped_log="${test_root}/stopped-success-docker.log"
write_compose "${stopped_target}"
(
  cd "${stopped_target}"
  FIXTURE_ROOT="${fixture_root}" DOCKER_LOG="${stopped_log}" MOCK_HEALTH=healthy \
    PATH="${fake_bin}:${PATH}" \
    bash "${hamster_installer}"
)

grep -q 'image: hamster-switch/sub2api:hamster-v1.2.3' "${stopped_target}/deploy/docker-compose.yml"
grep -q 'up -d --no-build sub2api' "${stopped_log}"

rollback_target="${test_root}/rollback"
rollback_log="${test_root}/rollback-docker.log"
write_compose "${rollback_target}"
if FIXTURE_ROOT="${fixture_root}" DOCKER_LOG="${rollback_log}" MOCK_HEALTH=unhealthy \
  PATH="${fake_bin}:${PATH}" \
  bash "${installer}" --target "${rollback_target}"; then
  echo 'expected unhealthy installation to fail' >&2
  exit 1
fi

grep -q 'image: hamster-switch/sub2api:hs-v1.0.0' "${rollback_target}/deploy/docker-compose.yml"
[[ "$(grep -c 'up -d --no-build sub2api' "${rollback_log}")" -eq 2 ]]

bad_fixture_root="${test_root}/bad-release"
cp -R "${fixture_root}" "${bad_fixture_root}"
printf 'corrupt\n' >>"${bad_fixture_root}/new-api-hamster-image-v1.2.3.tar.gz"
checksum_target="${test_root}/checksum-failure"
checksum_log="${test_root}/checksum-failure-docker.log"
write_compose "${checksum_target}" api 'private/new-api:latest'
if FIXTURE_ROOT="${bad_fixture_root}" DOCKER_LOG="${checksum_log}" MOCK_HEALTH=healthy \
  PATH="${fake_bin}:${PATH}" \
  bash "${new_api_installer}" --mode compose \
    --compose-file "${checksum_target}/deploy/docker-compose.yml" --service api; then
  echo 'expected new-api image checksum verification to fail' >&2
  exit 1
fi
grep -q 'image: private/new-api:latest' "${checksum_target}/deploy/docker-compose.yml"
if grep -q '^load$' "${checksum_log}"; then
  echo 'checksum failure must happen before docker load' >&2
  exit 1
fi

ambiguous_log="${test_root}/ambiguous-docker.log"
if FIXTURE_ROOT="${fixture_root}" DOCKER_LOG="${ambiguous_log}" MOCK_MULTIPLE_CONTAINERS=1 \
  PATH="${fake_bin}:${PATH}" bash "${installer}"; then
  echo 'expected ambiguous auto-detection to fail' >&2
  exit 1
fi

echo 'prebuilt image installer tests passed'
